# Executable tests

`e2e/` exercises the built Plous executable through its CLI and a pseudo-terminal, and the playground installer through fake command-line tools. `fixtures/` contains synthetic kubeconfig data, Kubernetes resource examples, and the playground chart. Automated tests create state in temporary directories and clean it up after each test.

Run `make test` from the repository root for the complete suite, or `make test-e2e` for these tests only. Python 3.9+ is required; no Python packages need to be installed.

Go unit tests live beside the packages they test under `internal/`. See the [development and testing guide](../docs/development.md) for commands, coverage, and the repository layout.

Documentation checks run separately with `make docs-check` after `make docs-install`. The tests in `docs/` check website link conversion and reject links to local internals. The production build then checks page coverage, rendered links, anchors, assets, search, and documented application commands. See [documentation maintenance](../docs/documentation.md).

`make test-playground` checks the installer independently of the application build. Chart lint/render checks run when Helm is installed. Manual setup instructions are in the [playground guide](../docs/playground.md); the default test suite never deploys to Minikube.

## Assistant coverage

Default assistant tests use temporary profiles, scripted models, and local HTTP fixtures. They do not use saved credentials, call paid providers, or access a live cluster. Mock results do not establish live-provider compatibility.

| Behavior | Tests |
| --- | --- |
| OpenAI and Anthropic requests, streaming, cancellation, credential redaction, errors, and tool-call responses | [Provider adapters](../internal/ai/client_test.go), [provider tool loops](../internal/agent/provider_wire_test.go) |
| Direct and recursive tools, captured scope/model, shared limits, and evidence excluded from later Q&A prompts | [Agent orchestration](../internal/agent/agent_test.go), [Kubernetes integration](../internal/agent/kubernetes_integration_test.go), [limits](../internal/agent/limits_test.go) |
| Compaction preserves source records, recovers from failure or cancellation, and produces valid Anthropic requests after reopening | [Compaction](../internal/agent/agent_test.go), [failure recovery](../internal/agent/compaction_failure_test.go), [Anthropic compaction and restart](../internal/agent/anthropic_context_test.go) |
| Read-only policy, rejected writes, namespace scope, bounded output, and sanitization | [Command policy](../internal/kubernetes/command_policy_test.go), [Kubernetes reads](../internal/kubernetes/agent_reads_test.go) |
| Recursive scripts, retained variables, restricted host access, cancellation, and resource limits | [RLM runtime](../internal/rlm/runtime_test.go), [adversarial cases](../internal/rlm/adversarial_test.go) |
| Workspace isolation, stable references, bounded retrieval, interrupted appends, and streaming checkpoints | [Memory](../internal/memory/memory_test.go), [JSONL records](../internal/workspace/records_test.go) |
| Setup, model selection, send/stop, investigation progress, evidence retrieval, and reopening through real terminal input | [Chat terminal tests](e2e/test_chat.py) |

Use `make test-unit` for package checks, `make test-race` for concurrency checks, and `make test-e2e` for executable workflows. See [focused test commands](../docs/development.md#run-checks) to run individual cases.

## Explicit live AI acceptance

Run this only when provider usage is authorized. It uses the profile's saved model, endpoint, instructions, and credential, and incurs real API usage. The profile is read without opening its writable store. Test conversations and settings copies stay in temporary storage; credentials stay in memory. Only synthetic evidence is sent. The Kubernetes check uses a local API fixture and a temporary kubeconfig.

```sh
PLOUS_LIVE_AI_PROFILE="$HOME/.plous" make test-ai-live
```

[Live provider tests](../internal/agent/live_test.go) check streamed replies, memory search and read, guarded Kubernetes reads, recursive investigation with a child call, and compaction followed by reopening. Compaction uses a reduced test context budget to keep API usage small. The tests verify execution records, not just the answer text. A failure can reflect provider availability or model behavior; there are no retries or fallback models. Logs show the selected provider/model and reported token usage without credentials or saved conversation content.

Without `PLOUS_LIVE_AI_PROFILE`, the live test skips. It does not establish compatibility for other saved connections or models, and it does not test a live cluster.

## Explicit Minikube acceptance

`make test-minikube` is separate from `make test` and requires `PLOUS_MINIKUBE_TEST_KUBECONFIG` to name a private JSON kubeconfig whose current context is `minikube`. Run it only with authorization to test that cluster. It expects the prepared playground described in [the playground guide](../docs/playground.md), including the certificate example.

The target runs API and real-terminal checks using temporary Plous storage. It creates uniquely named, labeled namespaces for bounded test workloads, edits, scale, restart, deletion, logs, Exec, and limited-access credentials. Cleanup removes those test namespaces. The prepared `plous-demo` resources are inspected read-only; the supplied configuration and external active context remain unchanged. Unit tests use a synthetic credential plugin for renewal checks, not installed cloud credentials.

```sh
PLOUS_MINIKUBE_TEST_KUBECONFIG=/private/path/minikube.json make test-minikube
```

Kubernetes client tests remain beside the implementation. Terminal acceptance lives in `tests/live/`, outside default executable-test discovery. On interrupted tests, inspect any remaining `plous-operations-*` or `plous-ui-acceptance-*` namespaces and verify the `app.kubernetes.io/managed-by=plous-acceptance` label before cleanup.
