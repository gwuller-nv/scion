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

package util

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupGitRepo(t *testing.T) string {
	dir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Config user for commits
	configCmds := [][]string{
		{"config", "user.email", "you@example.com"},
		{"config", "user.name", "Your Name"},
		{"commit", "--allow-empty", "-m", "root commit"},
	}

	for _, args := range configCmds {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to run git %v: %v", args, err)
		}
	}

	return dir
}

func TestGitUtils(t *testing.T) {
	// Need to be inside the repo for most tests
	repoDir := setupGitRepo(t)

	// Save current working dir to restore later
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalWd)

	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}

	t.Run("IsGitRepo", func(t *testing.T) {
		if !IsGitRepo() {
			t.Error("expected true, got false")
		}
	})

	t.Run("RepoRoot", func(t *testing.T) {
		root, err := RepoRoot()
		if err != nil {
			t.Errorf("RepoRoot failed: %v", err)
		}
		// RepoRoot usually returns path with symlinks resolved, matching t.TempDir behavior
		// On macOS t.TempDir might be in /var/folders/... which is a symlink to /private/var/folders/...
		// We resolve both to compare safely.
		evalRoot, _ := filepath.EvalSymlinks(root)
		evalRepoDir, _ := filepath.EvalSymlinks(repoDir)

		if evalRoot != evalRepoDir {
			t.Errorf("expected root %q, got %q", evalRepoDir, evalRoot)
		}
	})

	t.Run("IsIgnored", func(t *testing.T) {
		ignoreFile := filepath.Join(repoDir, ".gitignore")
		if err := os.WriteFile(ignoreFile, []byte("ignored.txt"), 0644); err != nil {
			t.Fatal(err)
		}

		if !IsIgnored("ignored.txt") {
			t.Error("expected ignored.txt to be ignored")
		}

		if IsIgnored("not-ignored.txt") {
			t.Error("expected not-ignored.txt to NOT be ignored")
		}
	})

	t.Run("Worktrees", func(t *testing.T) {
		worktreePath := filepath.Join(repoDir, "wt-test")
		branchName := "test-branch"

		// Create
		if err := CreateWorktree(worktreePath, branchName); err != nil {
			t.Fatalf("CreateWorktree failed: %v", err)
		}

		if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
			t.Errorf("worktree dir does not exist")
		}

		// Remove
		if _, err := RemoveWorktree(worktreePath, false); err != nil {
			t.Fatalf("RemoveWorktree failed: %v", err)
		}
		// Wait/Check? git worktree remove deletes the directory usually.
		if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
			t.Errorf("worktree dir still exists after removal")
		}

		// Test PruneWorktrees
		prunePath := filepath.Join(repoDir, "prune-test")
		pruneBranch := "prune-branch"
		if err := CreateWorktree(prunePath, pruneBranch); err != nil {
			t.Fatalf("CreateWorktree for prune failed: %v", err)
		}
		// Manually remove directory to simulate stale worktree
		if err := os.RemoveAll(prunePath); err != nil {
			t.Fatalf("Failed to remove prune path: %v", err)
		}
		// Prune
		if err := PruneWorktrees(); err != nil {
			t.Fatalf("PruneWorktrees failed: %v", err)
		}
		// Verify we can create it again (if prune failed, this might fail with 'already exists')
		if err := CreateWorktree(prunePath, pruneBranch); err != nil {
			t.Errorf("Failed to recreate worktree after prune: %v", err)
		}
		// Clean up
		_, _ = RemoveWorktree(prunePath, true)
	})

	t.Run("PruneWorktreesIn", func(t *testing.T) {
		prunePath := filepath.Join(repoDir, "prune-in-test")
		pruneBranch := "prune-in-branch"
		if err := CreateWorktree(prunePath, pruneBranch); err != nil {
			t.Fatalf("CreateWorktree failed: %v", err)
		}
		// Manually remove directory to simulate stale worktree
		if err := os.RemoveAll(prunePath); err != nil {
			t.Fatalf("Failed to remove prune path: %v", err)
		}

		// PruneWorktreesIn should work even when CWD is outside the repo
		outsideDir := t.TempDir()
		prevWd, _ := os.Getwd()
		os.Chdir(outsideDir)
		defer os.Chdir(prevWd)

		if err := PruneWorktreesIn(repoDir); err != nil {
			t.Fatalf("PruneWorktreesIn failed: %v", err)
		}

		// Verify we can create the worktree again (prune cleared the stale record)
		os.Chdir(prevWd)
		if err := CreateWorktree(prunePath, pruneBranch); err != nil {
			t.Errorf("Failed to recreate worktree after PruneWorktreesIn: %v", err)
		}
		// Clean up
		_, _ = RemoveWorktree(prunePath, true)
	})

	t.Run("DeleteBranchIn", func(t *testing.T) {
		// Create a branch via worktree, then remove the worktree without deleting the branch
		wtPath := filepath.Join(repoDir, "branch-del-test")
		branch := "delete-me-branch"
		if err := CreateWorktree(wtPath, branch); err != nil {
			t.Fatalf("CreateWorktree failed: %v", err)
		}
		if _, err := RemoveWorktree(wtPath, false); err != nil {
			t.Fatalf("RemoveWorktree failed: %v", err)
		}

		// Branch should still exist
		if !BranchExists(branch) {
			t.Fatal("expected branch to still exist after RemoveWorktree(deleteBranch=false)")
		}

		// DeleteBranchIn should remove it
		if !DeleteBranchIn(repoDir, branch) {
			t.Error("DeleteBranchIn returned false, expected true")
		}

		// Branch should be gone
		if BranchExists(branch) {
			t.Error("expected branch to be deleted after DeleteBranchIn")
		}

		// Deleting a non-existent branch should return false
		if DeleteBranchIn(repoDir, "no-such-branch") {
			t.Error("DeleteBranchIn returned true for non-existent branch")
		}
	})

	t.Run("FindWorktreeByBranch", func(t *testing.T) {
		wtPath := filepath.Join(repoDir, "wt-find")
		branch := "find-branch"

		if err := CreateWorktree(wtPath, branch); err != nil {
			t.Fatalf("setup failed: %v", err)
		}

		foundPath, err := FindWorktreeByBranch(branch)
		if err != nil {
			t.Errorf("FindWorktreeByBranch failed: %v", err)
		}

		// Normalize paths for comparison (resolve symlinks)
		evalFound, _ := filepath.EvalSymlinks(foundPath)
		evalWt, _ := filepath.EvalSymlinks(wtPath)

		if evalFound != evalWt {
			t.Errorf("expected %q, got %q", evalWt, evalFound)
		}

		// Clean up
		_, _ = RemoveWorktree(wtPath, true)
	})

	t.Run("RemoveWorktreeWithBranch", func(t *testing.T) {
		wtPath := filepath.Join(repoDir, "wt-rm-branch")
		branch := "rm-branch-test"

		if err := CreateWorktree(wtPath, branch); err != nil {
			t.Fatalf("CreateWorktree failed: %v", err)
		}

		deleted, err := RemoveWorktree(wtPath, true)
		if err != nil {
			t.Fatalf("RemoveWorktree failed: %v", err)
		}
		if !deleted {
			t.Error("expected branch to be deleted")
		}
		if BranchExists(branch) {
			t.Error("branch still exists after RemoveWorktree with deleteBranch=true")
		}
	})

	t.Run("CompareGitVersion", func(t *testing.T) {
		tests := []struct {
			version string
			major   int
			minor   int
			wantErr bool
		}{
			{"2.47.0", 2, 47, false},
			{"2.48.0", 2, 47, false},
			{"3.0.0", 2, 47, false},
			{"2.46.9", 2, 47, true},
			{"1.9.0", 2, 47, true},
			{"2.47.1.windows.1", 2, 47, false},
			{"invalid", 2, 47, true},
		}

		for _, tt := range tests {
			err := CompareGitVersion(tt.version, tt.major, tt.minor)
			if (err != nil) != tt.wantErr {
				t.Errorf("CompareGitVersion(%q, %d, %d) error = %v, wantErr %v", tt.version, tt.major, tt.minor, err, tt.wantErr)
			}
		}
	})

	t.Run("NormalizeGitRemote", func(t *testing.T) {
		tests := []struct {
			remote string
			want   string
		}{
			{"https://github.com/GoogleCloudPlatform/scion.git", "github.com/GoogleCloudPlatform/scion"},
			{"http://github.com/GoogleCloudPlatform/scion.git", "github.com/GoogleCloudPlatform/scion"},
			{"git@github.com:GoogleCloudPlatform/scion.git", "github.com/GoogleCloudPlatform/scion"},
			{"github.com/GoogleCloudPlatform/scion.git", "github.com/GoogleCloudPlatform/scion"},
			{"git@github.com:GoogleCloudPlatform/scion", "github.com/GoogleCloudPlatform/scion"},
			{"HTTPS://github.com/GoogleCloudPlatform/scion.GIT", "github.com/GoogleCloudPlatform/scion"},
			{"", ""},
		}

		for _, tt := range tests {
			got := NormalizeGitRemote(tt.remote)
			if got != tt.want {
				t.Errorf("NormalizeGitRemote(%q) = %q, want %q", tt.remote, got, tt.want)
			}
		}
	})
}

func TestIsGitURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Valid URLs
		{"https://github.com/org/repo.git", true},
		{"https://github.com/org/repo", true},
		{"http://github.com/org/repo.git", true},
		{"git@github.com:org/repo.git", true},
		{"git@github.com:org/repo", true},
		{"ssh://git@github.com/org/repo", true},
		{"git://github.com/org/repo.git", true},
		{"HTTPS://GITHUB.COM/org/repo.git", true},
		{"git@gitlab.com:group/subgroup/repo.git", true},

		// Invalid inputs
		{"", false},
		{"/local/path/to/repo", false},
		{"./relative/path", false},
		{"../parent/path", false},
		{"github.com", false},          // bare hostname, no scheme recognized
		{"git@github.com:", false},     // no path after colon
		{"git@github.com:repo", false}, // no '/' in path
		{"https://github.com/", false}, // path is just '/'
		{"https://github.com", false},  // no path
	}

	for _, tt := range tests {
		got := IsGitURL(tt.input)
		if got != tt.want {
			t.Errorf("IsGitURL(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestToHTTPSCloneURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// SSH shorthand → HTTPS
		{"git@github.com:org/repo.git", "https://github.com/org/repo.git"},
		{"git@github.com:org/repo", "https://github.com/org/repo.git"},

		// ssh:// → HTTPS
		{"ssh://git@github.com/org/repo", "https://github.com/org/repo.git"},
		{"ssh://git@github.com/org/repo.git", "https://github.com/org/repo.git"},

		// HTTPS passthrough
		{"https://github.com/org/repo.git", "https://github.com/org/repo.git"},
		{"https://github.com/org/repo", "https://github.com/org/repo.git"},

		// git:// → HTTPS
		{"git://github.com/org/repo.git", "https://github.com/org/repo.git"},

		// http:// → HTTPS
		{"http://github.com/org/repo.git", "https://github.com/org/repo.git"},

		// Empty
		{"", ""},
	}

	for _, tt := range tests {
		got := ToHTTPSCloneURL(tt.input)
		if got != tt.want {
			t.Errorf("ToHTTPSCloneURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractOrgRepo(t *testing.T) {
	tests := []struct {
		input    string
		wantOrg  string
		wantRepo string
	}{
		{"https://github.com/acme/widgets.git", "acme", "widgets"},
		{"git@github.com:acme/widgets.git", "acme", "widgets"},
		{"ssh://git@github.com/acme/widgets", "acme", "widgets"},
		{"https://github.com/Acme/Widgets.git", "acme", "widgets"},
		{"git://github.com/org/repo.git", "org", "repo"},
		{"", "", ""},
	}

	for _, tt := range tests {
		org, repo := ExtractOrgRepo(tt.input)
		if org != tt.wantOrg || repo != tt.wantRepo {
			t.Errorf("ExtractOrgRepo(%q) = (%q, %q), want (%q, %q)", tt.input, org, repo, tt.wantOrg, tt.wantRepo)
		}
	}
}

func TestHashGroveID(t *testing.T) {
	// Determinism: same input → same output
	id1 := HashGroveID("github.com/acme/widgets")
	id2 := HashGroveID("github.com/acme/widgets")
	if id1 != id2 {
		t.Errorf("HashGroveID not deterministic: %q != %q", id1, id2)
	}

	// Length: always 16 hex characters
	if len(id1) != 16 {
		t.Errorf("HashGroveID length = %d, want 16", len(id1))
	}

	// Different inputs → different outputs
	id3 := HashGroveID("github.com/acme/gadgets")
	if id1 == id3 {
		t.Errorf("HashGroveID collision: %q == %q for different inputs", id1, id3)
	}

	// Branch qualifier produces different ID
	id4 := HashGroveID("github.com/acme/widgets@release/v2")
	if id1 == id4 {
		t.Errorf("HashGroveID collision with branch qualifier: %q == %q", id1, id4)
	}
}
