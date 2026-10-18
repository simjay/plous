# Learn the interface

[Documentation](../README.md) · [Keymap](../reference/keymap.md)

Plous has three panels. The blue focus border and footer identify the panel receiving your keys. This is called **focus**. Main's tab outlines turn neutral when another panel has focus.

| Panel | What it contains |
| --- | --- |
| Left: Connections | Current Workspace, saved workspaces, connection views, and Settings. |
| Main | Overview, Explore, or Activity for the current selection. |
| Right: AI assistant | Plous chat with its bottom composer, or the selected native agent's terminal UI. |

The panels grow with the terminal: 12.5% for the sidebar, 62.5% for the main view, and 25% for the assistant. The sidebar normally stays at least 32 columns wide and the assistant at least 40. Compact terminals relax those minimums to preserve room for the main view. Cluster Overview keeps visible side panels at least 28 columns wide from an 80-column terminal. Switching focus keeps these widths unchanged. Hiding either side panel gives its space to the main view. Below 80 columns, Overview shows only the focused panel.

Click a panel to focus it, click tabs and controls to activate them, and use the wheel over the content you want to scroll. Data rows use one click to select and a double click to open. Click text in fields and the composer to place the cursor without submitting. Drag selects text; double click selects a word. Click **Copy selection** in the footer to copy selected text. Mouse support can be turned off in **Settings → Application** for native terminal selection and copy shortcuts. See the [mouse reference](../reference/keymap.md#mouse-controls) for viewer behavior and limitations.

Press **Alt+Right** to move focus forward or **Alt+Left** to move backward, including while typing or using an embedded terminal. Arrows, Enter, scrolling, and `/` apply to the focused panel.

Headings use near-white, while active headings, links and actions use light blue. Secondary details use muted cool gray. Status labels and symbols accompany green success, amber warning and red error colors.

The footer names the current view and shows a few relevant controls. A hint such as **n Namespace** means press `n` to choose a namespace. **hh Help** means press `h` twice for the full controls. Narrow terminals omit lower-priority hints; Help keeps the complete list. While typing, the footer shows the keys for finishing or leaving that input.

## Understand the workspace tree

The sidebar starts with **Current Workspace**, followed by **Workspaces** and **Application / Settings**. Current Workspace contains the selected cluster under Monitor and the launch directory under Code. Both reuse the same tree and controls as saved connections. Saved workspaces follow this structure. These are example names, not preloaded data:

```text
Commerce
  Monitor
    Production (k8s)
      Kubernetes
        Workloads
          Pods
      Helm
        Releases
        Charts
      Argo
      Flux
  Code
    platform-config (local)
      Files
      Helm
      Kustomize
      Argo
      Flux
```

**Monitor** and **Code** organize connections within each workspace. Kubernetes is always available; Helm appears after release metadata is detected, and Argo and Flux appear after their APIs are discovered. A connection identifies the configured context or source. Its name does not establish that the target is reachable.

Code provides **Files** for browsing and editing, grouped Helm, Argo and Flux configuration cards, and Kustomize dependency groups. H/K/A/F directory tags identify detected technologies. Open a Kustomize group, then a graph node, to choose **G Graph**, **r Render**, or **d Open source**. Back restores the preceding view, selection and search. Rendered objects have contextual field history, findings and comparisons. See [Code and Kustomize](../integrations/kustomize/README.md) for the full workflow.

Connection labels stay on one line. Long names end in `...` so their muted type or source label, such as `(k8s)` or `(local)`, stays visible. Sidebar search uses the full name. Overview and Settings show the saved name. Equivalent legacy cluster records with the same name share one sidebar entry; their saved identities and history remain available.

Every row with a triangle uses the same fold controls, including workspaces, tools such as **Kubernetes**, connections such as **minikube**, and resource categories:

- **Enter** toggles expanded/collapsed. On a workspace, tool, or connection, it also selects that scope for the main panel.
- **Right** expands without changing the active scope or main view.
- **Left** collapses without changing the active scope or main view. If already collapsed, it moves the cursor to the parent.

Opening a branch keeps keyboard focus in the sidebar, even when its content opens in Explore. Opening a leaf view such as Files moves focus to main. Use Left/Right when you only want to arrange the sidebar. Enter on a resource type opens its main-panel table; rows without children have nothing to fold. Enter on a search result selects and expands the branch so the result stays visible.

Selecting a Code source by Enter or clicking its name opens **Files**, its first view. Folding and reopening the selected source keeps its current page and any unfinished file edit. Clicking a disclosure triangle, like pressing Right or Left, only changes the tree.

Collapsing a cluster connection also collapses its tool branches. Reopening the connection shows Kubernetes and any detected tools as collapsed branches. At startup, Current Workspace opens Overview and expands the selected cluster and Kubernetes branch. Saved workspace connections and their cluster tool branches start collapsed. Choose a connection to resume its saved preferences. Workspace, Monitor/Code section, resource-category, and file-folder fold choices remain saved. Loading or reconnecting Kubernetes preserves these folds; choosing a resource explicitly can reveal its path. Refreshes preserve fold choices. Selecting or expanding a cluster connection discovers Helm release metadata and Argo and Flux APIs. Press `g` on the cluster or a tool branch to refresh discovery. **Tool discovery incomplete** means some checks failed or were denied; it does not mean those tools are absent. Collapsing it only changes sidebar visibility. Live Kubernetes resource requests follow the [active Explore view](../integrations/kubernetes/README.md#explore-resources).

The filled highlight follows your keyboard selection. The active resource path remains accented when you move elsewhere. Opening resources can expand their path automatically. Deliberate fold choices remain until you change them, collapse the containing cluster connection, or restart as described above.

A small lighthouse appears above Application when the full tree fits with spare room. It disappears as the tree grows, on smaller sidebars, or during search. It never reduces the visible tree area.

Choose which agent supplies the right panel in **Settings → Application → Agent**. Fresh profiles use Plous. Claude Code and Codex run your installed CLI with its own prompt, transcript, and menus. Plous records their tool evidence separately for Sources. See [AI chat](ai-chat.md#choose-an-agent) for setup.

## Choose a main view

| View | What you use it for | Shortcut | Command |
| --- | --- | --- | --- |
| Overview | See [cluster health, observed changes, warnings, and inventory](overview.md) for a Monitor connection. Workspace and Code selections show saved connections. | `11` | `:1` |
| Explore | Open the selected tool connection. Available views depend on the integration. | `22` | `:2` |
| Activity | Review workspace changes, Helm operations, and observed release history. | `33` | `:3` |

Press a shortcut's two keys consecutively. It runs on the second key, without Enter. A single number does not switch views.

Plous starts on Overview in Current Workspace each time. Explore starts with Pods in the kubeconfig context namespace, or `default` when unset. Your saved workspaces, conversations, drafts, panel visibility, and tool preferences remain available. Only the current cluster connects at startup. The launch directory is scanned when you open Code, select or expand its source, or refresh it; saved Code sources are scanned when you select their workspace. Code remains available without a resolved Kubernetes target. Use `:K` to switch context, or select a saved connection under Workspaces. Without a resolved target, Enter opens context selection and `:s` offers a kubeconfig path. Use `:n` for optional workspace setup.

See [Tool integrations](../integrations/README.md) for each tool's current capabilities.

Kubernetes **Events** describe events reported by the cluster for an object. Plous **Activity** combines saved workspace changes with Helm operation outcomes and observed release history. It is not a complete cluster audit. See [Activity and history](activity.md).

## Run a command

1. While navigating, press **:**.
2. Type a command name in the box at the bottom of the main panel. For example, type `s` for Settings.
3. Press **Enter** to run it, or **Esc** to cancel and restore the previous focus.

The docs write that sequence as `:s`. Commands work from any navigation panel. They are not shell commands, and you do not need to enter a separate command mode first.

Help keeps global navigation visible and shows view controls for the current screen, such as Logs, Exec, YAML, or Settings.

Use short commands: `:h` for Help, `:s` for Settings, and `:c` for chat. Full names are optional alternatives listed in the [keymap](../reference/keymap.md#commands).

## Switch between tree and main

Press **Tab** during navigation to switch between the tree and main. It reveals the tree if hidden and restores the layout when leaving main fullscreen. **Ctrl+]** handles the agent separately. Tab never enters or leaves the agent; Shift+Tab does not switch panels. Forms, search and command input, editable documents, Help, Sources and native terminals keep their local Tab behavior.

## Know when you are typing

A focused search field, form field, command box, or chat composer accepts text. Typing `cc`, `22`, or `:pods` inside an ordinary text field inserts those characters; it does not navigate.

With the Plous agent selected, use `:c` or **cc** while navigating to start a chat draft. Merely focusing the right panel does not enter typing. Press **Esc** to leave Plous chat typing and keep your draft. For a native agent, **cc** enters its terminal; **Ctrl+]** toggles focus between the agent and the previous panel, preserving its running session. See [native agent controls](ai-chat.md#choose-an-agent).

In forms, **Up / Down** moves between fields and buttons. **Enter** advances to the next field or activates the selected button. **Left / Right** changes a choice such as an AI vendor. Save applies changes; Esc cancels them. Alt+Left/Right leave and resume Settings and Help without losing unfinished work.

Paste works in fields, searches, and active chat input. Paste into navigation or the colon command box is ignored. Type colon commands directly.

## Search and clear a filter

Press **/** while navigating. A bordered field opens inside the focused panel.

| Where you search | What happens |
| --- | --- |
| Workspace tree | Names are filtered, including matching items inside collapsed branches. Arrows select; Enter opens. |
| Kubernetes table | Type a row filter and press Enter to apply it. |
| Activity or conversation history | Entries are filtered by their displayed text. Arrows select; Enter opens. |
| Cluster Overview | Filter the lower lists; Enter inspects a match. Esc clears the query. |
| Details, logs, workspace summary, or a conversation | Matches in the view's text are highlighted and counted. Enter finishes query input; n/N moves to the next or previous match. While typing, Down/Up steps through matches. |

Tree search finds resource types after their connection's catalog has loaded. It does not contact other clusters to search them.

An applied table filter stays in its bar with **Backspace clear**. Press **Backspace** while navigating the table to clear the text filter. Namespace, label selector, sorting, and selection stay as they were. Backspace still deletes a character while you are typing.

A search in a covered view keeps its query but does not receive input. After Enter finishes text-find input, visible action keys work normally; `n`/`N` still navigates matches.

**Esc** closes an active search or cancels an unfinished filter edit. It preserves a table filter you already applied with Enter. In tree, Activity, and history search, Esc restores the unfiltered list. Log search pauses following; after closing search, **Space** resumes it.

## Go back with Esc

Each press closes one current layer:

- An unfinished command, search, picker, or form closes or cancels first.
- Chart inspection closes before the underlying Describe search, expanded layout, or detail view.
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

Press **bb** to hide or show the sidebar. **ii** or `:i` cycles the assistant through normal width, double width, hidden, and back to normal. Double width keeps room for main on smaller terminals. Panel travel skips hidden panels. Plous remembers size and visibility across restarts.

**Ctrl+]** switches to the agent panel and back to the previous panel. It reveals a hidden assistant and preserves drafts, forms and overlays. For Claude Code or Codex, use it to reach Plous navigation, then press it again to resume the same CLI. Captured Exec retains Ctrl+] for output review. **ff** temporarily expands main and restores the previous assistant size when closed.

Use `:c` to reveal the assistant and type again. Hiding it preserves drafts and lets a reply continue.

Use **ff**, `:f`, or `:fullscreen` from any navigation panel to expand main to the full width. This works in every main view, including Overview, Explore tables and details, and Activity. Navigating within main keeps it expanded. Repeat the command to restore your previous layout, or use Alt+Left/Right to move to a side panel. Saved panel visibility stays unchanged. In an editable document, use `:f` so document keys retain their meaning. Read-only Kubernetes YAML also supports `ff`. See [Expand a view](../reference/keymap.md#expand-a-view) for focus and Esc behavior.

## Use Help

Press **hh** or run `:h`. Help opens above the retained view with workspace, view, chat, and global controls.

Each upper section describes its own context. Workspace Help follows the highlighted tree row. The center follows the open Monitor or Code view, integration, resource, and any active editor, picker, or dialog. Chat Help follows the composer, history, reply reader, or Sources. Opening Help selects the section you came from.

The controls also follow the current interaction: typing a search or namespace, selecting text, reading a comparison, or choosing its actions. Global view commands (`:1` to `:3`, or `11`, `22`, and `33`) stay in the bottom box.

Primary keys appear first; alternative keys and full command names follow below. A position indicator shows when more controls are available by scrolling. Help captures the current controls when opened, so background updates do not move the reference while you read. Close and reopen it to refresh the context.

**Left / Right** selects one of its three upper sections. **Up / Down** and **Page Up / Page Down** scroll that section. **Esc** closes Help and returns to what you were doing, including an unfinished Settings form. Alt+Left/Right pause Help and restore it when you return to its panel.

Selecting a Help section does not change the underlying panel, connection, or resource. Local keys describe the view you will return to; pressing them in Help does not run that action. The global box keeps shared navigation and exit controls visible.

Next: [Manage workspaces](workspaces.md) or [Explore Kubernetes](../integrations/kubernetes/README.md#explore-resources).
