# Kubernetes playground

[Documentation](../../README.md) · [Kubernetes](README.md)

Practice with disposable workloads in an existing cluster. Run commands from a Plous checkout. The packaged application does not include the playground script.

Start with [Investigate a restarting Pod](../../guides/investigate-restarting-pod.md) for a guided exercise. Argo CD and Flux use a [separate playground](../gitops/playground.md).

## Before starting

You need Python 3.9+, Helm 3.13+ or Helm 4, kubectl, and a disposable Kubernetes 1.26+ cluster. Use a default StorageClass or supply `--storage-class NAME`. Images and charts need network access.

Examples use the exact context `minikube`. Every operation requires `--context`. Add `--kubeconfig PATH` for another file. The script does not change active contexts or saved Helm repositories.

Reserve `plous-demo`, `plous-failures`, `plous-cert-manager`, `plous-ingress`, `plous-metrics`, and `plous-list-a` through `plous-list-c`. Reset and cleanup delete their contents, including PVCs. The script refuses namespaces without its ownership label and unexpected releases.

## Install

Use `plan` to inspect the selection without cluster access or downloads. Use `setup` for healthy workloads, and add `--failures all` for broken examples.

```sh
# Preview locally
python3 scripts/playground.py plan --context minikube --failures all

# Install healthy examples
python3 scripts/playground.py setup --context minikube

# Include deliberate failures
python3 scripts/playground.py setup --context minikube --failures all
```

Healthy releases wait up to five minutes each. Deliberate failures do not wait for readiness. Use `--timeout-seconds 600` for slow provisioning.

Repeating setup updates the releases. The failure list replaces the old selection. Setup without `--failures` removes previous failure fixtures. Reapply optional flags when updating or resetting.

## What to explore

In Plous, select the cluster, press `22`, and choose namespace `plous-demo`.

| Resource | Try this |
| --- | --- |
| `frontend-podinfo` and `backend-podinfo` Deployments | Follow Related links through ReplicaSets, Pods, and Services. Scale or restart the frontend. |
| `log-lab` Deployment | Open Logs. The `sh` container emits continuous messages, Unicode, long lines, and sample errors. Search, select, copy, hold your position, and jump live. |
| `log-lab` containers | Choose `prepare` for completed init logs, `bash` for another stream, or `no-shell` for an image without an interactive shell. |
| `log-lab` Exec | Use `/bin/sh` in `sh`. For `bash`, choose Custom command and enter `/usr/local/bin/bash`. Try either shell in `no-shell` to inspect launch failure and explicit retry. |
| `playground-config` ConfigMap | Open YAML and search the 100 example entries. Edit a value and save with `:w`. Optionally preview the diff with `:v` first. |
| `playground-credentials` Secret | Practice reveal, value inspection, and editing with a dummy password. |
| `store` StatefulSet | Follow its headless Service, Pod, PVC, and PV. The container writes a counter to its 128 MiB volume and serves it over HTTP. Restart and inspect the continuing counter. |
| `node-reporter` DaemonSet | Inspect the Pod on each eligible node and its logs. |
| `completed-example` Job | Inspect successful completion and retained logs. |
| `periodic-report` CronJob | Wait for the five-minute schedule and follow its Job/Pod relationships. History is limited to one successful and one failed Job. |
| `playground-observer` ServiceAccount, Role, RoleBinding | Inspect read permissions. Its Role omits Secrets, Exec, and writes. Generate its separate kubeconfig with the `credentials` action below. Adding the fixture does not change your identity. |
| LimitRange, ResourceQuota, NetworkPolicy, frontend PDB | Inspect defaults, limits, selectors, and disruption settings. The sample NetworkPolicy allows traffic and does not test enforcement. |

```sh
i=0; while [ "$i" -lt 500 ]; do echo "review example $i"; i=$((i + 1)); done
```

In Exec, Ctrl+] opens output review and Ctrl+C exits Plous. The loop above provides scrollback practice.

For an edit conflict, keep a modified ConfigMap draft open and change the same object from another terminal. Saving should report the conflict and keep your draft.

## Select failure cases

Choose failures by comma-separated names, then inspect namespace `plous-failures`. Allow time for backoff states to appear.

```sh
python3 scripts/playground.py setup --context minikube --failures crash,readiness,storage
```

| Name | Object | Expected result |
| --- | --- | --- |
| `crash` | `broken-crash` Pod | Repeated exits, then CrashLoopBackOff. Previous-run logs contain the example error. |
| `image` | `broken-image` Pod | ImagePullBackOff for an intentionally nonexistent image tag. |
| `readiness` | `broken-readiness` Pod and Service | Running but unready. Service endpoints report no ready backend. |
| `scheduling` | `broken-scheduling` Pod | Pending because no node matches the deliberately missing label. |
| `config` | `broken-config` Pod | CreateContainerConfigError for a missing ConfigMap. |
| `storage` | `broken-storage` Pod and PVC | Pending because the requested StorageClass does not exist. |
| `init` | `broken-init` Pod | A failing init container prevents the application container from starting. |
| `job` | `broken-job` Job | A failed Pod and Job with retries disabled. |

These fixtures have resource limits and do not deliberately exhaust node memory or disk.

## Add custom resources and TLS examples

This adds cert-manager, a self-signed Issuer, a Certificate, and a generated TLS Secret. The certificate is for local testing and is not publicly trusted.

```sh
python3 scripts/playground.py setup --context minikube --failures all --with-cert-manager
```

Press `g` in a resource table to discover the new APIs.

Do not use this option with an unrelated cert-manager installation. It installs cluster-wide CRDs, RBAC, and webhooks. Cleanup removes its CRDs and refuses if cert-manager objects exist outside `plous-demo`.

## Extended playground

Add ingress, metrics, autoscaling, rollout examples, and 300 ConfigMaps:

```sh
python3 scripts/playground.py setup --context minikube \
  --failures all --with-cert-manager --with-traefik \
  --with-metrics-server --with-rollout --list-count 100 \
  --timeout-seconds 600
```

Reapply the same flags when updating. Omitting HPA, rollout, or list flags removes those examples. Installed controllers stay until cleanup.

Traefik watches `plous-demo`, uses the non-default `plous-playground` IngressClass, and exposes an internal Service. It does not install Traefik CRDs or expose its dashboard.

The metrics-server option skips kubelet certificate verification for disposable clusters. It does not change kubeconfig. Setup refuses unrelated metrics API registrations or IngressClass ownership.

`--list-count` accepts 0–500 ConfigMaps per namespace and starts no Pods.

### Follow an ingress route

Open the `frontend` Ingress in `plous-demo`. Follow its class, route, TLS Secret, and Service.

For a local request, keep the port-forward command running in another terminal. The curl commands use the playground hostname without changing DNS.

```sh
kubectl --context minikube -n plous-ingress port-forward service/plous-ingress 8080:80 8443:443
```

```sh
curl -H 'Host: plous.local' http://127.0.0.1:8080/
curl --insecure --resolve plous.local:8443:127.0.0.1 https://plous.local:8443/
```

HTTPS uses the self-signed sample certificate. Stop port forwarding when finished.

### Exercise autoscaling

Start a fixed-duration load Job:

```sh
python3 scripts/playground.py load --context minikube --load-seconds 180
```

The Job uses eight workers with a combined half-CPU limit. Duration accepts 30–600 seconds. Only one load Job can run at a time. Finished Jobs expire after ten minutes.

Watch `frontend-podinfo` in HorizontalPodAutoscalers, Deployments, and Pods. Its HPA targets 70% CPU and at most three replicas. Scaling depends on measured load and is not guaranteed. Scale-down can take several minutes.

Open workload or Pod Describe to collect metrics. `I` inspects charts, `W` changes the window, and `P` follows latest. See [chart meanings](README.md#usage-and-charts).

### Add a restricted connection

Create a separate read-only kubeconfig:

```sh
python3 scripts/playground.py credentials --context minikube \
  --output .local/playground/minikube-observer.kubeconfig
```

The private file contains a `minikube-plous-observer` context and a temporary ServiceAccount token. The script requests 24 hours and prints the granted expiry, without printing the token or copying administrator credentials.

Add that file in Plous, refresh contexts, and choose `plous-demo`. The identity can read the example workloads and logs, but not Secrets, Exec, writes, all namespaces, or namespace lists. Enter `plous-demo` manually if namespace discovery fails.

Rerun to renew. Only files marked for the same source context are replaced. Keep `.local` out of source control. Cleanup invalidates the token by deleting its ServiceAccount but leaves the local file.

### Break and recover a rollout

Use `rollout-break` to set a missing image on `rollout-lab`. Inspect its ReplicaSet, Pod, and conditions. Use `rollout-recover` to restore the recorded image.

```sh
python3 scripts/playground.py rollout-break --context minikube
```

```sh
python3 scripts/playground.py rollout-recover --context minikube
```

The two healthy replicas stay available. The failure deadline is 45 seconds, though controller status can arrive later. These commands leave the frontend unchanged. Setup also restores the healthy chart image.

### Exercise validation and concurrent edits

Edit `playground-config` in Plous and leave its draft open. Run this in another terminal:

```sh
python3 scripts/playground.py edit-conflict --context minikube
```

The command adds `data.concurrent-edit` without replacing other data. Return to Plous and validate or save to inspect the conflict.

Also try malformed YAML, an object where a ConfigMap string is required, or an immutable Deployment selector change. Read the error, then correct or discard the draft.

## Use existing metrics and ingress controllers

For an existing Minikube metrics-server, enable its addon if needed and use `--with-hpa`. For an existing ingress controller, supply its class with `--ingress-class`.

```sh
minikube -p minikube addons enable metrics-server
python3 scripts/playground.py setup --context minikube --with-hpa
```

```sh
python3 scripts/playground.py setup --context minikube --ingress-class traefik --with-cert-manager
```

`--with-hpa` checks the Metrics API before installation. The HPA may override manual replica changes.

`--ingress-class` does not install a controller, change DNS, or start a tunnel. The playground does not install Prometheus or Grafana.

## Status, reset, and cleanup

`status` reads resources. `reset` removes the owned playground and reinstalls the requested selection. `cleanup` removes the owned releases and namespaces.

```sh
python3 scripts/playground.py status --context minikube
python3 scripts/playground.py reset --context minikube --failures all --with-cert-manager
python3 scripts/playground.py cleanup --context minikube
```

Use the same extended flags with `reset` to recreate that selection.

Cleanup removes sample certificates before cert-manager. It refuses to remove metrics-server while outside HPAs exist, or Traefik while outside Ingresses use its class. It leaves the cluster, addons, and unrelated namespaces intact.

PVC deletion follows the StorageClass reclaim policy. Retain can leave PVs and storage behind. Cleanup does not bypass finalizers or force storage removal. Inspect terminating resources before retrying a timed-out cleanup.

## Render and verify locally

The [fixtures](../../../tests/fixtures/playground) and [version pins](../../../tests/fixtures/playground/versions.json) define these examples.

Render the local chart and run offline checks:

```sh
python3 scripts/playground.py render --failures all --with-cert-manager > /tmp/plous-playground.yaml
make test-playground
```

Render prints only the local fixture chart. It does not download upstream charts or contact Kubernetes.

Tests use fake CLIs and temporary storage. With Helm installed, they also lint and render. They do not verify a live deployment. See the separate [Minikube acceptance tests](../../../tests/README.md#explicit-minikube-acceptance).
