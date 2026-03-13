---
title: Permissions & Policy
description: Designing access control for Scion groves and agents.
---

Scion is implementing a robust, principal-based access control system to manage resources across distributed groves and teams. While currently in the design and early implementation phase, this document outlines the core concepts and policy model.

For a detailed technical specification of the policy language and agent identity claims, see the [Policy & Permissions Reference](../reference/permissions-policy/).

## Core Concepts

### Principals
A **Principal** is an identity that can be granted permissions.
- **Users**: Identified by their email address.
- **Groups**: Collections of users or other groups, allowing for hierarchical team structures.

### Resources
Permissions are granted on specific resource types:
- `hub`: The global Scion Hub instance.
- `grove`: A project-level workspace.
- `agent`: An individual agent instance.
- `template`: An agent configuration blueprint.

### Actions
Scion uses a standardized set of actions:
- **CRUD**: `create`, `read`, `update`, `delete`, `list`.
- **Administrative**: `manage`.
- **Resource-Specific**: `start`, `stop`, `attach`, `message`.

## Policy-Based Authorization

Scion enforces strict policy-based authorization for all agent operations:
- **Agent Creation**: Requires active membership in the target grove.
- **Agent Interaction**: Interacting with an agent (e.g., via PTY/terminal or structured messaging) is restricted to the agent's owner (the creator) or system administrators.
- **Agent Deletion**: Only the agent's owner or a system administrator can delete an agent.

Scion uses a **Hierarchical Override Model** for policies. Policies can be attached at three levels:

1.  **Hub Level**: Global policies applying to all resources.
2.  **Grove Level**: Policies applying to all resources within a specific grove.
3.  **Resource Level**: Policies applying to a single specific agent or template.

### Resolution Logic
When an action is attempted, Scion resolves effective permissions by traversing the hierarchy from the most specific to the most general:
- A policy at the **Resource level** overrides a policy at the **Grove level**.
- A policy at the **Grove level** overrides a global **Hub level** policy.

This model allows for granular delegation, where grove owners can manage their own team's access without global administrator intervention.

## Capability-Based Access Control

The Hub API and Web UI utilize a capability gating system. Resource responses from the API include `_capabilities` annotations. These annotations explicitly state the actions the authenticated user is permitted to perform on that specific resource. This ensures granular UI controls (e.g., disabling the "Delete" button if the user lacks permission) and provides a secondary layer of API-level enforcement.

## Policy Structure

A policy defines the rules for access:

```json
{
  "name": "Grove Developer Policy",
  "scopeType": "grove",
  "scopeId": "grove-uuid",
  "resourceType": "agent",
  "actions": ["create", "read", "start", "stop"],
  "effect": "allow"
}
```

- **Effect**: Can be `allow` or `deny`.
- **Conditions**: (Future) Optional rules based on resource labels or time-of-day.

## Roles

To simplify management, Scion provides built-in roles that bundle common permissions:

| Role | Description |
|------|-------------|
| `hub:admin` | Full control over the entire Hub. |
| `hub:member` | Standard user; can create their own groves. |
| `grove:admin` | Full control over a specific grove and its agents. |
| `grove:developer` | Can create and manage agents within a grove. |
| `grove:viewer` | Read-only access to grove status and logs. |

## Implementation Status

The permissions system features:
- **Identity Resolution**: Core identity and domain-based authorization.
- **Capability Gating**: UI and API enforcement via `_capabilities`.
- **Policy Enforcement**: Strict authorization for agent creation, interaction, and deletion based on grove membership and ownership.
- **Group & Policy Management**: Full support for group and policy schemas in the database, manageable via the Web Dashboard.

## Managing Users and Groups

The Scion Web Dashboard includes a centralized **Admin Management Suite** (accessible to users with administrative privileges) that provides dedicated views for access control management:

- **Server Configuration Editor**: A full-featured settings editor at `/admin/server-config`. This allows administrators to view and modify the global `settings.yaml` through the Web UI with support for tabbed navigation, sensitive field masking, and hot-reloading of key settings like log levels, telemetry defaults, and admin emails.
- **Users List**: View all authenticated users, search for specific accounts, track "Last Seen" timestamps, and manage their system-wide roles (e.g., granting `hub:admin` access).
- **Groups Management**: Create organizational groups and manage their membership. This enables policy-based authorization where permissions can be granted to an entire team at once.
- **Broker Visibility**: Comprehensive broker detail pages provide a grouped view of all active agents by their respective groves, helping administrators understand resource distribution.
- **Maintenance Mode**: Administrators can toggle maintenance mode for the Hub and Web servers directly from the UI to facilitate safe infrastructure updates.

By leveraging these administrative views, Platform Ops can efficiently map their organization's structure directly into Scion's Principal and Policy hierarchy.