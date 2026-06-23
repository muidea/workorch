# Goal/DoD NextAction S1.5 Restart Replacement Semantics

Status: active

Type: contract-audit

Last Updated: 2026-06-23

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S1.3 Run Command Projection](./goal-dod-nextaction-s1-3-run-command-projection.md)
- [S1.4 Replan Idempotency](./goal-dod-nextaction-s1-4-replan-idempotency.md)

## Contract Audit

- Scope: restart replacement run command semantics.
- Observed state: restart enters `TopicWorkorchDCommandWorkspaceRunRequested` from REST/TUI/EventHub with workspace scope. The scheduler routes it to the workspace execution queue with `Command.Restart=true`, and appservice activation creates a replacement run while superseding the old non-terminal active run.
- Expected state: restart is a workspace-scoped replacement command. It must not be treated as run-scoped resume, governance replan, or recover. Projection must expose an accepted restart command and a deterministic workspace idempotency key.
- Deviation: the main implementation already separates restart from resume/replan, but S1.5 did not have an independent audit record or regression assertions proving queue separation and projection idempotency.
- Owner: workorchd scheduler/projection plus appservice activation. This is core command lifecycle behavior, not workspaceTemplate/Soul/policy.
- Event path: TUI/Web/REST command -> `TopicWorkorchDCommandWorkspaceRunRequested` -> scheduler accepted projection -> `workspace_restart/<workspace>` queue -> `ActivateWorkspaceExecutionCommand{Restart:true}` -> replacement run activation.
- Contract gap: restart separation was implicit in implementation and partially covered by tests, but the roadmap lacked S1.5-specific evidence that restart does not enter run continuation or governance replan queues.
- Proposed change: document S1.5 and extend scheduler/projection tests to assert restart `state=accepted`, `idempotencyKey=restart/<workspace>`, workspace restart queue usage, and empty run/governance queues.
- Boundary check: restart idempotency is derived only from command intent and workspace scope. No template, language, file, provider, command output, or business scenario rule is introduced.
- Verification: targeted workorchd server tests, appservice replacement test, boundary search, and `git diff --check`.

## Closure Items

1. Done: restart command projection exposes accepted restart metadata and `restart/<workspace>` idempotency key.
2. Done: restart scheduler path queues `workspace_restart/<workspace>` with `Restart=true`.
3. Done: restart scheduler path does not create run continuation or governance replan pending entries.
4. Done: duplicate restart commands for the same workspace coalesce to one pending scheduler work item while keeping the latest accepted projection metadata.
5. Done: appservice activation has regression coverage proving restart supersedes the old active run and starts a replacement run.

## Verification

Commands:

```bash
go test ./internal/modules/application/workorchd/service/server -run 'WorkorchDWorkspaceRestartHTTPProjectsRestartIntent|WorkorchDSchedulerWorkspaceRunCommandEnqueuesWorkspaceRestartByWorkspaceScope|WorkorchDWorkspaceRunCommandRestartEnqueuesWorkspaceRestart' -count=1
go test ./internal/modules/application/workorchd/biz/appservice -run 'ActivateWorkspaceExecutionWithBackendRestartReplacesCreatedActiveRun' -count=1
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|pkg/lsp|mcp-lsp-bridge" internal/modules/application/workorchd/service/server/http_test.go docs/70-roadmap/active/goal-dod-nextaction-s1-5-restart-replacement.md -S
git diff --check
```
