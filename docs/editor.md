# Edit and review YAML

[Documentation](README.md) · [Kubernetes actions](kubernetes-actions.md)

Plous Editor edits live Kubernetes resources. Open a resource's details and press uppercase **E**. Lowercase **y** opens its read-only YAML view.

## Open the view

Saving updates the live resource in the context and namespace shown in the editor header.

1. Open the object and press uppercase **E**. **Plous Editor** opens in **NORMAL** mode.
2. Press **i** to type or paste YAML. Press **Esc** to finish typing.
3. Type **`:w`** and press Enter to **Save**. Plous validates the YAML and saves it to Kubernetes automatically. You can also press Esc in Normal to focus the action row, choose **Save**, and press Enter.
4. Wait for **Saved to Kubernetes**. Use **`:q`** to close, or use **`:wq` Save and close** instead of `:w` in step 3.

## Review the difference

**Review changes is optional.** Use **`:v`** / **`:review`** to open the diff without saving. The comparison includes server defaults. **`:w` Save** has the same meaning there; **`:wq`** saves and closes. Alternatively, press **a**, choose **Save** with Right, and press Enter. Enter while reading never saves. Esc returns to the editor after closing local search/selection or restoring an expanded view. Returning preserves the draft, cursor, selection, and undo history.

## Recover from a failed save

Invalid YAML, rejected updates, and conflicts leave you in the editor with **Save failed** and your draft intact. Press **!** in Normal or editor actions for the full error; Esc returns to editing. For example, a Deployment requires `apiVersion: apps/v1` and `kind: Deployment`; an incorrect field is named with its expected value. Fix the draft and use `:w` again. A conflict means the resource changed since you opened it; reconcile your edits with the current resource before saving.

**Save result unknown** means Plous could not confirm whether Kubernetes accepted the write, including cancellation after submission. Check the live resource before retrying. Nothing retries automatically. Cancelling during validation saves nothing. **Saved to Kubernetes** confirms server acceptance; check resource details separately for rollout or readiness status.

## Undo, close, and discard

Use **u** to undo and **U** (Shift+U) to redo in Normal or Select. Named commands `:u` / `:undo` and `:U` / `:redo` remain available. **`:q`** closes an unchanged document and offers **Keep editing** or **Discard changes** for an unsaved draft. **`:q!`** discards and closes. **`:wq`** closes only after a successful save; if there are no edits, it closes without a request. **`:w`** reports **No unsaved changes** for an unchanged document.

**`:quit`** and Ctrl+C exit Plous without saving resource drafts. `:q` is local to the editable document or its review while the main panel has focus. See the [keymap](keymap.md#plous-editor-and-diff-reviewer) for all editor commands.

The editor supports a small Vim-style vocabulary, not full Vim. Type **`:h`** in Normal or Select to open the complete reference in Help's **Views** section. In Insert, press Esc first. Page Up/Down scrolls the reference; Esc closes Help and keeps the draft, selection, and undo history. The same reference is available from validation errors and review. See [editor keys](keymap.md#plous-editor-and-diff-reviewer) for the written reference. Tab in Normal/Select moves to another panel; Tab in Insert indents. Shift+Tab moves panels.

## Limits and sensitive values

Some resources allow metadata-only edits. Editing a Secret explicitly reveals its values. Documents, undo history, comparisons, and diagnostics stay in memory and are not saved in Plous's workspace or session files. The editor accepts up to 2 MiB and rejects oversized edits without truncating the document. Undo retains up to 100 snapshots within a 16 MiB budget.

See [Keyboard shortcuts](keymap.md) for the complete key reference.
