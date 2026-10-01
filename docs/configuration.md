# Configuration

[Documentation](README.md)

Use Settings for saved configuration and command-line flags to choose a profile at launch. Plous manages its data files; editing revision records by hand is not a configuration interface.

## Command-line flags

Run these in your shell, outside Plous:

| Flag | Behavior |
| --- | --- |
| `--version` | Print the embedded application version and exit. |
| `--data-dir PATH` | Use an independent data directory instead of `~/.plous/`. |
| `--help` | Print command-line usage and exit. |

```sh
./bin/plous --data-dir "$HOME/plous-profiles/testing"
```

Profiles have separate workspaces, credentials, conversations, and preferences. They can still point to the same cluster. Only one process can open a profile at a time.

## Settings

Press **ss** while navigating, or run `:s`.

| Setting | Scope | Guide |
| --- | --- | --- |
| AI vendor connections and credentials | Application profile | [AI setup](ai-chat.md) |
| Selected AI model | Application profile; changed in the composer | [Choose a model](ai-chat.md#choose-a-model) |
| Workspace name and tool connections | Current workspace | [Workspaces](workspaces.md) |
| Workspace location | Local; Remote is disabled | [Workspaces](workspaces.md) |

Select **Save** to apply a form. Esc cancels to its preceding menu. Exiting the application does not apply unfinished edits.

## Tool connections

Connection fields belong to the selected tool. A workspace can combine context references, working directories, and repositories. See [Tool integrations](integrations.md#saved-tool-connections) for the required and optional fields. Saved connection changes appear in [Activity](activity.md).

## Kubernetes connections

Each connection retains its kubeconfig path, context, and optional namespace. Setup starts with `~/.kube/config`; choose **Refresh contexts** after changing that path. The shown file takes precedence over the shell's `KUBECONFIG` setting during setup.

The connection's default namespace and Explore's selected namespace are separate. Changing the picker updates the browsing scope for that connection. It does not rewrite kubeconfig or change another client's context.

Live access uses the credentials and authentication plugins in the referenced kubeconfig. Plous does not copy those credentials into its profile. See [connection setup](kubernetes.md#add-a-kubernetes-context) and [connection troubleshooting](troubleshooting.md).

## AI connections

Add OpenAI or Anthropic in Settings. Advanced fields include a display name, API proxy URL, environment-variable credential reference, and custom instructions. A proxy does not add models to the supported catalog.

Use `/model` or `/m` as a new, complete message in the composer to select a model. The selection applies across workspaces. Workspace chat captures saved connection details and the selected page when you send a message; see [AI chat](ai-chat.md) for its contents and read-only investigation scope.

## Remembered preferences

Plous restores panel visibility, selected scopes, tree folds, conversations, and chat drafts. Explore also remembers each connection's namespace, resource type, filters, and sort order.

Search text, open resource content, and unsaved form or resource edits are temporary. The panel layout adapts to terminal size; there is no user-configurable layout or theme file. See [navigation](navigation.md) for panel controls and [Data and privacy](data-and-privacy.md) for storage details.
