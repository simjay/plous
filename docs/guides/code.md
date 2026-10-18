# Code: from source to rendered resources

[Documentation](../README.md) · [Code and Kustomize reference](../integrations/kustomize/README.md) · [Editor keymap](../reference/keymap.md#plous-editor-and-diff-reviewer)

Use Code to understand the Kubernetes configuration you are about to change. Browse files, edit a local manifest, follow Kustomize dependencies, render a chart or overlay, and compare the generated resources. You can do this without a cluster or an AI account.

## Open the directory you work in

Launch Plous from your configuration directory, then open **Current Workspace → Code → your directory**. Plous uses that exact launch directory, with no Git requirement. It scans when you open or expand Code, or refresh it.

To keep another directory or repository available, use **Code → + Add Source** in a saved workspace. Choose a local directory or a Git repository URL, with an optional branch and starting path. See [source setup](../integrations/kustomize/README.md#open-a-source) for authentication and acquisition details.

Local sources read saved files on your computer. Remote Git sources use an acquired revision until refreshed. A local checkout remains a local source even when it has a Git remote.

## Choose a browser

| Browser | Use it for |
| --- | --- |
| **Files** | Folders, text files, and local editing |
| **Helm** | Charts and declared chart references grouped by source path |
| **Kustomize** | Groups of connected bases, overlays, and components |
| **Argo** | Application, ApplicationSet, and AppProject declarations |
| **Flux** | Controller and source declarations |

These views describe source configuration. For live controller status, use [Monitor's Argo and Flux views](../integrations/gitops/README.md). Source declarations do not establish what is deployed.

## Make a first local edit

1. Open **Files**, select a text file, and press Enter. Local text opens in Plous Editor; a remote snapshot opens read-only.
2. Press **i** to enter Insert and make your change. Press Esc to return to Normal.
3. Use `:v` if you want an optional review, then `:w` to save. `:wq` saves and closes after success.
4. Use `:q` to close. Modified text offers **Keep editing** or **Discard changes**; `:q!` explicitly discards it.

Saving writes the selected local file after checking for external changes. It does not apply the configuration to Kubernetes. Help and panel travel preserve the in-memory draft and undo history, but unsaved text is not restored after application exit. See the [editor controls](../reference/keymap.md#plous-editor-and-diff-reviewer).

## Render a Kustomize target

1. Open **Kustomize** and select a group. Enter opens its dependency graph.
2. Select the overlay or base you want to inspect, then Enter for its overview. Includes and Used by show its relationships.
3. Press **r** or choose **Render**. Plous renders that target using its saved inputs and supported declared dependencies.
4. Select a generated resource to inspect its YAML. Open a field for captured **Field History**, or use **d** to open a verified resource origin when available.
5. Open **a → View Findings** to inspect reported parser, reference, image, or configuration issues.

Selecting a graph node does not render it. Components need a consuming configuration; a base can be rendered on its own. Render uses saved bytes, so save your edit before rebuilding. A successful render shows generated configuration, not API-server acceptance or workload readiness.

When a render fails, use **Technical details** for the full masked diagnostic and **d Open source** to inspect the target. A failed retry keeps earlier successful output labeled **Previous output**. See [render behavior](../integrations/kustomize/README.md#render) and [engine limits](../integrations/kustomize/README.md#engines-and-boundaries).

## Render a Helm chart

Select a local chart in **Helm**, then use **r**. **a → Helm Inputs** sets the release name, namespace, ordered values files, overrides, and Kubernetes/API versions. The default release is `preview` in namespace `default`.

For a chart included by Kustomize, choose the chart alone or the including Kustomization. Composed rendering uses the Kustomization's declarations. An Argo or Flux chart reference is inspectable but does not automatically acquire a renderable remote workload. See [configuration selection](../integrations/kustomize/README.md#files-and-configuration-selection).

## Compare the output

After a local save, an already rendered selected entry can refresh and compare its new output with the pre-edit result. **Show Changes** and **Show Final YAML** switch between the comparison and complete output. A save that produces identical resources shows **No manifest changes**.

For an explicit comparison, press **v** to choose another entry in the same source, or use **a → More Tools → Compare Git Revision**. Comparison controls can swap direction, show changed lines, or open the full Diff Reviewer. Comparisons do not deploy or save resources. See [snapshot comparison](../integrations/kustomize/README.md#compare-snapshots).

Field history identifies captured changes where the engine can establish them. Plain YAML may have exact source lines; generated or ambiguous fields can have partial attribution. A resource's origin alone does not prove which file wrote every field. See [field history](../integrations/kustomize/README.md#field-history).

## Ask about the captured result

With the source selected and a render completed, ask the assistant “Explain this field's value and show its captured source” or “Summarize the changes between these outputs.” Source tools read saved files and captured analysis; they cannot read an unsaved draft, render, acquire another revision, or write a file.

Use [AI chat](ai-chat.md) for the built-in agent or [Plous MCP](plous-mcp.md) for an installed native agent. A native session must be reopened to capture a different source.

## Know what is retained

Source settings and navigation preferences are saved. Discovered configuration, acquired Git content, render inputs, and output are temporary. Requested assistant evidence is saved separately in Sources. Scans honor the profile's `.plousignore` and source `.gitignore` rules; exact reads and declared render dependencies have separate access rules. See [source exclusions and privacy](../reference/data-and-privacy.md#global-source-scanning-exclusions).

Code has no deployment action, file watcher, or Terraform execution. See the [full Code reference](../integrations/kustomize/README.md) for supported engines, findings, export, capture limits, and unsupported inputs.
