# workorch

workorch minimal skeleton, rectified to the go-multi-module-dev module-oriented layout.

## Structure

- `application/workorchd`: executable entry
- `application/workorchd/bootstrap`: backend/runtime assembly and repository backend selection
- `application/workorchd/appservice`: application-level workorch orchestration, env-backed execution entry, and workflow helpers
- `application/workorchd/demo`: thin compatibility shim over the workorch appservice entry
- `internal/modules/kernel/{workspace,run,decision,orchestrator}`: workorch kernel modules
- `internal/modules/blocks/{approval,legacy}`: reusable/block modules
- `internal/pkg/{common,events,repository}`: shared internal primitives
- `docs/`: repo docs
- `.github/workflows/ci.yml`: GitHub Actions CI workflow
- `Makefile`: local and CI entry targets

Each module is now normalized around the go-multi-module-dev minimum shape:

- `module.go`: module-level constructor/export entry
- `biz/`: business logic
- `service/`: module-facing contract layer or adapter-facing surface
- `pkg/common`: module-local constants
- `pkg/models`: module-local models when the module owns domain data

The application layer is also split by responsibility:

- `main.go`: process entry and output loop that delegates workorch execution to `appservice`
- `bootstrap/`: backend assembly, repository wiring, and runtime construction
- `appservice/`: application-level business orchestration, workorch workflow assembly, env-backed execution entry, and workorch runtime adapter
- `observed/`: workorch observed-envelope adapter for acceptance snapshots
- `demo/`: compatibility export entry that forwards to `appservice`
- `presenter/`: observable output rendering

Inside `demo/`, responsibilities are now intentionally thin:

- `demo.go`: compatibility shim that delegates to `appservice.RunAllWorkorchFromEnv(...)` / `appservice.RunAllWorkorch(...)`

Current boundary:

- `bootstrap.Runtime`: assembled capability export surface via `Exports()` plus lifecycle shutdown
- `bootstrap.Runtime`: now stores a single assembled `Exports` bundle plus module lifecycle teardown, rather than scattering repo/service fields internally
- `bootstrap.Exports`: now groups assembled capabilities into `Repositories`, `Services`, and `Infrastructure`
- `appservice.Runtime`: workorch application runtime contract
- `appservice/runtime_adapter.go`: bridges grouped `bootstrap.Exports` into the workorch runtime contract through grouped runtime dependencies
- `appservice`: now keeps workspace write flows behind `workspace` module service instead of direct repository updates
- `appservice`: now also uses explicit read-side vs action-side runtime faces inside workorch orchestration
- `appservice`: workorch action-side is now further grouped by workspace/run/decision/approval business actions
- `appservice`: workorch read-side is now further grouped by guard checks vs snapshot assembly
- `appservice/runtime_adapter.go`: now also groups its own dependencies into read-side and action-side capabilities instead of flat repo/service fields
- `appservice/runtime_dependencies.go`: now centralizes `bootstrap.Exports` to grouped runtime dependency assembly in one place
- `appservice/runtime_assembly.go`: now centralizes managed runtime adapter plus collector assembly in one place
- `appservice/runtime_session.go`: now centralizes repeated bootstrap-plus-managed-runtime lifecycle handling in one place
- `appservice/entry.go`: now centralizes repeated workorch execution through `WithSession` and `WithBackend` helpers, with `demo` forwarding to the same path
- `appservice/demo` tests: now centralize repeated session setup and backend run patterns through shared test helpers
- `observed/`: owns workorch acceptance `observed` envelope assembly
- `bootstrap.Exports.Infrastructure`: now carries the application `EventHub`
- `internal/pkg/events/workorch_payload.go`: now centralizes reusable workorch event payload keys and builders
- `internal/pkg/events/workorch_event.go`: now centralizes typed workorch event envelope decoding, normalization, topic listing, and observer subscription helpers
- `internal/pkg/events/workorch_version.go`: now centralizes strict workorch event/snapshot version constants and topic-scoped contract lookup
- `observed/workorch_contract.go`: now decodes and validates published workorch payloads before turning them into acceptance snapshots
- `observed/snapshot_contract.go`: now centralizes typed workorch snapshot items while `presenter` handles the final JSON-compatible rendering shape
- `workspace / decision / approval`: now publish create/resolve events into the shared `EventHub`
- `orchestrator`: now also publishes `workspace` runtime state changes when `activeRunId` is bound or cleared
- `observed/collector.go`: now subscribes to workorch event topics and collects observed input without relying on `MemoryHub.Published()`
- `appservice/runtime_managed.go`: now owns explicit collector lifecycle via `ManagedRuntime`
- `appservice/runtime_adapter.go`: now acts as a pure business runtime adapter without collector ownership
- `observed/`: now prefers subscribed/published `workspace / run / step / decision / approval` events when available, including workspace runtime `activeRunId` changes, falls back to state-derived envelopes only when an event source is still absent, and centralizes the workorch event-to-snapshot mapper contract in one place
- `presenter/`: owns snapshot view mapping and output only

## Workorch semantics retained

- kernel: `workspace`, `run`, `decision`, `orchestrator`
- blocks: `approval`, `legacy`
- shared common/events/repository primitives moved under `internal/pkg`

## Development workflow

This repository is still a single `go.mod` codebase. Here, `go-multi-module-dev` means module-oriented directory organization and dependency boundaries inside one repository, not a split into multiple Go modules yet.

Common entrypoints:

```bash
make fmt
make fmt-check
make vet
make test
make test-race
make build
make run
make run-file
make ci
```

Notes:

- `make build` writes the binary to `./bin/workorchd`
- `make run-file` runs the demo with the file-backed repository backend
- file-backed demo execution is now repeatable against the same data directory for the shipped workorch demo IDs
- `make ci` is the local equivalent of the GitHub Actions baseline

## Workorch acceptance demo

Run:

```bash
make run
# or
go run ./application/workorchd
```

Optional file-backed demo run:

```bash
make run-file
# or
WORKORCH_REPOSITORY_BACKEND=file WORKORCH_DATA_DIR=.workorch-data go run ./application/workorchd
```

Expected observable output:

- `workorch runnable slice`: shows workspace/run/step reaching `completed`
- `workorch decision-blocked-resume slice`: shows a decision ticket being created, resolved, then the run resuming to `completed`
- `workorch approval-blocked-resume slice`: shows an approval gate being created, approved, then the run resuming to `completed`
- `workorch approval-rejected-terminal slice`: shows an approval gate being rejected, then the run reaching `failed` without dispatching a runtime step
- `observed`: emits minimal envelope-style JSON snapshots for workspace/run/step/blocking objects, only as acceptance output, not a full event bus

## Development targets

```bash
make fmt
make vet
make test
make build
make ci
```

## Docs

- `docs/structure.md`: layout normalization mapping
- `docs/workorch-status.md`: delivered scope, acceptance baseline, and next batch boundary
