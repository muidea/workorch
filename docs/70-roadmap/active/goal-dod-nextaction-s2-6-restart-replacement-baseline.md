# Goal/DoD NextAction S2.6 Restart Replacement Baseline

Status: active

Type: contract-audit

Last Updated: 2026-06-23

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S1.5 Restart Replacement Semantics](./goal-dod-nextaction-s1-5-restart-replacement.md)
- [S2.5 Replan Definition And Baseline Inheritance](./goal-dod-nextaction-s2-5-replan-definition-baseline.md)

## Contract Audit

- Scope: workspace-scoped restart replacement activation.
- Observed state: restart is routed as a workspace command and `ActivateWorkspaceExecutionView` calls `createReplacementWorkspaceExecutionRun` when `Command.Restart=true`. The replacement path builds a fresh `workspaceExecutionContext` from selected current Goal/DoD definitions, supersedes the old active run, creates a new run, and starts it. It does not clone the old active run `ExecutionContext`.
- Expected state: restart must create a replacement run from the latest effective definition revision. The replacement run must not reuse old run `coverageWorkItems`, `gapBaseline`, accepted/rejected evidence refs, compressed context, repair state, or other stale run-local convergence data.
- Deviation: implementation already rebuilds context, but S2.6 lacked explicit regression coverage proving stale old run coverage/baseline markers are not copied when restart replaces an active run after definitions changed.
- Owner: workorchd appservice activation plus core run creation/start. This is neutral workspace execution lifecycle behavior, not template/Soul/policy.
- Event path: TUI/Web/REST restart command -> scheduler workspace restart queue -> `ActivateWorkspaceExecutionView` -> `createReplacementWorkspaceExecutionRun` -> `BuildWorkspaceExecutionContext` -> CreateRun -> StartRun -> dispatch gap baseline.
- Contract gap: restart replacement needed an acceptance test binding latest definition revision and absence of stale old baseline/evidence state.
- Proposed change: extend the restart replacement activation regression test to seed stale old run coverage/baseline state, update Goal/DoD, execute `Restart:true`, and assert the replacement run uses latest definition revision without old baseline/evidence markers.
- Boundary check: the test uses generic Goal/DoD and coverage/gap state only. No template ID, Soul ID, programming language, tool command, provider, filename, or business scenario rule is introduced in production code.
- Verification: targeted restart activation test, affected appservice/server packages, boundary search, and `git diff --check`.

## Closure Items

1. Done: restart replacement path is documented as workspace-scoped replacement, not run resume/replan.
2. Done: replacement run is verified to use latest effective Goal/DoD and definition revision.
3. Done: old active run `coverageWorkItems` and `gapBaseline` stale markers are verified not to appear in the replacement run.
4. Done: old active run is superseded and workspace active run points to the replacement.

## Verification

Commands:

```bash
go test ./internal/modules/application/workorchd/biz/appservice -run 'ActivateWorkspaceExecutionWithBackendRestartReplacesCreatedActiveRun|ActivateWorkspaceExecutionWithBackendReplacesActiveRunWhenExplicitDefinitionsChange|ActivateWorkspaceExecutionWithBackendUsesLatestDefinitionRevisionWithoutDuplicateLineage' -count=1
go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server -count=1
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|pkg/lsp|mcp-lsp-bridge" docs/70-roadmap/active/goal-dod-nextaction-s2-6-restart-replacement-baseline.md internal/modules/application/workorchd/biz/appservice/workspace_execution_activation_service_test.go -S
git diff --check
```
