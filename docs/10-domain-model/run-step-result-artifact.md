# Run / Step / Result / Artifact Domain Model

Status: active

Type: domain-model

Last Updated: 2026-07-27

## Purpose

本文件定义 WorkOrch 执行链中的核心过程对象和正式业务结果对象。

生产主线以 **Runtime** 为准：权威过程面是 Bootstrap / Facts / CoverageFocus / EvidenceLedger / SealedDispatch / Authority 与 Result 裁决边界。Step 与部分过程存储对象可继续作为 **非权威投影 / legacy 过程面** 存在，用于观测、分页与历史证据关联，但不得被叙述为 Runtime 调度权威或 `NextAction` / graph dispatch 载体。

## Core Entities

### Run

Run 是一次围绕当前 Goal / DoD 定义发起的正式执行实例。

建议稳定字段：

- `id`
- `workspaceId`
- `goalId`
- `doDId`
- `definitionRevision`
- `runtimeBootstrap` / bootstrap ref（创建时封存；后续不得由调用方临时 map 重建）
- `state`
- `priority`
- `blockingTicketId`
- `blockingApprovalId`
- `failureReason`
- `runtimeStateRecord` / aggregate CAS 相关投影字段（实现层）

边界：

- Run lifecycle（pending/active/blocked/terminal）由 Run owner 维护。
- Runtime drive 权威状态在 Orchestrator aggregate + Run owner CAS 的 `RuntimeStateRecord`。
- 不得通过 `ExecutionContextPatch` 或 禁止的 lifecycle facade 重建执行上下文。

### RuntimeBootstrap

`RuntimeBootstrap` 是 activation 的正式封存产物。

组成（概念边界）：

- Definition：effective Goal / DoD / definitionRevision 与中立定义载荷
- Activation：Soul / Skill / MCP / Memory / provider 等选定上下文绑定
- Focuses：有序 `CoverageFocus` 队列（至少一个）
- Requirements：capability / evidence / assessment 等中立要求

边界：

- 创建时封存；Initialize 幂等 load，不因重复 Start/Resume 重建。
- 不是 禁止的 execution context map。
- 新 definition 或 replan 语义 = 新 bootstrap + 新 Run。

### RuntimeFacts

`RuntimeFacts` 是 Drive 循环的权威事实输入。

边界：

- 由 Orchestrator aggregate 维护。
- 包含当前 focus、remaining focus 队列、evidence ledger 相关事实、authority 相关状态等。
- reducer(`RuntimeFacts`) → `RuntimeDecision` + `AuthorityTransaction`。

### CoverageFocus

`CoverageFocus` 是有序 focus 队列元素。

建议稳定概念字段：

- focus identity / ordering
- coverage work items / coverage targets
- required evidence kinds / accepted or rejected step kinds（中立）
- optional `RequireAssessment`
- requiredOutput 相关约束（与 capability 中立交集）

边界：

- focus 的 required evidence（及可选 assessment）满足后原子切换下一 focus，再 reduce。
- CoverageWorkItem 可嵌于或关联 CoverageFocus，仍是 run-scoped ledger，不是新 Goal/DoD。
- stage policy / profile 只作为 activation / candidate 输入，不成为 focus 的业务领域语义。

### EvidenceLedger

`EvidenceLedger` 记录 accepted / rejected operation evidence 与 lineage。

边界：

- evidence 必须与 authority snapshot / sealed dispatch / operation 一致。
- stale → diagnostic lineage；duplicate → 幂等。
- process evidence 支撑 Result，不等于 accepted business outcome。
- 最后 assessment 关闭后 Orchestrator 密封 terminal delivery fact；Result、Workspace、Run 各 owner 顺序完成自身迁移。

### SealedDispatch / Authority

| 对象 | 边界 |
| --- | --- |
| `AuthoritySnapshot` / `AuthorityTransaction` | reducer 密封；candidate attempt 绑定同一 transaction |
| `RequiredOutputContract` | 当前 focus 与 capability 中立交集 |
| `SealedDispatch` | 执行前持久化；execution evidence 唯一合法入口；绑定 transaction + candidate hash |

### Step

Step 是 Run 过程执行单元的 **非权威投影 / legacy 过程面**。

建议稳定字段（存储/API 兼容）：

- `id`
- `runId`
- `kind`
- `state`
- `input`
- `output`

稳定边界：

- Step 可用于 tool/patch/validation 过程证据关联、分页 detail、observability timeline。
- Step **不是** Runtime reducer 状态枚举，也不是 `NextAction` / graph dispatch 权威。
- sealed operation 的正式入口是 `SealedDispatch` + `ExecuteRuntimeOperation`，不是通用 Step dispatcher。
- 读模型若展示 step graph / node 进度，只作观测，不得驱动调度。

### Result

Result 是正式业务结果对象，不等于普通 step/operation output 或 DoD summary。

Result 只应在正式 DoD assessment 和 evidence gate 通过后，由 **Orchestrator 裁决并密封 terminal payload**；
Result owner消费该 immutable fact 后在自身 reducer 中写入。

建议稳定字段：

- `id`
- `workspaceId`
- `runId`
- `goalId`
- `doDId`
- `state`
- `title`
- `summary`
- `sourceType`
- `sourceRef`
- `evidenceRefs`
- `artifactRefs`
- `doDAssessment`
- `summaryDetail`

Result 边界：

- Result **不裁决** accepted/rejected；裁决权在 Orchestrator，Result lifecycle 迁移权在 Result owner。
- accepted Result 必须引用 current definition 与 accepted evidence。
- rejected Result 是正式 outcome 语义，不等于 Run process failure 的全部诊断。

### Artifact

Artifact 是受 Run 驱动生成的结构化产物对象。

Artifact 至少需要区分：

- 用户交付物 artifact
- runtime evidence artifact

Artifact 不应被限定为“只属于 Result”。它可以有三种稳定归属：

- 直接绑定 `runId`，表示运行级产物
- 绑定 `stepId` 或 operation/evidence ref，表示某次执行、验证或恢复动作产生的过程产物
- 绑定 `resultId`，表示 accepted / rejected Result 的正式交付或附件

建议稳定字段：

- `id`
- `workspaceId`
- `runId`
- `resultId`
- `stepId`
- `artifactType`
- `category`
- `name`
- `summary`
- `uri`
- `contentType`
- `state`
- `evidenceRefs`

归属约束：

- deliverable artifact 通常应绑定 `resultId`
- runtime evidence artifact 可以只绑定 `runId` / `stepId` / operation ref
- 同一 artifact 不得在读模型层被同时投影为“正式 deliverable”与“普通过程证据”

## Evidence Objects

以下对象属于 Run 过程证据，不是正式业务结果：

- ToolCall
- Patch
- ValidationResult
- RuntimeCheckpoint

这些对象可支撑 Result / Artifact，但不能被重新标记为最终业务 outcome。在 Runtime 下，它们应能追溯到 sealed dispatch / operation evidence / assessment 路径；不得包装为 `StagePacket` 进度事件。

### ToolCall

ToolCall 表示一次正式外部动作或适配块调用证据。

建议稳定字段：

- `id`
- `runId`
- `stepId`
- `toolKind`
- `toolName`
- `inputRef` / `argumentRef`
- `outputRef`
- `state`
- `startedAt`
- `finishedAt`
- `evidenceRefs`

稳定边界：

- ToolCall 是过程动作证据，不是业务结果
- 它通常关联 `stepId` 或 operation projection，并通过 `runId` 进入 run 级聚合
- ToolCall 可以被 Result / Validation / Observability 引用，但不应被当作正式 deliverable

### Patch

Patch 表示一次 state-changing artifact 或变更请求证据。

建议稳定字段：

- `id`
- `runId`
- `stepId`
- `targetRef`
- `patchType`
- `baseHash`
- `coverageTargets`
- `state`
- `summary`
- `artifactRefs`

稳定边界：

- Patch 是变更证据，不等于已被接受的业务交付
- Patch 通常关联执行单元投影，必要时可关联 `resultId` 对应的 accepted closure
- patch content / raw diff 属于 detail/raw 打开面，不应默认等于 Artifact 正文

### ValidationResult

ValidationResult 表示 build/test/check/assessment 这类验证动作的正式证据。

建议稳定字段：

- `id`
- `runId`
- `stepId`
- `validationKind`
- `targetRef`
- `state`
- `summary`
- `evidenceRefs`

稳定边界：

- ValidationResult 只表达验证证据，不直接表达 accepted business outcome
- 它可被 DoD assessment / Result closeout 引用
- ValidationResult 通过 evidence quality / alignment 参与 closure，但不能替代 Result 本体
- `state` 的持久化枚举以 runtime evidence contract 为准：`passed` / `failed` /
  `inconclusive` / `warning`。产品展示可以映射成 pass/fail/warning 文案，
  但不得把展示标签反写成 owner state。

### RuntimeCheckpoint

RuntimeCheckpoint 表示一次可恢复、可诊断的运行时快照证据。

建议稳定字段：

- `id`
- `runId`
- `stepId`
- `checkpointType`
- `state`
- `summary`
- `dataRef`
- `createdAt`

稳定边界：

- RuntimeCheckpoint 是运行时 **诊断 / 观测** 证据，不是业务结果
- Runtime 恢复权威 = load 已持久化 aggregate / `RuntimeStateRecord`，**不** 以 禁止的 progress/terminal checkpoint 重放作为驱动
- 它可以进入 observability、timeline、repair lineage 和 audit 输入面
- RuntimeCheckpoint 不应被前端默认展示成正式 Artifact 或 Result

### CoverageWorkItem

CoverageWorkItem 是 run-scoped 的 coverage ledger 对象。

它的作用是把当前 effective Goal / DoD / deliverable source 投影成可收敛、可追溯、
可验证的工作项，并服务于 `CoverageFocus`。

建议稳定字段：

- `id`
- `runId`
- `definitionRevision`
- `normalizedSourceKey`
- `sourceScope`
- `sourceRequirementId`
- `targetText`
- `requiredEvidenceKinds`
- `status`
- `acceptedEvidenceRefs`
- `rejectedEvidenceRefs`
- `currentGap`
- `updatedByStepId` / updated-by operation ref

稳定边界：

- CoverageWorkItem 不是新的 Goal / DoD 定义
- 它属于当前 run 的 coverage ledger，而不是 workspace formal definition
- 它可以被 verification / acceptance / repair 引用，但不替代 Run / Result 主状态机
- 同一 `definitionRevision + normalizedSourceKey` 在当前 run 内只能有唯一 active item
- `status` 的 canonical 集合是 `open` / `in_progress` / `evidence_ready` /
  `satisfied` / `failed` / `blocked`。历史或展示层的 `closed`、`completed`
  不得作为 coverage ledger 终态参与收敛判断。

### ArtifactContinuation

ArtifactContinuation 是 run-scoped 的 runtime internal continuation 对象。

它用于承接 `incrementalArtifact` 的 plan / chunk / assemble / apply 恢复链。

建议稳定字段：

- `id`
- `runId`
- `rootStepId`
- `currentStepId`
- `artifactId`
- `targetRef`
- `operation`
- `outputMode`
- `expectedSequence`
- `receivedSequences`
- `finalReceived`
- `bytesWritten`
- `rollingHash`
- `lastChunkHash`
- `coverageTargets`
- `state`
- `createdAt`
- `updatedAt`

稳定边界：

- ArtifactContinuation 属于 runtime internal state，不属于 workspace deliverable
- 它服务恢复、诊断和 metadata-only projection，不直接表达 accepted business outcome
- 它可以被 snapshot / observability / raw-detail 引用，但默认不暴露正文内容
- apply / validation 完成后，正式 closeout 仍回到 Result / Artifact / Validation evidence
- public lifecycle 只应表达 `active` / `sealed` / `failed` / `aborted` 这类
  对外可解释状态；runtime 内部可以持久化 `planning` / `chunking` /
  `assembling` / `applying` / `applied` 等 phase state，用于恢复与诊断。
  这些 phase state 不能被前端或 Result gate 当作正式 deliverable 完成语义。

## Ownership Mapping

这些过程证据对象与 Step / Result / Artifact 的稳定拥有关系应保持：

| 对象 | 主要归属 | 可被谁引用 | 不应被谁替代 |
| --- | --- | --- | --- |
| `RuntimeBootstrap` / `RuntimeFacts` | Run + Orchestrator aggregate | Drive、audit、projection summary | Step / Result |
| `CoverageFocus` / `EvidenceLedger` | Orchestrator aggregate | completion gate、projection、diagnostics | Goal/DoD 本体 |
| `SealedDispatch` | Orchestrator | operation execution、evidence gate | generic Step dispatcher |
| `ToolCall` | `runId` / 过程投影 `stepId` | Result、Observability、Validation、TaskProgress | Result / Artifact |
| `Patch` | `runId` / 过程投影 `stepId` | Result、Artifact、Observability | accepted deliverable 本体 |
| `ValidationResult` | `runId` / 过程投影 `stepId` | DoD assessment、Result、Observability | Result 本体 |
| `RuntimeCheckpoint` | `runId`，必要时过程投影 `stepId` | Observability、audit、timeline | Runtime drive authority / Result / Artifact |
| `Step` | `runId` | Observability、detail pages | RuntimeDecision / SealedDispatch |

约束：

- 这些对象都属于运行过程证据族或过程投影族
- 它们可以支撑 accepted / rejected Result，但不直接拥有业务完成语义
- Web/TUI/API 的默认 summary 视图不得把它们与 Result / Artifact 混成同一对象层
- 不得将 Step 过程投影重新包装为 禁止的 `StagePacket` / `NextAction` 生产对象

## State Boundary

Run 终态与 Result 终态不是一回事：

- Run 关注执行生命周期与 blocker 投影
- Result 关注正式 outcome 语义（承接 Orchestrator 裁决）

同理，Artifact 是 Result 的组成部分或过程产物，不是 Run 的替代状态。

Runtime 过程权威状态（facts / focus / ledger / sealed dispatch）与 Step 过程投影状态不是一回事。

## Invariants

- accepted Result 必须有可追踪 evidence refs
- Result 不裁决；Orchestrator 只密封 terminal payload，Result owner消费 immutable delivery fact 后内部写入
- Artifact 必须通过逻辑 URI 暴露，不得暴露本地绝对路径
- ToolCall / Patch / ValidationResult / RuntimeCheckpoint 仍保留为过程证据
- CoverageWorkItem / CoverageFocus 必须保持 run-scoped，不能回写 formal Goal / DoD
- ArtifactContinuation 必须保持 internal state，不能默认投影成正式 deliverable artifact
- Web / TUI / API 不得把普通 step/operation output 直接冒充为正式 Result
- Bootstrap 创建后不得由调用方临时 map 重建；Resume 不得注入 ExecutionContextPatch
- replan 语义不得自动创建 successor Run；定义变更 = 新 activation / 新 Run

## Projection Boundary

Run snapshot、run observability、workspace outcome、task progress 都是运行态投影。
它们可以引用 Result / Artifact / focus / ledger summary，但不是这些对象的拥有者。

Step graph / RecoveryPlan / nextAction 类展示字段若存在，仅为诊断，不驱动调度。

## Source Documents

- `../00-core/runtime-stage-contract.md`
- `../20-runtime-flow/overview.md`
- `../20-runtime-flow/activation-definition-flow.md`
- `../20-runtime-flow/evidence-result-flow.md`
- `../90-archive/completed/result-event-development-plan.md`
- `../90-archive/superseded/workspace-data-layout-plan.md`
- `../90-archive/records/delivery-history.md`
