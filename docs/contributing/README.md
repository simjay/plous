# Contributing and development

[Documentation](../README.md)

Plous is an early development build. Keep changes focused and discuss product, navigation or storage changes before building large features. Report bugs with version, OS, terminal, reproduction steps and the full sanitized error. Never attach credentials, kubeconfig or a private profile.

## Build and layout

Use the Go version in go.mod, Make and Python 3.9+ for documentation/terminal checks. Build with `make build`; the executable is `bin/plous`. The embedded `cmd/plous/VERSION` is the application version source. Do not bump it or create release tags as part of another change.

| Package | Owns |
| --- | --- |
| `cmd/plous` | Executable entry and version |
| `internal/app` | Application lifecycle and composition |
| `internal/app/{chat,monitor,code,settings,activity}` | Feature controllers and contextual Help |
| `internal/app/navigation` | Prepare agent-requested views for the app loop |
| `internal/ui` | Shared text, Markdown, logs, diff and editor components |
| `internal/ai` | Turn lifecycle, selected backend, Eino loop, guarded tool session and compaction |
| `internal/ai/agent` | Standard-library-only backend and tool contracts |
| `internal/ai/cli` | Installed runtime lifecycle, native protocols and MCP transport |
| `internal/ai/model` | Provider adapters, catalog, requests and usage |
| `internal/ai/history` | Conversation storage, projected evidence, Sources and indexes |
| `internal/integrations/<tool>/{monitor,code,common}` | Integration reads, projections and schema rules |
| `internal/source` | Source acquisition and cross-tool composition |
| `internal/workspace` | Profile, revisions, credentials and scope identities |
| `benchmark` | Tool benchmark toolkit; production-session checks in `benchmark/integration` |
| `tests` | Executable, terminal and cross-package tests and fixtures |

Monitor and Code do not depend on each other; common depends on neither. Integrations do not import AI, history, source or UI. Keep unit tests beside implementation and test helpers in `_test.go` files or `tests/support`, outside production packages. Keep feature Help beside its controller, including separate Helm and GitOps Help. Root app files coordinate navigation, lifecycle and persistence with the feature controllers. Chat state and readers belong in `app/chat`; the agent selector belongs in `app/settings`. The CLI package may import the standard library, `ai/agent`, the PTY library and the TOML encoder; it cannot import UI, provider, history, profile or integration implementations. The agent contract itself remains standard-library-only. Architecture tests enforce these dependencies. A normal projection extension changes its integration, registration and tests, not the agent architecture. See [AI implementation](ai-assistant.md).

## Design principles

Keep one owner for each rule and use direct Go calls between the existing packages. An abstraction should remove a duplicated rule or enforce a boundary; it should not create a second registry, a universal resource model, or a controller framework. Remove retired execution paths and their tests together. Preserve saved identities, exact revisions and supported settings when simplifying storage or navigation.

| Rule | Owner and enforcement |
| --- | --- |
| Calculate facts once | Integrations own workload readiness, measurements, budgets and quota accounting. Tables, Describe, charts and tool projections consume those facts. |
| Preserve meaning until display | Keep identity, units, source/collection times, coverage and unknown values typed. Formatting never drives arithmetic or numeric sorting. |
| Enforce agent authority once | The host tool session captures scope, serializes calls, saves evidence before delivery and closes access with the turn. MCP transports that contract. |
| Keep human actions explicit | The TUI calls shared Go logic directly. Its separate save path validates the exact draft and target; shared facts do not grant agents write access. |
| Resolve input ownership once | Root composition selects the active panel or overlay. Its controller owns local input and retained state; mouse and keyboard actions use the same guards. |
| Measure before drawing | Shared viewport measurement supplies wrapped rows, bounds and hit areas. Controllers update selection and scales before rendering. |

Domain fixtures specify expected facts independently. Consumer tests check shared meaning; [schema tests](../../internal/ai/tool_schema_test.go) check advertised arguments; [tool parity tests](../../internal/ai/tool_parity_test.go) check direct and MCP delivery, scope and durable evidence. [Interaction tests](../../internal/app/interaction_test.go) and [viewport tests](../../internal/ui/viewport_test.go) check input ownership and geometry. Identical results from separate live reads are not required because observation times can differ.

## Reuse components and controls

Shared viewers own text, selection, scrolling, search and theme. App adapters supply data, target identity and actions. Keep API calls, credentials, persistence and writes outside viewers. The Editor owns drafts/undo; the Diff Reviewer returns an identity-bound decision. The caller validates the exact draft and target before writing. Navigation or validation alone never authorizes a save.

`internal/app/interaction.go` resolves panel and overlay ownership for local input. Inactive searches retain state without receiving keys, mouse actions or paste. Visibility checks must not capture connections or initialize feature state. `internal/ui/viewport.go` supplies wrapped rows, visible bounds and scroll limits to charts and read-only details. Rendered mouse markers travel with those rows; rendering must not change selection or scale state.

Use the shared text theme and form input rather than adding resource-specific copies. Markdown rendering does not fetch content or execute links. Log retention counts source bytes, even when formatting changes. Preserve original text for copy/search and captured identity for caches.

Application aliases live in `internal/app/commands.go`; resource actions have a shared registry. Use one case-sensitive letter and update handler, contextual Help and [keymap](../reference/keymap.md) together. Keep field/editor/terminal input ownership intact. Tests check aliases, collisions and advertised actions.

## Run checks

| Command | Purpose |
| --- | --- |
| `make check` | Documentation, formatting, vet, Go tests and terminal tests |
| `make test` | All Go and executable tests |
| `make test-unit` | Go package tests |
| `make test-e2e` | Build and run CLI/terminal journeys |
| `make test-ux` | Focused screen matrix, overlays, drafts and restart |
| `make test-race` | Go data-race checks |
| `make test-coverage` | Coverage under `.cache/tests/` |
| `make eval` | Scripted single-agent acceptance cases |
| `make test-plousbench` | Offline tool, evidence and report checks |
| `make bench-plousbench` | Repeated tool workloads through direct calls and MCP |
| `make build-plousbench` | Tool report CLI |
| `make bench` | Synthetic view performance measurements |
| `make docs-check` | Markdown navigation, links and anchors |
| `make dist` | Local distribution archives and verification |

Focus a Go run with `make test-unit TEST_PACKAGES=./internal/ai/... TEST_FLAGS='-count=1 -run TestName'`. Use `TEST_PACKAGES` with `test-race` too. Focus a terminal run with `make test-e2e TEST_E2E_PATTERN=test_chat.py`. `GO` and `PYTHON` select alternate installed tools.

The default suite uses temporary profiles, synthetic kubeconfig and loopback APIs. It does not alter the owner's profile or clusters and does not call paid providers. HTTP fixtures need loopback ports; terminal tests need pseudo-terminals. `make test-ai-live` and `make test-minikube` require explicit opt-in for their provider or cluster. See [test setup and coverage](../../tests/README.md) and [benchmark instructions](../../benchmark/README.md).

Verify the changed behavior and meaningful failure cases. Run make check for application changes and make dist for packaging changes. Distinguish fixtures from live verification. Scripted models test mechanics, not answer quality.

## Screen and performance review

The screen matrix spans 80x16 to 240x60 with side panels shown/hidden. Terminal journeys reconstruct the actual current screen and check visible content, not only earlier output. Native stdout/stderr must not escape the renderer. Use `PLOUS_REVIEW_DIR=/tmp/plous-review make test-ux` to retain synthetic review frames and inspect compact/wide layouts after visual changes.

`make bench` measures opening, unchanged redraws, scrolling, search and appends separately. Compare the same workload and machine; there is no timing pass/fail threshold. Override `BENCH_FLAGS` for selected repeated measurements. Performance does not replace correctness checks.

## Providers

The maintained catalog in `internal/ai/model/models.go` owns exact IDs, display names, capacities and initial defaults. One saved application-wide selection is captured per operation. Defaults apply to setup or an explicit connection change, never failed-request fallback. API proxies do not expand the allowlist. Verify streaming/tool calls and keep the public model table aligned.

The pinned Eino Anthropic adapter needs delayed reads from successful response bodies until its stream is returned to avoid a streaming initialization race. Preserve provider race coverage when upgrading it. Local mock results do not verify a vendor service.

## Documentation and packaging

Maintain Markdown under docs/. Give each guide one title and an index link. Keep the README concise: purpose, features, a working quick start, MCP, documentation links, contributing, and license. The documentation index groups guides, integrations, references, and development material. Keep setup and examples in guides, detailed contracts in reference pages, and implementation details in contributing docs. Keep a single page per workflow, detailed controls in Keymap and settings in Configuration. Write actual current behavior, with plain wording and no em dashes. Update incoming links when moving a page. Do not copy proposals, internal decision records or private paths into public guides.

`make docs-check` needs no third-party Python packages and does not contact websites. Read the Markdown after structural changes. Command-registry tests also scan nested guides.

`make dist` builds macOS/Linux archives for amd64/arm64 with executable, license, README, guides and assets, verifies checksums and runs the matching executable. It does not publish. Hosted CI/release success must be verified separately. Keep binaries, reports, private data and local dot-directories out of source control; root `.github/` is the workflow exception.
