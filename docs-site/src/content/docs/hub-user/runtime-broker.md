---
title: Runtime Broker
description: How a Hub user can register their local machine as a compute resource for your team's Scion Hub.
---

A **Runtime Broker** is the component of Scion that actually runs agents. While a centralized **Scion Hub** manages metadata and agent configurations, you can register your own machine as a Runtime Broker to execute agents locally while still participating in your team's Hub environment.

This is especially useful if you need agents to access local resources (like an intranet database, local files, or specialized hardware) or if you want to contribute compute power to your team's projects.

## Architecture

When you run a Runtime Broker connected to a Hub, your machine establishes a persistent WebSocket connection (a "Control Channel") to the Hub.

```d2
direction: right
You -> Hub: "Start Agent on My Machine"
Hub -> Your Machine (Broker): "CreateAgent (via WS Tunnel)"
Your Machine (Broker) -> Runtime: "Run Agent"
Agent -> Hub: "Status: RUNNING"
```

The Hub acts as the control plane, but the actual execution (and the git worktrees) stay on your machine.

## Registering Your Machine

To allow the Hub to dispatch agents to your machine, you must start a Runtime Broker and register it.

### 1. Start the Broker

You can start a standalone broker process in the background:

```bash
scion broker start
```

*(Alternatively, if you run `scion server start --workstation`, a broker is automatically started alongside a local workstation server.)*

### Subprocess Mode For SSH-Reachable Hosts

If your broker is running inside an already-provisioned container or VM that is only reachable over SSH, you can run agents as local host subprocesses instead of containers:

```bash
scion broker start --runtime subprocess
```

Use this mode when:

- The execution environment already exists and you do not want Scion to provision per-agent containers.
- SSH tunnels expose the Hub and control-plane endpoints on `127.0.0.1` inside that environment.
- The required harness CLIs are already installed and authenticated on that host.

Prerequisites for subprocess mode:

- `tmux` is installed on the host running the broker.
- The harness CLI you plan to use (`codex`, `claude`, `gemini`, `opencode`) is installed and already authenticated.
- Any SSH tunnels needed for Hub connectivity are created out-of-band before starting the broker.

In subprocess mode, the broker bundles the built-in harness templates from the current Scion binary and launches each agent as a tmux-backed subprocess on the host. The Hub registration and provider flow stays the same.

### 2. Link to the Hub

Before the broker can receive commands, it must be registered with the Hub you are connected to. This establishes a secure trust relationship.

```bash
scion broker register
```

This command will securely exchange credentials with the Hub, linking your machine's broker to your Hub user account.

### 3. Provide Compute for a Grove

Even after registration, your broker will not accept arbitrary agents. It only executes agents for specific **Groves** (projects) that you explicitly authorize it to serve.

Navigate to the directory of a project that is connected to the Hub, and run:

```bash
scion broker provide
```

This tells the Hub: *"My local broker is now a provider for this specific Grove."* When anyone on your team starts an agent in this Grove and targets your broker, the agent will execute on your machine.

To verify which groves your broker is currently serving:

```bash
scion broker status
```

## Using The Subprocess Runtime

Once the broker is registered and providing for a grove, Hub-dispatched agents targeted at that broker run directly on the host:

- Each agent gets its own workspace and broker-managed state on disk.
- The harness runs in a dedicated tmux session so `attach`, `message`, and log streaming continue to work.
- Built-in harness configurations are sourced from the broker binary, so the broker does not need Hub template hydration for the bundled harness set.

Typical operator flow:

```bash
# 1. Ensure tunnels exist and localhost endpoints are reachable

# 2. Start the broker in subprocess mode
scion broker start --runtime subprocess

# 3. Register it with the connected Hub
scion broker register

# 4. Authorize it for the current grove
scion broker provide
```

After that, create agents from the Hub or with `scion start --broker <broker-id>` as usual. The difference is only the execution backend used by that broker.

## Security & Isolation

When you register your machine as a broker:
- **Execution isolation depends on runtime**: The default broker runtime runs each agent in its own container. The `subprocess` runtime runs each agent as a host subprocess in its own tmux session and worktree instead.
- **No Source Code Sharing**: The Hub does not store your source code. The broker simply creates local branches and commits.
- **Safe Secrets**: Sensitive API keys and environment variables managed in the Hub are injected into the agent process at runtime. They are not saved to your local disk by the Hub.
- **Mutual Authentication**: All communication over the Control Channel uses HMAC-SHA256 signatures, ensuring that only the authorized Hub can send commands to your machine.

Because subprocess mode runs directly on the host, it should only be used in environments you already trust and isolate operationally, such as a dedicated remote container or VM that is provisioned specifically for broker workloads.

## Stopping the Broker

If you want to stop accepting agent workloads from the Hub, you can simply stop the broker daemon:

```bash
scion broker stop
```

Agents that are currently running on your machine may be interrupted or left orphaned depending on their state.
