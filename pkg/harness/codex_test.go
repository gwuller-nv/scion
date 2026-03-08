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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ptone/scion-agent/pkg/api"
)

func TestCodexGetEnv(t *testing.T) {
	c := &Codex{}

	// GetEnv should return empty map (auth handled by ResolvedAuth)
	env := c.GetEnv("test-agent", "/tmp", "user")
	if len(env) != 0 {
		t.Errorf("expected empty env (auth handled by ResolvedAuth), got %v", env)
	}
}

func TestCodexGetCommand(t *testing.T) {
	c := &Codex{}

	// Test standard command
	cmd := c.GetCommand("do something", false, []string{})
	if len(cmd) < 3 || cmd[0] != "codex" || cmd[1] != "--full-auto" || cmd[2] != "do something" {
		t.Errorf("unexpected command structure: %v", cmd)
	}

	// Test resume
	cmd = c.GetCommand("", true, []string{})
	if len(cmd) < 3 || cmd[1] != "--full-auto" || cmd[2] != "resume" {
		t.Errorf("unexpected resume command: %v", cmd)
	}
}

func TestCodexInjectAgentInstructions(t *testing.T) {
	agentHome := t.TempDir()
	c := &Codex{}
	content := []byte("# Agent Instructions\nDo good work.")

	if err := c.InjectAgentInstructions(agentHome, content); err != nil {
		t.Fatalf("InjectAgentInstructions failed: %v", err)
	}

	target := filepath.Join(agentHome, ".codex", "AGENTS.md")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected file at %s: %v", target, err)
	}
	if string(data) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(data), string(content))
	}
}

func TestCodexResolveAuth_CodexAPIKey(t *testing.T) {
	c := &Codex{}
	auth := api.AuthConfig{CodexAPIKey: "codex-key"}
	result, err := c.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "api-key" {
		t.Errorf("Method = %q, want %q", result.Method, "api-key")
	}
	if result.EnvVars["CODEX_API_KEY"] != "codex-key" {
		t.Errorf("CODEX_API_KEY = %q, want %q", result.EnvVars["CODEX_API_KEY"], "codex-key")
	}
}

func TestCodexResolveAuth_OpenAIAPIKey(t *testing.T) {
	c := &Codex{}
	auth := api.AuthConfig{OpenAIAPIKey: "openai-key"}
	result, err := c.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "api-key" {
		t.Errorf("Method = %q, want %q", result.Method, "api-key")
	}
	if result.EnvVars["OPENAI_API_KEY"] != "openai-key" {
		t.Errorf("OPENAI_API_KEY = %q, want %q", result.EnvVars["OPENAI_API_KEY"], "openai-key")
	}
}

func TestCodexResolveAuth_AuthFile(t *testing.T) {
	c := &Codex{}
	auth := api.AuthConfig{CodexAuthFile: "/home/user/.codex/auth.json"}
	result, err := c.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "auth-file" {
		t.Errorf("Method = %q, want %q", result.Method, "auth-file")
	}
	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file mapping, got %d", len(result.Files))
	}
	if result.Files[0].SourcePath != "/home/user/.codex/auth.json" {
		t.Errorf("SourcePath = %q, want %q", result.Files[0].SourcePath, "/home/user/.codex/auth.json")
	}
}

func TestCodexResolveAuth_PreferenceOrder(t *testing.T) {
	c := &Codex{}
	// CodexAPIKey should win over OpenAIAPIKey and auth file
	auth := api.AuthConfig{
		CodexAPIKey:   "codex",
		OpenAIAPIKey:  "openai",
		CodexAuthFile: "/auth.json",
	}
	result, err := c.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "api-key" {
		t.Errorf("CodexAPIKey should win; Method = %q, want %q", result.Method, "api-key")
	}

	// OpenAIAPIKey should win over auth file
	auth = api.AuthConfig{
		OpenAIAPIKey:  "openai",
		CodexAuthFile: "/auth.json",
	}
	result, err = c.ResolveAuth(auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "api-key" {
		t.Errorf("OpenAIAPIKey should win over auth file; Method = %q, want %q", result.Method, "api-key")
	}
}

func TestCodexResolveAuth_NoCreds(t *testing.T) {
	c := &Codex{}
	_, err := c.ResolveAuth(api.AuthConfig{})
	if err == nil {
		t.Fatal("expected error for empty AuthConfig")
	}
	if !strings.Contains(err.Error(), "CODEX_API_KEY") {
		t.Errorf("error should mention CODEX_API_KEY: %v", err)
	}
}

func TestCodexInjectSystemPrompt_NoOp(t *testing.T) {
	agentHome := t.TempDir()
	c := &Codex{}

	// First inject agent instructions
	agentContent := []byte("# Existing Instructions\nDo things.")
	if err := c.InjectAgentInstructions(agentHome, agentContent); err != nil {
		t.Fatalf("InjectAgentInstructions failed: %v", err)
	}

	// System prompt injection should be a no-op (not yet supported)
	sysContent := []byte("You are a helpful assistant.")
	if err := c.InjectSystemPrompt(agentHome, sysContent); err != nil {
		t.Fatalf("InjectSystemPrompt failed: %v", err)
	}

	// AGENTS.md should remain unchanged — no system prompt prepended
	target := filepath.Join(agentHome, ".codex", "AGENTS.md")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected file at %s: %v", target, err)
	}

	if string(data) != string(agentContent) {
		t.Errorf("AGENTS.md was modified by InjectSystemPrompt; got %q, want %q", string(data), string(agentContent))
	}
}

func TestCodexApplyTelemetrySettings_EnabledMergesOtelAndPreservesKeys(t *testing.T) {
	agentHome := t.TempDir()
	c := &Codex{}

	codexDir := filepath.Join(agentHome, ".codex")
	requireNoErr(t, os.MkdirAll(codexDir, 0755))
	initial := `approval_policy = "never"
custom_key = "keep-me"

[projects."/workspace"]
trust_level = "trusted"
`
	requireNoErr(t, os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(initial), 0644))

	enabled := true
	telemetry := &api.TelemetryConfig{
		Enabled: &enabled,
		Cloud: &api.TelemetryCloudConfig{
			Endpoint: "collector.example.com:4317",
			Protocol: "grpc",
			Headers: map[string]string{"x-api-key": "test123"},
		},
	}
	err := c.ApplyTelemetrySettings(agentHome, telemetry, nil)
	requireNoErr(t, err)

	data, err := os.ReadFile(filepath.Join(codexDir, "config.toml"))
	requireNoErr(t, err)
	out := string(data)
	containsAll(t, out,
		`custom_key = "keep-me"`,
		`[otel]`,
		`enabled = true`,
		`log_user_prompt = false`,
		`exporter = { otlp-grpc = {`,
		`endpoint = "collector.example.com:4317"`,
		`headers = { "x-api-key" = "test123" }`,
	)
	if strings.Contains(out, "notify") {
		t.Fatalf("should not inject notify script, got:\n%s", out)
	}
}

func TestCodexApplyTelemetrySettings_DisabledDoesNotInjectOtel(t *testing.T) {
	agentHome := t.TempDir()
	c := &Codex{}

	// Seed a config that already has an [otel] section to verify it gets removed.
	codexDir := filepath.Join(agentHome, ".codex")
	requireNoErr(t, os.MkdirAll(codexDir, 0755))
	initial := `approval_policy = "never"

[otel]
enabled = false
exporter = { otlp-grpc = {
  endpoint = "localhost:4317"
}}
`
	requireNoErr(t, os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(initial), 0644))

	enabled := false
	telemetry := &api.TelemetryConfig{Enabled: &enabled}

	err := c.ApplyTelemetrySettings(agentHome, telemetry, nil)
	requireNoErr(t, err)

	data, err := os.ReadFile(filepath.Join(codexDir, "config.toml"))
	requireNoErr(t, err)
	out := string(data)
	if strings.Contains(out, "[otel]") {
		t.Fatalf("did not expect [otel] section when telemetry disabled, got:\n%s", out)
	}
	if strings.Contains(out, "notify") {
		t.Fatalf("should not inject notify script, got:\n%s", out)
	}
}

func TestCodexProvision_ReconcilesTelemetryFromScionAgentConfig(t *testing.T) {
	agentDir := t.TempDir()
	agentHome := filepath.Join(agentDir, "home")
	requireNoErr(t, os.MkdirAll(agentHome, 0755))

	enabled := true
	cfg := api.ScionConfig{
		Telemetry: &api.TelemetryConfig{
			Enabled: &enabled,
			Cloud: &api.TelemetryCloudConfig{
				Endpoint: "otel.local:4317",
				Protocol: "grpc",
			},
		},
	}
	data, err := jsonMarshal(cfg)
	requireNoErr(t, err)
	requireNoErr(t, os.WriteFile(filepath.Join(agentDir, "scion-agent.json"), data, 0644))

	c := &Codex{}
	err = c.Provision(context.Background(), "agent", agentHome, "/workspace")
	requireNoErr(t, err)

	out, err := os.ReadFile(filepath.Join(agentHome, ".codex", "config.toml"))
	requireNoErr(t, err)
	containsAll(t, string(out), `[otel]`, `endpoint = "otel.local:4317"`, `enabled = true`, `log_user_prompt = false`)
}

func TestCodexApplyTelemetrySettings_LogUserPromptFromFilter(t *testing.T) {
	agentHome := t.TempDir()
	c := &Codex{}

	enabled := true
	telemetry := &api.TelemetryConfig{
		Enabled: &enabled,
		Cloud: &api.TelemetryCloudConfig{
			Endpoint: "collector.example.com:4317",
			Protocol: "grpc",
		},
		Filter: &api.TelemetryFilterConfig{
			Events: &api.TelemetryEventsConfig{
				Include: []string{"agent.user.prompt"},
			},
		},
	}
	err := c.ApplyTelemetrySettings(agentHome, telemetry, nil)
	requireNoErr(t, err)

	data, err := os.ReadFile(filepath.Join(agentHome, ".codex", "config.toml"))
	requireNoErr(t, err)
	out := string(data)
	containsAll(t, out, `log_user_prompt = true`)

	// Now test exclusion takes precedence over inclusion.
	telemetry.Filter.Events.Exclude = []string{"agent.user.prompt"}
	err = c.ApplyTelemetrySettings(agentHome, telemetry, nil)
	requireNoErr(t, err)

	data, err = os.ReadFile(filepath.Join(agentHome, ".codex", "config.toml"))
	requireNoErr(t, err)
	out = string(data)
	containsAll(t, out, `log_user_prompt = false`)
}

func requireNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func containsAll(t *testing.T, s string, substrings ...string) {
	t.Helper()
	for _, sub := range substrings {
		if !strings.Contains(s, sub) {
			t.Fatalf("expected output to contain %q, got:\n%s", sub, s)
		}
	}
}

func jsonMarshal(v interface{}) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}
