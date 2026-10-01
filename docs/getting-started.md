# Create your first workspace

[Documentation](README.md)

Start with a workspace, then add the tools you use. A cluster and AI account are optional.

## Before you begin

Use macOS or Linux and an interactive terminal at least **80 columns by 16 rows**. Building from source requires Git, Make, and Go **1.26.4 or newer**. See [Installation](installation.md) for setup and updates.

## Build and launch

Run these commands in your shell:

```sh
git clone https://github.com/simjay/plous.git
cd plous
make run
```

To launch the built executable later, run `./bin/plous`. It saves data under `~/.plous/`. For an independent profile:

```sh
./bin/plous --data-dir /tmp/plous-demo
```

A separate profile isolates saved settings and conversations. Its connections still point to the infrastructure you choose. See [Data and privacy](data-and-privacy.md) for profiles and backups.

## Create your first workspace

A workspace groups connections for a project or environment. It can hold several tools and multiple connections for each tool.

1. Press **Enter** in the empty main panel.
2. Enter a name, such as `Commerce`.
3. Press **Enter** to move to **Save**, then **Enter** again to create it.
4. Choose a tool in the setup picker, or press **Esc** to leave the workspace empty.

Your workspace appears in the left panel. Selecting it opens its saved configuration in the main panel. It remains saved if you skip tool setup.

## Add a tool connection

Select **+ Add tool** under the workspace whenever you want to add a connection. Choose Kubernetes, Helm, Kustomize, Terraform, Argo, or Kargo and complete the form.

Each tool has its own setup and capabilities. The [integration guide](integrations.md) covers those differences. A Terraform directory and a Kubernetes context can belong to the same workspace; they remain separate connections.

## Learn five controls

| Key | Use it to |
| --- | --- |
| Tab / Shift+Tab | Move between the workspace, main, and assistant panels. |
| Up / Down, then Enter | Select and open an item. |
| `/` | Search the focused panel. |
| Esc | Close the current search, editor, or detail view. |
| `hh` | Open Help while navigating. |

Settings uses arrows and Enter. Tab does not leave that window. When typing in a field, shortcuts become ordinary text.

Press **11** for Overview, **22** for Explore, or **33** for Activity. See [Interface and navigation](navigation.md) for the full tour.

## Add AI when you need it

Follow [AI assistant](ai-chat.md) to connect a vendor and start a conversation. You can discuss a configuration, ask a technical question, or return to earlier workspace conversations. You choose when to send a message.

## Leave and return

Run `:quit` while navigating, or use **Ctrl+C** from anywhere. Plous saves the session and chat drafts. It does not apply unsaved form edits.

Launch with the same data directory to return to your workspaces, conversations, and navigation preferences.

Next: [Workspaces and connections](workspaces.md) or [Tool integrations](integrations.md).
