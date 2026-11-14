# Active diagnostics

[MCP server](README.md) · [Tools and permissions](../reference/mcp.md#active-diagnostics)

Agent tools read cluster data by default. Enable commands or temporary tests only for the cluster and namespace you intend to investigate. The examples pin a context. If you omit `--context`, these capabilities follow the current kubeconfig context.

## Run commands in a container

Add `--allow-exec` to your client's server arguments:

```sh
plous mcp serve \
  --agent codex \
  --context staging \
  --namespace shop \
  --allow-exec
```

`kubernetes_exec_run` runs one noninteractive command in an existing container. It uses an argument array and a Pod UID from a previous read. Plous does not add a shell, stdin, or a TTY.

The executable must exist in the container. Commands can change application state. A timeout does not guarantee that the remote command stopped.

## Run a temporary test

Choose an image containing `curl`, `nslookup`, and `nc`. It must run as UID 65532 with a read-only filesystem.

```sh
plous mcp serve \
  --agent codex \
  --context staging \
  --namespace shop \
  --diagnostic-image registry.example.com/team/diagnostics@sha256:YOUR_IMAGE_DIGEST
```

`kubernetes_diagnostic_run` performs one HTTP GET, DNS lookup, or TCP connection check in a temporary Job. It accepts the probe type and target. Plous builds the command and Job from the probe and the configured image.

HTTP tests do not follow redirects or retry. Choose an endpoint that is safe to call. Even a GET can change data if the application implements it that way.

Plous collects the result and logs, then requests Job deletion. It reports failed or pending cleanup. Job TTL provides fallback cleanup if the process exits.

## Permissions and limits

| Requirement | Details |
| --- | --- |
| Namespace | Commands and tests use the configured default namespace |
| Exec RBAC | Access to `pods/exec` |
| Temporary-test RBAC | Create, read, and delete Jobs. List and read Pods and their logs. |
| Deadline | Up to 120 seconds per operation, plus 10 seconds for temporary cleanup |
| Output | Up to 64 KiB of combined command output or diagnostic logs |
| Temporary Job | One container, no retries, resource limits, no mounted service-account token, no elevated privileges |

A temporary Pod has different labels and network policies from your application. A successful request there does not prove that the application can make it.

Example requests:

> In the checkout container, check whether payments.shop.svc resolves. Tell me which Pod and container you used.

> From a temporary diagnostic Pod, GET http://payments.shop.svc:8080/health. Report the response and whether cleanup finished.

## Enable diagnostics with managed skill setup

Add the same options to `plous setup codex` or `plous setup claude`, then reconnect. See [plous-debug](../agent-skills/plous-debug.md).

For the assistant panel inside Plous, launch with `plous --allow-exec` or `plous --diagnostic-image IMAGE`. These options apply only to that launch and are not saved preferences.

## Next steps

Read [scope and saved evidence](scope.md) for cancellation and storage behavior.
