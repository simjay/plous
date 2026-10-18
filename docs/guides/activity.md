# Activity and history

[Documentation](../README.md) · [Workspaces](workspaces.md) · [Helm](../integrations/helm/README.md)

Use Activity to answer “What changed in this workspace?” and “What happened to this Helm operation?” It combines saved workspace configuration revisions, recorded Helm operation outcomes, and retained Helm revisions observed by Plous.

## Open and filter the timeline

Press **33** or `:3` while navigating. Entries are grouped by date. Left/Right selects **All activity**, **Plous actions**, **Workspace changes**, or **Observed Helm history**. Use **/** to filter and **g** to refresh.

Wide layouts show the selected entry's preview beside the timeline. Enter opens details in narrow layouts. Esc returns with selection and filters preserved. If loading fails, **d** opens the full error. Exact controls are in the [keymap](../reference/keymap.md).

## Review a workspace change

Workspace entries record changes to saved configuration, such as a connection's name or settings. Select an entry to see its Before/After fields. **d View changes** or `:v` opens the exact saved revision comparison. The first revision compares against an empty snapshot.

The comparison is read-only. It has no Apply or rollback action and does not restore infrastructure. Workspace changes cover the workspace; Helm entries follow the selected connection.

## Inspect a Helm operation

A recorded outcome describes the submitted operation and its captured target. An unconfirmed submission stays **unknown**. A successful Helm operation does not establish that its workloads are ready, and a failed or interrupted operation can have partial effects.

Select a release link to inspect it. Plous checks that the saved connection still matches the recorded target before following the link. If it has changed, the link stops rather than opening another cluster. Inspect current release and workload state before deciding whether to retry an operation. See [Helm rollback](../integrations/helm/README.md#rollback).

## Revisit observed release history

Opening Helm release details can retain observations of its stored revisions. Activity separates the source's timestamp from the time Plous read it and labels fallback times. It does not invent an actor or claim that Plous initiated an observed change.

Records group only when the connection, configuration, namespace, release, and revision match exactly. **O** opens the original observations. A recorded `deployed` status describes Helm's release record; it does not prove current Pod health.

## Choose the right history

| If you need… | Use | Coverage |
| --- | --- | --- |
| Saved connection or workspace changes | Activity → Workspace changes | Exact Plous configuration revisions |
| A Helm operation's recorded outcome | Activity → Plous actions | Operations recorded by Plous, including unknown outcomes |
| Previously observed Helm release records | Activity → Observed Helm history | Retained observations, not every cluster event |
| An answer and the evidence it received | [Chat history and Sources](ai-chat.md#inspect-a-replys-sources) | Saved conversations and tool observations in their captured scope |
| Recent cluster change signals | [Overview → Recent Deployments](overview.md#recent-deployments) | Timestamped observations retained during the current cluster session |
| CPU and memory samples | [Usage charts](../integrations/kubernetes/README.md#usage-and-charts) | A bounded in-memory window while the view collects samples |

Activity is not a complete Kubernetes audit log, a Git commit browser, or a continuous archive of resource contents. Browsing a resource does not save all of its values and logs for future comparison. History tools can compare observations that were explicitly saved as evidence.

## Keep history across restarts

Activity records remain in the selected local profile. Relaunch with the same data directory to retain them; `--data-dir` selects an independent profile. A renamed connection keeps its identity and history. Chart samples and Overview's recent-observation feed remain temporary.

See [Data and privacy](../reference/data-and-privacy.md#back-up-or-recover-a-profile) for profile backups and [troubleshooting](../reference/troubleshooting.md#plous-reports-corrupt-data) for storage recovery.
