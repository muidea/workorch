# Runtime Contract Conformance Audit

Status: code-closed

Type: audit

Last Updated: 2026-06-22

## Purpose

本文承接运行期问题的合同符合性核查，只记录核查矩阵、偏差、修复要求和回归要求，不引入新的业务规则。

## Audit Principles

- 每个偏差先对照 canonical runtime contract，再定位代码落点与测试覆盖
- 核心框架保持功能中立，只处理 stage、contract、evidence、target、repair、validation、governance 和 projection
- 没有测试覆盖的修复不视为完成

## Current Focus Paths

- initial planning contract
- evidence acquisition follow-up
- repair-of-repair
- replan successor 首步 hydration
- decision resolved 恢复
- validation / acceptance repair
- context gate fail-fast
- workspace/run/ticket projection

## Confirmed Deviations

### Evidence Follow-up Context Gap

- evidence acquisition follow-up 曾缺少 `repairActionPackage.currentFailure`
- `currentExecutionProgress` 没有从 source package / replan continuation 恢复
- 结果是 context gate 误判 `runtime_repair_current_failure_missing`
- 收口状态（2026-06-22）：已修复。
  - `runtime_graph.go` 的 concrete evidence acquisition follow-up 恢复逻辑会从当前 input、source step input、source `repairActionPackage`、`currentProgressDigest`、coverage work item context 中重建 `currentFailure` 与 `currentExecutionProgress`，并同步写入 top-level input 与 `repairActionPackage`。
  - 回归覆盖：
    - `TestEvidenceAcquisitionFollowUpRestoresCurrentFailureFromRepairOfScope`
    - `TestEvidenceAcquisitionFollowUpRestoresCurrentProgressDigest`
    - `TestEvidenceAcquisitionFollowUpPreservesLastFailureDiagnostics`

### Context Gate Resolved Replay

- `runtime_context_required` resolved 后，恢复路径曾重放旧的 failed context-gate step
- 没有创建新的 context retry step
- 同一 blocker 会被重复创建
- 收口状态（2026-06-22）：已修复。
  - `runtime_dispatch_specs.go` 对 context-gate decision resume 走 `contextIntegritySpecsAfterResolvedDecision`，基于失败 step 生成新的 `context-gate-retry` step，并注入 `contextGateRecovery` 标记。
  - retry input 会通过 `hydrateContextIntegrityRetryRepairPackage` 恢复原始 actionable failure 与 current progress，避免再次把 context gate 包装错误当作业务失败。
  - 回归覆盖：
    - `TestRuntimeOrchestrator_ResumeResolvedContextGateCreatesFreshRetryStep`

### Workspace Blocker Projection Gap

- run 已持久化 blocker，但 workspace detail/list 投影没有暴露 `blockingTicketId` / `blockingApprovalId`
- 收口状态（2026-06-22）：已修复。
  - `readmodelprojection` 将 active run blocker 投影到 `TaskProgress.Recovery`、`TaskProgress.Blockers`、workspace detail 与 run snapshot。
  - `readmodelports` 的 workspace list 以最新 active run projection 为准暴露 `blockingTicketId` / `blockingApprovalId`。
  - 回归覆盖：
    - `TestRunSnapshotFromModelsProjectsBlockingDecisionStepAndCoverage`
    - `TestProjectWorkspaceSnapshotDetailPreservesActiveBlockingGovernance`
    - `TestWorkspaceTaskProgressOnlyUsesActiveRunBlockers`
    - `TestWorkspaceTaskProgressIgnoresHistoricalBlockersWithoutActiveRun`
    - `TestListProjectedWorkspacesUsesLatestRunProjection`

## Required Regression Coverage

- evidence acquisition follow-up 必须继承 `currentFailure`：已覆盖。
- evidence acquisition follow-up 必须继承等价 current progress digest：已覆盖。
- resolved `runtime_context_required` 必须走新的 context retry step：已覆盖。
- workspace detail/list 必须暴露 active run blocker projection：已覆盖。
- 所有测试保持 domain-neutral

## Verification Record

2026-06-22 已执行：

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'TestEvidenceAcquisitionFollowUp(RestoresCurrentFailureFromRepairOfScope|RestoresCurrentProgressDigest|PreservesLastFailureDiagnostics)|TestRuntimeOrchestrator_ResumeResolvedContextGateCreatesFreshRetryStep' -count=1 -timeout=180s
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection ./internal/modules/application/workorchd/biz/appservice/readmodelports -run 'TestRunSnapshotFromModelsProjectsBlockingDecisionStepAndCoverage|TestProjectWorkspaceSnapshotDetailPreservesActiveBlockingGovernance|TestWorkspaceTaskProgressOnlyUsesActiveRunBlockers|TestWorkspaceTaskProgressIgnoresHistoricalBlockersWithoutActiveRun|TestListProjectedWorkspacesUsesLatestRunProjection' -count=1 -timeout=180s
```

## Tracking State

当前核查记录表明，上述偏差对应的设计约束、实现路径与回归测试均已闭环；后续审计重点转向：

- follow-up / repair / replan 继承链的一致性
- context gate fail-fast 与 retry 之间的分流正确性
- workspace/run projection 对 blocker / coverage / result 的一致投影

## Source Documents

- `../../00-core/runtime-stage-contract.md`
