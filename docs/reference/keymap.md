# Keymap

[Documentation](../README.md) · [Learn the interface](../guides/navigation.md)

Navigation means you are outside a text field, search, or composer. Ordinary keys affect the focused panel. Type a colon command and press Enter to run it.

Commands are case-sensitive. `:c` opens chat and `:C` starts a new conversation.

Jump to [commands](#commands), [Kubernetes](#kubernetes-explore), [Helm](#helm-explore), [Editor](#plous-editor-and-diff-reviewer), [logs and Exec](#containers-and-logs), or [AI](#ai-assistant).

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

## Double-key shortcuts

Press the same key twice consecutively. The second key runs the shortcut without Enter or a timing deadline.

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

The footer shows a pending first key. Esc, another input, a focus change, resize, or paste clears it. A different reserved key starts its own sequence. Single numbers do not switch views.

Fields accept these keys as text. Editable documents keep their own keys. Navigation cannot discard an unfinished form.

## Commands

These commands work from navigation. In a text field, colon is ordinary text.

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

Esc cancels the command box. Alt+Left / Alt+Right leaves and resumes its draft. A new colon or completed double-key shortcut from another panel replaces it. Pasted commands are ignored.

## Switch panels

Alt+Right cycles sidebar → main → assistant. Alt+Left reverses the order. Hidden panels are skipped. These keys work in forms, editors, overlays, and captured terminals.

Tab switches tree and main during navigation. Ctrl+] handles the agent separately. Drafts and running sessions stay intact.

On macOS, use Option+Left / Option+Right. Plous accepts Alt and Meta arrow encodings, Kitty variants, and Escape+b/f. Alt+B/F therefore also switch panels. A terminal binding that consumes the shortcut must be changed in the terminal.

### Show or hide panels

Use `bb` for the sidebar and `ii` to cycle the assistant through normal, double width, and hidden. Hiding the focused panel moves focus to main. Visibility and assistant size survive restart.

Ctrl+] reveals the agent. `ff` expands main temporarily. Leave a native terminal with Ctrl+] before using navigation shortcuts.

## Search the focused panel

| View | `/` behavior | Finish or clear |
| --- | --- | --- |
| Workspace tree | Filter names, including collapsed paths in loaded resource catalogs. | Arrows select. Enter opens. Esc restores the unfiltered tree. |
| Kubernetes table | Edit the row text filter. | Enter applies. Esc cancels the edit. Backspace clears an applied filter while navigating rows. |
| Activity list / chat history | Filter displayed entries. | Arrows select. Enter opens. Esc restores the list. |
| Cluster Overview | Filter the lower lists. | Enter inspects a match. Esc clears the query. |
| Workspace summary, connection settings, details, logs, conversation | Find and highlight text. Conversation find also searches unloaded saved messages. | Enter finishes query input. N/N next/previous match. Esc clears find. |

Backspace clears only an applied table text filter. Namespace, labels, sorting, and selection remain. In input, Backspace deletes text.

Search can be left and resumed with Alt+Left / Alt+Right. Log search pauses following. Close it, then press Space to resume.

## Mouse controls

Mouse support is on by default. Change it under **Settings → Application → Mouse support**.

- Click a panel to focus it. Click a row to select and double-click to open.
- Click tabs, buttons, and actions to activate them. Save uses the same checks as keyboard Save.
- Click a field to place its cursor. Drag selects text, and double-click selects a word in fields and viewers.
- Wheel scrolls the view under the pointer without discarding edits.
- Clicking another panel leaves a native agent running. Clicking its panel resumes input.

Logs keep collecting while you scroll. Use **Jump live** to follow again. Exec suspends mouse reporting while it captures input.

Horizontal scrolling, hover, right-click menus, dragging panel sizes, and forwarding clicks into Exec are unavailable. Turn mouse support off for native terminal selection.

| Target | Action |
| --- | --- |
| Panel background or title | Focus that panel. |
| Tabs, buttons, named footer actions, menus, and Settings entries | Click to activate. |
| Sidebar label | Click to select or open. Click its triangle to fold or expand only. |
| Resource rows, Helm cards and revisions, Activity, conversation history, and Sources lists | Click to select. Double click to open. |
| Table heading or namespace control | Click to sort or choose scope. |
| Field, search box, command box, or composer | Click text to place the cursor. Drag to select. Double click to select a word. Typing replaces a field selection. Labels and borders focus without submitting. |
| Editor or shared viewer text | Click to position. Drag to select. Double click to select a word. Click **Copy selection** in the footer or use `y`. |
| Tables, labels, Help, chat transcript, and native agent output | Drag to select visible text, then click **Copy selection** in the footer. A drag does not activate the control underneath it. |
| Option value or context checkbox | Click to change the pending value. Save remains explicit. |
| Reply and its Sources hint | Click the reply to select it. Click Sources to open saved evidence. |
| Scrollable content | Wheel scrolls under the pointer without changing keyboard focus. |
| Help section | Click to select. Wheel scrolls that section. |

## Copy and paste

For a Plous selection, drag over text and click **Copy selection** in the footer. Viewers also support `y`. If the footer itself is selected, the copy control appears in the top bar.

Terminal Copy uses the terminal's own selection. macOS commonly uses ⌘C / ⌘V, and GNOME Terminal uses Ctrl+Shift+C / Ctrl+Shift+V. Turn Plous mouse support off to use native selection everywhere.

Plous also accepts these shortcuts when the terminal forwards them. Copy needs a selection. Paste inserts text without sending or running it. Navigation and the command box ignore paste. Native terminals and Exec receive terminal paste.

Screen selections include visible wrapping and table formatting. Text-aware viewers copy source text without gutters. Masked credential fields do not expose their contents.

Esc clears selection. Typing, paste, scrolling, focus changes, resizing, or changed selected content also clear a screen selection.

Local clipboard support uses macOS tools or `wl-copy` / `wl-paste`, `xclip`, or `xsel` on Linux. If unavailable, use terminal Copy/Paste. SSH and Mosh use terminal copying rather than the remote desktop clipboard.

Ctrl+C is not Copy. It goes to the active native agent, or saves and exits Plous elsewhere.

## Sidebar refresh

Press `g`, `:g`, or `:refresh` on a cluster or tool branch to rediscover APIs and Helm metadata. In an Argo CD or Flux main view, `g` refreshes that view.

## Workspace tree

| Key | Action |
| --- | --- |
| Up / Down | Previous / next row. |
| Home / End | First / last row. |
| Page Up / Page Down | Move ten rows. |
| Right | Expand a branch without changing the active scope or main view. |
| Left | Collapse an expanded branch, otherwise move to its parent. |
| Enter | Select workspace/tool/connection and toggle its branch. Toggle a category. Open a resource. Activate a setup/Settings row. |

Enter selects a workspace or connection and toggles its branch. Left / Right only folds branches. Opening a resource selects Explore and focuses its table. Search results expand their path.

## Cluster Overview

Select a cluster connection and use `11` or `:1`. See [Overview](../guides/overview.md) for data meanings.

| Key | Command | Action |
| --- | --- | --- |
| Left / Right | | Move across Pulse tiles or between lower columns, without wrapping. |
| Up / Down, j / k | | Move through rows and cross into the section above or below at list edges. Down from Pulse enters the column below. Up from its first row returns above. |
| Page Up / Page Down, Home / End | | Move through the section's items. |
| Enter | | Open the selected metric, resource, release, warning group, inventory, or evidence. |
| Up / Down or j / k in Snapshot | | Move through integration headings and their counters. Continue at the edges to adjacent cards or sections. |
| e | `:e` / `:expandsection` | Expand or restore the selected section. |
| g | `:g` / `:refresh` | Refresh discovery and cluster observations. |
| / | | Filter Overview lists. Search text and paste remain literal. |
| Esc | | Clear search, return from a warning group or Snapshot counter list, cancel pending inspection, restore section expansion, then go back. |

In metric details, Enter opens Source details. Esc returns through search or selection, Sources, the chart, then Overview. `g` refreshes and `ff` expands main.

Arrows follow the displayed rows and columns. Up / Down moves between sections at their edges. Selection and scroll are kept where possible.

## Kubernetes Explore



### Resource commands

Resource commands open Explore from any navigation panel. If needed, a connection picker opens first. In Current Workspace, `:K` searches kubeconfig contexts. In a saved workspace, it searches saved connections.

| Resource or control | Commands |
| --- | --- |
| Connection picker | `:K`, `:connections` |
| Namespace picker | `:N`, `:namespace` |
| Set namespace directly | `:N shop`, `:namespace shop`. Use `*` for all namespaces. |
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

Discovered names and unambiguous abbreviations work. Qualify ambiguous types, such as `:widgets.example.io`.

`:N` chooses a namespace. `:ns` opens Namespaces. Resource commands accept a namespace, such as `:pods shop` or `:pods *`. Only `*` means all namespaces.

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
| `S` | Choose a sort column. Choosing the current column reverses it. |
| `g` | Reconnect and rediscover APIs. `:g` / `:refresh` also works. |
| Esc | Return to the previous related view, or reveal and focus the sidebar at the root. |

Click the resource heading or press Esc from a root table to reveal the sidebar. Use sidebar search to choose a resource type. `o` is a Pod container chooser, not a table action.

In pickers, type to search, use arrows and Enter to choose, or Esc to cancel. The namespace picker offers Reload namespaces and manual entry. Alt+Left / Alt+Right leaves and resumes a picker.

### Object details

| Key | Command | Action |
| --- | --- | --- |
| `d` | `:d` / `:describe` | Describe. |
| `y` | `:y` / `:yaml` | Open YAML. Within YAML, `y` copies text. |
| `e` | | Events. |
| `r` | | Related resources. Enter follows a selected relationship. |
| `[` / `]` | | Previous / next detail section. |
| `/` | | Find text in the section. |
| | `:g` / `:refresh` | Refresh the object and section. |
| `v` in Secret YAML | | Reveal / hide values for this view. |
| Esc | | Return to the preceding resource or list. |

In read-only YAML, `/` searches, `n/N` selects matches, `z` selects text, and `y` copies. Secret YAML uses `v` to reveal or hide values.

Esc clears search or selection, then returns to the view that opened YAML. Lowercase `e` opens Events. Uppercase `E` edits.

### Detail actions

Open resource details before running an action. Availability depends on the resource API and permissions.

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

Case matters: `D/E/R` perform actions, while `d/e/r` open sections.

After Scale, Restart, or Delete, Enter or Esc returns to details. `g` checks progress again without repeating the change. Confirmations use Left / Right and Enter.

See [resource actions](../integrations/kubernetes/README.md#resource-actions).

### Expand a view

| Short command | Full command | Action |
| --- | --- | --- |
| `:f` | `:fullscreen` | Expand or restore main in any view, including Overview, tables, details, Activity, Logs, and Editor. |
| `:L` | `:linenumbers` | Toggle line numbers in YAML, a value preview, or the editing document. |

`ff` expands main during navigation. In an editable document, use `:f` from Normal or Select. Text input keeps literal keys.

Expansion stays while navigating within main. Esc handles search, selection, and editor mode before restoring layout. Panel focus changes and explicit panel toggles also restore it.

Help and Settings preserve expansion underneath. Saved panel visibility does not change. In live Exec, enter review with Ctrl+] first.

### Plous Editor and Diff Reviewer

The editor starts in NORMAL. INSERT accepts literal text and colons. SELECT extends the selection. Document keys take precedence over app double-key shortcuts.

Use `:h` for editor Help. In Insert, press Esc first. Help preserves the draft, cursor, selection, and undo history.

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
| Esc in actions | Request Close. Changed documents offer Keep Editing / Discard Changes. |
| Tab in Insert | Insert two spaces. |
| Alt+Left / Alt+Right | Switch panels in every editor mode and retain the document. |

| Short command | Full command | Action in the active document workflow |
| --- | --- | --- |
| `:w` | | **Save**: validate and save to the named target, then stay in the editor. Works in the editor and optional review. |
| `:wq` | | **Save and close**: validate, write, and close after success. If unchanged, close without a request. |
| `:q` | | Close the document. Changed text offers Keep Editing / Discard Changes. |
| `:q!` | | Discard the document and close without applying it. |
| `:v` | `:review` | **Review changes**: validate and open an optional comparison without saving. |
| `:u` | `:undo` | Undo in the editor. |
| `:U` | `:redo` | Redo in the editor. |

Paste is one literal, undoable edit. The editor does not implement full Vim counts, macros, or registers.

Kubernetes Save writes to the cluster after validation. Helm values Save updates the preview draft only. Failed saves keep the draft. `!` opens the error. Ctrl+C exits without applying it.

Diff Reviewer uses arrows, page keys, `[` / `]` for changes, `/` and `n/N` for search, and `v/y` for selection/copy. `a` focuses its actions. Enter saves only when the Save action is focused. Use `:w` for explicit Save.

Review alone saves nothing. Esc returns to the editor with the draft intact. **Saved to Kubernetes** appears only after success.

### Read-only value inspection

Press `V` or `:V` in resource details, then choose a value. Editing and replacement paste are disabled.

| Key | Action |
| --- | --- |
| Arrows, Page Up / Page Down | Read the value. Left/Right chooses a button while actions are focused. |
| Home / End | Document start / end. |
| `/`, Enter, `n` / `N` | Find, finish entering the query, and move between matches. |
| `v` / `V`, `y` | Select text / lines and copy. `y` alone copies the current line. |
| `a` | Switch between reading and actions. |
| Enter in actions | Activate Back, Copy value, or Export PEM where available. |
| `:f` / `:fullscreen` | Expand / restore the value view. |
| `:L` / `:linenumbers` | Toggle line numbers. |
| Esc | Clear search or selection, restore expansion, then hide the value. |

Copy value copies the whole value. Hiding it clears the preview. Global navigation remains available outside search input.

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

Press Ctrl+] in Exec for output review:

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

In Exec, input, Esc, Tab, colons, and paste go to the container. Ctrl+] enters output review while the process continues. Alt+Left / Alt+Right moves panels and resumes input on return. Ctrl+C exits Plous in either Exec mode.

Finished sessions keep output. `e` edits the command and `R` retries explicitly. Closing does not guarantee remote process termination.

## Usage charts

Use `I` in Pod, Node, or workload Describe for chart inspection. In Pod or Node lists, it shows the top ten CPU and memory consumers within current filters.

| Short command | Full spelling | Action |
| --- | --- | --- |
| `:I` | `:charts` | Open or close usage charts or top consumers. |
| `:W` | `:chartrange` | Switch between 5 and 15 minutes. |
| `:P` | `:chartlatest` | Follow the latest sample. |

Left / Right selects samples, `W` chooses 5 or 15 minutes, and `P` follows latest. Clicking a sample selects it. Selection survives new data until it leaves the visible range.

Use arrows and page keys to scroll. `ff` expands. Esc leaves inspection before restoring fullscreen. `/` returns to Describe search.

Top-consumer rankings are read-only. Press `I` or Esc to return to the list before opening a resource.

## Helm Explore

Choose Releases or Charts in the sidebar. Enter opens an item. `/` filters, Backspace clears the filter, `S` sorts, and `g` refreshes.

In a release, Up / Down selects a revision, page keys scroll its block, `v` opens changes, and Enter inspects the revision. Left / Right switches Resources, Values, and Chart.

Use `n` or `:N` for namespaces. `:N *` selects all. `J` and `F` open searchable release and revision pickers.

| Short command | Full spelling | Action |
| --- | --- | --- |
| `:B` | `:rollback` | Preview rollback to an older selected revision. From a prepared rollback, open confirmation. |
| `:J` | `:release` | Open the chart inspector's release dropdown. Selection loads its latest revision. |
| `:F` | `:revision` | Open the chart inspector's recorded revision dropdown. |

The values editor saves to a draft with `:w` or `:wq`. `:v` reviews without saving, and `:q!` discards unsaved text. These actions do not change the cluster or source files.

Enter in a comparison does not execute rollback. See [Helm](../integrations/helm/README.md).

## Argo and Flux

Expand Argo or Flux under a cluster and choose a category. These views are read-only. Enter opens full details.

| Key | Command | Action |
| --- | --- | --- |
| Up/Down | | Select a list row or a link in details. |
| Enter | | Open details or the selected resource, source object, or dependency. |
| Esc | | Return through the detail trail to the list. |
| / | | Filter the list. Enter opens a match. Find text in details. |
| Page Up/Page Down | | Scroll details. |
| End, Enter | | Select and expand Configuration & revisions in details. |
| Backspace | | Clear the applied list filter. |
| n | `:N` / `:namespace` | Choose the namespace of the controller objects. |
| g | `:g` / `:refresh` | Refresh the current view. |

See [Argo CD](../integrations/argo/README.md) and [Flux](../integrations/flux/README.md) for status meanings and destination access.

## Main-panel lists and Activity

| Context | Key | Action |
| --- | --- | --- |
| Settings / tool lists | Up / Down, Enter | Select and activate. |
| Empty workspace Overview | Enter | Add Connection. |
| Context setup | Up / Down | Select path, context, or action. |
| Context row | Space or Enter | Toggle its checkbox. |
| Available context row | Right or click Name | Set an optional connection display name. Use name returns to the picker. Adding still requires Add Selected Contexts. |
| Kubeconfig Path | Enter | Move to Refresh Contexts. Another Enter loads it. |
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

Fields and paste stay literal. Chat can preserve an unfinished form while you use the assistant.

## AI assistant

For Claude Code or Codex, the right panel contains the CLI's native terminal:

| Key or command | Result |
| --- | --- |
| **cc** / `:c` | Open, resume or enter the native terminal |
| Alt+Left / Alt+Right | Move directly to the previous / next panel. Returning resumes native input |
| Ctrl+] | Toggle focus between the agent and previous panel. Preserve the running session |
| Wheel over the native panel / Page Up / Page Down | Read retained normal-screen terminal output. Fullscreen CLI views handle their own scrolling |
| End / Esc while reading terminal output | Return to live output. Typing or paste also returns live |
| Tab, Esc, arrows, slash commands and typing while captured | Forward to the native CLI |
| `:X` after leaving the agent panel | Close the native session and revoke its tools |
| `:C` after closing the session | Open a new native conversation in the selected scope |
| Ctrl+C during native input | Forward to the CLI for its native interrupt behavior. Plous stays open |
| Ctrl+] then Ctrl+C | Return to tree/main, then save and exit Plous |

With **Plous** selected:

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

The CLI manages `/resume`, models, and native conversation storage. Normal-screen history retains up to 5,000 rows. The session's cluster stays fixed while you browse elsewhere.

For Plous chat, `cc` enters typing and Esc keeps the draft. A freshly typed `/model` opens the picker. Pasted or restored commands stay literal.

`:A` opens a finished answer in the wider reader. Use `/` to search, `v/y` to select/copy, `Y` to copy Markdown, and `R` for source mode. Enter opens Sources.

In Sources, arrows choose and Enter opens details or saved content. Esc returns through each layer without making a new cluster request. Alt+Left / Alt+Right keeps your reading position.

See [AI chat](../guides/ai-chat.md) for setup and history.

## Help

Press `hh` or `:h`. Left / Right chooses a top section. Arrows and page keys scroll it. Home / End goes to its start or end.

Esc restores the previous view, including an unfinished Settings form. Alt+Left / Alt+Right leaves and restores Help without losing position.

## Text editing

| Key | Action in a text field, command box, or chat draft |
| --- | --- |
| Left / Right | Move the caret one character. |
| Home / End | Beginning / end of the whole input. |
| Backspace / Delete | Delete before / at the caret. |
| Ctrl+H | Backspace compatibility in text input for terminals that send the BS byte. |
| Ctrl+A / Ctrl+E | Beginning / end of the whole input. |
| Ctrl+U | Delete text before the caret. |

Chat Home / End moves across the whole draft. Up / Down does not move between its lines. Plous Editor has separate document controls.

Paste does not send messages or run commands. Navigation and the command box ignore it. Backspace works as DEL or BS. Ctrl+H has no navigation action.

## Cancel and exit

Esc closes one layer: pending shortcut, search or input, picker, detail, then root focus. It keeps applied table filters and does not stop a running reply. Use Backspace for the table filter and `:X` for the reply.

`:quit` exits Plous. `:q` also exits unless a focused editable document owns it. Ctrl+C saves and exits everywhere except the focused native agent terminal. There, press Ctrl+] first, then Ctrl+C.

Exiting does not apply unsaved forms or resource drafts and does not undo submitted operations.

Below 80×16, most views have restricted navigation. Overview has a minimal view from 40×12. Resize to restore the full interface.
