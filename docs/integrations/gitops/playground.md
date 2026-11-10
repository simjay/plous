# Argo and Flux playground

[Argo CD](../argo/README.md) · [Flux](../flux/README.md) · [Kubernetes playground](../kubernetes/playground.md)

Install real controllers and sample workloads in a disposable Minikube cluster. These examples are separate from `scripts/playground.py`.

Run from the Plous checkout with Helm, kubectl, and network access.

## Install

Reserve `plous-argo`, `plous-argo-demo`, `plous-flux`, and `plous-flux-demo`. Check that another installation does not own the controller APIs.

Commands explicitly select `minikube` and do not change the active context. Repeating them updates the same examples.

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

The fixtures pin charts, source revisions, and Podinfo 6.11.2. They need no GitHub token, fork, controller login, or repository write.

Argo runs its application controller, repository server, and Redis. Its API server and ApplicationSet controller have zero replicas. Flux runs source, Kustomize, and Helm controllers.

Sources use pinned [Flux examples](https://github.com/fluxcd/flux2-kustomize-helm-example/tree/4f1a5b1051b469108d1bdf0ebec6c814427e3834) and [Podinfo examples](https://github.com/stefanprodan/podinfo/tree/d77ade33c0ac45b83a674fab93ee304652dfc53e/kustomize). The upstream bootstrap and infrastructure are not applied.

## Explore in Plous

Select the cluster and press `g` to discover its APIs. Open Argo or Flux, choose a category, and keep All namespaces selected. Enter opens details and resource links. Esc returns.

| View | Example | Expected observation |
| --- | --- | --- |
| Argo / Applications | `podinfo-healthy` | Synced and Healthy. Open its Deployment or Service. |
| Argo / Applications | `podinfo-degraded` | Synced and Degraded after the rollout deadline. Its Deployment requests a deliberately nonexistent image. |
| Argo / Applications | `podinfo-outofsync` | OutOfSync and Missing. Automatic deployment is disabled and no workloads are created. |
| Flux / Kustomizations | `podinfo` Kustomization | Ready. Open its source object, HelmRelease, and resource inventory. |
| Flux / HelmReleases | `podinfo` HelmRelease in `plous-flux-demo` | Ready with the installed chart version and release result. |
| Flux / Kustomizations | `broken-path` | Not ready because its source path does not exist. |
| Flux / Kustomizations | `waiting-on-dependency` | Not ready. Follow its dependency link to `broken-path`. |
| Flux / Kustomizations | `paused-example` | Suspended. It has never applied resources. |

```sh
kubectl --context minikube get applications.argoproj.io -n plous-argo
kubectl --context minikube get kustomizations.kustomize.toolkit.fluxcd.io -n plous-flux
kubectl --context minikube get helmreleases.helm.toolkit.fluxcd.io -n plous-flux-demo
kubectl --context minikube get gitrepositories.source.toolkit.fluxcd.io -n plous-flux
```

Argo Projects contains `plous-playground`. ApplicationSets is empty. Flux Sources includes `flux-example`, `podinfo`, and controller-created HelmCharts.

Downloads can take several minutes. The degraded Argo workload has a 60-second rollout deadline and can show Progressing first. Broken and suspended cases are intentional. Do not wait for every object to become Ready.

The kubectl commands above show live controller status. Use `kubectl --context minikube apply --dry-run=server -k tests/fixtures/playground/gitops` to validate without saving. This requires installed controller APIs. Fixture files do not fabricate status.

## Remove these examples

Remove applications while their controllers can process finalizers, then remove the controllers. Stop and inspect any deletion failure before continuing.

These commands delete the four reserved namespaces and their contents. They leave the separate Kubernetes playground intact.

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

Charts retain their CRDs on uninstall, so Plous can still discover empty APIs. Remove definitions only when no other installation or objects need them.

The default tests do not perform this installation or cleanup.
