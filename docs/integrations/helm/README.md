# Helm

[Documentation](../../README.md) · [Helm controls](../../reference/keymap.md#helm-explore)

Read release history, compare revisions, preview chart changes, and roll back a release. Helm appears under a cluster after Plous detects release metadata.

## Connection and discovery

Helm shares the Kubernetes connection. Plous includes the Helm 3.22 Go SDK, so you do not need the Helm executable. Kubeconfig authentication helpers still need to be installed.

Discovery checks Secrets, then ConfigMaps, unless release storage is configured explicitly. SQL storage is unsupported.

If discovery fails or access is denied, Plous shows **Tool discovery incomplete**. A previously detected Helm branch remains visible. Press `g` on the cluster or tool to retry.

## Releases and Charts

**Releases** lists namespace, chart version, revision, and recorded status. Press Enter for the revision timeline.

| Key | Timeline action |
| --- | --- |
| Up / Down | Select a revision |
| Page Up / Down | Scroll within the selected revision |
| `v` | Open recorded changes |
| Enter | Inspect that exact revision |
| `g` | Refresh history and retry unavailable comparisons |

Each revision compares with its predecessor. The first is an initial installation. An unavailable baseline is labeled rather than replaced with another revision.

**Charts** groups installed releases by chart name. It is not a public catalog. Use arrows and Enter to inspect a release.

In either collection, `n` chooses a namespace, `/` filters, Backspace clears the filter, and `S` sorts. `:N *` selects all namespaces.

### Inspect a release

Use `J` for the release picker and `F` for the revision picker. Left / Right switches sections:

| Section | Contents |
| --- | --- |
| Resources | Recorded resources and hooks. Enter opens full YAML. More offers recorded/live comparison. |
| Values | Computed values, supplied values, or chart defaults |
| Chart | Metadata, schema, manifests, hooks, notes, and stored chart files |

Press `/` to find text. Enter finishes the query, then `n/N` moves between matches. Esc returns to the timeline or list.

Sensitive values are masked by default. Explicit reveal actions show them in memory without adding them automatically to assistant context.

Values apply to the release as a whole. A live comparison can include server defaults and controller changes, not just changes made by a user.

## Chart previews

1. Choose **Preview chart changes** from the release's Actions.
2. Set the chart, release name, namespace, and values. Remote charts need an exact version.
3. Use **Edit Values** if needed. Saving updates the draft only.
4. Choose **Preview**, then **Review changes** for the comparison.

Plous snapshots the chart, runs lint and local rendering, and compares resources with the stored release. Dependencies must already be packaged.

Rendering does not prove API acceptance or workload readiness. Cluster lookups, admission, hooks, and controllers can produce different results.

Installation, namespace creation, and upgrade execution are unavailable. Preview drafts stay in memory until exit. Use **Resume unfinished action** or **Discard unfinished action** in Actions to manage them.

## Rollback

1. Select an older release revision.
2. Press `B` or `:B` for a preview.
3. Choose **Execute rollback** in Actions.
4. Check the cluster, namespace, release, and revision, then confirm.

Plous rechecks the release before invoking Helm. Other Helm users can still change it afterward.

Rollback creates a new revision and may run hooks. It does not restore external databases or undo other application side effects.

## Activity and assistant

[Activity](../../guides/activity.md) stores operation outcomes and observed revision metadata, without values or manifests. It is not a complete audit log.

A successful Helm command does not prove workload readiness. A failed or interrupted operation can have partial effects. Check the release before retrying an unknown outcome.

Agents can read release inventory, status, values, chart metadata, filtered resource fields, and revision differences. They cannot install, upgrade, roll back, acquire charts, or read arbitrary paths through Helm tools.

Kubernetes diagnostics are separate and require [explicit setup](../../mcp/active-diagnostics.md). See the [MCP reference](../../reference/mcp.md#available-tools).
