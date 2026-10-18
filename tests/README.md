# Tests

`e2e/` exercises the built Plous executable through its CLI and a pseudo-terminal, and the playground installer through fake command-line tools. `fixtures/` contains synthetic kubeconfig data, Kubernetes resource examples, and the playground chart. Automated tests create state in temporary directories and clean it up after each test.

Run `make test` from the repository root for the complete suite, or `make test-e2e` for these tests only. Python 3.9+ is required; no Python packages need to be installed.

Executable tests first run `--version` with a separate 30-second startup deadline. This lets macOS finish its security assessment of a newly built executable before interactive checks begin. The UI wait limit remains 8 seconds. A failed launch stops the suite with its exit code, signal, or launch timeout; no retry or security-setting change is performed. `make test-ux` uses the same startup check.

Go unit tests live beside the packages they test under `internal/`. Cross-package AI tests live in `integration/ai/`, deterministic algorithm acceptance in `acceptance/ai/`, and package ownership rules in `architecture/`. See the [development and testing guide](../docs/contributing/README.md) for commands, coverage, and the repository layout.

`make docs-check` checks Markdown titles, local links, section anchors, navigation from the documentation index, and links to local internals. It uses Python with no third-party packages and also runs as part of `make check`. Go command-registry tests scan the nested guides for documented commands and shortcuts. See [documentation maintenance](../docs/contributing/README.md#documentation-and-packaging).

`make test-playground` checks the installer independently of the application build. Chart lint/render checks run when Helm is installed. Manual setup instructions are in the [playground guide](../docs/integrations/kubernetes/playground.md); the default test suite never deploys to Minikube.

## Keyboard compatibility

`e2e/test_keyboard.py` sends actual bytes through the executable's PTY reader. It covers xterm CSI and application-cursor SS3 keys, Escape-prefixed Alt, Mac word arrows, iTerm2 Meta arrows, CSI-u, Kitty lock modifiers and key releases, xterm modifyOtherKeys, and Win32 input records. Each family checks panel travel, Unicode editing, Enter, Tab tree/main switching and local input Tab/Shift+Tab, literal bracketed paste, Help and Ctrl+C. BS-style Backspace has a separate text-input regression. When tmux is installed, the same journey runs through a separate server with `/dev/null` configuration and a temporary socket, leaving the user's server and configuration alone.

Run `make test-e2e TEST_E2E_PATTERN=test_keyboard.py` for this matrix. `test_external_agents.py` checks panel keys during native input capture and retained drafts. Shared terminal encoder unit tests cover modified editing keys, keypad navigation, repeat metadata and lock states; shared text/editor tests keep BS-style Backspace restricted to text input.

The formats follow the [Kitty protocol](https://sw.kovidgoyal.net/kitty/keyboard-protocol/), [WezTerm encoding reference](https://wezterm.org/config/key-encoding.html), [iTerm2 modifier settings](https://iterm2.com/documentation-preferences-profiles-keys.html), and [Windows input sequences](https://learn.microsoft.com/en-us/windows/console/console-virtual-terminal-sequences#input-sequences). They represent common terminal input paths, not a claim that every terminal GUI was launched. In particular, Win32 record coverage does not add native Windows support. Terminal shortcut interception, modifier loss, clipboard permissions, and font/rendering differences still need verification in the relevant app and profile. See the [terminal troubleshooting checks](../docs/reference/troubleshooting.md#panel-or-editing-keys-do-not-work).

## Assistant coverage

Default tests use temporary profiles, scripted models, and synthetic tool results. They do not use saved credentials, paid providers, or live clusters. Mock results do not establish live-provider compatibility.

Run `make test-unit` for provider transport, credential redaction, captured tool policies, projected persistence, scope isolation, pagination, history calculations, compaction, cancellation, budgets and restart/Sources. `make eval` exercises deterministic single-agent acceptance. The built-in runtime needs no interpreter or worker sandbox. Native-agent fixtures verify PTY input and output, unexpected process exit, launch configuration validation, inherited tool exclusion and bridge revocation. Native CLI checks remain an explicit [local mock-provider test](../docs/contributing/ai-assistant.md#verify-follow-ups-and-history).

`e2e/test_external_agents.py` exercises native PTY screens for both agents, Settings selection, literal keys, Ctrl+C forwarding in legacy/CSI-u/modifyOtherKeys encodings, Ctrl+] then Ctrl+C shutdown, resize, process closure, persisted CLI conversations, restart and a missing executable without accessing a vendor account. Native tool-session tests verify captured scope, durable observations and restart. Installed-binary mock-provider tests also inspect the actual model tool inventory and disabled execution host.

Use `make test-race` for concurrency and `make test-e2e` for executable workflows. See [focused commands](../docs/contributing/README.md#run-checks). Scripted models verify mechanics, not provider answer quality.

Shared-fact tests cover missing versus zero values, stale generations, replacement identities, workload ownership, asynchronous measurements and incomplete totals. The [schema tests](../internal/ai/tool_schema_test.go) check integration argument contracts. The [production tool parity fixture](../internal/ai/tool_parity_test.go) sends Kubernetes usage through direct `agent.Tools` and the native Claude launcher's actual MCP bridge with a fixture executable. It checks schemas/results, scope denial before acquisition, closed sessions, persistence before delivery and reopened evidence. This fixture does not establish parity for every operation or the Codex route.

## Explicit live AI acceptance

`test-ai-live` is a one-request native Go smoke test with an explicit saved-profile opt-in and temporary conversation storage. It checks provider connectivity, not answer quality. PlousBench runs tools without a provider. See [provider verification](../docs/contributing/ai-assistant.md#verify-follow-ups-and-history).

Local acceptance tests do not incur provider charges. A provider result verifies only the connection and path actually exercised; it does not verify another model or a live cluster.

## Explicit Minikube acceptance

See [PlousBench](../benchmark/README.md) for synthetic tool checks and measurements that need no model or real cluster.

`make test-minikube` is separate from `make test` and requires `PLOUS_MINIKUBE_TEST_KUBECONFIG` to name a private JSON kubeconfig whose current context is `minikube`. Run it only with authorization to test that cluster. It expects the prepared playground described in [the playground guide](../docs/integrations/kubernetes/playground.md), including the certificate example.

The target runs API and real-terminal checks using temporary Plous storage. It creates uniquely named, labeled namespaces for bounded test workloads, edits, scale, restart, deletion, logs, Exec, and limited-access credentials. Cleanup removes those test namespaces. The prepared `plous-demo` resources are inspected read-only; the supplied configuration and external active context remain unchanged. Unit tests use a synthetic credential plugin for renewal checks, not installed cloud credentials.

```sh
PLOUS_MINIKUBE_TEST_KUBECONFIG=/private/path/minikube.json make test-minikube
```

Kubernetes client tests remain beside the implementation. Terminal acceptance lives in `tests/live/`, outside default executable-test discovery. On interrupted tests, inspect any remaining `plous-operations-*` or `plous-ui-acceptance-*` namespaces and verify the `app.kubernetes.io/managed-by=plous-acceptance` label before cleanup.

## PlousBench toolkit

[PlousBench](../benchmark/README.md) lives in `benchmark/`, with production-tool tests in `benchmark/integration/*_test.go`. `make test-plousbench` checks explicit facts, history/Sources, scope, pagination, failure handling and MCP parity. `make bench-plousbench` repeats those workloads and measures tool latency, response size and API reads. Neither target runs an agent or model. Earlier agent reports remain historical artifacts.

Kustomize uses [backend tests](../internal/integrations/kustomize/code/kustomize_test.go), [Code inspector tests](../internal/app/code_inspector_test.go), and [terminal tests](e2e/test_kustomize.py). They exercise captured inputs, temporary Git repositories, dependency boundaries, graph and tree layouts, inline YAML, field navigation, finding links, and masked comparisons. The terminal fixture never contacts a cluster. Synthetic native builds, including a metadata-only builtin transformer and composed Helm charts, run with an empty executable search path. Helm terminal tests use local Kubernetes API and chart repository fixtures. Native download tests include a local authenticated OCI registry.

Startup regressions use deprecated Kustomize fields and a restored Sources view with automatic rendering. The terminal test captures stderr separately and rejects output outside the renderer. [Output boundary and frame tests](../internal/app/terminal_output_test.go) cover direct stdout/stderr writes, logger output, restoration on exit, and exact terminal bounds during loading, rendering, completion, and failure. Include these states when changing native integrations or Code layouts; rendered-string checks alone do not catch stray terminal writes.

[Argo and Flux terminal tests](e2e/test_gitops.py) use a synthetic Kubernetes API and temporary profile. They cover controller discovery, split sync and health, dependency and resource navigation, namespace permissions, partial and stale observations, and empty results. Repository and artifact references point to a local trap endpoint so an unintended source fetch fails the test. No live cluster, controller CLI, or source repository is used.

## UX acceptance

`make test-ux` runs the [screen matrix](../internal/app/ux_review_test.go) and [current-screen terminal journeys](e2e/test_ux.py). Both also run in `make check`. The matrix covers startup, workspace setup, Settings, Help, chat, Kubernetes, Helm, Code, and shared viewers at five sizes from 80×16 to 240×60, with side panels shown and hidden. It checks exact frame dimensions, retained drafts and scope, and visible primary content in compact readers.

[Chart terminal journeys](e2e/test_usage.py) cover inspection, range changes, latest selection, fullscreen, Help, retained search and reduced-color output. [Interaction tests](../internal/app/interaction_test.go) cover the active input owner and matching keyboard/mouse behavior; [viewport tests](../internal/ui/viewport_test.go) cover wrapping, clipping and reachable final rows. These complement independently expected measurement facts, rather than deriving expected values from the chart renderer.

Terminal journeys replay cursor movement, erases, Unicode, and resizing through the [VT helper](support/terminalscreen/main.go). Use `expect_screen` for visible controls and content. Historical output searches can match a vanished screen or miss text assembled through partial redraws. The helper is built only for tests and is excluded from distribution archives.

Input writes and brief output reads do not confirm that the app has handled every event. After large input bursts, wait for an observable transition caused by a later event before comparing screens. Send double clicks with `mouse_double_click` so output-read waits cannot split the gesture beyond the application's double-click window.

Run `PLOUS_REVIEW_DIR=/tmp/plous-ux-review make test-ux` to retain ANSI frames and terminal text for visual review. Use the same variable with `make check` for the full suite, including restored native rendering. Inspect compact and wide captures after changing layouts; geometry checks do not establish readability. Failed terminal waits distinguish exit codes, termination signals, and a process that is still running. They include elapsed time, received output size, and the current screen. With a review directory, they also save diagnostic JSON and replayable terminal events. Startup checks repeatedly open fresh and saved profiles without keyboard input at three terminal sizes. Captures use synthetic profiles and must not contain real credentials or cluster data.

The [Argo and Flux playground](../docs/integrations/gitops/playground.md) provides separately installed live Minikube examples in `fixtures/playground/gitops`. Its pinned controllers and deliberate failures are manual fixtures, outside the default automated suite.
