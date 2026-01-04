package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ptone/scion-agent/pkg/api"
	"github.com/ptone/scion-agent/pkg/config"
)

func TestGeminiDiscoverAuth(t *testing.T) {
	// Setup a temporary home directory
	tmpHome, err := os.MkdirTemp("", "scion-home-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpHome)

	// Mock HOME environment variable
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	geminiDir := filepath.Join(tmpHome, ".gemini")
	if err := os.MkdirAll(geminiDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Test OAuth discovery via host settings
	settingsPath := filepath.Join(geminiDir, "settings.json")
	settingsData := `{
		"security": {
			"auth": {
				"selectedType": "oauth-personal"
			}
		}
	}`
	if err := os.WriteFile(settingsPath, []byte(settingsData), 0644); err != nil {
		t.Fatal(err)
	}

	oauthCredsPath := filepath.Join(geminiDir, "oauth_creds.json")
	if err := os.WriteFile(oauthCredsPath, []byte(`{"dummy":"creds"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Setup agent home
	agentHome, err := os.MkdirTemp("", "agent-home-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(agentHome)

	g := &GeminiCLI{}
	auth := g.DiscoverAuth(agentHome)
	if auth.OAuthCreds != oauthCredsPath {
		t.Errorf("expected OAuthCreds to be %s, got %s", oauthCredsPath, auth.OAuthCreds)
	}

	// 2. Test OAuth discovery via agent settings (overriding host)
	// Create agent-specific settings.json
	agentGeminiDir := filepath.Join(agentHome, ".gemini")
	os.MkdirAll(agentGeminiDir, 0755)
	agentSettingsPath := filepath.Join(agentGeminiDir, "settings.json")
	os.WriteFile(agentSettingsPath, []byte(`{"security":{"auth":{"selectedType":"gemini-api-key"}}}`), 0644)
	
	auth = g.DiscoverAuth(agentHome)
	// wait, if agent settings says gemini-api-key, and we have oauth-personal on host,
	// DiscoverAuth currently prefers agent setting if present.
	// But it only checks agent settings for "SelectedType".
	// If agent settings has SelectedType="gemini-api-key", it will NOT return OAuthCreds.
	if auth.OAuthCreds != "" {
		t.Errorf("expected OAuthCreds to be empty when requested by agent settings, got %s", auth.OAuthCreds)
	}

	// 3. Test API Key fallback from host settings
	os.Remove(settingsPath)
	os.Remove(agentSettingsPath)
	settingsData = `{
		"apiKey": "test-api-key"
	}`
	if err := os.WriteFile(settingsPath, []byte(settingsData), 0644); err != nil {
		t.Fatal(err)
	}

	// Clear env vars that might interfere
	origApiKey := os.Getenv("GEMINI_API_KEY")
	origGoogleApiKey := os.Getenv("GOOGLE_API_KEY")
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("GOOGLE_API_KEY")
	defer func() {
		os.Setenv("GEMINI_API_KEY", origApiKey)
		os.Setenv("GOOGLE_API_KEY", origGoogleApiKey)
	}()

	auth = g.DiscoverAuth(agentHome)
	if auth.GeminiAPIKey != "test-api-key" {
		t.Errorf("expected GeminiAPIKey to be 'test-api-key', got '%s'", auth.GeminiAPIKey)
	}
}

func TestGeminiHasSystemPrompt(t *testing.T) {
	agentHome, err := os.MkdirTemp("", "agent-home-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(agentHome)

	g := &GeminiCLI{}
	geminiDir := filepath.Join(agentHome, ".gemini")
	if err := os.MkdirAll(geminiDir, 0755); err != nil {
		t.Fatal(err)
	}

	promptPath := filepath.Join(geminiDir, "system_prompt.md")

	// 1. No file
	if g.HasSystemPrompt(agentHome) {
		t.Error("expected HasSystemPrompt to be false when file is missing")
	}

	// 2. Placeholder content
	if err := os.WriteFile(promptPath, []byte("# Placeholder"), 0644); err != nil {
		t.Fatal(err)
	}
	if g.HasSystemPrompt(agentHome) {
		t.Error("expected HasSystemPrompt to be false when content is placeholder")
	}

	// 3. Placeholder content with whitespace
	if err := os.WriteFile(promptPath, []byte("  # Placeholder  \n"), 0644); err != nil {
		t.Fatal(err)
	}
	if g.HasSystemPrompt(agentHome) {
		t.Error("expected HasSystemPrompt to be false when content is placeholder with whitespace")
	}

	// 4. Real content
	if err := os.WriteFile(promptPath, []byte("# My Prompt\nDo something."), 0644); err != nil {
		t.Fatal(err)
	}
	if !g.HasSystemPrompt(agentHome) {
		t.Error("expected HasSystemPrompt to be true when content is real")
	}

	// 5. Empty content
	if err := os.WriteFile(promptPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if g.HasSystemPrompt(agentHome) {
		t.Error("expected HasSystemPrompt to be false when content is empty")
	}
}

func TestGeminiGetCommand(t *testing.T) {
	g := &GeminiCLI{}

	// 1. Normal task
	cmd := g.GetCommand("do something", false, nil)
	expected := []string{"gemini", "--yolo", "--prompt-interactive", "do something"}
	if !reflect.DeepEqual(cmd, expected) {
		t.Errorf("expected %v, got %v", expected, cmd)
	}

	// 2. Empty task
	cmd = g.GetCommand("", false, nil)
	expected = []string{"gemini", "--yolo"}
	if !reflect.DeepEqual(cmd, expected) {
		t.Errorf("expected %v, got %v", expected, cmd)
	}

	// 3. Task with baseArgs
	cmd = g.GetCommand("do something", false, []string{"--foo", "bar"})
	expected = []string{"gemini", "--yolo", "--foo", "bar", "--prompt-interactive", "do something"}
	if !reflect.DeepEqual(cmd, expected) {
		t.Errorf("expected %v, got %v", expected, cmd)
	}

	// 4. Resume
	cmd = g.GetCommand("do something", true, nil)
	expected = []string{"gemini", "--yolo", "--resume"}
	if !reflect.DeepEqual(cmd, expected) {
		t.Errorf("expected %v, got %v", expected, cmd)
	}
}

func TestGeminiProvision(t *testing.T) {
	// Setup temp agent structure
	agentHome, err := os.MkdirTemp("", "agent-home-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(agentHome)

	agentDir := filepath.Dir(agentHome)

	// Write initial scion-agent.json
	initialConfig := `{
		"gemini": {
			"auth_selectedType": "vertex-ai"
		}
	}`
	scionPath := filepath.Join(agentDir, "scion-agent.json")
	if err := os.WriteFile(scionPath, []byte(initialConfig), 0644); err != nil {
		t.Fatal(err)
	}

	g := &GeminiCLI{}
	if err := g.Provision(nil, "test-agent", agentHome, ""); err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	// 1. Verify ~/.gemini/settings.json
	settingsPath := filepath.Join(agentHome, ".gemini", "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Error("settings.json was not created")
	}

	agentSettings, err := config.LoadAgentSettings(settingsPath)
	if err != nil {
		t.Errorf("failed to load generated settings: %v", err)
	}
	if agentSettings.Security.Auth.SelectedType != "vertex-ai" {
		t.Errorf("expected selectedType vertex-ai, got %s", agentSettings.Security.Auth.SelectedType)
	}

	// 2. Verify scion-agent.json updates
	data, err := os.ReadFile(scionPath)
	if err != nil {
		t.Fatal(err)
	}
	var updatedCfg api.ScionConfig
	if err := json.Unmarshal(data, &updatedCfg); err != nil {
		t.Fatal(err)
	}

	if _, ok := updatedCfg.Env["GOOGLE_CLOUD_PROJECT"]; !ok {
		t.Error("expected GOOGLE_CLOUD_PROJECT in env")
	}

	hasGcloud := false
	for _, v := range updatedCfg.Volumes {
		if strings.Contains(v.Target, ".config/gcloud") {
			hasGcloud = true
			break
		}
	}
	if !hasGcloud {
		t.Error("expected .config/gcloud volume")
	}
}

func TestGeminiSettingsUpdateOnStart(t *testing.T) {
	// Setup temp agent structure
	agentHome, err := os.MkdirTemp("", "agent-home-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(agentHome)

	agentDir := filepath.Dir(agentHome)
	
	// 1. Create agent with defaults (no auth type in settings.json yet, or old one)
	geminiDir := filepath.Join(agentHome, ".gemini")
	os.MkdirAll(geminiDir, 0755)
	initialSettings := `{"security":{"auth":{"selectedType":"gemini-api-key"}}}`
	os.WriteFile(filepath.Join(geminiDir, "settings.json"), []byte(initialSettings), 0644)

	// 2. Create scion-agent.json with NEW auth type
	newConfig := `{
		"gemini": {
			"auth_selectedType": "oauth-personal"
		}
	}`
	os.WriteFile(filepath.Join(agentDir, "scion-agent.json"), []byte(newConfig), 0644)

	g := &GeminiCLI{}

	// 3. Run DiscoverAuth - should pick up new type from scion-agent.json
	auth := g.DiscoverAuth(agentHome)
	if auth.SelectedType != "oauth-personal" {
		t.Errorf("DiscoverAuth: expected selectedType oauth-personal, got %s", auth.SelectedType)
	}

	// 4. Run PropagateFiles - should update settings.json
	if err := g.PropagateFiles(agentHome, "root", auth); err != nil {
		t.Fatalf("PropagateFiles failed: %v", err)
	}

	// 5. Verify settings.json updated
	settingsPath := filepath.Join(geminiDir, "settings.json")
	agentSettings, err := config.LoadAgentSettings(settingsPath)
	if err != nil {
		t.Fatalf("failed to load settings: %v", err)
	}
	if agentSettings.Security.Auth.SelectedType != "oauth-personal" {
		t.Errorf("PropagateFiles: expected updated settings to match oauth-personal, got %s", agentSettings.Security.Auth.SelectedType)
	}
}