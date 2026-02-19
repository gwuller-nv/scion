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

package agent

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ptone/scion-agent/pkg/api"
	"github.com/ptone/scion-agent/pkg/config"
	"github.com/ptone/scion-agent/pkg/runtime"
)

func TestExtractWorkspaceFromVolumes(t *testing.T) {
	tests := []struct {
		name     string
		volumes  []api.VolumeMount
		expected string
	}{
		{
			name:     "empty volumes",
			volumes:  nil,
			expected: "",
		},
		{
			name: "no workspace volume",
			volumes: []api.VolumeMount{
				{Source: "/host/data", Target: "/data"},
				{Source: "/host/config", Target: "/config"},
			},
			expected: "",
		},
		{
			name: "has workspace volume",
			volumes: []api.VolumeMount{
				{Source: "/host/data", Target: "/data"},
				{Source: "/path/to/shared/worktree", Target: "/workspace"},
				{Source: "/host/config", Target: "/config"},
			},
			expected: "/path/to/shared/worktree",
		},
		{
			name: "first workspace volume wins",
			volumes: []api.VolumeMount{
				{Source: "/first/workspace", Target: "/workspace"},
				{Source: "/second/workspace", Target: "/workspace"},
			},
			expected: "/first/workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractWorkspaceFromVolumes(tt.volumes)
			if result != tt.expected {
				t.Errorf("extractWorkspaceFromVolumes() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFilterWorkspaceVolume(t *testing.T) {
	tests := []struct {
		name           string
		volumes        []api.VolumeMount
		expectedLen    int
		expectedAbsent string
	}{
		{
			name:           "empty volumes",
			volumes:        nil,
			expectedLen:    0,
			expectedAbsent: "/workspace",
		},
		{
			name: "no workspace volume",
			volumes: []api.VolumeMount{
				{Source: "/host/data", Target: "/data"},
				{Source: "/host/config", Target: "/config"},
			},
			expectedLen:    2,
			expectedAbsent: "/workspace",
		},
		{
			name: "filters workspace volume",
			volumes: []api.VolumeMount{
				{Source: "/host/data", Target: "/data"},
				{Source: "/path/to/worktree", Target: "/workspace"},
				{Source: "/host/config", Target: "/config"},
			},
			expectedLen:    2,
			expectedAbsent: "/workspace",
		},
		{
			name: "filters multiple workspace volumes",
			volumes: []api.VolumeMount{
				{Source: "/first", Target: "/workspace"},
				{Source: "/second", Target: "/workspace"},
				{Source: "/host/data", Target: "/data"},
			},
			expectedLen:    1,
			expectedAbsent: "/workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterWorkspaceVolume(tt.volumes)
			if len(result) != tt.expectedLen {
				t.Errorf("filterWorkspaceVolume() returned %d volumes, want %d", len(result), tt.expectedLen)
			}
			for _, v := range result {
				if v.Target == tt.expectedAbsent {
					t.Errorf("filterWorkspaceVolume() should have removed volume with target %q", tt.expectedAbsent)
				}
			}
		})
	}
}

func TestBuildAgentEnv(t *testing.T) {
	// Setup host env for inheritance test
	os.Setenv("INHERITED_KEY", "inherited-value")
	defer os.Unsetenv("INHERITED_KEY")

	scionCfg := &api.ScionConfig{
		Env: map[string]string{
			"NORMAL_KEY":     "normal-value",
			"INHERITED_KEY":  "${INHERITED_KEY}",
			"EMPTY_CFG_KEY":  "",               // Should be omitted
			"OVERRIDDEN_KEY": "original-value", // Should be omitted because of override
		},
	}

	extraEnv := map[string]string{
		"EXTRA_KEY":       "extra-value",
		"OVERRIDDEN_KEY":  "", // Should cause omission
		"EMPTY_EXTRA_KEY": "", // Should be omitted
	}

	env, warnings := buildAgentEnv(scionCfg, extraEnv)

	expected := map[string]string{
		"NORMAL_KEY":    "normal-value",
		"INHERITED_KEY": "inherited-value",
		"EXTRA_KEY":     "extra-value",
	}

	envMap := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	if len(env) != len(expected) {
		t.Errorf("expected %d env vars, got %d: %v", len(expected), len(env), env)
	}

	if len(warnings) != 3 {
		t.Errorf("expected 3 warnings, got %d: %v", len(warnings), warnings)
	}

	for k, v := range expected {
		if envMap[k] != v {
			t.Errorf("expected env[%s] = %q, got %q", k, v, envMap[k])
		}
	}

	// Explicitly check for omitted keys
	omitted := []string{"EMPTY_CFG_KEY", "OVERRIDDEN_KEY", "EMPTY_EXTRA_KEY"}
	for _, k := range omitted {
		if _, ok := envMap[k]; ok {
			t.Errorf("expected key %s to be omitted, but it was present", k)
		}
	}
}

func TestBuildAgentEnv_HubEndpointOverride(t *testing.T) {
	t.Run("scion config hub endpoint overrides extraEnv", func(t *testing.T) {
		scionCfg := &api.ScionConfig{
			Hub: &api.AgentHubConfig{
				Endpoint: "https://tunnel.example.com",
			},
		}

		// Simulate what Start() does: set hub endpoint in opts.Env from broker,
		// then override with scion config hub endpoint.
		extraEnv := map[string]string{
			"SCION_HUB_ENDPOINT": "http://localhost:9810",
			"SCION_HUB_URL":     "http://localhost:9810",
		}

		// Apply the override logic from Start()
		if scionCfg.Hub != nil && scionCfg.Hub.Endpoint != "" {
			extraEnv["SCION_HUB_ENDPOINT"] = scionCfg.Hub.Endpoint
			extraEnv["SCION_HUB_URL"] = scionCfg.Hub.Endpoint
		}

		env, _ := buildAgentEnv(scionCfg, extraEnv)

		envMap := make(map[string]string)
		for _, e := range env {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				envMap[parts[0]] = parts[1]
			}
		}

		if got := envMap["SCION_HUB_ENDPOINT"]; got != "https://tunnel.example.com" {
			t.Errorf("expected SCION_HUB_ENDPOINT='https://tunnel.example.com', got %q", got)
		}
		if got := envMap["SCION_HUB_URL"]; got != "https://tunnel.example.com" {
			t.Errorf("expected SCION_HUB_URL='https://tunnel.example.com', got %q", got)
		}
	})

	t.Run("no hub config preserves extraEnv", func(t *testing.T) {
		scionCfg := &api.ScionConfig{}
		extraEnv := map[string]string{
			"SCION_HUB_ENDPOINT": "https://hub.example.com",
			"SCION_HUB_URL":     "https://hub.example.com",
		}

		env, _ := buildAgentEnv(scionCfg, extraEnv)

		envMap := make(map[string]string)
		for _, e := range env {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				envMap[parts[0]] = parts[1]
			}
		}

		if got := envMap["SCION_HUB_ENDPOINT"]; got != "https://hub.example.com" {
			t.Errorf("expected SCION_HUB_ENDPOINT='https://hub.example.com', got %q", got)
		}
	})
}

func TestScionCreatorEnvVar(t *testing.T) {
	t.Run("SCION_CREATOR is set from OS user when not present", func(t *testing.T) {
		env := make(map[string]string)
		// Simulate the logic from Start(): if SCION_CREATOR is not set, set it from os/user
		if _, ok := env["SCION_CREATOR"]; !ok {
			if u, err := user.Current(); err == nil {
				env["SCION_CREATOR"] = u.Username
			}
		}

		if env["SCION_CREATOR"] == "" {
			t.Error("expected SCION_CREATOR to be set from OS user")
		}

		u, _ := user.Current()
		if env["SCION_CREATOR"] != u.Username {
			t.Errorf("expected SCION_CREATOR = %q, got %q", u.Username, env["SCION_CREATOR"])
		}
	})

	t.Run("SCION_CREATOR is preserved when already set", func(t *testing.T) {
		env := map[string]string{
			"SCION_CREATOR": "hub-user@example.com",
		}
		// Simulate the logic from Start(): if SCION_CREATOR is not set, set it from os/user
		if _, ok := env["SCION_CREATOR"]; !ok {
			if u, err := user.Current(); err == nil {
				env["SCION_CREATOR"] = u.Username
			}
		}

		if env["SCION_CREATOR"] != "hub-user@example.com" {
			t.Errorf("expected SCION_CREATOR = %q, got %q", "hub-user@example.com", env["SCION_CREATOR"])
		}
	})
}

func TestStartResumeNonExistentAgent(t *testing.T) {
	// Create a temporary directory to act as the grove
	tmpDir := t.TempDir()

	// Move to tmpDir to avoid being inside the project's git repo
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Mock HOME for global settings
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	// Create .scion directory structure (minimum required)
	scionDir := filepath.Join(tmpDir, ".scion")
	if err := os.MkdirAll(scionDir, 0755); err != nil {
		t.Fatalf("failed to create .scion dir: %v", err)
	}

	// Create a mock runtime
	mockRuntime := &runtime.MockRuntime{
		ListFunc: func(ctx context.Context, labelFilter map[string]string) ([]api.AgentInfo, error) {
			return []api.AgentInfo{}, nil
		},
	}

	mgr := NewManager(mockRuntime)

	// Try to resume a non-existent agent
	opts := api.StartOptions{
		Name:      "non-existent-agent",
		GrovePath: scionDir,
		Resume:    true,
	}

	_, err := mgr.Start(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when resuming non-existent agent, got nil")
	}

	if !strings.Contains(err.Error(), "cannot resume agent") {
		t.Errorf("expected error message to contain 'cannot resume agent', got: %v", err)
	}

	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected error message to contain 'does not exist', got: %v", err)
	}
}

func TestStartDurationTimer(t *testing.T) {
	t.Run("stops container after duration", func(t *testing.T) {
		var mu sync.Mutex
		var stoppedID string

		mockRT := &runtime.MockRuntime{
			StopFunc: func(ctx context.Context, id string) error {
				mu.Lock()
				defer mu.Unlock()
				stoppedID = id
				return nil
			},
		}

		startDurationTimer(mockRT, "test-container", 50*time.Millisecond)

		// Wait for the timer to fire
		time.Sleep(150 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()
		if stoppedID != "test-container" {
			t.Errorf("expected Stop to be called with 'test-container', got %q", stoppedID)
		}
	})

	t.Run("no-op for zero duration", func(t *testing.T) {
		stopCalled := false
		mockRT := &runtime.MockRuntime{
			StopFunc: func(ctx context.Context, id string) error {
				stopCalled = true
				return nil
			},
		}

		startDurationTimer(mockRT, "test-container", 0)

		time.Sleep(50 * time.Millisecond)
		if stopCalled {
			t.Error("Stop should not be called for zero duration")
		}
	})

	t.Run("no-op for negative duration", func(t *testing.T) {
		stopCalled := false
		mockRT := &runtime.MockRuntime{
			StopFunc: func(ctx context.Context, id string) error {
				stopCalled = true
				return nil
			},
		}

		startDurationTimer(mockRT, "test-container", -1*time.Second)

		time.Sleep(50 * time.Millisecond)
		if stopCalled {
			t.Error("Stop should not be called for negative duration")
		}
	})
}

func TestStartResolvesHarnessConfigUser(t *testing.T) {
	// Regression test: the container user (e.g. "scion") defined in the on-disk
	// harness-config config.yaml must flow into RunConfig.UnixUsername.
	// Previously, an empty User from settings.ResolveHarnessConfig() overwrote
	// the default, producing empty mount paths like /home//.config/gcloud.
	tmpDir := t.TempDir()

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	globalScionDir := filepath.Join(tmpDir, ".scion")

	// Create harness-config with user field
	hcDir := filepath.Join(globalScionDir, "harness-configs", "test-harness")
	os.MkdirAll(hcDir, 0755)
	os.WriteFile(filepath.Join(hcDir, "config.yaml"), []byte("harness: gemini\nuser: scion\nimage: test-image:latest\n"), 0644)

	// Create a minimal template
	tplDir := filepath.Join(globalScionDir, "templates", "default")
	os.MkdirAll(tplDir, 0755)
	os.WriteFile(filepath.Join(tplDir, "scion-agent.json"), []byte(`{"default_harness_config": "test-harness"}`), 0644)

	// Settings without harness_configs entries (simulating default_settings.yaml)
	os.WriteFile(filepath.Join(globalScionDir, "settings.yaml"), []byte(`schema_version: "1"
active_profile: local
profiles:
  local:
    runtime: docker
`), 0644)

	// Create project grove
	projectDir := filepath.Join(tmpDir, "project")
	projectScionDir := filepath.Join(projectDir, ".scion")
	os.MkdirAll(projectScionDir, 0755)

	// Capture the RunConfig
	var capturedConfig runtime.RunConfig
	mockRT := &runtime.MockRuntime{
		ListFunc: func(ctx context.Context, labelFilter map[string]string) ([]api.AgentInfo, error) {
			return []api.AgentInfo{}, nil
		},
		RunFunc: func(ctx context.Context, config runtime.RunConfig) (string, error) {
			capturedConfig = config
			return "mock-id", nil
		},
	}

	mgr := NewManager(mockRT)

	_, err := mgr.Start(context.Background(), api.StartOptions{
		Name:      "test-agent",
		GrovePath: projectScionDir,
		NoAuth:    true,
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if capturedConfig.UnixUsername != "scion" {
		t.Errorf("expected UnixUsername = %q, got %q", "scion", capturedConfig.UnixUsername)
	}
}

func TestStartResolvesHarnessConfigUserSettingsOverride(t *testing.T) {
	// When settings define a user in harness_configs, it should override
	// the on-disk harness-config user.
	tmpDir := t.TempDir()

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	globalScionDir := filepath.Join(tmpDir, ".scion")

	// Create harness-config with user field
	hcDir := filepath.Join(globalScionDir, "harness-configs", "test-harness")
	os.MkdirAll(hcDir, 0755)
	os.WriteFile(filepath.Join(hcDir, "config.yaml"), []byte("harness: gemini\nuser: scion\nimage: test-image:latest\n"), 0644)

	// Create a minimal template
	tplDir := filepath.Join(globalScionDir, "templates", "default")
	os.MkdirAll(tplDir, 0755)
	os.WriteFile(filepath.Join(tplDir, "scion-agent.json"), []byte(`{"default_harness_config": "test-harness"}`), 0644)

	// Settings WITH harness_configs that override the user
	os.WriteFile(filepath.Join(globalScionDir, "settings.yaml"), []byte(`schema_version: "1"
active_profile: local
harness_configs:
  test-harness:
    harness: gemini
    user: custom-user
    image: test-image:latest
profiles:
  local:
    runtime: docker
`), 0644)

	// Create project grove
	projectDir := filepath.Join(tmpDir, "project")
	projectScionDir := filepath.Join(projectDir, ".scion")
	os.MkdirAll(projectScionDir, 0755)

	// Capture the RunConfig
	var capturedConfig runtime.RunConfig
	mockRT := &runtime.MockRuntime{
		ListFunc: func(ctx context.Context, labelFilter map[string]string) ([]api.AgentInfo, error) {
			return []api.AgentInfo{}, nil
		},
		RunFunc: func(ctx context.Context, config runtime.RunConfig) (string, error) {
			capturedConfig = config
			return "mock-id", nil
		},
	}

	mgr := NewManager(mockRT)

	_, err := mgr.Start(context.Background(), api.StartOptions{
		Name:      "test-agent",
		GrovePath: projectScionDir,
		NoAuth:    true,
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if capturedConfig.UnixUsername != "custom-user" {
		t.Errorf("expected UnixUsername = %q, got %q", "custom-user", capturedConfig.UnixUsername)
	}
}

func TestStartReturnsRunningStatus(t *testing.T) {
	// This tests the early-return path when a container is already running.
	// The runtime's List() may return a stale Status (e.g. "created") from the
	// container runtime, but Start() should override it to "running" since
	// isRunning is confirmed true via ContainerStatus.
	mockRT := &runtime.MockRuntime{
		ListFunc: func(ctx context.Context, labelFilter map[string]string) ([]api.AgentInfo, error) {
			return []api.AgentInfo{
				{
					ContainerID:     "abc123",
					Name:            "test-agent",
					ContainerStatus: "Up 2 hours",
					Status:          "created", // stale status from runtime
				},
			}, nil
		},
	}

	mgr := NewManager(mockRT)

	result, err := mgr.Start(context.Background(), api.StartOptions{
		Name: "test-agent",
		// No Task — triggers the early return for already-running containers
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != "running" {
		t.Errorf("expected Status = %q, got %q", "running", result.Status)
	}
}

func TestBuildAgentEnv_TelemetryInjection(t *testing.T) {
	// Simulate the telemetry injection that Start() performs before buildAgentEnv.
	enabled := true
	cloudEnabled := true
	insecure := false

	scionCfg := &api.ScionConfig{
		Telemetry: &api.TelemetryConfig{
			Enabled: &enabled,
			Cloud: &api.TelemetryCloudConfig{
				Enabled:  &cloudEnabled,
				Endpoint: "otel.example.com:4317",
				Protocol: "grpc",
				TLS: &api.TelemetryTLS{
					InsecureSkipVerify: &insecure,
				},
			},
		},
	}

	opts := make(map[string]string)

	// Replicate the injection logic from Start()
	if scionCfg.Telemetry != nil {
		telemetryEnv := config.TelemetryConfigToEnv(scionCfg.Telemetry)
		for k, v := range telemetryEnv {
			if _, exists := opts[k]; !exists {
				opts[k] = v
			}
		}
	}

	env, _ := buildAgentEnv(scionCfg, opts)

	envMap := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	expected := map[string]string{
		"SCION_TELEMETRY_ENABLED":       "true",
		"SCION_TELEMETRY_CLOUD_ENABLED": "true",
		"SCION_OTEL_ENDPOINT":           "otel.example.com:4317",
		"SCION_OTEL_PROTOCOL":           "grpc",
		"SCION_OTEL_INSECURE":           "false",
	}

	for k, want := range expected {
		got, ok := envMap[k]
		if !ok {
			t.Errorf("missing env var %s", k)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestTelemetryEnabledFlag(t *testing.T) {
	// Verify the TelemetryEnabled derivation logic used in Start().
	// telemetryEnabled = cfg != nil && cfg.Telemetry != nil &&
	//   (cfg.Telemetry.Enabled == nil || *cfg.Telemetry.Enabled)

	boolPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name     string
		cfg      *api.ScionConfig
		expected bool
	}{
		{
			name:     "nil config",
			cfg:      nil,
			expected: false,
		},
		{
			name:     "nil telemetry",
			cfg:      &api.ScionConfig{},
			expected: false,
		},
		{
			name:     "telemetry enabled nil (default on)",
			cfg:      &api.ScionConfig{Telemetry: &api.TelemetryConfig{}},
			expected: true,
		},
		{
			name:     "telemetry explicitly enabled",
			cfg:      &api.ScionConfig{Telemetry: &api.TelemetryConfig{Enabled: boolPtr(true)}},
			expected: true,
		},
		{
			name:     "telemetry explicitly disabled",
			cfg:      &api.ScionConfig{Telemetry: &api.TelemetryConfig{Enabled: boolPtr(false)}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cfg != nil && tt.cfg.Telemetry != nil &&
				(tt.cfg.Telemetry.Enabled == nil || *tt.cfg.Telemetry.Enabled)
			if result != tt.expected {
				t.Errorf("telemetryEnabled = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestTelemetryEnabledRunConfig(t *testing.T) {
	// Integration test: verify that harness telemetry env vars appear in
	// RunConfig when telemetry is enabled, and are absent when disabled.
	tmpDir := t.TempDir()

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	globalScionDir := filepath.Join(tmpDir, ".scion")

	// Create harness-config
	hcDir := filepath.Join(globalScionDir, "harness-configs", "test-harness")
	os.MkdirAll(hcDir, 0755)
	os.WriteFile(filepath.Join(hcDir, "config.yaml"), []byte("harness: gemini\nuser: scion\nimage: test-image:latest\n"), 0644)

	// Create template
	tplDir := filepath.Join(globalScionDir, "templates", "default")
	os.MkdirAll(tplDir, 0755)
	os.WriteFile(filepath.Join(tplDir, "scion-agent.json"), []byte(`{"default_harness_config": "test-harness"}`), 0644)

	os.WriteFile(filepath.Join(globalScionDir, "settings.yaml"), []byte(`schema_version: "1"
active_profile: local
profiles:
  local:
    runtime: docker
`), 0644)

	projectDir := filepath.Join(tmpDir, "project")
	projectScionDir := filepath.Join(projectDir, ".scion")
	os.MkdirAll(projectScionDir, 0755)

	t.Run("telemetry enabled passes TelemetryEnabled to RunConfig", func(t *testing.T) {
		var capturedConfig runtime.RunConfig
		mockRT := &runtime.MockRuntime{
			ListFunc: func(ctx context.Context, labelFilter map[string]string) ([]api.AgentInfo, error) {
				return []api.AgentInfo{}, nil
			},
			RunFunc: func(ctx context.Context, config runtime.RunConfig) (string, error) {
				capturedConfig = config
				return "mock-id", nil
			},
		}

		// Create agent with telemetry enabled in scion-agent.json
		agentDir := filepath.Join(projectScionDir, "agents", "telem-on")
		os.MkdirAll(filepath.Join(agentDir, "home"), 0755)
		os.WriteFile(filepath.Join(agentDir, "scion-agent.json"), []byte(`{
			"harness": "gemini",
			"telemetry": {"enabled": true}
		}`), 0644)

		mgr := NewManager(mockRT)
		_, err := mgr.Start(context.Background(), api.StartOptions{
			Name:      "telem-on",
			GrovePath: projectScionDir,
			NoAuth:    true,
		})
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		if !capturedConfig.TelemetryEnabled {
			t.Error("expected TelemetryEnabled = true, got false")
		}
	})

	t.Run("telemetry disabled omits TelemetryEnabled from RunConfig", func(t *testing.T) {
		var capturedConfig runtime.RunConfig
		mockRT := &runtime.MockRuntime{
			ListFunc: func(ctx context.Context, labelFilter map[string]string) ([]api.AgentInfo, error) {
				return []api.AgentInfo{}, nil
			},
			RunFunc: func(ctx context.Context, config runtime.RunConfig) (string, error) {
				capturedConfig = config
				return "mock-id", nil
			},
		}

		agentDir := filepath.Join(projectScionDir, "agents", "telem-off")
		os.MkdirAll(filepath.Join(agentDir, "home"), 0755)
		os.WriteFile(filepath.Join(agentDir, "scion-agent.json"), []byte(`{
			"harness": "gemini",
			"telemetry": {"enabled": false}
		}`), 0644)

		mgr := NewManager(mockRT)
		_, err := mgr.Start(context.Background(), api.StartOptions{
			Name:      "telem-off",
			GrovePath: projectScionDir,
			NoAuth:    true,
		})
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		if capturedConfig.TelemetryEnabled {
			t.Error("expected TelemetryEnabled = false, got true")
		}
	})
}

func TestBuildAgentEnv_TelemetryNoOverrideExplicit(t *testing.T) {
	// Explicit opts.Env values must not be overwritten by telemetry config.
	enabled := true

	scionCfg := &api.ScionConfig{
		Telemetry: &api.TelemetryConfig{
			Enabled: &enabled,
			Cloud: &api.TelemetryCloudConfig{
				Endpoint: "from-config.example.com:4317",
			},
		},
	}

	// Pre-set an explicit override in opts.Env (e.g. from Hub/broker)
	opts := map[string]string{
		"SCION_OTEL_ENDPOINT": "from-broker.example.com:4317",
	}

	// Replicate the injection logic from Start()
	if scionCfg.Telemetry != nil {
		telemetryEnv := config.TelemetryConfigToEnv(scionCfg.Telemetry)
		for k, v := range telemetryEnv {
			if _, exists := opts[k]; !exists {
				opts[k] = v
			}
		}
	}

	env, _ := buildAgentEnv(scionCfg, opts)

	envMap := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// The broker's explicit value should win
	if got := envMap["SCION_OTEL_ENDPOINT"]; got != "from-broker.example.com:4317" {
		t.Errorf("SCION_OTEL_ENDPOINT = %q, want %q (explicit override should win)",
			got, "from-broker.example.com:4317")
	}

	// But the telemetry-derived enabled var should still be present
	if got := envMap["SCION_TELEMETRY_ENABLED"]; got != "true" {
		t.Errorf("SCION_TELEMETRY_ENABLED = %q, want %q", got, "true")
	}
}

func TestBuildAgentEnv_HubEnvVarsSurviveMerge(t *testing.T) {
	// Verify that hub env vars injected into opts.Env (from grove settings
	// or dev token resolution) survive the buildAgentEnv merge.
	scionCfg := &api.ScionConfig{}
	extraEnv := map[string]string{
		"SCION_HUB_ENDPOINT":          "http://localhost:9810",
		"SCION_HUB_URL":              "http://localhost:9810",
		"SCION_SERVER_AUTH_DEV_TOKEN": "scion-dev-test-token-123",
		"SCION_AGENT_NAME":           "test-agent",
	}

	env, _ := buildAgentEnv(scionCfg, extraEnv)

	envMap := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	expected := map[string]string{
		"SCION_HUB_ENDPOINT":          "http://localhost:9810",
		"SCION_HUB_URL":              "http://localhost:9810",
		"SCION_SERVER_AUTH_DEV_TOKEN": "scion-dev-test-token-123",
		"SCION_AGENT_NAME":           "test-agent",
	}
	for k, want := range expected {
		got, ok := envMap[k]
		if !ok {
			t.Errorf("missing env var %s", k)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestStartInjectsHubEnvFromGroveSettings(t *testing.T) {
	// When grove settings have hub enabled with an endpoint, Start() should
	// inject SCION_HUB_ENDPOINT and SCION_HUB_URL into the container env.
	tmpDir := t.TempDir()

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	// Clear dev token env vars so we control the test
	for _, k := range []string{"SCION_DEV_TOKEN", "SCION_SERVER_AUTH_DEV_TOKEN", "SCION_DEV_TOKEN_FILE"} {
		if old, ok := os.LookupEnv(k); ok {
			defer os.Setenv(k, old)
			os.Unsetenv(k)
		}
	}

	globalScionDir := filepath.Join(tmpDir, ".scion")

	// Create harness-config
	hcDir := filepath.Join(globalScionDir, "harness-configs", "test-harness")
	os.MkdirAll(hcDir, 0755)
	os.WriteFile(filepath.Join(hcDir, "config.yaml"), []byte("harness: gemini\nuser: scion\nimage: test-image:latest\n"), 0644)

	// Create a minimal template
	tplDir := filepath.Join(globalScionDir, "templates", "default")
	os.MkdirAll(tplDir, 0755)
	os.WriteFile(filepath.Join(tplDir, "scion-agent.json"), []byte(`{"default_harness_config": "test-harness"}`), 0644)

	// Global settings
	os.WriteFile(filepath.Join(globalScionDir, "settings.yaml"), []byte(`schema_version: "1"
active_profile: local
profiles:
  local:
    runtime: docker
`), 0644)

	// Create project grove with hub-enabled settings
	projectDir := filepath.Join(tmpDir, "project")
	projectScionDir := filepath.Join(projectDir, ".scion")
	os.MkdirAll(projectScionDir, 0755)
	os.WriteFile(filepath.Join(projectScionDir, "settings.yaml"), []byte(`hub:
  enabled: true
  endpoint: "http://localhost:9810"
`), 0644)

	// Write a dev-token file so the token resolution finds it
	os.WriteFile(filepath.Join(globalScionDir, "dev-token"), []byte("scion-dev-test-token-abc"), 0644)

	// Capture the RunConfig
	var capturedConfig runtime.RunConfig
	mockRT := &runtime.MockRuntime{
		ListFunc: func(ctx context.Context, labelFilter map[string]string) ([]api.AgentInfo, error) {
			return []api.AgentInfo{}, nil
		},
		RunFunc: func(ctx context.Context, cfg runtime.RunConfig) (string, error) {
			capturedConfig = cfg
			return "mock-id", nil
		},
	}

	mgr := NewManager(mockRT)

	_, err := mgr.Start(context.Background(), api.StartOptions{
		Name:      "test-agent",
		GrovePath: projectScionDir,
		NoAuth:    true,
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Convert env slice to map
	envMap := make(map[string]string)
	for _, e := range capturedConfig.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	if got := envMap["SCION_HUB_ENDPOINT"]; got != "http://localhost:9810" {
		t.Errorf("SCION_HUB_ENDPOINT = %q, want %q", got, "http://localhost:9810")
	}
	if got := envMap["SCION_HUB_URL"]; got != "http://localhost:9810" {
		t.Errorf("SCION_HUB_URL = %q, want %q", got, "http://localhost:9810")
	}
	if got := envMap["SCION_SERVER_AUTH_DEV_TOKEN"]; got != "scion-dev-test-token-abc" {
		t.Errorf("SCION_SERVER_AUTH_DEV_TOKEN = %q, want %q", got, "scion-dev-test-token-abc")
	}
}

func TestStartPreservesExplicitHubEndpoint(t *testing.T) {
	// When hub endpoint is already set in opts.Env (e.g. from broker dispatch),
	// grove settings should NOT override it.
	tmpDir := t.TempDir()

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tmpDir)

	globalScionDir := filepath.Join(tmpDir, ".scion")

	// Create harness-config
	hcDir := filepath.Join(globalScionDir, "harness-configs", "test-harness")
	os.MkdirAll(hcDir, 0755)
	os.WriteFile(filepath.Join(hcDir, "config.yaml"), []byte("harness: gemini\nuser: scion\nimage: test-image:latest\n"), 0644)

	// Create a minimal template
	tplDir := filepath.Join(globalScionDir, "templates", "default")
	os.MkdirAll(tplDir, 0755)
	os.WriteFile(filepath.Join(tplDir, "scion-agent.json"), []byte(`{"default_harness_config": "test-harness"}`), 0644)

	// Global settings
	os.WriteFile(filepath.Join(globalScionDir, "settings.yaml"), []byte(`schema_version: "1"
active_profile: local
profiles:
  local:
    runtime: docker
`), 0644)

	// Create project grove with hub-enabled settings (different endpoint)
	projectDir := filepath.Join(tmpDir, "project")
	projectScionDir := filepath.Join(projectDir, ".scion")
	os.MkdirAll(projectScionDir, 0755)
	os.WriteFile(filepath.Join(projectScionDir, "settings.yaml"), []byte(`hub:
  enabled: true
  endpoint: "http://grove-setting:9810"
`), 0644)

	// Capture the RunConfig
	var capturedConfig runtime.RunConfig
	mockRT := &runtime.MockRuntime{
		ListFunc: func(ctx context.Context, labelFilter map[string]string) ([]api.AgentInfo, error) {
			return []api.AgentInfo{}, nil
		},
		RunFunc: func(ctx context.Context, cfg runtime.RunConfig) (string, error) {
			capturedConfig = cfg
			return "mock-id", nil
		},
	}

	mgr := NewManager(mockRT)

	_, err := mgr.Start(context.Background(), api.StartOptions{
		Name:      "test-agent",
		GrovePath: projectScionDir,
		NoAuth:    true,
		Env: map[string]string{
			"SCION_HUB_ENDPOINT": "http://broker-dispatch:9810",
		},
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	envMap := make(map[string]string)
	for _, e := range capturedConfig.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// Broker-dispatched endpoint should be preserved, not overwritten by grove settings
	if got := envMap["SCION_HUB_ENDPOINT"]; got != "http://broker-dispatch:9810" {
		t.Errorf("SCION_HUB_ENDPOINT = %q, want %q (explicit should win over grove settings)", got, "http://broker-dispatch:9810")
	}
}
