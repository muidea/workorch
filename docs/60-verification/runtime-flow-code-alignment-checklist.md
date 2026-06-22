# Runtime Flow Code Alignment Checklist

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件用于核对 `20-runtime-flow/` 当前运行链路设计是否已经被代码正确承接。

它不重新定义 flow，只把当前需要逐项核对的实现对齐项拆成可执行清单，
便于后续边看 `30-module-architecture/` 边做定点设计-实现核查。

## Canonical Sources

核对时以以下文档为准：

- `../20-runtime-flow/activation-definition-flow.md`
- `../20-runtime-flow/planning-execution-flow.md`
- `../20-runtime-flow/context-assembly-flow.md`
- `../20-runtime-flow/repair-replan-flow.md`
- `../20-runtime-flow/verification-acceptance-flow.md`
- `../20-runtime-flow/evidence-result-flow.md`
- `../00-core/runtime-stage-contract.md`
- `../30-module-architecture/overview.md`
- `../50-product-interface/service-api.md`

## Review Goal

这一轮只回答三个问题：

1. flow 文档中的关键合同是否已经有明确代码落点。
2. 实现分流、先后顺序和对象继承是否与 flow 文档一致。
3. 哪些点属于“文档超前”，哪些点属于“实现偏差”。

## Suggested Review Scope

建议优先核对以下代码目录：

- `internal/modules/kernel/orchestrator`
- `internal/modules/blocks/llm`
- `internal/modules/application/workorchd`
- `internal/pkg/repository`

## Review Order

建议顺序：

1. activation / definition
2. planning / execution
3. context assembly / gate
4. repair / replan / governance recovery
5. verification / acceptance
6. evidence / result / projection

## 1. Activation / Definition Flow

重点核对：

- activation 是否基于当前 effective Goal / DoD，而不是旧 run 历史直接继续。
- activation 是否会检查当前 `definitionRevision`。
- activation 是否形成新的 Run，而不是直接推进 graph / tool / patch。
- activation 后首个正式 step 是否为 `llm/workspace_execution` 或等价正式入口。
- activation 是否形成首轮 coverage baseline / 初始 work item scope。
- replan successor / governance continuation / definition change 后的新 run，
  是否只继承仍有效的 contract/context/lineage。

常见偏差：

- activation 不处理 definition revision
- activation 后没有 coverage/work item 初始面
- successor run 机械继承旧 run 的 active coverage / failure focus

## 2. Planning / Execution Flow

重点核对：

- planning 是否只负责拆解可执行单元，不直接形成 accepted Result。
- execution 是否只产生真实状态变更或明确 blocker，不把只读 graph 当作完成。
- graph 调度是否由 orchestrator 持有，而不是 block adapter 持有。
- graph exhausted 后是否会进入 follow-up / verification / acceptance，而不是无限扩图。
- patch preview / apply / revert 是否职责分离。
- file / git / web capability 是否只作为 execution evidence，不直接拥有 closeout 语义。
- `incrementalArtifact` 是否按 flow 文档主链执行，而不是旁路处理。

常见偏差：

- execution 允许无限只读 graph 替代 state-changing output
- graph exhausted 后直接进入无意义失败或无界 follow-up
- patch preview / apply 语义混杂

## 3. Context Assembly / Gate

重点核对：

- 每次 provider request 前是否真的执行 context assembly，而不是复用旧 step input。
- context bundle 是否至少覆盖：
  - contract
  - continuity
  - failure
  - progress
  - coverage
  - snapshots
  - capabilities
  - memory
  - budget
- context gate 是否存在，并支持：
  - `pass`
  - `read_required`
  - `degrade_to_assessment`
  - `blocker_required`
  - `fail_fast`
  - `replan_required`
- canonical failure hydration 是否真实存在，而不是因为单字段缺失就误判“当前失败缺失”。
- `coverageTargetSet` 缺失但已有 gap 事实时，是否能 auto-rebuild。
- `selectedContext` / memory recall 是否留下正式 trace，而不是只留 raw executionContext。

常见偏差：

- 没有真正的 context gate，只是 prompt 拼装
- `currentFailure` 缺一层就直接创建 blocker
- coverage gap 已存在，但没有 auto-rebuild `coverageTargetSet`

## 4. Repair / Replan / Governance Recovery

重点核对：

- failure 是否先结构化分类，再进入恢复链。
- provider retry、schema invalid、repair-of-repair 是否保持 wrapper recovery inheritance。
- wrapper/schema/mode contract failure 后，`currentFailure` 与 `structuredFailureSignals` 是否仍锚定 primary actionable failure，而不是被最新 wrapper 文本覆盖。
- replan successor 同时带有 rejected result continuation 时，是否优先恢复 predecessor progress digest 中未关闭的 concrete actionable failure，而不是让 coverage / deliverable gap 覆盖当前失败面。
- context-gate failure 是否不会落入普通 state-changing repair。
- repair exhausted / duplicate failure / provider/config gate 是否升级到 decision / approval / replan。
- `Decision` / `Approval` / `auto-governance` 是否只选择恢复路径，不替代 Run / Result 本体。
- `runtime_context_required` resolved 后，是否创建新的 context retry step，而不是直接重放旧失败 step。
- replan successor 是否继承 predecessor failure context，但不机械复制旧 active state。

常见偏差：

- wrapper failure 覆盖原 failure surface
- wrapper failure 的 error/reason 覆盖 primary actionable failure reason
- rejected result continuation 抢占 replan 中仍未关闭的 build/tool/patch concrete failure
- context gate failure 进入普通 repair
- decision resolved 后重放旧 failed step

## 5. Verification / Acceptance Flow

重点核对：

- verification 是否只收集/判定证据，不继续承担实现主动作。
- acceptance 是否基于当前 effective Goal / DoD + evidence refs 生成正式 accepted/rejected Result。
- failed assessment 是否进入 action routing，而不是停在 no-actionable continuation。
- assessment-only correction 是否存在，并且不会直接强制新的 state-changing patch。
- current-run evidence gate 是否优先于历史 summary / superseded evidence / memory-only ref。
- graph follow-up 达到上限后，是否转 assessment-only，而不是继续自由扩图。
- `coverageWorkItems` 是否作为 verification / acceptance 的 ledger 投影工作。
- claim-to-evidence alignment 是否有真实实现落点。

常见偏差：

- assessment 字段不完整就直接强制 patch
- accepted 结果引用历史或 memory-only evidence
- graph follow-up 无限继续

## 6. Evidence / Result / Projection Flow

重点核对：

- evidence 是否来源于：
  - Step
  - ToolCall
  - Patch
  - ValidationResult
  - RuntimeCheckpoint
  - EventRecord
  - MemoryRecallTrace
- accepted Result 是否依赖正式 `dodAssessment` 和 accepted closure evidence。
- deliverable artifact 与 process evidence artifact 是否在 ownership 上被区分。
- event payload 是否支持：
  - `payload`
  - `payloadSummary`
  - `payloadRef`
- raw payload 是否默认走 ref-only，而不是混进 summary。
- workspace/run outcome 是否优先引用最新 accepted Result，而不是旧失败 evidence。

常见偏差：

- raw payload 默认混入主投影
- artifact ownership 没有 deliverable / evidence 分层
- latest accepted closure 没有压过历史失败 evidence

## 7. Cross-Flow Consistency Checks

重点核对以下跨文档一致性：

- activation 形成的 definition revision / coverage baseline，是否被 verification / acceptance 正确消费。
- context gate 的 `read_required` / `blocker_required` / `replan_required`，
  是否在 repair/replan 中有对应恢复路径。
- repair/replan 继承的 `processEvidenceTimeline` / `currentProgressDigest`，
  是否在 context assembly 中被保留。
- verification/acceptance 的 accepted Result 优先级，
  是否在 evidence-result / outcome projection 中一致成立。

常见偏差：

- activation 建了 baseline，但 verification 不用
- context gate 有动作定义，但 repair/replan 不识别
- accepted Result 在 closeout 成立，但 outcome projection 仍回退到旧失败 run

## Classification Rules

核对结果建议分三类：

- `pass`
  - 设计与代码一致，分流和时序都成立
- `doc-ahead`
  - 文档定义已明确，但代码尚未正式落地
- `impl-drift`
  - 代码已有实现，但顺序、继承链或分流语义与当前设计冲突

## High-Priority Checks

优先检查这些高风险点：

1. activation 是否真正形成 coverage baseline / 初始 work item scope
2. context gate 是否真实存在，而不是只有 prompt 侧说明
3. canonical failure hydration 是否能避免误判“当前失败缺失”
4. context-gate failure 是否不会进入普通 state-changing repair
5. accepted Result 是否真的压过历史 failed step / failed run
6. event payload 是否已经形成 `payloadSummary + payloadRef` 分层

## 2026-05-30 Code Alignment Result

| Area | Code Path | Result | Notes |
| --- | --- | --- | --- |
| activation / definition | `internal/modules/application/workorchd/biz/appservice/workspace_execution_activation_service.go`, `internal/modules/kernel/run/biz/run_service.go` | pass | activation 会基于 effective Goal / DoD 构造新 run，写入 `definitionRevision`，active run stale 时 supersede 旧 run 并创建 successor run；首个入口为 `llm/workspace_execution`。 |
| activation coverage baseline | `internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items.go`, `runtime_step_input.go`, `internal/pkg/workorchruntimecontract/coverage_work_item.go` | pass | `ensureRunCoverageWorkItems` 会从当前 Goal / DoD / profile contract 生成 run-scoped coverage ledger，并在 initial/dispatch 前注入 `coverageWorkItemContext`；正式 `CoverageWorkItem` model/repository 已落地，orchestrator 写路径已同步到独立 owner repository，read-model 列表已通过 EventHub owner command 读取 owner repository；dispatch/provider context 前会从 owner repository 水合 `Run.ExecutionContext` coverage 投影并生成 gap baseline。 |
| planning / execution | `internal/modules/kernel/orchestrator/biz/runtime_graph.go`, `runtime_orchestrator.go`, `runtime_output_mode_selector.go` | pass | graph 调度由 orchestrator 持有；graph exhausted 后进入 completion follow-up、assessment-only 或 completion gate；incremental artifact 通过 output-mode contract 进入 plan/context/chunk/apply 链路，没有旁路直接改状态。 |
| context assembly | `internal/modules/kernel/orchestrator/biz/execution_context_package.go`, `runtime_step_input.go`, `internal/modules/blocks/llm/biz/prompt_messages.go` | pass | 每次 LLM step 执行前会重新生成 `executionContextPackage`，覆盖 contract、continuity、progress、coverage、capability、memory、budget 所需投影，并记录 memory recall trace。 |
| context gate | `internal/modules/blocks/llm/biz/provider_context_budget.go`, `runtime_adapter.go`, `runtime_repair.go` | pass | provider 前置 gate 真实存在，`validateProviderContextGate` 会阻断 provider 调用；budget 层已输出 `pass` / `read_required` / `degrade_to_assessment` / `blocker_required` / `fail_fast` / `replan_required`。LLM adapter 不做业务恢复，orchestrator 将非 pass gate 转为结构化 context decision ticket，并写入 `recommendedRecoveryAction` 与收窄后的 `allowedRecoveryActions`。 |
| canonical failure hydration | `internal/modules/kernel/orchestrator/biz/runtime_orchestrator.go`, `runtime_repair.go`, `runtime_llm_context.go` | pass | context-gate retry 会从 ticket context、failed input/output、repair package、progress digest、replan/rejected continuation hydrate `currentFailure` 与 `currentExecutionProgress`，避免单字段缺失就误判“当前失败缺失”。 |
| context-gate recovery | `internal/modules/kernel/orchestrator/biz/runtime_repair.go`, `runtime_orchestrator.go` | pass | `llm_context_integrity_failed` 被识别为 provider 前置失败；除 coverage target 可自动重建场景外，不进入普通 state-changing repair，而是创建 context decision 或新的 context retry step。 |
| repair / replan inheritance | `internal/modules/kernel/orchestrator/biz/runtime_orchestrator.go`, `runtime_step_input.go`, `runtime_llm_context.go` | pass | replan successor 会继承 predecessor failure context、process evidence 和仍有效 contract，同时去除旧 active state；rejected-result continuation 优先于历史 replan continuation。 |
| scheduler continuation diagnostics | `internal/modules/application/workorchd/service/server/workorchd_scheduler_commands.go`, `http_run_continuation_scheduler.go`, `scheduler_diagnostics.go`, `scripts/observe-workorch-runtime-closure.sh`, `scripts/verify-real-runtime-closure.sh` | pass | workorchd scheduler command 只有在 payload/scope 校验并成功提交 `BackgroundRoutine` 后才返回 accepted；missing response、not accepted、enqueue failure 会同步失败并记录 metrics/diagnostic。周期 watchdog 只恢复 stale dispatched runtime gap，不创建 replan continuation。观察脚本会在治理 resolution 出现时要求 scheduler accepted/error metric 或 continuation diagnostic，避免 hidden watchdog compensation。 |
| verification / acceptance | `internal/modules/kernel/orchestrator/biz/runtime_graph.go`, `runtime_evidence_quality.go`, `runtime_result_recording.go`, `runtime_completion.go` | pass | completion gate 会 normalize/validate DoD assessment、校验证据质量、更新 coverage ledger，并在 `validateRunCoverageWorkItemsReadyForAcceptance` 通过后才记录 accepted Result；direct `CompleteRun` 对 DoD-gated LLM run 也要求存在匹配 workspace/run/Goal/DoD、`SourceType=dod_assessment`、`DoDAssessment.Satisfied=true` 且带 evidenceRefs 的 accepted Result。 |
| evidence / result / projection | `internal/modules/application/workorchd/biz/appservice/read_model_service.go`, `internal/pkg/repository/file/file.go`, `internal/modules/kernel/eventstream/pkg/models/event_record.go` | pass | accepted Result 在 run/workspace outcome 与 task progress 投影中优先于历史 failed step；EventRecord 已支持 `payload` / `payloadSummary` / `payloadRef`，file backend 默认将 raw payload 写 ref 并保留 summary。 |
| cross-flow consistency | `internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items.go`, `execution_context_package.go`, `read_model_service.go` | pass | definition revision、coverage baseline、context package、context gate、accepted outcome 的主链一致；`CoverageWorkItem` 已完成正式 model/repository 对象化、orchestrator 写路径同步、read-model owner command 读路径和 dispatch/provider context 前 owner hydration。 |

### Remaining Follow-Up

- `CoverageWorkItem` 独立 model/repository 已完成落地，orchestrator 写路径已同步 owner storage，read-model 和 dispatch/provider context 前 hydration 均已读取 owner repository；后续修改 provider-facing projection 前应保持现有 `Run.ExecutionContext.coverageWorkItems` 的稳定顺序与 provider cache 前缀规则。
- context gate 的 `degrade_to_assessment`、`blocker_required`、`replan_required` 已作为 provider budget action 输出；budget 模块只判定 gate action，LLM adapter 只执行 provider 前阻断，orchestrator 负责结构化 decision/retry 恢复路径。
- 本轮未发现必须立即修复的 `impl-drift`；`CoverageWorkItem` owner storage / read-model owner command / dispatch hydration 主链已闭合。后续差异应按新增审计证据逐项记录，避免继续沿用已失效的前置缺口结论。

## Suggested Output Template

后续逐项核对时，建议按以下格式记录：

| Area | Code Path | Result | Notes |
| --- | --- | --- | --- |
| activation / definition | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| planning / execution | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| context assembly / gate | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| repair / replan | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| verification / acceptance | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| evidence / result / projection | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| cross-flow consistency | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
