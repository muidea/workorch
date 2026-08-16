# Runtime Ledger Authority Dispatch Hardening Final Closure Plan

Status: implemented

Type: closure-plan

Last Updated: 2026-07-07

Related:

- `../superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-validation-failure-diagnostic-repair-dispatch-closure-plan.md`
- `runtime-authority-derived-snapshot-hardening-closure-plan.md`
- `runtime-validation-required-output-authority-closure-plan.md`

## Purpose

本文用于收口当前 live 验证中仍然存在的主链路偏差：

```text
validation failed diagnostic
  -> rejected evidence record
  -> coverage ledger recompute
  -> convergenceAuthority.nextAction
  -> convergenceContract
  -> stagePacket
  -> requiredOutput mirrors
  -> repair dispatch preflight
```

目标不是继续对单个 live workspace 做局部补丁，而是按主设计固化唯一权威路径：

- `coverage/evidence ledger` 是状态权威。
- `NextAction` 只能由 runtime deterministic 推导。
- `convergenceContract`、`stagePacket`、`repairActionPackage.requiredOutput`、top-level `requiredOutput`、`currentActionContract.requiredOutput` 必须来自同一次 authority snapshot。
- old failed-step input、old `repairActionPackage`、old `stagePacket`、old `requiredOutput`、old `currentActionContract` 只能作为 diagnostic lineage，不能反向决定当前合同。

当前阶段不考虑兼容性。若现有实现、测试或端侧行为与本文冲突，应按本文逐项收口。

## Contract Audit

- Scope:
  - `ws-705249a8d7816330/run-092-replan-001`
  - `ws-db040d24a190f6e5/run-094`
  - `internal/modules/kernel/orchestrator/biz/runtime_coverage_evidence.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_coverage_contract.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_current_action_contract.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`

- Observed state:
  - `ws-705249a8d7816330/run-092-replan-001` 的最后 validation step 输入合同正确：`nextAction=validate`、`convergenceContract=need_validation`、`requiredOutput.acceptedKinds=["shell/command","validation/managed_session"]`。
  - 同一 validation step 失败后，validation evidence record 明确绑定 `cwi-088`，但 coverage repository 中 `cwi-001` 也被写入同一 rejected refs，最终 run authority 转向 `cwi-001 validate`。
  - `ws-db040d24a190f6e5/run-094` 最终 run execution context 已对齐为 `state_change + need_state_change + state-changing requiredOutput`，但 repair dispatch 中间态仍失败于 `contractState=need_state_change requires state-changing requiredOutput`。
  - 现有 targeted test 可通过，但没有覆盖上述 live 失败形态。

- Expected state:
  - failed validation evidence 必须只绑定当前 step 的 exact coverage focus。
  - 若 failed step 已声明 `coverageWorkItemIds`，rejected evidence 不得再通过 `coverageTargetSet`、`currentFocusItems`、`ledgerSummary` 扩散到其他 coverage item。
  - coverage ledger recompute 后必须产生唯一 current focus 和唯一 NextAction。
  - dispatch 前必须用同一次 current authority snapshot 原子渲染 `convergenceContract`、`stagePacket` 和所有 requiredOutput mirrors。
  - authority render 失败后必须 fail-fast，不能返回混合态 input 继续 dispatch。

- Deviation:
  - Evidence binding 仍把候选 coverage target set 当作写账匹配依据。
  - RequiredOutput seed 仍允许旧 mirrors 参与当前合同形状选择。
  - Repair finalization 在 authority refresh 失败后仍存在局部 fallback，可能把应 fail-fast 的内部合同偏差转成下一轮混合态。

- Owner:
  - WorkOrch core runtime。
  - 不属于 LLM/provider/template/Soul 特判问题。

- Event path:
  - runtime validation step failed
  - `recordValidationEvidenceRecordForStep`
  - `coverageSubsystem.updateFromFailedStep`
  - `coverageWorkItemContextFromItems`
  - `refreshRunConvergenceAuthority`
  - `runtimeRepairFollowUpStepInput`
  - `refreshCurrentAuthorityDerivedSnapshot`
  - `validateConvergenceDispatchInput`

- Contract gap:
  - 缺少 evidence exact binding 和 candidate target selection 的硬边界。
  - 缺少 canonical current authority renderer 作为 repair dispatch 唯一入口。
  - 缺少“旧 requiredOutput 只读诊断、不可作为 seed”的硬约束。
  - 缺少 repair finalization 失败后的 fail-fast 语义。

- Proposed change:
  - 先补失败样本测试，再逐项收口 P0。
  - 禁止在同一补丁里同时调整 evidence binding、requiredOutput seed、repair dispatch 和 projection。

- Boundary check:
  - 本计划只使用 coverage item id、structured target refs、evidence kind、contract state、NextAction 等通用结构化字段。
  - 禁止加入 workspace id、template id、Soul id、语言、文件名、命令、provider、错误文本 marker 特判。

- Verification:
  - 新增失败样本单测必须先红灯。
  - P0 收口后 targeted tests 与 orchestrator package tests 必须通过。
  - live 验证前必须证明旧失败路径不再生成 mixed requiredOutput mirrors。

## Root Cause

当前反复收口仍未彻底解决，是因为代码实现仍允许三类派生数据参与当前权威判断：

1. Candidate target context 被用于 evidence 写账。
   - `coverageTargetSet` 是 LLM/repair 的候选上下文，不是 evidence ledger 的 exact binding。
   - 当前 `coverageWorkItemMatchesUpdate` 会同时消费 `WorkItemIDs` 与 `TargetTokens`，当 `TargetTokens` 来自宽泛候选上下文时，会把 rejected evidence 写到非当前 focus。

2. Old requiredOutput mirrors 被用于 current requiredOutput seed。
   - `currentActionRequiredOutputSeed` 仍读取旧 `repairActionPackage.requiredOutput`、旧 `stagePacket.requiredOutput`、旧 top-level `requiredOutput`、旧 `currentActionContract.requiredOutput`。
   - 这些字段按设计只能作为 diagnostic lineage。

3. Repair dispatch finalization 失败后仍能继续返回局部修正 input。
   - `finalizeRuntimeRepairFollowUpStepInput` 在 `refreshCurrentAuthorityDerivedSnapshot` 报错后追加 diagnostic 并继续返回 input。
   - 这会隐藏真正的 internal contract misalignment，并可能导致下一轮继续混合旧合同。

## Closure Principles

1. 先测试，后改代码。
   - 每个 P0 改动前必须有能复现当前偏差的红灯测试。

2. 一次只固化一个环节。
   - 禁止在同一轮同时改 evidence ledger、authority renderer、requiredOutput seed、dispatch gate、projection。

3. Dispatch gate 不放宽。
   - `validateConvergenceDispatchRequiredOutputSemantics` 当前拦截方向正确。
   - 修复重点是让 gate 前只流入 canonical current authority input。

4. Old lineage 不参与当前合同。
   - old failed-step input、old repair package、old requiredOutput、old currentActionContract、old stagePacket 只能进入 `diagnosticLineage` 或 `authorityDiagnostics`。

5. Core 保持业务中立。
   - 不能写入具体文件名、命令、语言、workspace、provider、template 或 Soul 分支。

## Phase 0: Failing Sample Tests

- [x] Add test: rejected validation evidence with exact `coverageWorkItemIds=["cwi-088"]` only updates `cwi-088`.
  - Target: `runtime_coverage_evidence.go`
  - Expected: `cwi-001` must not receive rejected refs from `cwi-088` validation failure.

- [x] Add test: validation failure on focus requiring `state_change` recomputes authority to state_change and renders all requiredOutput mirrors as state-changing.
  - Target: `runtime_repair_input.go`, `runtime_gap_baseline.go`
  - Expected:
    - `convergenceAuthority.nextAction.kind=state_change`
    - `convergenceContract.state=need_state_change`
    - `repairActionPackage.requiredOutput.mustIncludeStateChangingNode=true`
    - `stagePacket.requiredOutput.mustIncludeStateChangingNode=true`
    - `currentActionContract.requiredOutput.mustIncludeStateChangingNode=true`
    - top-level `requiredOutput.mustIncludeStateChangingNode=true`

- [x] Add test: stale validation-only mirrors cannot seed state_change requiredOutput.
  - Target: `runtime_current_action_contract.go`
  - Expected: old top-level/stage/currentAction/repair requiredOutput are ignored as seed when current authority requires state_change.

- [x] Add test: repair finalization fails fast on authority misalignment.
  - Target: `runtime_repair_input.go`
  - Expected: refresh/rebuild failure is surfaced as typed internal contract error or failed dispatch request, not returned as a mixed input.

## Phase 1: Evidence Exact Binding

- [x] Split coverage update matching into two explicit modes:
  - `exactEvidenceBinding`
  - `candidateTargetMatching`

- [x] For runtime step evidence writing, use exact mode.
  - If `coverageWorkItemIds` exists, only those ids can match.
  - If `structuredTargetRefs` exists, match by strict structured refs.
  - If neither exists, fallback must be conservative and diagnostic-only.

- [x] Stop using `coverageTargetSet` as evidence ledger binding input.
  - It may remain in LLM input and repair package.
  - It may not cause rejected evidence refs to be written to unrelated coverage items.

- [x] Ensure `runtimeStepCoverageScopeMetadata` distinguishes:
  - `evidenceBindingRefs`
  - `candidateTargetRefs`
  - `coverageTargetSet`

Acceptance:

- Failed validation on `cwi-088` does not mark `cwi-001` failed.
- Failed validation on `cwi-036` does not mark unrelated `cwi-001` or `cwi-039` failed.

## Phase 2: Canonical Authority Renderer

- [x] Introduce or consolidate one current-authority rendering path:

```text
RenderCurrentAuthorityDispatchInput(run, stepID, input, mode)
  -> load current coverage/evidence ledger
  -> derive current focus
  -> derive deterministic NextAction
  -> build convergenceContract
  -> build stagePacket
  -> build canonical requiredOutput
  -> copy canonical requiredOutput to mirrors
  -> validate all mirrors
```

- [x] The renderer may read old input only for:
  - `diagnosticLineage`
  - `latestRejectedDiagnostic`
  - `processEvidenceTimeline`
  - target snapshots already accepted by runtime

- [x] The renderer must not read old input for:
  - current contract state
  - current next action
  - current requiredOutput shape
  - current focus

Acceptance:

- Same authority snapshot generates aligned `gapBaseline`、`convergenceContract`、`stagePacket`、`requiredOutput` and `currentActionContract`。

## Phase 3: RequiredOutput Seed Demotion

- [x] Change `currentActionRequiredOutputSeed` semantics.
  - Allowed seed:
    - current `convergenceContract.repairActionPackage.requiredOutput` generated in the same render pass.
  - Diagnostic-only sources:
    - top-level `requiredOutput`
    - `repairActionPackage.requiredOutput`
    - `stagePacket.requiredOutput`
    - `stagePacket.nextAction.requiredOutput`
    - `currentActionContract.requiredOutput`
    - `executionContext.*.requiredOutput`

- [x] When state is `need_state_change`, requiredOutput must be generated directly from:
  - `convergenceContract.State`
  - current authority focus
  - capability/evidence policy
  - current target snapshot prerequisite

- [x] When state is `need_validation`, requiredOutput must be generated directly from:
  - `convergenceContract.State`
  - current authority focus
  - validation capability policy

Acceptance:

- A stale validation-only requiredOutput cannot survive a state_change authority render.
- A stale state-changing requiredOutput cannot survive a validation authority render.

## Phase 4: Repair Finalization Fail-Fast

- [x] Remove fallback behavior in `finalizeRuntimeRepairFollowUpStepInput`.
  - If `refreshCurrentAuthorityDerivedSnapshot` fails, return a typed internal contract error through the repair dispatch path.
  - Do not append diagnostic and then return a potentially mixed input for dispatch.

- [x] Ensure runtime records this as rejected diagnostic/internal failure without executing side effects.

- [x] Ensure retry path recomputes from ledger on the next scheduler attempt, not from the failed mixed input.

Acceptance:

- `runtime_internal_contract_misalignment` no longer produces a dispatchable mixed input.
- Run cannot reach `running + failed currentStep + no blocker` through this path.

## Phase 5: Dispatch Gate Confirmation

- [x] Keep `validateConvergenceDispatchRequiredOutputSemantics` strict.

- [x] Add mirror equality checks after canonical render:
  - `repairActionPackage.requiredOutput`
  - `stagePacket.requiredOutput`
  - `stagePacket.nextAction.requiredOutput`
  - `currentActionContract.requiredOutput`
  - top-level `requiredOutput`
  - `executionContext.*` mirrors when present

- [x] Add authority diagnostic only after canonical render comparison.
  - Diagnostic is for observability.
  - Diagnostic must not drive current contract shape.

Acceptance:

- Any remaining mismatch is fail-fast before provider dispatch.
- No ordinary LLM retry is created for internal mirror mismatch.

## Phase 6: Verification

Run targeted tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'Validation|Authority|RequiredOutput|Coverage|Repair|Dispatch' -count=1
```

Run package tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
```

Run boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg -g "*.go"
```

Expected:

- No new core business/template/language/provider special cases.
- Any existing hits must be tests, template/defaults, capability metadata, or generic fields such as `serverId`/MCP server.

Live verification entry criteria:

- All P0 tests pass.
- Package tests pass.
- Failed samples no longer reproduce:
  - rejected validation evidence does not pollute non-focus coverage items.
  - state_change authority cannot dispatch validation-only requiredOutput.
  - repair dispatch preflight receives canonical current authority input.

## Out Of Scope

- Do not modify `docs-product`.
- Do not add run recovery finalization watchdogs as a workaround.
- Do not relax requiredOutput semantics gate.
- Do not add template, language, command, file, provider or workspace-specific branches.
- Do not rely on prompt-only changes to solve authority drift.

## Completion Criteria

This plan is complete only when:

- [x] Evidence binding is exact for runtime accepted/rejected evidence writes.
- [x] Candidate target context no longer affects evidence ledger write scope.
- [x] RequiredOutput mirrors are generated from a single current authority renderer.
- [x] Old requiredOutput mirrors are diagnostic-only.
- [x] Repair finalization fail-fast replaces mixed-input fallback.
- [x] Targeted regression tests cover both live failure shapes.
- [x] Orchestrator package tests pass.
- [ ] Live verification shows the workspace either:
  - continues to the unique next action,
  - enters a real governed blocker,
  - or fails terminally with a precise non-recoverable reason,
  and never again reports `need_state_change` with validation-only requiredOutput at dispatch.
