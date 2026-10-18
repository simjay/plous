# Keymap

[Documentation](../README.md) · [Learn the interface](../guides/navigation.md)

Keyboard reference for Plous 0.0.1. **Navigation** means you are outside a text field, search editor, or chat composer. Colon commands work from any navigation panel; ordinary keys affect the focused panel. Type a colon command and press **Enter** to run it.

Application action aliases use one letter and are case-sensitive. Capitals distinguish actions when lowercase is taken, such as `c` for chat and `C` for new chat. Full names are optional alternatives. The double-key navigation shortcuts below, editor sequences such as `gg`, `dd`, and `:wq`, and Kubernetes resource abbreviations such as `po` and `svc` keep their own meanings.

Jump to [essential controls](#essential-controls), [commands](#commands), [Overview](#cluster-overview), [Kubernetes](#kubernetes-explore), [Helm](#helm-explore), [Argo and Flux](#argo-and-flux), [Code](#kustomize-explore), [Editor and Diff](#plous-editor-and-diff-reviewer), [logs and terminals](#containers-and-logs), or [AI assistant](#ai-assistant).

## Mouse controls

Mouse support is on by default. Change **Settings → Application → Mouse support** with a click or the keyboard. Off is saved across restarts and lets the terminal handle mouse input normally.

| Target | Action |
| --- | --- |
| Panel background or title | Focus that panel. |
| Tabs, buttons, named footer actions, menus, and Settings entries | Click to activate. |
| Sidebar label | Click to select or open. Click its triangle to fold or expand only. |
| Resource rows, Helm cards and revisions, Activity, conversation history, and Sources lists | Click to select; double click to open. |
| Table heading or namespace control | Click to sort or choose scope. |
| Field, search box, command box, or composer | Click text to place the cursor. Drag to select; double click to select a word. Typing replaces a field selection. Labels and borders focus without submitting. |
| Editor or shared viewer text | Click to position; drag to select; double click to select a word. Click **Copy selection** in the footer or use `y`. |
| Option value or context checkbox | Click to change the pending value. Save remains explicit. |
| Reply and its Sources hint | Click the reply to select it; click Sources to open saved evidence. |
| Scrollable content | Wheel scrolls under the pointer without changing keyboard focus. |
| Help section | Click to select; wheel scrolls that section. |

A double click means two left clicks within 400 ms on the same item. Dragging selects text in fields and viewers; it cancels button and row activation. Resizing, scrolling, source changes, or intervening keyboard input cancels the gesture. Overlays block clicks on the panels behind them. Clicked Save uses the same validation and confirmation as keyboard Save.

Wheel scrolling preserves editor drafts, cursor, mode, selection, and undo history. In logs it pauses following while collection continues. Click **Jump live** to return to new log output. Mouse controls remain available while a native assistant captures keyboard input. Click another panel to move focus; click the native panel to resume its input without restarting the session. Mouse reporting is suspended while Exec captures terminal input.

Text placement respects wrapping, tabs, and Unicode characters. Viewer copying excludes gutters and visual wrapping. Esc clears field selections before leaving input. Clicking an editable document preserves Insert mode; dragging or selecting a word enters Select. Dragging outside the text area keeps the last selection without scrolling or activating another control. In chat, use **Read reply** for text selection; the transcript retains reply selection.

Horizontal scrolling, hover, right-click menus, panel resizing, and forwarding clicks into Exec remain unavailable. Turn mouse support Off or use your terminal's own mouse override modifier for native terminal selection. The modifier varies by terminal.

## Copy and paste

On macOS, use your terminal's **⌘C / ⌘V**. GNOME Terminal on Ubuntu uses **Ctrl+Shift+C / Ctrl+Shift+V**. These shortcuts normally belong to the terminal itself. Its Copy command copies a native terminal selection, not a selection drawn by Plous.

For a Plous selection, click **Copy selection** in the footer. Shared viewers also retain **y** to copy a selection or line. Copy preserves source text without gutters or visual wrapping. Fields, searches, the composer, documents, logs, and comparisons support the Copy button; masked credential fields do not expose their contents through it. Selecting text alone does not replace the clipboard.

Plous also recognizes **⌘C / ⌘V** and **Ctrl+Shift+C / Ctrl+Shift+V** when a terminal forwards those keys. Copy requires selected text. Paste inserts literal text into the active field, search, composer, or editable document without submitting it. Paste in navigation and colon-command input is ignored. Captured Exec and native assistants forward keys and receive native terminal paste.

Local clipboard actions use macOS's clipboard tools or available Linux tools (`wl-copy` / `wl-paste`, `xclip`, or `xsel`). Clipboard reads happen only after an explicit forwarded Paste shortcut. If a clipboard read fails or those tools are unavailable, use the terminal's Paste command. Moving focus or editing before a read finishes cancels that paste. Copy falls back to a terminal clipboard request when native access is unavailable; its success depends on the terminal. SSH and Mosh sessions use terminal copying and native terminal paste rather than the remote machine's desktop clipboard.

To use native selection and copy shortcuts throughout the screen, turn **Settings → Application → Mouse support Off**. Terminal shortcuts that the terminal consumes cannot be remapped by Plous. **Ctrl+C goes to the active native agent terminal; elsewhere it saves and exits Plous.**

## Essential controls

| Key | Action |
| --- | --- |
| Ctrl+] | Toggle agent focus and return to the previous panel. Captured Exec enters output review. |
| Alt+Left / Alt+Right | Previous / next panel, wrapping through visible panels. Works in fields, editors, forms, pickers, overlays and captured terminals. |
| Tab | Toggle tree and main during navigation, revealing the tree if hidden. Never enters or leaves the agent. |
| Tab / Shift+Tab in local input | Keep local behavior in forms, search, editors, and captured terminals. Shift+Tab never switches panels. |
| Up / Down | Select or scroll within the focused panel. |
| Enter | Open the selected item or activate the selected control. |
| `/` | Search the focused panel. |
| Esc | Cancel or close the current layer. At a root main view, focus the visible sidebar. |
| `:` | Open the command box at the bottom of the main panel. |
| Ctrl+C | Forward to the focused native agent terminal. Elsewhere, save the session and drafts, stop active work, and exit. Unsaved form edits are not applied. |

## Sidebar source refresh

With the sidebar focused, **g**, **:g**, or **:refresh** rescans the highlighted source. Highlight **Code** or a workspace to refresh all its sources. The main view and open editor drafts stay in place. Connections start collapsed when Plous opens. Selecting or expanding a source also rescans it; simply moving the cursor does not. Search fields keep literal input.

For a Monitor cluster connection or tool branch, **g** refreshes controller API and Helm release metadata discovery. In the main Argo or Flux view, **g** refreshes that view.

## Double-key shortcuts

Press the same key twice consecutively. These run on the second key, with no Enter and no timing deadline.

| Shortcut | Command alternative | Action |
| --- | --- | --- |
| `11` | `:1` | Overview. |
| `22` | `:2` | Explore. |
| `33` | `:3` | Activity. |
| `bb` | `:b` | Show or hide the left panel. |
| `ii` | `:i` | Cycle right panel: normal → double → hidden. |
| `ss` | `:s` | Settings. |
| `cc` | `:c` | Reveal the assistant and enter typing. |
| `hh` | `:h` | Help. |
| `ff` | `:f` | Fullscreen or restore the previous layout. |

The footer shows a pending first key. Esc cancels that prefix first. Another key, panel focus change, resize, or paste also clears it; another reserved first key starts its own sequence. Single numbers do not change views.

These shortcuts work during navigation, including Help, history, and form action buttons. They also work in read-only YAML and value views outside search input. YAML keeps `z` for selection. Text fields accept them literally, and the editable document's Vim keys take precedence while its document has focus. Switching Settings or views cannot discard an unfinished form.

## Commands

| Short command | Full spelling | Action |
| --- | --- | --- |
| `:n` | `:new` | Create a workspace. |
| `:s` | `:settings` | Open Settings. |
| `:c` | `:chat` | Enter the current composer. |
| `:C` | `:newchat` | Start a conversation and enter typing, keeping the unsent draft. |
| `:H` | `:openchat` | Open conversation history in the current scope. |
| `:A` | `:readreply` | Open the selected finished answer in the wider Markdown viewer. |
| `:X` | `:stopchat` | Stop the running reply, including one in another workspace. |
| `:p` | `:restoreprompt` | Restore the last sent message into an empty draft for editing. Does not send it. |
| `:b` | `:sidebar` | Toggle the left panel. |
| `:i` | `:aiinteraction` | Cycle right-panel size and visibility. |
| `:f` | `:fullscreen` | Fullscreen or restore the previous layout. |
| `:h` | `:help` | Open Help. |
| `:q` | `:quit` | Save and exit Plous. In the focused editable document, `:q` closes that document. |

`:1`, `:2`, and `:3` select Overview, Explore, and Activity. They have no word aliases.

In the command box, Enter runs and Esc cancels, restoring the originating focus. Alt+Left/Right can leave and resume the draft, including inside overlays. A fresh colon from another navigation panel replaces it. Completing a double-key shortcut from another panel also replaces it. Pasting into the command box is ignored.

## Switch panels

**Tab** switches between the tree and main during ordinary navigation. A hidden tree is revealed, and leaving main fullscreen restores the layout. Tab does not move into or out of the agent. **Shift+Tab** has no panel-switching role. Forms, search and command input, editable documents, Help, Sources and native terminals retain local Tab behavior.

**Ctrl+]** focuses the agent panel, revealing it if hidden. Press it again to return to the previous sidebar or main panel. It works during typing, in editors, forms, overlays, command input, and native Claude Code or Codex input. Drafts, overlays and running sessions stay intact. If the previous panel is hidden, return goes to main. In captured Exec, Ctrl+] retains its output-review action; it toggles agent focus from Exec review.

Alt+Right cycles sidebar → main → assistant through visible panels; Alt+Left reverses that order. They work everywhere, including chat typing, every editor mode, forms, pickers, command input and captured terminals. Drafts and running processes stay intact. Settings, Help, Sources and the reply reader pause when you leave their panel and return when you focus it again.

On macOS, use Option+Left/Right. Plous accepts Alt-arrows (`ESC [ 1 ; 3 D` / `ESC [ 1 ; 3 C`), iTerm2’s Meta-arrows (`ESC [ 1 ; 9 D` / `ESC [ 1 ; 9 C`), and Escape+b/f. Both Alt and Meta arrow encodings also work with the Kitty keyboard protocol. Because Escape+b/f is also Alt+B/F, those keys switch panels too, including in text fields and embedded terminals. Plous reserves these keys before forwarding input to Claude Code, Codex or Exec. A shortcut intercepted by the terminal itself must be changed in that terminal’s settings.

### Show or hide panels

Use **bb** / `:b` to toggle the sidebar. **ii** / `:i` cycles the assistant from normal width to double width, hidden, then normal. Double width retains room for main on smaller terminals. Hiding the focused panel moves focus to main. Size and visibility are saved; panel travel skips hidden panels. Chat commands and Ctrl+] reveal the assistant.

**ff** temporarily expands main and restores the assistant size on exit. In a native Claude Code or Codex terminal, press **Ctrl+]** to return to Plous navigation before using sizing shortcuts; press it again to resume the same terminal.

## Search the focused panel

| View | `/` behavior | Finish or clear |
| --- | --- | --- |
| Workspace tree | Filter names, including collapsed paths in loaded resource catalogs. | Arrows select; Enter opens; Esc restores the unfiltered tree. |
| Kubernetes table | Edit the row text filter. | Enter applies; Esc cancels the edit; Backspace clears an applied filter while navigating rows. |
| Activity list / chat history | Filter displayed entries. | Arrows select; Enter opens; Esc restores the list. |
| Cluster Overview | Filter the lower lists. | Enter inspects a match; Esc clears the query. |
| Workspace summary, connection settings, details, logs, conversation | Find and highlight text. Conversation find also searches unloaded saved messages. | Enter finishes query input; n/N next/previous match; Esc clears find. |

The bar shows **Backspace clear** for an applied table filter. Clearing preserves namespace, labels, sorting, and selection. While typing, Backspace deletes a character. Root Esc preserves an applied filter.

Alt+Left/Right can leave and resume search, including inside overlays. Search fields accept literal paste. Log search pauses following; Space resumes after search closes. `/` in a chat draft or form remains text, apart from the explicit chat model commands below.

## Workspace tree

| Key | Action |
| --- | --- |
| Up / Down | Previous / next row. |
| Home / End | First / last row. |
| Page Up / Page Down | Move ten rows. |
| Right | Expand a branch without changing the active scope or main view. |
| Left | Collapse an expanded branch, otherwise move to its parent. |
| Enter | Select workspace/tool/connection and toggle its branch; toggle a category; open a resource; activate a setup/Settings row. |

The same fold controls apply to every row with a triangle: workspace, tool, Kubernetes connection, and category. Enter preserves the main mode and tree focus while selecting that row's scope. Left/Right only arrange the tree. Search results expand when selected with Enter. Opening a resource selects Explore and focuses its table. Setup actions open a centered window.

## Kubernetes Explore

### Resource commands

These work from any navigation panel and open Explore. If no Kubernetes connection is selected, a connection picker opens first. In Current Workspace, `:K` / `:connections` searches local kubeconfig contexts; in a saved workspace it searches saved cluster connections. Choosing a target changes Plous only.

| Resource or control | Commands |
| --- | --- |
| Connection picker | `:K`, `:connections` |
| Namespace picker | `:N`, `:namespace` |
| Set namespace directly | `:N shop`, `:namespace shop`; use `*` for all namespaces. |
| Pods | `:po`, `:pod`, `:pods` |
| Deployments | `:deploy`, `:deployment`, `:deployments` |
| StatefulSets / DaemonSets / ReplicaSets | `:sts`, `:ds`, `:rs`, or each plural name. |
| Jobs / CronJobs | `:job`, `:jobs`, `:cj`, `:cronjobs` |
| Services / Endpoints / EndpointSlices | `:svc`, `:ep`, `:eps`, or each plural name. |
| Ingresses / NetworkPolicies | `:ing`, `:netpol`, or each plural name. |
| ConfigMaps / Secrets | `:cm`, `:sec`, or each plural name. |
| PersistentVolumeClaims / PersistentVolumes / StorageClasses | `:pvc`, `:pv`, `:sc`, or each plural name. |
| Nodes / Namespaces / Events / ServiceAccounts | `:no`, `:ns`, `:ev`, `:sa`, or each plural name. |
| CustomResourceDefinitions | `:crd`, `:customresourcedefinitions` |
| Refresh | `:g`, `:refresh` |

Other discovered names and unambiguous short names work too. Qualify an ambiguous resource with its API group, for example `:widgets.example.io`. Commands are case-sensitive: `:N` chooses a namespace scope, while `:ns` opens Namespaces. `:X` stops chat, while `:sc` opens StorageClasses.

Resource commands accept an optional namespace, such as `:pods shop` or `:pods *`. Only `*` means all namespaces; `all` is treated as a namespace named `all`.

### Tables and pickers

| Key | Action |
| --- | --- |
| Up / Down or `k` / `j` | Select a row. |
| Home / End | First / last row. |
| Page Up / Page Down | Move by a page. |
| Enter or `d` | Open Describe. |
| `y` | Open the selected object's YAML. |
| `n` | Search and choose namespace. |
| `/` | Edit the row text filter. |
| Backspace | Clear only the applied text filter while navigating. |
| `I` | In Pod or Node lists, open [top consumers](#usage-charts) within the current filters. |
| `L` | Edit a Kubernetes label selector. Apply an empty value to clear it. |
| `S` | Choose a sort column; choosing the current column reverses it. |
| `g` | Reconnect and rediscover APIs. `:g` / `:refresh` also works. |
| Esc | Return to the previous related view, or reveal and focus the sidebar at the root. |

Click the resource heading or press Esc from the resource table to reveal and focus the sidebar. Use its search to choose a resource type. There is no table `o` shortcut. In the connection picker, type to search, arrows select, Enter applies, and Esc cancels. In the namespace picker, clear the search and choose **Reload namespaces** to reload; manual entry remains available. Alt+Left/Right leave and resume a picker without applying it.

### Object details

| Key | Command | Action |
| --- | --- | --- |
| `d` | `:d` / `:describe` | Describe. |
| `y` | `:y` / `:yaml` | Open YAML; within YAML, `y` copies text. |
| `e` | | Events. |
| `r` | | Related resources. Enter follows a selected relationship. |
| `[` / `]` | | Previous / next detail section. |
| `/` | | Find text in the section. |
| | `:g` / `:refresh` | Refresh the object and section. |
| `v` in Secret YAML | | Reveal / hide values for this view. |
| Esc | | Return to the preceding resource or list. |

YAML uses Plous Editor in read-only mode: **/** searches, **n/N** moves between matches, **z** selects, and **y** copies the selection or current line without line numbers. Secret YAML reserves **v** for reveal/hide. Esc clears search or selection first, then returns to the section that opened YAML with its scroll position preserved. YAML opened directly from a list returns to that list. The tab shows `:y` inside YAML because `y` copies text there. Lowercase **e** opens Events; uppercase **E** starts editing. Paste in navigation is ignored; paste into search stays literal.

### Detail actions

Open an object's details first. Commands and keys run the same action, subject to its API capabilities and permissions.

| Key | Full command | Short alias | Action |
| --- | --- | --- | --- |
| `a` | `:actions` | `:a` | More actions. |
| `l` | `:logs` | `:l` | Pod or workload logs. |
| `x` | `:exec` | `:x` | Container terminal. |
| `S` | `:scale` | `:S` | Set replicas. |
| `R` | `:restart` | `:R` | Workload rollout restart. |
| `E` | `:edit` | `:E` | Edit manifest. |
| `D` | `:delete` | `:D` | Graceful deletion. |
| `Y` | `:copyidentity` | `:Y` | Copy object identity. |
| `V` | `:copyvalue` | `:V` | Inspect/copy supported values. |
| `w` | `:exportyaml` | `:w` | Export YAML to a new file. |
| `u` | `:usenamespace` | `:u` | Use the open Namespace as browsing scope. |

Case matters: **D/E/R** are actions; **d/e/r** are sections.

After Scale, Restart, or Delete is accepted, the progress view uses **Enter/Esc** to return to details, arrows or **j/k** and Page Up/Down to scroll, and Home/End for the first/last page. When checking has stopped without confirmed completion, **g** checks status again. It does not repeat the mutation.

Confirmations use Left/Right to select and Enter to activate. Resource editing and comparison use the dedicated controls below. See [Kubernetes actions](../integrations/kubernetes/README.md#resource-actions) for the validation and submission steps.

### Expand a view

| Short command | Full command | Action |
| --- | --- | --- |
| `:f` | `:fullscreen` | Expand or restore main in any view, including Overview, tables, details, Activity, Logs, and Editor. |
| `:L` | `:linenumbers` | Toggle line numbers in YAML, a value preview, or the editing document. |

Press **ff** during navigation to expand or restore without a colon or Enter. The first `f` appears as a pending shortcut in the footer; the second runs it. Text fields and search accept `ff` literally. Editable documents keep their own keys; use `:f` from Normal or Select. Read-only Kubernetes YAML supports `ff` and keeps `z` for selection.

Expansion focuses main and temporarily gives it the full width. It works from any navigation panel and stays active when you navigate between main views. Esc closes search or selection first, then restores the layout before leaving the view. In Editor Insert mode, Esc first returns to Normal. Alt+Left/Right restore the layout when switching panel focus, including in Editor Insert mode. Tab still indents in Insert. Chat commands also restore it. Help and Settings temporarily cover the expanded view and return to it. Close those overlays before changing expansion. Saved panel visibility stays unchanged; restoring does not reveal a side panel you had already hidden. In live Exec, enter output review with Ctrl+] before using these commands.

Log Viewer and Diff Reviewer show the visible line range within their retained content. Wrapped display rows count as their original source line; diff ranges refer to comparison rows, including hunk headers.

### Plous Editor and Diff Reviewer

Editing a resource opens the Editor in **NORMAL**. Its document keys take precedence over app double-key shortcuts. In **INSERT**, text and colons are literal. In **SELECT**, movements extend the selection. Read-only [YAML](#object-details) and [value inspection](#read-only-value-inspection) use the controls in their own sections. Simple forms and chat retain their existing controls.

Type **`:h`** / **`:help`** in Normal or Select for the editor reference in Help's center section. In Insert, press Esc first. Page Up/Down scrolls the reference; Esc returns to your draft without changing its cursor, selection, or undo history. Help follows the active editor, diagnostic, or review screen and names its actual save target. Read-only comparisons omit Save. `hh` belongs to document navigation while editing, so use the colon command there.

| Editor key | Action |
| --- | --- |
| Arrows or `h/j/k/l` | Move the cursor. |
| `w` / `b` / `e` | Next word / previous word / word end. |
| `0` / `$` or Home / End | Line start / end. |
| `gg` / `G` | Document start / end. |
| Page Up / Page Down | Move a page. |
| `i` / `a` / `o` | Insert here / after cursor / on a new line below. |
| `v` / `V` | Select characters / whole lines. |
| `x` / `dd` | Delete character or selection / whole line. |
| `y` in Select / `yy` in Normal | Copy selection / line. |
| `p` | Insert the most recent text copied or deleted with `y` or `d` in this editor. |
| `u` | Undo. |
| `U` (Shift+U) | Redo. |
| `>` / `<` | Indent / outdent current or selected lines. |
| `/`, then Enter | Find text and finish entering the query. |
| `n` / `N` | Next / previous match. |
| Esc in Insert or Select | Return to Normal. |
| Esc in Normal | Focus the editor action row. |
| Left / Right, Enter in actions | Choose and run an action. |
| Esc in actions | Request Close; changed documents offer Keep Editing / Discard Changes. |
| Tab in Insert | Insert two spaces. |
| Alt+Left / Alt+Right | Switch panels in every editor mode and retain the document. |

Bracketed paste is one literal undoable edit. Search, selection, syntax colors, line numbers, and screen wrapping do not alter document contents. The editor has no counts, macros, registers, or full Vim command language. In the focused editable document or its review, the commands below take precedence. Outside that workflow, `:q` exits Plous and `:w` exports resource YAML. `:quit` always exits Plous.

| Short command | Full command | Action in the active document workflow |
| --- | --- | --- |
| `:w` | | **Save**: validate and save to the named target, then stay in the editor. Works in the editor and optional review. |
| `:wq` | | **Save and close**: validate, write, and close after success. If unchanged, close without a request. |
| `:q` | | Close the document; changed text offers Keep Editing / Discard Changes. |
| `:q!` | | Discard the document and close without applying it. |
| `:v` | `:review` | **Review changes**: validate and open an optional comparison without saving. |
| `:u` | `:undo` | Undo in the editor. |
| `:U` | `:redo` | Redo in the editor. |

The target is the captured Kubernetes object, local source file, or Helm values draft shown by the editor. A draft save does not apply to the cluster. Use `:w` or the visible Save control to save directly with automatic validation. Failed saves preserve the draft in the editor; **!** in Normal or editor actions opens full error details. Ctrl+C exits without saving the draft.

Diff Reviewer uses arrows, page keys, Home/End, **[ / ]** for previous/next change, **/** search, **n/N** matches, and **v/y** select/copy. **a** focuses its action row. Enter activates an action only while that row is focused; navigation Enter never saves. The **`:w` Save** control is visible while reading. Review alone saves nothing. **Saved to Kubernetes** appears only after a successful save response. The Esc label names its next action: cancel/clear search, clear selection, restore layout, or **Back to editor**. Returning preserves the draft.

### Read-only value inspection

Open **Values** from resource details with `V` or `:V` / `:copyvalue`, then choose a key. Back starts selected; reading keys enter the document. Editing and pasted replacements are disabled.

| Key | Action |
| --- | --- |
| Arrows, Page Up / Page Down | Read the value. Left/Right chooses a button while actions are focused. |
| Home / End | Document start / end. |
| `/`, Enter, `n` / `N` | Find, finish entering the query, and move between matches. |
| `v` / `V`, `y` | Select text / lines and copy; `y` alone copies the current line. |
| `a` | Switch between reading and actions. |
| Enter in actions | Activate Back, Copy value, or Export PEM where available. |
| `:f` / `:fullscreen` | Expand / restore the value view. |
| `:L` / `:linenumbers` | Toggle line numbers. |
| Esc | Clear search or selection, restore expansion, then hide the value. |

Global commands and double-key shortcuts remain available outside search input. Copy value copies the entire value; hiding it clears the preview buffer.

### Containers and logs

| Context | Key | Action |
| --- | --- | --- |
| Pod Describe or Logs | `o` | Choose container, including init and ephemeral containers. Logs also offers All application containers. For workloads, filter across Pods by container name. |
| Pod Describe | `p` | Open the selected container's previous logs. |
| Logs | `p` | Previous instance: show logs from the selected container before its last restart. |
| Logs | Space | Hold your place / jump to latest. Collection continues while reading. |
| Logs | `g` | Reload logs, clearing and fetching the selected history again. |
| Logs | `w` | Wrap lines: split long lines across screen rows. |
| Logs | `u` | Time range: request the latest 200 lines or a recent time window. |
| Logs | `t` | Toggle timestamps. |
| Logs | `ff` | Fullscreen / restore layout. `:f` / `:fullscreen` also work. |
| Logs | `/` | Find text in the loaded buffer and pause following. |
| Logs | `n` / `N` | Next / previous retained match. |
| Logs | `v` / `y` | Select / copy selection or current line. |
| Logs | Esc | Clear search or selection before returning to the originating detail. |

In **Exec**, normal input, Esc, Tab, and colons go to the container. **Ctrl+]** enters **Plous controls active** for output review while the remote process continues. **Alt+Left/Right** move panels directly and resume terminal input on return. **Ctrl+C exits Plous** in either mode.

| Key in Exec review | Action |
| --- | --- |
| Up / Down or `k` / `j` | Scroll retained output. |
| Page Up / Page Down | Scroll one page. |
| Home | Go to oldest retained output. |
| End | Refresh the review with latest output. |
| `/`, `n` / `N` | Search retained output and step through matches. |
| `v` / `y` | Select / copy selection or current line. |
| `r` | Resume terminal input. |
| `t` | Send Ctrl+C to interrupt the remote process. |
| `x` or Esc | Close and return to resource details. |

Finished sessions keep output for review; Resume and Interrupt are unavailable. **e** opens the command editor with shell choices, and **R** explicitly retries. The launcher uses Up/Down and Enter to select sh, bash, or Custom command, then Start. Escape from a field stops editing; Escape from the launcher returns to the previous view.

## Cluster Overview

Select a Monitor connection and use **11** or **:1**. The [Overview guide](../guides/overview.md) explains scope and availability.

| Key | Command | Action |
| --- | --- | --- |
| Left / Right | | Move across Pulse tiles or between lower columns, without wrapping. |
| Up / Down, j / k | | Move through rows and cross into the section above or below at list edges. Down from Pulse enters the column below; Up from its first row returns above. |
| Page Up / Page Down, Home / End | | Move through the section's items. |
| Enter | | CPU and Memory open focused metric details. Nodes, Pods, and Autoscaling open resource lists. Other rows inspect resources or releases, grouped warnings open members, and Snapshot counters open their lists. Previous change records and Availability open evidence. |
| Up / Down or j / k in Snapshot | | Move through integration headings and their counters; continue at the edges to adjacent cards or sections. |
| e | `:e` / `:expandsection` | Expand or restore the selected section. |
| g | `:g` / `:refresh` | Refresh discovery and cluster observations. |
| / | | Filter Overview lists. Search text and paste remain literal. |
| Esc | | Clear search, return from a warning group or Snapshot counter list, cancel pending inspection, restore section expansion, then go back. |

Clicking a Pulse tile performs the same action as selecting it and pressing Enter. In CPU and Memory details, Enter opens Source details; Esc clears local search or selection, then returns from sources to the chart, then to Overview. Arrows and page keys scroll, Home/End reach the first/last row, and **/** finds text with **n/N** moving between matches. Coverage and relevant warnings remain in the chart view. **g** refreshes and **ff** expands main; **e** is only for Overview sections.

Alt+Left/Right switch the outer panels. **ff** keeps its global fullscreen behavior, and **hh** opens contextual Help. Arrows follow the displayed layout. When Attention is above Snapshot, Down from its last row enters Snapshot and Up from Snapshot’s first row returns to Attention. In compact or expanded views, Up/Down at section edges moves through Pulse, Deployments, Attention, and Snapshot in order. Expanded Pulse uses Up/Down between tile rows. Left/Right never moves down into another row or section. Snapshot counters use Up/Down; Tab switches between the outer tree and main panels. Section selection and scroll positions survive refresh and resizing when possible.

## Main-panel lists and Activity

| Context | Key | Action |
| --- | --- | --- |
| Settings / tool lists | Up / Down, Enter | Select and activate. |
| Empty workspace Overview | Enter | Add Connection. |
| Context setup | Up / Down | Select path, context, or action. |
| Context row | Space or Enter | Toggle its checkbox. |
| Available context row | Right or click Name | Set an optional connection display name. Use name returns to the picker; adding still requires Add Selected Contexts. |
| Kubeconfig Path | Enter | Move to Refresh Contexts; another Enter loads it. |
| Context setup outside path input | Home / End | Go to path field / Cancel. |
| Context setup | Enter on an action | Refresh Contexts, add checked contexts, or cancel. |
| Activity list | Up / Down, Page Up / Page Down, Home / End | Select an entry and update its preview. |
| Activity list | Enter | Open full details. |
| Activity list | Left / Right | Cycle All activity, Plous actions, Workspace changes, and Observed Helm history. Left selects the previous type. |
| Activity list | `g` or `:g` / `:refresh` | Reload local history. |
| Activity load failure | `d` or `:d` / `:describe` | Read the complete load error. |
| Helm Activity detail | Enter | Open the recorded release when its saved connection still matches. |
| Observed Helm detail | `O` or `:O` / `:observations` | Show or hide original status observations. |
| Activity detail | Arrows, Page Up / Page Down, Home / End | Scroll or go to the first / last page. |
| Workspace Activity detail | `d` or `:v` / `:review` | Open a read-only comparison of saved snapshots. |
| Activity detail | Esc | Return to the Activity list with its search, type, and selection preserved. |
| Activity comparison | `[` / `]`, `/`, `v`, `y` | Navigate changes, search, select, and copy. |
| Activity comparison | Esc | Close search/selection first, then return to the summary. |
| Tool setup | Esc | Return one setup step. |

## Forms

| Key | Action |
| --- | --- |
| Up / Down | Move between fields and actions. |
| Enter in a field | Advance, checking required values. |
| Enter on an action | Activate Save, Cancel, or the named action. |
| Left / Right in a choice field | Change the choice. |
| Esc | Cancel unsaved edits and return to the preceding setup step or Settings. |
| `:` or double-key shortcut on an action button | Run navigation without applying the form. Commands cannot discard unfinished edits. |
| Alt+Left / Alt+Right | Leave and resume Settings without submitting or discarding edits. |

Text and paste in fields remain text. An explicit chat command from an action button can preserve an unfinished form while you use the assistant.

## AI assistant

With Claude Code or Codex selected, the right panel hosts its native terminal interface:

| Key or command | Result |
| --- | --- |
| **cc** / `:c` | Open, resume or enter the native terminal |
| Alt+Left / Alt+Right | Move directly to the previous / next panel; returning resumes native input |
| Ctrl+] | Toggle focus between the agent and previous panel; preserve the running session |
| Wheel over the native panel / Page Up / Page Down | Read retained normal-screen terminal output; fullscreen CLI views handle their own scrolling |
| End / Esc while reading terminal output | Return to live output; typing or paste also returns live |
| Tab, Esc, arrows, slash commands and typing while captured | Forward to the native CLI |
| `:X` after leaving the agent panel | Close the native session and revoke its tools |
| `:C` after closing the session | Open a new native conversation in the selected scope |
| Ctrl+C during native input | Forward to the CLI for its native interrupt behavior; Plous stays open |
| Ctrl+] then Ctrl+C | Return to tree/main, then save and exit Plous |

Ctrl+C is interpreted by the CLI: while idle it may clear input or exit that CLI. `:X` closes the whole native session. Use the native CLI's `/resume` for its saved conversations. Normal-screen terminal scrollback retains up to 5,000 history rows in memory and holds the reading position during output and resizing. Scrolling does not recall prompts or change keyboard focus. Changing focus preserves the screen and process. The panel heading keeps the session's captured scope while you browse elsewhere. Click another panel to move focus while the native process continues. Click the native panel to resume input, including after Ctrl+] moved focus away. Fullscreen CLI views receive wheel events only when they enable mouse input; clicks remain Plous panel controls. The following chat controls apply to **Plous**:

Use **cc** or `:c` to enter typing; Esc leaves it and keeps the draft. Chat commands work before workspace creation as well as within workspaces.

| Key or input | Action |
| --- | --- |
| Enter while typing | Send the draft. |
| `[` / `]` during navigation | Select the previous / next reply and bring it into view. |
| Enter during navigation | Open the selected reply's Sources or Reply details. |
| Shift+Enter | Insert a newline. |
| Ctrl+J | Newline fallback for terminals without distinct Shift+Enter. |
| `/model` or `/m`, then Enter | With Plous selected, open the model picker when freshly typed as the entire draft. |
| `//model` or `//m`, then Enter | With Plous selected, send literal command text. |
| `:p` / `:restoreprompt` | Restore the last sent message into an empty draft. Does not send it. |
| Up / Down or k / j during navigation | Scroll the conversation one line. |
| Page Up / Page Down | Scroll a page, including while typing. |
| Home during navigation | Go to the oldest message. |
| End during navigation | Go to the latest reply and follow new output. |
| Esc | Leave typing or close history, preserving the draft and running reply. |
| Up / Down, Enter in history | Select and open a conversation. |
| `/` during navigation | Find conversation text, or filter history. |

Reading older messages keeps your position steady as replies stream. Home/End moves the draft caret while typing. In the model picker, arrows select, Enter applies, Esc cancels, and Home/End moves to the first entry or Manage in Settings. Selection is application-wide. Pasted and restored drafts are literal, including `/model`. See [AI chat](../guides/ai-chat.md) for setup and conversation commands.

Use `:A` / `:readreply` for the selected finished answer in the wider Markdown viewer. `/` searches, `v` selects, `y` copies, `Y` copies Markdown source, and `R` switches rendered/source text. Enter opens Sources for that answer. Esc clears local selection or search before returning; the draft and reading position are preserved.

In Sources, arrows choose a source and Enter opens its details. One source skips the list. Enter in details opens the saved result in a read-only viewer: `/` finds text, `v` selects, and `y` copies. Arrows and page keys scroll details. Esc closes search or selection first, then returns through source details and the list to the same reply. Alt+Left/Right leave and resume Sources with its reading position intact. **hh** opens Help; **cc** returns to your draft.

## Help

Open with **hh** or `:h`. Left/Right selects a top section; Up/Down and Page Up/Page Down scroll it; Home/End jumps to its start/end. The global section stays visible below. Alt+Left/Right pause Help and restore it when its panel regains focus. Esc closes Help and restores the previous view, including an unfinished Settings form.

## Text editing

| Key | Action in a text field, command box, or chat draft |
| --- | --- |
| Left / Right | Move the caret one character. |
| Home / End | Beginning / end of the whole input. |
| Backspace / Delete | Delete before / at the caret. |
| Ctrl+H | Backspace compatibility in text input for terminals that send the BS byte. |
| Ctrl+A / Ctrl+E | Beginning / end of the whole input. |
| Ctrl+U | Delete text before the caret. |

Chat Home/End operates on the whole draft; Up/Down does not move between its lines. Plous Editor has its own document modes and line-based controls. Paste edits fields and chat without running commands or sending messages; navigation and the command box ignore paste.

The BS form of Backspace also works in searches and Editor Insert mode. Ctrl+H has no navigation action. Captured terminals receive it unchanged.

## Cancel and exit

Esc closes one layer at a time: pending shortcut, command/search/editor, picker or operation, detail, then root focus. It does not erase applied table filters or stop chat replies. Use `:X` for a running reply and Backspace for an applied table text filter.

`:quit` exits during navigation. `:q` does the same except in the focused editable document, where it requests closing that document. Ctrl+C saves and exits outside captured native agent input, including forms, Help, small-terminal fallback, and captured Exec input. In an active Claude Code or Codex terminal, Ctrl+C goes to that CLI; press Ctrl+] first to return to Plous navigation before exiting. Exiting does not save unfinished form edits or undo submitted cluster operations.

Below 80 columns by 16 rows, most views offer only restricted navigation and exit controls. Cluster Overview additionally supports a minimal view from 40 columns by 12 rows, including its metric details and temporary resource lists. Direct Overview links to individual resources use recorded evidence below 80 columns. Alt+Left/Right can focus the sidebar or assistant at these widths. Resize the terminal to restore the full interface.

## Helm Explore

Each Helm connection contains **Releases** and **Charts**. Select these in the sidebar with Up/Down and Enter. Left/Right folds sidebar branches and switches sections inside an opened release. Arrow keys navigate chart cards. Enter opens an item. `/` filters the release inventory; Backspace clears an applied filter. `g` refreshes the selected view.

| Short command | Full spelling | Action |
| --- | --- | --- |
| `:B` | `:rollback` | Preview rollback to an older selected revision. From a prepared rollback, open confirmation. |
| `:J` | `:release` | Open the chart inspector's release dropdown; selection loads its latest revision. |
| `:F` | `:revision` | Open the chart inspector's recorded revision dropdown. |

Press `n` or `:N` to open the searchable namespace picker. Choose **All namespaces**, select a discovered name, or enter one manually when discovery is unavailable. Esc from manual entry returns to the picker; another Esc restores the view. `:N <name>` selects a scope directly; `:N *` lists all namespaces. `S` opens table sorting; select the same column again to reverse the order.

Releases opens a revision timeline, newest first. Up/Down selects revisions; Page Up/Page Down scrolls within the selected block, keeping its revision visible. `v` opens full recorded changes, or the complete object summary when the diff is too large; Enter opens that revision in the chart inspector. Charts shows groups of release cards, navigated with arrows. In the inspector, `J` chooses a release, `F` chooses a revision, and Left/Right switches Resources, Values, and Chart. The Release and Revision pickers accept search and mark the current selection. Enter in Resources opens full configuration; Enter in Values or Chart chooses the displayed document. `/` finds text in place, including loaded timeline diffs; Enter finishes typing and `n`/`N` moves between matches. Esc clears search before returning to the originating timeline or list.

Helm also uses `:g` / `:refresh`, `:N` / `:namespace`, `:a` / `:actions`, and `:v` / `:review`. These operate on the selected Helm connection. Enter in a comparison never applies a rollback.

In the Helm values editor, `:w` validates and saves values to the **draft**; `:wq` also closes the editor. `:v` reviews changes to draft values. `:q` closes with an unsaved-change prompt and `:q!` discards unsaved editor changes. These saves do not change the cluster or source files.

## Argo and Flux

Expand **Argo** or **Flux** under a Monitor connection in Current Workspace or a saved workspace and select a category. These read-only Explore tables show a compact selection preview; Enter opens full details.

| Key | Command | Action |
| --- | --- | --- |
| Up/Down | | Select a list row or a link in details. |
| Enter | | Open details or the selected resource, source object, or dependency. |
| Esc | | Return through the detail trail to the list. |
| / | | Filter the list; Enter opens a match. Find text in details. |
| Page Up/Page Down | | Scroll details. |
| End, Enter | | Select and expand Configuration & revisions in details. |
| Backspace | | Clear the applied list filter. |
| n | `:N` / `:namespace` | Choose the namespace of the controller objects. |
| g | `:g` / `:refresh` | Refresh the current view. |

See [Argo and Flux](../integrations/gitops/README.md) for status meanings and destination access.

## Kustomize Explore

`G` opens the selected Kustomize node’s graph, `r` renders, and `d` opens its source. These single keys act in the focused Code view. `:G` / `:graph` and `:r` / `:render` remain available as colon commands. `g` / `:g` / `:refresh` still refreshes the source.

Each **Code → source** has **Files**, **Helm**, **Kustomize**, **Argo**, and **Flux** in the sidebar. Render opens from supported configurations in the technology browsers. Kustomize opens a group directory before its dependency graphs. Formats are detected automatically from the source files. Monitor and Code are sidebar sections; the main tabs remain Overview, Explore, and Activity.

| Key | Command | Action |
| --- | --- | --- |
| Enter | | Toggle a Files folder, open a file in Plous Editor, open a Kustomize group or node overview, inspect rendered resources, or retry a failed source load. |
| Up/Down, Enter | | In the sidebar: select Files or a technology browser. |
| Left/Right | | In Files: collapse/expand folders or move to the parent/first child. In a Kustomize graph: follow a consumer / included node. In Render: choose an area inside main. |
| Up/Down, Page Up/Down | | Select a source, resource, or YAML field in the active area; scroll entry, finding, or field-history details. |
| Home / End | | Jump to the first/last list item, or the start/end of focused entry or finding details. |
| g | `:g` / `:refresh` | Capture current local files or resolve the selected remote revision again. |
| r | `:r` / `:render` | Render the selected configuration in a technology view. Kustomize renders one selected overlay; shared bases use Actions → Render Base Only. |
| v | `:v` / `:review` | Compare Entry Points; review a local document when editing. |
| a | `:a` / `:actions` | Inputs, captured stages, validation, source files, editing, comparisons, revisions, local copy, diagnostics. |
| d | `:d` / `:describe` | Open a verified resource source; in Field History, open the field's captured source. |
| y | `:y` / `:yaml` | Open a rendered resource. |
| F | `:F` / `:revision` | Choose a remote Git Revision. |
| / | | Filter the current Code list; find text in a node overview or document. |
| Backspace, Esc | | Clear the applied Code list filter. |
| Esc | | Cancel work or close a local layer, then return to the sidebar. |

Local source documents use the [Editor controls](#plous-editor-and-diff-reviewer). Saves validate applicable syntax and recheck the original file identity and bytes. Files opens local text for editing and remote snapshots read-only. Unsaved buffers stay in memory. In a read-only local preview, **a → Edit Selected File** opens the editor.

Kustomize starts at its group directory. Enter opens a graph, then a node overview. **G** opens relationships, **r** renders, and **d** opens source. **Esc** returns to the overview, group graph and directory with selection retained. **a → Choose Group** returns to the directory. **View output** opens the last result without rebuilding. Other technology browsers retain contextual Graph and Output controls.

In rendered output, Left/Right chooses Resources or YAML. Enter or a field click opens Field History; Left/Right switches its areas, Up/Down and page keys scroll, and **d** opens an available captured source or observed writer configuration. Esc returns through nested details to the original resource. A chart included by Kustomize offers a chart-only or including-Kustomize render choice.

The **Actions** menu contains the remaining contextual controls:

| Menu location | Controls |
| --- | --- |
| View Findings | Selected-entry findings; nested inspection returns to the same source, category and position |
| Findings → Actions | Findings, Images, Generators, Filter Category, Filter Source |
| More Tools | Repository Findings, Validate All Entry Points, Captured Stages, Export Rendered YAML |
| Show Final YAML / Show Changes | Output or comparison after saving an already rendered input |
| Comparison Options | Full reviewer, direction, layout, changed-line controls |
| Field History | Trace Across All Overlays using the captured snapshot and field |

The local setup form's **Browse Folders** picker uses Up/Down to select, Enter to open or use a directory, Left for parent, `/` for path input, and Esc to return to the form.

See [Kubernetes Code](../integrations/kustomize/README.md) for engine support, input limits, trace coverage, comparison meaning and failure handling.

## Usage charts

| Short command | Full spelling | Action |
| --- | --- | --- |
| `:I` | `:charts` | Open or close usage charts or top consumers. |
| `:W` | `:chartrange` | Switch between 5 and 15 minutes. |
| `:P` | `:chartlatest` | Follow the latest sample. |

In Pod, Node, or workload Describe views, **I** opens chart inspection. In Pod or Node lists, **I** opens the top ten CPU and memory consumers within the current filters. **I** or **Esc** returns without changing the underlying list or detail scroll. Rankings are read-only; Enter and resource-table actions are inactive until you return to the list.

In time-series charts, **Left / Right** selects actual samples, **W** switches 5 / 15 minutes, and **P** follows the latest sample. Clicking a plotted sample selects it. The same observation stays selected as new measurements arrive or the view resizes. Pod and Node points use source time; workload snapshots use collection time and show their source timestamp range. If it leaves the visible range, Plous reports that and follows latest. **Up / Down**, **PgUp / PgDn**, and **Home / End** scroll. **ff** expands the view; Esc leaves chart inspection before restoring fullscreen. Alt+Left/Right still switch panels. Help and Settings preserve the selection. An existing Describe search is suspended during inspection and restored on return. **/** returns to Describe and resumes that query, or opens a new search when none exists.
