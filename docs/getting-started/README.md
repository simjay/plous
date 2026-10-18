# Getting started

[Documentation](../README.md)

Get Plous running, inspect a workload or render some configuration, and add an assistant when you need one. Start with your terminal's Kubernetes context and working directory; saved workspaces are optional.

Choose your path after building:

- **Have a cluster?** [Inspect a workload](#inspect-your-first-workload).
- **Have configuration files?** [Explore Code](#try-code-without-a-cluster).
- **Want AI help?** [Set up an assistant](#add-ai-when-you-need-it).
- **Want a disposable example?** Use the [Kubernetes playground](../integrations/kubernetes/playground.md) or [GitOps playground](../integrations/gitops/playground.md).

## Before you begin

Use macOS or Linux and an interactive terminal at least **80 columns by 16 rows**. Building from source requires Git, Make, and Go **1.26.4 or newer**. Plous is an early development build; native Windows is not supported.

Plous embeds the Go libraries for Kubernetes access, Helm, Kustomize, and Git source reads. Running these features does not require `kubectl`, `helm`, `kustomize`, or `git` executables. A kubeconfig that uses an external authentication helper still requires that helper. Git SSH sources use an authenticated SSH agent and known-host verification; private HTTPS sources use host-specific entries in `~/.netrc` (`~/_netrc` on Windows). Git credential-helper commands are not executed. The Markdown guides need no additional tools to read.

## Build and launch

Run these commands in your shell:

```sh
git clone https://github.com/simjay/plous.git
cd plous
make run
```

To launch the built executable later from this checkout, run `./bin/plous`. To open your own configuration directory in Code, [install it on PATH](#install-on-your-path), change to that directory, and run `plous`.

Plous saves data under `~/.plous/`. For an independent profile:

```sh
./bin/plous --data-dir /tmp/plous-demo
```

A separate profile isolates saved settings and conversations. Its connections still point to the infrastructure you choose. See [Data and privacy](../reference/data-and-privacy.md) for profiles and backups.

## Start with Current Workspace

Plous opens **Overview** for the merged kubeconfig `current-context`. Switch to **Explore** with `22` or `:2` to see Pods in its namespace, or `default` when unset. It reads the inherited `KUBECONFIG` path list, otherwise `~/.kube/config`. It contacts only the selected target. Use `:K` / `:connections` to search local context names and select another target within Plous. Selection never changes kubeconfig.

If configuration is missing or invalid, choose a context with Enter or `:K`, or open **Settings → Choose Kubeconfig**. Use `:g` to retry after correcting the file. Code, General chat, and AI setup remain available.

**Current Workspace → Code** contains the directory where you launched Plous. Open its source row to browse Files, or expand it for Helm, Kustomize, Argo, and Flux. The first scan starts when you open or expand the source, open Code, or request a refresh. The directory stays fixed for that launch and does not require Git or Kubernetes.

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

## Try Code without a cluster

1. Launch Plous from a directory containing your manifests, or add a saved Code source.
2. Open **Current Workspace → Code → your directory → Files** to browse. Local text opens in the editor; `:q` closes it.
3. Open **Kustomize** or **Helm**, select a configuration, and use **r Render**. Selecting an item alone does not run a build.
4. Inspect generated YAML, findings, or captured field history. Rendering reads saved inputs and does not deploy anything.

An empty technology browser means no matching configuration was found in the scanned source. Use Files for ordinary browsing. The [Code guide](../guides/code.md) walks through editing, rendering, and comparing output.

## Add AI when you need it

Fresh profiles use the built-in Plous agent. Open **Settings → Agent → Plous → Add provider connection** to configure OpenAI or Anthropic, then press **cc** to write a message. With a Pod selected, try “Why is this Pod restarting? Check status, events, and recent logs.”

To use your installed Claude Code or Codex instead, select the intended cluster or source first, choose **Settings → Application → Agent**, and Save. Plous connects its tools through MCP automatically. External agents use their own setup and authentication; they need no Plous API key.

See [AI chat](../guides/ai-chat.md) for setup, model selection, and Sources, or [Plous MCP](../guides/plous-mcp.md) for native-agent examples and session scope.

## Save a workspace when you need one

A workspace groups connections for a project or environment. It can hold several tools and multiple connections for each tool.

1. Run `:n`, or choose **Workspaces → + New Workspace**.
2. Enter a name, such as `Commerce`.
3. Press **Enter** to move to **Save**, then **Enter** again to create it.
4. Choose a connection in the setup picker, or press **Esc** to leave the workspace empty.

You can also use **Settings → Save as Workspace** from Current Workspace to save its available cluster connection and launch directory as explicit connections. Earlier conversations remain in Current Workspace history.

Your workspace appears under **Workspaces** in the left panel. Selecting it opens its saved configuration in the main panel. It remains saved if you skip tool setup.

## Add a tool connection

Select **+ Add Connection** under the workspace whenever you want to add a connection. Choose **Monitor → Kubernetes Cluster** or **Code → Source**. Monitor exposes Kubernetes and detected Helm releases from the same cluster, plus [Argo and Flux](../integrations/gitops/README.md) when their APIs are discovered. Each Code source exposes **Files**, **Helm**, **Kustomize**, **Argo**, and **Flux**. Use Files to browse and edit; select a supported configuration in a technology browser and use **:r** or **r** to render it. Plous detects formats from its contents automatically. Local directories do not require Git.

Each tool has its own setup and capabilities. The [integration guide](../integrations/README.md) covers those differences. A Code directory and a Kubernetes context can belong to the same workspace as separate connections. Terraform files can be browsed, but Terraform execution is unavailable.

## Leave and return

Run `:quit` while navigating, or press **Ctrl+C** outside native agent input. In Claude Code or Codex input, Ctrl+C goes to the agent; press **Ctrl+]**, then **Ctrl+C** to exit Plous. Plous saves the session and chat drafts. It does not apply unsaved form edits.

Launch with the same data directory to keep your workspaces, conversations, and navigation preferences. Each launch starts from the current kubeconfig default.

Next: [Workspaces and connections](../guides/workspaces.md) or [Tool integrations](../integrations/README.md).

## Install on your PATH

To make it available outside the checkout, copy it into a directory on your shell's `PATH`. For example:

```sh
mkdir -p "$HOME/.local/bin"
install -m 755 bin/plous "$HOME/.local/bin/plous"
```

If that directory is not on `PATH`, run `~/.local/bin/plous` or add the directory using your shell's configuration.

## Update

Close Plous and [back up its data](../reference/data-and-privacy.md#back-up-or-recover-a-profile) before trying a newer development build. Data formats can change during early development.

Update your source checkout, run `make build`, and replace any copy you installed elsewhere. Check `--version` on the executable you actually launch. Rebuilding alone does not update a separate installed copy.

## Distribution archives

Maintainers can run `make dist` to build macOS and Linux archives for amd64 and arm64. Each contains the executable, license, and Markdown guides. This does not imply that a release has been published. There is no Homebrew or other package-manager installation documented for this build.

## Uninstall

Remove the executable you installed. Workspace data remains in `~/.plous/`, or the directory supplied with `--data-dir`. Keep it if you plan to return; removing the executable does not remove credentials or conversations. See [Data and privacy](../reference/data-and-privacy.md) before deleting or moving a profile.
