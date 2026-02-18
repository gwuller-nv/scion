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
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCopyFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcFile := filepath.Join(srcDir, "test.txt")
	content := []byte("hello world")
	// Use 0644 permissions
	if err := os.WriteFile(srcFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	dstFile := filepath.Join(dstDir, "test_copy.txt")

	if err := CopyFile(srcFile, dstFile); err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}

	// Verify content
	got, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(got), string(content))
	}

	// Verify permissions
	info, err := os.Stat(dstFile)
	if err != nil {
		t.Fatal(err)
	}
	// Check specifically for user read/write (0600 part) as umask might affect group/world
	if info.Mode()&0600 != 0600 {
		t.Errorf("permission mismatch: got %v, expected at least 0600", info.Mode())
	}
}

func TestCopyDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create structure:
	// src/
	//   file1.txt
	//   subdir/
	//     file2.txt

	if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("file1"), 0644); err != nil {
		t.Fatal(err)
	}

	subDir := filepath.Join(srcDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte("file2"), 0644); err != nil {
		t.Fatal(err)
	}

	targetDir := filepath.Join(dstDir, "target")

	if err := CopyDir(srcDir, targetDir); err != nil {
		t.Fatalf("CopyDir failed: %v", err)
	}

	// Verify file1
	got1, err := os.ReadFile(filepath.Join(targetDir, "file1.txt"))
	if err != nil {
		t.Errorf("file1 not found: %v", err)
	} else if string(got1) != "file1" {
		t.Errorf("file1 content mismatch")
	}

	// Verify file2
	got2, err := os.ReadFile(filepath.Join(targetDir, "subdir", "file2.txt"))
	if err != nil {
		t.Errorf("file2 not found: %v", err)
	} else if string(got2) != "file2" {
		t.Errorf("file2 content mismatch")
	}
}

func TestMakeWritableRecursive(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a read-only file
	readOnlyFile := filepath.Join(tmpDir, "readonly.txt")
	if err := os.WriteFile(readOnlyFile, []byte("readonly"), 0400); err != nil {
		t.Fatal(err)
	}

	// Create a subdirectory
	readOnlySubDir := filepath.Join(tmpDir, "readonlydir")
	if err := os.Mkdir(readOnlySubDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Create a file inside that directory
	fileInDir := filepath.Join(readOnlySubDir, "file.txt")
	if err := os.WriteFile(fileInDir, []byte("file"), 0400); err != nil {
		t.Fatal(err)
	}

	// NOW make the directory read-only
	if err := os.Chmod(readOnlySubDir, 0500); err != nil {
		t.Fatal(err)
	}

	// Ensure they are indeed read-only (u+w is NOT set)
	info, _ := os.Stat(readOnlyFile)
	if info.Mode().Perm()&0200 != 0 {
		t.Fatal("file should be read-only")
	}

	// Run the function
	if err := MakeWritableRecursive(tmpDir); err != nil {
		t.Fatalf("MakeWritableRecursive failed: %v", err)
	}

	// Verify they are now writable
	info, _ = os.Stat(readOnlyFile)
	if info.Mode().Perm()&0200 == 0 {
		t.Error("file should be writable now")
	}

	info, _ = os.Stat(readOnlySubDir)
	if info.Mode().Perm()&0200 == 0 {
		t.Error("subdir should be writable now")
	}

	info, _ = os.Stat(fileInDir)
	if info.Mode().Perm()&0200 == 0 {
		t.Error("file in subdir should be writable now")
	}

	// Verify we can now remove all
	if err := os.RemoveAll(tmpDir); err != nil {
		t.Errorf("os.RemoveAll failed even after MakeWritableRecursive: %v", err)
	}
}

func TestRemoveAllSafe_BasicTree(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "agent-dir")
	if err := os.MkdirAll(filepath.Join(target, "subdir", "deep"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{
		filepath.Join(target, "file.txt"),
		filepath.Join(target, "subdir", "nested.txt"),
		filepath.Join(target, "subdir", "deep", "leaf.txt"),
	} {
		if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := removeAllSafe(target); err != nil {
		t.Fatalf("removeAllSafe failed: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected target to be fully removed")
	}
}

func TestRemoveSymlinkSafe(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("removes dangling symlink", func(t *testing.T) {
		link := filepath.Join(tmpDir, "dangling-link")
		if err := os.Symlink("/home/scion/.claude/debug/test.txt", link); err != nil {
			t.Fatal(err)
		}
		removeSymlinkSafe(link)
		if _, err := os.Lstat(link); !os.IsNotExist(err) {
			t.Error("expected dangling symlink to be removed")
		}
	})

	t.Run("removes valid symlink", func(t *testing.T) {
		target := filepath.Join(tmpDir, "real-target")
		if err := os.WriteFile(target, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(tmpDir, "valid-link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		removeSymlinkSafe(link)
		if _, err := os.Lstat(link); !os.IsNotExist(err) {
			t.Error("expected valid symlink to be removed")
		}
		// Target should still exist.
		if _, err := os.Stat(target); err != nil {
			t.Error("expected symlink target to still exist")
		}
	})

	t.Run("leaves no temp files behind", func(t *testing.T) {
		subDir := filepath.Join(tmpDir, "clean-check")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(subDir, "link")
		if err := os.Symlink("/nonexistent/path", link); err != nil {
			t.Fatal(err)
		}
		removeSymlinkSafe(link)
		entries, err := os.ReadDir(subDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			names := make([]string, len(entries))
			for i, e := range entries {
				names[i] = e.Name()
			}
			t.Errorf("expected empty directory, found: %v", names)
		}
	})
}

func TestRemoveAllSafe_WithDanglingSymlinks(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "agent-dir")
	debugDir := filepath.Join(target, ".claude", "debug")
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Multiple dangling symlinks mimicking container-internal paths.
	for _, name := range []string{"latest", "session-1", "session-2"} {
		if err := os.Symlink("/home/scion/.claude/debug/"+name+".txt", filepath.Join(debugDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	// A regular file alongside the symlinks.
	if err := os.WriteFile(filepath.Join(debugDir, "real.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := removeAllSafe(target); err != nil {
		t.Fatalf("removeAllSafe failed: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected target to be fully removed")
	}
}

func TestRemoveAllSafe_ReadOnlyFilesAndDirs(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "agent-dir")
	readOnlyDir := filepath.Join(target, "readonly-dir")
	if err := os.MkdirAll(readOnlyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(readOnlyDir, "file.txt"), []byte("x"), 0400); err != nil {
		t.Fatal(err)
	}
	// Lock down the directory.
	if err := os.Chmod(readOnlyDir, 0500); err != nil {
		t.Fatal(err)
	}

	if err := removeAllSafe(target); err != nil {
		t.Fatalf("removeAllSafe failed: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected target to be fully removed even with read-only entries")
	}
}

func TestRemoveAllAsync_RemovesOriginalPath(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "agent-dir")
	if err := os.MkdirAll(filepath.Join(target, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "file.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "subdir", "nested.txt"), []byte("nested"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RemoveAllAsync(target); err != nil {
		t.Fatalf("RemoveAllAsync failed: %v", err)
	}

	// Original path should be gone immediately (renamed).
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected original path to be gone after RemoveAllAsync")
	}

	// Wait briefly for background rm to finish, then verify tombstone is gone too.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(tmpDir)
		found := false
		for _, e := range entries {
			if strings.Contains(e.Name(), ".deleting-") {
				found = true
				break
			}
		}
		if !found {
			return // tombstone cleaned up
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("tombstone directory was not cleaned up by background rm within timeout")
}

func TestRemoveAllAsync_WithDanglingSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "agent-dir")
	debugDir := filepath.Join(target, ".claude", "debug")
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create a dangling symlink similar to what Claude Code creates.
	if err := os.Symlink("/nonexistent/container/path/debug.txt", filepath.Join(debugDir, "latest")); err != nil {
		t.Fatal(err)
	}

	if err := RemoveAllAsync(target); err != nil {
		t.Fatalf("RemoveAllAsync failed: %v", err)
	}

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected original path to be gone after RemoveAllAsync")
	}
}

func TestCleanupPendingDeletions(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a couple of fake tombstone directories.
	tomb1 := filepath.Join(tmpDir, "old-agent.deleting-111")
	tomb2 := filepath.Join(tmpDir, "other.deleting-222")
	normal := filepath.Join(tmpDir, "real-agent")
	for _, d := range []string{tomb1, tomb2, normal} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "f.txt"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	CleanupPendingDeletions(tmpDir)

	// Wait for background cleanup.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, err1 := os.Stat(tomb1)
		_, err2 := os.Stat(tomb2)
		if os.IsNotExist(err1) && os.IsNotExist(err2) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if _, err := os.Stat(tomb1); !os.IsNotExist(err) {
		t.Error("tombstone 1 should be removed")
	}
	if _, err := os.Stat(tomb2); !os.IsNotExist(err) {
		t.Error("tombstone 2 should be removed")
	}
	// Normal directory should not be touched.
	if _, err := os.Stat(normal); err != nil {
		t.Errorf("normal directory should still exist: %v", err)
	}
}
