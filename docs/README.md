# Plous documentation

Plous is a terminal workspace for infrastructure and deployment tools. Keep related tool connections, configuration history, and AI conversations together in a workspace.

## Start here

1. [Install Plous](installation.md).
2. [Create your first workspace](getting-started.md). No cluster or AI account is required.
3. [Choose a tool integration](integrations.md) for the work you want to do.

## Using Plous

The core structure is **Workspace → Tool → Connection**. A workspace can hold multiple tools, contexts, and repositories. For example, one project might group a Terraform directory, a Kustomize overlay, and two Kubernetes contexts.

| Guide | What it covers |
| --- | --- |
| [Workspaces and connections](workspaces.md) | Group tools, add connections, and manage settings. |
| [Interface and navigation](navigation.md) | Move between panels, search, and use commands. |
| [Activity and history](activity.md) | Read configuration changes and compare saved revisions. |
| [AI assistant](ai-chat.md) | Discuss your work, retrieve prior conversations, and investigate supported sources. |

**Overview** shows saved configuration. **Explore** opens the selected tool connection. **Activity** shows workspace changes. The assistant stays alongside these views.

## What this version supports

Plous **0.0.1** is an early development build, not ready to ship.

Workspaces, configuration history, and AI conversations are implemented. Kubernetes has live browsing and resource operations. Helm, Kustomize, Terraform, Argo, and Kargo currently save connection settings. The [integration guide](integrations.md) distinguishes those capabilities.

Remote workspaces, sharing, deployment pipelines, and assistant write operations are not available.

## Find a task

- [Configure Plous](configuration.md) or [look up a shortcut](keymap.md).
- [Check data storage and privacy](data-and-privacy.md) or [fix a problem](troubleshooting.md).
- [Contribute](contributing.md), [run development checks](development.md), or [maintain these docs](documentation.md).

## Reading the instructions

**Bold text** names a key or on-screen control. `:s` means type a colon, type `s`, then press **Enter** inside Plous. `11` means press **1** twice, with no Enter. Shell commands appear in code blocks.

Press **hh** while navigating for in-app Help. See the [project README](../README.md) for project information.
