# Activity and workspace history

[Documentation](README.md) · [Workspaces](workspaces.md)

Activity records changes to a workspace and its tool connections. Use it to see what changed, when, and how two saved revisions differ.

## Open a change

Press **33** or run `:3` to open Activity for the current workspace.

1. Select a change with **Up / Down**.
2. Press **Enter** to read its details.
3. Scroll with arrows or Page Up/Page Down.
4. Press **Esc** to return to the list.

Activity shows workspace creation, renaming, and connection changes. Details use **Before** and **After** values and show only changed fields. Additions, removals, and updates have symbols as well as colors. Use **/** to filter the list or find text in a detail.

## Compare revisions

Press **d View changes** from the detail, or run `:v` / `:review`, to open **Plous Diff Reviewer**. It compares the exact saved workspace snapshots and labels their revisions. Creation compares an empty snapshot with the first revision. The original summary stays available and its scroll position is preserved.

The comparison is read-only. Use **[ / ]** for previous/next change, **/** to search, **v** to select text, and **y** to copy. **Esc** closes search or selection first, then returns to the summary. There is no Apply or rollback action.

Activity is configuration history. Kubernetes object Events and live operation results belong to Explore.

Workspace history stays in the local profile. See [Data and privacy](data-and-privacy.md) for storage and backups.
