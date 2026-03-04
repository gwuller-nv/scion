// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ptone/scion-agent/pkg/api"
	"github.com/ptone/scion-agent/pkg/config"
	"github.com/ptone/scion-agent/pkg/util"
)

// GatherAuth populates an AuthConfig from the environment and filesystem.
// It is source-agnostic: it checks env vars and well-known file paths
// without knowing which harness will consume the result.
func GatherAuth() api.AuthConfig {
	return GatherAuthWithEnv(nil)
}

// GatherAuthWithEnv is like GatherAuth but checks the provided env overlay
// before falling back to os.Getenv for each key. This allows hub-resolved
// or CLI-gathered env vars (passed via opts.Env) to be visible during auth
// resolution, even when the broker process itself lacks those env vars.
func GatherAuthWithEnv(env map[string]string) api.AuthConfig {
	lookup := func(key string) string {
		if v, ok := env[key]; ok && v != "" {
			return v
		}
		return os.Getenv(key)
	}

	home, _ := os.UserHomeDir()

	auth := api.AuthConfig{
		// Env-var sourced fields
		GeminiAPIKey:    lookup("GEMINI_API_KEY"),
		GoogleAPIKey:    lookup("GOOGLE_API_KEY"),
		AnthropicAPIKey: lookup("ANTHROPIC_API_KEY"),
		OpenAIAPIKey:    lookup("OPENAI_API_KEY"),
		CodexAPIKey:     lookup("CODEX_API_KEY"),
		GoogleCloudProject: util.FirstNonEmpty(
			lookup("GOOGLE_CLOUD_PROJECT"),
			lookup("GCP_PROJECT"),
			lookup("ANTHROPIC_VERTEX_PROJECT_ID"),
		),
		GoogleCloudRegion: util.FirstNonEmpty(
			lookup("GOOGLE_CLOUD_REGION"),
			lookup("CLOUD_ML_REGION"),
			lookup("GOOGLE_CLOUD_LOCATION"),
		),
		GoogleAppCredentials: lookup("GOOGLE_APPLICATION_CREDENTIALS"),
	}

	// File-sourced fields: check well-known paths
	if auth.GoogleAppCredentials == "" && home != "" {
		adcPath := filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
		if _, err := os.Stat(adcPath); err == nil {
			auth.GoogleAppCredentials = adcPath
		}
	}

	if home != "" {
		oauthPath := filepath.Join(home, ".gemini", "oauth_creds.json")
		if _, err := os.Stat(oauthPath); err == nil {
			auth.OAuthCreds = oauthPath
		}

		codexPath := filepath.Join(home, ".codex", "auth.json")
		if _, err := os.Stat(codexPath); err == nil {
			auth.CodexAuthFile = codexPath
		}

		opencodePath := filepath.Join(home, ".local", "share", "opencode", "auth.json")
		if _, err := os.Stat(opencodePath); err == nil {
			auth.OpenCodeAuthFile = opencodePath
		}
	}

	return auth
}

// OverlaySettings applies settings-based overrides to an AuthConfig.
// Currently this handles Gemini's SelectedType from scion-agent.json,
// agent settings, and host settings. For non-Gemini harnesses this is a no-op.
func OverlaySettings(auth *api.AuthConfig, h api.Harness, agentHome string) {
	g, ok := h.(*GeminiCLI)
	if !ok {
		return
	}

	selectedType := ""

	// 1. Check scion-agent.json for gemini.authSelectedType
	scionAgentPath := filepath.Join(filepath.Dir(agentHome), "scion-agent.json")
	if data, err := os.ReadFile(scionAgentPath); err == nil {
		var cfg api.ScionConfig
		if err := json.Unmarshal(data, &cfg); err == nil {
			if cfg.Gemini != nil {
				selectedType = cfg.Gemini.AuthSelectedType
			}
		}
	}

	// 2. Check agent settings
	agentSettingsPath := filepath.Join(agentHome, g.DefaultConfigDir(), "settings.json")
	if agentSettings, err := config.LoadAgentSettings(agentSettingsPath); err == nil {
		if selectedType == "" {
			selectedType = agentSettings.Security.Auth.SelectedType
		}
		if auth.GeminiAPIKey == "" && auth.GoogleAPIKey == "" {
			auth.GeminiAPIKey = agentSettings.ApiKey
		}
	}

	// 3. Check host settings for fallbacks
	hostSettings, _ := config.GetAgentSettings()
	if hostSettings != nil {
		if selectedType == "" {
			selectedType = hostSettings.Security.Auth.SelectedType
		}
		if auth.GeminiAPIKey == "" && auth.GoogleAPIKey == "" {
			auth.GeminiAPIKey = hostSettings.ApiKey
		}
	}

	auth.SelectedType = selectedType
}

// ValidateAuth checks a ResolvedAuth for completeness before container launch.
// It acts as a post-resolution safety net: ResolveAuth should produce correct
// results, but ValidateAuth catches any bugs or race conditions (e.g., a
// credential file deleted between GatherAuth and container launch).
func ValidateAuth(resolved *api.ResolvedAuth) error {
	if resolved == nil {
		return fmt.Errorf("auth validation failed: resolved auth is nil")
	}

	if resolved.Method == "" {
		return fmt.Errorf("auth validation failed: no auth method selected")
	}

	// Check for empty env var values — an env var with an empty value
	// indicates a bug in ResolveAuth (it should not emit keys it cannot fill).
	var emptyVars []string
	for k, v := range resolved.EnvVars {
		if v == "" {
			emptyVars = append(emptyVars, k)
		}
	}
	if len(emptyVars) > 0 {
		return fmt.Errorf("auth validation failed: env vars have empty values: %s", strings.Join(emptyVars, ", "))
	}

	// Check file mappings: source must exist, container path must be set.
	for _, f := range resolved.Files {
		if f.ContainerPath == "" {
			return fmt.Errorf("auth validation failed: file mapping for %q has no container path", f.SourcePath)
		}
		if _, err := os.Stat(f.SourcePath); err != nil {
			return fmt.Errorf("auth validation failed: credential file %q does not exist: %w", f.SourcePath, err)
		}
	}

	return nil
}
