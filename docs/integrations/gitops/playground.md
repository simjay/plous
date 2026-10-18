# Argo and Flux playground

[Argo and Flux](README.md) · [Kubernetes playground](../kubernetes/playground.md)

These examples install real controllers and disposable workloads on Minikube for testing Plous Monitor. They are separate from `scripts/playground.py` and its existing workloads. Run the commands from the repository root with Helm and kubectl installed.

## Install

Use a running disposable cluster with internet access. Reserve `plous-argo`, `plous-argo-demo`, `plous-flux`, and `plous-flux-demo` for these examples. Check that no other Argo or Flux installation owns their APIs before installing. Repeating the commands updates the same examples. Every cluster command below explicitly selects `minikube`; none changes the active context.

```sh
kubectl --context minikube get namespaces
kubectl --context minikube get crd

kubectl --context minikube apply -f tests/fixtures/playground/gitops/namespaces.yaml

helm --kube-context minikube upgrade --install plous-argo argo-cd \
  --repo https://argoproj.github.io/argo-helm --version 10.9.6 \
  --namespace plous-argo --values tests/fixtures/playground/gitops/argo-values.yaml \
  --wait --timeout 5m

helm --kube-context minikube upgrade --install plous-flux flux2 \
  --repo https://fluxcd-community.github.io/helm-charts --version 2.19.1 \
  --namespace plous-flux --values tests/fixtures/playground/gitops/flux-values.yaml \
  --wait --timeout 5m

kubectl --context minikube apply -k tests/fixtures/playground/gitops
```

The Argo chart runs its application controller, repository server, and Redis. Its API server and ApplicationSet controller have zero replicas; Dex and notifications are disabled. Flux uses its source, Kustomize, and Helm controllers through the community Helm chart. The fixtures pin controller charts, Git revisions, and Podinfo 6.11.2. No GitHub token, fork, Git write, or controller login is needed.

Flux reads the staging app directory from [flux2-kustomize-helm-example](https://github.com/fluxcd/flux2-kustomize-helm-example/tree/4f1a5b1051b469108d1bdf0ebec6c814427e3834). Local patches place it in `plous-flux-demo`, pin the chart, and disable Redis and HTTPRoute. The upstream infrastructure and bootstrap configurations are not applied. Argo uses Podinfo's public Helm chart and a pinned [Kustomize example](https://github.com/stefanprodan/podinfo/tree/d77ade33c0ac45b83a674fab93ee304652dfc53e/kustomize).

## Explore in Plous

Select the Minikube connection under **Monitor** and press **g** to discover the new APIs. Expand **Argo** or **Flux**, select its category, and keep **All namespaces** selected. Choose a row and press **Enter** for details; managed resources open in the Kubernetes view, and **Esc** returns.

| View | Example | Expected observation |
| --- | --- | --- |
| Argo / Applications | `podinfo-healthy` | Synced and Healthy; open its Deployment or Service. |
| Argo / Applications | `podinfo-degraded` | Synced and Degraded after the rollout deadline; its Deployment requests a deliberately nonexistent image. |
| Argo / Applications | `podinfo-outofsync` | OutOfSync and Missing; automatic deployment is disabled and no workloads are created. |
| Flux / Kustomizations | `podinfo` Kustomization | Ready; open its source object, HelmRelease, and resource inventory. |
| Flux / HelmReleases | `podinfo` HelmRelease in `plous-flux-demo` | Ready with the installed chart version and release result. |
| Flux / Kustomizations | `broken-path` | Not ready because its source path does not exist. |
| Flux / Kustomizations | `waiting-on-dependency` | Not ready; follow its dependency link to `broken-path`. |
| Flux / Kustomizations | `paused-example` | Suspended; it has never applied resources. |

**Argo / Projects** includes `plous-playground` with its namespace-scoped Application links. **Argo / ApplicationSets** is empty in this fixture. **Flux / Sources** includes `flux-example` and `podinfo`, plus HelmChart objects created by the controllers.

Controller startup, repository downloads, and image pulls can take several minutes. The degraded Argo workload has a 60-second rollout deadline and may show Progressing first. A Synced Application can be Degraded because sync and workload health measure different things. The broken and suspended cases are intentional; do not wait for every object to become Ready.

Check the live controller reports directly:

```sh
kubectl --context minikube get applications.argoproj.io -n plous-argo
kubectl --context minikube get kustomizations.kustomize.toolkit.fluxcd.io -n plous-flux
kubectl --context minikube get helmreleases.helm.toolkit.fluxcd.io -n plous-flux-demo
kubectl --context minikube get gitrepositories.source.toolkit.fluxcd.io -n plous-flux
```

To validate local changes without saving them, use `kubectl --context minikube apply --dry-run=server -k tests/fixtures/playground/gitops`. This requires the controllers' APIs to be installed. These manifests contain desired configuration only; controller status is never fabricated.

## Remove these examples

Remove the applications while their controllers can process finalizers, then remove the controllers. Stop if application deletion fails; inspect the error before continuing. These commands remove the four reserved namespaces and their contents. They do not remove the separate Kubernetes playground.

```sh
kubectl --context minikube delete applications.argoproj.io -n plous-argo \
  -l plous.dev/gitops-playground=v1 --timeout=2m
kubectl --context minikube delete kustomizations.kustomize.toolkit.fluxcd.io -n plous-flux \
  -l plous.dev/gitops-playground=v1 --timeout=2m
kubectl --context minikube delete -k tests/fixtures/playground/gitops --ignore-not-found
helm --kube-context minikube uninstall plous-argo -n plous-argo --wait
helm --kube-context minikube uninstall plous-flux -n plous-flux --wait
kubectl --context minikube delete -f tests/fixtures/playground/gitops/namespaces.yaml --ignore-not-found
```

The charts retain their CRDs on uninstall. Plous can therefore still discover empty Argo and Flux APIs after cleanup. Remove those definitions only when no other installation or custom resources need them. Setup and removal are manual, explicit cluster operations; the default test suite does not run them.
