# Quickstart

Open Plous, find a Pod, and read its logs from your terminal.

## Install and launch

<a name="before-you-begin"></a>

Use Homebrew on macOS or Linux with a working kubeconfig. See [Installation and updates](installation.md) for other install methods.

```sh
brew install simjay/tap/plous
plous
```

Use a terminal at least 80 columns wide and 16 rows tall.

## Start with Current Workspace

Plous opens your current Kubernetes context. Check the context name in the header before continuing.

To choose another context, type `:K` and press Enter. This changes the selection in Plous without editing your kubeconfig.

If Plous cannot connect, open **Settings → Choose Kubeconfig** or follow [connection troubleshooting](../reference/troubleshooting.md).

## Inspect your first workload

1. Press `22` to open Explore.
2. Type `:pods` and press Enter.
3. Press `n` to choose a namespace.
4. Select a Pod and press Enter.
5. Press `l` to open its logs. Press `o` to choose another container.
6. Press Esc to return to the Pod.

<!-- plous-example: terminal-replay -->

## Learn the basic controls

Use `Alt+Right` to move to the next visible panel and `Alt+Left` to move back.

<!-- plous-example: panel-focus -->

| Key | Action |
| --- | --- |
| Up / Down, Enter | Select and open an item |
| `/` | Search the current panel |
| Esc | Close the current search, input, or detail view |
| `hh` | Open Help while navigating |
| `ff` | Expand or restore the main panel |

Use `11` for Overview, `22` for Explore, and `33` for Activity. See the [keymap](../reference/keymap.md) for all controls.

## Leave and return

Type `:quit` and press Enter. Plous saves your settings and conversations.

## Next steps

- [Investigate a restarting Pod](../guides/investigate-restarting-pod.md) in a disposable cluster.
- [Save a workspace](../guides/workspaces.md) for clusters you use often.
- [Set up an AI assistant](../guides/ai-chat.md), [connect an MCP client](../mcp/setup.md), or install [plous-debug](../agent-skills/plous-debug.md).
