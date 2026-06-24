# S9.6 Service Restart Running Recoverable

Status: completed

Owner: workorchd

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

Related:

- [S9.4 Watchdog External Timeout Scope](./goal-dod-nextaction-s9-4-watchdog-external-timeout-scope.md)
- [S9.5 Watchdog Idempotency Key](./goal-dod-nextaction-s9-5-watchdog-idempotency-key.md)
- [MVVP Workspace ViewModel Architecture](../../30-module-architecture/mvvp-workspace-viewmodel-architecture.md)
- [EventHub Driven Workorchd State Loop Design](../../30-module-architecture/eventhub-driven-workorchd-state-loop-design.md)

## Contract Audit

- Scope: workorchd service startup recovery for persisted workspaces whose workspace/run state was `running` or `validating` before the process stopped.
- Observed state: process-local execution cannot survive a service restart. If persisted `running` state is left unchanged, TUI/Web can render a false active run even though no runtime worker owns it.
- Expected state: during workorchd startup, old process-local `running` or `validating` work without a live execution context is converted into an idle/manual-activation projection before the projection loop starts. workorchd must not automatically resume the old run only because the service restarted.
- Deviation: without this startup reconciliation, the owner state and projection can claim active execution for work that has no in-process executor.
- Owner: workorchd appservice startup recovery and workorchd ViewModel projection. TUI/Web remain read-only projection consumers.
- Event path: service `Run` -> `recoverStartupInactiveRunningWorkspaces` -> `RecoverStartupInactiveRunningWorkspaceViews` -> run/workspace owner state update -> projection rebuild/readmodel -> scheduler diagnostic.
- Contract gap: startup recovery must be explicit, idempotent, and non-scheduling. It should clear false active execution and expose manual next action instead of compensating by watchdog or hidden continuation.
- Proposed change: keep startup reconciliation as a pre-projection step; cancel stale process-local `running`/`validating` runs, clear `activeRunId`, set workspace state to active/idle, record a manual retry diagnostic, and assert that no run/replan continuation background task is scheduled by this path.
- Boundary check: the rule uses only neutral lifecycle states and process ownership. It does not inspect template ID, Soul ID, workspace title, programming language, command text, provider, tool name, or output text.
- Verification: appservice startup recovery test, service startup projection test, no-auto-continuation assertion, affected package tests, boundary search, and `git diff --check`.

## Design Notes

- Service restart is not a runtime event replay mechanism. It is a process lifecycle boundary.
- A run that was `running` or `validating` before restart has no live executor after restart. It must not remain projected as active running.
- The old run is terminalized as `cancelled` with a startup-inactive diagnostic reason. This preserves auditability while preventing false active status.
- The workspace is reset to `active` with no `activeRunId`. Projection therefore emits `Recovery.Mode=idle` and `Recovery.NextAction=start_workspace_execution`.
- Governance-blocked runs are not cancelled. If a unique open decision ticket or pending approval owns the blocker, startup reconciliation projects the workspace as waiting for that governance action.
- Startup reconciliation records scheduler diagnostics with `manual_retry_required` and recommended actions `run_resume` and `workspace_restart`, but does not enqueue either action.
- Watchdog remains limited to normal-runtime external timeout compensation. It does not recover service-restart process-local work.

## Acceptance

- Restarting workorchd does not leave old process-local `running` or `validating` workspace state visible as active running.
- Startup reconciliation runs before workspace projection baseline loading.
- Reconciled workspace projection is idle/manual activation, not running.
- No run continuation or replan continuation is scheduled by startup reconciliation.
- Re-running startup reconciliation is idempotent.
- Decision/approval blocked workspaces retain the governance blocker state instead of being cancelled.

## Verification

- `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestRecoverStartupInactiveRunningWorkspaces' -count=1`
- `go test ./internal/modules/application/workorchd/service/server -run 'TestServiceRunRecoversStartupInactiveRunningWorkspaceBeforeProjection|TestHTTPServerStartupOnlySchedulesWatchdogTimer' -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server -count=1`
- `go test ./... -count=1`
- `git diff --check`
- Boundary search for business/template terms: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh" internal/modules/application/workorchd/biz/appservice/runports internal/modules/application/workorchd/service/server -g '!**/consoleassets/**'`
- Boundary review note: the generic term `server` is package and HTTP/MCP terminology in `service/server`, so it is reviewed through the touched diff instead of used as a broad recursive grep term for this slice.
