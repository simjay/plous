---
name: plous-monitor
description: Prepare commands for recurring Kubernetes checks and review saved Plous alerts. Use for Deployments, DaemonSets, and Service endpoints.
---

# Monitor Kubernetes with Plous

Confirm a unique monitor name, Kubernetes context, explicit namespace, targets, and interval. With no resource targets, Plous checks every Deployment in the namespace. The interval defaults to five minutes.

In a standalone session, use `kubernetes_context_read` to verify the current context and connection ID. Use the existing user-confirmed target selection flow if the requested connection differs. A running monitor stays pinned to its starting context.

Give the user the foreground command for the confirmed scope.

```sh
plous monitor start payments --context prod --namespace payments --deployment checkout --service checkout-api --every 5m
```

Repeat `--deployment`, `--daemonset`, or `--service` for additional resources. Include `--kubeconfig PATH` when the user needs a specific file.

Use `monitor_read` with `status` for coverage and timing, `list` for alerts, and `get` with an exact `alert_id`. Supply the captured Kubernetes connection ID. Use `monitor_name` to narrow the read. Report the last complete check, an overdue check, and relevant coverage gaps. The `next_check` fact includes retry backoff. Keep the alert ID and observation reference for follow-up. Use `plous-debug` for user-requested diagnosis.

Use `plous monitor list` to see saved monitors. Use `plous monitor stop NAME` or Ctrl+C in the monitor's terminal to stop it. Use `plous monitor start NAME` to resume and add `--every` to change the interval. In the TUI, `:4` opens running monitors and `:j` opens all saved alerts.

To replace a stopped monitor, give the user a start command for the new scope. Plous prompts before clearing the existing alerts. To delete the saved settings and alerts, give the user `plous monitor remove NAME` to confirm in their terminal.
