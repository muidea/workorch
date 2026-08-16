# S11.5 Service Restart Recoverable E2E

Status: completed

Owner: e2e

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

Related:

- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [EventHub Driven Workorchd State Loop Design](../superseded/eventhub-driven-workorchd-state-loop-design.md)
- [Service Restart Running Recoverable](./goal-dod-nextaction-s9-6-service-restart-running-recoverable.md)
- [S11.1 Managed E2E Harness](./goal-dod-nextaction-s11-1-managed-e2e-harness.md)

## Contract Audit

- Scope: managed/deployed WorkOrch service startup recovery for persisted workspaces whose workspace/run state was `running` before the process stopped.
- Observed state: service and appservice unit tests already prove startup recovery for file-backed state. `recoverStartupInactiveWorkspace` cancels stale non-governance running work and resets the workspace to active/manual activation, while preserving governance-blocked runs. There was no managed/deployed e2e target that seeds a stale running workspace, restarts the actual binary, and verifies the recovered projection and run status.
- Expected state: after restarting `./bin/workorch` with the same file-backed repository, a stale `running` workspace without a live executor becomes `active` with no `activeRunId`, the old run becomes `cancelled`, and the projection reports `Recovery.Mode=idle` with `Recovery.NextAction=start_workspace_execution`. Governance-blocked runs remain blocked.
- Deviation: S9.6 confirms the behavior at unit and service-test level, but the full roadmap lacked a managed e2e that exercises the actual restart boundary with a persisted data dir and verifies the projection after restart.
- Owner: e2e. This slice validates existing workorchd startup recovery behavior and does not change core runtime, template, Soul, TUI, Web, or LLM context rendering.
- Event path: file-backed workspace/run state -> service startup recovery -> startup projection refresh -> workspace snapshot/status API after restart.
- Ledger impact: none. The script only proves that stale process-local `running` state is normalized to idle/manual activation and that the old run is cancelled. It does not alter coverage, evidence, or completion policy.
- Stage Packet impact: none.
- Contract gap: missing managed/deployed restart recovery coverage in the P11 e2e chain.
- Proposed change: add a standalone managed restart recovery script, expose it through Makefile, include it in `verify-e2e-full`, and document the recovery contract.
- Boundary check: no production core code is changed. The fixture uses neutral workspace/run state and generic-governed file-backed persistence only.
- Verification: shell syntax check, managed service restart recovery execution, focused appservice/service tests, `go test ./... -count=1`, `git diff --check`, and boundary search over the changed files.

## E2E Contract

- The script seeds a file-backed workspace/run pair directly into the managed data dir before the first service start.
- The seeded run is intentionally stale `running` without a live executor.
- The script starts `./bin/workorch`, waits for startup recovery, and verifies the workspace becomes `active` with no `activeRunId`.
- The old run must become `cancelled`.
- The workspace snapshot must report `Recovery.Mode=idle` and `Recovery.NextAction=start_workspace_execution`.
- The script must clean up the temporary binary seed file and managed data dir on exit.

## Acceptance

- `verify-service-restart-recoverable-e2e` passes.
- `verify-e2e-full` includes `verify-service-restart-recoverable-e2e`.
- The script proves restart recovery without changing production runtime code.
- The script is repeatable and leaves no managed service process behind.

## Verification

- `bash -n scripts/verify-service-restart-recoverable-e2e.sh`
- `WORKORCH_MANAGED_VERIFY_TARGET=verify-service-restart-recoverable-e2e WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=1 WORKORCH_MANAGED_SKIP_PREFLIGHT=1 ./scripts/verify-workorch-managed-full.sh`
- `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestRecoverStartupInactiveRunningWorkspacesWithBackendResetsWorkspaceToIdleManualActivation|TestRecoverStartupInactiveRunningWorkspacesPreservesGovernanceBlockedRun|TestRecoverStartupInactiveRunningWorkspacesReconcilesWaitingDecisionRun|TestRecoverStartupInactiveRunningWorkspacesReconcilesApprovalBlockedRun' -count=1`
- `go test ./internal/modules/application/workorchd/service/server -run 'TestServiceRunRecoversStartupInactiveRunningWorkspaceBeforeProjection|TestHTTPServerStartupOnlySchedulesWatchdogTimer' -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|mcp-lsp-bridge" scripts/verify-service-restart-recoverable-e2e.sh Makefile docs/70-roadmap/active/goal-dod-nextaction-s11-5-service-restart-recoverable-e2e.md`
