---
title: Working with Templates & Harnesses
---

Scion separates the **role** of an agent (what it does) from its **execution mechanics** (how it runs). This is achieved through two complementary concepts: **Templates** and **Harness-Configs**.

## Core Concepts

### 1. Templates (The "Role")
A template defines the agent's purpose, personality, and instructions. It is **harness-agnostic**, meaning a "Code Reviewer" template can theoretically run on Claude, Gemini, or any other LLM.

A template typically contains:
- `scion-agent.yaml`: The agent definition (resources, env vars).
- `agents.md`: Operational instructions for the agent.
- `system-prompt.md`: The core persona and role definition.
- `home/`: Optional portable configuration files (e.g., linter configs).

### 2. Harness-Configs (The "Mechanics")
A harness-config defines the runtime environment and tool-specific settings. It includes the base files required by the underlying tool (e.g., `.claude.json` for Claude, `.gemini/settings.json` for Gemini).

Harness-configs live in `~/.scion/harness-configs/` and contain:
- `config.yaml`: Runtime parameters (container image, model, auth).
- `home/`: Base files that are copied to the agent's home directory.

### 3. Composition
When you create an agent, Scion composes the final environment by layering:
1.  **Harness-Config Base Layer**: The foundation (e.g., Gemini CLI settings).
2.  **Template Overlay**: The role definition (prompts, instructions).
3.  **Profile/Runtime Overrides**: User-specific tweaks.

## Creating an Agent

To create an agent, you specify both a template and a harness-config.

```bash
# Explicitly specify both
scion create my-agent --template code-reviewer --harness-config gemini

# Use the template's default harness-config (if defined)
scion create my-agent --template code-reviewer

# Use the system default harness-config (from settings.yaml)
scion create my-agent --template code-reviewer
```

### Resolution Order
Scion determines which harness-config to use in this order:
1.  CLI flag: `--harness-config`
2.  Template default: `default_harness_config` in `scion-agent.yaml`
3.  System default: `default_harness_config` in global `settings.yaml`

## Managing Templates

### Template Bootstrapping

Local agent templates are automatically bootstrapped into the Hub database during server startup. This ensures that all defined templates are consistently available across the system, allowing for seamless deployment without manual importing steps.

### Template Traceability

To provide clear visibility into the exact configuration version associated with each agent, Scion captures and displays template IDs and hashes. When you view an agent's details in the CLI (`scion list`, `scion info`) or the Web UI, you can see the precise template version that was used to provision it.

### Structure of a Template
A typical template directory looks like this:

```text
my-template/
├── scion-agent.yaml      # Configuration
├── agents.md             # Instructions
├── system-prompt.md      # Persona
└── home/                 # Portable files (optional)
    └── .config/
        └── my-tool.conf
```

**`scion-agent.yaml` Example:**

```yaml
schema_version: "1"
name: code-reviewer
description: "Thorough code review agent"

agent_instructions: agents.md
system_prompt: system-prompt.md
default_harness_config: gemini

env:
  REVIEW_STRICTNESS: high

resources:
  requests:
    cpu: "500m"
    memory: "512Mi"
```

### Template Commands

```bash
# List available templates
scion templates list

# Create a new template
scion templates create my-new-role

# Clone an existing template
scion templates clone code-reviewer my-custom-reviewer

# Import definitions from Claude or Gemini sub-agents
scion templates import .claude/agents/code-reviewer.md

# Delete a template
scion templates delete my-old-template
```

## Managing Harness-Configs

Harness-configs are directories stored in `~/.scion/harness-configs/` (global) or `.scion/harness-configs/` (project-level).

### Customizing a Harness-Config
To change the default model or add custom hooks for a specific harness, edit the files directly in the harness-config directory.

**Example: Changing the Gemini model**
Edit `~/.scion/harness-configs/gemini/config.yaml`:

```yaml
harness: gemini
model: gemini-1.5-pro
# ...
```

**Example: Adding a persistent CLI flag**
Edit `~/.scion/harness-configs/gemini/home/.gemini/settings.json`.

### Creating Variants
You can create custom variants of harness-configs by copying the directory.

```bash
cp -r ~/.scion/harness-configs/gemini ~/.scion/harness-configs/gemini-experimental
```

Now you can use this variant:
```bash
scion create test-agent --harness-config gemini-experimental
```

### Resetting Defaults
If you mess up a harness-config, you can restore the factory defaults:

```bash
scion harness-config reset gemini
```
