#!/usr/bin/env python3
"""Record installer requests without contacting a cluster or registry."""

import json
import base64
import os
from pathlib import Path
import sys

args = sys.argv[1:]
program = Path(sys.argv[0]).name
state_path = Path(os.environ["PLAYGROUND_FAKE_STATE"])
state = json.loads(state_path.read_text())
body = sys.stdin.read() if "-" in args else ""
record = {"program": program, "args": args, "body": body}
if "--values" in args:
    record["values"] = json.loads(Path(args[args.index("--values") + 1]).read_text())
with Path(os.environ["PLAYGROUND_FAKE_LOG"]).open("a") as output:
    output.write(json.dumps(record) + "\n")

for flag in ("--context", "--kube-context", "--kubeconfig"):
    if flag in args:
        i = args.index(flag)
        del args[i:i + 2]
args = [a for a in args if not a.startswith("--request-timeout=")]
namespace = args[args.index("--namespace") + 1] if "--namespace" in args else None
result = ""

if program == "helm":
    if args[0] == "list":
        result = [{"name": n} for n in state.get("releases", {}).get(namespace, [])]
    elif args[0] == "upgrade":
        release = args[2]
        releases = state.setdefault("releases", {}).setdefault(namespace, [])
        if release not in releases:
            releases.append(release)
    elif args[0] == "uninstall":
        state["releases"][namespace].remove(args[1])
else:
    if "--patch-file" in args and args[args.index("--patch-file") + 1] == "-":
        print("unable to read patch file: open -: no such file or directory", file=sys.stderr)
        sys.exit(1)
    if args[:2] == ["get", "namespace"]:
        result = state.get("namespaces", {}).get(args[2], "")
    elif args[:2] == ["get", "storageclass"]:
        result = {"items": state.get("storageclasses", [{"metadata": {"name": "standard", "annotations": {"storageclass.kubernetes.io/is-default-class": "true"}}}])}
    elif args[:2] == ["get", "crd"]:
        result = state.get("crds", {}).get(args[2], "")
    elif args[:2] == ["get", "ingressclass"]:
        result = state.get("ingressclasses", {}).get(args[2], "")
    elif args[:2] == ["get", "apiservice"]:
        result = state.get("apiservices", {}).get(args[2], "")
    elif args[:2] == ["get", "--raw"]:
        if state.get("metrics_error"):
            print("Metrics API unavailable", file=sys.stderr)
            sys.exit(1)
        result = {"items": []}
    elif args[0] == "get":
        result = state.get("objects", {}).get(args[1] + "/" + args[2], "") if "--ignore-not-found" in args else {"items": state.get("custom", {}).get(args[1], [])}
    elif args[:2] == ["config", "view"]:
        assert args[-1] == "jsonpath={.clusters[0].cluster}"
        result = {"server": "https://synthetic-cluster.example", "certificate-authority-data": "ZmFrZQ=="}
    elif args[:2] == ["create", "token"]:
        payload = base64.urlsafe_b64encode(json.dumps({"exp": 2000000000}).encode()).decode().rstrip("=")
        result = "fake." + payload + ".not-a-real-credential"
    elif args[0] == "create":
        if body.startswith("{"):
            obj = json.loads(body)
            state.setdefault("namespaces", {})[obj["metadata"]["name"]] = obj
        else:
            result = "job.batch/hpa-load-fake created"
    elif args[:2] == ["delete", "namespace"]:
        state["namespaces"].pop(args[2], None)

state_path.write_text(json.dumps(state))
print(json.dumps(result) if not isinstance(result, str) else result)
