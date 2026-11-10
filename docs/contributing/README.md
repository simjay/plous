# Contributing and development

[Documentation](../README.md)

Keep changes focused. Discuss product, navigation, or storage changes before building a large feature. Plous is still in early development.

## Choose your task

| Task | Start here |
| --- | --- |
| Understand the application | [Build and layout](#build-and-layout) and [design principles](#design-principles) |
| Change agents or evidence handling | [AI implementation](ai-assistant.md) |
| Verify a change | [Run checks](#run-checks) and [test setup](../../tests/README.md) |
| Measure tool behavior | [Benchmarks](#benchmarks) |
| Prepare a release | [Releasing Plous](releases.md) |
| Update or deploy the docs | [Website maintenance](website.md) |

## Build and layout

Use the Go version in `go.mod`, Make, and Python 3.11 or newer. `make build` produces `bin/plous`.

`cmd/plous/VERSION` is the application version source. Change it or create release tags only for an authorized release.

| Package | Owns |
| --- | --- |
| `cmd/plous` | Executable entry and version |
| `internal/app` | Command-line entry and interactive/standalone dispatch |
| `internal/app/tui` | Interactive lifecycle, global input, panel composition, and feature coordination |
| `internal/app/terminal` | Host terminal input/output reservation and restoration |
| `internal/app/{chat,monitor,settings,activity}` | Feature controllers and contextual Help |
| `internal/app/navigation` | Prepare agent-requested views for the app loop |
| `internal/app/debugging` | Standalone coding-agent setup and MCP startup |
| `internal/ui` | Shared text, Markdown, logs, diff and editor components |
| `internal/ai` | Turn lifecycle, selected backend, Eino loop, guarded tool session and compaction |
| `internal/ai/agent` | Standard-library-only backend and tool contracts |
| `internal/ai/cli` | Installed runtime lifecycle, native protocols and MCP transport |
| `internal/ai/model` | Provider adapters, catalog, requests and usage |
| `internal/ai/history` | Conversation storage, projected evidence, Sources and indexes |
| `internal/integrations/<tool>/{monitor,common}` | Integration reads, projections and schema rules |
| `internal/workspace` | Profile, revisions, credentials and scope identities |
| `benchmark` | Tool benchmark toolkit. Production-session checks in `benchmark/integration` |
| `tests` | Executable, terminal and cross-package tests and fixtures |

Keep these dependencies one-way:

- Integrations do not import AI, history, or UI packages.
- Shared integration packages do not depend on Monitor controllers.
- Feature controllers do not import `app/tui`.
- `ai/agent` uses only the standard library.
- `ai/cli` uses the standard library, `ai/agent`, PTY support, and TOML encoding. It does not import provider, UI, history, workspace, or integration implementations.

Keep package unit tests beside implementation. Test helpers belong in `_test.go` or `tests/support`, outside the application binary. Architecture tests enforce these boundaries.

## Design principles

Give each rule one owner. Add an abstraction only when it removes duplication or enforces a boundary. Switch consumers and remove the old path together.

| Rule | Owner and enforcement |
| --- | --- |
| Calculate facts once | Integrations own workload readiness, measurements, budgets and quota accounting. Tables, Describe, charts and tool projections consume those facts. |
| Preserve meaning until display | Keep identity, units, source/collection times, coverage and unknown values typed. Formatting never drives arithmetic or numeric sorting. |
| Enforce agent authority once | The host tool session captures scope, serializes calls, saves evidence before delivery and closes access with the turn. MCP transports that contract. |
| Keep human actions explicit | The TUI calls shared Go logic directly. Its separate save path validates the exact draft and target. Shared facts do not grant agents write access. |
| Resolve input ownership once | Root composition selects the active panel or overlay. Its controller owns local input and retained state. Mouse and keyboard actions use the same guards. |
| Measure before drawing | Shared viewport measurement supplies wrapped rows, bounds and hit areas. Controllers update selection and scales before rendering. |

Preserve saved identities, exact revisions, and supported settings. Remove retired execution paths and their tests together.

Use independent domain fixtures for expected facts. Test schema arguments, direct/MCP parity, input ownership, and layout geometry. Separate live reads can have different observation times.

## Reuse components and controls

Shared viewers own text layout, selection, search, scrolling, and theme. App adapters supply data and actions. Keep API calls, credentials, persistence, and writes outside viewers.

The Editor owns the draft and undo history. The Diff Reviewer returns a decision tied to that draft and target. Callers validate before writing. Opening or reviewing a document does not authorize a save.

Use the shared Monitor namespace picker and typed action IDs. Dispatch must not depend on display labels.

`app/tui/panels.go` chooses the input owner. Inactive searches keep state without receiving input. `ui/viewport.go` supplies measured rows and bounds. Rendering must not change selection or scales.

Keep application aliases in `app/tui/commands.go` and resource actions in their registry. Update handlers, Help, and the [keymap](../reference/keymap.md) together. Aliases use one case-sensitive letter.

## Run checks

| Command | Purpose |
| --- | --- |
| `make check` | Documentation, formatting, vet, Go tests and terminal tests |
| `make test` | All Go and executable tests |
| `make test-unit` | All Go tests, including integration, acceptance and PlousBench |
| `make test-e2e` | Build and run CLI/terminal journeys |
| `make test-ux` | Focused screen matrix, overlays, drafts and restart |
| `make test-race` | Go data-race checks |
| `make test-coverage` | Coverage under `.cache/tests/` |
| `make eval` | Focused subset of Go acceptance tests |
| `make test-plousbench` | Offline tool, evidence and report checks |
| `make bench-plousbench` | Repeated tool workloads through direct calls and MCP |
| `make build-plousbench` | Tool report CLI |
| `make bench` | Synthetic view performance measurements |
| `make docs-check` | Markdown navigation, links and anchors |
| `make dist` | Local distribution archives and verification |

Focused examples:

```sh
make test-unit \
  TEST_PACKAGES=./internal/ai/... \
  TEST_FLAGS='-count=1 -run TestName'

make test-e2e TEST_E2E_PATTERN=test_chat.py
```

`GO` and `PYTHON` select alternate installed tools. Use `TEST_PACKAGES` with race checks too.

Default tests use temporary profiles, synthetic kubeconfig, loopback APIs, and scripted models. They do not use the owner's clusters or paid providers. Live provider and Minikube checks require explicit opt-in.

Run `make check` for application changes and `make dist` for packaging. See [test setup](../../tests/README.md). Scripted models verify mechanics, not answer quality.

## Continuous integration

CI runs on main pushes and pull requests. Jobs independently check source docs, formatting, vet, Go tests, terminal tests, and archives.

Linux Go tests use race detection. macOS runs the normal Go suite. Both platforms run terminal tests with tmux. Optional playground chart tests run when Helm is available.

Terminal failures retain diagnostics for seven days. Release waits for every CI job, verifies the tag against `VERSION`, and publishes the already-built archives after checksum checks.

## Screen and performance review

The screen matrix covers 80×16 through 240×60 with side panels shown and hidden. Terminal journeys check the current rendered screen rather than earlier output.

```sh
PLOUS_REVIEW_DIR=/tmp/plous-review make test-ux
```

Review compact and wide layouts after visual changes. Native stdout and stderr must remain inside the renderer.

`make bench` measures opening, redraw, scrolling, search, and append workloads. Compare the same workload on the same machine. There is no timing pass/fail threshold.

## Providers

`internal/ai/model/models.go` owns model IDs, names, capacities, and defaults. Each operation captures one application-wide selection. Defaults are not failed-request fallbacks. Proxies do not expand the catalog.

The pinned Eino Anthropic adapter delays successful-body reads until the stream is returned to avoid an initialization race. Preserve race coverage when upgrading it. Mock tests do not verify a live provider.

## Benchmarks

[PlousBench](../../benchmark/README.md) checks tool contracts and evidence through direct calls and MCP. Use `make test-plousbench` for offline checks and `make bench-plousbench` for measurements.

Reports describe fixture workloads and run conditions. They do not measure live-provider answer quality.

## Documentation and packaging

Write task instructions in guides, exact controls in Keymap, settings in Configuration, and implementation details here. Keep one source under `docs/`. The [website guide](website.md) covers rendering and publication.

Use short sentences and concrete names. Put an action before its edge cases. Avoid semicolons and em dashes in prose. Keep warnings beside the operations they affect. Review rendered pages as well as links.

Do not publish proposals, private paths, or local decision files. Preserve incoming URLs when moving content.

Run `make docs-check` for Markdown. Website changes also need `make website-check` and `make website-build`. Check examples and code colors in both static HTML and live Rune rendering.

See [releases](releases.md) for application archives and Homebrew. Keep generated outputs out of source control.
