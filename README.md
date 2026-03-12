# Scion

_sci·on /ˈsīən/ noun 1. a young shoot or twig of a plant, especially one cut for grafting or rooting._

Scion is an experimental multi-agent orchestration testbed designed to manage concurrent LLM-based agents running in containers across your local machine and remote clusters. It enables developers to run groups of specialized agents with isolated identities, credentials, and workspaces, allowing for a dynamic and evolving graph of parallel execution of tasks such as research, coding, auditing, and testing.

Scion takes a "less is more" approach to allowing modern powerful models to determine the execution of orchestration patterns by leveraging the progressive skill approach of dynamically loading the usage help text of the `scion` cli tool to manage other agents. This provides a system to rapidly experiment with different orchestration patterns and approaches through natural language prompting. See more in [philosophy](docs-site/src/content/docs/philosophy.md)

**NOTE** Currently this project is early and experimental. Most of the concepts are settled in, but many features may not be fully implemented, anything might break or change and the future is not set. Local use is relatively stable, Hub based workflows are ~80% verified, Kubernetes runtime support and remote-git Groves are early and have rough edges.

## Key Features

- **Parallelism**: Run multiple agents concurrently as full independent processes either locally or remote.
- **Isolation**: Each agent runs in its own container with strict separation of credentials, configuration, and environment. Scion uses `git worktree` to provide each agent with a dedicated workspace, preventing merge conflicts and ensuring clean separation of concerns.
- **Context Management**: Each agent has a dedicated context window, and optionally its own system instruction.
- **Runtimes**: Manage multiple execution environments (e.g., Local, Docker, Kubernetes) via named profiles. Distributed across local workstion, remote VMs, and container clusters.
- **Specialization**: Agents can be customized via [Templates](docs-site/src/content/docs/advanced-local/templates.md) (e.g., "Security Auditor", "QA Tester") to perform specific roles.
- **Interactivity**: Agents run in `tmux` sessions by default, allowing for "detached" background operation, enqueuing messages to running agents, and "attaching" for human-in-the-loop interaction. Attach to running agents across automatically established network tunnels for secure remote control.
- **Harness Agnostic**: Works with Gemini CLI, Claude Code, OpenCode, and Codex. Easily adaptable to any harness which can run in a container.
- **Observability**: Supports normalized OTEL telemetry across harnesses for logging and metrics allowing easy aggregation across agent swarms.

## Documentation

Visit our **[Documentation Site](docs-site/src/content/docs/)** for comprehensive guides and reference.

- **[Overview](docs-site/src/content/docs/overview.md)**: Introduction to Scion.
- **[Installation](docs-site/src/content/docs/getting-started/install.md)**: How to get Scion up and running.
- **[Concepts](docs-site/src/content/docs/concepts.md)**: Understanding Agents, Groves, Harnesses, and Runtimes.
- **[CLI Reference](docs-site/src/content/docs/reference/cli.md)**: Comprehensive guide to all Scion commands.
- **Guides**:
    - [Using Templates](docs-site/src/content/docs/advanced-local/templates.md)
    - [Using Tmux](docs-site/src/content/docs/advanced-local/tmux.md)
    - [Kubernetes Runtime](docs-site/src/content/docs/hub-admin/kubernetes.md)

## Installation

See the **[Installation Guide](docs-site/src/content/docs/getting-started/install.md)** for detailed instructions.

Quick start from source:
```bash
go install github.com/GoogleCloudPlatform/scion/cmd/scion@latest
```

## Quick Start

### 1. Initialize a Grove

Navigate to your project root and initialize a new Scion grove. This creates the `.scion` directory and seeds default templates.

```bash
cd my-project
scion init
```

Note: If you are in a git repository, it is recommended to add `.scion/agents` to your `.gitignore` to avoid issues with nested git worktrees:
```bash
echo ".scion/agents" >> .gitignore
```

Note: Scion automatically detects your operating system and configures the default runtime (Docker for Linux/Windows, Container for macOS). You can change this in `.scion/settings.json`.

### 2. Start Agents

You can launch an agent immediately using `start` (or its alias `run`). By default, this runs in the background using the `gemini` template.

```bash
# Start a gemini agent named "coder"
scion start coder "Refactor the authentication middleware in pkg/auth"

# Start a Claude-based agent
scion run auditor "Audit the user input validation" --type claude

# Start and immediately attach to the session
scion start debug "Help me debug this error" --attach
```

### 3. Manage Agents

- **List active agents**: `scion list` (alias `ps`)
- **Attach to an agent**: `scion attach <agent-name>`
- **Send a message**: `scion message <agent-name> "New task..."` (alias `msg`)
- **View logs**: `scion logs <agent-name>`
- **Stop an agent**: `scion stop <agent-name>`
- **Resume an agent**: `scion resume <agent-name>`
- **Delete an agent**: `scion delete <agent-name>` (removes container, directory, and worktree)

## Configuration

Scion settings are managed in `settings.json` files, following a precedence order: **Grove** (`.scion/settings.json`) > **Global** (`~/.scion/settings.json`) > **Defaults**.

Profiles allow you to switch runtimes and configurations easily (e.g. `scion --profile remote start ...`).

Templates serve as blueprints and can be managed via the `templates` subcommand. See the [Templates Guide](docs-site/src/content/docs/advanced-local/templates.md) for more details.

## Disclaimers

This is not an officially supported Google product. This project is not eligible for the [Google Open Source Software Vulnerability Rewards Program](https://bughunters.google.com/open-source-security)

## License

This project is licensed under the Apache License, Version 2.0. See the [LICENSE](LICENSE) file for details.
