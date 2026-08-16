# S8.3 TUI List And Main View Same Revision

Status: completed

Type: roadmap-slice

Owner: TUI

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S8.1 Workspace ViewModel Revision Fields](./goal-dod-nextaction-s8-1-workspace-viewmodel-revision-fields.md)
- [S8.2 TUI Prompt Mode From Projection](./goal-dod-nextaction-s8-2-tui-prompt-mode-projection.md)
- [MVVP Workspace ViewModel Architecture](../superseded/mvvp-workspace-viewmodel-architecture.md)

## Contract Audit

- Scope: TUI workspace list, active workspace overview, and active workspace main view context application.
- Observed state: TUI already tracks workspace projection revisions for list/overview and ignores stale request sequence responses. However, a `WorkspaceContextMsg` can still carry an older detail snapshot than the latest workspace overview revision. `mergeWorkspaceOverview` protects the overview itself, but `applyWorkspaceContext` may still apply older run/task/status/detail fields to the main view.
- Expected state: workspace list and active main view should be rendered from a same-or-newer projection revision. If detail projection lags behind the list/overview projection, TUI should keep the current main view and request another context refresh rather than mixing new list state with old detail state.
- Deviation: request sequence protects transport ordering but does not prove that the detail snapshot revision is at least the active overview revision.
- Owner: TUI consumes projection. workorchd remains the ViewModel owner.
- Event path: workorchd projection -> workspace list overview revision -> TUI runtime state -> workspace detail snapshot revision -> main view application guard.
- Ledger impact: none.
- Stage Packet impact: none.
- Contract gap: no guard compares active detail revision against active overview/list revision before applying main view fields.
- Proposed change: add a neutral TUI helper that derives context revision from `WorkspaceViewModel`, `ProjectionInfo`, or workspace projection; ignore and refresh stale detail contexts whose revision is lower than the current active overview revision.
- Boundary check: the change only compares generic projection revisions. It does not inspect template ID, Soul ID, provider, command output, programming language, goal text or business content.
- Verification: focused TUI regression test for stale detail being rejected, full TUI package test, `git diff --check`, and diff-only boundary search.

## Slice Scope

In scope:

- TUI revision comparison before applying active workspace detail context.
- Preserve existing request sequence protection.
- Keep current main view stable when detail is stale.
- Request a fresh context load after stale detail is detected.

Out of scope:

- Web detail/list same-revision guard. This should be a separate Web-focused slice because it touches React state sequencing.
- Command pending/accepted visual state.
- SSE reconnect behavior.
- Decision/approval view regression.

## Acceptance

- A detail context with revision lower than the active workspace overview revision does not update main view fields.
- TUI schedules a fresh context refresh after rejecting stale detail.
- Same-revision or newer detail context still updates the main view.
- Existing inactive workspace protection remains intact.
- The change does not add business/template/language/provider special casing.

## Verification

- Passed: `go test ./internal/services/workorch/service/tui -run 'SameRevision|StaleWorkspaceContext|WorkspaceSwitch|RejectsWorkspaceContextOlderThanOverviewRevision' -count=1`
- Passed: `go test ./internal/services/workorch/service/tui -count=1`
- Passed: `git diff --check`
- Passed: diff-only boundary search found no newly added business/template/language/provider special casing in touched Go files.
