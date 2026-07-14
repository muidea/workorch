# Acceptance Record: Runtime Flow

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件记录对 `runtime-flow-code-alignment-checklist.md` 的当前正式验收结果。

判定口径：

- `pass`
  - 当前代码与 flow 设计一致，分流、时序和继承链成立
- `partial`
  - 主体合同已经落地，但仍有明确的后续结构化收口项
- `doc-ahead`
  - 文档要求已明确，但当前证据不足以证明实现已经完全落地
- `impl-drift`
  - 当前实现与 flow 设计冲突

## Evidence Base

本轮主要检查以下实现与测试证据：

- `internal/modules/application/workorchd/biz/appservice/workspace_execution_activation_service.go`
- `internal/modules/kernel/run/biz/run_service.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/execution_context_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_graph.go`
- `internal/modules/kernel/orchestrator/biz/runtime_orchestrator.go`
- `internal/modules/kernel/orchestrator/biz/runtime_output_mode_selector.go`
- `internal/modules/blocks/llm/biz/provider_context_budget.go`
- `internal/modules/blocks/llm/biz/prompt_messages.go`
- `internal/modules/application/workorchd/biz/appservice/read_model_service.go`
- `internal/pkg/repository/file/file.go`
- 相关 tests / verifier：
  - `runtime_coverage_work_items_test.go`
  - `runtime_context_integrity_gate_test.go`
  - `runtime_output_mode_selector_test.go`
  - `runtime_artifact_assemble_test.go`
  - `runtime_orchestrator_test.go`
  - `provider_context_budget_test.go`
  - `execution_context_package_test.go`
  - `runtime_verify_script_test.go`

## Acceptance Result

| Area | Evidence | Result | Notes |
| --- | --- | --- | --- |
| activation / definition | activation service, run service, run/workspace models | pass | activation 基于 effective Goal / DoD 创建新 run，写入 definition revision，stale active run 会被 supersede，首个正式入口为 `llm/workspace_execution`。 |
| activation coverage baseline | `runtime_coverage_work_items.go`, `runtime_step_input.go`, `workorchruntimecontract.CoverageWorkItem` | pass | initial/dispatch 前会 `ensureRunCoverageWorkItems` 并形成 `coverageWorkItemContext`；`CoverageWorkItem` 正式 model/repository 已落地，orchestrator 写路径已同步到 owner repository，read-model 列表已通过 EventHub owner command 读取 owner repository；dispatch/provider context 前会从 owner repository 水合 `Run.ExecutionContext` coverage 投影并生成 gap baseline。 |
| planning / execution | `runtime_graph.go`, `runtime_orchestrator.go`, `runtime_output_mode_selector.go` | pass | graph 调度由 orchestrator 持有；graph exhausted 后进入 follow-up、assessment-only 或 completion gate；`incrementalArtifact` 走正式 output-mode 主链。 |
| context assembly | `execution_context_package.go`, `runtime_step_input.go`, `prompt_messages.go` | pass | 每次 provider request 前重新组装 context package，覆盖 contract、continuity、failure、progress、coverage、snapshots、capabilities、memory、budget。 |
| context gate | `provider_context_budget.go`, `runtime_adapter.go`, `runtime_repair.go`, tests | pass | provider 前置 gate 真实存在，稳定实现 `pass` / `read_required` / `degrade_to_assessment` / `blocker_required` / `fail_fast` / `replan_required`；非 pass action 会阻断 provider 调用，并由 orchestrator 形成结构化 context decision ticket。 |
| canonical failure hydration | `runtime_orchestrator.go`, `runtime_repair.go`, `runtime_llm_context.go` | pass | 可从 ticket context、failed input/output、repair package、progress digest、replan/rejected continuation 重建 current failure/progress，避免单字段缺失误判。 |
| repair / replan / governance recovery | `runtime_repair.go`, `runtime_orchestrator.go`, tests | pass | wrapper failure inheritance 成立；context-gate failure 不进入普通 state-changing repair；decision/approval resolved 后创建新的 retry/continuation step。 |
| verification / acceptance | `runtime_graph.go`, `runtime_evidence_quality.go`, `runtime_result_recording.go` | pass | completion gate 会 normalize/validate DoD assessment、校验证据质量、更新 coverage ledger，并在 coverage ready 后记录 accepted/rejected Result。 |
| evidence / result / projection | `read_model_service.go`, event repository, result recording | pass | evidence 来源分层成立；accepted Result 在 outcome/taskProgress 中优先于旧失败 evidence；payload summary/ref 与 raw open 边界成立。 |
| cross-flow consistency | coverage ledger + context package + read-model projection | pass | definition revision、coverage baseline、context package、context gate、accepted outcome 主链一致；coverage ledger 写路径已独立持久化，read-model 列表已从 owner repository 读取；dispatch/provider context 前会从 owner repository 水合 cache-stable coverage projection。 |

## Requirement Notes

### 1. Activation / Definition

已验收：

- activation 不直接推进旧 run 历史，而是基于当前 effective definition 创建或替换 run。
- definition revision 进入 run 本体和后续 context。
- successor/replan path 保留 lineage，但不机械复制旧 active state。

### 2. Planning / Execution

已验收：

- planning/execution graph 由 orchestrator 调度，不由 block adapter 拥有。
- 只读 graph 不能直接替代 accepted closeout。
- graph exhausted 后有 follow-up、assessment-only、completion gate 等收口路径。
- incremental artifact 通过 selector、plan/context/chunk/apply/validation 链路进入主状态机。

### 3. Context Assembly / Gate

已验收：

- provider request 前存在 context assembly，不复用旧 step input。
- context package 包含合同、连续性、失败、进度、coverage、capability、memory、budget 等主要分区。
- provider budget/gate 会输出 contextGateAction/contextGateReason，并覆盖 pass/read-required/degrade-to-assessment/blocker-required/fail-fast/replan-required。
- coverage gap 已存在时可恢复 coverage target/focus 上下文。

保留项：

- flow 文档中的完整 gate action 枚举已落地为 provider budget action；LLM adapter 只做 provider 前阻断，orchestrator 负责恢复 ticket / retry routing。
- 后续落地时应保持 LLM adapter 只负责 gate decision/budget，orchestrator 负责恢复路径。

### 4. Repair / Replan / Governance

已验收：

- provider retry、schema invalid、repair-of-repair 保持 wrapper recovery inheritance。
- `llm_context_integrity_failed` 被识别为 provider 前置失败，不进入普通 state-changing repair。
- replan successor 继承 predecessor failure context 和 process evidence，但不继承旧 active state。
- Decision / Approval / auto-governance 只选择恢复路径，不替代 Run / Result 本体。

### 5. Verification / Acceptance

已验收：

- verification/acceptance 基于当前 effective Goal / DoD 与 current-run evidence refs。
- assessment-only correction 存在，且不会直接强制新的 state-changing patch。
- weak/insufficient evidence 不会生成 accepted closeout。
- coverage ledger 被 completion gate 消费。

### 6. Evidence / Result / Projection

已验收：

- Step、ToolCall、Patch、ValidationResult、RuntimeCheckpoint、EventRecord、MemoryRecallTrace 都可作为过程证据来源。
- accepted Result 依赖正式 DoD assessment 和 evidence refs。
- raw payload 默认走 ref-only，不混进 summary 投影。
- workspace/run outcome 优先引用最新 accepted Result，而不是旧 failed evidence。

## Summary

本轮结论：

- runtime-flow 主链已经完成代码对齐。
- 当前未发现需要立即修复的 `impl-drift`。
- 保留项集中在：
  - 当前无 `CoverageWorkItem` activation/read-model/provider dispatch 主路径缺口；`Run.ExecutionContext` coverage 字段保留为 provider-facing/cache-stable 投影。

总体结果：`pass`

## Verification Evidence

本轮收口使用以下证据确认当前结论：

- `make verify-boundary-contract`
  - 证明 runtime flow 收口没有通过业务特化逻辑破坏核心中立性。
- `go test ./internal/modules/kernel/orchestrator/biz -run 'Test(CoverageWorkItems|CoverageWorkItem|RuntimeOrchestrator_.*Coverage|ApplyOutputModeSelection|SelectOutputMode|HandleArtifactOutput|ValidateArtifact|BuildExecutionContextPackage|ProviderContext|RuntimeOrchestrator_Context|VerifyWorkorchRuntime)' -count=1`
  - 证明 activation coverage baseline、planning/execution、context package、context gate、artifact output mode、repair/recovery 与 verifier fixture 的核心路径仍通过。
- `go test ./internal/modules/kernel/orchestrator ./internal/modules/kernel/orchestrator/biz ./internal/modules/application/workorchd/biz/bootstrap ./internal/initiators/workorchruntime ./internal/pkg/repository ./internal/pkg/workorchrepo -count=1 -timeout=240s`
  - 证明 CoverageWorkItem owner storage 接线、orchestrator 主路径同步、purge evidence、bootstrap/framework initiator 与 repository backend 回归通过。
- `go test ./internal/modules/application/workorchd/biz/appservice/ownerreadports ./internal/modules/application/workorchd/biz/appservice/readmodelports ./internal/modules/application/workorchd/biz/appservice/readmodelprojection ./internal/modules/application/workorchd/biz/appservice ./internal/modules/kernel/orchestrator -count=1 -timeout=240s`
  - 证明 CoverageWorkItem read-model 列表通过 EventHub owner command 读取 owner repository 后，runtime snapshot 投影仍保持兼容的展示字段。
- `go test ./internal/modules/kernel/orchestrator/biz -run 'Test(CoverageWorkItemsSyncProjectionToRepository|EnsureGapBaselineHydratesCoverageContextFromOwnerRepository|CoverageWorkItemsGovernanceOwnerSyncRespectsRequiredCounts|NextDispatchSpecsPersistsGapBaselineBeforeInitialStep|InitialDispatchBuildsStateChangingContractFromGapBaseline)' -count=1 -timeout=120s`
  - 证明 dispatch/provider context 前会从 owner repository 水合 coverage projection，并据此生成 gap baseline、accepted/rejected evidence refs 与当前 todo/focus。
- `go test ./internal/pkg/repository ./internal/modules/application/workorchd/biz/appservice -run 'Test(ArtifactContinuation|MemoryRecallTrace|EventRecord|Decision|Approval|ReadRunSnapshotIncludesArtifactContinuation|ListDecision|ListApproval|ReadRunSnapshotWithBackendReturnsExecutionEvidence|ReadRunObservability|MapTaskProgress|MapResultQuality)' -count=1`
  - 证明 evidence/result/projection flow 与治理对象、event payload summary/ref、read-model 投影仍通过。
- `git diff --check`
  - 证明本轮文档与当前改动没有 whitespace/formatted diff 问题。

当前无需扩大 runtime 状态机；coverage owner storage 与 provider-facing 投影边界已经收口。
