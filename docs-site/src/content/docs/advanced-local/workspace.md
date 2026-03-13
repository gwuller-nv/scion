---
title: About Workspaces
---

Every Scion agent has a dedicated **Workspace**, mounted at `/workspace` inside the agent's container. This is where the agent reads code, makes changes, and runs commands.

Scion provides flexible options for how this workspace is backed on your host machine, ranging from isolated git worktrees to direct directory mounts.

## Workspace Resolution

When you start an agent, Scion determines its workspace based on the following precedence:

1.  **Explicit Workspace** (`--workspace` flag):
    If you provide a path via `--workspace`, Scion mounts that directory directly. This works in both Git and non-Git environments.

2.  **Git Worktree** (Git repositories):
    If you are in a Git repository and do not provide an explicit workspace, Scion uses [Git Worktrees](https://git-scm.com/docs/git-worktree) to give the agent its own isolated working directory and branch.

3.  **Project Root / CWD** (Non-Git environments):
    If you are not in a Git repository, Scion mounts the project root (or current directory for global agents) directly.

---

## 1. Explicit Workspaces (`--workspace`)

You can tell Scion exactly which directory to use as the workspace. This is useful for:
- Working on a specific subfolder.
- Using a shared directory across multiple agents.
- Working on a path outside the current repository without creating a worktree.

```bash
# Mount a specific directory
scion start my-agent "fix bugs" --workspace ./my-service
```

- **Behavior**: The specified directory is mounted directly to `/workspace`.
- **Isolation**: **None**. Changes made by the agent are immediately visible on the host and to any other agents sharing this directory.
- **Git**: No new worktree or branch is created, even if inside a repo.

---

## 2. Git Worktrees (Automatic Isolation)

When working inside a Git repository without an explicit `--workspace`, Scion automatically manages **Git Worktrees**. This ensures that each agent has its own isolated checkout of the code, allowing them to work on different branches simultaneously without interfering with your main working directory.

### Prerequisites
- Git **2.47.0** or newer is required (for relative path support).

### Branch Resolution
Scion determines which branch to check out in the worktree:

1.  **Explicit Branch** (`--branch`, `-b`):
    ```bash
    scion start my-agent -b feature/login "add logging"
    ```
    - If the branch exists and has a worktree, Scion **reuses the existing worktree** (see below).
    - If the branch exists but has no worktree, Scion creates a new worktree for it.
    - If the branch doesn't exist, Scion creates it (based on current HEAD) and a worktree.

2.  **Agent Name Matching**:
    If you don't specify a branch, Scion checks if a branch named after the agent exists (e.g., `my-agent`).
    - **Match Found**: It behaves exactly as if you passed `-b my-agent`.
    - **No Match**: Scion creates a new branch named `my-agent` and a corresponding worktree.

### Reusing Existing Worktrees
If you request a branch that is already checked out in another worktree (e.g., by another agent or manually created), Scion detects this.
- Instead of failing or creating a conflict, Scion **mounts the existing worktree path**.
- A warning is displayed: `Warning: Relying on existing worktree for branch '...'`.
- This allows multiple agents to collaborate on the same branch/worktree if desired.

---

## 3. Non-Git Environments

In non-git projects (where no `.git` directory is found):
- Scion defaults to mounting the **project root** (the directory containing `.scion`).
- For global agents, it defaults to the **current working directory**.
- All agents share the same files. There is no isolation or branching.

---

## 4. Hub-First Remote Workspaces

When working with a Scion Hub, you can create remote workspaces that decouple agent execution from your local machine. The Hub supports two types of remote workspaces:

### Hub-Native Groves
Hub-Native groves allow you to create project workspaces directly through the Hub API and Web Dashboard **without an external Git repository**. 
- The Hub automatically initializes a seeded `.scion` structure.
- Workspace files are managed locally by the Hub and its distributed runtime brokers.
- You can directly download individual workspace files or generate ZIP archives of entire groves using the Hub API or Web Dashboard, making it easy to export your data.

### Git Groves
Groves can also be created directly from a remote git repository URL. In this mode, the agent's container **clones the repository at startup**.

```bash
# Create a git grove from a URL (Hub-managed)
scion hub grove create https://github.com/org/repo.git
```

#### How Git Groves Work

1. The Hub stores the git remote URL and default branch as grove metadata.
2. When an agent starts, the Runtime Broker injects `SCION_GIT_CLONE_URL`, `SCION_GIT_BRANCH`, and `SCION_GIT_DEPTH` as environment variables.
3. The `sciontool init` process inside the container clones the repo into `/workspace` before the harness starts.
4. A feature branch `scion/<agent-name>` is created and checked out automatically.

#### Agent Branch Strategy

Each agent gets its own branch named `scion/<agent-name>`. This prevents conflicts when multiple agents work on the same repository concurrently.

#### Shallow Clones

By default, git groves use a shallow clone with `depth=1` for fast startup. If an agent needs full history (e.g., for `git log` or `git blame`), it can fetch the rest:

```bash
git fetch --unshallow
```

#### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SCION_GIT_CLONE_URL` | HTTPS URL of the repository to clone | *(required)* |
| `SCION_GIT_BRANCH` | Branch to clone | `main` |
| `SCION_GIT_DEPTH` | Clone depth | `1` |

Authentication is handled via the `GITHUB_TOKEN` environment variable, which is injected from the grove's secrets.

---

## 5. Grove Shared Directories

Grove Shared Directories provide a persistent, mutable storage layer that can be shared between multiple agents within a single grove. This is ideal for sharing build artifacts, shared caches, or state files without relying on version control or the Hub database.

### Managing Shared Directories

You can manage shared directories using the `scion shared-dir` CLI commands:

```bash
# Create a new shared directory
scion shared-dir create <name>

# List shared directories in the current grove
scion shared-dir list

# View details about a specific shared directory
scion shared-dir info <name>

# Remove a shared directory (permanently deletes contents)
scion shared-dir remove <name>
```

### Mounting Shared Directories

When an agent is created in a grove that has shared directories, they are automatically mounted into the agent's container. 

By default, they are available at two locations within the agent:
- **Standard Path:** `/scion-volumes/<name>`
- **Workspace Path:** `/workspace/.scion-volumes/<name>`

### Storage Backends
- **Local Workstations:** Backed by directories on the host filesystem.
- **Kubernetes:** Backed by PersistentVolumeClaims (PVCs) with grove-scoped lifecycle management, ensuring data persists across pod restarts and can be accessed by any agent in the grove.

---

## The `cdw` Command

Scion provides a helper command, `cdw` (Change Directory to Worktree), to quickly navigate to an agent's workspace on your host.

```bash
scion cdw <agent-name>
```

- Spawns a new shell inside the agent's workspace directory.
- Works for both managed worktrees and manual mounts (if resolvable).

Will also take a branch/worktree name outside of scion agents, most useful for getting back to main.

```bash
scion cdw <agent-name>
```

## Cleanup

When you delete an agent:
```bash
scion delete <agent-name>
```
- **Worktrees**: The worktree directory is removed and git metadata is pruned.
- **Branches**: By default, the branch is deleted. Use `--preserve-branch` (or `-b`) to keep it.
- **Explicit Workspaces**: Directories mounted via `--workspace` are **NOT** deleted. Scion only cleans up resources it created.
