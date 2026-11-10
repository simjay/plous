# Learn the interface

[Documentation](../README.md) · [Keymap](../reference/keymap.md)

Plous has three panels. The blue border shows which panel receives your keys.

| Panel | Contents |
| --- | --- |
| Left | Current Workspace, saved workspaces, connections, and Settings |
| Main | Overview, Explore, or Activity |
| Right | Plous chat, Claude Code, or Codex |

Use **Alt+Left / Alt+Right** to move between panels. These keys also work while typing or using an agent terminal.

Click a panel to focus it. Click a data row to select it and double-click to open it. Drag over text to select it, then use **Copy selection** in the footer. See [mouse controls](../reference/keymap.md#mouse-controls) for details.

The footer shows controls for the current view. **n Namespace** means press `n`. **hh Help** means press `h` twice.

## Understand the workspace tree

**Current Workspace** uses the cluster selected from your terminal kubeconfig. **Workspaces** contains connections you have saved. Both use the same tree:

- A workspace contains Monitor connections.
- A cluster connection contains Kubernetes and any detected Helm, Argo CD, or Flux tools.
- A tool contains resource groups and lists.

| Key | Tree action |
| --- | --- |
| Enter | Select a workspace or connection and toggle its branch. Open a resource list. |
| Right | Expand a branch |
| Left | Collapse a branch, or move to its parent |
| `/` | Search names, including items in collapsed branches |
| `g` | Refresh discovery for the selected cluster or tool |

Kubernetes is always available. Helm appears when Plous finds release metadata. Argo CD and Flux appear when their APIs are available.

**Tool discovery incomplete** means a check failed or access was denied. It does not confirm that the tool is absent.

Use [Workspaces and connections](workspaces.md) to save clusters and organize them.

## Choose a main view

| View | Purpose | Shortcut | Command |
| --- | --- | --- | --- |
| [Overview](overview.md) | Cluster health, warnings, and inventory | `11` | `:1` |
| Explore | Resource lists and details | `22` | `:2` |
| [Activity](activity.md) | Workspace changes and Helm history | `33` | `:3` |

Press both keys consecutively. A single number does not switch views.

Plous starts in Overview. Explore starts with Pods in the context's namespace, or `default` if none is set. Type `:K` to choose another context.

## Run a command

1. Press `:` while navigating.
2. Type a command, such as `s` for Settings.
3. Press Enter to run it, or Esc to cancel.

These docs write that sequence as `:s`. Commands work from any navigation panel.

Common commands are `:h` for Help, `:s` for Settings, and `:c` for chat. The [keymap](../reference/keymap.md#commands) lists their full names.

## Switch between tree and main

Press Tab during navigation to switch between the sidebar and main panel. Tab reveals a hidden sidebar and leaves main fullscreen.

Use **Ctrl+]** for the agent panel. Tab does not enter or leave it. Forms, searches, editors, Help, Sources, and native terminals keep their own Tab behavior.

## Know when you are typing

Text fields accept literal text. Typing `cc`, `22`, or `:pods` in a field does not run a shortcut.

- Press `cc` from navigation to enter chat or the native agent terminal.
- Press Esc to leave Plous chat typing and keep your draft.
- Press Ctrl+] to leave a native agent terminal.
- Use arrows and Enter to move through forms. Save applies changes. Esc cancels.

Paste works in text fields and chat. Pasted navigation keys and colon commands are ignored.

## Search and clear a filter

Press `/` in the panel you want to search.

| View | Search behavior |
| --- | --- |
| Sidebar, Activity, history | Filter entries. Use arrows and Enter to open a result. |
| Kubernetes table | Type a row filter and press Enter to apply it. |
| Overview | Filter its lower lists |
| Details, logs, chat | Find text. Press Enter to finish typing, then `n` / `N` for the next / previous match. |

Esc closes search. In a resource table, it cancels an unfinished edit but keeps an applied filter. Press Backspace in table navigation to clear that filter without changing the namespace, labels, or sorting.

Log search pauses following. Press Space after closing search to resume it.

## Go back with Esc

Esc closes one layer at a time: search, selection, input, then the current detail or document. A modified editor asks whether to keep editing or discard changes.

Returning from a detail view keeps the list's filter and selection. At a root main view, Esc focuses the visible sidebar.

Esc does not stop a running chat reply. Use `:X` for that. Leaving a resource operation does not undo a request Kubernetes has already accepted.

## Make more room

| Key | Action |
| --- | --- |
| `bb` | Hide or show the sidebar |
| `ii` | Cycle the assistant through normal width, double width, and hidden |
| `ff` or `:f` | Expand or restore main |
| Ctrl+] | Focus the agent, or return to the previous panel |

Fullscreen lasts while you navigate within main. Repeat `ff`, change panel focus, or toggle a panel to restore the layout. In an editable document, use `:f`.

Panel visibility and assistant size are saved across restarts. Hiding the assistant keeps its draft and running session.

## Use Help

Press `hh` or `:h`. Help shows controls for the workspace tree, current view, and chat, with global controls below.

Use Left / Right to choose a section. Use arrows or page keys to scroll. Esc returns to your previous view, including an unfinished form.

Next: [Save a workspace](workspaces.md) or [explore Kubernetes](../integrations/kubernetes/README.md#explore-resources).
