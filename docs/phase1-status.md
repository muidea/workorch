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

## Acceptance

### Command

```bash
go run ./application/workorchd
```

### Expected output

Two observable snapshots are printed:

- `phase1 runnable slice`
- `phase1 blocked-resume slice`

Each snapshot includes minimal envelope-style observed objects for:

- workspace
- run
- step
- blocking ticket when applicable

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

## Explicit non-goals of current phase1

Not implemented yet:

- real event bus integration
- persistent storage adapter beyond in-memory repository
- approval-driven resume chain in the application demo
- richer block execution graph
- external tool/runtime adapters beyond minimal legacy adapter
- multi-run scheduling / queue semantics
- observability pipeline beyond demo snapshots

## Recommended next batch

1. add approval-blocking runnable slice symmetrical to decision-blocking slice
2. freeze event topic usage against slash-style topic paths only
3. separate acceptance snapshot builder from `main.go` into reusable presenter code
4. add persistence contract tests shared by future repository adapters
5. add application-level smoke test for `workorchd`
