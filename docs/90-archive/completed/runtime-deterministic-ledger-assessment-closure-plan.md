# Runtime Deterministic Ledger Assessment Closure Plan

本文用于收口 live verification 中暴露的最小主链路偏差：runtime 已经能产生并接受当前 focus 的 validation evidence，但 `evidence_ready` 后仍依赖 LLM assessment / follow-up repair，导致已具备机械判断条件的 coverage item 无法快速进入 `satisfied`，并继续消耗 CPU/IO 与 LLM 上下文。

本计划必须按条逐一收口，禁止继续追加单点 prompt、contract violation 文案或场景特判。WorkOrch core 必须保持业务中立；具体业务目标、文件、语言、命令选择仍属于 workspaceTemplate / Soul / policy。

## Contract Audit

- Scope: `ws-db040d24a190f6e5/run-011-replan-001-replan-001-replan-001-replan-001`, current focus `cwi-006`.
- Observed state:
  - `graph-005/001-verify-lsp-sync-behavior` produced accepted validation evidence.
  - `cwi-006` moved to `evidence_ready` with accepted refs:
    - `run-011-replan-001-replan-001-replan-001-replan-001/graph-005/001-verify-lsp-sync-behavior/validation/runtime-step`
    - `run-011-replan-001-replan-001-replan-001-replan-11b6e26bab1d-validation`
  - Later LLM assessment failed with `dod_evidence_quality_gap` / `dod_coverage_missing`, then runtime entered additional repair/follow-up steps.
- Expected state:
  - Once current focus has accepted validation evidence with required kind, target binding and observed proof, core ledger should deterministically move the item from `evidence_ready` to `satisfied`, or emit a precise mechanical missing reason.
  - LLM assessment must not be the primary owner of this mechanical state transition.
- Deviation:
  - `evidence_ready` remains open to LLM assessment and follow-up expansion.
  - Historical or insufficient evidence can participate in assessment failure and trigger new repairs even when the current focus already has accepted evidence.
- Owner: WorkOrch core, specifically coverage ledger, evidence ledger, deterministic NextAction and contract violation recovery.
- Event path:
  - runtime validation result -> evidence record normalization -> coverage ledger binding -> coverage item `evidence_ready` -> deterministic assessment -> `satisfied` or exact missing reason -> NextAction recompute -> projection.
- Contract gap:
  - Missing deterministic ledger-owned transition for `evidence_ready -> satisfied`.
  - Assessment failure is allowed to re-enter repair/follow-up instead of only rejecting the assessment and recomputing from ledger.
- Proposed change:
  - Add a deterministic assessment gate in core ledger before LLM assessment.
  - Narrow LLM assessment to non-mechanical semantic judgement only.
- Boundary check:
  - The rule is schema-based: evidence kind, accepted status, coverage item binding, structured target refs and observed claims.
  - No template ID, Soul ID, language, command, file path or business text can be hardcoded in core.
- Verification:
  - Unit tests for evidence normalization, deterministic coverage assessment, NextAction recompute and assessment violation isolation.
  - Boundary search for core business/template/language/command special cases.
  - Live verification that current focus with accepted evidence advances to `satisfied` and the run moves to next unresolved focus.

## 最小主链路

```text
runtime validation accepted
  -> evidence record normalized
  -> coverage item evidence_ready
  -> deterministic ledger assessment
  -> satisfied
  -> next unresolved focus
```

异常链路：

```text
runtime validation accepted
  -> evidence record missing required binding/proof
  -> rejected/supporting diagnostic evidence
  -> coverage item remains unresolved with exact missing reason
  -> deterministic NextAction chooses validation/state_change/decision
```

LLM assessment 不再位于主链路上。只有 deterministic ledger 无法判断语义充分性时，才进入 LLM assessment。

## 收口原则

1. `Coverage ledger` 与 `Evidence ledger` 是状态转换权威。
2. `evidence_ready` 是机械 gate，不是 LLM 自称完成。
3. 当前 focus 的 accepted evidence 优先级高于历史 rejected evidence、旧 run evidence、memory 和自然语言 assessment。
4. LLM assessment 只能生成 candidate judgement，不能覆盖 deterministic ledger state。
5. Contract violation 只能拒绝当前候选输出，不能回退已 accepted evidence，也不能重新打开已机械满足的 focus。
6. 所有判断必须基于结构化 DTO / ledger fields，不解析业务文本。

## 功能切片

### S1 Evidence Record Normalization

目标：确保 accepted validation evidence 具备可被 coverage ledger 机械消费的最小字段。

收口项：

- `validation` accepted record 必须稳定携带：
  - `status=accepted`
  - `kind=validation`
  - `sourceRef`
  - `coverageWorkItemIds`
  - `structuredTargetRefs`
  - `observedClaims` 或等价可审计 proof summary
  - `evidenceText` / `outputSummary` / payload ref 中至少一种可审计内容引用
- 字段缺失时，不得进入 accepted coverage binding。
- 缺字段的记录只能作为 supporting diagnostic 或 rejected diagnostic。
- normalization 只能消费结构化字段，不能按业务文本推断 target。

主要落点：

- `internal/modules/kernel/orchestrator/biz/runtime_step_evidence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence_records.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_evidence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_managed_validation.go`

验收：

- Accepted validation with complete metadata creates an accepted `EvidenceRecord`.
- Missing `coverageWorkItemIds` or `structuredTargetRefs` does not bind coverage.
- Missing observed proof does not move coverage to `evidence_ready`.

### S2 Deterministic Coverage Assessment

目标：对 `evidence_ready` item 做 core-owned 机械判断，满足条件时直接进入 `satisfied`。

确定性判定条件：

- 当前 item required evidence kinds 全部存在。
- 至少一条 accepted evidence 绑定当前 `coverageWorkItemId`。
- accepted evidence 的 `structuredTargetRefs` 命中当前 item target refs。
- validation evidence 有 `observedClaims` 或等价可审计 output summary。
- evidence status 是 accepted，且未被同 scope 的 rejected override 标记否定。

输出：

- 全部满足：`status=satisfied`，记录 `satisfiedByEvidenceRefs`。
- 不满足：保持 unresolved，写入 exact missing reason。
- 无法机械判断语义充分性：进入 `assessment_required`，但不得丢失 accepted refs。

主要落点：

- `internal/modules/kernel/orchestrator/biz/runtime_coverage_reconciler.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items.go`
- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`

验收：

- `evidence_ready` + complete accepted validation -> `satisfied`。
- `evidence_ready` + missing observed proof -> unresolved with exact reason。
- Historical rejected evidence does not override current complete accepted evidence unless it explicitly targets the same current evidence ref with a structured invalidation reason.

### S3 NextAction Recompute From Ledger

目标：NextAction 只消费最新 ledger recompute result，不从旧 LLM assessment、repair lineage 或 memory 推断当前状态。

顺序：

1. Open scoped approval -> request approval.
2. Open scoped decision -> request decision.
3. Current focus deterministic satisfied -> select next unresolved focus.
4. Current focus evidence ready but deterministic assessment incomplete -> emit exact missing reason or semantic assessment action.
5. Missing validation evidence -> validation action.
6. Missing state-changing evidence -> state-changing action.
7. No automatic action -> blocker/fail/decision by eligibility gate.

主要落点：

- `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_current_action_contract.go`

验收：

- Current satisfied focus cannot dispatch another validation/repair for the same focus.
- Next unresolved focus becomes the active focus after deterministic satisfaction.
- LLM assessment failure cannot preempt a newer ledger-derived NextAction.

### S4 Assessment Role Narrowing

目标：LLM assessment 只用于无法机械判断的语义充分性，不参与可机械闭合的主链路。

收口项：

- Assessment input 只包含 current focus compact slice、current accepted/rejected refs、required evidence policy 和必要 Goal/DoD slice。
- Assessment 不得读取全量历史 rejected evidence 作为 satisfied 判定权威。
- Assessment 输出引用 evidence 时，必须引用 current focus allowed evidence refs。
- Assessment contract violation 只拒绝该 assessment output。

主要落点：

- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- `internal/modules/blocks/llm/biz` request/context rendering code, if assessment context is built there.

验收：

- Deterministically satisfied item does not trigger LLM assessment.
- Assessment request context excludes unrelated historical rejected evidence from authority sections.
- Assessment violation leaves accepted evidence and coverage item refs intact.

### S5 Contract Violation Isolation

目标：LLM assessment 或 repair 输出违例时，只影响该候选输出，不污染 accepted ledger。

收口项：

- Contract violation creates rejected diagnostic evidence.
- Rejected diagnostic cannot:
  - become accepted validation evidence
  - create accepted result/artifact
  - reopen deterministic satisfied item
  - write high-value memory
- Recovery path must first recompute ledger before dispatching repair.

主要落点：

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_result_recording.go`
- `internal/modules/kernel/orchestrator/biz/runtime_memory.go`

验收：

- Assessment contract violation does not change current focus from `evidence_ready` / `satisfied` to failed.
- Recovery action is derived from recomputed ledger, not from rejected LLM output text.

### S6 Projection And Live Verification

目标：workorchd projection 显示 ledger authority state，TUI/Web 不做二次业务推断。

收口项：

- Projection should expose:
  - current focus id
  - focus status
  - accepted evidence refs
  - next deterministic action
  - blocker/decision only when owner state has same-scope open governance object
- Status API should make it possible to see whether current run is in:
  - deterministic ledger recompute
  - LLM candidate generation
  - contract retry
  - external validation

主要落点：

- `internal/modules/application/workorchd/biz/appservice/readmodelprojection`
- `internal/modules/application/workorchd/biz/appservice`
- `internal/pkg/workorchport`

验收：

- Projection does not show waiting decision without same-scope open decision.
- Running workspace with `evidence_ready` shows current deterministic next action.
- Live verification can confirm `evidence_ready -> satisfied -> next focus` without full snapshot scanning.

## 测试计划

P0 tests:

- accepted validation evidence with full target/proof closes current focus.
- missing validation proof does not close current focus.
- historical rejected evidence does not override current accepted evidence.
- assessment contract violation is rejected diagnostic only.
- NextAction advances to next unresolved focus after deterministic satisfaction.

P1 tests:

- managed validation evidence normalizes to the same accepted evidence schema.
- provider retry / contract retry inherits current ledger state and cannot reopen satisfied focus.
- projection exposes current focus status and next deterministic action.

P2 live verification:

- Use live workspace with a known `evidence_ready` item.
- Confirm CPU/IO decreases after deterministic closure because LLM assessment is not invoked for mechanically satisfied items.
- Confirm run no longer generates repeated repair/follow-up for the same satisfied focus.

## 反打圈门禁

本计划不允许以下类型改动作为完成项：

- 只修改 prompt，要求 LLM 更好地引用 evidence。
- 只增加新的 contract violation 文案或 rule，但不改变 ledger transition。
- 在 core 中加入模板、语言、命令、文件名或业务文本特判。
- 放宽 coverage evidence gate 以快速标记 satisfied。
- 让 assessment failure 驱动新的 repair，而不先 recompute ledger。

每个代码切片必须回答：

1. 旧失败进入哪个函数？
2. 该函数当前输出是什么？
3. 修改后输出如何直接改变 ledger / NextAction？
4. 哪个测试证明不会再次进入同一循环？
5. 哪个边界搜索证明 core 仍业务中立？

## 完成标准

- 当前 focus accepted validation evidence 能通过 deterministic ledger assessment 关闭。
- `evidence_ready` 不再长期停留并触发开放式 assessment/follow-up。
- LLM assessment 只处理无法机械判断的语义充分性。
- Contract violation 不污染 accepted evidence，不回退 deterministic state。
- Projection / status 能轻量展示 current focus、evidence refs 和 next deterministic action。
- Live verification 中同一 workspace 能从已满足 focus 自动推进到下一个 unresolved focus，且 CPU/IO 不因重复 assessment 持续升高。

## Implementation Progress

### 2026-06-30 Code Closure

- S1-S3:
  - Accepted validation evidence with complete structured binding now closes `evidence_ready` coverage items through the deterministic ledger path.
  - `NextActionForRun` recomputes after deterministic close and selects `complete` or the next unresolved focus instead of dispatching assessment for the same focus.
- S4:
  - `BuildConvergenceContract` no longer converts a deterministically closable focus into `need_assessment`, even when stale `gapBaseline.nextAction` or `convergenceAuthority.nextAction` still says `assessment`.
  - A neutral `complete` convergence state is used only to clear stale LLM assessment/repair contracts and expose that runtime ledger owns the transition.
- S5:
  - Coverage-assessment contract violation recovery now recomputes against the current structured focus and avoids repeated assessment-only retry when accepted binding already makes the focus mechanically closable.
  - Rejected assessment output remains diagnostic-only; existing result/evidence guard tests continue to cover that it cannot become accepted result, artifact or memory.
- S6:
  - Gap baseline and convergence authority nextAction now report `complete` for deterministically closable focus instead of `assessment`.
  - `coverageWorkItemContext.currentFocus.nextAllowedConvergenceStates` no longer advertises `need_assessment` for deterministically closable items.

Validation:

- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestBuildConvergenceContract.*(Assessment|Deterministic|ValidationFocus)|TestCoverageAssessmentIncomplete.*|TestGapBaselineNextAction|TestConvergenceAuthoritySnapshot|TestNextActionForRunEvidenceReady' -count=1`
- `go test ./internal/modules/kernel/orchestrator/biz -count=1`
- `go test ./... -count=1`
- `git diff --check`
- Boundary search over changed core files for template/business/language/command special cases.
