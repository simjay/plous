# Investigate a restarting Pod

[Documentation](../README.md) · [Kubernetes playground](../integrations/kubernetes/playground.md)

Find a deliberate crash, read its previous logs, and identify the command that caused it. No AI provider is needed.

## Before you begin

Install [Plous](../getting-started/installation.md) and prepare a disposable cluster with the [playground requirements](../integrations/kubernetes/playground.md#before-starting).

Run shell commands from the Plous checkout. Examples use context `minikube`. Replace that name consistently if yours differs.

Setup and cleanup affect the playground namespaces, including their PVCs. Use a cluster reserved for these examples.

## 1. Create the example

```sh
python3 scripts/playground.py plan --context minikube --failures crash
python3 scripts/playground.py setup --context minikube --failures crash
```

The failing Pod is `broken-crash`, with container `example` in namespace `plous-failures`. Wait for its image to download and the container to exit.

## 2. Find the Pod in Plous

```sh
plous --data-dir /tmp/plous-pod-tutorial
```

1. Use `:K` to select `minikube` and check the header.
2. Press `22`, then type `:pods` and Enter.
3. Press `n` and choose `plous-failures`.
4. Press `/`, type `broken-crash`, and Enter.
5. Open the Pod with Enter.

The restart count should grow. Depending on timing, the container may be running, terminated, or waiting in CrashLoopBackOff. Use `:g` after another exit if needed.

An image-pull or scheduling error means the example has not reached the intended crash yet.

## 3. Follow status, events, and logs

In Describe, check the container's current and last termination state. It is designed to exit with code `1`.

Press `e` for Events. Look for repeated starts and backoff warnings.

Press `l` for logs, then `p` for the previous instance. Use `o` to choose `example` if needed. Look for:

```text
ERROR deliberate crash; previous logs should retain this
```

Previous logs require a terminated instance. If unavailable, wait for another exit and reload with `g`, or read the current instance while it runs.

## 4. Check the cause

Press Esc, then `y` for YAML. Find the `example` container's command. The [fixture](../../tests/fixtures/playground/chart/templates/failures.yaml) prints the message, sleeps, and exits:

```sh
echo 'ERROR deliberate crash; previous logs should retain this'
sleep 3
exit 1
```

The nonzero exit causes the restart. CrashLoopBackOff is Kubernetes's retry delay, not the underlying cause.

No edit is needed for this exercise. In a real workload, inspect its owning controller before changing configuration. Restarting the same failing command will not fix it.

## 5. Clean up

Exit Plous with `:quit`, then run:

```sh
python3 scripts/playground.py cleanup --context minikube
```

This removes both healthy and broken playground examples. It keeps the cluster, kubeconfig, and local Plous profile. See [cleanup limits](../integrations/kubernetes/playground.md#status-reset-and-cleanup).

## Next steps

Try [other failure cases](../integrations/kubernetes/playground.md#select-failure-cases) or investigate with [plous-debug](../agent-skills/plous-debug.md).
