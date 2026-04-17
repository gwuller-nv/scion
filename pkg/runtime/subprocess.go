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

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/scion/pkg/api"
	"github.com/GoogleCloudPlatform/scion/pkg/config"
)

type SubprocessRuntime struct {
	baseDir string
	tmuxBin string
}

type subprocessAgentState struct {
	ContainerID  string            `json:"containerId"`
	SessionName  string            `json:"sessionName"`
	Name         string            `json:"name"`
	InternalName string            `json:"internalName"`
	Template     string            `json:"template,omitempty"`
	Harness      string            `json:"harness"`
	HarnessAuth  string            `json:"harnessAuth,omitempty"`
	HarnessCfg   string            `json:"harnessConfig,omitempty"`
	Grove        string            `json:"grove,omitempty"`
	GroveID      string            `json:"groveId,omitempty"`
	GrovePath    string            `json:"grovePath,omitempty"`
	HomeDir      string            `json:"homeDir"`
	Workspace    string            `json:"workspace,omitempty"`
	LogPath      string            `json:"logPath"`
	ScriptPath   string            `json:"scriptPath"`
	InterruptKey string            `json:"interruptKey,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	Created      time.Time         `json:"created"`
	Updated      time.Time         `json:"updated"`
}

func NewSubprocessRuntime() Runtime {
	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		return &ErrorRuntime{Err: fmt.Errorf("subprocess runtime requires tmux on PATH: %w", err)}
	}
	if _, err := exec.LookPath("bash"); err != nil {
		return &ErrorRuntime{Err: fmt.Errorf("subprocess runtime requires bash on PATH: %w", err)}
	}

	baseDir := ""
	if globalDir, err := config.GetGlobalDir(); err == nil {
		baseDir = filepath.Join(globalDir, "subprocess-runtime")
	} else if homeDir, homeErr := os.UserHomeDir(); homeErr == nil {
		baseDir = filepath.Join(homeDir, ".scion", "subprocess-runtime")
	}
	if baseDir == "" {
		return &ErrorRuntime{Err: fmt.Errorf("failed to resolve subprocess runtime base directory")}
	}

	return &SubprocessRuntime{
		baseDir: baseDir,
		tmuxBin: tmuxBin,
	}
}

func (r *SubprocessRuntime) Name() string {
	return "subprocess"
}

func (r *SubprocessRuntime) ExecUser() string {
	return "scion"
}

func (r *SubprocessRuntime) Run(ctx context.Context, config RunConfig) (string, error) {
	if err := r.validateConfig(config); err != nil {
		return "", err
	}
	if config.Harness == nil {
		return "", fmt.Errorf("subprocess runtime requires a harness")
	}
	if err := r.ensureDirs(); err != nil {
		return "", err
	}

	sessionName := sanitizeSessionName(config.Name)
	logPath := filepath.Join(r.logsDir(), sessionName+".log")
	scriptPath := filepath.Join(r.scriptsDir(), sessionName+".sh")
	workDir := config.Workspace
	if workDir == "" {
		workDir = config.HomeDir
	}
	if workDir == "" {
		return "", fmt.Errorf("subprocess runtime requires a workspace or home directory")
	}

	state := &subprocessAgentState{
		ContainerID:  sessionName,
		SessionName:  sessionName,
		Name:         agentNameFromEnv(config.Env, config.Name),
		InternalName: config.Name,
		Template:     config.Template,
		Harness:      config.Harness.Name(),
		HarnessAuth:  config.Labels["scion.harness_auth"],
		HarnessCfg:   config.Labels["scion.harness_config"],
		Grove:        config.Labels["scion.grove"],
		GroveID:      config.Labels["scion.grove_id"],
		GrovePath:    config.Annotations["scion.grove_path"],
		HomeDir:      config.HomeDir,
		Workspace:    config.Workspace,
		LogPath:      logPath,
		ScriptPath:   scriptPath,
		InterruptKey: config.Harness.GetInterruptKey(),
		Labels:       copyMap(config.Labels),
		Annotations:  copyMap(config.Annotations),
		Created:      time.Now(),
		Updated:      time.Now(),
	}

	commandArgs := config.Harness.GetCommand(config.Task, config.Resume, config.CommandArgs)
	script := buildSubprocessLaunchScript(workDir, config.HomeDir, config.UnixUsername, config.Env, logPath, commandArgs)
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return "", fmt.Errorf("write launch script: %w", err)
	}
	if err := os.WriteFile(logPath, []byte{}, 0644); err != nil {
		return "", fmt.Errorf("create log file: %w", err)
	}

	_ = r.killSession(ctx, sessionName)
	if err := r.tmux(ctx, "new-session", "-d", "-s", sessionName, "-n", "agent", scriptPath); err != nil {
		return "", fmt.Errorf("start tmux session: %w", err)
	}
	if err := r.tmux(ctx, "set-option", "-t", sessionName, "remain-on-exit", "on"); err != nil {
		return "", fmt.Errorf("configure tmux session: %w", err)
	}
	if err := r.tmux(ctx, "new-window", "-t", sessionName, "-n", "shell"); err != nil {
		return "", fmt.Errorf("create tmux shell window: %w", err)
	}
	if err := r.writeState(state); err != nil {
		return "", err
	}

	return sessionName, nil
}

func (r *SubprocessRuntime) Stop(ctx context.Context, id string) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	if state.InterruptKey == "" {
		state.InterruptKey = "C-c"
	}
	_ = r.tmux(ctx, "send-keys", "-t", state.SessionName+":0", state.InterruptKey)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status, _, statusErr := r.sessionStatus(ctx, state.SessionName)
		if statusErr != nil || status != "running" {
			state.Updated = time.Now()
			return r.writeState(state)
		}
		time.Sleep(200 * time.Millisecond)
	}

	if err := r.killSession(ctx, state.SessionName); err != nil {
		return err
	}
	state.Updated = time.Now()
	return r.writeState(state)
}

func (r *SubprocessRuntime) Delete(ctx context.Context, id string) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	_ = r.killSession(ctx, state.SessionName)
	_ = os.Remove(state.ScriptPath)
	_ = os.Remove(r.statePath(state.ContainerID))
	return nil
}

func (r *SubprocessRuntime) List(ctx context.Context, labelFilter map[string]string) ([]api.AgentInfo, error) {
	states, err := r.loadAllStates()
	if err != nil {
		return nil, err
	}

	agents := make([]api.AgentInfo, 0, len(states))
	for _, state := range states {
		info, statusErr := r.stateToAgentInfo(ctx, state)
		if statusErr != nil {
			continue
		}
		if matchesSubprocessFilter(info, labelFilter) {
			agents = append(agents, info)
		}
	}

	sort.Slice(agents, func(i, j int) bool {
		if agents[i].Created.Equal(agents[j].Created) {
			return agents[i].Name < agents[j].Name
		}
		return agents[i].Created.Before(agents[j].Created)
	})

	return agents, nil
}

func (r *SubprocessRuntime) GetLogs(ctx context.Context, id string) (string, error) {
	state, err := r.loadState(id)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(state.LogPath)
	if err == nil {
		return string(data), nil
	}
	if _, _, statusErr := r.sessionStatus(ctx, state.SessionName); statusErr == nil {
		out, captureErr := r.tmuxOutput(ctx, "capture-pane", "-p", "-S", "-", "-t", state.SessionName+":0")
		if captureErr == nil {
			return out, nil
		}
	}
	return "", err
}

func (r *SubprocessRuntime) Attach(ctx context.Context, id string) error {
	state, err := r.loadState(id)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, r.tmuxBin, "attach-session", "-t", state.SessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (r *SubprocessRuntime) ImageExists(ctx context.Context, image string) (bool, error) {
	return true, nil
}

func (r *SubprocessRuntime) PullImage(ctx context.Context, image string) error {
	return nil
}

func (r *SubprocessRuntime) Sync(ctx context.Context, id string, direction SyncDirection) error {
	return nil
}

func (r *SubprocessRuntime) Exec(ctx context.Context, id string, cmd []string) (string, error) {
	state, err := r.loadState(id)
	if err != nil {
		return "", err
	}
	cmd = rewriteSessionTarget(cmd, state.SessionName)
	if len(cmd) == 0 {
		return "", fmt.Errorf("empty exec command")
	}
	command := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	command.Env = append(os.Environ(), "HOME="+state.HomeDir)
	if state.Workspace != "" {
		command.Dir = state.Workspace
	} else if state.HomeDir != "" {
		command.Dir = state.HomeDir
	}
	out, err := command.CombinedOutput()
	return string(out), err
}

func (r *SubprocessRuntime) GetWorkspacePath(ctx context.Context, id string) (string, error) {
	state, err := r.loadState(id)
	if err != nil {
		return "", err
	}
	if state.Workspace != "" {
		return state.Workspace, nil
	}
	return state.HomeDir, nil
}

func (r *SubprocessRuntime) validateConfig(config RunConfig) error {
	if len(config.Volumes) > 0 {
		return fmt.Errorf("subprocess runtime does not support volume mounts")
	}
	if len(config.ResolvedSecrets) > 0 {
		for _, s := range config.ResolvedSecrets {
			if s.Type == "file" {
				return fmt.Errorf("subprocess runtime does not support file-projected secrets")
			}
		}
	}
	if config.Kubernetes != nil {
		return fmt.Errorf("subprocess runtime does not support kubernetes config")
	}
	if config.Resources != nil {
		return fmt.Errorf("subprocess runtime does not support container resources")
	}
	if config.NetworkMode != "" || len(config.ExtraHosts) > 0 {
		return fmt.Errorf("subprocess runtime does not support container networking options")
	}
	return nil
}

func (r *SubprocessRuntime) ensureDirs() error {
	for _, dir := range []string{r.baseDir, r.stateDir(), r.logsDir(), r.scriptsDir()} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create subprocess runtime dir %s: %w", dir, err)
		}
	}
	return nil
}

func (r *SubprocessRuntime) stateDir() string {
	return filepath.Join(r.baseDir, "agents")
}

func (r *SubprocessRuntime) logsDir() string {
	return filepath.Join(r.baseDir, "logs")
}

func (r *SubprocessRuntime) scriptsDir() string {
	return filepath.Join(r.baseDir, "scripts")
}

func (r *SubprocessRuntime) statePath(id string) string {
	return filepath.Join(r.stateDir(), id+".json")
}

func (r *SubprocessRuntime) writeState(state *subprocessAgentState) error {
	if err := r.ensureDirs(); err != nil {
		return err
	}
	state.Updated = time.Now()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal subprocess agent state: %w", err)
	}
	return os.WriteFile(r.statePath(state.ContainerID), data, 0644)
}

func (r *SubprocessRuntime) loadState(id string) (*subprocessAgentState, error) {
	for _, candidate := range []string{id, sanitizeSessionName(id)} {
		raw, err := os.ReadFile(r.statePath(candidate))
		if err != nil {
			continue
		}
		var state subprocessAgentState
		if err := json.Unmarshal(raw, &state); err != nil {
			return nil, fmt.Errorf("parse subprocess agent state: %w", err)
		}
		return &state, nil
	}
	return nil, fmt.Errorf("subprocess agent %q not found", id)
}

func (r *SubprocessRuntime) loadAllStates() ([]*subprocessAgentState, error) {
	if err := r.ensureDirs(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(r.stateDir())
	if err != nil {
		return nil, err
	}
	states := make([]*subprocessAgentState, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(r.stateDir(), entry.Name()))
		if err != nil {
			continue
		}
		var state subprocessAgentState
		if err := json.Unmarshal(raw, &state); err != nil {
			continue
		}
		states = append(states, &state)
	}
	return states, nil
}

func (r *SubprocessRuntime) stateToAgentInfo(ctx context.Context, state *subprocessAgentState) (api.AgentInfo, error) {
	status, exitCode, err := r.sessionStatus(ctx, state.SessionName)
	if err != nil {
		status = "stopped"
	}
	containerStatus := status
	phase := "running"
	activity := "idle"
	if status != "running" {
		phase = "stopped"
		activity = "completed"
		if exitCode > 0 {
			phase = "error"
			containerStatus = fmt.Sprintf("exited (%d)", exitCode)
		}
	}

	labels := copyMap(state.Labels)
	if labels == nil {
		labels = map[string]string{}
	}

	return api.AgentInfo{
		ContainerID:     state.ContainerID,
		Name:            state.Name,
		Template:        state.Template,
		HarnessConfig:   state.HarnessCfg,
		HarnessAuth:     state.HarnessAuth,
		Grove:           state.Grove,
		GroveID:         state.GroveID,
		GrovePath:       state.GrovePath,
		Labels:          labels,
		Annotations:     copyMap(state.Annotations),
		ContainerStatus: containerStatus,
		Phase:           phase,
		Activity:        activity,
		Detached:        true,
		Runtime:         r.Name(),
		Created:         state.Created,
		Updated:         state.Updated,
		LastSeen:        time.Now(),
		Image:           "host-process",
		RuntimeState:    status,
	}, nil
}

func (r *SubprocessRuntime) sessionStatus(ctx context.Context, session string) (string, int, error) {
	if err := r.tmux(ctx, "has-session", "-t", session); err != nil {
		return "stopped", 0, err
	}
	out, err := r.tmuxOutput(ctx, "display-message", "-t", session+":0", "-p", "#{pane_dead}:#{pane_dead_status}")
	if err != nil {
		return "stopped", 0, err
	}
	parts := strings.Split(strings.TrimSpace(out), ":")
	if len(parts) != 2 {
		return "running", 0, nil
	}
	if parts[0] == "1" {
		exitCode := 0
		fmt.Sscanf(parts[1], "%d", &exitCode)
		return "stopped", exitCode, nil
	}
	return "running", 0, nil
}

func (r *SubprocessRuntime) killSession(ctx context.Context, session string) error {
	if err := r.tmux(ctx, "kill-session", "-t", session); err != nil {
		if strings.Contains(err.Error(), "can't find session") {
			return nil
		}
		return err
	}
	return nil
}

func (r *SubprocessRuntime) tmux(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, r.tmuxBin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r *SubprocessRuntime) tmuxOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.tmuxBin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func sanitizeSessionName(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		result = "agent"
	}
	if !strings.HasPrefix(result, "scion-") {
		result = "scion-" + result
	}
	return result
}

func buildSubprocessLaunchScript(workDir, homeDir, unixUsername string, env []string, logPath string, commandArgs []string) string {
	lines := []string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"export HOME=" + shellQuote(homeDir),
		"export USER=" + shellQuote(unixUsername),
		"export LOGNAME=" + shellQuote(unixUsername),
		"cd " + shellQuote(workDir),
		"mkdir -p " + shellQuote(filepath.Dir(logPath)),
		"exec >>" + shellQuote(logPath) + " 2>&1",
	}
	for _, entry := range env {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		lines = append(lines, "export "+parts[0]+"="+shellQuote(parts[1]))
	}
	lines = append(lines, "exec "+shellJoin(commandArgs))
	return strings.Join(lines, "\n") + "\n"
}

func shellJoin(args []string) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func copyMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func agentNameFromEnv(env []string, fallback string) string {
	for _, entry := range env {
		if strings.HasPrefix(entry, "SCION_AGENT_NAME=") {
			return strings.TrimPrefix(entry, "SCION_AGENT_NAME=")
		}
	}
	if idx := strings.LastIndex(fallback, "--"); idx >= 0 && idx+2 < len(fallback) {
		return fallback[idx+2:]
	}
	return fallback
}

func rewriteSessionTarget(cmd []string, session string) []string {
	rewritten := append([]string(nil), cmd...)
	for i := 0; i < len(rewritten)-1; i++ {
		if rewritten[i] != "-t" {
			continue
		}
		target := rewritten[i+1]
		if target == "scion" || strings.HasPrefix(target, "scion:") {
			rewritten[i+1] = strings.Replace(target, "scion", session, 1)
		}
	}
	return rewritten
}

func matchesSubprocessFilter(info api.AgentInfo, filter map[string]string) bool {
	if len(filter) == 0 {
		return true
	}
	for key, value := range filter {
		switch key {
		case "status":
			if info.RuntimeState != value && info.Phase != value && info.ContainerStatus != value {
				return false
			}
		default:
			if info.Labels[key] != value {
				return false
			}
		}
	}
	return true
}
