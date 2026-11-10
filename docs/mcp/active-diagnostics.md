# Active diagnostics

[MCP server](README.md) · [Tools and permissions](../reference/mcp.md#active-diagnostics)

The server exposes reads by default. Add diagnostic grants to your MCP client’s server arguments for the environment you intend to test. Equivalent launch commands are shown below; the client owns the stdio connection.

Enable bounded commands in existing containers with `--allow-exec`:

```sh
plous mcp serve --agent codex --context staging --namespace shop --allow-exec
```

`kubernetes_exec_run` runs one noninteractive command in an existing container, using an argument array and the exact Pod UID obtained from a read. No shell, stdin, or TTY is inserted. The executable must exist in that container. Commands can modify application state. A timeout or broken connection leaves the remote outcome unknown and does not guarantee that the remote process stopped.

For temporary tests, choose a diagnostic image containing `curl`, `nslookup`, and `nc`, with support for running as UID 65532 on a read-only filesystem:

```sh
plous mcp serve --agent codex --context staging --namespace shop \
  --diagnostic-image registry.example.com/team/diagnostics@sha256:YOUR_IMAGE_DIGEST
```

`kubernetes_diagnostic_run` supports one HTTP GET, DNS lookup, or TCP connection check. The agent supplies the test and target; it cannot override the configured image, attach credentials or volumes, copy application labels, or supply an arbitrary Pod manifest. HTTP does not follow redirects or retry. An HTTP GET can still change application data if the endpoint implements it that way.

Temporary tests create a Job with one container, no retries, a runtime deadline, resource limits, no mounted service-account token, and no elevated privileges. Plous collects the exit code and bounded logs, then requests identity-checked cleanup. Finished Job TTL is a fallback if the client or Plous exits abruptly. Cleanup failures and pending deletion are reported rather than presented as confirmed removal.

Both execution capabilities are restricted to the captured default namespace. Resource reads may still use other namespaces allowed by Kubernetes credentials. Kubernetes RBAC must permit the requested operation: exec needs access to `pods/exec`; temporary diagnostics need Job creation/get/deletion, Pod list/get, and Pod logs.

Each operation lasts at most 120 seconds, plus up to 10 seconds for temporary cleanup. Combined exec output and diagnostic log delivery are bounded to 64 KiB. A temporary Pod has its own identity, labels, and network policy. A successful test there does not establish that the application can make the same request.

Example requests:

> From the checkout container, test whether payments.shop.svc resolves. Use the exact Pod identity and report where the test ran.

> Use a temporary diagnostic workload to GET http://payments.shop.svc:8080/health. Report its response, exit code, and cleanup status.

To enable these same tools for assistants inside the TUI, launch `plous --allow-exec` and/or `plous --diagnostic-image IMAGE`. These are launch-time grants, not saved preferences. They apply to the connection captured by each assistant operation or native session. Personal skill discovery remains disabled inside hosted native terminals; Plous supplies the shared debugging guidance itself.

## Enable diagnostics with managed skill setup

For [plous-debug](../agent-skills/plous-debug.md), add these same grants to `plous setup codex` or `plous setup claude`, then reconnect the client. Setup installs the skill and registers its MCP entry together. Direct MCP registration works without the skill.

## Next steps

Read [scope and lifecycle](scope.md) before interpreting a timeout, cancellation, or pending cleanup. Use the [plous-debug guide](../agent-skills/plous-debug.md) for investigation and verification prompts.
