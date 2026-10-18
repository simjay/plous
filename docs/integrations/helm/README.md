# Helm

[Documentation](../../README.md) · [Tool integrations](../README.md) · [Keymap](../../reference/keymap.md#helm-explore)

Helm appears under **Monitor → cluster → Helm** when Plous detects release metadata in that context. This applies to Current Workspace and saved workspaces. Add a Kubernetes context once through **Monitor → + Add Context** or **+ Add Connection → Kubernetes Cluster**. Both views share the same connection. Existing separate connections retain their identities and settings. Setup reads local context metadata; saving does not run Helm or change your active kubecontext.

Use [Releases and Charts](#releases-and-charts) to inspect what Helm recorded, [chart previews](#chart-previews) to examine proposed output, and [rollback](#rollback) to review and explicitly restore a retained revision. Helm's recorded status and current workload readiness are separate observations.

## Connection and discovery

Plous embeds the Helm 3.22 Go SDK; the Helm executable is not required. The embedded engine version is fixed by the Plous build. Authentication comes from the selected kubeconfig, including its configured credential plugins. An explicit Release Storage setting is respected. Otherwise, discovery checks Secrets first, then ConfigMaps, and uses the detected storage for Helm reads. SQL storage is not supported.

Discovery requests only metadata for one record labeled `owner=helm`, without loading release contents. If listing across namespaces is denied, it checks the context's default namespace. Failed or denied checks leave an unconfirmed Helm branch hidden and show **Tool discovery incomplete**; they do not prove Helm is absent. Previously detected Helm remains visible if a refresh fails. Press `g` on the cluster or a tool branch to retry discovery.

Choose **Releases** or **Charts** in the sidebar with Up/Down and Enter. Left/Right expands or collapses sidebar branches. In the main panel, Left/Right stays within the current view: it switches sections inside an opened release and moves between chart cards. Left/Right does nothing in tables.

## Releases and Charts

**Releases** keeps a table of release identity, namespace, chart/version, revision, and recorded status. Enter opens its revision timeline, newest first. Each block shows status, time, chart version, description, and recorded changes against the immediately preceding revision. Separated blocks use explicit Revision labels and readable timestamps. Change summaries count resources and hooks added, changed, or removed and name changed release-wide inputs. The selected revision also lists up to three affected objects; raw YAML stays in the full comparison. The first revision is an initial installation. Missing or inaccessible baselines are labeled unavailable; Plous does not substitute another revision. Failed or pending revisions describe recorded configuration, not proof that it was applied.

Up/Down selects a revision; Page Up/Page Down scrolls within its block while the selected revision stays visible. `v` opens full changes in the Diff Reviewer. When the full diff is too large, counts and object names remain available and `v` opens the complete change summary; Enter still inspects either revision. Enter opens the chart inspector at that exact revision. Esc returns to the timeline with its selection and reading position preserved. `g` refreshes retained history and retries unavailable comparisons. Changes load for the selected block and nearby blocks, with a bounded in-memory cache.

**Charts** groups installed releases under their reported chart name; it is not a public chart catalog. Each release card shows its namespace, chart version, release revision, and recorded status. Arrow keys move between cards; Enter opens the chart inspector directly. Different chart versions can appear in one group. Equal names or versions do not establish common origin or identical contents. There is no cross-release comparison screen.

Both collections retain the Kubernetes-style context header and namespace picker. Press `n` or `:N` to choose a namespace, **All namespaces**, or manual entry. Listing denied by RBAC still allows manual entry. `:N <name>` selects a namespace directly; `:N *` selects all namespaces. `/` filters releases and chart names. Backspace clears the applied filter while keeping the selected release and scope. `S` sorts releases within the collection; choosing the same column reverses its order. Refresh preserves the selected release.

The shared chart inspector has separate **Release** (`J`) and **Revision** (`F`) dropdowns. Both dropdowns support search and mark the current choice. Changing Release loads its latest revision. Changing Revision loads the exact retained record and labels historical inspection. The release picker lists installations of that chart in the loaded namespace scope, independently of the applied text filter.

- **Resources** groups recorded objects by purpose, including Workloads, Networking, Configuration, Storage, Access, and Hooks. Whitespace separates the groups. Selection shows the template and resource-specific configuration with distinct identity and configuration sections. Enter opens full recorded YAML in a scrollable reader. More offers explicit recorded/live comparison. Wide panels show inventory and configuration beside each other; narrow panels stack them. Controller-created descendants are not inferred.
- **Values** shows computed release values. Enter opens a selector for computed values, supplied values, or chart defaults.
- **Chart** shows retained chart metadata and its values schema. Enter selects metadata, rendered manifests, hooks, notes, release metadata, or retained chart files.

Left/Right switches inspector sections. Up/Down selects resources or scrolls text; Home/End and page keys also navigate. Refresh and section switches keep the selected resource and reading position. `/` searches the current section in place; Enter finishes query entry and `n`/`N` moves between matches. Timeline search includes revision metadata, loaded summaries, and loaded diffs, with the matching revision selected. Select a revision to load its changes. Sensitive fields are masked by default and hidden changes retain a visible marker in comparisons. Explicit reveal actions are available through More and resource actions. Revealed documents remain in memory and are excluded from automatic assistant context.

Values are release-wide. Plous does not invent a values file for each resource or claim an exact mapping from every values key to generated fields. Live comparisons include server defaults and controller changes, and do not classify every difference as a fault.

## Chart previews

Choose **Preview chart changes** from a release's Actions. The preview inherits its Helm context. Configure the release name, namespace, chart reference or local archive/directory, exact remote version, optional repository URL, and values. **Edit Values** opens Plous Editor. Saving there validates YAML and updates the draft, without editing source files or the cluster.

**Preview** snapshots the chart archive, runs Helm lint and local template rendering, and compares the proposed resources with the retained release. The summary identifies known cluster-scoped resources, hooks, and changes to replicas, selectors, Pod templates, and volume claim templates. Actions opens the complete summary. Rendering does not reproduce unrecorded pipeline steps. Missing chart dependencies must already be packaged; Plous does not build them automatically.

Use **Review changes** to inspect the shared Diff Reviewer. Local rendering checks templates and values schemas, but does not establish API-server acceptance or workload readiness. Hooks, cluster lookups, admission, random template values, and controllers can produce effects beyond this preview. Resource comparisons mask sensitive fields and mark hidden changes as `[redacted: changed]`. **Reveal resource differences** in Actions opens the full comparison.

Chart changes are previews. Installation, namespace creation and upgrade execution are unavailable; rollback is the only Helm write action. Draft values and downloaded chart contents remain in memory while navigating, opening Help, or using chat. Esc from chart inputs or a preview returns to the chart or release that opened it. Use **Resume unfinished action** or **Discard unfinished action** in Actions to manage retained work. Drafts are not saved across application exit. Section, namespace, applied filter, sort order, and release selection are saved.

## Rollback

Select an older revision in the release timeline and press **B** / `:B` to preview rollback. The same action is available while inspecting a historical revision. On the latest revision, choose an earlier revision first. The preview uses stored release manifests, values, and hooks without requiring the original repository.

From the preview's Actions, choose **Execute rollback**, review the captured context, namespace, release, and revision, then explicitly confirm. Plous rechecks the current and retained revisions before invoking Helm. This check is not an atomic lock against other Helm users. Rollback creates a new release revision and may run hooks; it does not restore external databases or other side effects.

## Activity and assistant

Activity combines existing workspace configuration history with Helm operation outcomes and retained revisions observed while opening release details. Retained revisions show their source timestamp separately from observation time, with no invented actor. They are not a complete audit. Operation records contain scope and outcome metadata, not release values or manifests. Enter opens the recorded release; if the saved connection settings have changed, Plous stops instead of following the link to a different target.

A completed Helm command does not prove workload readiness. Failed or interrupted operations can have partial effects. Unknown outcomes are never retried automatically; inspect the release before deciding what to do next.

The assistant can read release inventories, metadata, status, history, values, chart metadata, masked resource fields from manifests/hooks, and differences between retained revisions of one release through the guarded executor. The comparison defaults to the preceding revision when no explicit pair is selected. Status includes the recorded release description. It can list releases across namespaces on the one selected cluster. Kubernetes and Helm capabilities share that captured target. Select another connection in Plous before asking the assistant to access it. It cannot install, roll back, test, upgrade, acquire charts, read arbitrary paths, or run arbitrary commands. Release notes are kept in the user-operated view because they can embed credentials.
