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
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ptone/scion-agent/pkg/api"
	claudeEmbeds "github.com/ptone/scion-agent/pkg/harness/claude"
	"github.com/ptone/scion-agent/pkg/util"
)

type ClaudeCode struct{}

func (c *ClaudeCode) Name() string {
	return "claude"
}

func (c *ClaudeCode) DiscoverAuth(agentHome string) api.AuthConfig {
	// Placeholder for Claude specific auth discovery
	return api.AuthConfig{
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
	}
}

func (c *ClaudeCode) GetEnv(agentName string, agentHome string, unixUsername string, auth api.AuthConfig) map[string]string {
	env := make(map[string]string)
	if auth.AnthropicAPIKey != "" {
		env["ANTHROPIC_API_KEY"] = auth.AnthropicAPIKey
	}
	return env
}

func (c *ClaudeCode) GetCommand(task string, resume bool, baseArgs []string) []string {
	args := []string{"claude", "--no-chrome", "--dangerously-skip-permissions"}
	if resume {
		args = append(args, "--continue")
	}
	args = append(args, baseArgs...)
	if task != "" {
		args = append(args, task)
	}
	return args
}

func (c *ClaudeCode) PropagateFiles(homeDir, unixUsername string, auth api.AuthConfig) error {
	return nil
}

func (c *ClaudeCode) GetVolumes(unixUsername string, auth api.AuthConfig) []api.VolumeMount {
	return nil
}

func (c *ClaudeCode) DefaultConfigDir() string {
	return ".claude"
}

func (c *ClaudeCode) HasSystemPrompt(agentHome string) bool {
	return false
}

func (c *ClaudeCode) Provision(ctx context.Context, agentName, agentHome, agentWorkspace string) error {
	claudeJSONPath := filepath.Join(agentHome, ".claude.json")
	if _, err := os.Stat(claudeJSONPath); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(claudeJSONPath)
	if err != nil {
		return err
	}

	var claudeCfg map[string]interface{}
	if err := json.Unmarshal(data, &claudeCfg); err != nil {
		return err
	}

	repoRoot, err := util.RepoRoot()
	containerWorkspace := "/workspace"
	if err == nil {
		relWorkspace, err := filepath.Rel(repoRoot, agentWorkspace)
		if err == nil && !strings.HasPrefix(relWorkspace, "..") {
			containerWorkspace = filepath.Join("/repo-root", relWorkspace)
		}
	}

	// Update projects map
	projects, ok := claudeCfg["projects"].(map[string]interface{})
	if !ok {
		projects = make(map[string]interface{})
		claudeCfg["projects"] = projects
	}

	var projectSettings interface{}
	for _, v := range projects {
		projectSettings = v
		break
	}

	if projectSettings == nil {
		projectSettings = map[string]interface{}{
			"allowedTools":                            []interface{}{},
			"mcpContextUris":                          []interface{}{},
			"mcpServers":                              map[string]interface{}{},
			"enabledMcpjsonServers":                  []interface{}{},
			"disabledMcpjsonServers":                 []interface{}{},
			"hasTrustDialogAccepted":                  false,
			"projectOnboardingSeenCount":              1,
			"hasClaudeMdExternalIncludesApproved":    false,
			"hasClaudeMdExternalIncludesWarningShown": false,
			"exampleFiles":                            []interface{}{},
		}
	}

	newProjects := make(map[string]interface{})
	newProjects[containerWorkspace] = projectSettings
	claudeCfg["projects"] = newProjects

	newData, err := json.MarshalIndent(claudeCfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(claudeJSONPath, newData, 0644)
}

func (c *ClaudeCode) GetEmbedDir() string {
	return "claude"
}

func (c *ClaudeCode) GetInterruptKey() string {
	return "Escape"
}

func (c *ClaudeCode) GetHarnessEmbedsFS() (embed.FS, string) {
	return claudeEmbeds.EmbedsFS, "embeds"
}

func (c *ClaudeCode) InjectAgentInstructions(agentHome string, content []byte) error {
	target := filepath.Join(agentHome, ".claude", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("failed to create directory for agent instructions: %w", err)
	}
	return os.WriteFile(target, content, 0644)
}

func (c *ClaudeCode) GetTelemetryEnv() map[string]string {
	return map[string]string{
		"CLAUDE_CODE_ENABLE_TELEMETRY": "1",
		"OTEL_METRICS_EXPORTER":        "otlp",
		"OTEL_LOGS_EXPORTER":           "otlp",
		"OTEL_EXPORTER_OTLP_PROTOCOL":  "grpc",
		"OTEL_EXPORTER_OTLP_ENDPOINT":  "http://localhost:4317",
		"OTEL_METRIC_EXPORT_INTERVAL":  "30000",
	}
}

func (c *ClaudeCode) InjectSystemPrompt(agentHome string, content []byte) error {
	// System prompt is not yet supported for the Claude harness.
	return nil
}
