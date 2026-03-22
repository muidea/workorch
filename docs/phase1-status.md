# WorkOrch Phase1 Status

## Current repository state

- module: `github.com/muidea/workorch`
- executable entry: `application/workorchd`
- shared internal packages: `internal/pkg/{common,contracts,events,repository}`
- kernel modules:
  - `internal/modules/kernel/workspace`
  - `internal/modules/kernel/run`
  - `internal/modules/kernel/decision`
  - `internal/modules/kernel/orchestrator`
- block modules:
  - `internal/modules/blocks/approval`
  - `internal/modules/blocks/legacy`

## Delivered in phase1

### Slice A: normal completion chain

Implemented and runnable:

1. create workspace
2. create run
3. start run
4. runtime orchestrator delegates to legacy minimal runtime adapter
5. adapter produces one step
6. step completes
7. run reaches `completed`

### Slice B: decision-blocking chain

Implemented and runnable:

1. create workspace
2. create run
3. create decision ticket
4. run transitions to blocked-on-decision state
5. resolve ticket
6. resume run
7. runtime orchestrator runs minimal slice
8. run reaches `completed`

### Slice C: approval-blocking chain

Implemented and runnable:

1. create workspace
2. create run
3. create approval gate
4. run transitions to blocked-on-approval state
5. approve gate
6. resume run
7. runtime orchestrator runs minimal slice
8. run reaches `completed`

### Slice D: approval-rejection terminal path

Implemented and runnable:

1. create workspace
2. create run
3. create approval gate
4. run transitions to blocked-on-approval state
5. reject gate
6. resume run
7. run reaches `failed` without dispatching a runtime step

## Acceptance

### Command

```bash
go run ./application/workorchd
```

### Expected output

Three observable snapshots are printed:

- `phase1 runnable slice`
- `phase1 decision-blocked-resume slice`
- `phase1 approval-blocked-resume slice`

Each snapshot includes minimal envelope-style observed objects for:

- workspace
- run
- step
- blocking ticket or approval when applicable

Observed topic paths are frozen to slash-style demo output:

- `/workspace/snapshot`
- `/run/snapshot`
- `/step/snapshot`
- `/decision/ticket/snapshot`
- `/approval/snapshot`

### Test baseline

```bash
go test ./...
```

Validated coverage currently includes:

- workspace happy path
- run happy path
- orchestrator happy path
- repository memory path
- blocked/resume semantics
- decision ticket resolve conflict guard
- approval resolve conflict guard
- approval reject -> terminal failed path
- application-level smoke test for `workorchd`

## Explicit non-goals of current phase1

Not implemented yet:

- real event bus integration
- persistent storage adapter beyond in-memory repository
- richer block execution graph
- external tool/runtime adapters beyond minimal legacy adapter
- multi-run scheduling / queue semantics
- observability pipeline beyond demo snapshots

## Recommended next batch

1. add persistence contract tests shared by future repository adapters
2. add a file-backed repository adapter as the first non-memory persistence implementation
3. evaluate whether richer step/block graph semantics should start in phase1.5 or phase2
