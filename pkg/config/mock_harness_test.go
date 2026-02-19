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

package config

import (
	"context"
	"embed"
	"os"
	"path/filepath"

	"github.com/ptone/scion-agent/pkg/api"
)

type MockHarness struct {
	NameVal      string
	EmbedDirVal  string
	ConfigDirVal string
}

func (m *MockHarness) Name() string { return m.NameVal }
func (m *MockHarness) DiscoverAuth(agentHome string) api.AuthConfig { return api.AuthConfig{} }
func (m *MockHarness) GetEnv(agentName string, agentHome string, unixUsername string, auth api.AuthConfig) map[string]string {
	return nil
}
func (m *MockHarness) GetCommand(task string, resume bool, baseArgs []string) []string { return nil }
func (m *MockHarness) PropagateFiles(homeDir, unixUsername string, auth api.AuthConfig) error {
	return nil
}
func (m *MockHarness) GetVolumes(unixUsername string, auth api.AuthConfig) []api.VolumeMount {
	return nil
}
func (m *MockHarness) DefaultConfigDir() string { return m.ConfigDirVal }
func (m *MockHarness) HasSystemPrompt(agentHome string) bool { return false }
func (m *MockHarness) Provision(ctx context.Context, agentName, agentHome, agentWorkspace string) error {
	return nil
}
func (m *MockHarness) GetEmbedDir() string                    { return m.EmbedDirVal }
func (m *MockHarness) GetInterruptKey() string                { return "C-c" }
func (m *MockHarness) GetHarnessEmbedsFS() (embed.FS, string) { return embed.FS{}, "" }
func (m *MockHarness) InjectAgentInstructions(agentHome string, content []byte) error {
	target := filepath.Join(agentHome, "agents.md")
	return os.WriteFile(target, content, 0644)
}
func (m *MockHarness) InjectSystemPrompt(agentHome string, content []byte) error {
	target := filepath.Join(agentHome, ".scion", "system_prompt.md")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	return os.WriteFile(target, content, 0644)
}
func (m *MockHarness) GetTelemetryEnv() map[string]string { return nil }

func GetMockHarnesses() []api.Harness {
	return []api.Harness{
		&MockHarness{NameVal: "gemini", EmbedDirVal: "gemini", ConfigDirVal: ".gemini"},
		&MockHarness{NameVal: "claude", EmbedDirVal: "claude", ConfigDirVal: ".claude"},
		&MockHarness{NameVal: "opencode", EmbedDirVal: "opencode", ConfigDirVal: ".config/opencode"},
		&MockHarness{NameVal: "codex", EmbedDirVal: "codex", ConfigDirVal: ""},
	}
}
