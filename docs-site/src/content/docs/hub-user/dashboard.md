---
title: Web Dashboard
description: Using the Scion Web Dashboard for visualization and control.
---

The Scion Web Dashboard provides a visual interface for managing your agents, groves, and runtime brokers. It complements the CLI by providing real-time status updates and easier management of complex environments.

## Overview

The dashboard is organized into several key areas:

### Dashboard Home
The landing page provides an overview of your active agents across all groves and the status of your runtime brokers.

### Notifications & Alerts
The dashboard features an integrated notification framework with real-time SSE delivery. 
- **Notification Tray**: Provides agent-scoped filtering for status events, accessible directly from the top navigation.
- **Browser Push Notifications**: Opt-in native browser push notifications ensure you receive alerts even when the dashboard is in the background. Default triggers include `stalled` and `error` states, as well as requests for user input.

### Groves
View and manage your registered groves.
- **Create/Register Grove**: Create a Hub-Native workspace directly on the Hub, or connect a new remote Git repository.
- **Grove Settings**: Centralized configuration interface for managing grove-scoped environment variables and secrets, including "Injection Mode" controls (Always vs. As-Needed).
- **Workspace Management**: Download individual workspace files or generate ZIP archives of entire groves directly from the UI.
- **Agent List**: See all agents belonging to the grove.

### Agents
Detailed view for individual agents, featuring a high-density tabbed layout and improved breadcrumb navigation with a dedicated back button.
- **Advanced Agent Creation**: A comprehensive form for Just-In-Time (JIT) configuration, allowing granular control over models, resource limits (`max_turns`, `max_duration`), and harness settings at creation time. It features a native **Runtime Profile Selector** that dynamically populates available profiles based on the selected broker, and **Custom Branch Targeting**, which allows users to direct agents to clone and check out specific git branches immediately upon creation.
- **Status Tab**: Real-time view of agent lifecycle (Starting, Thinking, Waiting, etc.). Includes **stalled agent detection** to flag agents that have stopped responding (setting their activity status to `offline`).
- **Logs Tab**: Streamed logs from the agent container via the integrated Cloud Log Viewer.
- **Messages Tab**: A dedicated tab for viewing structured messages sent to and from the agent.
- **Configuration Tab**: Dedicated tab for viewing the applied configuration of the agent, featuring a new telemetry configuration card.
- **Debug Panel**: A full-height panel providing a real-time stream of SSE events and internal state transitions for advanced troubleshooting and observability.
- **Terminal**: Interactive terminal access to the agent's workspace, featuring built-in Tmux support with a mouse toggle (`C-b m`) and a dedicated terminal toolbar.
- **Lifecycle Control**: Start, stop, restart, or delete agents from the UI. Includes bulk operations like the "Stop All" button for efficient bulk shutdown of all agents within a grove.

### Runtime Brokers
Monitor the infrastructure nodes where your agents are executing.
- **Status**: See which brokers are online and their current load.
- **Configuration**: View broker capabilities (Docker, K8s, etc.).

### Admin Management Suite
Centralized views for managing the Scion infrastructure and access control (available to administrative users).
- **Users**: View and manage user accounts and roles.
- **Groups**: Create and manage organizational groups for policy-based authorization.
- **Brokers**: Comprehensive broker detail pages providing a grouped view of all active agents by their respective groves.
- **Maintenance Mode**: Toggle maintenance mode for the Hub and Web servers to facilitate safe infrastructure updates.

## Authentication

The dashboard supports several authentication methods:
- **OAuth (Google/GitHub)**: For standard user access.
- **Development Auto-login**: For local development.

See the [Authentication Guide](../hub-admin/auth/) for setup instructions.

## API Proxying
The Go server handles API proxying, token injection, and session management so the browser never handles raw API keys or long-lived tokens directly.
