package harness

import (
	"context"
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

	// 1. Check agent settings (from template) first to see if they specify a type
	selectedType := ""
	agentSettingsPath := filepath.Join(agentHome, g.DefaultConfigDir(), "settings.json")
	if agentSettings, err := config.LoadAgentSettings(agentSettingsPath); err == nil {
		selectedType = agentSettings.Security.Auth.SelectedType
		if auth.GeminiAPIKey == "" && auth.GoogleAPIKey == "" && agentSettings.ApiKey != "" {
			auth.GeminiAPIKey = agentSettings.ApiKey
		}
	}

	// 2. Load host settings if we don't have a type yet, or to find fallback API key
	hostSettings, _ := config.GetAgentSettings()

	if selectedType == "" && hostSettings != nil {
		selectedType = hostSettings.Security.Auth.SelectedType
	}

	// 3. Fallback to settings.json for Gemini API Key if none found in env or agent settings
	if auth.GeminiAPIKey == "" && auth.GoogleAPIKey == "" && hostSettings != nil && hostSettings.ApiKey != "" {
		auth.GeminiAPIKey = hostSettings.ApiKey
	}

	// 4. Handle OAuth if selected
	if selectedType == "oauth-personal" {
		oauthPath := filepath.Join(home, g.DefaultConfigDir(), "oauth_creds.json")
		if _, err := os.Stat(oauthPath); err == nil {
			auth.OAuthCreds = oauthPath
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

	if auth.GeminiAPIKey != "" {
		env["GEMINI_API_KEY"] = auth.GeminiAPIKey
		env["GEMINI_DEFAULT_AUTH_TYPE"] = "gemini-api-key"
	}
	if auth.GoogleAPIKey != "" {
		env["GOOGLE_API_KEY"] = auth.GoogleAPIKey
		env["GEMINI_DEFAULT_AUTH_TYPE"] = "gemini-api-key"
	}

	if auth.VertexAPIKey != "" {
		env["VERTEX_API_KEY"] = auth.VertexAPIKey
		env["GEMINI_DEFAULT_AUTH_TYPE"] = "vertex-ai"
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
	return nil
}

func (g *GeminiCLI) GetEmbedDir() string {
	return "gemini"
}
