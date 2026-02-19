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
	"fmt"
	"os"
	"path/filepath"

	"github.com/ptone/scion-agent/pkg/api"
	codexEmbeds "github.com/ptone/scion-agent/pkg/harness/codex"
	"github.com/ptone/scion-agent/pkg/util"
)

type Codex struct{}

func (c *Codex) Name() string {
	return "codex"
}

func (c *Codex) DiscoverAuth(agentHome string) api.AuthConfig {
	auth := api.AuthConfig{}
	// Check for Codex auth file in standard location
	home, _ := os.UserHomeDir()
	authPath := filepath.Join(home, ".codex", "auth.json")
	if _, err := os.Stat(authPath); err == nil {
		auth.CodexAuthFile = authPath
	}
	return auth
}

func (c *Codex) GetEnv(agentName string, agentHome string, unixUsername string, auth api.AuthConfig) map[string]string {
	env := make(map[string]string)
	if os.Getenv("OPENAI_API_KEY") != "" {
		env["OPENAI_API_KEY"] = os.Getenv("OPENAI_API_KEY")
	}
	if os.Getenv("CODEX_API_KEY") != "" {
		env["CODEX_API_KEY"] = os.Getenv("CODEX_API_KEY")
	}
	return env
}

func (c *Codex) GetCommand(task string, resume bool, baseArgs []string) []string {
	args := []string{"codex", "--yolo"}
	if resume {
		args = append(args, "resume", "--last")
	} else {
		if task != "" {
			args = append(args, task)
		}
	}

	args = append(args, baseArgs...)
	return args
}

func (c *Codex) PropagateFiles(homeDir, unixUsername string, auth api.AuthConfig) error {
	if auth.CodexAuthFile != "" {
		dst := filepath.Join(homeDir, ".codex", "auth.json")
		// Check if it already exists in the template/agent home
		if _, err := os.Stat(dst); err == nil {
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		if err := util.CopyFile(auth.CodexAuthFile, dst); err != nil {
			return fmt.Errorf("failed to copy codex auth file: %w", err)
		}
	}
	return nil
}

func (c *Codex) GetVolumes(unixUsername string, auth api.AuthConfig) []api.VolumeMount {
	return nil
}

func (c *Codex) DefaultConfigDir() string {
	return ""
}

func (c *Codex) HasSystemPrompt(agentHome string) bool {
	return false
}

func (c *Codex) Provision(ctx context.Context, agentName, agentHome, agentWorkspace string) error {
	auth := c.DiscoverAuth(agentHome)
	return c.PropagateFiles(agentHome, "", auth)
}

func (c *Codex) GetEmbedDir() string {
	return "codex"
}

func (c *Codex) GetInterruptKey() string {
	return "C-c"
}

func (c *Codex) GetHarnessEmbedsFS() (embed.FS, string) {
	return codexEmbeds.EmbedsFS, "embeds"
}

func (c *Codex) GetTelemetryEnv() map[string]string {
	// Codex uses a TOML config file for telemetry, not env vars.
	// File-based injection is handled via PropagateFiles.
	return nil
}

func (c *Codex) InjectAgentInstructions(agentHome string, content []byte) error {
	target := filepath.Join(agentHome, "AGENTS.md")
	return os.WriteFile(target, content, 0644)
}

func (c *Codex) InjectSystemPrompt(agentHome string, content []byte) error {
	// Codex has no native system prompt support — downgrade by prepending to AGENTS.md
	agentsPath := filepath.Join(agentHome, "AGENTS.md")
	header := fmt.Sprintf("# System Prompt\n\n%s\n\n---\n\n", string(content))

	existing, err := os.ReadFile(agentsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing agent instructions: %w", err)
	}

	merged := []byte(header)
	if len(existing) > 0 {
		merged = append(merged, existing...)
	}
	return os.WriteFile(agentsPath, merged, 0644)
}
