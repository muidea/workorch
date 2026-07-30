# Governance Domain Model

Status: active

Type: domain-model

Last Updated: 2026-07-29

## Purpose

本文件定义 WorkOrch 中的治理对象：Decision、Approval、Resolution 及其边界。

**Runtime V2 边界**：canonical `need_governance` 时 Orchestrator 打开 Decision ticket 并将 ticket↔authority snapshot 绑定写入
V2 aggregate，再发布 immutable governance wait fact；Run owner在自身 reducer 中进入 blocked。继续路径是
Decision → `CommitRuntimeGovernance*Resolution` → Orchestrator reducer/evidence → immutable continuation fact → Run owner-local transition；**application 不得**在 resolve 后自动 Resume。Approval 仍服务高风险能力准入；不能绕过 Orchestrator contract gate。

## Core Entities

### Decision Ticket

Decision Ticket 表示 runtime 无法继续自动推进、需要结构化治理决策的对象。

建议稳定字段：

- `id`
- `workspaceId`
- `runId`
- `stepId`
- `reason`
- `decisionKind`
- `question`
- `context`
- `state`
- `failureClass`
- `failureSignature`
- `allowedRecoveryActions`
- `autoGovernance`

Decision Ticket 的典型来源至少包括：

- runtime 无法自动取得的必要信息
- repair exhausted
- duplicate failure
- provider / config / policy blocker
- replan-required strategy decision

这些来源只表达治理触发面，不等价于最终恢复动作。

### Resolution

Resolution 表示对 Decision Ticket 的正式处理结果。

建议稳定字段：

- `id`
- `ticketId`
- `type`
- `payload`
- `continuationAction`
- `resumeRunId`
- `successorRunId`
- `createdAt`

常见类型：

- `provide_missing_info`
- `reject_current_path`
- `replan`

稳定衔接约束：

- `continuationAction` 只表达恢复动作类型，例如 `resume` / `replan` / `terminate`
- `resumeRunId` 只引用被恢复的既有 Run，不代替 Run 本体
- `successorRunId` 只引用 replan 后的新 Run，不代替 successor scope 的治理结论
- `payload` 负责承载决策输入、原因、补充参数和人工说明，不承担 Run 对象持久化职责

#### Resolution / Recovery Mapping

Resolution 至少应能稳定表达以下恢复引用关系：

| 字段 | 当前方向 |
| --- | --- |
| `continuationAction` | 恢复类型：`resume` / `replan` / `terminate` / 等价中立动作 |
| `resumeRunId` | 被恢复的既有 run |
| `successorRunId` | replan 后的新 run |
| `payload` | 结构化补充信息、策略选择、operator note、recovery parameters |

约束：

- `resumeRunId` 与 `successorRunId` 可以都为空，但不能都替代治理对象本体
- `replan` 结论只选择 successor 路径；successor run 的对象边界仍属于 Run 本体
- `reject_current_path` / `terminate` 属于治理结论，不等于 rejected Result 本体

### Approval

Approval 表示需要显式审批的治理对象。

建议稳定字段：

- `id`
- `workspaceId`
- `runId`
- `stepId`
- `reason`
- `state`
- `action`
- `context`
- `autoGovernance`

Approval 的典型来源至少包括：

- 高风险 state-changing action
- profile / policy 要求的显式人工确认
- auto-governance 已识别但仍需人工批准的动作

## Boundary

Decision / Approval 都是治理对象，不是业务结果对象。

它们的作用是：

- 中断自动执行（Runtime V2：`need_governance` → ticket + blocked 投影）
- 记录治理上下文与 ticket↔authority snapshot 绑定所需 scope
- 允许结构化 resolution（resume / reject / replan / terminate 等动作类型）

它们不应承担：

- 代替 Result 表达正式业务完成
- 代替 Memory 表达长期知识
- 代替 Run 表达执行生命周期或 V2 aggregate
- 代替 Orchestrator 消费 resolution 后的 drive / reducer 推进
- application 在 resolve 后自动 Resume（V2 governance ticket 禁止该路径）

## Invariants

- 只有真实治理缺口才应产生 Decision / Approval
- 已存在 concrete target evidence 且可机械推进时，不应滥用 Decision Ticket
- Resolution / Approval 行为必须可追踪到对应 Run / Step scope
- replan 只表达治理结论，不直接等于 successor run 本体；V2 重定义目标 = 新 bootstrap + 新 Run
- duplicate failure / repair exhausted 形成 typed no-progress/terminal fact，不是治理触发面，也不得据此创建 ticket
- auto-governance 只能选择恢复动作或治理建议，不能替代正式 Decision / Approval / Resolution 对象
- `runtime_v2_governance` resolution 必须经 typed `CommitRuntimeGovernance*Resolution` 提交 Orchestrator；同一 resolution ID + authority snapshot 最多消费一次

## Recovery Trigger Boundary

治理对象与恢复链之间的对象边界至少应保持：

| 触发面 | 当前治理对象语义 |
| --- | --- |
| `typed external dependency` | 已封存权限、外部依赖、配置或策略明确要求人工输入；是 Runtime 自动创建 ticket 的唯一来源 |
| `auto-governance` | policy 可针对已封存 dependency 给出建议，但不能把普通失败或预算耗尽转换为 dependency |
| `operator replan` | 用户显式选择新 run 路径；旧 run 的 terminal/superseded 仍由对应 owner 合同完成 |
| `runtime_v2_governance` | Orchestrator 打开的 V2 governed request；ticket 只拥有治理选择状态，authority/dispatch 绑定在 V2 aggregate |

这些对象层语义只表达“为什么进入治理”和“治理选择了哪条恢复路”，
不直接描述 flow 细节或脚本行为，也不推导 `StagePacket` / `NextAction` / `DispatchNextStep`。

## Projection Boundary

Decision list、Approval list、governance summary、task progress 只是治理对象的投影。

它们不应反向改写 runtime contract、Run 本体状态或 Runtime V2 authority。

## Source Documents

- `../00-core/runtime-stage-contract.md`
- `../30-module-architecture/kernel-orchestrator-governance.md`
- `../50-product-interface/service-api.md`
- `run-step-result-artifact.md`
