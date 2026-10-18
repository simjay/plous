# PlousBench

[Project](../README.md) · [Development](../docs/contributing/README.md)

PlousBench checks Plous tools directly and measures their cost on fixed synthetic workloads. It runs the production guarded tool session through direct Go calls and the native agents' MCP bridge. It does not run an agent, model, installed CLI, or paid provider.

## Run

```sh
make test-plousbench
make bench-plousbench
```

The first command checks every scenario through both transports. The second repeats the same scenarios five times per transport, with a fresh temporary profile for each attempt. Go benchmark calibration samples are excluded from the retained report. Change the repetition count with `PLOUSBENCH_FLAGS='-benchtime=10x -count=1'`.

To retain results, choose a new file in an existing directory:

```sh
PLOUSBENCH_REPORT=/tmp/plous-tools.json make bench-plousbench
make build-plousbench
bin/plousbench report /tmp/plous-tools.json
```

Existing report files are never overwritten. Use one Go benchmark invocation per output file. Reports include the workload, source and fixture hashes, effective schema hashes, platform, call inputs/results, failures and measurements. The report command checks the recorded expectations again; it does not authenticate the provenance of a supplied report.

## Checks

- Expected resource values, readiness, stale generations, controller status, Helm revision values and source facts.
- Complete and partial inventories, denied access, typed history counts and saved pagination.
- Captured connection and history scope, invalid arguments, cancellation, closure and fatal failure revocation.
- Persistence before delivery and unchanged Sources after reopening storage.
- Equivalent direct and MCP schemas and results across the 17-tool catalog.
- Capability boundaries and retired-name rejection before acquisition.
- Workload log ownership, unavailable sources and shared byte limits; explicit Helm revision pairs.
- UI view preparation and actual before/after Helm values and manifest content in both revision directions, plus scope rejection, missing/replaced targets, source boundaries and blocked display requests.

History cases include original decisions, corrections and background exchanges. All expected facts are explicit in [tools.json](fixtures/tools.json). `checks` verifies tool results; `display_checks` verifies prepared display content (`before`, `after`, or `text`). Reports retain display content separately and recheck those expectations when loaded. Fixture API responses and source files are separate input files; the fake server receives no expected answers. These development cases do not claim exhaustive tool coverage or agent answer quality.

## Measurements

Each sample reports elapsed tool-call time, tool-result bytes and external API reads, including discovery. Byte counts exclude the MCP/HTTP envelope. Time includes persistence and, for MCP, transport. Display-content capture and assessment are outside the measured call and response-byte count. Fixture setup, history seeding, restart verification and result assessment are excluded. A workload with several calls reports their combined time. Expected denial/error cases are successful checks when the required behavior is observed. Unexpected failures and skipped calls remain visible; failed attempts remain in report timing samples.

Compare runs with the same fixture and schema hashes, transport, repetition count and machine conditions. Source hashes identify implementation changes. Read correctness results before comparing speed. The Go benchmark output uses `ns/op`, `response-B/op` and `api-reads/op`; one operation is one complete scenario, not one tool call.

## Files and call chain

| Files | Purpose |
| --- | --- |
| `fixtures/tools.json`, `scenario.go` | Tool calls, expected JSON facts, validation |
| Other `fixtures/*.json`, `cluster.go` | Synthetic API responses and source files |
| `runner.go`, `assessment.go` | Execute calls, bind returned references, measure and check results |
| `integration/fixture_test.go` | Temporary workspace/history, production session and durability checks |
| `integration/mcp_test.go` | Scripted client of the production MCP bridge |
| `integration/tools_test.go` | Correctness suite, repetitions and transport comparisons |
| `report.go`, `cli.go`, `cmd/plousbench` | Retain and inspect tool reports |

```text
scenario → temporary profile → guarded Tools.Call → integration → fixture API/files
                            ↳ MCP → same Tools.Call
         → check returned facts and saved evidence → report
```

UI cases run the production navigation loader against fixture APIs/files and use a deterministic display endpoint. App tests exercise the actual viewers, focus, drafts and late-result guards. UI preparation and execution-receipt persistence are included in measured calls; terminal rendering is not measured.

The toolkit stays out of the application binary. Tests use synthetic kubeconfig, temporary profiles and loopback APIs. No owner profile, kubeconfig, cluster or credentials are used.

The old agent runner, scripted models, provider pilot, answer grading, token budgets, review packets, fixture-inspector executable and report protocol are removed. Existing saved reports remain historical artifacts and are not migrated. Skill evaluation is deferred until Plous has actual skill packages.
