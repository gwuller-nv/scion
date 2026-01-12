# Supported Agent Harnesses

Scion supports multiple LLM agent "harnesses". A harness is an adapter that allows Scion to manage the lifecycle, authentication, and configuration of a specific agent tool.

## 1. Gemini CLI (`gemini`)

The default harness for interacting with Google's Gemini models via the `gemini` CLI tool.

### Authentication
The Gemini harness supports multiple authentication methods:
- **API Key**: Set `GEMINI_API_KEY` in your environment or Scion settings.
- **OAuth (Personal)**: Uses `oauth_creds.json` if available.
- **Google Cloud (Vertex AI)**: Uses Application Default Credentials (ADC) via `gcloud`.

### Configuration
- **Settings File**: `~/.gemini/settings.json` (inside the agent container).
- **System Prompt**: `~/.gemini/system_prompt.md` can be used to set a custom system prompt.

### Known Limitations
- The `gemini` CLI tool must be installed in the container image (included in default images).
- Some advanced Vertex AI configurations might require specific environment variables like `GOOGLE_CLOUD_LOCATION`.

---

## 2. Claude Code (`claude`)

A harness for Anthropic's "Claude Code" agent.

### Authentication
- **API Key**: Set `ANTHROPIC_API_KEY` in your host environment. Scion propagates this to the agent.
- **Manual Login**: You can run `claude login` inside the agent, which generates a `~/.claude.json` file. Scion will preserve this file across sessions.

### Configuration
- **Config File**: `~/.claude.json`. Scion manages project-specific settings in this file to ensure the agent respects the workspace boundaries.
- **Projects**: Scion automatically configures the current workspace as a project in `.claude.json`.

### Known Limitations
- Claude Code is a beta tool and its configuration format may change.
- Requires `ANTHROPIC_API_KEY` to be set or manual login.

---

## 3. OpenCode (`opencode`) [Experimental]

The OpenCode TUI.

### Authentication
- **Auth File**: OpenCode uses an `auth.json` file located at `~/.local/share/opencode/auth.json`.
- **Propagation**: If you have `~/.local/share/opencode/auth.json` on your host machine, Scion copies it to the agent's home directory upon creation.

### Configuration
- **Config File**: `~/.config/opencode/opencode.json`.
- **Environment**: Respects standard OpenCode environment variables.

### Known Limitations
- **Auth File Copy**: The `auth.json` file is copied only when the agent is **created**. If you update your host credentials, you may need to manually update the file in the agent or recreate the agent.
- **No Hook support** opencode does not have analogous hook support, and so will require use of plugin system to notify the scion orchestrator.

---

## 4. Codex (`codex`)

A harness for the OpenAI Codex CLI.

### Authentication
- **API Keys**: Respects `OPENAI_API_KEY` or `CODEX_API_KEY` environment variables.
- **Auth File**: Codex uses an `auth.json` file located at `~/.codex/auth.json`.
- **Propagation**: If you have `~/.codex/auth.json` on your host machine, Scion copies it to the agent's home directory upon creation.

### Configuration
- **Config File**: `~/.codex/config.toml`.
- **Default Flags**: Runs with `--yolo` enabled by default.
- **Resume Support**: Automatically uses the `resume` positional argument to continue existing sessions.

### Known Limitations
- **Auth File Copy**: The `auth.json` file is only copied when the agent is **created**.
- **Model selection**: Specific model selection must currently be handled via the `config.toml` or environment variables within the agent.

---

## Feature Capability Matrix

The following table summarizes the capabilities supported by each agent harness within Scion.

| Capability | Gemini | Claude | OpenCode | Codex |
| :--- | :---: | :---: | :---: | :---: |
| **Resume** | ✅ | ✅ | ✅ | ✅ |
| With Prompt | ✅ | ✅ | ✅ | ❌ |
| Custom Session ID | ❌ | ✅ | ❌ | ❌ |
| **Interject** | ✅ | ✅ | ✅ | ✅ |
| Interupt Key | C-c | C-c | Esc / C-c | C-c |
| **Enqueue** | ✅ | ✅ | ✅ | ✅ |
| **Hooks** | ✅ | ✅ | ❌ | ❌ |
| Support | ✅ | ✅ | ❌ | ❌ |
| **OpenTelemetry** | ✅ | ✅  | ❌ | ✅  |

* **Resume with Prompt**: Ability to provide a new task/prompt when resuming an existing session.
* **Interject** (pending feature): Key used to interrupt the agent (e.g., stop generation).
* **Enqueue** (pending feature): Ability to send messages to the agent while it's running (requires Tmux).
* **Hooks**: Support for lifecycle hooks (e.g., `SessionStart`, `AfterTool`).
* **OpenTelemetry** (pending feature): Specific events vary