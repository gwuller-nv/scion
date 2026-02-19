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
	"reflect"
	"testing"
)

func TestClaudeCode_GetCommand(t *testing.T) {
	c := &ClaudeCode{}

	// 1. Normal task
	cmd := c.GetCommand("do something", false, nil)
	expected := []string{"claude", "--no-chrome", "--dangerously-skip-permissions", "do something"}
	if !reflect.DeepEqual(cmd, expected) {
		t.Errorf("expected %v, got %v", expected, cmd)
	}

	// 2. Empty task
	cmd = c.GetCommand("", false, nil)
	expected = []string{"claude", "--no-chrome", "--dangerously-skip-permissions"}
	if !reflect.DeepEqual(cmd, expected) {
		t.Errorf("expected %v, got %v", expected, cmd)
	}

	// 3. Resume
	cmd = c.GetCommand("do something", true, nil)
	expected = []string{"claude", "--no-chrome", "--dangerously-skip-permissions", "--continue", "do something"}
	if !reflect.DeepEqual(cmd, expected) {
		t.Errorf("expected %v, got %v", expected, cmd)
	}

	// 4. Task with baseArgs
	cmd = c.GetCommand("do something", false, []string{"--foo", "bar"})
	expected = []string{"claude", "--no-chrome", "--dangerously-skip-permissions", "--foo", "bar", "do something"}
	if !reflect.DeepEqual(cmd, expected) {
		t.Errorf("expected %v, got %v", expected, cmd)
	}

	// 5. With Model (via baseArgs)
	cmd = c.GetCommand("do something", false, []string{"--model", "claude-3-opus"})
	expected = []string{"claude", "--no-chrome", "--dangerously-skip-permissions", "--model", "claude-3-opus", "do something"}
	if !reflect.DeepEqual(cmd, expected) {
		t.Errorf("expected %v, got %v", expected, cmd)
	}
}

func TestClaudeCode_Provision(t *testing.T) {
	tmpDir := t.TempDir()
	agentHome := filepath.Join(tmpDir, "home")
	agentWorkspace := filepath.Join(tmpDir, "workspace")
	os.MkdirAll(agentHome, 0755)
	os.MkdirAll(agentWorkspace, 0755)

	claudeJSONPath := filepath.Join(agentHome, ".claude.json")
	initialCfg := map[string]interface{}{
		"projects": map[string]interface{}{
			"/old/path": map[string]interface{}{
				"allowedTools": []interface{}{"test-tool"},
			},
		},
	}
	data, _ := json.Marshal(initialCfg)
	os.WriteFile(claudeJSONPath, data, 0644)

	c := &ClaudeCode{}
	// Note: Provision uses util.RepoRoot() which might return an error or different path 
	// depending on where tests run. In a real environment it would be more predictable.
	err := c.Provision(context.Background(), "test-agent", agentHome, agentWorkspace)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	// Verify .claude.json was updated
	updatedData, err := os.ReadFile(claudeJSONPath)
	if err != nil {
		t.Fatal(err)
	}

	var updatedCfg map[string]interface{}
	json.Unmarshal(updatedData, &updatedCfg)

	projects, ok := updatedCfg["projects"].(map[string]interface{})
	if !ok {
		t.Fatal("projects map not found in updated config")
	}

	// It should have one project entry, we don't strictly check the key because it depends on util.RepoRoot
	if len(projects) != 1 {
		t.Errorf("expected 1 project entry, got %d", len(projects))
	}
	
	for _, v := range projects {
		settings := v.(map[string]interface{})
		if settings["allowedTools"].([]interface{})[0] != "test-tool" {
			t.Errorf("expected preserved allowedTools, got %v", settings["allowedTools"])
		}
	}
}

func TestClaudeCode_GetTelemetryEnv(t *testing.T) {
	c := &ClaudeCode{}
	env := c.GetTelemetryEnv()

	expected := map[string]string{
		"CLAUDE_CODE_ENABLE_TELEMETRY": "1",
		"OTEL_METRICS_EXPORTER":        "otlp",
		"OTEL_LOGS_EXPORTER":           "otlp",
		"OTEL_EXPORTER_OTLP_PROTOCOL":  "grpc",
		"OTEL_EXPORTER_OTLP_ENDPOINT":  "http://localhost:4317",
		"OTEL_METRIC_EXPORT_INTERVAL":  "30000",
	}

	if len(env) != len(expected) {
		t.Fatalf("expected %d env vars, got %d: %v", len(expected), len(env), env)
	}

	for k, want := range expected {
		got, ok := env[k]
		if !ok {
			t.Errorf("missing env var %s", k)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestClaudeInjectAgentInstructions(t *testing.T) {
	agentHome := t.TempDir()
	c := &ClaudeCode{}
	content := []byte("# Agent Instructions\nDo good work.")

	if err := c.InjectAgentInstructions(agentHome, content); err != nil {
		t.Fatalf("InjectAgentInstructions failed: %v", err)
	}

	target := filepath.Join(agentHome, ".claude", "CLAUDE.md")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected file at %s: %v", target, err)
	}
	if string(data) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(data), string(content))
	}
}

func TestClaudeInjectSystemPrompt(t *testing.T) {
	agentHome := t.TempDir()
	c := &ClaudeCode{}
	content := []byte("You are a helpful coding assistant.")

	if err := c.InjectSystemPrompt(agentHome, content); err != nil {
		t.Fatalf("InjectSystemPrompt failed: %v", err)
	}

	// System prompt is unsupported for Claude; no file should be written.
	target := filepath.Join(agentHome, ".claude", "CLAUDE.md")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("expected no file at %s, but it exists", target)
	}
	target = filepath.Join(agentHome, ".claude", "claude.md")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("expected no file at %s, but it exists", target)
	}
}
