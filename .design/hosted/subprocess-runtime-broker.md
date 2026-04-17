# Subprocess RuntimeBroker Under `scion broker`

## Summary

Implement the specialized broker as a new mode of the existing `broker` command group, selected explicitly at startup with `scion broker start --runtime subprocess`. The broker remains Hub-protocol compatible, is manually started, uses localhost Hub/control-channel endpoints supplied through SSH tunnels, refreshes the current built-in harness material locally, and preserves operational parity for start, stop, list, logs, message, and attach.

## Implementation

- Extend `scion broker start` and `scion broker restart` with an explicit runtime selection flag.
- Thread broker runtime selection through `server start` so the existing runtime broker server starts with a non-container runtime.
- Add a `subprocess` implementation of `runtime.Runtime` that launches harnesses in host `tmux` sessions and persists broker-owned state.
- Keep the existing `pkg/runtimebroker` server, `agent.Manager`, template hydration, and `StartOptions` flow intact.
- Use the existing global embedded template and harness-config refresh path as the bundled built-in harness source.
- Skip container-runtime probing during fresh global initialization when starting a subprocess broker on a host that does not have Docker or Podman installed.

## Runtime Behavior

- `Run` provisions the agent as today, then launches the requested harness as a host subprocess in a dedicated tmux session.
- `List` synthesizes `api.AgentInfo` from persisted subprocess state plus live tmux status.
- `Stop` sends the harness interrupt key first, then kills the tmux session if needed.
- `Delete` removes tmux state and runtime metadata.
- `Exec` rewrites tmux targets from the generic in-agent `scion` session name to the real per-agent host tmux session.
- `Attach` and broker PTY streaming attach directly to the host tmux session.
- `GetLogs` returns the broker-captured harness log file.

## Constraints

- Harness CLIs are assumed to be installed and authenticated on the host already.
- `tmux` is required and startup should fail fast if it is missing.
- Container-only runtime settings remain unsupported in subprocess mode.
- SSH tunnel setup remains out-of-band; the broker only consumes the localhost endpoints it is given.
