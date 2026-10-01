# Development and testing

[Documentation](README.md) · [Build and launch](getting-started.md#build-and-launch)

This guide is for contributors. User workflows start in [Get started](getting-started.md).

## Set up a checkout

Use Go 1.26.4+, Git, and Make on macOS or Linux. Executable and terminal tests require Python 3.9+ with no third-party packages. The Go race detector also requires a C compiler.

From the repository root:

```sh
make build
./bin/plous --data-dir /tmp/plous-development
```

Keep development storage separate from your normal profile. For cluster tests, use synthetic kubeconfig files and local mock APIs, or a separately authorized disposable cluster. A temporary Plous profile does not isolate the real infrastructure its connections reference.

## Repository layout

```text
cmd/plous/
  main.go                  Executable entry point
  VERSION                  Application version source
internal/
  app/                     TUI, forms, navigation, actions, and rendering
  ai/                      Eino provider adapters and model catalog
  agent/                   Outer agent, guarded dispatch, and Q&A compaction
  rlm/                     Independent recursive Starlark investigation
  memory/                  Bounded workspace record search and reads
  editor/                  Document buffer, modes, history, and YAML rendering
  diffview/                Independent comparison and review decisions
  logview/                 Retained output and source frame
  textview/                Shared search, selection, and text rendering
  kubeconfig/              Local context discovery
  kubernetes/              API access, resources, logs, and operations
  workspace/               Configuration, credentials, and chat persistence
tests/
  e2e/                     Executable and terminal tests
  live/                    Explicit Minikube acceptance tests
  fixtures/                Synthetic configuration and playground chart
docs/                      User guides and references
scripts/                   Distribution packaging and playground setup
```

Package tests stay beside their implementation as `*_test.go`. Executable and terminal tests live in `tests/`. Test helpers must not enter the application binary. `go build ./cmd/plous` is also supported; `make build` writes `bin/plous`.

Application command aliases live in `internal/app/commands.go`; resource actions share the registry in `internal/app/resource_actions.go`. Use one case-sensitive letter, choosing uppercase when lowercase is occupied. Update Help and the [keymap](keymap.md) with the handlers. Tests reject collisions, undocumented aliases, and unregistered commands or Ctrl bindings in UI text, comments, and public docs. Reserved double-key navigation, editor sequences, and Kubernetes resource abbreviations follow their existing rules.

## Reuse the view components

| Component | Current consumers | Owns |
| --- | --- | --- |
| [Plous Log Viewer](../internal/logview/logview.go) | Pod logs; Exec output review | Retained output, follow state, viewport, search, selection, copy, and output frame. |
| [Plous Diff Reviewer](../internal/diffview/diffview.go) | Kubernetes validated changes; Activity snapshot comparisons | Diff generation, comparison identity, change navigation, search, selection, and review decisions. |
| [Plous Editor](../internal/editor/editor.go) | Resource editing; read-only YAML and value inspection | Document text, cursor, selection, undo history, search, syntax, and document frame. |

The application adapters supply source data, labels, and actions. Keep Kubernetes requests, permissions, redaction, explicit writes, persistence, and panel layout in the application or service packages. Keep generic viewing behavior in these components and their shared [text primitives](../internal/textview/viewer.go).

For logs, append source text to the Log Viewer or supply an Exec snapshot. `SetFormatter` controls displayed text and styles while retention counts source bytes. Call it again when display options change. `Refresh` reuses prepared content until the source, formatter, or `Message` changes. Do not copy the retained buffer, scroll position, or search state into the adapter. Exec review holds a snapshot; live Exec still uses its terminal emulator and input transport.

For edits, create the Editor directly from the captured resource. Save validates and writes the captured document revision to its original target; the caller owns transport, conflict checks, and outcomes. Optional review receives immutable before/after snapshots and returns an identity-bound Save decision. Verify that decision still matches the document and target before writing. Never let validation alone, navigation, or a stale result authorize a write.

Read-only YAML preparation uses the captured object, resource version, scope, and value visibility as its cache identity. API updates replace the captured object; do not mutate it in place without changing its resource version. Visibility changes replace the Editor buffer and clear stale search and selection. Value inspection uses a separate read-only Editor buffer that is cleared on Back, hide, copy completion, or closing the action.

The Diff Reviewer builds comparison structure once and updates its content styling when the supplied theme changes. Restyling cannot change comparison identity, confirmation, search, or selection. The shared text viewer measures wrapped lines, then styles only visible source lines. Search and copy still operate on the complete retained content.

Names, paths, command fields, and the chat composer use the small form input in `internal/app/editor.go`. Trees, resource tables, Help, chat transcripts, Activity summaries, and operation confirmations keep their own presentation. These are separate from the document editing, comparison, and log workflows.

When changing a shared interaction, test it in the component and exercise its application routing. Preserve local input ownership, source identity, Secret masking, and explicit write confirmation. Avoid adding a second implementation in a resource-specific screen.

## Run checks

| Command | What it does |
| --- | --- |
| `make check` | Formatting check, `go vet`, and all Go and executable/terminal tests. |
| `make test` | All Go package tests, then executable/terminal tests. |
| `make test-unit` | Go package tests. |
| `make test-e2e` | Build the executable and run real terminal and CLI tests. |
| `make test-playground` | Test the playground installer with fake tools; lint/render the chart when Helm is installed. |
| `make test-ai-live` | Test the saved AI connection with synthetic evidence and real API usage; see [live AI test setup](../tests/README.md#explicit-live-ai-acceptance). |
| `make test-minikube` | Run explicitly configured live acceptance tests; see [Minikube test setup](../tests/README.md#explicit-minikube-acceptance). |
| `make test-race` | Go tests with data-race detection. |
| `make test-coverage` | Write Go coverage to `.cache/tests/coverage.out` and `.cache/tests/coverage.html`. |
| `make bench` | Measure log, diff, and YAML preparation and interaction with synthetic data. |
| `make dist` | Build distribution archives and verify their checksums and native executable. |

Test result caching is disabled by default. To focus on one Go test:

```sh
make test-unit TEST_FLAGS='-count=1 -run TestMainCommandAliases'
```

To run one terminal test:

```sh
make build
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests/e2e -p 'test_terminal.py' -k test_colon_encodings_and_numeric_modes -v
```

Set `GO` or `PYTHON` when using another interpreter, for example `make test PYTHON=python3.13`.

## Measure view performance

`make bench` reports time, bytes allocated, and allocation counts per operation. It uses 4,000 log lines, a 2,000-line comparison and YAML document, and a 1,000-key ConfigMap for YAML preparation. All data is synthetic and stays in memory. It does not connect to a cluster or provider.

Opening, unchanged redraws, scrolling, searching, and log appends are measured separately. To repeat selected measurements:

```sh
make bench BENCH_FLAGS='-bench=BenchmarkLogs -benchmem -benchtime=1s -count=3'
```

Compare the same workload on the same machine. Benchmarks have no timing pass/fail threshold and do not replace correctness tests. An unchanged preparation measurement describes reuse, not the cost of processing new output or a changed resource.

## Test isolation and coverage

The default suite uses temporary storage, synthetic configuration, local mock providers, and local Kubernetes API fixtures. It does not modify the user's `~/.plous`, kubeconfig, or clusters, or make paid AI calls. HTTP fixtures require loopback ports; terminal tests use pseudo-terminals without opening a terminal window. Live AI acceptance requires an explicit profile and authorization for provider usage. Live cluster acceptance runs separately through `make test-minikube` with an explicit configuration and cluster authorization.

For manual testing in your own disposable cluster, the [Kubernetes playground](playground.md) provides pinned Helm releases, native resources, failure cases, ingress and autoscaling options, restricted credentials, and list/rollout/edit exercises. Setup, reset, and cleanup manage the selection. Its automated tests do not deploy those resources.

| Area | Checks |
| --- | --- |
| Application | Input routing, forms, overlays, layout, search, actions, chat, Activity, and restart. |
| AI | Catalog validation, streaming, guarded tools, recursive exploration, compaction, cancellation, and credential-redacted errors. |
| Storage | Revision history, credentials, locking, interrupted writes, and corruption handling. |
| Kubernetes | Discovery, tables, watch recovery, permission handling, logs, relationships, operations, and Secret redaction. |
| Executable | CLI behavior, actual key sequences, terminal restoration, and saved state. |

Go coverage excludes the Python terminal tests. Layout tests check geometry; terminal tests check input, emitted text, state, and process behavior. Mock tests do not establish compatibility with every live vendor, Kubernetes version, or authentication plugin. Report live verification separately when it has actually been performed.

The [assistant coverage map](../tests/README.md#assistant-coverage) points to provider, tool-policy, recursive investigation, compaction, persistence, and terminal regression tests.

## Packaging and CI

`make dist` produces archives for macOS and Linux on amd64 and arm64, plus `checksums.txt`. Each archive includes the executable, license, README, docs, and logo assets. The script verifies checksums and runs the matching executable on a supported build host.

The CI workflow is configured to run `make check` and `make test-race` on Linux and macOS, with `make dist` on Linux. The release workflow is configured to run CI for version tags, package archives, and publish through GitHub. Workflow configuration alone does not establish that a hosted run or release succeeded.

Run `make check` for application changes and `make dist` for packaging changes. Keep generated binaries, archives, and coverage out of source control.

## Versioning

`cmd/plous/VERSION` is the single application version source, embedded by the entry point. Plous is currently 0.0.1. Dependency and Go toolchain versions are independent.

Do not bump the version, create release tags, or publish a release as a side effect of another change. The packaging script requires a tag-shaped version matching `v` plus the embedded source version.

## Maintain the AI catalog

`internal/ai/models.go` owns supported vendors, exact model IDs, display names, context capacities, and one initial default per vendor. Vendor setup, model selection, and requests use this allowlist. Add or retire models deliberately and verify their text streaming and tool calls through Eino.

Settings stores vendor connections separately from the resolved model selection. `Settings.Agent` is that single selection; requests capture it at operation start. Do not add task-specific routing, discovery, manual model IDs, prefix matching, or silent model fallback.

`Config.WithDefaultModel` applies during initial setup, a selected-vendor change, or removal of the selected connection. It must not reset an existing choice at startup or run as a failed-request fallback. API proxies do not expand the allowlist. The new JSONL conversation format has no old chat migration path.

Replies use text. `internal/agent` exposes the shared executor to both Eino tools and RLM callbacks. Kubernetes operations are classified and validated by `internal/kubernetes/command_policy.go`; unknown input and writes are rejected before client creation and checked again at execution. Memory accepts logical references, never filesystem paths.

The pinned Eino Anthropic adapter has a streaming initialization race. Plous delays reads from successful streaming response bodies until the adapter returns its stream, preserving streaming without a fork. Keep the provider and compacted-chat race tests when updating the adapter.

The RLM package embeds Starlark with guarded read callbacks, bounded scripts, cancellation, and shared recursive budgets. It has no Eino dependency or host I/O. Microsoft's Go compaction package summarizes active Q&A; summaries and coverage IDs live in the reasoning log. Full source records remain available. Tests use temporary stores, scripted models, and mock HTTP APIs.

Keep the catalog table in [AI chat](ai-chat.md#choose-a-model) aligned with the implementation. Run `make check` after catalog changes and distinguish mock-provider results from live calls.

## Update documentation

Use the [documentation index](README.md) as the entry point. Keep first-time setup in Getting started, workflows in their topic guides, and exhaustive bindings in Keymap. Troubleshooting should name a symptom, explain what to check, and link to the relevant workflow.

Document implemented behavior using the current labels and controls. Explain a new term before relying on it. Avoid duplicating long instructions across pages, product roadmap material, and implementation details in beginner guides. Use plain wording without em dashes.

Use [Maintain these docs](documentation.md) for the local website preview, page structure, and GitHub Pages workflow. Run `make docs-install` once, then `make docs-check` after editing guides.

For documentation changes, check relative links and section anchors, shell examples, command names, and consistency with the current app. Preserve useful existing page URLs when reorganizing content.
