package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGetGlobalDir(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	dir, err := GetGlobalDir()
	if err != nil {
		t.Fatalf("GetGlobalDir failed: %v", err)
	}
	expected := filepath.Join(tmpHome, GlobalDir)
	if dir != expected {
		t.Errorf("expected %q, got %q", expected, dir)
	}
}

func TestGetGroveName(t *testing.T) {
	tmpDir := t.TempDir()
	
	tests := []struct {
		path string
		want string
	}{
		{filepath.Join(tmpDir, "My Project", ".scion"), "my-project"},
		{filepath.Join(tmpDir, "simple", ".scion"), "simple"},
		{filepath.Join(tmpDir, "CamelCase", ".scion"), "camelcase"},
	}

	for _, tt := range tests {
		if err := os.MkdirAll(tt.path, 0755); err != nil {
			t.Fatal(err)
		}
		if got := GetGroveName(tt.path); got != tt.want {
			t.Errorf("GetGroveName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// Helper to init a git repo
func initGitRepo(t *testing.T, dir string) {
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
}

func TestGetRepoDir(t *testing.T) {
	// 1. Not a git repo
	nonRepo := t.TempDir()
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	
	os.Chdir(nonRepo)
	if _, ok := GetRepoDir(); ok {
		t.Error("GetRepoDir should return false when not in a git repo")
	}

	// 2. Git repo with .scion
	repo := t.TempDir()
	initGitRepo(t, repo)
	scionDir := filepath.Join(repo, ".scion")
	os.Mkdir(scionDir, 0755)

	os.Chdir(repo)
	got, ok := GetRepoDir()
	if !ok {
		t.Error("GetRepoDir should return true in git repo with .scion")
	}
	
	// Evaluate symlinks for comparison (macOS /var/folders issue)
	evalGot, _ := filepath.EvalSymlinks(got)
	evalScion, _ := filepath.EvalSymlinks(scionDir)
	
	if evalGot != evalScion {
		t.Errorf("expected %q, got %q", evalScion, evalGot)
	}
}

func TestGetResolvedProjectDir(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	globalDir := filepath.Join(tmpHome, GlobalDir)
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		explicit string
		want     string
	}{
		{"home", globalDir},
		{"global", globalDir},
		{tmpHome, tmpHome},
	}

	for _, tt := range tests {
		got, err := GetResolvedProjectDir(tt.explicit)
		if err != nil {
			t.Errorf("GetResolvedProjectDir(%q) error: %v", tt.explicit, err)
			continue
		}
		
		evalGot, _ := filepath.EvalSymlinks(got)
		evalWant, _ := filepath.EvalSymlinks(tt.want)

		if evalGot != evalWant {
			t.Errorf("GetResolvedProjectDir(%q) = %q, want %q", tt.explicit, evalGot, evalWant)
		}
	}
}

func TestGetResolvedProjectDir_WalkUp(t *testing.T) {
	// Create structure:
	// /tmp/grove/.scion
	// /tmp/grove/subdir/deep
	
	tmpGrove := t.TempDir()
	scionDir := filepath.Join(tmpGrove, ".scion")
	if err := os.Mkdir(scionDir, 0755); err != nil {
		t.Fatal(err)
	}
	
	subDir := filepath.Join(tmpGrove, "subdir", "deep")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	
	// Set HOME to a clean temp dir so we don't fall back to real global .scion
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	if err := os.Chdir(subDir); err != nil {
		t.Fatal(err)
	}
	
	// Expect to find the .scion dir in the parent
	got, err := GetResolvedProjectDir("")
	if err != nil {
		t.Fatalf("GetResolvedProjectDir failed: %v", err)
	}
	
	evalGot, _ := filepath.EvalSymlinks(got)
	evalScion, _ := filepath.EvalSymlinks(scionDir)
	
	if evalGot != evalScion {
		t.Errorf("Expected %q, got %q", evalScion, evalGot)
	}
}