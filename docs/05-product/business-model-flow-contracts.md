# WorkOrch Business Model And Flow Contracts

Status: active

Type: business-model-flow-contracts

Last Verified: 2026-08-16

## Scope

本文给出产品对象、owner 和跨组件流程的简明映射。字段以 [domain model](../10-domain-model/overview.md) 为准，
状态转换以 [Runtime contract](../00-core/runtime-stage-contract.md) 为准。

## Business Objects

| Object | Sole owner | Role |
| --- | --- | --- |
| Workspace | `blocks/workspace` | 顶层定义容器、目录、资源绑定、active Run 与终态 slot acknowledgement |
| Goal / DoD | `blocks/workspace` | 当前正式目标、验收定义及 revision |
| Run | `blocks/run` | SM-1、bootstrap binding、aggregate persistence、resume/restart transaction |
| Runtime authority/focus/candidate/evidence | `kernel/orchestrator` | SM-2/3/4 canonical convergence |
| Result | `blocks/result` | 已裁决终态结果、DoD assessment 和 evidence/artifact refs |
| Decision / Approval | 对应 block | 治理 ticket/request lifecycle 与 resolution |
| Memory / Skill / Soul / MCP registration | 对应 resource block | 可复用资源及其 owner projection |
| Operation receipt | 对应 capability block | sealed operation 的事实输出 |
| Event record | `blocks/eventstream` | 审计记录，不是 authority |
| UI projection | `workorchd` | 可重建展示模型，不是正式对象 owner |

Step、tool call、patch、validation、checkpoint 和 artifact continuation 可以作为过程事实、审计或 projection 存在，
但不构成第二套调度 authority。

## Definition And Activation Contract

1. Workspace owner 提供当前 Goal、DoD、template/profile、resource binding 和 revision。
2. workorchd 从 typed owner reads 组装 `RuntimeBootstrap`；bootstrap 在 Run 创建时冻结。
3. Run owner 接受 workspace-scoped execution intent，并发布 SM-1 内部状态事实。
4. Orchestrator 只能从已持久化 aggregate 与 concrete facts drive。

调用方不得上传 authority map、raw target/recipe/check/metric/artifact graph，也不得指定 Run 的目标 lifecycle state。

## Operation Contract

```text
Orchestrator candidate
  -> capability feasibility result
  -> Orchestrator sealed dispatch
  -> capability owner execution
  -> concrete receipt
  -> Orchestrator evidence judgement
```

- capability owner定义自己的 concrete command/result schema。
- create-only、revision fence、path scope、risk/approval 等约束由 capability owner 校验。
- receipt 只陈述实际发生的结果；它不能返回 next action 或 authoritative acceptance。
- duplicate/stale receipt 由 operation/dispatch/revision identity 幂等处理。

## Governance Contract

Orchestrator 在 canonical `need_governance` 时提交 typed request。Decision 或 Approval owner 创建、转换并持久化对象，
再返回 typed resolution fact。Orchestrator内部 reducer 决定继续或终止；application 不代替任何 owner 改状态。

## Result And Terminal Contract

1. Orchestrator密封 immutable terminal outcome。
2. Result owner幂等写入 formal Result 并确认 delivery。
3. Workspace owner确认同一 delivery，更新 active Run/terminal fence。
4. Run owner最终确认 SM-1 terminal delivery。
5. 若 predecessor 存在有效 `ReplacementIntent`，Run owner 才创建并关联 successor。

任何 projection、SSE consumer 或 application callback 都不得提前释放 slot、创建 successor 或重建 authority。

## User Intent Contract

| Intent | Meaning | Mutation owner |
| --- | --- | --- |
| save definition/resource selection | 更新 Workspace 或资源 owner 数据 | 对应 owner |
| `resume` | 继续当前 active/recoverable Run | Workspace 受理，Run 转换 SM-1 |
| `restart` | 以当前 definition 建立新 lineage | Workspace 受理，Run 管理 replacement transaction |
| resolve decision/approval | 提交治理 resolution | Decision/Approval owner |
| query/observe | 读取 projection、SSE 或审计记录 | workorchd/Eventstream read side |

## Invariants

- 一个状态机只有一个 owner；跨组件只能提交意图或事实。
- application、capability、projection 与 Eventstream 都不选择 Runtime 下一步。
- Runtime core 保持领域中立；业务策略来自 profile/template/Soul/capability。
- `file/write` 等具体能力语义不得由 Orchestrator 猜测；必须来自 capability catalog/feasibility。
- recovery 是 aggregate re-drive，不是事件历史或 UI 状态回放。
