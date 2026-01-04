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
	expected = []string{"claude", "--no-chrome", "--dangerously-skip-permissions", "--continue"}
	if !reflect.DeepEqual(cmd, expected) {
		t.Errorf("expected %v, got %v", expected, cmd)
	}

	// 4. Task with baseArgs
	cmd = c.GetCommand("do something", false, []string{"--foo", "bar"})
	expected = []string{"claude", "--no-chrome", "--dangerously-skip-permissions", "--foo", "bar", "do something"}
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
