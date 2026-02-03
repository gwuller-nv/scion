package hubclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ptone/scion-agent/pkg/transfer"
)

func TestWorkspaceSyncFrom(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/agents/agent-123/workspace/sync-from" {
			t.Errorf("expected path /api/v1/agents/agent-123/workspace/sync-from, got %s", r.URL.Path)
		}

		// Check request body if provided
		if r.ContentLength > 0 {
			var req struct {
				ExcludePatterns []string `json:"excludePatterns"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("failed to decode request: %v", err)
			}
			if len(req.ExcludePatterns) != 1 || req.ExcludePatterns[0] != ".git/**" {
				t.Errorf("expected excludePatterns ['.git/**'], got %v", req.ExcludePatterns)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SyncFromResponse{
			Manifest: &transfer.Manifest{
				Version:     "1.0",
				ContentHash: "sha256:abc123",
				Files: []transfer.FileInfo{
					{Path: "src/main.go", Size: 1024, Hash: "sha256:def456"},
					{Path: "README.md", Size: 256, Hash: "sha256:789abc"},
				},
			},
			DownloadURLs: []transfer.DownloadURLInfo{
				{Path: "src/main.go", URL: "https://storage.example.com/main.go", Size: 1024, Hash: "sha256:def456"},
				{Path: "README.md", URL: "https://storage.example.com/readme.md", Size: 256, Hash: "sha256:789abc"},
			},
			Expires: time.Now().Add(15 * time.Minute),
		})
	}))
	defer server.Close()

	client, _ := New(server.URL)
	resp, err := client.Workspace().SyncFrom(context.Background(), "agent-123", &SyncFromOptions{
		ExcludePatterns: []string{".git/**"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Manifest == nil {
		t.Fatal("expected non-nil manifest")
	}
	if len(resp.Manifest.Files) != 2 {
		t.Errorf("expected 2 files in manifest, got %d", len(resp.Manifest.Files))
	}
	if len(resp.DownloadURLs) != 2 {
		t.Errorf("expected 2 download URLs, got %d", len(resp.DownloadURLs))
	}
	if resp.DownloadURLs[0].Path != "src/main.go" {
		t.Errorf("expected first file path 'src/main.go', got %q", resp.DownloadURLs[0].Path)
	}
}

func TestWorkspaceSyncTo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/agents/agent-456/workspace/sync-to" {
			t.Errorf("expected path /api/v1/agents/agent-456/workspace/sync-to, got %s", r.URL.Path)
		}

		var req struct {
			Files []transfer.FileInfo `json:"files"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if len(req.Files) != 3 {
			t.Errorf("expected 3 files, got %d", len(req.Files))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SyncToResponse{
			UploadURLs: []transfer.UploadURLInfo{
				{Path: "src/main.go", URL: "https://storage.example.com/upload/main.go", Method: "PUT"},
				{Path: "src/lib.go", URL: "https://storage.example.com/upload/lib.go", Method: "PUT"},
			},
			ExistingFiles: []string{"README.md"}, // Already exists, skip upload
			Expires:       time.Now().Add(15 * time.Minute),
		})
	}))
	defer server.Close()

	client, _ := New(server.URL)
	files := []transfer.FileInfo{
		{Path: "src/main.go", Size: 1024, Hash: "sha256:new123"},
		{Path: "src/lib.go", Size: 512, Hash: "sha256:new456"},
		{Path: "README.md", Size: 256, Hash: "sha256:existing"},
	}

	resp, err := client.Workspace().SyncTo(context.Background(), "agent-456", files)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.UploadURLs) != 2 {
		t.Errorf("expected 2 upload URLs, got %d", len(resp.UploadURLs))
	}
	if len(resp.ExistingFiles) != 1 {
		t.Errorf("expected 1 existing file, got %d", len(resp.ExistingFiles))
	}
	if resp.ExistingFiles[0] != "README.md" {
		t.Errorf("expected existing file 'README.md', got %q", resp.ExistingFiles[0])
	}
}

func TestWorkspaceSyncToFinalize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/agents/agent-789/workspace/sync-to/finalize" {
			t.Errorf("expected path /api/v1/agents/agent-789/workspace/sync-to/finalize, got %s", r.URL.Path)
		}

		var req struct {
			Manifest *transfer.Manifest `json:"manifest"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if req.Manifest == nil {
			t.Error("expected non-nil manifest in request")
		}
		if len(req.Manifest.Files) != 2 {
			t.Errorf("expected 2 files in manifest, got %d", len(req.Manifest.Files))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SyncToFinalizeResponse{
			Applied:          true,
			ContentHash:      "sha256:finalhash",
			FilesApplied:     2,
			BytesTransferred: 1536,
		})
	}))
	defer server.Close()

	client, _ := New(server.URL)
	manifest := &transfer.Manifest{
		Version: "1.0",
		Files: []transfer.FileInfo{
			{Path: "src/main.go", Size: 1024, Hash: "sha256:abc"},
			{Path: "src/lib.go", Size: 512, Hash: "sha256:def"},
		},
	}

	resp, err := client.Workspace().FinalizeSyncTo(context.Background(), "agent-789", manifest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Applied {
		t.Error("expected applied=true")
	}
	if resp.FilesApplied != 2 {
		t.Errorf("expected 2 files applied, got %d", resp.FilesApplied)
	}
	if resp.BytesTransferred != 1536 {
		t.Errorf("expected 1536 bytes transferred, got %d", resp.BytesTransferred)
	}
	if resp.ContentHash != "sha256:finalhash" {
		t.Errorf("expected content hash 'sha256:finalhash', got %q", resp.ContentHash)
	}
}

func TestWorkspaceGetStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/agents/agent-status/workspace" {
			t.Errorf("expected path /api/v1/agents/agent-status/workspace, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(WorkspaceStatusResponse{
			AgentID:    "agent-status",
			GroveID:    "grove-xyz",
			StorageURI: "gs://bucket/workspaces/grove-xyz/agent-status/",
			LastSync: &WorkspaceSyncInfo{
				Direction:   "from",
				Timestamp:   time.Now().Add(-1 * time.Hour),
				ContentHash: "sha256:lastsync",
				FileCount:   15,
				TotalSize:   102400,
			},
		})
	}))
	defer server.Close()

	client, _ := New(server.URL)
	resp, err := client.Workspace().GetStatus(context.Background(), "agent-status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AgentID != "agent-status" {
		t.Errorf("expected agent ID 'agent-status', got %q", resp.AgentID)
	}
	if resp.GroveID != "grove-xyz" {
		t.Errorf("expected grove ID 'grove-xyz', got %q", resp.GroveID)
	}
	if resp.StorageURI == "" {
		t.Error("expected non-empty storage URI")
	}
	if resp.LastSync == nil {
		t.Fatal("expected non-nil LastSync")
	}
	if resp.LastSync.Direction != "from" {
		t.Errorf("expected direction 'from', got %q", resp.LastSync.Direction)
	}
	if resp.LastSync.FileCount != 15 {
		t.Errorf("expected 15 files, got %d", resp.LastSync.FileCount)
	}
}

func TestWorkspaceSyncFromNilOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// With nil options, request body should be nil or empty
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SyncFromResponse{
			Manifest: &transfer.Manifest{
				Version: "1.0",
				Files:   []transfer.FileInfo{},
			},
			DownloadURLs: []transfer.DownloadURLInfo{},
			Expires:      time.Now().Add(15 * time.Minute),
		})
	}))
	defer server.Close()

	client, _ := New(server.URL)
	resp, err := client.Workspace().SyncFrom(context.Background(), "agent-empty", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Manifest == nil {
		t.Fatal("expected non-nil manifest")
	}
	if len(resp.Manifest.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(resp.Manifest.Files))
	}
}

func TestWorkspaceServiceAvailable(t *testing.T) {
	client, err := New("https://hub.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.Workspace() == nil {
		t.Error("expected non-nil workspace service")
	}
}
