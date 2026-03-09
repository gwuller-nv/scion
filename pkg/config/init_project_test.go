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
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureScionGitignore_AddsEntry(t *testing.T) {
	tmpDir := t.TempDir()

	// No .gitignore exists yet
	if err := EnsureScionGitignore(tmpDir); err != nil {
		t.Fatalf("EnsureScionGitignore failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if string(content) != ".scion/\n" {
		t.Errorf("expected '.scion/\\n', got %q", string(content))
	}
}

func TestEnsureScionGitignore_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()

	// Write .gitignore with .scion/ already present
	os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("node_modules/\n.scion/\n"), 0644)

	if err := EnsureScionGitignore(tmpDir); err != nil {
		t.Fatalf("EnsureScionGitignore failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if string(content) != "node_modules/\n.scion/\n" {
		t.Errorf("expected no change, got %q", string(content))
	}
}

func TestEnsureScionGitignore_AppendsToExisting(t *testing.T) {
	tmpDir := t.TempDir()

	// Write .gitignore without trailing newline
	os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("node_modules/"), 0644)

	if err := EnsureScionGitignore(tmpDir); err != nil {
		t.Fatalf("EnsureScionGitignore failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if string(content) != "node_modules/\n.scion/\n" {
		t.Errorf("expected 'node_modules/\\n.scion/\\n', got %q", string(content))
	}
}

func TestEnsureScionGitignore_RecognizesVariants(t *testing.T) {
	for _, pattern := range []string{".scion", ".scion/", "/.scion", "/.scion/"} {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(pattern+"\n"), 0644)

		if err := EnsureScionGitignore(tmpDir); err != nil {
			t.Fatalf("EnsureScionGitignore failed for pattern %q: %v", pattern, err)
		}

		content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
		if err != nil {
			t.Fatalf("failed to read .gitignore: %v", err)
		}
		// Should not have added another entry
		if string(content) != pattern+"\n" {
			t.Errorf("for pattern %q: expected no change, got %q", pattern, string(content))
		}
	}
}

func TestInitProject_CreatesEmptyTemplatesDir(t *testing.T) {
	// Create a temporary directory for the project
	tempDir, err := os.MkdirTemp("", "scion-init-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mockRuntimeDetection(t, "docker")

	// Run InitProject
	err = InitProject(tempDir, GetMockHarnesses())
	if err != nil {
		t.Fatalf("InitProject failed: %v", err)
	}

	// Verify that templates/ directory exists
	templatesDir := filepath.Join(tempDir, "templates")
	if info, err := os.Stat(templatesDir); err != nil || !info.IsDir() {
		t.Fatalf("Expected templates/ directory to exist at %s", templatesDir)
	}

	// Verify that templates/default does NOT exist (default template lives in global grove only)
	defaultDir := filepath.Join(tempDir, "templates", "default")
	if _, err := os.Stat(defaultDir); !os.IsNotExist(err) {
		t.Errorf("Expected templates/default to NOT exist at project level, but it does at %s", defaultDir)
	}

	// Verify per-harness templates were NOT created
	for _, name := range []string{"gemini", "claude", "opencode", "codex"} {
		perHarnessDir := filepath.Join(tempDir, "templates", name)
		if _, err := os.Stat(perHarnessDir); !os.IsNotExist(err) {
			t.Errorf("Expected per-harness template %s to NOT be created at project level", name)
		}
	}
}
