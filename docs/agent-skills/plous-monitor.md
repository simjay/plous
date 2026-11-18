# Monitor Kubernetes with plous-monitor

Use `plous-monitor` to prepare a scheduled Kubernetes check or review alerts from a saved monitor. Install it through your [coding agent’s setup](../guides/ai-chat.md#choose-an-agent).

## Prepare a monitor

> Prepare a monitor named `checkout` for Deployment `checkout` and Service `checkout-api` in `prod/payments` every two minutes.

Review the generated command, then run it in a terminal. See the [monitor command reference](../reference/configuration.md#monitors) for targets and intervals.

## Review an alert

Ask “What changed in the `checkout` monitor?” The skill reads saved alerts for the agent’s selected context. To investigate an alert, give its ID to [plous-debug](plous-debug.md). To see the alerts in the terminal UI, open [Monitors](../guides/navigation.md#monitor-cluster-health).
