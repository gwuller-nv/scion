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

package hub

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/ptone/scion-agent/pkg/api"
	"github.com/ptone/scion-agent/pkg/config"
	"github.com/ptone/scion-agent/pkg/storage"
	"github.com/ptone/scion-agent/pkg/store"
	"github.com/ptone/scion-agent/pkg/transfer"
)

// BootstrapTemplatesFromDir imports local templates from a directory into
// the Hub's database and storage. It is a one-time operation: if any
// templates already exist in the database, it returns immediately.
func (s *Server) BootstrapTemplatesFromDir(ctx context.Context, templatesDir string) error {
	// Check if the directory exists
	info, err := os.Stat(templatesDir)
	if err != nil || !info.IsDir() {
		s.templateLog.Debug("template bootstrap: directory not found, skipping", "dir", templatesDir)
		return nil
	}

	// Check if the DB already has templates — if so, skip bootstrap
	result, err := s.store.ListTemplates(ctx, store.TemplateFilter{}, store.ListOptions{Limit: 1})
	if err != nil {
		return err
	}
	if result.TotalCount > 0 {
		s.templateLog.Debug("template bootstrap: templates already exist in database, skipping")
		return nil
	}

	// Check that storage is configured
	stor := s.GetStorage()
	if stor == nil {
		s.templateLog.Warn("template bootstrap: no storage backend configured, skipping")
		return nil
	}

	// Scan the directory for template subdirectories
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		return err
	}

	imported := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		templatePath := filepath.Join(templatesDir, name)

		if err := s.bootstrapSingleTemplate(ctx, name, templatePath); err != nil {
			s.templateLog.Warn("template bootstrap: failed to import template, skipping",
				"template", name, "error", err)
			continue
		}
		imported++
	}

	if imported > 0 {
		s.templateLog.Info("template bootstrap: imported local templates", "count", imported)
	}

	return nil
}

// bootstrapSingleTemplate imports one local template directory into the
// Hub's database and storage backend.
func (s *Server) bootstrapSingleTemplate(ctx context.Context, name, templatePath string) error {
	stor := s.GetStorage()

	// Collect files from the template directory
	files, err := transfer.CollectFiles(templatePath, nil)
	if err != nil {
		return err
	}

	// Detect harness type from the template config
	harness := detectHarnessFromConfig(templatePath, name)

	slug := api.Slugify(name)
	scope := store.TemplateScopeGlobal

	// Create a pending template record
	storagePath := storage.TemplateStoragePath(scope, "", slug)
	tmpl := &store.Template{
		ID:            api.NewUUID(),
		Name:          name,
		Slug:          slug,
		Harness:       harness,
		Scope:         scope,
		Status:        store.TemplateStatusPending,
		StoragePath:   storagePath,
		StorageBucket: stor.Bucket(),
		StorageURI:    storage.TemplateStorageURI(stor.Bucket(), scope, "", slug),
		Visibility:    store.VisibilityPrivate,
	}

	if err := s.store.CreateTemplate(ctx, tmpl); err != nil {
		return err
	}

	// Upload each file to storage
	var templateFiles []store.TemplateFile
	for _, fi := range files {
		objectPath := storagePath + "/" + fi.Path

		f, err := os.Open(fi.FullPath)
		if err != nil {
			s.templateLog.Warn("template bootstrap: failed to open file, skipping",
				"file", fi.Path, "error", err)
			continue
		}

		_, err = stor.Upload(ctx, objectPath, f, storage.UploadOptions{})
		f.Close()
		if err != nil {
			s.templateLog.Warn("template bootstrap: failed to upload file, skipping",
				"file", fi.Path, "error", err)
			continue
		}

		templateFiles = append(templateFiles, store.TemplateFile{
			Path: fi.Path,
			Size: fi.Size,
			Hash: fi.Hash,
			Mode: fi.Mode,
		})
	}

	// Compute content hash and activate the template
	contentHash := computeContentHash(templateFiles)
	tmpl.Files = templateFiles
	tmpl.ContentHash = contentHash
	tmpl.Status = store.TemplateStatusActive

	if err := s.store.UpdateTemplate(ctx, tmpl); err != nil {
		return err
	}

	s.templateLog.Info("template bootstrap: imported template",
		"name", name, "files", len(templateFiles), "harness", harness)
	return nil
}

// detectHarnessFromConfig reads a template's config and returns the harness type.
// It checks the ScionConfig fields (HarnessConfig, DefaultHarnessConfig, Harness)
// and falls back to name-based inference.
func detectHarnessFromConfig(templatePath, templateName string) string {
	t := &config.Template{Name: templateName, Path: templatePath}
	cfg, err := t.LoadConfig()
	if err == nil && cfg != nil {
		// Check config fields in priority order
		if cfg.HarnessConfig != "" {
			return inferHarnessFromName(cfg.HarnessConfig)
		}
		if cfg.DefaultHarnessConfig != "" {
			return inferHarnessFromName(cfg.DefaultHarnessConfig)
		}
		if cfg.Harness != "" {
			return cfg.Harness
		}
	}

	// Fall back to name-based inference
	return inferHarnessFromName(templateName)
}

// inferHarnessFromName guesses the harness type from a name string.
func inferHarnessFromName(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "claude"):
		return "claude"
	case strings.Contains(lower, "gemini"):
		return "gemini"
	case strings.Contains(lower, "opencode"):
		return "opencode"
	case strings.Contains(lower, "codex"):
		return "codex"
	default:
		return ""
	}
}
