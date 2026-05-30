# Acceptance Record: Incremental Artifact

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件记录对
`incremental-artifact-code-review-checklist.md`
的第一轮正式验收结果。

判定口径：

- `pass`
  - 当前代码与验证证据足以证明设计要求已经成立
- `doc-ahead`
  - 文档要求已明确，但当前证据不足以证明实现已经成立
- `impl-drift`
  - 当前实现与文档要求冲突

## Evidence Base

本轮主要检查以下实现与验证证据：

- `internal/modules/kernel/orchestrator/biz/runtime_output_mode_selector.go`
- `internal/modules/kernel/orchestrator/biz/runtime_artifact_assemble.go`
- `internal/modules/blocks/llm/biz/prompt_messages.go`
- `internal/modules/blocks/llm/biz/runtime_adapter.go`
- `internal/modules/application/workorchd/biz/appservice/read_model_service.go`
- `internal/modules/application/workorch/service/tui/*`
- `internal/pkg/repository/*/artifact_continuation_repository.go`
- `internal/pkg/repository/repository_contract_test.go`
- `scripts/verify-incremental-artifact-e2e.sh`
- 相关单测：
  - `runtime_output_mode_selector_test.go`
  - `runtime_artifact_assemble_test.go`
  - `read_model_service_test.go`
  - `file_test.go`
  - `model_test.go`

## Acceptance Result

| Area | Evidence | Result | Notes |
| --- | --- | --- | --- |
| core contract | `runtime_output_mode_selector.go`, `prompt_messages.go`, `runtime_adapter.go` | pass | 支持 `executionGraph` / `incrementalArtifact` 双模式；`outputMode` 投影到 step input；response 顶层 schema 互斥 |
| orchestrator / runtime flow | `runtime_output_mode_selector.go`, `runtime_artifact_assemble.go`, `runtime_artifact_assemble_test.go` | pass | 存在 selector -> plan -> chunk -> assemble -> apply -> validation/acceptance 主链；`artifact_context` 恢复链存在 |
| provider assembly | `prompt_messages.go`, `runtime_adapter.go`, `runtime_output_mode_selector.go` | pass | `outputModeSelector`、`artifactOutputContract`、`artifactContinuationContext` 进入 provider-facing stage contract；模式回落时清理 incremental-only 字段 |
| persistence | `artifact_continuation_repository.go`, `repository_contract_test.go`, `file_test.go` | pass | `ArtifactContinuation` 有正式 repository contract；plan/chunk/assembled metadata+content 持久化；reload/restart 合同存在 |
| read-model | `read_model_service.go`, `read_model_service_test.go` | pass | `artifactContinuations` 投影为 metadata-first；默认不暴露正文内容 |
| service API | `read_model_types.go`, `service-api.md` 对应 appservice 投影证据 | pass | snapshot/workspace 投影暴露 `artifactContinuations` 元数据；raw 内容通过独立打开路径 |
| Web / TUI | `service/tui/view.go`, `service/tui/model_test.go`, console built asset evidence | pass | TUI 展示 progress/summary；Web 页面默认展示 metadata/进度摘要，而非 chunk/assembled 正文 |
| evidence / acceptance | `runtime_artifact_assemble.go`, `verify-incremental-artifact-e2e.sh` | pass | apply 生成正式 state-changing evidence；e2e 断言 accepted Result / Artifact 与 lifecycle 可观测 |
| failure recovery | `runtime_artifact_assemble.go`, `runtime_artifact_assemble_test.go` | pass | hash mismatch、missing final chunk、sequence gap、context graph 后续恢复、terminal continuation 回落等路径均有处理 |
| verifier / tests | `verify-incremental-artifact-e2e.sh` + 多组单测 | pass | selector、continuation lifecycle、metadata-only projection、raw payload leakage guard 均有直接测试或脚本证据 |

## Requirement Notes

### 1. Output mode selector

证据：

- `applyLLMOutputModeSelection(...)`
- `selectOutputMode(...)`
- `artifactContinuationContextTerminal(...)`
- `clearIncrementalArtifactSelection(...)`

结论：

- 已证明 `outputMode` 不只是内部变量，而会写回 step input
- 已证明 selector 具备：
  - open continuation 恢复
  - schema invalid large preview
  - graph budget 超限
  - requested mode + budget signal
- 已证明 terminal continuation 会回落到 `executionGraph`

### 2. Mutual-exclusive mode contract

证据：

- `artifactPlanRequiredOutputSchema()`
- `artifactChunkRequiredOutputSchema()`
- `artifactContextRequiredOutputSchema()`
- `runtime_adapter.go` 的 artifactPlan/artifactChunk/artifactChunkRepair 解析

结论：

- 已证明 mode-specific schema 在 request-side 被显式表达
- 已证明 response 顶层允许字段包含 artifactPlan/artifactChunk/artifactChunkRepair
- 文档要求的“response 顶层不得返回 outputMode”在实现设计和 verifier 中成立

### 3. Continuation persistence and recovery

证据：

- file/memory artifact continuation repository
- repository contract tests
- `assembleArtifactContinuation(...)`
- `resumeArtifactContinuationAfterContextGraph(...)`
- `TestArtifactContinuationRecoverySpecsDispatchByState`

结论：

- 已证明 continuation 是正式 run-scoped internal state
- 已证明 plan/chunk/assembled 均有 persistence 接口
- 已证明服务恢复后不会要求重生成已确认 chunk

### 4. Metadata-first projection

证据：

- `projectArtifactContinuation(...)`
- `TestReadRunSnapshotIncludesArtifactContinuationMetadata`
- TUI `TestModelRendersArtifactContinuationProgress`
- `verify-incremental-artifact-e2e.sh` 中 raw payload leakage guard

结论：

- 已证明 snapshot/read-model 投影默认只暴露 metadata
- 已证明 TUI 只展示 progress/summary
- 已证明脚本显式检查 raw payload leakage

### 5. Apply / validation / acceptance

证据：

- `TestApplyAssembledArtifactContinuationWritesDeliverableAndEvidence`
- `verify-incremental-artifact-e2e.sh`
- `runtime_artifact_assemble.go` 中 apply / fail / publish 路径

结论：

- 已证明 assembled artifact 会进入 apply
- 已证明 lifecycle 与 accepted Result / linked Artifact 形成闭环
- 当前证据足以支持“chunk 不直接等于 accepted evidence”的设计要求

## Summary

本轮结论：

- 当前 `incrementalArtifact` 专题的设计要求已有较强代码与脚本证据支撑。
- 本轮未发现明显 `impl-drift`。
- 也未发现必须保留为 `doc-ahead` 的关键未落地点。

因此该清单当前可判定为：

- 总体结果：`pass`
