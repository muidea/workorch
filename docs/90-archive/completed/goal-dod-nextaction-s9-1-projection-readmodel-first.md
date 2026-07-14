# S9.1 Projection Readmodel First

Status: completed

Owner: workorchd appservice

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

## Contract Audit

- Scope: `workspaceviewcore.Manager` baseline projection rebuild and single workspace detail projection.
- Observed state: `rebuildAll()` loads the workspace overview list, then schedules `ScheduleRebuildWorkspace()` for every workspace in the list. A list-level refresh can therefore fan out into every workspace detail snapshot read.
- Expected state: workorchd remains the ViewModel owner, but list refresh must be overview/readmodel-first and must not trigger full detail snapshot rebuild for all workspaces. Detail projection should refresh only for the affected workspace event, explicit write path, or an on-demand single workspace detail read.
- Deviation: global projection refresh currently couples overview readiness with all workspace detail rebuilds, which can increase CPU/IO after long multi-workspace runs.
- Owner: workorchd appservice.
- Event path: workorchd startup/global workspace event -> `ScheduleRebuildAll` -> `rebuildAll` -> overview store; workspace event/write/detail request -> single workspace snapshot rebuild -> projection event.
- Contract gap: full projection rebuild lacks a boundary between overview list projection and detail projection, so overview refresh can behave like a global detail scan.
- Proposed change: make `rebuildAll()` replace only the workspace overview list and publish a baseline projection event; add an explicit single-workspace on-demand detail read path for projection detail consumers.
- Boundary check: no template ID, Soul ID, programming language, command output, or software-development-specific policy is introduced. The change is purely projection scope control.
- Verification: workspaceviewcore regression tests for no baseline detail fan-out, on-demand single workspace detail rebuild, event-triggered workspace rebuild, and focused package test.

## Design Notes

- `ListWorkspaces()` remains memory projection only.
- `ReadWorkspaceSnapshot()` remains memory projection only for callers that need a non-mutating read.
- `ReadWorkspaceSnapshotWithContext(ctx, workspaceID)` is the explicit on-demand path. It first reads memory projection and only rebuilds the requested workspace when the overview is known but detail is missing.
- Workspace events and write paths continue to call single workspace rebuilds, so active workspaces refresh by event instead of global scanning.

## Acceptance

- Startup/global baseline rebuild does not schedule one detail rebuild per workspace.
- Reading one workspace detail can load only that workspace's readmodel snapshot.
- Workspace-scoped events still schedule only the affected workspace rebuild.
- Tests prove the above without relying on live workspace data.

## Verification

- `go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -count=1`
- `go test ./internal/modules/application/workorchd/service/server -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/application/workorchd/biz/appservice/workspaceviewcore internal/modules/application/workorchd/service/server/workspace_projection.go`
