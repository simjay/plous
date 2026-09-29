# Keymap

Keyboard controls for **Plous 0.0.1**. Commands belong to the focused panel; the highlighted border shows which panel receives input.

## Switch panels

| Key | Action |
| --- | --- |
| Tab | Move focus forward: workspace tree → main panel → AI assistant. |
| Shift+Tab | Move focus backward. |

These keys also work while editing a form or chat draft. Unfinished input stays in place when focus changes.

## Workspace tree

| Key | Action |
| --- | --- |
| Up / Down | Move to the previous / next row. |
| Home / End | Move to the first / last row. |
| Page Up / Page Down | Move up / down ten rows. |
| Right | Expand a workspace or tool group. |
| Left | Collapse an expanded group; otherwise move to its parent. |
| Enter | Select the highlighted workspace, tool, or connection. Open settings or create a workspace when that row is selected. |

Selecting a workspace, tool, or connection preserves the current main-panel mode and keeps focus in the tree. Opening **Workspace settings** or **+ New workspace** moves focus to the main panel. Finish or cancel an open form before selecting a different workspace or connection.

There are no numeric mode shortcuts in the tree. `:` does not open a command prompt here.

## Main-panel commands

With the main panel focused and no form being edited, type **:** to open a bordered command box at the bottom of the panel. The current content stays above it. Enter a command, then press **Enter** to run it. Every command accepts a full name and a short alias; the tab labels also show numeric mode shortcuts.

| Full command | Short alias | Numeric shortcut | Action |
| --- | --- | --- | --- |
| `:overview` | `:o` | `:1` | Open Overview. |
| `:resources` | `:r` | `:2` | Open Resources. |
| `:deployment` | `:d` | `:3` | Open Deployment. |
| `:activity` | `:a` | `:4` | Open Activity. |
| `:new` | `:n` | — | Create a workspace. |
| `:settings` | `:s` | — | Open settings for the active workspace. |
| `:help` | `:h` | — | Open in-app help. |
| `:quit` | `:q` | — | Save the session and exit Plous. |

Switching modes preserves the selected workspace and tool. **Esc** cancels a command without running it. Commands are not available while a form field is being edited; `:` is ordinary text there.

Live resource browsing and tool-specific resource actions are not implemented yet. There is no shared resource-action keymap. `:pod` is not an implemented resource command.

## Main-panel lists and Activity

| Context | Key | Action |
| --- | --- | --- |
| Settings and tool lists | Up / Down | Select an item. |
| Settings and tool lists | Enter | Activate the selected item. |
| Kubecontext selection | Up / Down | Select a row. |
| Kubecontext selection | Space | Toggle the highlighted context. |
| Kubecontext selection | Enter | Add checked contexts, or the highlighted context if none are checked. |
| Activity list | Up / Down | Select a change. |
| Activity list | Enter | Open the selected change's details. |
| Activity details and help | Up / Down | Scroll one line. |
| Activity details and help | Page Up / Page Down | Scroll a page. |
| Open page or Activity details | Esc | Close the page or details and return to the selected mode. |

## Forms

| Key | Action |
| --- | --- |
| Up / Down | Move between fields and actions. |
| Enter in a field | Advance to the next field or action. Required fields must have a value. |
| Enter on an action | Activate Save, Cancel, Find contexts, or Remove connection. |
| Esc | Cancel the form without saving its edits. |
| Tab / Shift+Tab | Switch panels while preserving the form. |

Typing and pasting into a field inserts text. It never executes mode commands or tree actions.

## AI assistant draft

Select a workspace, focus the AI assistant panel, and type in the draft box at the bottom.

| Key | Action |
| --- | --- |
| Ordinary text, including `:` and `/` | Edit the local draft. |
| Enter while editing | Insert a newline. |
| Esc | Leave draft editing without discarding the draft. |
| Enter after leaving editing | Resume editing. Typing also resumes editing. |
| Tab / Shift+Tab | Switch panels and keep the draft. |

Drafts save locally for each workspace. **There is no send action or AI connection yet.** Slash commands are not implemented.

## Text editing

These bindings apply within form fields, the main command prompt, and chat drafts.

| Key | Action |
| --- | --- |
| Left / Right | Move the caret one character. |
| Home or Ctrl+A | Move to the beginning of the entire input. |
| End or Ctrl+E | Move to the end of the entire input. |
| Backspace | Delete the character before the caret. |
| Delete | Delete the character at the caret. |
| Ctrl+U | Delete all text before the caret. |

For a multiline chat draft, Home / End and Ctrl+A / Ctrl+E refer to the whole draft, not the current line. Up / Down do not move the draft caret between lines yet.

Terminal paste is accepted in forms and chat, where its content stays text. Pasting does not run commands; paste into the command prompt or navigation views is ignored.

## Cancel and exit

**Ctrl+C saves the session and chat drafts, then exits Plous from any panel.** It also works while editing a form or command. Unsaved form edits are not applied.

Use **Esc** to cancel a form or command without exiting. You can also exit with `:quit` or `:q`: focus the main panel, cancel any open form with **Esc**, enter the command, and press **Enter**.

If the terminal is smaller than 80 columns × 16 rows, Plous shows a resize message and disables hidden actions. **Ctrl+C**, panel focus, and the exit commands `:quit` / `:q` remain available. Other commands are unavailable until the terminal is large enough.
