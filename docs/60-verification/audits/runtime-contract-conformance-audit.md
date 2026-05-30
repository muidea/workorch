# Runtime Contract Conformance Audit

Status: active

Type: audit

Last Updated: 2026-05-29

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

### Context Gate Resolved Replay

- `runtime_context_required` resolved 后，恢复路径曾重放旧的 failed context-gate step
- 没有创建新的 context retry step
- 同一 blocker 会被重复创建

### Workspace Blocker Projection Gap

- run 已持久化 blocker，但 workspace detail/list 投影没有暴露 `blockingTicketId` / `blockingApprovalId`

## Required Regression Coverage

- evidence acquisition follow-up 必须继承 `currentFailure`
- evidence acquisition follow-up 必须继承等价 current progress digest
- resolved `runtime_context_required` 必须走新的 context retry step
- workspace detail/list 必须暴露 active run blocker projection
- 所有测试保持 domain-neutral

## Tracking State

当前核查记录表明，上述偏差对应的设计约束已存在，问题主要属于实现与恢复路径偏差；后续审计重点转向：

- follow-up / repair / replan 继承链的一致性
- context gate fail-fast 与 retry 之间的分流正确性
- workspace/run projection 对 blocker / coverage / result 的一致投影

## Source Documents

- `../../00-core/runtime-stage-contract.md`
