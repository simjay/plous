# Executable tests

`e2e/` exercises the built Plous executable through its CLI and a pseudo-terminal. `fixtures/` contains synthetic kubeconfig data. All state is created in temporary directories and cleaned up after each test.

Run `make test` from the repository root for the complete suite, or `make test-e2e` for these tests only. Python 3.9+ is required; no Python packages need to be installed.

Go unit tests live beside the packages they test under `internal/`. See the [development and testing guide](../docs/development.md) for commands, coverage, and the repository layout.
