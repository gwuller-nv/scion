# Agent Services: Template-Defined Sidecar Processes

## Overview

Templates should be able to specify additional processes ("services") that start automatically alongside the main harness process inside the container. The motivating use case is MCP servers (e.g., Chrome DevTools MCP) and supporting daemons (e.g., Xvfb for headless browser rendering), but the mechanism should be general enough to support any background process an agent template needs.

Today, `sciontool init` supervises exactly one child process (the harness command). This design extends that to support a set of named services defined in `scion-agent.yaml`, managed by sciontool with restart and lifecycle semantics.

## Current Architecture

### Container startup sequence

```
Container entrypoint
  └─ sciontool init -- <harness-command>
       ├─ StartReaper()           (zombie cleanup, critical for PID 1)
       ├─ setupHostUser()         (UID/GID sync)
       ├─ Initialize telemetry
       ├─ Initialize lifecycle hooks
       ├─ RunPreStart hooks
       ├─ Create supervisor       (manages ONE child process)
       ├─ Start signal handler
       ├─ Spawn child process     (the harness: claude, gemini, etc.)
       ├─ RunPostStart hooks
       ├─ Hub reporting + heartbeat
       ├─ Wait for child exit
       └─ RunSessionEnd hooks
```

### Relevant types

- `api.ScionConfig` (`pkg/api/types.go:105`) — The struct backing `scion-agent.yaml`. Currently has `harness`, `env`, `volumes`, `command_args`, `model`, `image`, etc.
- `supervisor.Supervisor` (`pkg/sciontool/supervisor/`) — Manages the child process lifecycle with signal forwarding, grace period shutdown, and UID/GID dropping.
- Lifecycle hooks (`pkg/sciontool/hooks/`) — Pre-start, post-start, pre-stop, session-end events with registered handlers.

### Current scion-agent.yaml (minimal)

```yaml
harness: claude
```

## Schema

Add a `services` field to `scion-agent.yaml`:

```yaml
harness: claude

services:
  - name: chrome-mcp
    command: ["npx", "@anthropic-ai/chrome-devtools-mcp@latest"]
    restart: on-failure
    env:
      CHROME_PATH: "/usr/bin/chromium"

  - name: xvfb
    command: ["Xvfb", ":99", "-screen", "0", "1280x1024x24"]
    restart: always
    env:
      DISPLAY: ":99"
```

Services are defined in `scion-agent.yaml` only — they travel with the template and are provisioned per-agent. No per-agent service overrides are supported; modifying services for a specific agent instance is handled through post-create agent editing.

### ServiceSpec fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | yes | — | Unique identifier, used for logging and status |
| `command` | string[] | yes | — | Command and arguments (exec form only, no shell) |
| `restart` | string | no | `"no"` | Restart policy: `"no"`, `"on-failure"`, `"always"` |
| `env` | map[string]string | no | — | Environment variables, merged with template env |
| `ready_check` | ReadyCheck | no | — | Optional readiness gate (see below) |

### Corresponding Go type

```go
// ServiceSpec defines a sidecar process to run alongside the main harness.
type ServiceSpec struct {
    Name       string            `json:"name" yaml:"name"`
    Command    []string          `json:"command" yaml:"command"`
    Restart    string            `json:"restart,omitempty" yaml:"restart,omitempty"`
    Env        map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
    ReadyCheck *ReadyCheck       `json:"ready_check,omitempty" yaml:"ready_check,omitempty"`
}

type ReadyCheck struct {
    Type    string `json:"type" yaml:"type"`       // "tcp", "http", "delay"
    Target  string `json:"target" yaml:"target"`   // "localhost:9222", "http://localhost:8080/health", "3s"
    Timeout string `json:"timeout" yaml:"timeout"` // max wait before giving up
}
```

## Config Discovery

The `scion-agent.yaml` file remains at the root of the template/agent definition. On agent creation, the host-side `scion` CLI copies the `services` block from `scion-agent.yaml` into a separate `scion-services.yaml` file in the agent's home directory (`$HOME/.scion/scion-services.yaml`). At container startup, `sciontool init` reads service definitions from this well-known path.

This separation keeps `scion-agent.yaml` as the source of truth for template configuration while giving `sciontool` a clean, dedicated file to consume without needing awareness of the full template config schema.

## Lifecycle Semantics

### Startup order

1. `sciontool init` runs pre-start hooks (existing)
2. `sciontool` reads `scion-services.yaml` from the agent's home directory
3. Services are started in array order
4. If a service has a `ready_check`, sciontool waits for it to pass before starting the next service
5. After all services are running (or ready), the main harness process is started via the existing supervisor
6. Post-start hooks run (existing)

### Steady state

- Services are monitored in a background goroutine (supervised sidecar model)
- If a service exits and its restart policy allows, it is restarted with backoff
- After 3 consecutive failed restarts, the service is abandoned (no further restart attempts). Failure events are logged to the service's sciontool lifecycle log
- The main process (harness) remains the "primary" — the container's exit code is determined by the main process, not services

### Shutdown

1. Main process exits (or signal received)
2. Pre-stop hooks run (existing)
3. All services receive SIGTERM
4. After grace period, remaining services receive SIGKILL
5. Session-end hooks run (existing)

### Process identity

Services run with the same UID/GID as the main process (the `scion` user after `setupHostUser()`).

## Logging

Each service produces two log streams at well-known paths under the agent's home directory:

### Service output logs

Service stdout and stderr are captured to per-service log files:

```
~/.scion/services/logs/<name>.stdout.log
~/.scion/services/logs/<name>.stderr.log
```

These contain the raw output from the service process.

### Sciontool lifecycle logs

Sciontool writes its own per-service lifecycle log covering management events:

```
~/.scion/services/logs/<name>.lifecycle.log
```

Events recorded include:
- Service started (with PID, timestamp)
- Service exited (exit code, signal, timestamp)
- Restart attempted (attempt number, timestamp)
- Restart limit reached (after 3 consecutive failures)
- Ready check passed/failed
- Service stopped during shutdown

These lifecycle logs are also forwarded to the Hub logging endpoint via sciontool's existing OpenTelemetry logging integration, making service health visible in the hosted architecture without additional plumbing.

### Log access

When tmux is enabled, users can create a new tmux window and tail logs from these well-known paths. No special tmux integration is built for services.

## Implementation Surface

### New code

- **`pkg/sciontool/services/manager.go`** — `ServiceManager` struct
  - `Start(specs []ServiceSpec, uid, gid int) error` — starts all services in order, honoring ready checks
  - `Shutdown(ctx context.Context) error` — graceful stop with timeout
  - Background goroutine per service for restart monitoring (max 3 consecutive failures)
  - Per-service output logs written to `~/.scion/services/logs/<name>.{stdout,stderr}.log`
  - Per-service lifecycle logs written to `~/.scion/services/logs/<name>.lifecycle.log`

### Modified code

- **`pkg/api/types.go`** — Add `Services []ServiceSpec` to `ScionConfig`
- **`cmd/sciontool/commands/init.go`** — Wire ServiceManager into `runInit()`:
  1. After pre-start hooks, read `scion-services.yaml` from `$HOME/.scion/`
  2. Start ServiceManager
  3. Start main process via existing supervisor
  4. During shutdown, stop ServiceManager before session-end hooks
- **Agent creation path** (host-side) — Extract `services` block from `scion-agent.yaml` and write it as `scion-services.yaml` into the agent's home directory during provisioning

### Unchanged

- The existing supervisor remains responsible for the main harness process only
- Lifecycle hooks are unaffected
- `scion-agent.yaml` schema gains the `services` field but remains at the template root
