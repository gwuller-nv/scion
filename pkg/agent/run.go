package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ptone/scion-agent/pkg/api"
	"github.com/ptone/scion-agent/pkg/config"
	"github.com/ptone/scion-agent/pkg/harness"
	"github.com/ptone/scion-agent/pkg/runtime"
	"github.com/ptone/scion-agent/pkg/util"
)

func (m *AgentManager) Start(ctx context.Context, opts api.StartOptions) (*api.AgentInfo, error) {
	// 0. Check if container already exists
	agents, err := m.Runtime.List(ctx, nil)
	if err == nil {
		for _, a := range agents {
			if a.ID == opts.Name || a.Name == opts.Name {
				status := strings.ToLower(a.ContainerStatus)
				isRunning := strings.HasPrefix(status, "up") || status == "running"
				if isRunning {
					// If a new task is provided, we might want to recreate even if running
					// but if no task provided, we just return the running one
					if opts.Task == "" {
						a.Detached = true
						if opts.Detached != nil {
							a.Detached = *opts.Detached
						}
						return &a, nil
					}
				}
				// If it exists but not running (or we have a new task), we delete it so we can recreate it
				if err := m.Runtime.Delete(ctx, a.ID); err != nil {
					return nil, fmt.Errorf("failed to cleanup existing container: %w", err)
				}
			}
		}
	}

	projectDir, err := config.GetResolvedProjectDir(opts.GrovePath)
	if err != nil {
		return nil, err
	}
	groveName := config.GetGroveName(projectDir)

	agentDir, agentHome, agentWorkspace, finalScionCfg, err := GetAgent(ctx, opts.Name, opts.Template, opts.Image, opts.GrovePath, opts.Profile, "")
	if err != nil {
		return nil, err
	}

	promptFile := filepath.Join(agentDir, "prompt.md")
	promptFileContent := ""
	if content, err := os.ReadFile(promptFile); err == nil {
		promptFileContent = strings.TrimSpace(string(content))
	}

	task := opts.Task
	// If we are explicitly attaching or resuming, we allow starting without a task
	isAttaching := opts.Detached != nil && !*opts.Detached
	if task == "" && promptFileContent == "" && !isAttaching && !opts.Resume {
		return nil, fmt.Errorf("no task provided: prompt.md is empty and no task was given in options")
	}

	if task != "" && promptFileContent != "" && task != promptFileContent {
		return nil, fmt.Errorf("task conflict: both prompt.md and start options provide a task")
	}

	if task == "" && !opts.Resume {
		task = promptFileContent
	} else if promptFileContent == "" && task != "" {
		_ = os.WriteFile(promptFile, []byte(task), 0644)
	}

	// Load settings for registry resolution
	settings, err := config.LoadSettings(projectDir)
	if err != nil {
		// Fallback to defaults or log?
	}

	harnessName := ""
	if finalScionCfg != nil {
		harnessName = finalScionCfg.Harness
	}

	// Default values
	resolvedImage := "gemini-cli-sandbox"
	unixUsername := "root"
	useTmux := false
	profileName := opts.Profile

	if settings != nil && harnessName != "" {
		hConfig, err := settings.ResolveHarness(opts.Profile, harnessName)
		if err == nil {
			resolvedImage = hConfig.Image
			unixUsername = hConfig.User
		}
	}

	if settings != nil {
		if profileName == "" {
			profileName = settings.ActiveProfile
		}
		if p, ok := settings.Profiles[profileName]; ok {
			// 1. Start with runtime default if available
			if rCfg, ok := settings.Runtimes[p.Runtime]; ok && rCfg.Tmux != nil {
				useTmux = *rCfg.Tmux
			}
			// 2. Override with profile setting if explicitly set
			if p.Tmux != nil {
				useTmux = *p.Tmux
			}
		}
	}

	if m.Runtime.Name() == "container" && !useTmux {
		fmt.Fprintf(os.Stderr, "Warning: Apple container runtime does not support 'attach' without tmux. Sessions will be non-interactive after start.\n")
	}

	// CLI Overrides
	if opts.Image != "" {
		resolvedImage = opts.Image
	}

	h := harness.New(harnessName)

	// 3. Propagate credentials
	var auth api.AuthConfig
	if !opts.NoAuth {
		if opts.Auth != nil {
			auth, err = opts.Auth.GetAuthConfig(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get auth: %w", err)
			}
		} else {
			// Fallback to legacy discovery if no harness given
			auth = h.DiscoverAuth(agentHome)
		}
	}

	// 4. Launch container
	detached := true

	if finalScionCfg != nil {
		detached = finalScionCfg.IsDetached()
	}

	if opts.Detached != nil {
		detached = *opts.Detached
	}

	if useTmux {
		// We no longer automatically append -tmux to the image tag.
		// We launch optimistically and provide a clear error if tmux is missing.
	}

	exists, err := m.Runtime.ImageExists(ctx, resolvedImage)
	if err != nil || !exists {
		if err := m.Runtime.PullImage(ctx, resolvedImage); err != nil {
			return nil, fmt.Errorf("failed to pull image '%s': %w", resolvedImage, err)
		}
	}

	agentEnv := buildAgentEnv(finalScionCfg, opts.Env)

	template := ""
	if finalScionCfg != nil && finalScionCfg.Info != nil {
		template = finalScionCfg.Info.Template
	}

	repoRoot := ""
	if util.IsGitRepo() {
		repoRoot, _ = util.RepoRoot()
	}

	runCfg := runtime.RunConfig{
		Name:         opts.Name,
		Template:     template,
		UnixUsername: unixUsername,
		Image:        resolvedImage,
		HomeDir:      agentHome,
		Workspace:    agentWorkspace,
		RepoRoot:     repoRoot,
		Auth:         auth,
		Harness:      h,
		UseTmux:      useTmux,
		Task:         task,
		CommandArgs: func() []string {
			if finalScionCfg != nil {
				return finalScionCfg.CommandArgs
			}
			return nil
		}(),
		Env:          agentEnv,
		Volumes: func() []api.VolumeMount {
			if finalScionCfg != nil {
				return finalScionCfg.Volumes
			}
			return nil
		}(),
		Resume: opts.Resume,
		Labels: map[string]string{
			"scion.agent": "true",
			"scion.name":  opts.Name,
			"scion.grove": groveName,
		},
		Annotations: map[string]string{
			"scion.grove_path": projectDir,
		},
	}
	id, err := m.Runtime.Run(ctx, runCfg)
	if err != nil {
		if useTmux && (strings.Contains(err.Error(), "executable file not found") ||
			strings.Contains(err.Error(), "no such file or directory") ||
			strings.Contains(err.Error(), "tmux")) {
			return nil, fmt.Errorf("failed to launch container with tmux: tmux binary not found in image '%s'. "+
				"Please ensure the image has tmux installed, use an image with a :tmux tag, or disable tmux in settings. Error: %w", resolvedImage, err)
		}
		return nil, fmt.Errorf("failed to launch container: %w", err)
	}

	status := "running"
	if opts.Resume {
		status = "resumed"
	}
	_ = UpdateAgentConfig(opts.Name, opts.GrovePath, status, m.Runtime.Name(), profileName, "ACTIVE")

	// Fetch fresh info
	allAgents, err := m.Runtime.List(ctx, map[string]string{"scion.name": opts.Name})
	if err == nil {
		for _, a := range allAgents {
			if a.ID == id || a.Name == opts.Name {
				a.Detached = detached
				return &a, nil
			}
		}
	}

	return &api.AgentInfo{ID: id, Name: opts.Name, Status: status, Detached: detached}, nil
}

func buildAgentEnv(scionCfg *api.ScionConfig, extraEnv map[string]string) []string {
	agentEnv := []string{}
	if scionCfg != nil && scionCfg.Env != nil {
		for k, v := range scionCfg.Env {
			if v == "" {
				// Inherit from host environment
				if hostVal, exists := os.LookupEnv(k); exists {
					v = hostVal
				}
			}
			agentEnv = append(agentEnv, fmt.Sprintf("%s=%s", k, v))
		}
	}
	// Add extraEnv
	for k, v := range extraEnv {
		agentEnv = append(agentEnv, fmt.Sprintf("%s=%s", k, v))
	}
	return agentEnv
}
		