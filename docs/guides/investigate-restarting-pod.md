# Investigate a restarting Pod

[Documentation](../README.md) · [Getting started](../getting-started/README.md) · [Kubernetes playground](../integrations/kubernetes/playground.md)

Follow a Pod from a restart symptom to its previous logs and the command that caused the exit. This walkthrough uses the playground's deliberate `crash` failure. You do not need an AI provider.

## Before you begin

Install Plous using [Installation](../getting-started/installation.md). Have a disposable cluster and the [playground prerequisites](../integrations/kubernetes/playground.md#before-starting) ready, including Helm, kubectl, Python, and a default StorageClass.

Run the shell commands from the Plous checkout. The examples use the exact context name `minikube`; replace it consistently if yours differs. Setup creates healthy demo workloads and a deliberately failing Pod. Cleanup deletes the owned playground resources, including PVCs, so use a disposable cluster reserved for these examples.

## 1. Create the example

Preview the selection locally, then install it:

```sh
python3 scripts/playground.py plan --context minikube --failures crash
python3 scripts/playground.py setup --context minikube --failures crash
```

The failing Pod is `broken-crash`, its container is `example`, and its namespace is `plous-failures`. Healthy releases wait for readiness; deliberate failures do not. Allow time for the image to download and the container to exit repeatedly.

## 2. Find the Pod in Plous

Launch a separate Plous profile:

```sh
plous --data-dir /tmp/plous-pod-tutorial
```

1. Run **`:K`** and select `minikube`. Verify the context shown in the header.
2. Press **22** to enter Explore, then run **`:pods`**.
3. Press **n** and choose `plous-failures`.
4. Press **/**, type `broken-crash`, and press Enter to apply the filter.
5. Select the Pod and press Enter to open Describe.

Expect a growing restart count and a non-ready container. Depending on when you read it, the container can be running briefly, terminated, or waiting in `CrashLoopBackOff`. Refresh with **`:g`** after another exit if necessary. An image-pull or scheduling error means the fixture has not reached the intended failure yet.

## 3. Follow status, events, and logs

In Describe, inspect the container's current and last termination state. The command is designed to exit with code `1`; the Pod phase alone does not explain the failure.

Press **e** for Events. Look for repeated starts and a backoff warning. Events depend on Kubernetes retention and permissions, so missing events do not establish that the container never restarted.

Press **l** for Logs, then **p** for the previous container instance. If needed, press **o** and select `example`. Look for this fixture message:

```text
ERROR deliberate crash; previous logs should retain this
```

Previous logs need a terminated instance. If they are unavailable, wait for another exit and reload with **g**, or inspect the current instance while it runs. Plous searches only the retained log buffer; **/** does not search every historical instance.

## 4. Check the cause

Press Esc to return from Logs, then **y** to open the Pod YAML. Find the `example` container's command. The [fixture source](../../tests/fixtures/playground/chart/templates/failures.yaml) defines it as:

```sh
echo 'ERROR deliberate crash; previous logs should retain this'; sleep 3; exit 1
```

The conclusion is specific: this container prints the message and exits unsuccessfully after three seconds. Kubernetes restarts it and backs off after repeated failures. The status, previous log, and configured command support that explanation. `CrashLoopBackOff` is the retry state, not the underlying cause.

No resource edit is needed to complete this exercise. For a real workload, inspect its owning controller and configuration before choosing a correction; restarting an unchanged workload does not remove the failing command.

## 5. Clean up

Exit Plous with **`:quit`**, then remove the owned playground:

```sh
python3 scripts/playground.py cleanup --context minikube
```

Cleanup removes the healthy demo and failure fixtures together. It leaves the cluster, kubeconfig, and local Plous profile in place. See [cleanup limits](../integrations/kubernetes/playground.md#status-reset-and-cleanup) for storage and teardown details.

## Next steps

Try the playground's [readiness, image, or scheduling failures](../integrations/kubernetes/playground.md#select-failure-cases) and compare their evidence. For assisted investigation, follow [Debug from your coding agent](../agent-skills/plous-debug.md). Keep [Kubernetes resource actions](../integrations/kubernetes/README.md#explore-resources) nearby for deeper inspection.
