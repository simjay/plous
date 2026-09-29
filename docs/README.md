# Plous documentation

This guide describes the working workspace layer in **Plous 0.0.1**. Start with the [keymap](keymap.md) for keyboard controls or the [project README](../README.md) to build and run Plous.

Contributors can use the [development and testing guide](development.md) for the repository layout and test commands.

## Create a workspace

1. On the first launch, press **Enter** in the main panel.
2. Enter a workspace name. Press **Enter** to select **Save**, then **Enter** again to create it.
3. Choose **Add tool**, or choose **Back** to start with an empty workspace.

To create another workspace, select **+ New workspace** in the left tree, or run `:new` (`:n`) in the main panel.

## Add Kubernetes contexts

1. Open **Workspace settings** from the left tree, or run `:settings` (`:s`) in the main panel.
2. Choose **Add tool → Kubernetes**.
3. Leave the kubeconfig path blank to use `KUBECONFIG`, falling back to `~/.kube/config`. You can also enter a specific file path.
4. Select **Find contexts**. Use **Up / Down** to move and **Space** to select one or more contexts.
5. Press **Enter** to add the selected contexts. If none are selected, Plous adds the highlighted context.

Discovery reads local context names and references without contacting a cluster, changing your current context, or running authentication plugins. Plous does not copy kubeconfig credentials into its saved configuration.

## Add or edit other tools

**Add tool** also offers Helm, Kustomize, Terraform, Argo, and Kargo. Each form asks for relevant references, such as a kubecontext, working directory, or Git repository. These entries save configuration; Plous does not yet run the tools or verify live connectivity.

Use **Workspace settings** to rename a workspace or edit any connection. To remove a connection, open its edit form and select **Remove connection**, then confirm. This removes the saved Plous entry without changing the external tool or its resources.

Form edits survive switching panels. Select **Save** to persist them; **Esc** cancels them. Finish or cancel an open form before selecting another workspace or connection in the tree.

## Explore the interface

The **left tree** lists all workspaces and groups their connections by tool. Expanding a group only changes the tree. Press **Enter** on a workspace, tool, or connection to select it. Selecting a different tool preserves the current main view.

The **main panel** has four tabs across the top, with each name on the left and its command on the right. In narrow terminals, names shorten while commands remain visible. With this panel focused and no form open, typing `:` opens a bordered command box at the bottom, below the current content. Press **Enter** to run the command or **Esc** to cancel it. Every command accepts a full name and a short alias; the four tabs also accept `:1`–`:4`.

| Tab | Current behavior |
| --- | --- |
| Overview | Summarizes saved workspace and tool configuration. |
| Resources | Shows the selected connection's configuration. Live resource browsing is not available yet. |
| Deployment | Shows an empty state. Deployment creation and execution are not available yet. |
| Activity | Lists workspace configuration changes, newest first. Press Enter on a change to inspect its before/after values. |

The **AI assistant** panel has a draft box at the bottom. Each workspace keeps its own draft. Drafts save locally as you type and when you exit normally. AI is not connected; Enter inserts a newline while editing and nothing is sent.

A single footer shows contextual controls or a status message. Controls depend on the focused panel. Read the [keymap](keymap.md) for mode commands, forms, and text editing.

## Saved data

By default, all persistent Plous data lives under `~/.plous/`. Workspace configuration changes have chronological records. Renaming a workspace or connection preserves its history. The session also remembers navigation state and chat drafts.

Use `plous --data-dir PATH` for a separate profile or a disposable test directory. Only one Plous process can use a data directory at a time. Press **Ctrl+C** from any panel, or use `:quit` / `:q` from main-panel navigation, to save the session and chat drafts, exit, and restore the terminal. Unsaved form edits are not applied on exit; use **Esc** to cancel a form or command and stay in Plous.
