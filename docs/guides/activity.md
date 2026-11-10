# Activity and history

[Documentation](../README.md) · [Workspaces](workspaces.md)

Activity shows saved workspace changes, recorded Helm operations, and Helm revisions that Plous has observed.

## Open and filter the timeline

Press `33` or `:3`. Use Left / Right to choose **All activity**, **Plous actions**, **Workspace changes**, or **Observed Helm history**.

Press `/` to filter and `g` to refresh. Wide layouts show a preview beside the timeline. In narrow layouts, press Enter for details. If loading fails, press `d` for the full error.

## Review a workspace change

Select a workspace entry to see its Before / After fields. Press **d View changes** or `:v` for the saved revision comparison.

The first revision compares against an empty workspace. Comparisons are read-only and do not restore cluster resources.

## Inspect a Helm operation

An operation entry shows what Plous submitted and the cluster it targeted. An unconfirmed result stays **unknown**.

A successful Helm operation does not prove that its Pods are ready. Failed or interrupted operations can have partial effects. Check the current release before retrying.

Release links open only when the saved connection still matches the recorded target. See [Helm rollback](../integrations/helm/README.md#rollback).

## Revisit observed release history

Plous can save the Helm revisions it reads while you inspect a release. Activity shows when Helm recorded the revision and when Plous read it.

Press `O` for the original observations. A recorded `deployed` status describes that Helm revision, not current workload health.

## Choose the right history

| Question | Where to look |
| --- | --- |
| What workspace settings changed? | Activity → Workspace changes |
| What happened to a Helm operation? | Activity → Plous actions |
| Which Helm revisions did Plous observe? | Activity → Observed Helm history |
| What supported an assistant's answer? | [Chat history and Sources](ai-chat.md#inspect-a-replys-sources) |
| What changed recently in this cluster session? | [Overview → Recent Deployments](overview.md#recent-deployments) |
| What were CPU and memory measurements? | [Usage charts](../integrations/kubernetes/README.md#usage-and-charts) |

Activity is not a complete Kubernetes audit log. Browsing a resource does not save all its values and logs.

## Keep history across restarts

Use the same Plous data directory to retain Activity. `--data-dir` selects a separate profile. Renaming a connection keeps its history.

Chart samples and Overview's recent-change feed are temporary. See [backups](../reference/data-and-privacy.md#back-up-or-recover-a-profile) for saved data.
