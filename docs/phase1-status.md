# WorkOrch Phase1 Status

## Current repository state

- module: `github.com/muidea/workorch`
- executable entry: `application/workorchd`
- shared internal runtime packages: `internal/pkg/{common,events,repository}`
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

Optional file-backed demo run:

```bash
WORKORCH_REPOSITORY_BACKEND=file WORKORCH_DATA_DIR=.workorch-data go run ./application/workorchd
```

### Expected output

Four observable slices are printed:

- `phase1 runnable slice`
- `phase1 decision-blocked-resume slice`
- `phase1 approval-blocked-resume slice`
- `phase1 approval-rejected-terminal slice`

Each slice includes minimal envelope-style observed objects for:

- workspace
- run
- step when a runtime step is actually dispatched
- blocking ticket or approval when applicable

Observed topic paths are frozen to slash-style demo output:

- `/workspace/snapshot`
- `/run/snapshot`
- `/step/snapshot`
- `/decision/ticket/snapshot`
- `/approval/snapshot`

### File-backed acceptance verification

Verified with:

```bash
WORKORCH_REPOSITORY_BACKEND=file WORKORCH_DATA_DIR=.workorch-data go run ./application/workorchd
```

Observed persisted files:

- `.workorch-data/workspace.json`
- `.workorch-data/run.json`
- `.workorch-data/step.json`
- `.workorch-data/ticket.json`
- `.workorch-data/approval.json`
- `.workorch-data/resolution.json`

Verified terminal states:

- runnable / decision-blocked-resume / approval-blocked-resume runs persisted as `completed`
- approval-rejected-terminal run persisted as `failed`
- repeated file-backed demo execution against the same data directory is now supported for the shipped phase1 demo IDs

### Test baseline

```bash
go test ./...
```

Validated coverage currently includes:

- workspace happy path
- run happy path
- orchestrator happy path
- repository memory path
- repository file-backed path
- blocked/resume semantics
- decision ticket resolve conflict guard
- approval resolve conflict guard
- approval reject -> terminal failed path
- application-level smoke test for `workorchd`
- repeatable file-backed demo execution against the same data directory

## Explicit non-goals of current phase1

Not implemented yet:

- real event bus integration
- richer block execution graph
- external tool/runtime adapters beyond minimal legacy adapter
- multi-run scheduling / queue semantics
- observability pipeline beyond demo snapshots

Implemented in minimal form, but not yet expanded to phase2-grade persistence semantics:

- file-backed repository adapter as the first non-memory persistence path

## Recommended next batch

1. add persistence contract tests shared by repository adapters
2. define the minimal eventHub / real event bus integration path
3. evaluate whether richer step/block graph semantics should start in phase1.5 or phase2
