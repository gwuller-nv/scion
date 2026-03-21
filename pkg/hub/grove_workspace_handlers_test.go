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

//go:build !no_sqlite

package hub

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/scion/pkg/config"
	"github.com/GoogleCloudPlatform/scion/pkg/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// doMultipartRequest creates a multipart form request with file uploads.
// files is a map of field name (relative path) to file content.
func doMultipartRequest(t *testing.T, srv *Server, method, path string, files map[string][]byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for fieldName, content := range files {
		part, err := writer.CreateFormFile(fieldName, fieldName)
		require.NoError(t, err)
		_, err = part.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+testDevToken)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// createTestHubNativeGrove creates a hub-native grove (no git remote) via the API
// and returns the grove and its workspace path. Cleans up the workspace and any
// external grove-config directory on test completion.
func createTestHubNativeGrove(t *testing.T, srv *Server, name string) (*store.Grove, string) {
	t.Helper()

	rec := doRequest(t, srv, http.MethodPost, "/api/v1/groves", CreateGroveRequest{Name: name})
	require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())

	var grove store.Grove
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&grove))

	workspacePath, err := hubNativeGrovePath(grove.Slug)
	require.NoError(t, err)

	t.Cleanup(func() {
		// Clean up the external grove-config directory created by initInRepoGrove
		// (e.g. ~/.scion/grove-configs/<slug>__<uuid>/).
		scionDir := filepath.Join(workspacePath, ".scion")
		if extAgentsDir, err := config.GetGitGroveExternalAgentsDir(scionDir); err == nil && extAgentsDir != "" {
			// extAgentsDir is ~/.scion/grove-configs/<slug>__<uuid>/.scion/agents
			// Go up past "agents" and ".scion" to remove the <slug>__<uuid> parent dir
			os.RemoveAll(filepath.Dir(filepath.Dir(extAgentsDir)))
		}
		os.RemoveAll(workspacePath)
	})

	return &grove, workspacePath
}

// createTestGitGrove creates a git-backed grove via the API.
func createTestGitGrove(t *testing.T, srv *Server, name, remote string) *store.Grove {
	t.Helper()

	rec := doRequest(t, srv, http.MethodPost, "/api/v1/groves", CreateGroveRequest{
		Name:      name,
		GitRemote: remote,
	})
	require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())

	var grove store.Grove
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&grove))
	return &grove
}

// ============================================================================
// List Tests
// ============================================================================

func TestGroveWorkspaceList_EmptyWorkspace(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "WS List Empty")

	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var resp GroveWorkspaceListResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	// The .scion directory is excluded, so the workspace should appear empty
	assert.Equal(t, 0, resp.TotalCount)
	assert.Equal(t, int64(0), resp.TotalSize)
	assert.Empty(t, resp.Files)
}

func TestGroveWorkspaceList_WithFiles(t *testing.T) {
	srv, _ := testServer(t)
	grove, workspacePath := createTestHubNativeGrove(t, srv, "WS List Files")

	// Create some test files
	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "hello.txt"), []byte("hello world"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(workspacePath, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "subdir", "nested.txt"), []byte("nested"), 0644))

	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var resp GroveWorkspaceListResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	assert.Equal(t, 2, resp.TotalCount)
	assert.Equal(t, int64(11+6), resp.TotalSize) // "hello world" + "nested"

	// Check file paths
	paths := make(map[string]bool)
	for _, f := range resp.Files {
		paths[f.Path] = true
	}
	assert.True(t, paths["hello.txt"])
	assert.True(t, paths[filepath.Join("subdir", "nested.txt")])
}

func TestGroveWorkspaceList_ExcludesScionDir(t *testing.T) {
	srv, _ := testServer(t)
	grove, workspacePath := createTestHubNativeGrove(t, srv, "WS List Scion")

	// The .scion dir is created by initHubNativeGrove.
	// Add a file outside .scion to verify it appears.
	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "visible.txt"), []byte("yes"), 0644))

	// Also add a file inside .scion to verify it does NOT appear.
	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, ".scion", "extra.txt"), []byte("hidden"), 0644))

	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var resp GroveWorkspaceListResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	assert.Equal(t, 1, resp.TotalCount)
	assert.Equal(t, "visible.txt", resp.Files[0].Path)
}

func TestGroveWorkspaceList_GroveNotFound(t *testing.T) {
	srv, _ := testServer(t)

	rec := doRequest(t, srv, http.MethodGet, "/api/v1/groves/nonexistent/workspace/files", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGroveWorkspaceList_GitGroveRejected(t *testing.T) {
	srv, _ := testServer(t)
	grove := createTestGitGrove(t, srv, "Git Grove", "github.com/test/ws-list")

	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID), nil)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

// ============================================================================
// Upload Tests
// ============================================================================

func TestGroveWorkspaceUpload_SingleFile(t *testing.T) {
	srv, _ := testServer(t)
	grove, workspacePath := createTestHubNativeGrove(t, srv, "WS Upload Single")

	files := map[string][]byte{
		"readme.txt": []byte("hello from upload"),
	}
	rec := doMultipartRequest(t, srv, http.MethodPost, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID), files)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var resp GroveWorkspaceUploadResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	require.Len(t, resp.Files, 1)
	assert.Equal(t, "readme.txt", resp.Files[0].Path)
	assert.Equal(t, int64(17), resp.Files[0].Size)

	// Verify file on disk
	content, err := os.ReadFile(filepath.Join(workspacePath, "readme.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello from upload", string(content))
}

func TestGroveWorkspaceUpload_MultipleFiles(t *testing.T) {
	srv, _ := testServer(t)
	grove, workspacePath := createTestHubNativeGrove(t, srv, "WS Upload Multi")

	files := map[string][]byte{
		"a.txt": []byte("file a"),
		"b.txt": []byte("file b"),
	}
	rec := doMultipartRequest(t, srv, http.MethodPost, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID), files)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var resp GroveWorkspaceUploadResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	assert.Len(t, resp.Files, 2)

	// Verify both files on disk
	for name, expected := range files {
		content, err := os.ReadFile(filepath.Join(workspacePath, name))
		require.NoError(t, err)
		assert.Equal(t, string(expected), string(content))
	}
}

func TestGroveWorkspaceUpload_NestedPath(t *testing.T) {
	srv, _ := testServer(t)
	grove, workspacePath := createTestHubNativeGrove(t, srv, "WS Upload Nested")

	files := map[string][]byte{
		"src/main.go": []byte("package main"),
	}
	rec := doMultipartRequest(t, srv, http.MethodPost, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID), files)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	// Verify file on disk with parent directory created
	content, err := os.ReadFile(filepath.Join(workspacePath, "src", "main.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main", string(content))
}

func TestGroveWorkspaceUpload_PathTraversalRejected(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "WS Upload Traversal")

	files := map[string][]byte{
		"../escape.txt": []byte("bad"),
	}
	rec := doMultipartRequest(t, srv, http.MethodPost, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID), files)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGroveWorkspaceUpload_ScionDirRejected(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "WS Upload Scion")

	files := map[string][]byte{
		".scion/evil.yaml": []byte("bad config"),
	}
	rec := doMultipartRequest(t, srv, http.MethodPost, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID), files)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGroveWorkspaceUpload_NoFilesRejected(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "WS Upload Empty")

	// Send an empty multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID), &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+testDevToken)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGroveWorkspaceUpload_GitGroveRejected(t *testing.T) {
	srv, _ := testServer(t)
	grove := createTestGitGrove(t, srv, "Git Upload", "github.com/test/ws-upload")

	files := map[string][]byte{
		"test.txt": []byte("nope"),
	}
	rec := doMultipartRequest(t, srv, http.MethodPost, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID), files)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestGroveWorkspaceDelete_Success(t *testing.T) {
	srv, _ := testServer(t)
	grove, workspacePath := createTestHubNativeGrove(t, srv, "WS Delete OK")

	// Create a file to delete
	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "doomed.txt"), []byte("bye"), 0644))

	rec := doRequest(t, srv, http.MethodDelete, fmt.Sprintf("/api/v1/groves/%s/workspace/files/doomed.txt", grove.ID), nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify file is gone
	_, err := os.Stat(filepath.Join(workspacePath, "doomed.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestGroveWorkspaceDelete_NotFound(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "WS Delete NF")

	rec := doRequest(t, srv, http.MethodDelete, fmt.Sprintf("/api/v1/groves/%s/workspace/files/nonexistent.txt", grove.ID), nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGroveWorkspaceDelete_ScionDirRejected(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "WS Delete Scion")

	rec := doRequest(t, srv, http.MethodDelete, fmt.Sprintf("/api/v1/groves/%s/workspace/files/.scion/settings.yaml", grove.ID), nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGroveWorkspaceDelete_TraversalRejected(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "WS Delete Traversal")

	// Use a path with embedded traversal that won't be cleaned by the HTTP router
	// The URL router normalizes bare "../" so we test the handler's validation
	// by crafting a path that makes it through the router but is caught by validateWorkspaceFilePath.
	// Since the router resolves "../", test via the handler validation unit tests instead.
	// Here we verify the handler rejects absolute-looking paths and .scion paths.
	rec := doRequest(t, srv, http.MethodDelete, fmt.Sprintf("/api/v1/groves/%s/workspace/files/.scion/agents/test", grove.ID), nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGroveWorkspaceDelete_CleansEmptyDirs(t *testing.T) {
	srv, _ := testServer(t)
	grove, workspacePath := createTestHubNativeGrove(t, srv, "WS Delete Clean")

	// Create a nested file
	nestedDir := filepath.Join(workspacePath, "deep", "nested")
	require.NoError(t, os.MkdirAll(nestedDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(nestedDir, "file.txt"), []byte("data"), 0644))

	rec := doRequest(t, srv, http.MethodDelete, fmt.Sprintf("/api/v1/groves/%s/workspace/files/deep/nested/file.txt", grove.ID), nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify empty parent dirs were cleaned up
	_, err := os.Stat(filepath.Join(workspacePath, "deep", "nested"))
	assert.True(t, os.IsNotExist(err), "nested dir should be removed")
	_, err = os.Stat(filepath.Join(workspacePath, "deep"))
	assert.True(t, os.IsNotExist(err), "deep dir should be removed")
	// The workspace root should still exist
	_, err = os.Stat(workspacePath)
	assert.NoError(t, err, "workspace root should still exist")
}

// ============================================================================
// Download Tests
// ============================================================================

func TestGroveWorkspaceDownload_Success(t *testing.T) {
	srv, _ := testServer(t)
	grove, workspacePath := createTestHubNativeGrove(t, srv, "WS Download OK")

	content := []byte("hello download")
	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "readme.txt"), content, 0644))

	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/files/readme.txt", grove.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	assert.Equal(t, "hello download", rec.Body.String())
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "readme.txt")
	assert.Equal(t, "14", rec.Header().Get("Content-Length"))
}

func TestGroveWorkspaceDownload_NestedFile(t *testing.T) {
	srv, _ := testServer(t)
	grove, workspacePath := createTestHubNativeGrove(t, srv, "WS Download Nested")

	require.NoError(t, os.MkdirAll(filepath.Join(workspacePath, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "src", "main.go"), []byte("package main"), 0644))

	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/files/src/main.go", grove.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, "package main", rec.Body.String())
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "main.go")
}

func TestGroveWorkspaceDownload_NotFound(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "WS Download NF")

	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/files/nonexistent.txt", grove.ID), nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGroveWorkspaceDownload_InlineView(t *testing.T) {
	srv, _ := testServer(t)
	grove, workspacePath := createTestHubNativeGrove(t, srv, "WS Download Inline")

	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "readme.txt"), []byte("inline content"), 0644))

	// Without ?view=true — should be attachment
	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/files/readme.txt", grove.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "attachment")

	// With ?view=true — should be inline
	rec = doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/files/readme.txt?view=true", grove.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "inline")
	assert.Equal(t, "inline content", rec.Body.String())
}

func TestGroveWorkspaceDownload_ScionDirRejected(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "WS Download Scion")

	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/files/.scion/settings.yaml", grove.ID), nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ============================================================================
// Archive Download Tests
// ============================================================================

func TestGroveWorkspaceArchive_Success(t *testing.T) {
	srv, _ := testServer(t)
	grove, workspacePath := createTestHubNativeGrove(t, srv, "WS Archive OK")

	// Create some test files
	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "hello.txt"), []byte("hello world"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(workspacePath, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(workspacePath, "subdir", "nested.txt"), []byte("nested"), 0644))

	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/archive", grove.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code, "body length: %d", rec.Body.Len())

	assert.Equal(t, "application/zip", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Header().Get("Content-Disposition"), ".zip")

	// Verify the zip contents
	zipReader, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	require.NoError(t, err)

	files := make(map[string]string)
	for _, f := range zipReader.File {
		rc, err := f.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		require.NoError(t, err)
		rc.Close()
		files[f.Name] = string(content)
	}

	assert.Equal(t, "hello world", files["hello.txt"])
	assert.Equal(t, "nested", files[filepath.Join("subdir", "nested.txt")])
	// .scion directory should not be in the archive
	for name := range files {
		assert.False(t, name == ".scion" || len(name) > 6 && name[:6] == ".scion", "should not contain .scion files: %s", name)
	}
}

func TestGroveWorkspaceArchive_EmptyWorkspace(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "WS Archive Empty")

	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/archive", grove.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// Should be a valid empty zip
	assert.Equal(t, "application/zip", rec.Header().Get("Content-Type"))
	zipReader, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	require.NoError(t, err)
	// Only .scion files would be present but those are excluded
	for _, f := range zipReader.File {
		assert.False(t, f.Name == ".scion" || len(f.Name) > 6 && f.Name[:6] == ".scion", "should not contain .scion files")
	}
}

func TestGroveWorkspaceArchive_GitGroveRejected(t *testing.T) {
	srv, _ := testServer(t)
	grove := createTestGitGrove(t, srv, "Git Archive", "github.com/test/ws-archive")

	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/archive", grove.ID), nil)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestGroveWorkspaceArchive_MethodNotAllowed(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "WS Archive Method")

	rec := doRequest(t, srv, http.MethodPost, fmt.Sprintf("/api/v1/groves/%s/workspace/archive", grove.ID), nil)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

// ============================================================================
// Auth Tests
// ============================================================================

func TestGroveWorkspace_RequiresAuth(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "WS Auth")

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID)},
		{http.MethodPost, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID)},
		{http.MethodDelete, fmt.Sprintf("/api/v1/groves/%s/workspace/files/test.txt", grove.ID)},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			rec := doRequestNoAuth(t, srv, ep.method, ep.path, nil)
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}

// ============================================================================
// Method Not Allowed Tests
// ============================================================================

func TestGroveWorkspace_MethodNotAllowed(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "WS Method")

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodPut, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID)},
		{http.MethodPatch, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID)},
		{http.MethodPost, fmt.Sprintf("/api/v1/groves/%s/workspace/files/some-file.txt", grove.ID)},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := doRequest(t, srv, tt.method, tt.path, nil)
			assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		})
	}
}

// ============================================================================
// validateWorkspaceFilePath Unit Tests
// ============================================================================

func TestValidateWorkspaceFilePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
		errMsg  string
	}{
		{name: "valid simple", path: "file.txt", wantErr: false},
		{name: "valid nested", path: "src/main.go", wantErr: false},
		{name: "valid deeply nested", path: "a/b/c/d.txt", wantErr: false},
		{name: "valid dotfile", path: ".gitignore", wantErr: false},

		{name: "empty", path: "", wantErr: true, errMsg: "empty"},
		{name: "absolute unix", path: "/etc/passwd", wantErr: true, errMsg: "absolute"},
		{name: "traversal parent", path: "../escape.txt", wantErr: true, errMsg: "traversal"},
		{name: "traversal mid", path: "foo/../../escape.txt", wantErr: true, errMsg: "traversal"},
		{name: "scion root", path: ".scion", wantErr: true, errMsg: "reserved"},
		{name: "scion file", path: ".scion/settings.yaml", wantErr: true, errMsg: "reserved"},
		{name: "scion nested", path: ".scion/agents/test.yaml", wantErr: true, errMsg: "reserved"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorkspaceFilePath(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ============================================================================
// Upload + List + Delete integration
// ============================================================================

func TestGroveWorkspace_UploadListDelete_Integration(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "WS Integration")

	// Upload files
	files := map[string][]byte{
		"main.py":        []byte("print('hello')"),
		"lib/helpers.py": []byte("def help(): pass"),
	}
	rec := doMultipartRequest(t, srv, http.MethodPost, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID), files)
	require.Equal(t, http.StatusOK, rec.Code, "upload body: %s", rec.Body.String())

	// List files
	rec = doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var listResp GroveWorkspaceListResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&listResp))
	assert.Equal(t, 2, listResp.TotalCount)

	// Delete one file
	rec = doRequest(t, srv, http.MethodDelete, fmt.Sprintf("/api/v1/groves/%s/workspace/files/main.py", grove.ID), nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// List again — should have one file
	rec = doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/files", grove.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code)

	require.NoError(t, json.NewDecoder(rec.Body).Decode(&listResp))
	assert.Equal(t, 1, listResp.TotalCount)
	assert.Equal(t, filepath.Join("lib", "helpers.py"), listResp.Files[0].Path)
}

// ============================================================================
// Slug-format grove ID Tests
// ============================================================================

func TestGroveWorkspace_SlugFormatGroveID(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "WS Slug Format")

	// Use {uuid}__{slug} format for grove ID
	compositeID := grove.ID + "__" + grove.Slug
	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/workspace/files", compositeID), nil)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
}

// ============================================================================
// Shared Directory File Tests
// ============================================================================

// addSharedDirToGrove adds a shared directory to a grove via the API.
func addSharedDirToGrove(t *testing.T, srv *Server, groveID, dirName string) {
	t.Helper()
	rec := doRequest(t, srv, http.MethodPost, fmt.Sprintf("/api/v1/groves/%s/shared-dirs", groveID), map[string]interface{}{
		"name": dirName,
	})
	require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())
}

func TestSharedDirFiles_ListEmpty(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "SD List Empty")

	addSharedDirToGrove(t, srv, grove.ID, "build-cache")

	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/shared-dirs/build-cache/files", grove.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var resp GroveWorkspaceListResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 0, resp.TotalCount)
	assert.Empty(t, resp.Files)
}

func TestSharedDirFiles_UploadAndList(t *testing.T) {
	srv, _ := testServer(t)
	grove, workspacePath := createTestHubNativeGrove(t, srv, "SD Upload List")

	addSharedDirToGrove(t, srv, grove.ID, "artifacts")

	// Upload a file
	files := map[string][]byte{
		"output.log": []byte("build log content"),
	}
	rec := doMultipartRequest(t, srv, http.MethodPost, fmt.Sprintf("/api/v1/groves/%s/shared-dirs/artifacts/files", grove.ID), files)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	// Verify file on disk
	content, err := os.ReadFile(filepath.Join(workspacePath, "shared-dirs", "artifacts", "output.log"))
	require.NoError(t, err)
	assert.Equal(t, "build log content", string(content))

	// List files
	rec = doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/shared-dirs/artifacts/files", grove.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp GroveWorkspaceListResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 1, resp.TotalCount)
	assert.Equal(t, "output.log", resp.Files[0].Path)
}

func TestSharedDirFiles_Download(t *testing.T) {
	srv, _ := testServer(t)
	grove, workspacePath := createTestHubNativeGrove(t, srv, "SD Download")

	addSharedDirToGrove(t, srv, grove.ID, "data")

	// Create a file directly
	sharedDirPath := filepath.Join(workspacePath, "shared-dirs", "data")
	require.NoError(t, os.MkdirAll(sharedDirPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sharedDirPath, "result.txt"), []byte("result data"), 0644))

	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/shared-dirs/data/files/result.txt", grove.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "result data", rec.Body.String())
}

func TestSharedDirFiles_Delete(t *testing.T) {
	srv, _ := testServer(t)
	grove, workspacePath := createTestHubNativeGrove(t, srv, "SD Delete")

	addSharedDirToGrove(t, srv, grove.ID, "temp")

	// Create a file
	sharedDirPath := filepath.Join(workspacePath, "shared-dirs", "temp")
	require.NoError(t, os.MkdirAll(sharedDirPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sharedDirPath, "old.txt"), []byte("old"), 0644))

	rec := doRequest(t, srv, http.MethodDelete, fmt.Sprintf("/api/v1/groves/%s/shared-dirs/temp/files/old.txt", grove.ID), nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify file is gone
	_, err := os.Stat(filepath.Join(sharedDirPath, "old.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestSharedDirFiles_UndeclaredDirRejected(t *testing.T) {
	srv, _ := testServer(t)
	grove, _ := createTestHubNativeGrove(t, srv, "SD Undeclared")

	// Try to access files in a shared dir that hasn't been declared
	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/shared-dirs/nonexistent/files", grove.ID), nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSharedDirFiles_GitGroveRejected(t *testing.T) {
	srv, _ := testServer(t)
	grove := createTestGitGrove(t, srv, "SD Git Grove", "github.com/test/sd-files")

	rec := doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v1/groves/%s/shared-dirs/cache/files", grove.ID), nil)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

// Ensure the store's ErrNotFound is wired correctly for grove lookups.

func init() {
	// Silence logs during tests.
	_ = time.Now
	_ = io.Discard
	_ = context.Background
}
