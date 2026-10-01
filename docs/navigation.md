# Learn the interface

[Documentation](README.md) · [Keymap](keymap.md)

Plous has three panels. The highlighted border and footer identify the panel receiving your keys. This is called **focus**.

| Panel | What it contains |
| --- | --- |
| Left: Workspaces | Your workspaces, tool connections, available tool views, and Settings. |
| Main | Overview, Explore, or Activity for the current selection. |
| Right: AI assistant | The current conversation, with a message composer at the bottom. |

The panels grow with the terminal: 12.5% for Workspaces, 62.5% for the main view, and 25% for the assistant. Workspaces normally stays at least 32 columns wide and the assistant at least 40. Compact terminals relax those minimums to preserve room for the main view. Hiding either side panel gives its space to the main view.

Press **Tab** to move focus forward or **Shift+Tab** to move backward. Arrows, Enter, scrolling, and `/` apply to the focused panel.

The footer names the current view and shows a few relevant controls. A hint such as **n Namespace** means press `n` to choose a namespace. **hh Help** means press `h` twice for the full controls. Narrow terminals omit lower-priority hints; Help keeps the complete list. While typing, the footer shows the keys for finishing or leaving that input.

## Understand the workspace tree

The tree follows this structure. These are example names, not preloaded data:

```text
Commerce                    workspace
  Terraform                 tool
    Production              connection
  Kustomize                 tool
    Staging overlay         connection
  Kubernetes                tool
    Production cluster      connection
      Workloads             resource category
        Pods                resource type
        Deployments
```

A **tool** is a technology such as Kubernetes or Terraform. A **connection** is the saved configuration Plous uses with that tool, such as a Kubernetes context or a working directory. A connection name does not indicate whether a server is currently reachable.

Every row with a triangle uses the same fold controls, including workspaces, tools such as **Kubernetes**, connections such as **minikube**, and resource categories:

- **Enter** toggles expanded/collapsed. On a workspace, tool, or connection, it also selects that scope for the main panel.
- **Right** expands without changing the active scope or main view.
- **Left** collapses without changing the active scope or main view. If already collapsed, it moves the cursor to the parent.

Use Left/Right when you only want to arrange the sidebar. Enter on a resource type opens its main-panel table; rows without children have nothing to fold. Enter on a search result selects and expands the branch so the result stays visible.

Fold choices survive refreshes and restarts. Folding only changes sidebar visibility. Live Kubernetes requests follow the [active Explore view](kubernetes-explore.md#when-kubernetes-requests-run).

The filled highlight follows your keyboard selection. The active resource path remains accented when you move elsewhere. Opening resources expands their path automatically; branches you deliberately open or close keep that choice.

## Choose a main view

| View | What you use it for | Shortcut | Command |
| --- | --- | --- | --- |
| Overview | See the workspace's saved connections and revision. Selecting a tool narrows the summary. | `11` | `:1` |
| Explore | Open the selected tool connection. Available views depend on the integration. | `22` | `:2` |
| Activity | Review saved changes to workspace configuration. | `33` | `:3` |

Press a shortcut's two keys consecutively. It runs on the second key, without Enter. A single number does not switch views.

With a workspace open, Explore shows the Plous logo until you select a connection in the workspace tree.

See [Tool integrations](integrations.md) for each tool's current capabilities.

Kubernetes **Events** describe events reported by the cluster for an object. Plous **Activity** describes edits to your workspace configuration. They answer different questions.

## Run a command

1. While navigating, press **:**.
2. Type a command name in the box at the bottom of the main panel. For example, type `s` for Settings.
3. Press **Enter** to run it, or **Esc** to cancel and restore the previous focus.

The docs write that sequence as `:s`. Commands work from any navigation panel. They are not shell commands, and you do not need to enter a separate command mode first.

Help keeps global navigation visible and shows view controls for the current screen, such as Logs, Exec, YAML, or Settings.

Use short commands: `:h` for Help, `:s` for Settings, and `:c` for chat. Full names are optional alternatives listed in the [keymap](keymap.md#commands).

## Know when you are typing

A focused search field, form field, command box, or chat composer accepts text. Typing `cc`, `22`, or `:pods` inside an ordinary text field inserts those characters; it does not navigate.

To start a chat draft, use `:c` or **cc** while navigating. Merely focusing the right panel does not enter typing. Press **Esc** to leave chat typing and keep your draft.

In forms, **Up / Down** moves between fields and buttons. **Enter** advances to the next field or activates the selected button. **Left / Right** changes a choice such as an AI vendor. Save applies changes; Esc cancels them. Tab does nothing in Settings and Help.

Paste works in fields, searches, and active chat input. Paste into navigation or the colon command box is ignored. Type colon commands directly.

## Search and clear a filter

Press **/** while navigating. A bordered field opens inside the focused panel.

| Where you search | What happens |
| --- | --- |
| Workspace tree | Names are filtered, including matching items inside collapsed branches. Arrows select; Enter opens. |
| Kubernetes table | Type a row filter and press Enter to apply it. |
| Activity or conversation history | Entries are filtered by their displayed text. Arrows select; Enter opens. |
| Details, logs, Overview, or a conversation | Matches in the view's text are highlighted and counted. Enter finishes query input; n/N moves to the next or previous match. While typing, Down/Up steps through matches. |

Tree search finds resource types after their connection's catalog has loaded. It does not contact other clusters to search them.

An applied table filter stays in its bar with **Backspace clear**. Press **Backspace** while navigating the table to clear the text filter. Namespace, label selector, sorting, and selection stay as they were. Backspace still deletes a character while you are typing.

**Esc** closes an active search or cancels an unfinished filter edit. It preserves a table filter you already applied with Enter. In tree, Activity, and history search, Esc restores the unfiltered list. Log search pauses following; after closing search, **Space** resumes it.

## Go back with Esc

Each press closes one current layer:

- An unfinished command, search, picker, or form closes or cancels first.
- Activity comparison returns to its summary.
- Search and selection close before leaving a document or output view.
- An expanded view restores the panel layout before closing.
- YAML returns to the detail section that opened it, retaining its scroll position. YAML opened from a list returns to that list.
- A value preview returns to its value picker and clears the preview buffer.
- Logs returns to the resource detail that opened it.
- A related resource returns to the previous resource.
- A resource detail returns to its list, keeping the filter and selection.
- At a root main view, Esc focuses the visible sidebar. It does not reveal a sidebar you hid.

If you pressed only the first key of a double-key shortcut, Esc clears that pending key first. Look at the footer to see the pending prefix.

Leaving chat typing does not stop a running reply. Use `:X` to stop it. Leaving an operation after Kubernetes has accepted it does not undo that operation.

## Make more room

Press **bb** to hide or show the sidebar and **ii** to hide or show the assistant. The main panel uses the freed space. Tab skips hidden panels. Plous remembers visibility across restarts.

Use `:c` to reveal the assistant and type again. Hiding it preserves drafts and lets a reply continue.

Use `:z` / `:zoom` to temporarily expand Logs, Exec output review, YAML, a value preview, the Editor, or a comparison. Repeat the command to restore the layout. This keeps your saved panel visibility unchanged. See [Expand a view](keymap.md#expand-a-view) for focus and Esc behavior.

## Use Help

Press **hh** or run `:h`. Help opens above the retained view with workspace, view, chat, and global controls.

**Left / Right** selects one of its three upper sections. **Up / Down** and **Page Up / Page Down** scroll that section. **Esc** closes Help and returns to what you were doing, including an unfinished Settings form. Tab does nothing while Help is open.

Next: [Manage workspaces](workspaces.md) or [Explore Kubernetes](kubernetes-explore.md).
