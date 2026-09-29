# Development and testing

Plous has one executable, with its entry point at the repository root. Application packages live under `internal/`, with Go unit tests alongside each package.

## Repository layout

```text
main.go                    Executable entry point
VERSION                    Single application version source
internal/
  app/                     Startup, TUI, forms, input, and rendering
    *_test.go              Application behavior and layout tests
  kubeconfig/              Local kubeconfig discovery and its unit tests
  workspace/               Persistence and its unit tests
tests/
  e2e/                     Tests of the built executable and real terminal input
  fixtures/                Synthetic configuration used by tests
docs/                      Public documentation
scripts/                   Distribution packaging
```

Build from the repository root with `make build` or `go build .`. Files ending in `_test.go` are Go tests and are excluded from normal builds. Keeping unit tests beside their package allows them to check internal behavior without exposing application internals as public APIs.

## Run tests

Use Go 1.26+, Make, and Python 3.9+ on macOS or Linux. Python is only used by the end-to-end test harness; it has no third-party dependencies. The race detector also requires a working C compiler.

| Command | Runs |
| --- | --- |
| `make test` | All Go package tests, then executable and terminal tests. |
| `make test-unit` | Go package tests, without building the standalone executable. |
| `make test-e2e` | Build `bin/plous`, test CLI behavior, then exercise actual terminal sessions. |
| `make test-race` | Go tests with data-race detection. |
| `make test-coverage` | Go statement coverage; writes `.cache/tests/coverage.out` and `.cache/tests/coverage.html`. |
| `make check` | Formatting check, `go vet`, and the complete test suite. |
| `make dist` | Four distribution archives, checksum verification, and native executable smoke check. |

Go test runs disable result caching by default. To run one Go test:

```sh
make test-unit TEST_FLAGS='-count=1 -run TestMainCommandAliases'
```

To run one terminal journey after building:

```sh
make build
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests/e2e -p 'test_terminal.py' -k test_colon_encodings_and_mode_aliases -v
```

Set `GO` or `PYTHON` when using a different interpreter, for example `make test PYTHON=python3.13`.

## What is covered

- Application tests cover workspace and tool forms, tree navigation, panel focus, commands and aliases, literal input and paste, chat drafts, Activity, resizing, tab placement, the bottom command/chat boxes, and exit behavior.
- Workspace tests cover revision history, exact before/after values, restart, concurrent updates, process locking, interrupted writes, corrupt data, permissions, and path validation.
- Kubeconfig tests cover context discovery, multiple-file precedence and provenance, explicit/default paths, malformed input, and keeping credentials and authentication plugins untouched.
- Executable tests cover version/help/error output, startup without a terminal, a two-workspace Kubernetes setup and restart, main command input through several terminal encodings, Ctrl+C from panels and editors, real process locking, corruption reporting, and terminal restoration.

Tests use temporary directories and synthetic kubeconfig files. They do not modify your `~/.plous` or connect to a cluster. End-to-end tests launch the executable through a pseudo-terminal; they need no open terminal window. Go layout tests check geometry, while terminal tests check input, emitted text, saved state, and process behavior.

Coverage reports measure Go code exercised by Go tests. They do not include the Python-driven executable journeys and are a way to find gaps, not a claim that every possible behavior has been tested.

## Continuous integration

The CI workflow runs `make check` and `make test-race` on Linux and macOS. Linux also runs `make dist`. Use the same commands locally before submitting a change. Run `make dist` whenever packaging changes.
