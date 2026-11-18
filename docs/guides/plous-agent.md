# Plous agent

The built-in agent runs in the right panel. It can read the selected Kubernetes cluster and save the evidence it used for each reply. General chat is available before you select a cluster.

## Connect a provider

1. [Install and launch Plous](../getting-started/README.md#install-and-launch).
2. Press `ss` and open **Application → Agent → Plous → Add provider connection**.
3. Choose OpenAI or Anthropic, enter the API key, and save the connection.
4. Back in **Agent**, save **Plous** as your selection. Select the cluster you want to inspect and press `cc` to start typing.

## Choose a model

Type `/model` into an empty draft, then press Enter. Choose a model under one of your saved connections. Your choice applies across workspaces. See [provider settings](../reference/configuration.md#ai-connections) and [how Plous handles AI requests](../reference/data-and-privacy.md#ai-requests).

## Investigate a workload

Check the cluster in the assistant header. Then select a workload or name it in your question. For example, “Why is checkout restarting in the shop namespace? Check the failing container’s logs and recent Pod events.”

Press Esc after the reply, select it with `[` or `]`, then press Enter for **Sources**. The observation shows what the agent saw and when it saw it. A live view may show newer data.

## Manage conversations

| Key | Action |
| --- | --- |
| `cc` | Start typing. Enter sends, Shift+Enter adds a newline, and Esc keeps the draft. |
| `:C` / `:H` | Start a conversation or open history |
| `:X` | Stop a reply |
| `:A` | Open the wider reply reader |
| `:p` | Restore the last prompt into an empty draft |

Drafts and conversations are saved per cluster connection. Moving panels leaves the reply running. See [assistant controls](../reference/keymap.md#ai-assistant) or work through the [restarting Pod exercise](investigate-restarting-pod.md).
