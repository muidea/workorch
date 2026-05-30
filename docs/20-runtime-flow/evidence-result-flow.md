# Evidence And Result Flow

Status: active

Type: runtime-flow

Last Updated: 2026-05-30

## Purpose

本文件定义过程证据如何被记录、整理、投影，并最终支撑正式 Result / Artifact。

## Flow

```text
step / tool / patch / validation / checkpoint / event
-> evidence summary and refs
-> verification / acceptance consumption
-> Result / Artifact creation
-> read model and observability projection
```

## Evidence Sources

主要 evidence 来源包括：

- Step
- ToolCall
- Patch
- ValidationResult
- RuntimeCheckpoint
- EventRecord
- MemoryRecallTrace

这些对象共同形成运行态证据面，但不应被重新命名为最终业务结果。

## Result Creation Path

Result 的形成应当：

- 依赖正式 `dodAssessment`
- 依赖 evidence refs
- 与 Goal / DoD / Run / Step 保持可追溯关系

Artifact 则是 Result 的具体业务交付物或 runtime evidence 附件。

artifact ownership 必须与领域层保持一致：

- deliverable artifact 应优先挂到 `resultId`
- 过程 evidence artifact 可以只挂到 `runId` / `stepId`
- 读模型不能把所有 artifact 都默认渲染为最终交付物

## Projection Path

evidence 进入以下投影：

- Run snapshot
- Workspace snapshot
- Run observability
- task progress
- context bundle
- final outcome display

## Evidence Quality And Closure Priority

evidence 在进入 Result / Artifact / outcome 投影前，必须先按中立质量分层。

当前最小质量层次应包括：

- `direct`
- `indirect`
- `smoke`
- `unrelated`
- `invalid`

稳定规则：

- 只有 `direct` 或可接受的 `indirect` evidence 可以关闭对应 coverage target
- `smoke` 只能证明基础可执行或环境可用，不得单独关闭功能 Goal / DoD / deliverable coverage
- `unrelated` / `invalid` 必须保留 rejected reason，但不得进入 accepted closure

## Coverage-Evidence Projection

当 runtime 维护 coverage ledger / `coverageWorkItems` 时，evidence flow 必须支持：

- accepted evidence refs
- rejected evidence refs
- latest closure evidence
- superseded failure evidence
- coverage target / work item 关联

这意味着 evidence flow 不只是“保留 refs”，还必须回答：

- 哪条证据正在关闭哪个 coverage source
- 哪条失败证据已经被后续成功 evidence 覆盖
- 哪些 wrapper failure 只属于诊断，不属于 closure evidence

## Result / Artifact Closeout Rules

Result / Artifact closeout 当前至少应满足以下顺序：

```text
evidence captured
-> quality / alignment / coverage target check
-> accepted vs rejected closure decision
-> Result creation
-> Artifact ownership projection
-> outcome / snapshot / observability projection
```

补充约束：

- accepted Result 必须引用当前最新 non-superseded closure evidence
- rejected Result 必须能解释 DoD gap、rejected evidence 和 next action 来源
- wrapper failure、schema invalid、provider retry、旧失败 step 只能保留为 lineage / diagnostics，
  不得覆盖最新 accepted closure
- 若存在 accepted Result，workspace/run outcome 投影必须优先引用该 Result 的 closure evidence

## Patch / Payload / Artifact Boundary

以下边界必须保持一致：

- `patch/apply`、`patchRequests`、`executionGraph.nodes[].input.coverageTargets`
  必须在副作用执行前通过 coverage target / target relevance / bounded verification 校验
- raw payload 继续默认走 ref-only 路径
- deliverable artifact 与 process evidence artifact 必须在 ownership 上继续区分：
  - deliverable 优先挂 `resultId`
  - process evidence 可挂 `runId` / `stepId`
- `public API` 与 read-model 不得把所有 artifact 都默认展示成最终交付物

## Raw Payload Ref Taxonomy

raw payload 默认不直接进入主投影，而应按受控 ref 分类打开。

当前稳定分类方向至少包括：

| Raw Ref 类型 | 典型来源 | 当前方向 |
| --- | --- | --- |
| provider request / response / stream | LLM call raw I/O | 只通过 ref/detail 打开，不进入默认 summary |
| step input / output | step 级原始输入输出 | 只作为诊断或 replay 输入 |
| tool call arguments / output | MCP / shell / adapter 原始内容 | 只服务 detail/debug，不替代 ToolCall summary |
| runtime checkpoint data | 恢复与诊断快照 | 只通过 ref 打开，不默认塞进 observability 主视图 |
| result detail blob | 长 summary / full assessment / manifest detail | 只在显式 detail 时读取 |

约束：

- raw ref 只表达“如何打开原始内容”，不表达业务结果等级
- raw ref 不应绕过 evidence quality / closeout gate
- raw 打开链路属于模块/API/detail 层，默认 summary 仍应优先展示 compact metadata

## Event Payload Summary Contract

Event evidence 进入 result / observability / event list 时，应优先使用：

- 小型结构化 `payload`
- `payloadSummary`
- `payloadRef`

当前稳定合同：

- `payloadSummary` 用于默认列表、snapshot、observability 摘要
- `payloadRef` 用于显式打开 raw event payload
- 大 payload 存在时，不得因为提供了 `payloadRef` 就完全丢失默认摘要
- event payload summary 属于 evidence view，不直接表达 accepted result 结论

## Final Result Summary Schema

final Result 的稳定摘要层应至少围绕以下字段组织：

| 字段 | 当前方向 |
| --- | --- |
| `title` | 最终结果标题 |
| `summary` | 紧凑结论摘要 |
| `summaryDetail` | 可展开的详细结果说明 |
| `doDAssessment` | DoD / Goal / deliverable coverage 结果 |
| `evidenceRefs` | 支撑该结果的 accepted closure evidence |
| `artifactRefs` | 最终交付物或附件引用 |
| `nextActions` / residual gap | rejected / incomplete 时的后续方向 |

约束：

- `summary` 与 `summaryDetail` 都属于 Result 本体层
- 它们可以引用 evidence / artifact，但不替代 evidence ledger
- `summaryDetail` 可较长，但仍应保持“结果解释”而不是原始 payload dump

## Artifact Manifest Classification

artifact manifest 至少应稳定区分：

| 分类 | 当前方向 |
| --- | --- |
| deliverable manifest | 最终交付物索引、面向用户交付 |
| evidence manifest | 运行证据、验证附件、诊断附件 |
| mixed summary manifest | 同时列出 deliverable 与 evidence，但必须显式分组 |

约束：

- deliverable artifact 优先挂 `resultId`
- evidence artifact 可挂 `runId` / `stepId`
- manifest 分类必须能告诉产品层“哪些可以默认展示为最终交付，哪些只应作为证据附件”

## Profile Boundary

artifact manifest 的最终样式可以被 profile / template 细化，但 evidence/result flow
层至少保持以下边界：

- core / flow 层只区分 deliverable vs evidence vs mixed grouping
- profile / capability 层才细化：
  - 软件开发的 diff / build / test 附件
  - 文档生成的正文 / 索引 / 摘要
  - 数据分析的报告 / 数据片段 / 图表

也就是说：

- flow 层定义“分类与关闭关系”
- profile 层定义“具体交付长相”

## Patch Target And Closeout Exceptions

当 state-changing artifact 与当前 target snapshot、coverage target 或 closeout 发生冲突时，
evidence/result flow 必须保持以下例外处理：

- patch target 例外
  - 缺少 concrete target snapshot / targetRef / contentHash 时，不得直接进入 accepted closeout
  - `coverageTargets` 缺失或与当前 scope 不匹配时，只能进入 repair / schema recovery / evidence acquisition
- closeout 例外
  - provider retry、schema invalid、wrapper failure、旧失败 step 只能保留为 lineage / diagnostics
  - 它们不得被提升为最新 accepted closure evidence
- latest evidence priority
  - 若旧失败 evidence 之后已有成功 validation / patch / action evidence，closeout 必须优先使用新的 non-superseded evidence

该规则只表达 closeout 优先级和 evidence 边界，不表达具体业务修复方案。

## Key Flow Guarantees

- raw payload 默认通过 ref 暴露，而不是直接塞进主投影
- 证据与正式业务结果必须分层
- accepted / rejected closeout 都必须显式依赖 evidence quality 与 coverage / alignment 结果
- 历史失败证据保留为诊断和 lineage，不应覆盖最新 accepted closure
- 若存在 accepted Result，workspace/run outcome 投影必须优先引用该 Result 的 evidence closure
- public API 不得暴露本地绝对路径

## Source Documents

- `../90-archive/completed/result-event-development-plan.md`
- `../90-archive/superseded/workspace-data-layout-plan.md`
- `../70-roadmap/active/run-closure-quality-improvement-plan.md`
- `../00-core/runtime-stage-contract.md`
