# Activation And Definition Flow

Status: active

Type: runtime-flow

Last Updated: 2026-07-27

## Purpose

本文件定义从正式 Workspace / Goal / DoD 到 **typed Runtime V2 activation**，再到
`InitializeRuntimeV2FromRun` 生成 initial facts 的链路。

生产主线不创建 V1 首个 `llm/workspace_execution` Step、不组装 V1 execution-context map，
也不以 graph dispatch 作为 activation 输出。

## State Machine Binding

本 flow 绑定核心规约中的：

- SM-1：新 Run 的 pending/active admission 与 predecessor lineage；
- SM-2：typed focus/requirement 初始事实；
- SM-4：Initialize 幂等与首次 drive。

本文件不重新定义状态迁移；完整合法/禁止迁移以 [Runtime V2 四状态机固化规约](../00-core/runtime-stage-contract.md#runtime-v2-四状态机固化规约) 为准。

## Flow

```text
Workspace / Goal / DoD / Soul / Skill / MCP / Memory selection
  -> application activation (resolve definition + activation context)
  -> seal RuntimeBootstrap
       Definition: RuntimeDefinitionContext
       Activation: RuntimeActivationContext
       Focuses: CoverageFocus queue
       Requirements: RuntimeRequirement[]
  -> Run creation (persist typed bootstrap on Run owner)
  -> Run owner accepts activation intent and internally transitions pending -> active
  -> RunStateChanged fact -> Orchestrator DriveRuntimeV2
  -> InitializeRuntimeV2FromRun
       load persisted typed bootstrap only
       InitialFacts (focus[0] + remaining queue)
       preflight / reducer
  -> first drive transition (prerequisite or candidate)
```

## Main Stages

### 1. Definition Resolution

activation 前必须明确并可校验：

- 当前 Workspace
- 当前有效 Goal / DoD 与 `definitionRevision`
- 当前 Soul / Skill / MCP / Memory 选择（进入 `RuntimeActivationContext`）
- deliverable / internal directory 边界（供 prerequisite / target snapshot 使用）

Goal / DoD 缺失、无效或不完整时，activation 不得进入 Run 创建。

`RuntimeDefinitionContext` 在 Run owner 中封存；后续 candidate request 与重启恢复
**只能读取**该持久化定义快照，不能由调用方临时重建。

definition 漂移（revision、Goal/DoD identity、正文、capability contract 任一变化）
在后续 commit 时必须返回 conflict。

### 2. Typed RuntimeBootstrap

activation 的正式产物是 **typed `RuntimeBootstrap`**，而不是 V1 execution context map。

```text
RuntimeBootstrap
  Definition   RuntimeDefinitionContext
  Activation   RuntimeActivationContext
  Focuses      []CoverageFocus          // 至少 1 个；含 coverage work items + evidence/assessment 要求
  Requirements []RuntimeRequirement     // prerequisite 需求（capability / manifest 等）
```

约束：

- bootstrap 必须 `Validate()` 通过后才能落库
- focus 需要 identity、coverage work item IDs，以及 required evidence kinds 或 `RequireAssessment`
- `RuntimeActivationContext` 保存 provider-facing 稳定内容快照（Soul/Skill/MCP/Memory、
  evidence policies、execution contract、continuation 摘要）以及当次封存的 `providerId/modelId` identity；
  base URL、credential、provider client、failover/runtime transport 状态不得进入可恢复业务状态
- 不使用 map / JSON bridge 表达组件合同

#### Activation Vs Workspace Setup

| 入口 | 职责 |
| --- | --- |
| workspace setup / draft | 创建前澄清、模板建议、Soul 预选、Goal/DoD 草案；**不创建 Run** |
| execution activation | 基于已确认并已落库的正式定义，封存 `RuntimeBootstrap` 并创建 V2 Run |

template / draft / Soul guidance 只能影响 activation 输入基线，不能替代正式定义。

### 3. InitializeRuntimeV2FromRun（幂等）

`InitializeRuntimeV2FromRun` 只做：

1. 读取 Run owner 已持久化的 typed bootstrap / aggregate
2. 若尚无 V2 state：由 bootstrap 生成 `InitialFacts`，执行 preflight 与首轮 reducer
3. 若已有 aggregate：只加载，**不得**因重复 Start/Resume 再次执行 reducer 或增加 attempt

禁止：

- 解析 V1 execution context map 重建定义或 focus
- 在 Initialize 中直接请求 LLM 或执行 operation
- 以 `RuntimeBootstrap == nil` 保留兼容路由；缺少 typed bootstrap 必须失败并投影 typed terminal

### 4. Start / Resume Entry

| 入口 | 行为 |
| --- | --- |
| Start | Application 向 Run owner 提交 activation intent；Run owner内部 `pending -> active` 后发布 `RunStateChanged`，再触发唯一 `DriveRuntimeV2` |
| Resume | 只携带 workspace/run scope；Run owner校验已解决 blocker 后内部恢复并发布事实；Orchestrator 消费事实进入同一 drive |

约束：

- application **不再**拥有 Start/Resume continuation queue
- Orchestrator service **不**暴露 execution-context-patch resume facade、`StartRun`、
  `DispatchNextStep`、`ResumeRun`、SM-1 lifecycle facade 或自动 replan lifecycle
- `ResumeRunCommand` **不得**重新注入 loaded Run、`ExecutionContextPatch` 或其他 V1 continuation payload
- 上下文变化只能通过**新的** Runtime V2 activation/bootstrap 表达（新 Run）

### 5. Successor / Continuation Semantics

Runtime V2 **不自动创建** V1 replan successor。

当需要重新定义目标或执行基准时：

- 必须通过新的 typed activation/bootstrap 创建**独立** V2 Run
- 旧 Run 的 supersede / lineage 只作为历史引用
- 新 Run **不得**机械复制旧 active focus、stale target snapshot 或已失效 failure surface

若 activation 携带 `RuntimeContinuationContext`，它只表达仍有效的 contract / lineage 摘要，
不表达 V1 patch resume 或 graph continuation。

## Key Flow Guarantees

- activation 围绕正式 Goal / DoD 与 typed bootstrap 工作
- activation 生成的是 V2 Run + 封存 definition/activation/focus，而不是最终业务结果
- Initialize 幂等：重启与重复 Drive 只加载 aggregate
- definition revision 在 facts、authority transaction、candidate 之间必须一致
- 生产入口缺少 typed bootstrap 必须失败，禁止 V1 兼容回落

## Output

activation 的正式输出是：

- 新的 V2 Run（含持久化 `RuntimeBootstrap`）
- 经 Initialize 得到的 initial `RuntimeFacts` / `RuntimeStateRecord`
- 进入 `DriveRuntimeV2` 的前提（首个 reducer decision）

## Source Documents

- [overview.md](overview.md)
- [planning-execution-flow.md](planning-execution-flow.md)
- `../00-core/runtime-stage-contract.md`
- `../../internal/modules/kernel/orchestrator/readme.md`
- `../../internal/pkg/workorchruntimecontract`（`runtime_bootstrap.go`、`runtime_activation.go`、`runtime_definition.go`）
