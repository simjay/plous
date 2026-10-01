#!/usr/bin/env python3
"""Install disposable Kubernetes examples for exploring Plous."""

import argparse
import base64
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
FIXTURES = ROOT / "tests/fixtures/playground"
CHART = FIXTURES / "chart"
OWNER_KEY = "plous.dev/playground"
OWNER = "v1"
DEMO, FAILURES, CERT = "plous-demo", "plous-failures", "plous-cert-manager"
INGRESS, METRICS = "plous-ingress", "plous-metrics"
INGRESS_CLASS = "plous-playground"
LIST_NAMESPACES = ("plous-list-a", "plous-list-b", "plous-list-c")
RELEASES = {DEMO: ["plous-fixtures", "frontend", "backend"], FAILURES: ["plous-failures"],
            **{name: ["plous-lists"] for name in LIST_NAMESPACES},
            CERT: ["plous-cert-manager"], INGRESS: [INGRESS], METRICS: [METRICS]}
SCENARIOS = ("crash", "image", "readiness", "scheduling", "config", "storage", "init", "job")
CERT_CRDS = (
    "certificates.cert-manager.io", "certificaterequests.cert-manager.io",
    "issuers.cert-manager.io", "clusterissuers.cert-manager.io",
    "orders.acme.cert-manager.io", "challenges.acme.cert-manager.io",
)


class PlaygroundError(Exception):
    pass


def scenarios(value):
    if value == "all":
        return list(SCENARIOS)
    names = list(dict.fromkeys(value.split(","))) if value else []
    unknown = set(names) - set(SCENARIOS)
    if unknown:
        raise argparse.ArgumentTypeError("Unknown failures: " + ", ".join(sorted(unknown)))
    return names


def parser():
    p = argparse.ArgumentParser(description=__doc__, epilog="See docs/playground.md. No command changes the active kubecontext.")
    p.add_argument("action", choices=["plan", "render", "setup", "status", "reset", "cleanup", "credentials", "load", "rollout-break", "rollout-recover", "edit-conflict"])
    p.add_argument("--context", help="Explicit disposable cluster context; required for cluster operations")
    p.add_argument("--kubeconfig", help="Optional kubeconfig file; read without modification")
    p.add_argument("--failures", type=scenarios, default=[], metavar="NAMES", help="Comma-separated: " + ", ".join(SCENARIOS) + ", or all")
    p.add_argument("--with-cert-manager", action="store_true", help="Install the pinned controller, CRDs, and a local certificate")
    p.add_argument("--with-hpa", action="store_true", help="Add a Podinfo HPA; requires a working metrics-server")
    p.add_argument("--with-metrics-server", action="store_true", help="Install playground metrics-server and enable HPA (disposable clusters only)")
    p.add_argument("--with-traefik", action="store_true", help="Install playground Traefik and route plous.local")
    p.add_argument("--with-rollout", action="store_true", help="Add a dedicated rollout exercise Deployment")
    p.add_argument("--list-count", type=int, default=0, help="ConfigMaps per list namespace, 0 to 500; three namespaces")
    p.add_argument("--output", help="Separate kubeconfig destination for the credentials action")
    p.add_argument("--load-seconds", type=int, default=180, help="Duration of the load action, 30 to 600 seconds")
    p.add_argument("--ingress-class", default="", help="Create an Ingress using an existing controller's class")
    p.add_argument("--storage-class", default="", help="Use this StorageClass instead of the cluster default")
    p.add_argument("--timeout-seconds", type=int, default=300)
    return p


class Playground:
    def __init__(self, args, directory):
        self.args, self.directory = args, Path(directory)
        self.versions = json.loads((FIXTURES / "versions.json").read_text())
        self.env = dict(os.environ)
        for name in ("CACHE", "CONFIG", "DATA"):
            self.env[f"HELM_{name}_HOME"] = str(self.directory / name.lower())

    def run(self, program, *args, data=None, capture=True, cluster=True):
        command = [program]
        if cluster:
            command += ["--context" if program == "kubectl" else "--kube-context", self.args.context]
            if self.args.kubeconfig:
                command += ["--kubeconfig", str(Path(self.args.kubeconfig).expanduser())]
            if program == "kubectl":
                command += ["--request-timeout=30s"]
        result = subprocess.run(command + list(args), input=data, text=True, capture_output=capture, env=self.env)
        if result.returncode:
            detail = (result.stderr or result.stdout or "See command output.").strip()
            raise PlaygroundError(f"{program} {' '.join(args[:3])} failed: {detail}")
        return result.stdout or ""

    def kube(self, *args, data=None):
        return self.run("kubectl", *args, data=data)

    def get(self, kind, name=None, namespace=None):
        args = ["get", kind]
        if name:
            args += [name, "--ignore-not-found"]
        if namespace:
            args += ["--namespace", namespace]
        text = self.kube(*args, "-o", "json")
        return json.loads(text) if text.strip() else None

    def namespace(self, name):
        obj = self.get("namespace", name)
        if obj and obj["metadata"].get("labels", {}).get(OWNER_KEY) != OWNER:
            raise PlaygroundError(f"Namespace {name} is not owned by this playground. Nothing will be adopted.")
        if obj and obj["metadata"].get("deletionTimestamp"):
            raise PlaygroundError(f"Namespace {name} is still terminating. Wait before retrying.")
        return obj

    def releases(self, namespace):
        names = [r["name"] for r in json.loads(self.run("helm", "list", "--namespace", namespace, "--all", "-o", "json"))]
        extra = set(names) - set(RELEASES[namespace])
        if extra:
            raise PlaygroundError(f"Unexpected Helm releases in {namespace}: {', '.join(sorted(extra))}. Refusing to remove or reuse it.")
        return names

    def check_cert_ownership(self, removing=False):
        for name in CERT_CRDS:
            crd = self.get("crd", name)
            if not crd:
                continue
            annotation = crd["metadata"].get("annotations", {})
            if annotation.get("meta.helm.sh/release-name") != CERT or annotation.get("meta.helm.sh/release-namespace") != CERT:
                raise PlaygroundError(f"{name} belongs to another installation. Use the playground without --with-cert-manager.")
            if removing:
                items = json.loads(self.kube("get", name, "--all-namespaces", "-o", "json"))["items"]
                if any(o["metadata"].get("namespace") != DEMO for o in items):
                    raise PlaygroundError(f"{name} has resources outside {DEMO}. Keep cert-manager until those resources are handled.")

    def preflight(self, removing=False):
        for name in RELEASES:
            if self.namespace(name):
                self.releases(name)
        if self.args.with_cert_manager or removing and self.get("namespace", CERT):
            self.check_cert_ownership(removing)
        if self.args.with_metrics_server or removing and self.get("namespace", METRICS):
            for name in ("v1.metrics.k8s.io", "v1beta1.metrics.k8s.io"):
                self.check_release_owner(self.get("apiservice", name), METRICS)
            if removing and any(obj["metadata"].get("namespace") != DEMO for obj in json.loads(self.kube("get", "hpa", "--all-namespaces", "-o", "json"))["items"]):
                raise PlaygroundError("HPAs exist outside plous-demo. Keep metrics-server until those consumers are handled.")
        if self.args.with_traefik or removing and self.get("namespace", INGRESS):
            self.check_release_owner(self.get("ingressclass", INGRESS_CLASS), INGRESS)
            if removing and any(obj["metadata"].get("namespace") != DEMO and obj.get("spec", {}).get("ingressClassName") == INGRESS_CLASS for obj in json.loads(self.kube("get", "ingress", "--all-namespaces", "-o", "json"))["items"]):
                raise PlaygroundError("Ingresses outside plous-demo use the playground controller. Keep it until those routes are handled.")
        if removing:
            return
        classes = self.get("storageclass")["items"]
        if self.args.storage_class:
            available = any(c["metadata"]["name"] == self.args.storage_class for c in classes)
        else:
            available = any(c["metadata"].get("annotations", {}).get("storageclass.kubernetes.io/is-default-class") == "true" for c in classes)
        if not available:
            raise PlaygroundError("No matching StorageClass. Enable Minikube storage provisioning or pass --storage-class NAME.")
        if self.args.with_hpa and not self.args.with_metrics_server:
            self.kube("get", "--raw", "/apis/metrics.k8s.io/v1beta1/nodes")
        if self.args.ingress_class and not self.args.with_traefik and not self.get("ingressclass", self.args.ingress_class):
            raise PlaygroundError(f"IngressClass {self.args.ingress_class} does not exist. Install its controller first.")

    def check_release_owner(self, obj, release):
        if not obj:
            return
        annotations = obj["metadata"].get("annotations", {})
        if annotations.get("meta.helm.sh/release-name") != release or annotations.get("meta.helm.sh/release-namespace") != release:
            raise PlaygroundError(f"{obj['metadata']['name']} belongs to another installation. Nothing will be adopted.")

    def ensure_namespace(self, name):
        if not self.namespace(name):
            obj = {"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": name, "labels": {OWNER_KEY: OWNER, "app.kubernetes.io/part-of": "plous-playground"}}}
            self.kube("create", "-f", "-", data=json.dumps(obj))

    def values(self, release, value):
        path = self.directory / (release + ".json")
        path.write_text(json.dumps(value))
        return str(path)

    def install(self, release, namespace, chart, values, version=None, wait=True, repo=None, skip_crds=False):
        print(f"Installing {release} in {namespace}…", flush=True)
        args = ["upgrade", "--install", release, chart, "--namespace", namespace, "--reset-values", "--values", self.values(release, values), "--timeout", f"{self.args.timeout_seconds}s"]
        if version:
            args += ["--version", version]
        if repo:
            args += ["--repo", repo]
        if skip_crds:
            args += ["--skip-crds"]
        if wait:
            args += ["--wait"]
        self.run("helm", *args, capture=False)

    def fixture_values(self):
        return {"storageClass": self.args.storage_class, "certificates": self.args.with_cert_manager,
                "ingressClass": self.args.ingress_class, "rollout": self.args.with_rollout}

    def install_controllers(self):
        resources = {"requests": {"cpu": "50m", "memory": "64Mi"}, "limits": {"cpu": "300m", "memory": "256Mi"}}
        if self.args.with_metrics_server:
            self.ensure_namespace(METRICS)
            chart = self.versions["metrics-server"]
            self.install(METRICS, METRICS, chart["chart"], {
                "fullnameOverride": METRICS, "args": ["--kubelet-insecure-tls"], "resources": resources,
            }, chart["version"], repo=chart["repo"])
            self.kube("wait", "apiservice/v1beta1.metrics.k8s.io", "--for=condition=Available", f"--timeout={self.args.timeout_seconds}s")
            self.kube("get", "--raw", "/apis/metrics.k8s.io/v1beta1/nodes")
        if self.args.with_traefik:
            self.ensure_namespace(INGRESS)
            chart = self.versions["traefik"]
            self.install(INGRESS, INGRESS, chart["chart"], {
                "fullnameOverride": INGRESS, "resources": resources,
                "service": {"spec": {"type": "ClusterIP"}},
                "ingressClass": {"name": INGRESS_CLASS, "isDefaultClass": False},
                "providers": {"kubernetesCRD": {"enabled": False}, "kubernetesGateway": {"enabled": False},
                              "kubernetesIngress": {"enabled": True, "namespaces": [DEMO], "ingressClass": INGRESS_CLASS}},
                "gateway": {"enabled": False}, "ingressRoute": {"dashboard": {"enabled": False}},
            }, chart["version"], repo=chart["repo"], skip_crds=True)

    def setup(self):
        self.preflight()
        self.ensure_namespace(DEMO)
        self.install_controllers()
        if self.args.with_cert_manager:
            self.ensure_namespace(CERT)
            chart = self.versions["cert-manager"]
            self.install(CERT, CERT, chart["chart"], {"crds": {"enabled": True, "keep": False}, "global": {"leaderElection": {"namespace": CERT}}, "prometheus": {"enabled": False}}, chart["version"])
            self.kube("wait", "--for=condition=Established", *["crd/" + n for n in CERT_CRDS], f"--timeout={self.args.timeout_seconds}s")
        chart = self.versions["podinfo"]
        common = {"resources": {"requests": {"cpu": "50m", "memory": "32Mi"}, "limits": {"cpu": "500m", "memory": "128Mi"}}}
        self.install("backend", DEMO, chart["chart"], dict(common, replicaCount=1), chart["version"])
        front = dict(common, replicaCount=2, backend="http://backend-podinfo:9898/echo", podDisruptionBudget={"maxUnavailable": 1})
        if self.args.with_hpa:
            front["hpa"] = {"enabled": True, "maxReplicas": 3, "cpu": 70}
        self.install("frontend", DEMO, chart["chart"], front, chart["version"])
        self.install("plous-fixtures", DEMO, str(CHART), self.fixture_values())
        if self.args.with_cert_manager:
            self.kube("wait", "certificate/playground-tls", "--namespace", DEMO, "--for=condition=Ready", f"--timeout={self.args.timeout_seconds}s")
        if self.args.failures:
            self.ensure_namespace(FAILURES)
            self.install("plous-failures", FAILURES, str(CHART), {"profile": "failures", "failures": self.args.failures}, wait=False)
        elif self.namespace(FAILURES) and "plous-failures" in self.releases(FAILURES):
            self.uninstall("plous-failures", FAILURES)
        for name in LIST_NAMESPACES:
            if self.args.list_count:
                self.ensure_namespace(name)
                self.install("plous-lists", name, str(CHART), {"profile": "lists", "listCount": self.args.list_count})
            elif self.namespace(name) and "plous-lists" in self.releases(name):
                self.uninstall("plous-lists", name)
        print("Playground installed. Explore plous-demo" + (" and plous-failures." if self.args.failures else "."))
        if self.args.failures:
            print("Expected failure cases: " + ", ".join(self.args.failures) + ". These are not readiness failures in the installer.")

    def uninstall(self, release, namespace):
        print(f"Removing {release} from {namespace}…", flush=True)
        self.run("helm", "uninstall", release, "--namespace", namespace, "--wait", "--timeout", f"{self.args.timeout_seconds}s", capture=False)

    def cleanup(self):
        self.preflight(removing=True)
        # Remove certificates while their controller still handles finalizers.
        for namespace in RELEASES:
            if not self.namespace(namespace):
                continue
            installed = self.releases(namespace)
            for release in RELEASES[namespace]:
                if release in installed:
                    self.uninstall(release, namespace)
            self.kube("delete", "namespace", namespace, "--wait=true", f"--timeout={self.args.timeout_seconds}s")
        print("Playground removed, including its namespaced data. PV disposal follows the StorageClass reclaim policy.")

    def status(self):
        for name in RELEASES:
            if self.namespace(name):
                print(f"\n{name}", flush=True)
                kinds = "configmaps" if name in LIST_NAMESPACES else "pods,deployments,statefulsets,daemonsets,jobs,cronjobs,services,pvc,ingresses,hpa"
                self.run("kubectl", "get", kinds, "--namespace", name, capture=False)

    def render(self):
        for release, namespace, values in [("plous-fixtures", DEMO, self.fixture_values()), ("plous-failures", FAILURES, {"profile": "failures", "failures": self.args.failures})]:
            if namespace == FAILURES and not self.args.failures:
                continue
            self.run("helm", "template", release, str(CHART), "--namespace", namespace, "--values", self.values(release, values), cluster=False, capture=False)
        if self.args.list_count:
            for namespace in LIST_NAMESPACES:
                self.run("helm", "template", "plous-lists", str(CHART), "--namespace", namespace,
                         "--values", self.values("plous-lists", {"profile": "lists", "listCount": self.args.list_count}), cluster=False, capture=False)

    def fixture(self, kind, name):
        if not self.namespace(DEMO):
            raise PlaygroundError("Install the playground first.")
        self.releases(DEMO)
        obj = self.get(kind, name, DEMO)
        labels = obj["metadata"].get("labels", {}) if obj else {}
        if labels.get("app.kubernetes.io/instance") != "plous-fixtures" or labels.get("app.kubernetes.io/part-of") != "plous-playground":
            raise PlaygroundError(f"{kind}/{name} is not an installed playground fixture.")
        return obj

    def credentials(self):
        self.fixture("serviceaccount", "playground-observer")
        path = Path(self.args.output).expanduser().absolute()
        if path.is_symlink() or path.resolve() == Path("~/.kube/config").expanduser().resolve() or (self.args.kubeconfig and path.resolve() == Path(self.args.kubeconfig).expanduser().resolve()):
            raise PlaygroundError("Choose a separate output file, not your input kubeconfig or a symlink.")
        marker = [{"name": "plous-playground", "extension": {"context": self.args.context}}]
        if path.exists() and json.loads(path.read_text()).get("extensions") != marker:
            raise PlaygroundError("Refusing to replace a kubeconfig not created for this playground context.")
        # Select only cluster transport data; never export the owner's user credentials.
        cluster = json.loads(self.kube("config", "view", "--minify", "--flatten", "--raw", "-o", "jsonpath={.clusters[0].cluster}"))
        token = self.kube("create", "token", "playground-observer", "--namespace", DEMO, "--duration=24h").strip()
        payload = token.split(".")[1]
        expiry = datetime.fromtimestamp(json.loads(base64.urlsafe_b64decode(payload + "=" * (-len(payload) % 4)))["exp"], timezone.utc)
        name = self.args.context + "-plous-observer"
        config = {"apiVersion": "v1", "kind": "Config", "extensions": marker,
                  "clusters": [{"name": name, "cluster": cluster}],
                  "users": [{"name": name, "user": {"token": token}}],
                  "contexts": [{"name": name, "context": {"cluster": name, "user": name, "namespace": DEMO}}], "current-context": name}
        path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        with tempfile.NamedTemporaryFile(mode="w", dir=path.parent, delete=False) as output:
            temporary = Path(output.name)
            try:
                os.chmod(temporary, 0o600)
                json.dump(config, output, indent=2)
                output.flush()
                os.replace(temporary, path)
            finally:
                temporary.unlink(missing_ok=True)
        print(f"Restricted kubeconfig: {path}\nContext: {name}\nToken expires: {expiry.isoformat()}. Rerun credentials to renew.")

    def load(self):
        self.fixture("configmap", "playground-config")
        if not self.get("hpa", "frontend-podinfo", DEMO):
            raise PlaygroundError("Enable --with-hpa before starting load.")
        jobs = json.loads(self.kube("get", "jobs", "--namespace", DEMO, "--selector=plous.dev/scenario=load", "-o", "json"))["items"]
        if any(not any(c["type"] in ("Complete", "Failed") and c["status"] == "True" for c in job.get("status", {}).get("conditions", [])) for job in jobs):
            raise PlaygroundError("A load Job is already pending or running. Wait for it to finish.")
        manifest = self.run("helm", "template", "plous-load", str(CHART), "--namespace", DEMO,
                            "--values", self.values("plous-load", {"profile": "load", "loadSeconds": self.args.load_seconds}), cluster=False)
        print(self.kube("create", "-f", "-", data=manifest).strip())
        print(f"Load runs for {self.args.load_seconds}s with a 500m CPU limit. Watch frontend-podinfo's HPA; scale-down may take several minutes.")

    def patch_fixture(self, kind, obj, patch):
        patch.setdefault("metadata", {})["resourceVersion"] = obj["metadata"]["resourceVersion"]
        return self.kube("patch", kind, obj["metadata"]["name"], "--namespace", DEMO,
                         "--type=strategic", "--patch", json.dumps(patch))

    def rollout_break(self):
        obj = self.fixture("deployment", "rollout-lab")
        key = "plous.dev/healthy-image"
        if obj["metadata"].get("annotations", {}).get(key):
            raise PlaygroundError("Rollout exercise is already broken. Use rollout-recover first.")
        original = obj["spec"]["template"]["spec"]["containers"][0]["image"]
        self.patch_fixture("deployment", obj, {"metadata": {"annotations": {key: original}},
            "spec": {"template": {"spec": {"containers": [{"name": "web", "image": "docker.io/library/busybox:plous-intentionally-missing"}]}}}})
        print("rollout-lab now requests a missing image. Inspect its new ReplicaSet and rollout conditions. Run rollout-recover to restore it.")

    def rollout_recover(self):
        obj = self.fixture("deployment", "rollout-lab")
        key = "plous.dev/healthy-image"
        original = obj["metadata"].get("annotations", {}).get(key)
        if not original:
            raise PlaygroundError("No broken rollout recorded. Use rollout-break first.")
        self.patch_fixture("deployment", obj, {"metadata": {"annotations": {key: None}},
            "spec": {"template": {"spec": {"containers": [{"name": "web", "image": original}]}}}})
        print(self.kube("rollout", "status", "deployment/rollout-lab", "--namespace", DEMO, f"--timeout={self.args.timeout_seconds}s").strip())

    def edit_conflict(self):
        obj = self.fixture("configmap", "playground-config")
        self.patch_fixture("configmap", obj, {"data": {"concurrent-edit": datetime.now(timezone.utc).isoformat()}})
        print("Updated playground-config. Validate an already-open Plous draft to exercise concurrent-edit handling.")

    def plan(self):
        print("Context: " + (self.args.context or "supply --context for setup"))
        print("plous-demo: frontend and backend (Podinfo " + self.versions["podinfo"]["version"] + "), plous-fixtures")
        print("plous-failures: " + (", ".join(self.args.failures) or "disabled; setup removes previously enabled failure fixtures"))
        print("cert-manager: " + (self.versions["cert-manager"]["version"] if self.args.with_cert_manager else "not requested"))
        print("HPA: " + ("enabled; requires metrics-server" if self.args.with_hpa else "disabled"))
        print("Ingress: " + (self.args.ingress_class or "disabled"))
        print("Install controllers: " + ", ".join(name for name, enabled in [("Traefik", self.args.with_traefik), ("metrics-server", self.args.with_metrics_server)] if enabled))
        print(f"List examples: {self.args.list_count * 3} ConfigMaps across three namespaces; rollout exercise: {self.args.with_rollout}")
        print("StorageClass: " + (self.args.storage_class or "cluster default"))
        print("No cluster requests were made. setup changes only the listed playground resources.")


def main(argv=None):
    p = parser()
    args = p.parse_args(argv)
    if args.timeout_seconds < 1:
        p.error("--timeout-seconds must be positive")
    if not 0 <= args.list_count <= 500 or not 30 <= args.load_seconds <= 600:
        p.error("--list-count must be 0 to 500 and --load-seconds must be 30 to 600")
    if args.load_seconds != 180 and args.action != "load":
        p.error("--load-seconds is only used by load")
    if args.with_traefik:
        if args.ingress_class and args.ingress_class != INGRESS_CLASS:
            p.error("--with-traefik uses the plous-playground IngressClass")
        args.ingress_class = INGRESS_CLASS
    args.with_hpa = args.with_hpa or args.with_metrics_server
    if (args.action == "credentials") != bool(args.output):
        p.error("credentials requires --output; --output is only used by credentials")
    if args.action not in ("plan", "render") and not args.context:
        p.error("--context is required; the current context is never selected implicitly")
    if args.action not in ("plan", "render", "setup", "reset") and (args.failures or args.with_cert_manager or args.with_hpa or args.ingress_class or args.storage_class or args.with_rollout or args.list_count):
        p.error("Feature flags apply to plan, render, setup, and reset; cleanup/status inspect the installed playground")
    required = [] if args.action == "plan" else ["helm"] if args.action == "render" else ["helm", "kubectl"]
    for binary in required:
        if not shutil.which(binary):
            p.error(f"{binary} is required")
    try:
        with tempfile.TemporaryDirectory(prefix="plous-playground-") as directory:
            playground = Playground(args, directory)
            if args.action == "reset":
                playground.preflight()
                playground.cleanup()
                playground.setup()
            else:
                getattr(playground, args.action.replace("-", "_"))()
    except (PlaygroundError, OSError, ValueError) as error:
        print(f"Playground: {error}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("Interrupted. Installed resources remain; use status or cleanup with the same context.", file=sys.stderr)
        return 130
    return 0


if __name__ == "__main__":
    sys.exit(main())
