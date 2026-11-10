# Quickstart

[Documentation](../README.md) · [Choose your workflow](workflows.md) · [Installation](installation.md)

Launch the terminal workspace, select your cluster, and inspect a Pod's status and logs. No AI provider is required. For tools in an independent coding-agent session, choose [MCP server](../mcp/README.md) or [plous-debug](../agent-skills/plous-debug.md).

## Before you begin

Use macOS or Linux and an interactive terminal at least **80 columns by 16 rows**. Plous is an early development build; native Windows is not supported.

Plous embeds the Go libraries for Kubernetes access and Helm. These features do not require `kubectl` or `helm` executables. A kubeconfig that uses an external authentication helper still requires that helper.

## Install and launch

Run these commands in your shell:

```sh
brew install simjay/tap/plous
plous
```

If Plous is already installed, run `plous`. See [Installation and updates](installation.md) for source builds, platform coverage, and differences between published releases and development docs.

Plous saves data under `~/.plous/`. For an independent profile:

```sh
plous --data-dir /tmp/plous-demo
```

A separate profile isolates saved settings and conversations. Its connections still point to the infrastructure you choose. See [Data and privacy](../reference/data-and-privacy.md) for profiles and backups.

## Start with Current Workspace

Plous opens **Overview** for the merged kubeconfig `current-context`. Switch to **Explore** with `22` or `:2` to see Pods in its namespace, or `default` when unset. It reads the inherited `KUBECONFIG` path list, otherwise `~/.kube/config`. It contacts only the selected target. Use `:K` / `:connections` to search local context names and select another target within Plous. Selection never changes kubeconfig.

If configuration is missing or invalid, choose a context with Enter or `:K`, or open **Settings → Choose Kubeconfig**. Use `:g` to retry after correcting the file. General chat and AI setup remain available.

An external kubeconfig change shows a notice; it does not switch an open view. Relaunching reads the default again.

## Learn the basic controls

| Key | Use it to |
| --- | --- |
| Alt+Left / Alt+Right | Move between the workspace, main, and assistant panels. |
| Up / Down, then Enter | Select and open an item. |
| `/` | Search the focused panel. |
| Esc | Close the current search, editor, or detail view. |
| `hh` | Open Help while navigating. |
| `ff` or `:f` | Fullscreen or restore the previous layout. |

Settings uses arrows and Enter. Alt+Left/Right leave and resume the window without applying edits. Unmodified letter shortcuts stay ordinary text while typing in a field.

Press **11** for Overview, **22** for Explore, or **33** for Activity. See [Interface and navigation](../guides/navigation.md) for the full tour.

## Inspect your first workload

1. In Overview, check **Needs Attention** for a resource to investigate. Or press **22** for Explore and type `:pods` in navigation.
2. Check the context and namespace in the header. Use **n** to select a namespace and **/** to filter the list.
3. Select a Pod and press Enter. Describe summarizes its reported state. Open Events or Related for supporting evidence.
4. Press **l** for logs. Use **o** if you need another container, **/** to find text in the retained buffer, and Esc to return to details.
5. If metrics are available, press **I** in Describe to inspect CPU and memory samples. Missing metrics remain unavailable rather than zero.

You have now moved from cluster health to an individual workload and its evidence. [Kubernetes Explore](../integrations/kubernetes/README.md) covers resource actions, YAML editing, container terminals, and charts. [Overview](../guides/overview.md) explains the dashboard's counts and coverage.

## Leave and return

Run `:quit` while navigating, or press **Ctrl+C** outside native agent input. Plous keeps your saved settings, conversations, and drafts in the same data directory. Each launch starts with the current kubeconfig default.

## Next steps

- [Investigate a restarting Pod](../guides/investigate-restarting-pod.md) in a disposable playground.
- [Save workspaces and connections](../guides/workspaces.md) for your existing clusters.
- [Configure the AI assistant](../guides/ai-chat.md) when you want help interpreting evidence.
- [Update or uninstall](installation.md#update) Plous.
