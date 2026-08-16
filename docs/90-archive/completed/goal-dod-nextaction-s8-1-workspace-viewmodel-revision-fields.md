# S8.1 Workspace ViewModel Revision Fields

Status: completed

Type: roadmap-slice

Owner: workorchd appservice

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [MVVP Workspace ViewModel Architecture](../superseded/mvvp-workspace-viewmodel-architecture.md)
- [S4.6 Coverage Summary Projection](./goal-dod-nextaction-s4-6-coverage-summary-projection.md)
- [S7.6 Contract Violation Recovery Projection](./goal-dod-nextaction-s7-6-contract-violation-recovery-projection.md)

## Contract Audit

- Scope: workspace projection read model, port/client DTOs, and backend snapshot decoding for ViewModel revision metadata.
- Observed state: workorchd projection store already tracks global, workspace, source event and projected source revisions. Workspace list responses expose `ProjectionInfo`, and workspace snapshots expose `ProjectionInfo` separately from `WorkspaceSnapshotInfo`. However, the snapshot payload itself does not carry an explicit ViewModel metadata object declaring that list, active workspace, run state, prompt mode, command status, coverage, focus and diagnostics are from the same projection revision.
- Expected state: workorchd should be the owner of a single `WorkspaceViewModel` metadata object. TUI/Web should be able to render list/main/status/prompt from fields tagged with the same revision without inventing another local state machine.
- Deviation: downstream clients currently need to infer revision consistency from `ProjectionInfo` plus scattered snapshot fields and local cache, which makes S8.2/S8.3 harder to enforce.
- Owner: workorchd appservice. Port/client/backend only transport the owner projection.
- Event path: EventHub source event -> workspaceviewcore store rebuild -> projection status -> `WorkspaceViewModel` metadata -> REST/EventHub/remote backend -> TUI/Web display.
- Ledger impact: none. This is read model metadata only.
- Stage Packet impact: none.
- Contract gap: no explicit ViewModel metadata describes the revision shared by list/run/prompt/commands/coverage/focus/diagnostic sections.
- Proposed change: add a neutral `WorkspaceViewModel` metadata model to snapshots and port/client DTOs, populate it from projection status, and test that required sections share the same revision.
- Boundary check: the change is business-neutral and only labels generic projection sections. It does not inspect template ID, Soul ID, workspace title, provider, command output, programming language or business text.
- Verification: workspace projection store/readmodel tests, backend decode tests, focused workorchd projection tests, `git diff --check`, and diff-only boundary search.

## Slice Scope

In scope:

- Define minimal ViewModel metadata fields.
- Include section revision markers for list, active workspace, run state, prompt mode, commands, coverage, current focus and latest diagnostic.
- Populate ViewModel metadata from existing projection status and snapshot content.
- Transport ViewModel metadata through workorchport/workorchclient/backends.

Out of scope:

- TUI prompt rendering changes.
- Workspace switching behavior changes.
- SSE reconnect behavior.
- Web page layout changes.

## Acceptance

- Workspace snapshot contains a `viewModel` object with revision/source revision fields.
- `viewModel.sections` includes list, activeWorkspace, runState, promptMode, commands, coverage, currentFocus and latestDiagnostic.
- Every populated section uses the same workspace projection revision.
- Remote/backend decoding preserves the ViewModel metadata.
- The change does not add business/template/language/provider special casing.

## Verification

- Passed: `go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -run 'TestStoreReplaceAllListSnapshotAndDelete' -count=1`
- Passed: `go test ./internal/services/workorch/biz/backends -run 'TestDecodeSnapshotInfoPreservesWorkspaceViewModel' -count=1`
- Passed: `go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -count=1`
- Passed: `go test ./internal/services/workorch/biz/backends -count=1`
- Passed: `go test ./internal/pkg/workorchclient ./internal/pkg/workorchport -count=1`
- Passed: `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'WorkspaceViewModel|WorkspaceSnapshot' -count=1`
- Passed: `git diff --check`
- Passed: diff-only boundary search found no newly added business/template/language/provider special casing in touched Go files.
