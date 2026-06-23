# Goal/DoD NextAction S1.4 Replan Idempotency

Status: active

Type: contract-audit

Last Updated: 2026-06-23

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S1.3 Run Command Projection](./goal-dod-nextaction-s1-3-run-command-projection.md)

## Contract Audit

- Scope: replan command scope and idempotency key.
- Observed state: replan commands already enter `TopicWorkorchDCommandWorkspaceRunRequested` with workspace/run/decision scope and are queued through `governanceRunResumeKey`. The key is internal only and not visible in `WorkspaceSnapshot.RunCommand`.
- Expected state: repeated replan for the same workspace/run/decision should coalesce into one pending scheduler work item, and the projection should expose the same deterministic idempotency key used by the scheduler owner.
- Deviation: clients can observe accepted/rejected command metadata after S1.3, but cannot correlate duplicate replan submissions to the scheduler owner's dedupe key.
- Owner: workorchd scheduler/projection. This is core command lifecycle metadata, not template/Soul policy.
- Event path: TUI/Web/REST command -> scheduler command event -> `governanceRunResumeKey` -> pending queue -> `WorkspaceSnapshot.RunCommand.idempotencyKey`.
- Contract gap: command projection lacks idempotency key, and duplicate replan behavior is not covered through the scheduler command path.
- Proposed change: project a deterministic command `idempotencyKey` for scheduler run commands and add a scheduler test proving duplicate scoped replan does not create duplicate background work.
- Boundary check: idempotency key is derived only from command intent and workspace/run/decision scope. No business text, template ID, provider, language, file or tool rule is used.
- Verification: server scheduler duplicate replan test, projection decode test, targeted Go tests, and command/template boundary search.

## Closure Items

1. Done: replan command projection exposes `idempotencyKey`.
2. Done: duplicate replan with the same workspace/run/decision scope coalesces to one pending scheduler work item.
3. Done: malformed scope commands still do not mutate workspace projection.

## Verification

Commands:

```bash
go test ./internal/modules/application/workorchd/service/server -run 'WorkorchDWorkspaceRunCommandReplanUsesScopedIdempotencyKey|WorkorchDSchedulerCommandRejectsWorkspaceScopeMismatch' -count=1
go test ./internal/services/workorch/biz/backends -run 'DecodeSnapshotInfoPreservesRunCommandStatus' -count=1
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|pkg/lsp|mcp-lsp-bridge" internal/modules/application/workorchd/service/server/workorchd_scheduler_commands.go internal/modules/application/workorchd/service/server/http_test.go internal/modules/application/workorchd/biz/appservice/readmodelcore/workspace_snapshot_types.go internal/pkg/workorchport/workspace.go internal/services/workorch/biz/backends/snapshot_test.go docs/70-roadmap/active/goal-dod-nextaction-s1-4-replan-idempotency.md -S
```
