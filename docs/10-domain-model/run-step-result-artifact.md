# Run / Step / Result / Artifact Domain Model

Status: active

Type: domain-model

Last Updated: 2026-05-30

## Purpose

本文件定义 WorkOrch 执行链中的核心过程对象和正式业务结果对象。

## Core Entities

### Run

Run 是一次围绕当前 Goal / DoD 定义发起的正式执行实例。

建议稳定字段：

- `id`
- `workspaceId`
- `goalId`
- `doDId`
- `definitionRevision`
- `initialStepKind`
- `executionContext`
- `state`
- `priority`
- `blockingTicketId`
- `blockingApprovalId`
- `failureReason`

### Step

Step 是 Run 的基本执行单元。

建议稳定字段：

- `id`
- `runId`
- `kind`
- `state`
- `input`
- `output`

### Result

Result 是正式业务结果对象，不等于普通 step output 或 DoD summary。

Result 只应在正式 DoD assessment 和 evidence gate 通过后生成。

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

### Artifact

Artifact 是受 Run 驱动生成的结构化产物对象。

Artifact 至少需要区分：

- 用户交付物 artifact
- runtime evidence artifact

Artifact 不应被限定为“只属于 Result”。它可以有三种稳定归属：

- 直接绑定 `runId`，表示运行级产物
- 绑定 `stepId`，表示某次执行、验证或恢复动作产生的过程产物
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
- runtime evidence artifact 可以只绑定 `runId` / `stepId`
- 同一 artifact 不得在读模型层被同时投影为“正式 deliverable”与“普通过程证据”

## Evidence Objects

以下对象属于 Run 过程证据，不是正式业务结果：

- ToolCall
- Patch
- ValidationResult
- RuntimeCheckpoint

这些对象可支撑 Result / Artifact，但不能被重新标记为最终业务 outcome。

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
- 它通常从属于 `stepId`，并通过 `runId` 进入 run 级聚合
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
- Patch 通常从属于 `stepId`，必要时可关联 `resultId` 对应的 accepted closure
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
- 它通常从属于 `stepId`，并可被 DoD assessment / Result closeout 引用
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

- RuntimeCheckpoint 是运行时诊断 / 恢复证据，不是业务结果
- 它可以进入 observability、timeline、repair lineage 和 replay 输入面
- RuntimeCheckpoint 不应被前端默认展示成正式 Artifact 或 Result

### CoverageWorkItem

CoverageWorkItem 是 run-scoped 的 coverage ledger 对象。

它的作用是把当前 effective Goal / DoD / deliverable source 投影成可收敛、可追溯、
可验证的工作项。

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
- `updatedByStepId`

稳定边界：

- CoverageWorkItem 不是新的 Goal / DoD 定义
- 它属于当前 run 的 coverage ledger，而不是 workspace formal definition
- 它可以被 verification / acceptance / repair / replan 引用，但不替代 Run / Result 主状态机
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
| `ToolCall` | `stepId` / `runId` | Result、Observability、Validation、TaskProgress | Result / Artifact |
| `Patch` | `stepId` / `runId` | Result、Artifact、Observability | accepted deliverable 本体 |
| `ValidationResult` | `stepId` / `runId` | DoD assessment、Result、Observability | Result 本体 |
| `RuntimeCheckpoint` | `runId`，必要时 `stepId` | Observability、repair/replay、timeline | Result / Artifact |

约束：

- 这些对象都属于运行过程证据族
- 它们可以支撑 accepted / rejected Result，但不直接拥有业务完成语义
- Web/TUI/API 的默认 summary 视图不得把它们与 Result / Artifact 混成同一对象层

## State Boundary

Run 终态与 Result 终态不是一回事：

- Run 关注执行生命周期
- Result 关注正式 outcome 语义

同理，Artifact 是 Result 的组成部分，不是 Run 的替代状态。

## Invariants

- accepted Result 必须有可追踪 evidence refs
- Artifact 必须通过逻辑 URI 暴露，不得暴露本地绝对路径
- ToolCall / Patch / ValidationResult / RuntimeCheckpoint 仍保留为过程证据
- CoverageWorkItem 必须保持 run-scoped，不能回写 formal Goal / DoD
- ArtifactContinuation 必须保持 internal state，不能默认投影成正式 deliverable artifact
- Web / TUI / API 不得把普通 step output 直接冒充为正式 Result

## Projection Boundary

Run snapshot、run observability、workspace outcome、task progress 都是运行态投影。
它们可以引用 Result / Artifact，但不是 Result / Artifact 的拥有者。

## Source Documents

- `../90-archive/completed/result-event-development-plan.md`
- `../90-archive/superseded/workspace-data-layout-plan.md`
- `../90-archive/records/delivery-history.md`
- `../00-core/runtime-stage-contract.md`
