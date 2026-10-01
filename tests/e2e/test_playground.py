"""Exercise the playground installer with isolated kubectl and Helm doubles."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

from support import ROOT

SCRIPT = ROOT / "scripts/playground.py"
CHART = ROOT / "tests/fixtures/playground/chart"


def owned_namespace(name):
    return {"metadata": {"name": name, "labels": {"plous.dev/playground": "v1"}}}


def fixture(name):
    return {"metadata": {"name": name, "resourceVersion": "12", "labels": {
        "app.kubernetes.io/instance": "plous-fixtures", "app.kubernetes.io/part-of": "plous-playground"}}}


class PlaygroundTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory(prefix="plous-playground-test-")
        self.addCleanup(directory.cleanup)
        self.directory = Path(directory.name)
        self.state = self.directory / "state.json"
        self.state.write_text("{}")
        self.log = self.directory / "commands.jsonl"
        fake = (ROOT / "tests/fixtures/playground/fake_cli.py").read_text()
        for program in ("helm", "kubectl"):
            path = self.directory / program
            path.write_text("#!" + sys.executable + "\n" + fake.split("\n", 1)[1])
            path.chmod(0o700)
        self.config = self.directory / "kubeconfig"
        self.config.write_text("synthetic configuration, never read by real kubectl\n")
        self.env = dict(os.environ, PATH=str(self.directory) + os.pathsep + os.environ["PATH"], PLAYGROUND_FAKE_STATE=str(self.state), PLAYGROUND_FAKE_LOG=str(self.log))

    def run_script(self, action, *flags, context=True):
        args = [sys.executable, str(SCRIPT), action, *flags]
        if context:
            args += ["--context", "disposable-test", "--kubeconfig", str(self.config)]
        result = subprocess.run(args, text=True, capture_output=True, env=self.env, timeout=20)
        self.assertEqual(self.config.read_text(), "synthetic configuration, never read by real kubectl\n")
        return result

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def writes(self):
        return [r for r in self.calls() if any(a in ("create", "delete", "upgrade", "uninstall") for a in r["args"])]

    def test_plan_and_invalid_input_never_access_cluster(self):
        self.assertEqual(self.run_script("plan", "--failures", "all", context=False).returncode, 0)
        self.assertEqual(self.calls(), [])
        for action, flags in [("setup", []), ("cleanup", []), ("reset", []), ("setup", ["--failures", "typo"])]:
            self.assertNotEqual(self.run_script(action, *flags, context=False).returncode, 0)
        self.assertEqual(self.calls(), [])

    def test_setup_pins_charts_scopes_every_request_and_does_not_wait_for_failures(self):
        result = self.run_script("setup", "--failures", "crash,storage,job", "--with-cert-manager")
        self.assertEqual(result.returncode, 0, result.stderr)
        for call in self.calls():
            flag = "--context" if call["program"] == "kubectl" else "--kube-context"
            self.assertEqual(call["args"][call["args"].index(flag) + 1], "disposable-test")
            self.assertIn(str(self.config), call["args"])
            self.assertNotIn("use-context", call["args"])
        installs = [c for c in self.calls() if "upgrade" in c["args"]]
        self.assertEqual(len(installs), 5)
        for call in installs:
            if "oci://" in " ".join(call["args"]):
                self.assertIn("--version", call["args"])
            failing = call["values"].get("profile") == "failures"
            self.assertEqual("--wait" in call["args"], not failing)
        cert = next(c for c in installs if "crds" in c["values"])
        self.assertEqual(cert["values"]["crds"], {"enabled": True, "keep": False})
        self.assertIn("Expected failure cases", result.stdout)
        self.assertEqual(self.run_script("setup", "--failures", "readiness").returncode, 0)
        last = [c for c in self.calls() if "upgrade" in c["args"]][-1]
        self.assertEqual(last["values"]["failures"], ["readiness"])

    def test_preflight_blocks_foreign_namespaces_crds_and_missing_dependencies(self):
        cases = [
            ({"namespaces": {"plous-demo": {"metadata": {"name": "plous-demo"}}}}, []),
            ({"namespaces": {"plous-demo": owned_namespace("plous-demo")}, "releases": {"plous-demo": ["unrelated"]}}, []),
            ({"crds": {"certificates.cert-manager.io": {"metadata": {"name": "certificates.cert-manager.io"}}}}, ["--with-cert-manager"]),
            ({"storageclasses": []}, []),
            ({"metrics_error": True}, ["--with-hpa"]),
            ({}, ["--ingress-class", "missing"]),
        ]
        for state, flags in cases:
            with self.subTest(state=state):
                self.state.write_text(json.dumps(state))
                self.log.unlink(missing_ok=True)
                result = self.run_script("setup", *flags)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(self.writes(), [])

    def test_cleanup_refuses_foreign_resources_before_any_deletion(self):
        self.state.write_text(json.dumps({"namespaces": {"plous-demo": owned_namespace("plous-demo"), "plous-cert-manager": owned_namespace("plous-cert-manager")}, "crds": {"certificates.cert-manager.io": {"metadata": {"annotations": {"meta.helm.sh/release-name": "plous-cert-manager", "meta.helm.sh/release-namespace": "plous-cert-manager"}}}}, "custom": {"certificates.cert-manager.io": [{"metadata": {"namespace": "production"}}]}}))
        result = self.run_script("cleanup")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("outside plous-demo", result.stderr)
        self.assertEqual(self.writes(), [])

    def test_cleanup_and_reset_remove_only_owned_releases_and_namespaces(self):
        self.assertEqual(self.run_script("setup", "--with-cert-manager", "--failures", "all").returncode, 0)
        result = self.run_script("cleanup")
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.calls()
        removed = [c["args"][c["args"].index("uninstall") + 1] for c in calls if "uninstall" in c["args"]]
        self.assertEqual(removed, ["plous-fixtures", "frontend", "backend", "plous-failures", "plous-cert-manager"])
        self.assertEqual(json.loads(self.state.read_text())["namespaces"], {})
        self.assertEqual(self.run_script("cleanup").returncode, 0)
        self.assertEqual(self.run_script("reset", "--failures", "image").returncode, 0)
        self.assertEqual(set(json.loads(self.state.read_text())["namespaces"]), {"plous-demo", "plous-failures"})

    def test_setup_without_failures_removes_previous_failure_release(self):
        self.assertEqual(self.run_script("setup", "--failures", "crash").returncode, 0)
        self.assertEqual(self.run_script("setup").returncode, 0)
        self.assertEqual(json.loads(self.state.read_text())["releases"]["plous-failures"], [])

    def test_extensions_install_pinned_controllers_and_replace_list_selection(self):
        result = self.run_script("setup", "--with-traefik", "--with-metrics-server", "--with-rollout", "--list-count", "100")
        self.assertEqual(result.returncode, 0, result.stderr)
        installs = [c for c in self.calls() if "upgrade" in c["args"]]
        self.assertEqual(len(installs), 8)
        controllers = [c for c in installs if "--repo" in c["args"]]
        self.assertEqual(len(controllers), 2)
        for call in controllers:
            self.assertIn("--version", call["args"])
        ingress = next(c for c in controllers if "plous-ingress" in c["args"])
        self.assertIn("--skip-crds", ingress["args"])
        self.assertFalse(ingress["values"]["ingressClass"]["isDefaultClass"])
        self.assertEqual(ingress["values"]["providers"]["kubernetesIngress"]["namespaces"], ["plous-demo"])
        self.assertEqual(len([c for c in installs if c["values"].get("listCount") == 100]), 3)
        self.assertEqual(self.run_script("setup").returncode, 0)
        state = json.loads(self.state.read_text())
        self.assertEqual(state["releases"]["plous-list-a"], [])
        self.assertEqual(state["releases"]["plous-ingress"], ["plous-ingress"])
        self.assertEqual(self.run_script("cleanup").returncode, 0)
        self.assertEqual(json.loads(self.state.read_text())["namespaces"], {})

    def test_extensions_refuse_foreign_controllers_before_mutating(self):
        for state, flag in [
            ({"apiservices": {"v1beta1.metrics.k8s.io": {"metadata": {"name": "v1beta1.metrics.k8s.io"}}}}, "--with-metrics-server"),
            ({"ingressclasses": {"plous-playground": {"metadata": {"name": "plous-playground"}}}}, "--with-traefik"),
        ]:
            self.state.write_text(json.dumps(state))
            self.log.unlink(missing_ok=True)
            self.assertNotEqual(self.run_script("setup", flag).returncode, 0)
            self.assertEqual(self.writes(), [])

    def test_cleanup_preserves_controllers_used_outside_playground(self):
        for namespace, kind, obj in [
            ("plous-metrics", "hpa", {"metadata": {"namespace": "another-app"}}),
            ("plous-ingress", "ingress", {"metadata": {"namespace": "another-app"}, "spec": {"ingressClassName": "plous-playground"}}),
        ]:
            self.state.write_text(json.dumps({"namespaces": {namespace: owned_namespace(namespace)}, "custom": {kind: [obj]}}))
            self.log.unlink(missing_ok=True)
            self.assertNotEqual(self.run_script("cleanup").returncode, 0)
            self.assertEqual(self.writes(), [])

    def test_credentials_are_private_scoped_and_do_not_export_owner_identity(self):
        self.state.write_text(json.dumps({"namespaces": {"plous-demo": owned_namespace("plous-demo")},
                                         "objects": {"serviceaccount/playground-observer": fixture("playground-observer")}}))
        output = self.directory / "observer.json"
        result = self.run_script("credentials", "--output", str(output))
        self.assertEqual(result.returncode, 0, result.stderr)
        config = json.loads(output.read_text())
        self.assertEqual(output.stat().st_mode & 0o777, 0o600)
        self.assertEqual(config["contexts"][0]["context"]["namespace"], "plous-demo")
        self.assertEqual(set(config["users"][0]["user"]), {"token"})
        self.assertNotIn(config["users"][0]["user"]["token"], result.stdout + result.stderr)
        self.assertEqual(self.run_script("credentials", "--output", str(output)).returncode, 0)
        output.write_text('{"unrelated": true}')
        self.assertNotEqual(self.run_script("credentials", "--output", str(output)).returncode, 0)
        self.assertEqual(output.read_text(), '{"unrelated": true}')
        output.unlink()
        output.symlink_to(self.config)
        self.assertNotEqual(self.run_script("credentials", "--output", str(output)).returncode, 0)
        self.assertNotEqual(self.run_script("credentials", "--output", str(self.config)).returncode, 0)

    def test_exercises_check_ownership_bound_load_and_keep_concurrent_changes(self):
        deployment = fixture("rollout-lab")
        deployment["spec"] = {"template": {"spec": {"containers": [{"name": "web", "image": "busybox:example"}]}}}
        state = {"namespaces": {"plous-demo": owned_namespace("plous-demo")}, "objects": {
            "deployment/rollout-lab": deployment, "configmap/playground-config": fixture("playground-config"),
            "hpa/frontend-podinfo": {"metadata": {"name": "frontend-podinfo"}}}}
        self.state.write_text(json.dumps(state))
        self.assertEqual(self.run_script("rollout-break").returncode, 0)
        call = next(c for c in self.calls() if "patch" in c["args"])
        patch = json.loads(call["args"][call["args"].index("--patch") + 1])
        self.assertEqual(patch["metadata"]["resourceVersion"], "12")
        self.assertEqual(patch["metadata"]["annotations"]["plous.dev/healthy-image"], "busybox:example")
        self.assertNotEqual(self.run_script("rollout-recover").returncode, 0)
        deployment["metadata"]["annotations"] = {"plous.dev/healthy-image": "busybox:example"}
        self.state.write_text(json.dumps(state))
        self.assertEqual(self.run_script("rollout-recover").returncode, 0)
        self.assertEqual(self.run_script("edit-conflict").returncode, 0)
        self.assertEqual(self.run_script("load", "--load-seconds", "30").returncode, 0)
        self.assertNotEqual(self.run_script("load", "--load-seconds", "601").returncode, 0)
        state["custom"] = {"jobs": [{"status": {"active": 1}}]}
        self.state.write_text(json.dumps(state))
        self.assertNotEqual(self.run_script("load").returncode, 0)
        deployment["metadata"]["labels"] = {}
        self.state.write_text(json.dumps(state))
        self.assertNotEqual(self.run_script("rollout-break").returncode, 0)


@unittest.skipUnless(shutil.which("helm"), "Helm is required to render the optional playground chart")
class PlaygroundChartTests(unittest.TestCase):
    def test_chart_lints_and_renders_all_scenarios_without_cluster_access(self):
        result = subprocess.run(["helm", "lint", str(CHART), "--strict"], capture_output=True, text=True, timeout=20)
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        result = subprocess.run([sys.executable, str(SCRIPT), "render", "--failures", "all", "--with-cert-manager", "--ingress-class", "traefik"], capture_output=True, text=True, timeout=20)
        self.assertEqual(result.returncode, 0, result.stderr)
        for name in ("ConfigMap", "Secret", "ServiceAccount", "Role", "RoleBinding", "ResourceQuota", "LimitRange", "NetworkPolicy", "Deployment", "DaemonSet", "Job", "CronJob", "StatefulSet", "Service", "Issuer", "Certificate", "Ingress", "PersistentVolumeClaim"):
            self.assertIn("kind: " + name + "\n", result.stdout)
        for name in ("crash", "image", "readiness", "scheduling", "config", "storage", "init", "job"):
            self.assertIn("name: broken-" + name + "\n", result.stdout)
        self.assertNotIn(":latest", result.stdout)
        self.assertIn("namespace: plous-failures", result.stdout)

    def test_new_profiles_render_namespace_twins_and_bounded_jobs(self):
        result = subprocess.run([sys.executable, str(SCRIPT), "render", "--with-rollout", "--list-count", "100"], capture_output=True, text=True, timeout=20)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.count("name: catalog-"), 300)
        for namespace in ("plous-list-a", "plous-list-b", "plous-list-c"):
            self.assertEqual(result.stdout.count("namespace: " + namespace + "\n"), 100)
        self.assertIn("name: rollout-lab", result.stdout)
        result = subprocess.run(["helm", "template", "plous-load", str(CHART), "--namespace", "plous-demo", "--set", "profile=load,loadSeconds=30"], capture_output=True, text=True, timeout=20)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("activeDeadlineSeconds: 60", result.stdout)
        self.assertIn("limits: {cpu: 500m, memory: 64Mi}", result.stdout)
        self.assertIn("generateName: hpa-load-", result.stdout)
