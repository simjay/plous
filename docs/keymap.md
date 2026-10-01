# Keymap

[Documentation](README.md) · [Learn the interface](navigation.md)

Keyboard reference for Plous 0.0.1. **Navigation** means you are outside a text field, search editor, or chat composer. Colon commands work from any navigation panel; ordinary keys affect the focused panel. Type a colon command and press **Enter** to run it.

Application action aliases use one letter and are case-sensitive. Capitals distinguish actions when lowercase is taken, such as `c` for chat and `C` for new chat. Full names are optional alternatives. The double-key navigation shortcuts below, editor sequences such as `gg`, `dd`, and `:wq`, and Kubernetes resource abbreviations such as `po` and `svc` keep their own meanings.

## Essential controls

| Key | Action |
| --- | --- |
| Tab / Shift+Tab | Next / previous visible panel. Disabled in Settings, Help, and resource pickers. |
| Up / Down | Select or scroll within the focused panel. |
| Enter | Open the selected item or activate the selected control. |
| `/` | Search the focused panel. |
| Esc | Cancel or close the current layer. At a root main view, focus the visible sidebar. |
| `:` | Open the command box at the bottom of the main panel. |
| Ctrl+C | Save the session and drafts, stop active work, and exit. Unsaved form edits are not applied. |

## Double-key shortcuts

Press the same key twice consecutively. These run on the second key, with no Enter and no timing deadline.

| Shortcut | Command alternative | Action |
| --- | --- | --- |
| `11` | `:1` | Overview. |
| `22` | `:2` | Explore. |
| `33` | `:3` | Activity. |
| `bb` | `:b` | Show or hide the left panel. |
| `ii` | `:i` | Show or hide the right panel. |
| `ss` | `:s` | Settings. |
| `cc` | `:c` | Reveal the assistant and enter typing. |
| `hh` | `:h` | Help. |

The footer shows a pending first key. Esc cancels that prefix first. Another key, panel focus change, resize, or paste also clears it; another reserved first key starts its own sequence. Single numbers do not change views.

These shortcuts work during navigation, including Help, history, and form action buttons. They also work in read-only YAML and value views outside search input. Text fields accept them literally, and the editable document's Vim keys take precedence while its document has focus. Switching Settings or views cannot discard an unfinished form.

## Commands

| Short command | Full spelling | Action |
| --- | --- | --- |
| `:n` | `:new` | Create a workspace. |
| `:s` | `:settings` | Open Settings. |
| `:c` | `:chat` | Enter the current composer. |
| `:C` | `:newchat` | Start a conversation and enter typing, keeping the unsent draft. |
| `:H` | `:openchat` | Open conversation history in the current scope. |
| `:X` | `:stopchat` | Stop the running reply, including one in another workspace. |
| `:p` | `:restoreprompt` | Restore the last sent message into an empty draft for editing. Does not send it. |
| `:b` | `:sidebar` | Toggle the left panel. |
| `:i` | `:aiinteraction` | Toggle the right panel. |
| `:h` | `:help` | Open Help. |
| `:q` | `:quit` | Save and exit Plous. In the focused editable document, `:q` closes that document. |

`:1`, `:2`, and `:3` select Overview, Explore, and Activity. They have no word aliases.

In the command box, Enter runs and Esc cancels, restoring the originating focus. Outside overlays, Tab can leave and resume the draft. A fresh colon from another navigation panel replaces it. Completing a double-key shortcut from another panel also replaces it. Pasting into the command box is ignored.

## Switch panels

Tab cycles left → main → right through visible panels; Shift+Tab reverses that order. They work while typing chat, retaining the draft. Settings and Help ignore both keys, including during forms and command input.

### Show or hide panels

Use **bb** / `:b` or **ii** / `:i`. Hiding the focused panel moves focus to main. Showing a panel does not change focus. Visibility is saved, and hidden panels are skipped by Tab. Chat entry, new-chat, and history commands reveal the assistant.

## Search the focused panel

| View | `/` behavior | Finish or clear |
| --- | --- | --- |
| Workspace tree | Filter names, including collapsed paths in loaded resource catalogs. | Arrows select; Enter opens; Esc restores the unfiltered tree. |
| Kubernetes table | Edit the row text filter. | Enter applies; Esc cancels the edit; Backspace clears an applied filter while navigating rows. |
| Activity list / chat history | Filter displayed entries. | Arrows select; Enter opens; Esc restores the list. |
| Overview, connection settings, details, logs, conversation | Find and highlight text in the current view. | Enter finishes query input; n/N next/previous match; Esc clears find. |

The bar shows **Backspace clear** for an applied table filter. Clearing preserves namespace, labels, sorting, and selection. While typing, Backspace deletes a character. Root Esc preserves an applied filter.

Tab can leave and resume search outside overlays. Search fields accept literal paste. Log search pauses following; Space resumes after search closes. `/` in a chat draft or form remains text, apart from the explicit chat model commands below.

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

These work from any navigation panel and open Explore. If no Kubernetes connection is selected, a connection picker opens first.

| Resource or control | Commands |
| --- | --- |
| Resource-type picker | `:k`, `:kinds` |
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
| `f` | Edit a Kubernetes label selector. Apply an empty value to clear it. |
| `S` | Choose a sort column; choosing the current column reverses it. |
| `g` | Reconnect and rediscover APIs. `:g` / `:refresh` also works. |
| Esc | Return to the previous related view, or focus the visible sidebar at the root. |

`:k` opens the optional resource picker; there is no table `o` shortcut. In resource and connection pickers, type to search, arrows select, Enter applies, and Esc cancels. Left/Right changes categories in the resource picker. In the namespace picker, clear the search and choose **Reload namespaces** to reload; manual entry remains available. Tab does not leave a picker.

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

Confirmations use Left/Right to select and Enter to activate. Resource editing and comparison use the dedicated controls below. See [Kubernetes actions](kubernetes-actions.md) for the validation and submission steps.

### Expand a view

| Short command | Full command | Action |
| --- | --- | --- |
| `:z` | `:zoom` | Expand or restore the current Log Viewer, Diff Reviewer, or Editor. |
| `:L` | `:linenumbers` | Toggle line numbers in YAML, a value preview, or the editing document. |

In Logs, press **z** to expand or restore immediately, without a colon or Enter. While typing a search, `z` remains query text.

Expansion temporarily gives the main panel the full width. Esc closes search or selection first, then restores the layout before leaving the view. In Editor Insert mode, Esc first returns to Normal. Tab restores the layout when switching panel focus; in Editor Insert mode it still indents. Chat commands also restore it. Help temporarily covers the expanded view and returns to it. Saved panel visibility stays unchanged. In live Exec, enter output review with Ctrl+] before using these commands.

Log Viewer and Diff Reviewer show the visible line range within their retained content. Wrapped display rows count as their original source line; diff ranges refer to comparison rows, including hunk headers.

### Plous Editor and Diff Reviewer

Editing a resource opens the Editor in **NORMAL**. Its document keys take precedence over app double-key shortcuts. In **INSERT**, text and colons are literal. In **SELECT**, movements extend the selection. Read-only [YAML](#object-details) and [value inspection](#read-only-value-inspection) use the controls in their own sections. Simple forms and chat retain their existing controls.

Type **`:h`** / **`:help`** in Normal or Select for the complete editor reference in the existing Help window's **Views** section. In Insert, press Esc first. Page Up/Down scrolls the reference; Esc returns to your draft without changing its cursor, selection, or undo history. Help from diagnostics or review includes that screen's controls followed by the editor reference. `hh` belongs to document navigation while editing, so use the colon command there.

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
| Esc in actions | Request Close; changed documents offer Keep editing / Discard changes. |
| Tab in Insert | Insert two spaces. |
| Tab in Normal/Select, Shift+Tab | Switch visible panels and retain the document. |

Bracketed paste is one literal undoable edit. Search, selection, syntax colors, line numbers, and screen wrapping do not alter document contents. The editor has no counts, macros, registers, or full Vim command language. In the focused editable document or its review, the commands below take precedence. Outside that workflow, `:q` exits Plous and `:w` exports resource YAML. `:quit` always exits Plous.

| Short command | Full command | Action in the active document workflow |
| --- | --- | --- |
| `:w` | | **Save**: validate and write to Kubernetes, then stay in the editor. Works in the editor and optional review. |
| `:wq` | | **Save and close**: validate, write, and close after success. If unchanged, close without a request. |
| `:q` | | Close the document; changed text offers Keep editing / Discard changes. |
| `:q!` | | Discard the document and close without applying it. |
| `:v` | `:review` | **Review changes**: validate and open an optional comparison without saving. |
| `:u` | `:undo` | Undo in the editor. |
| `:U` | `:redo` | Redo in the editor. |

Use `:w` or the visible Save control to save directly with automatic validation. Failed saves preserve the draft in the editor; **!** in Normal or editor actions opens full error details. Ctrl+C exits without saving the draft.

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
| `:z` / `:zoom` | Expand / restore the value view. |
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
| Logs | `z` | Expand / restore immediately. `:z` / `:zoom` also work. |
| Logs | `/` | Find text in the loaded buffer and pause following. |
| Logs | `n` / `N` | Next / previous retained match. |
| Logs | `v` / `y` | Select / copy selection or current line. |
| Logs | Esc | Clear search or selection before returning to the originating detail. |

In **Exec**, normal input, Esc, Tab, and colons go to the container. **Ctrl+]** enters **Plous controls active** for output review while the remote process continues. **Ctrl+C exits Plous** in either mode.

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

## Main-panel lists and Activity

| Context | Key | Action |
| --- | --- | --- |
| Settings / tool lists | Up / Down, Enter | Select and activate. |
| Empty workspace Overview | Enter | Add tool. |
| Context setup | Up / Down | Select path, context, or action. |
| Context row | Space or Enter | Toggle its checkbox. |
| Kubeconfig path | Enter | Move to Refresh contexts; another Enter loads it. |
| Context setup outside path input | Home / End | Go to path field / Cancel. |
| Context setup | Enter on an action | Refresh contexts, add checked contexts, or cancel. |
| Activity list | Up / Down, Enter | Select and open a change. |
| Activity detail | Arrows, Page Up / Page Down | Scroll. |
| Activity detail | `d` or `:v` / `:review` | Open a read-only comparison of saved snapshots. |
| Activity detail | Esc | Return to Activity list. |
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
| Tab / Shift+Tab | No action while Settings is visible. |

Text and paste in fields remain text. An explicit chat command from an action button can preserve an unfinished form while you use the assistant.

## AI assistant

Use **cc** or `:c` to enter typing; Esc leaves it and keeps the draft. Chat commands work before workspace creation as well as within workspaces.

| Key or input | Action |
| --- | --- |
| Enter while typing | Send the draft. |
| Shift+Enter | Insert a newline. |
| Ctrl+J | Newline fallback for terminals without distinct Shift+Enter. |
| `/model` or `/m`, then Enter | Open model picker when freshly typed as the entire draft. |
| `//model` or `//m`, then Enter | Send literal command text. |
| `:p` / `:restoreprompt` | Restore the last sent message into an empty draft. Does not send it. |
| Up / Down or k / j during navigation | Scroll the conversation one line. |
| Page Up / Page Down | Scroll a page, including while typing. |
| Home during navigation | Go to the oldest message. |
| End during navigation | Go to the latest reply and follow new output. |
| Esc | Leave typing or close history, preserving the draft and running reply. |
| Up / Down, Enter in history | Select and open a conversation. |
| `/` during navigation | Find conversation text, or filter history. |

Reading older messages keeps your position steady as replies stream. Home/End moves the draft caret while typing. In the model picker, arrows select, Enter applies, Esc cancels, and Home/End moves to the first entry or Manage in Settings. Selection is application-wide. Pasted and restored drafts are literal, including `/model`. See [AI chat](ai-chat.md) for setup and conversation commands.

## Help

Open with **hh** or `:h`. Left/Right selects a top section; Up/Down and Page Up/Page Down scroll it; Home/End jumps to its start/end. The global section stays visible below. Tab and Shift+Tab do nothing. Esc restores the previous view, including an unfinished Settings form.

## Text editing

| Key | Action in a text field, command box, or chat draft |
| --- | --- |
| Left / Right | Move the caret one character. |
| Home / End | Beginning / end of the whole input. |
| Backspace / Delete | Delete before / at the caret. |
| Ctrl+A / Ctrl+E | Beginning / end of the whole input. |
| Ctrl+U | Delete text before the caret. |

Chat Home/End operates on the whole draft; Up/Down does not move between its lines. Plous Editor has its own document modes and line-based controls. Paste edits fields and chat without running commands or sending messages; navigation and the command box ignore paste.

## Cancel and exit

Esc closes one layer at a time: pending shortcut, command/search/editor, picker or operation, detail, then root focus. It does not erase applied table filters or stop chat replies. Use `:X` for a running reply and Backspace for an applied table text filter.

`:quit` exits during navigation. `:q` does the same except in the focused editable document, where it requests closing that document. Ctrl+C saves and exits from anywhere, including forms, Help, small-terminal fallback, and captured Exec input. Exiting does not save unfinished form edits or undo submitted cluster operations.

Below 80 columns by 16 rows, only restricted navigation and exit controls remain. Resize the terminal to restore the full interface.
