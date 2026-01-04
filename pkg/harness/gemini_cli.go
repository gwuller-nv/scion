package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ptone/scion-agent/pkg/api"
	"github.com/ptone/scion-agent/pkg/config"
	"github.com/ptone/scion-agent/pkg/util"
)

type GeminiCLI struct{}

func (g *GeminiCLI) Name() string {
	return "gemini"
}

func (g *GeminiCLI) DiscoverAuth(agentHome string) api.AuthConfig {
	auth := api.AuthConfig{
		GeminiAPIKey:         os.Getenv("GEMINI_API_KEY"),
		GoogleAPIKey:         os.Getenv("GOOGLE_API_KEY"),
		VertexAPIKey:         os.Getenv("VERTEX_API_KEY"),
		GoogleAppCredentials: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
		GoogleCloudProject:   os.Getenv("GOOGLE_CLOUD_PROJECT"),
	}

	if auth.GoogleCloudProject == "" {
		auth.GoogleCloudProject = os.Getenv("GCP_PROJECT")
	}

	home, _ := os.UserHomeDir()

	// 1. Check scion-agent.json for overrides
	selectedType := ""
	agentDir := filepath.Dir(agentHome)
	scionAgentPath := filepath.Join(agentDir, "scion-agent.json")
	if data, err := os.ReadFile(scionAgentPath); err == nil {
		var cfg api.ScionConfig
		if err := json.Unmarshal(data, &cfg); err == nil {
			if cfg.Gemini != nil && cfg.Gemini.AuthSelectedType != "" {
				selectedType = cfg.Gemini.AuthSelectedType
			}
		}
	}

	// 2. Check agent settings (from template/provision) if not found in scion-agent.json
	agentSettingsPath := filepath.Join(agentHome, g.DefaultConfigDir(), "settings.json")
	if selectedType == "" {
		if agentSettings, err := config.LoadAgentSettings(agentSettingsPath); err == nil {
			selectedType = agentSettings.Security.Auth.SelectedType
			// We can fallback to keys stored in settings.json if env vars are empty
			// But careful not to overwrite if we already have one from env
			if auth.GeminiAPIKey == "" && auth.GoogleAPIKey == "" && agentSettings.ApiKey != "" {
				auth.GeminiAPIKey = agentSettings.ApiKey
			}
		}
	} else {
		// Even if we found selectedType in scion-agent.json, we might still want to load key from settings.json
		// if we need it for fallback
		if agentSettings, err := config.LoadAgentSettings(agentSettingsPath); err == nil {
			if auth.GeminiAPIKey == "" && auth.GoogleAPIKey == "" && agentSettings.ApiKey != "" {
				auth.GeminiAPIKey = agentSettings.ApiKey
			}
		}
	}

	// 3. Load host settings if we don't have a type yet
	hostSettings, _ := config.GetAgentSettings()

	if selectedType == "" && hostSettings != nil {
		selectedType = hostSettings.Security.Auth.SelectedType
	}

	// 4. Populate auth based on selected type
	auth.SelectedType = selectedType

	switch selectedType {
	case "gemini-api-key":
		if auth.GeminiAPIKey == "" && auth.GoogleAPIKey == "" && hostSettings != nil && hostSettings.ApiKey != "" {
			auth.GeminiAPIKey = hostSettings.ApiKey
		}
	case "oauth-personal":
		oauthPath := filepath.Join(home, g.DefaultConfigDir(), "oauth_creds.json")
		if _, err := os.Stat(oauthPath); err == nil {
			auth.OAuthCreds = oauthPath
		}
	case "vertex-ai":
		// Vertex might need project/location from env (already loaded) or settings
		// but currently we only support env for project/location in DiscoverAuth
	default:
		// Fallback for backward compatibility or undefined type: try key
		if auth.GeminiAPIKey == "" && auth.GoogleAPIKey == "" && hostSettings != nil && hostSettings.ApiKey != "" {
			auth.GeminiAPIKey = hostSettings.ApiKey
		}
	}

	return auth
}

func (g *GeminiCLI) GetEnv(agentName string, agentHome string, unixUsername string, auth api.AuthConfig) map[string]string {
	env := make(map[string]string)

	env["GEMINI_AGENT_NAME"] = agentName
	if g.HasSystemPrompt(agentHome) {
		env["GEMINI_SYSTEM_MD"] = fmt.Sprintf("%s/%s/system_prompt.md", util.GetHomeDir(unixUsername), g.DefaultConfigDir())
	}

	if auth.SelectedType == "gemini-api-key" || auth.SelectedType == "" {
		if auth.GeminiAPIKey != "" {
			env["GEMINI_API_KEY"] = auth.GeminiAPIKey
			env["GEMINI_DEFAULT_AUTH_TYPE"] = "gemini-api-key"
		}
		if auth.GoogleAPIKey != "" {
			env["GOOGLE_API_KEY"] = auth.GoogleAPIKey
			env["GEMINI_DEFAULT_AUTH_TYPE"] = "gemini-api-key"
		}
	}

	if auth.SelectedType == "vertex-ai" {
		if auth.VertexAPIKey != "" {
			env["VERTEX_API_KEY"] = auth.VertexAPIKey
			env["GEMINI_DEFAULT_AUTH_TYPE"] = "vertex-ai"
		}
	}

	if auth.GoogleCloudProject != "" {
		env["GOOGLE_CLOUD_PROJECT"] = auth.GoogleCloudProject
	}

	if auth.GoogleAppCredentials != "" {
		env["GEMINI_DEFAULT_AUTH_TYPE"] = "compute-default-credentials"
		// The path is fixed in PropagateFiles
		env["GOOGLE_APPLICATION_CREDENTIALS"] = fmt.Sprintf("%s/.config/gcp/application_default_credentials.json", util.GetHomeDir(unixUsername))
	}

	if auth.OAuthCreds != "" {
		env["GEMINI_DEFAULT_AUTH_TYPE"] = "oauth-personal"
	}

	return env
}

func (g *GeminiCLI) GetCommand(task string, resume bool, baseArgs []string) []string {
	args := []string{"gemini", "--yolo"}
	if resume {
		args = append(args, "--resume")
	}
	args = append(args, baseArgs...)
	if !resume && task != "" {
		args = append(args, "--prompt-interactive", task)
	}
	return args
}

func (g *GeminiCLI) PropagateFiles(homeDir, unixUsername string, auth api.AuthConfig) error {
	if homeDir == "" {
		return nil
	}

	if auth.SelectedType != "" {
		// Update ~/.gemini/settings.json to match selected type
		geminiSettingsPath := filepath.Join(homeDir, g.DefaultConfigDir(), "settings.json")
		var agentSettings map[string]interface{}
		sData, err := os.ReadFile(geminiSettingsPath)
		if err == nil {
			_ = json.Unmarshal(sData, &agentSettings)
		}
		if agentSettings == nil {
			agentSettings = make(map[string]interface{})
		}

		if _, ok := agentSettings["security"]; !ok {
			agentSettings["security"] = make(map[string]interface{})
		}
		sec := agentSettings["security"].(map[string]interface{})

		if _, ok := sec["auth"]; !ok {
			sec["auth"] = make(map[string]interface{})
		}
		authMap := sec["auth"].(map[string]interface{})

		currentType, _ := authMap["selectedType"].(string)

		if currentType != auth.SelectedType {
			authMap["selectedType"] = auth.SelectedType
			sData, _ = json.MarshalIndent(agentSettings, "", "  ")
			if err := os.MkdirAll(filepath.Dir(geminiSettingsPath), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(geminiSettingsPath, sData, 0644); err != nil {
				return fmt.Errorf("failed to update gemini settings: %w", err)
			}
		}
	}

	if auth.OAuthCreds != "" {
		dst := filepath.Join(homeDir, g.DefaultConfigDir(), "oauth_creds.json")
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		if err := util.CopyFile(auth.OAuthCreds, dst); err != nil {
			return fmt.Errorf("failed to copy oauth creds: %w", err)
		}
	}


	if auth.GoogleAppCredentials != "" {
		dst := filepath.Join(homeDir, ".config", "gcp", "application_default_credentials.json")
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		if err := util.CopyFile(auth.GoogleAppCredentials, dst); err != nil {
			return fmt.Errorf("failed to copy application default credentials: %w", err)
		}
	}

	return nil
}

func (g *GeminiCLI) GetVolumes(unixUsername string, auth api.AuthConfig) []api.VolumeMount {
	var volumes []api.VolumeMount
	if auth.OAuthCreds != "" {
		volumes = append(volumes, api.VolumeMount{
			Source:   auth.OAuthCreds,
			Target:   fmt.Sprintf("%s/%s/oauth_creds.json", util.GetHomeDir(unixUsername), g.DefaultConfigDir()),
			ReadOnly: true,
		})
	}
	if auth.GoogleAppCredentials != "" {
		volumes = append(volumes, api.VolumeMount{
			Source:   auth.GoogleAppCredentials,
			Target:   fmt.Sprintf("%s/.config/gcp/application_default_credentials.json", util.GetHomeDir(unixUsername)),
			ReadOnly: true,
		})
	}
	return volumes
}

func (g *GeminiCLI) DefaultConfigDir() string {
	return ".gemini"
}

func (g *GeminiCLI) HasSystemPrompt(agentHome string) bool {
	if agentHome == "" {
		return false
	}
	promptPath := filepath.Join(agentHome, g.DefaultConfigDir(), "system_prompt.md")
	data, err := os.ReadFile(promptPath)
	if err != nil {
		return false
	}
	content := strings.TrimSpace(string(data))
	if content == "" || content == "# Placeholder" {
		return false
	}
	return true
}

func (g *GeminiCLI) Provision(ctx context.Context, agentName, agentHome, agentWorkspace string) error {
	agentDir := filepath.Dir(agentHome)
	scionAgentPath := filepath.Join(agentDir, "scion-agent.json")

	data, err := os.ReadFile(scionAgentPath)
	if err != nil {
		return fmt.Errorf("failed to read scion-agent.json: %w", err)
	}
	var cfg api.ScionConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse scion-agent.json: %w", err)
	}

	selectedType := ""
	if cfg.Gemini != nil {
		selectedType = cfg.Gemini.AuthSelectedType
	}

	if selectedType == "" {
		return nil
	}

	// Update ~/.gemini/settings.json
	geminiSettingsPath := filepath.Join(agentHome, g.DefaultConfigDir(), "settings.json")
	var agentSettings map[string]interface{}
	sData, err := os.ReadFile(geminiSettingsPath)
	if err == nil {
		_ = json.Unmarshal(sData, &agentSettings)
	}
	if agentSettings == nil {
		agentSettings = make(map[string]interface{})
	}

	if _, ok := agentSettings["security"]; !ok {
		agentSettings["security"] = make(map[string]interface{})
	}
	sec := agentSettings["security"].(map[string]interface{})

	if _, ok := sec["auth"]; !ok {
		sec["auth"] = make(map[string]interface{})
	}
	auth := sec["auth"].(map[string]interface{})

	auth["selectedType"] = selectedType

	sData, _ = json.MarshalIndent(agentSettings, "", "  ")
	if err := os.MkdirAll(filepath.Dir(geminiSettingsPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(geminiSettingsPath, sData, 0644); err != nil {
		return fmt.Errorf("failed to update gemini settings: %w", err)
	}

	// Update scion-agent.json
	var envUpdates map[string]string
	var volUpdates []api.VolumeMount

	home, _ := os.UserHomeDir()

	switch selectedType {
	case "gemini-api-key":
		envUpdates = map[string]string{"GEMINI_API_KEY": ""}
	case "oauth-personal":
		envUpdates = map[string]string{"GOOGLE_CLOUD_PROJECT": ""}
	case "vertex-ai":
		envUpdates = map[string]string{
			"GOOGLE_CLOUD_PROJECT":  "",
			"GOOGLE_CLOUD_LOCATION": "",
		}
		volUpdates = append(volUpdates, api.VolumeMount{
			Source:   filepath.Join(home, ".config", "gcloud"),
			Target:   "/home/node/.config/gcloud",
			ReadOnly: true,
		})
	}

	if len(envUpdates) > 0 {
		if cfg.Env == nil {
			cfg.Env = make(map[string]string)
		}
		for k, v := range envUpdates {
			if _, exists := cfg.Env[k]; !exists {
				cfg.Env[k] = v
			}
		}
	}

	if len(volUpdates) > 0 {
		cfg.Volumes = append(cfg.Volumes, volUpdates...)
	}

	newData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal updated config: %w", err)
	}
	if err := os.WriteFile(scionAgentPath, newData, 0644); err != nil {
		return fmt.Errorf("failed to write updated scion-agent.json: %w", err)
	}

	return nil
}

func (g *GeminiCLI) GetEmbedDir() string {
	return "gemini"
}
