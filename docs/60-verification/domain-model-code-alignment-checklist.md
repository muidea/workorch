# Domain Model Code Alignment Checklist

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件用于核对 `10-domain-model/` 当前对象定义是否已经被代码实现正确承接。

它不重新定义领域模型，只把当前需要逐项核对的对象层实现问题拆成可执行清单，
方便后续按项处理。

## Canonical Sources

核对时以以下文档为准：

- `../10-domain-model/workspace-goal-dod.md`
- `../10-domain-model/run-step-result-artifact.md`
- `../10-domain-model/governance-decision-approval.md`
- `../10-domain-model/context-capability-memory.md`
- `../10-domain-model/event-observability.md`
- `../20-runtime-flow/activation-definition-flow.md`
- `../20-runtime-flow/verification-acceptance-flow.md`
- `../20-runtime-flow/evidence-result-flow.md`
- `../30-module-architecture/storage-readmodel.md`
- `../50-product-interface/service-api.md`

## Review Goal

这一轮只回答三个问题：

1. 文档定义的对象是否已经在代码中有正式落点。
2. 对象归属和字段语义是否与当前设计一致。
3. 哪些点属于“文档超前”，哪些点属于“实现偏差”。

## Suggested Review Scope

建议优先核对以下代码目录：

- `internal/modules/kernel`
- `internal/pkg/repository`
- `internal/modules/application/workorchd`
- `internal/pkg/events`

如对象定义集中在其他目录，也应一并纳入。

## Review Order

建议顺序：

1. Workspace / Goal / DoD
2. Run / Step / Result / Artifact
3. CoverageWorkItem / ArtifactContinuation
4. Governance
5. Context / Memory / MemoryRecallTrace
6. Event / Observability / Outcome / TaskProgress
7. API / read-model projection naming

## 1. Workspace / Goal / DoD

重点核对：

- `Workspace` 是否明确区分：
  - `deliverableDirectory`
  - `internalDirectory`
- 新建 workspace 时是否同时建立两个目录边界。
- `Goal` / `DoD` 是否有正式对象，而不是只保留压缩摘要。
- `Goal` / `DoD` 是否存在：
  - `definitionRevision`
  - `previousDefinitionHash`
  - lineage / superseded / historical 等价语义
- activation 是否能基于明确的 effective definition，而不是混用旧 revision。

常见偏差：

- internal state 写入 deliverable 目录
- Goal / DoD 只有当前文本，没有 revision / lineage
- superseded definition 仍被当作 active definition 使用

## 2. Run / Step / Result / Artifact

重点核对：

- `Run` 是否已经具备或等价承接：
  - `goalId`
  - `doDId`
  - `definitionRevision`
  - `blockingTicketId`
  - `blockingApprovalId`
- `Step` 是否明确是 run 的基本执行单元，而不是混入结果语义。
- `Result` 是否是正式业务结果对象，而不是普通 step output。
- `Artifact` 是否支持三种归属：
  - `runId`
  - `stepId`
  - `resultId`
- deliverable artifact 和 runtime evidence artifact 是否在归属上被区分。

常见偏差：

- Run 没有 definition revision 语义
- Artifact 被写死成只属于 Result
- 过程证据 artifact 和正式 deliverable artifact 混投影

## 3. CoverageWorkItem

对照 `run-step-result-artifact.md` 与 verification/acceptance flow，重点核对：

- `CoverageWorkItem` 是否已经是正式 run-scoped 对象，而不是临时拼装投影。
- 是否存在或等价承接以下字段：
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
- 是否保证同一 `definitionRevision + normalizedSourceKey` 只有唯一 active item。
- 是否保持 run-scoped，不会回写 formal Goal / DoD。

常见偏差：

- coverage ledger 只存在 read-model，没有正式对象/仓储落点
- 同一 source 生成多个 active item
- flow 层 gap 直接改写 formal Goal / DoD

## 4. ArtifactContinuation

重点核对：

- `ArtifactContinuation` 是否已经是正式 runtime internal state 对象。
- 是否存在或等价承接：
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
- 是否明确不属于 workspace deliverable。
- 是否默认只用于恢复、诊断和 metadata-only projection。

常见偏差：

- continuation 只是内存临时结构，没有正式 repository contract
- continuation 被错误投影成正式 artifact/deliverable
- continuation 正文进入默认 snapshot/observability

## 5. Governance

重点核对：

- `Decision` / `Approval` 是否是正式治理对象，不与 Result / Run 混用。
- `Resolution` 是否存在并能表达：
  - `continuationAction`
  - `resumeRunId`
  - `successorRunId`
  - `payload`
- `replan` 是否只表达治理结论，不替代 successor run 本体。
- `autoGovernance` 是否只是治理摘要/建议，不替代正式对象。

常见偏差：

- replan 直接改写 run，而没有独立 resolution 语义
- auto-governance 结果没有结构化对象或摘要
- blockingTicketId / blockingApprovalId 只存在投影，不存在正式 run 关联

## 6. Context / Memory / MemoryRecallTrace

重点核对：

- `MemoryItem` 是否只沉淀高价值可复用知识，而不是事件日志摘要。
- `MemoryRecallTrace` 是否是正式过程证据对象，而不是前端从 raw context 反推。
- `MemoryRecallTrace` 是否存在或等价承接：
  - `source`
  - `stage`
  - `policy`
  - `status`
  - `selectedMemoryIds`
  - `candidateMemoryIds`
  - `excludedMemoryIds`
  - `retrievedMemoryHits`
  - `reason`
  - `evidenceRef`
- recovery / activation / step_input recall 是否都有可追踪 trace。

常见偏差：

- 仍使用含混的 `retrievedMemoryIds`，没有 selected/candidate/excluded 区分
- recall 只存在执行上下文里，没有正式 trace 对象
- provider retry / 低价值流水被大量固化为 memory

## 7. Event / Observability / Outcome / TaskProgress

重点核对：

- `EventRecord` 是否支持：
  - `payload`
  - `payloadSummary`
  - `payloadRef`
- 大 payload 是否默认走 summary + ref，而不是直接内嵌。
- `Outcome` / `TaskProgress` 是否只是投影，不是正式状态拥有者。
- latest accepted result / latest terminal run 的选择优先级是否已在代码中体现。
- `RunObservability` 是否消费 summary / refs，而不是 raw payload 主视图。

常见偏差：

- 事件默认只存 payload，没有 summary/ref 分层
- Outcome / TaskProgress 反向覆盖正式 Result / Run 状态
- observability 默认依赖 raw payload 大对象

## 8. API / Read-Model Naming Alignment

重点核对：

- API / read-model 命名是否与对象层一致：
  - `coverageWorkItems`
  - `coverageWorkItemContext`
  - `artifactContinuations`
  - `selectedCapabilities`
  - `usedCapabilities`
  - `selectedMemoryIds`
  - `candidateMemoryIds`
  - `excludedMemoryIds`
- `blockingTicketId` / `blockingApprovalId` 是否从正式 run/gov 对象投影而来。
- `artifactContinuations` 是否默认只做 metadata-first 投影。

常见偏差：

- 对象层和 API 层命名分叉
- API 暴露的字段在 domain 层没有正式对象来源
- 投影字段由前端或 appservice 临时拼装，没有稳定本体支撑

## Classification Rules

核对结果建议分三类：

- `pass`
  - 设计与代码一致，语义和归属都成立
- `doc-ahead`
  - 文档定义已明确，但代码尚未正式落地
- `impl-drift`
  - 代码已有实现，但语义、字段或归属与当前设计冲突

## High-Priority Checks

优先检查这些高风险点：

1. `Run.definitionRevision` 是否真实存在或有等价实现。
2. `CoverageWorkItem` 是否是正式 run-scoped ledger 对象。
3. `ArtifactContinuation` 是否是正式 internal state，而不是临时投影。
4. `MemoryRecallTrace` 是否已经从单一 `retrievedMemoryIds` 演进到
   selected/candidate/excluded 语义。
5. `payloadSummary` / `payloadRef` 是否已经形成正式事件分层。
6. `blockingTicketId` / `blockingApprovalId` 是否是真实 run 关联，而不是只在 API 层临时拼装。

## 2026-05-30 Code Alignment Result

| Area | Code Path | Result | Notes |
| --- | --- | --- | --- |
| Workspace / Goal / DoD | `internal/modules/kernel/workspace/pkg/models/{goal,dod}.go` | pass | 已补齐 `definitionRevision`、`previousDefinitionHash`、previous definition id、`lineageState`，并由 `GoalDoDManager` 在定义/更新时写入。 |
| Run | `internal/modules/kernel/run/pkg/models/run.go` | pass | `Run.definitionRevision` 已成为正式字段；创建 run 时从 activation effective definition 派生稳定 revision id。 |
| Result / Artifact | `internal/modules/kernel/result/pkg/models` | pass | Result 与 Artifact 已有正式对象；Artifact 支持 run/step/result 等归属。 |
| CoverageWorkItem | `internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items.go` | doc-ahead | 当前仍是 run-scoped ledger，但落点为 `Run.ExecutionContext` map 与 read-model 投影；尚未拆成独立 model/repository。该项影响 orchestrator 恢复、acceptance gate、read-model 和 provider context，需作为单独迁移项处理。 |
| ArtifactContinuation | `internal/modules/kernel/orchestrator/pkg/models/artifact_continuation.go` | pass | 已补齐 `currentStepId` 与 `outputMode`；repository contract 覆盖 metadata/chunk/assembled 分离与新字段持久化。 |
| Governance | `internal/modules/kernel/decision/pkg/models` | partial | Ticket/Resolution 已有正式对象；Resolution 的 `continuationAction`、`resumeRunId`、`successorRunId` 仍主要由 `payload` 表达，后续可提升为顶层字段。 |
| Memory / Recall | `internal/modules/kernel/memory/pkg/models/memory_recall_trace.go` | pass | `selectedMemoryIds`、`candidateMemoryIds`、`excludedMemoryIds`、`retrievedMemoryHits` 等已是正式 trace 字段。 |
| Event / Observability | `internal/modules/kernel/eventstream/pkg/models/event_record.go` | pass | `payload` / `payloadSummary` / `payloadRef` 已进入正式 EventRecord；file repository 对大 payload 使用 summary + ref，并通过 raw hydrate 支持明细读取。 |
| API / read-model naming | `internal/modules/application/workorchd/biz/appservice/read_model_types.go` | pass | `payloadSummary` / `payloadRef` 已进入 read-model event projection；coverage/artifact continuation 命名保持现有对象层命名。 |

### Remaining Follow-Up

- `CoverageWorkItem` 独立对象化仍是主要 `doc-ahead` 项。落地时应新增正式 model/repository，并保持同一 `definitionRevision + normalizedSourceKey` 只有唯一 active item；迁移前不得破坏现有 `Run.ExecutionContext.coverageWorkItems` 的 LLM context 投影顺序和 cache 稳定性。
- `DecisionResolution` 顶层恢复引用字段仍是 `partial`。后续可补 `continuationAction`、`resumeRunId`、`successorRunId` 顶层字段，并保持 `payload` 作为扩展数据。

## Suggested Output Template

后续逐项核对时，建议按以下格式记录：

| Area | Code Path | Result | Notes |
| --- | --- | --- | --- |
| Workspace / Goal / DoD | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| Run / Result / Artifact | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| CoverageWorkItem | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| ArtifactContinuation | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| Governance | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| Memory / Recall | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| Event / Observability | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| API / read-model naming | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
