# Kubernetes playground

[Documentation](README.md) · [Explore Kubernetes](kubernetes-explore.md)

The playground adds disposable examples to an existing cluster so you can practice browsing, reading logs, opening terminals, and editing resources in Plous. It uses two real Podinfo Helm releases and a local fixture chart. Optional scenarios add broken resources and cert-manager.

Run these commands from a checkout of the Plous repository. The playground script is not included in the packaged application.

## Before starting

You need Python 3.9+, Helm 3.13+ or Helm 4, kubectl, and a running disposable Kubernetes cluster with a default StorageClass, or a specific class supplied with `--storage-class NAME`. The fixture chart requires Kubernetes 1.26+. Minikube's normal storage provisioner is suitable. Images and upstream charts require registry access.

Choose the exact context to use. The examples below use `minikube`. Every cluster operation requires `--context`; the script never changes your active context. Add `--kubeconfig /path/to/config` to use a particular file. It does not edit kubeconfig, Plous settings, or your saved Helm repositories.

Keep `plous-demo`, `plous-failures`, `plous-cert-manager`, `plous-ingress`, `plous-metrics`, and `plous-list-a` through `plous-list-c` reserved for this playground. Cleanup and reset delete their contents, including PVCs. The script refuses existing namespaces without its ownership label and unexpected Helm releases inside its namespaces.

## Install

Preview the selection without contacting Kubernetes or downloading charts:

```sh
python3 scripts/playground.py plan --context minikube --failures all
```

Install healthy workloads:

```sh
python3 scripts/playground.py setup --context minikube
```

Install healthy workloads and all deliberate failures:

```sh
python3 scripts/playground.py setup --context minikube --failures all
```

Healthy releases wait for readiness, with a five-minute timeout per release. Deliberate failures are installed without waiting for readiness. `setup` reports successful installation separately from the expected Pod and Job failures. Use `--timeout-seconds 600` for slower image downloads or storage provisioning.

Repeating setup updates the same releases. The selected failure list replaces the previous list; setup without `--failures` removes previously installed failure fixtures. Reapply optional flags when updating or resetting the playground. Omitting `--with-cert-manager` removes its sample certificate from the fixture chart, but keeps a previously installed controller until cleanup.

## What to explore

Select your Kubernetes connection in Plous, enter Explore with **22**, then use **n** to choose `plous-demo`.

| Resource | Try this |
| --- | --- |
| `frontend-podinfo` and `backend-podinfo` Deployments | Follow Related links through ReplicaSets, Pods, and Services. Scale or restart the frontend. |
| `log-lab` Deployment | Open Logs. The `sh` container emits continuous messages, Unicode, long lines, and sample errors. Search, select, copy, hold your position, and jump live. |
| `log-lab` containers | Choose `prepare` for completed init logs, `bash` for another stream, or `no-shell` for an image without an interactive shell. |
| `log-lab` Exec | Use `/bin/sh` in `sh`. For `bash`, choose Custom command and enter `/usr/local/bin/bash`. Try either shell in `no-shell` to inspect launch failure and explicit retry. |
| `playground-config` ConfigMap | Open YAML and search the 100 example entries. Edit a value and save with `:w`; optionally preview the diff with `:v` first. |
| `playground-credentials` Secret | Practice reveal, value inspection, and editing with a dummy password. |
| `store` StatefulSet | Follow its headless Service, Pod, PVC, and PV. The container writes a counter to its 128 MiB volume and serves it over HTTP. Restart and inspect the continuing counter. |
| `node-reporter` DaemonSet | Inspect the Pod on each eligible node and its logs. |
| `completed-example` Job | Inspect successful completion and retained logs. |
| `periodic-report` CronJob | Wait for the five-minute schedule and follow its Job/Pod relationships. History is limited to one successful and one failed Job. |
| `playground-observer` ServiceAccount, Role, RoleBinding | Inspect read permissions. Its Role omits Secrets, Exec, and writes. Generate its separate kubeconfig with the `credentials` action below; adding the fixture does not change your identity. |
| LimitRange, ResourceQuota, NetworkPolicy, frontend PDB | Inspect defaults, limits, selectors, and disruption settings. The sample NetworkPolicy allows traffic and does not test enforcement. |

In Exec, **Ctrl+]** enters output review. **Ctrl+C exits Plous**. For scrollback practice, run this inside the `sh` container:

```sh
i=0; while [ "$i" -lt 500 ]; do echo "review example $i"; i=$((i + 1)); done
```

To test a concurrent edit, leave a changed ConfigMap draft open in Plous, then modify its data in another terminal using the same explicit context. Return to Plous and use `:w` to check that the conflict preserves your draft. Setup restores fixture values; reset recreates their objects.

## Select failure cases

```sh
python3 scripts/playground.py setup --context minikube --failures crash,readiness,storage
```

Switch Plous to `plous-failures`. Failures may take a short time to settle into their backoff states.

| Name | Object | Expected result |
| --- | --- | --- |
| `crash` | `broken-crash` Pod | Repeated exits, then CrashLoopBackOff. Previous-run logs contain the example error. |
| `image` | `broken-image` Pod | ImagePullBackOff for an intentionally nonexistent image tag. |
| `readiness` | `broken-readiness` Pod and Service | Running but unready; Service endpoints report no ready backend. |
| `scheduling` | `broken-scheduling` Pod | Pending because no node matches the deliberately missing label. |
| `config` | `broken-config` Pod | CreateContainerConfigError for a missing ConfigMap. |
| `storage` | `broken-storage` Pod and PVC | Pending because the requested StorageClass does not exist. |
| `init` | `broken-init` Pod | A failing init container prevents the application container from starting. |
| `job` | `broken-job` Job | A failed Pod and Job with retries disabled. |

These cases use bounded resource requests and limits. They do not intentionally exhaust node memory or disk. The healthy log stream emits about one normal message per second plus a periodic multiline example.

## Add custom resources and TLS examples

```sh
python3 scripts/playground.py setup --context minikube --failures all --with-cert-manager
```

This installs cert-manager in `plous-cert-manager`, including its cluster-wide CRDs, RBAC, and webhooks. It also creates a namespaced self-signed Issuer, Certificate, and generated TLS Secret in `plous-demo`. No public domain, ACME account, or cloud credentials are required. The certificate is for local testing and is not publicly trusted.

If Plous was already browsing the cluster, press **g** or run **`:g`** from a resource table to discover the newly installed resource types.

The script refuses an existing cert-manager installation owned by another release. Use the core playground without this flag on such a cluster. The playground installs its CRDs with `crds.keep=false`, so uninstalling its controller also removes its CRDs. Cleanup refuses to do so if it finds cert-manager custom resources outside `plous-demo`.

## Extended playground

Install the existing examples plus an ingress controller, metrics-server, autoscaling, a rollout exercise, and 300 ConfigMaps:

```sh
python3 scripts/playground.py setup --context minikube \
  --failures all --with-cert-manager --with-traefik \
  --with-metrics-server --with-rollout --list-count 100 \
  --timeout-seconds 600
```

Reapply the same flags when updating or resetting this selection. `--with-metrics-server` also enables the frontend HPA. `--with-traefik` selects the dedicated `plous-playground` IngressClass. Both controllers are pinned Helm releases in their own playground namespaces. Traefik watches `plous-demo`, is not the default ingress class, and does not install Traefik CRDs or expose its dashboard. Its Service is internal to the cluster.

The metrics-server option is for disposable clusters: it skips verification of kubelet serving certificates to work with Minikube. This setting applies to metrics-server's kubelet requests and does not change your kubeconfig. The script refuses to adopt existing metrics API registrations or the playground IngressClass from another release. Use existing-controller options below if those components are already installed.

Omitting the controller flags keeps installed controllers until cleanup. Omitting `--with-hpa`, `--with-rollout`, or `--list-count` removes those examples on the next setup. The ConfigMaps share names across three namespaces and include long names, labels, and Unicode values. `--list-count` accepts 0 to 500 per namespace and creates no running Pods.

### Follow an ingress route

In Plous, open the `frontend` Ingress in `plous-demo`. Inspect its class, routing rules, TLS Secret, and related Service. To try the route from your computer, keep this running in a separate terminal:

```sh
kubectl --context minikube -n plous-ingress port-forward service/plous-ingress 8080:80 8443:443
```

Then request the route without changing DNS or your hosts file:

```sh
curl -H 'Host: plous.local' http://127.0.0.1:8080/
curl --insecure --resolve plous.local:8443:127.0.0.1 https://plous.local:8443/
```

The HTTPS example uses the playground's self-signed certificate. Stop port forwarding when finished.

### Exercise autoscaling

```sh
python3 scripts/playground.py load --context minikube --load-seconds 180
```

This starts one Job with eight request workers, a combined CPU limit of half a CPU, and a fixed duration. It sends requests to the frontend's token endpoint. Only one load Job may be pending or running at a time. The duration can be 30 to 600 seconds; Kubernetes also enforces a deadline and removes the completed Job after ten minutes. Cleanup removes it sooner.

Watch `frontend-podinfo` under HorizontalPodAutoscalers, Deployments, and Pods. The HPA targets 70% CPU utilization and allows up to three replicas. Scaling depends on observed CPU use; replica changes are not guaranteed merely by creating the Job. Scale-down can take several minutes after load stops. This supplies Kubernetes metrics for the HPA; it does not add usage charts to Plous.

### Add a restricted connection

```sh
python3 scripts/playground.py credentials --context minikube \
  --output .local/playground/minikube-observer.kubeconfig
```

The script writes a private kubeconfig with a separate `minikube-plous-observer` context and a time-limited ServiceAccount token. It requests 24 hours and prints the actual expiry granted by the API. It copies the cluster address and TLS configuration without copying your administrator credentials. It never prints the token or changes your active context. Renew by rerunning the same command. Existing files are replaced only when they carry this script's marker for the same source context.

In Plous, open Kubernetes **Add context**, enter that file's absolute path, refresh contexts, and select `minikube-plous-observer`. Use `plous-demo` as the namespace. This identity can read Pods, logs, Services, ConfigMaps, Events, and supported apps workloads in that namespace. Namespace listing, all-namespace browsing, Secrets, Exec, and writes are denied. If namespace listing fails, enter `plous-demo` manually. The normal Minikube connection remains available for administrative exercises.

The `.local` directory holds local credentials and must not be committed. Cleanup removes the ServiceAccount, invalidating its tokens; the local file remains and can be deleted or refreshed after setup.

### Break and recover a rollout

```sh
python3 scripts/playground.py rollout-break --context minikube
```

This changes only `rollout-lab` to a deliberately missing image, recording its prior image for recovery. Inspect its new ReplicaSet, failing Pod, and Deployment conditions. The two healthy replicas stay available. The progress deadline is 45 seconds, though the controller may report the failed condition later.

```sh
python3 scripts/playground.py rollout-recover --context minikube
```

Recovery restores the recorded image and waits for readiness. These commands do not alter the frontend. Setup also restores the chart's healthy image.

### Exercise validation and concurrent edits

Open `playground-config` in Plous Editor, change a value, and leave the draft open. In another terminal run:

```sh
python3 scripts/playground.py edit-conflict --context minikube
```

Return to the draft and validate it to inspect the conflict. The command adds a timestamp under `data.concurrent-edit` without replacing other ConfigMap data. Setup restores chart-managed fixture values; reset recreates the objects.

For other editor cases, try malformed YAML, change a ConfigMap data value from a string to an object, or change an immutable Deployment selector. Validate and read the diagnostic, then correct or discard the draft. Review a valid multi-line ConfigMap edit in the Diff Reviewer, switch to Help or chat, and return to confirm your draft and reading position remain intact. These exercises use the existing resources.

## Use existing metrics and ingress controllers

HPA testing requires metrics-server. If needed, enable the addon yourself in the intended Minikube profile:

```sh
minikube -p minikube addons enable metrics-server
python3 scripts/playground.py setup --context minikube --with-hpa
```

The script checks that the metrics API responds before installing. The frontend HPA has a CPU target of 70% and a maximum of three replicas. It may override manual replica changes. Plous can inspect this resource; this does not add live usage charts to Plous.

For an existing ingress controller, supply its IngressClass name:

```sh
python3 scripts/playground.py setup --context minikube --ingress-class traefik --with-cert-manager
```

This creates an Ingress for `plous.local` pointing to the frontend, with the sample TLS Secret when enabled. `--ingress-class` alone does not install a controller, change DNS or hosts files, or start a tunnel. Minikube also documents its [Traefik addon](https://minikube.sigs.k8s.io/docs/handbook/addons/traefik/). Monitoring stacks remain separate installations; the playground script does not install Prometheus or Grafana.

## Status, reset, and cleanup

```sh
python3 scripts/playground.py status --context minikube
python3 scripts/playground.py reset --context minikube --failures all --with-cert-manager
python3 scripts/playground.py cleanup --context minikube
```

Status reports the current resources without changing them. Reset removes the owned playground and installs the requested selection again. Use the extended setup flags with `reset` to recreate the full selection. Cleanup uninstalls releases before deleting their namespaces; it removes sample certificates before their controller. Cleanup refuses to remove playground metrics-server when HPAs exist outside `plous-demo`, or Traefik when outside Ingresses refer to `plous-playground`. Neither command deletes the cluster, changes addons, or removes unrelated namespaces.

PVC deletion follows the StorageClass reclaim policy. A policy of `Retain` can leave PVs and storage behind; the script does not force their removal. If teardown times out, inspect terminating objects and retry cleanup. It does not bypass finalizers. A failed install leaves its resources available for investigation and subsequent cleanup.

## Render and verify locally

The local chart and image tags are in [the fixture directory](../tests/fixtures/playground/). Upstream chart versions are pinned in [versions.json](../tests/fixtures/playground/versions.json): Podinfo `6.11.2`, cert-manager `v1.21.2`, Traefik `41.6.0`, and metrics-server `3.14.0`.

```sh
python3 scripts/playground.py render --failures all --with-cert-manager > /tmp/plous-playground.yaml
make test-playground
```

Render prints only the local fixture chart, including optional custom-resource, rollout, and list examples. It does not render upstream charts, create namespaces, download charts, or contact Kubernetes. The rendered YAML is useful for inspection; setup handles prerequisites and installation order.

Installer tests use fake command-line tools and temporary storage. If Helm is installed, they also lint and render the local chart. These tests do not contact a cluster. Verify deployment and image compatibility by running setup in the intended disposable cluster. Plous's separate [Minikube acceptance tests](../tests/README.md#explicit-minikube-acceptance) exercise the app against a prepared playground.

Upstream references: [Podinfo](https://github.com/stefanprodan/podinfo), [cert-manager Helm installation](https://cert-manager.io/docs/installation/helm/), [self-signed issuers](https://cert-manager.io/docs/configuration/selfsigned/), [Traefik Helm chart](https://github.com/traefik/traefik-helm-chart), [metrics-server Helm chart](https://github.com/kubernetes-sigs/metrics-server/tree/master/charts/metrics-server), and [ServiceAccount tokens](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/).
