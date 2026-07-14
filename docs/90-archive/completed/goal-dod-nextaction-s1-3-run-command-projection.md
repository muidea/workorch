# Goal/DoD NextAction S1.3 Run Command Projection

Status: completed

Type: contract-audit

Last Updated: 2026-06-25

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S1.1-S1.2 Run Command Contract](./goal-dod-nextaction-s1-1-s1-2-run-command-contract.md)

## Contract Audit

- Scope: `WorkspaceRunCommand` scheduler accepted/rejected projection.
- Observed state: accepted scheduler commands already call `ApplyRunCommandAccepted` and publish `workspace/projection/changed`. Scheduler validation or enqueue failures return errors and write scheduler diagnostics, but the workspace projection does not expose the rejected command metadata.
- Expected state: workorchd owns command state. TUI/Web/REST submit scoped commands and then observe accepted or rejected command status through the workspace projection.
- Deviation: rejected command outcomes are not represented in `WorkspaceSnapshot.RunCommand`, so UI clients cannot distinguish "command submitted and rejected by owner" from a stale projection unless they keep local state or inspect diagnostics separately.
- Owner: workorchd projection/read model. This is core command observability, not workspaceTemplate/Soul policy.
- Event path: TUI/Web/REST command -> `TopicWorkorchDCommandWorkspaceRunRequested` -> scheduler handler -> queue accept/reject -> `WorkspaceSnapshot.RunCommand` -> projection changed event.
- Contract gap: `RunCommandStatus` has accepted fields only and workspaceviewcore has no rejected update path.
- Proposed change: extend `RunCommandStatus` with neutral `state/error/rejectedAt` fields, add `ApplyRunCommandRejected`, and project scope-validated scheduler handler errors before returning the rejected command response. Malformed scope commands do not mutate any workspace projection.
- Boundary check: fields are generic command lifecycle metadata. No template, language, provider, file or workspace-specific logic is introduced.
- Verification: workspaceviewcore store tests for rejected projection, server scheduler test for queue failure projection, snapshot decoding test, targeted Go tests, and command/template boundary search.

## Closure Items

1. Done: accepted command projection remains unchanged and now explicitly reports `state=accepted`.
2. Done: rejected scheduler command outcomes are projected with request scope, intent, error and revision after scope validation.
3. Done: unknown workspace updates are still rejected by the projection store to avoid cross-workspace mutation.

## Verification

Commands:

```bash
go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -run 'RunCommandAccepted|RunCommandRejected' -count=1
go test ./internal/modules/application/workorchd/service/server -run 'WorkorchD.*RunCommand|WorkorchDSchedulerCommandReturnsErrorWhenRunContinuationQueueFails' -count=1
go test ./internal/services/workorch/biz/backends -run 'DecodeSnapshotInfoPreservesRunCommandStatus' -count=1
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|pkg/lsp|mcp-lsp-bridge" internal/modules/application/workorchd/biz/appservice/workspaceviewcore internal/modules/application/workorchd/service/server/workorchd_scheduler_commands.go internal/modules/application/workorchd/service/server/http_test.go internal/modules/application/workorchd/biz/appservice/readmodelcore/workspace_snapshot_types.go internal/pkg/workorchport/workspace.go internal/services/workorch/biz/backends/snapshot_test.go docs/70-roadmap/active/goal-dod-nextaction-s1-3-run-command-projection.md -S
```
