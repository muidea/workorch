# Runtime Structured Evidence Authority Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-28

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-convergence-regression-root-cause-analysis.md`
- `runtime-progress-quality-resource-closure-plan.md`

## Purpose

本文基于最新 live 核对结果，继续收口 WorkOrch runtime 在 Goal/DoD、Coverage/Evidence ledger、Convergence Authority 与 validation follow-up 上的剩余偏差。

目标不是继续为某个 workspace、某个 DoD 文本、某条验证命令或某次 LLM 输出增加补丁，而是把 accepted evidence 的结构化绑定、NextAction 的确定性推进、evidence_ready 的关闭路径和 resource 成本控制统一收口到 core 中立合同。

本文是后续代码开发的执行清单。所有代码改动必须能映射到本文的某个 closure item，并用对应验证项证明。

## Contract Audit

- Scope: `ws-7ced70e98ea50c43/run-012-replan-001` live verification, coverage/evidence ledger, validation follow-up, contract retry, resource metrics.
- Observed state:
  - `ws-4e83810542ac4d65` 已进入 `completed`。
  - `ws-7ced70e98ea50c43/run-012-replan-001` 仍为 `running`，projection 为 `ready`，`sourceEventRevision` 与 `projectedSourceRevision` 对齐。
  - runtime 已能在 LLM 合同违例后记录 rejected diagnostic evidence，并发起 bounded contract retry。
  - `contract-retry-001...-2` 成功生成 bounded validation graph，随后 `graph-002/001-verify-observability-endpoints-and-metrics-access` 完成。
  - `graph-003/001-verify-healthz-runtime-behavior` 完成，并使 `cwi-026` 进入 `evidence_ready`。
  - coverage 当前仍大量未关闭，观察到 `9 evidence_ready, 19 failed, 57 open, 4 satisfied`。
  - `cwi-088/version_control_snapshot` 仍持有旧的 weak accepted evidence，例如 `list-workspace-root` validation 和 `config.example.yaml` patch。
  - `graph-002` 的 evidence record 仅包含 `coverageItemRefs:["cwi-007"]`，但 `targetRefs` 同时包含 `goal-req-007`、`goal-req-012`、`goal-req-008` 和 DoD 自然语言文本 `接口与转发： GET /healthz 返回 200 和 JSON 健康状态。`。
  - 因为 `graph-002` 没有结构化绑定 `cwi-026` 或 `dod-...-item-001`，runtime 后续又发起 `graph-003` 专门验证 healthz。
  - workorch 主进程仍约 30% CPU、1GB+ RSS，event metadata scans 和 validation scratch 数量继续增长，且历史 scratch proxy/localproxy 进程仍存在。
- Expected state:
  - `Goal/DoD -> coverage ledger -> deterministic NextAction -> narrow Stage Packet -> candidate -> contract gate -> accepted/rejected evidence -> ledger recompute` 必须形成确定性闭环。
  - accepted validation evidence 必须强绑定结构化 CWI、source requirement、target refs 与 observed claims。
  - 自然语言 targetText、step id、summary、coverageTargets、LLM 自述只能作为 context/supporting evidence，不能单独推动 CWI 到 `evidence_ready`。
  - current focus 的 required evidence complete 后，应优先进入 deterministic assessment/closure，而不是继续扩散式 validation。
  - provider retry / contract retry / follow-up 可以保持运行，但不能以牺牲 evidence binding correctness 为代价。
- Deviation:
  - validation proof 已部分贯穿到 evidence record，但 proof scope 没有被规范化成结构化 coverage item refs。
  - accepted evidence reuse 与 validation reconciler 仍存在宽匹配路径。
  - history/replan 继承的 weak accepted evidence 没有在当前 run 下被重新审计或降级。
  - evidence_ready 可以持续积压，runtime 继续选择下一个 validation focus，导致 goal 收敛变慢。
  - contract retry 的 requiredOutput 仍会诱发 LLM 返回 assessment-only 或错误 output shape。
  - resource 压力主要由错误循环、宽验证和历史扫描放大。
- Owner: WorkOrch core.
  - 主要落点是中立 coverage/evidence ledger、convergence authority、contract gate、stage packet 和 resource lifecycle。
  - 不属于 workspaceTemplate/Soul 的业务策略问题，也不应在 core 中引入软件开发、Go、文件名、provider、workspace title 或 live workspace ID 特判。
- Event path:
  - LLM candidate graph -> runtime step execution -> ValidationResult -> EvidenceRecord -> coverage reconciler / evidence reuse -> coverage work item status -> convergence authority refresh -> NextAction -> projection/ViewModel.
- Contract gap:
  - 缺少 accepted evidence 的结构化 binding authority。
  - 缺少 evidence record scope 规范化与一致性校验。
  - 缺少 current-focus-first 的 evidence_ready assessment/closure 策略。
  - 缺少 replan/current run hydration 时对 legacy weak evidence 的重新审计。
  - 缺少对 contract retry output shape 的更窄 runtime-owned correction contract。
- Boundary check:
  - 本文只使用 CWI、sourceRequirementId、normalizedSourceKey、targetRefs、observedClaims、requiredEvidenceKinds、NextAction、EvidenceRecord、ValidationResult、resource lifecycle 等中立概念。
  - 所有软件开发验证偏好、文件粒度、测试选择、工具选择应继续由 workspaceTemplate/profile/Soul/policy/skill 承载。

## Current Live Evidence

### Positive Progress

当前 runtime 已经不是完全无推进：

- provider token / 401 / EOF / output shape 违例均能被记录为 failed/rejected，而不是污染 accepted result。
- `contract-retry-001...-2` 最终生成有效 validation graph。
- `graph-001/001-verify-single-binary-local-build` 和 `graph-001/002-verify-provider-routing-and-fallback-tests` 完成，并生成带 proof 的 accepted evidence。
- `graph-002/001-verify-observability-endpoints-and-metrics-access` 完成，并提升 `cwi-007`、`cwi-008`、`cwi-012` 等相关项的 evidence 状态。
- `graph-003/001-verify-healthz-runtime-behavior` 完成，并使 `cwi-026` 进入 `evidence_ready`。

这些事实说明近期改动改善了“能否继续运行”和“违例是否被拒绝”的问题。

### Remaining Quality Gap

当前问题不是“没有任何推进”，而是：

```text
validation executes
  -> evidence proof exists
  -> proof scope is weak or broad
  -> current or related CWI is not closed deterministically
  -> runtime asks LLM again
  -> more validation / retry / IO
```

典型样本：

```json
{
  "stepId": "run-012-replan-001/graph-002/001-verify-observability-endpoints-and-metrics-access",
  "coverageItemRefs": ["cwi-007"],
  "targetRefs": [
    "goal-req-007-95986952",
    "goal-req-012-4a697115",
    "goal-req-008-835361fc",
    "接口与转发： GET /healthz 返回 200 和 JSON 健康状态。"
  ],
  "observedClaims": 3
}
```

该 evidence 对 healthz DoD 有语义相关性，但缺少结构化 `cwi-026` / `dod-3f67d67944f71959-dod-item-001` 绑定，因此不能可靠关闭 `cwi-026`。后续 `graph-003` 才通过结构化 targetRefs 完成该项：

```json
{
  "stepId": "run-012-replan-001/graph-003/001-verify-healthz-runtime-behavior",
  "coverageItemRefs": ["cwi-026"],
  "targetRefs": ["cwi-026", "dod-3f67d67944f71959-dod-item-001"],
  "observedClaims": 2
}
```

这证明收口方向应是让 runtime 只接受结构化绑定，而不是依赖自然语言文本或 broad target 复用。

## Design Alignment

### Alignment With Goal/DoD NextAction Design

`goal-dod-nextaction-delivery-state-machine-design.md` 要求：

- accepted/rejected evidence 是 coverage 状态变化唯一依据。
- LLM 违例只能作为 rejected diagnostic evidence。
- NextAction 由 runtime 根据 ledger 推导。
- Stage Packet 必须携带当前 Goal/DoD、requiredOutput、accepted/rejected evidence、current failure、target snapshot。

当前实现已部分满足 LLM 违例拒绝与 retry，但未完全满足 accepted evidence 结构化绑定与 evidence_ready 后确定性 closure。

### Alignment With Convergence Authority Simplification Design

`convergence-authority-simplification-design.md` 要求：

- Coverage/Evidence Ledger -> Current Focus -> Deterministic NextAction 是唯一权威。
- `gapBaseline`、`convergenceContract`、`stagePacket`、`repairActionPackage.requiredOutput` 均为派生字段。
- old repair package、old execution graph、contractViolationRecovery、memory recall 等只能作为 diagnostic lineage。

当前实现已能在 dispatch 前重建 authority，并修复了大量 stale focus 问题。但 evidence record scope、coverage target matching 和 history inherited evidence 仍能让 ledger 本身污染 authority。

## Root Cause

### R1. Validation Evidence Scope Is Not Canonicalized

`runtimeStepCoverageScopeMetadata` 将 step input/output 的 `coverageWorkItemIds`、`coverageTargets`、`targetRefs`、`validationEvidence.targetRefs` 原样写入 evidence metadata。

问题：

- `coverageWorkItemIds` 和 `targetRefs` 不一致时，runtime 没有 fail-fast 或降级。
- `validationEvidence.targetRefs` 可以包含自然语言 targetText。
- text targetRef 没有规范化到 CWI/sourceRequirement/normalizedSourceKey。
- evidence record 可能同时宣称 broad targetRefs，但只绑定单个 CWI。

### R2. Accepted Evidence Matching Still Has Weak Paths

当前代码仍存在宽匹配路径：

- `coverageWorkItemMatchesUpdate` 可用 `targetText`、`sourcePath` 和 contains token 匹配。
- `coverageEvidenceTargetAligned` 使用 `coverageRepairTokenMatches`，可接受长文本包含关系。
- `coverageEvidenceRecordItemMatchReason` 允许 `target_ref` 匹配成为 reuse reason。
- `coverageRepairTokenMatches` 对 8 字符以上 token 允许 `strings.Contains`。

这些逻辑对上下文提示有价值，但不应作为 accepted evidence binding 的主路。

### R3. Legacy Accepted Evidence Is Not Re-Audited On Current Run

历史 run/replan 中形成的 weak accepted evidence 仍可出现在当前 run coverage ledger 中。例如：

- `cwi-088/version_control_snapshot` 使用 `list-workspace-root` validation。
- `cwi-085/source_change` 使用 broad validation / patch refs。

这会让 current authority 的输入本身不可信。

### R4. Evidence Ready Does Not Preempt More Validation

当前 coverage 中 `evidence_ready` 会积压，runtime 继续选择 failed/open CWI 生成 validation。该策略可以继续推进，但会造成：

- current slice 不及时 closure。
- evidence_ready 的 assessment 延迟。
- LLM 继续在 broad context 下生成更多验证。
- CPU/IO 成本放大。

### R5. Contract Retry Correction Is Still Too Ambiguous

多次失败显示：

```text
concrete validation evidence repair must return bounded shell/command or mcp/tool_call executionGraph;
plain failed dodAssessment does not satisfy repairActionPackage.requiredOutput
```

这说明 retry correction 虽然继承了当前 stage contract，但 requiredOutput 对 LLM 来说仍可能混入 assessment/follow-up/validation graph 语义。

### R6. Resource Cost Is A Symptom Amplifier

workorch 主进程 RSS 和 CPU 偏高，event metadata scan、payload hydrate、validation scratch 目录数量持续增长。资源问题主要由以下因素放大：

- 宽 evidence 绑定导致重复验证。
- evidence_ready 不关闭导致更多 follow-up。
- request/response capture 与 projection metadata scan 持续增长。
- 普通 validation scratch 和历史 orphan process 未完全纳入 lifecycle reconciliation。

## Target Closure Model

### Structured Evidence Binding

accepted evidence 必须满足以下之一：

1. `coverageItemRefs` 精确包含当前 CWI id。
2. `targetRefs` 精确包含当前 CWI id、sourceRequirementId、normalizedSourceKey 或 declared structured target ref。
3. `sourceRequirementIds` 精确包含当前 CWI sourceRequirementId。
4. profile/template policy 显式声明该 evidence kind 可跨 CWI 复用，并提供结构化复用规则。

以下字段不能单独驱动 accepted binding：

- `coverageTargets`
- natural language `targetText`
- step id leaf token
- summary / thoughtSummary
- stdout marker
- LLM 自述
- contains / fuzzy text match

### Scope Consistency Gate

runtime 记录 accepted validation evidence 前，必须检查：

```text
coverageWorkItemIds / coverageItemRefs
  must align with
validationEvidence.targetRefs / sourceRequirementIds / current authority focus
```

若不一致：

- 可作为 `supportingEvidenceByKind`。
- 可记录 rejected diagnostic evidence，指出 `scope_mismatch`。
- 不得进入 `acceptedEvidenceByKind`。
- 不得把 CWI 推到 `evidence_ready`。

### Evidence Ready Closure Priority

当 current focus 或 active slice 中存在 `evidence_ready` 时，NextAction 应优先选择：

```text
evidence_ready current focus
  -> assessment
  -> satisfied / failed with missing evidence kind / governed blocker
```

仅在以下情况继续 validation：

- assessment policy 明确允许 batch validation before assessment。
- current evidence 仍缺 required kind。
- evidence proof 被判定 supporting-only。
- governance 或 external prerequisite 要求先补证据。

### Legacy Evidence Re-Audit

replan/current run hydration 时，所有 inherited accepted evidence 必须重新通过当前强绑定 gate：

- 通过：保留 accepted evidence。
- 不通过但有参考价值：降级为 supporting evidence。
- 已被 superseded 或 definition revision 不匹配：移除或标记 diagnostic lineage。

该逻辑必须业务中立，不得根据 `version_control_snapshot`、`source_change`、文件名、Go 测试或当前 workspace title 做特判。

### Contract Retry Narrow Output

contract retry 必须只描述当前 allowed output shape：

- 如果当前 NextAction 是 `validate`，只允许 bounded validation graph / managed validation / supported tool_call。
- 如果当前 NextAction 是 `assessment`，只允许 assessment object，不允许 executionGraph。
- 如果当前 NextAction 是 `state_change`，只允许 state-changing patch/artifact candidate。

diagnostic lineage 中的旧 expectedContract 可以保留，但必须标记 `diagnosticOnly`，不能与 current requiredOutput 并列竞争。

## Closure Items

### P0. Make Accepted Validation Binding Structured-Only

Scope:

- `internal/modules/kernel/orchestrator/biz/runtime_coverage_evidence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence_reuse.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_reconciler.go`
- tests under `internal/modules/kernel/orchestrator/biz/*coverage*`

Changes:

- Split target matching into two modes:
  - `strictStructuredBinding` for accepted evidence.
  - `contextualTokenMatch` for LLM context/supporting evidence only.
- Remove `targetText` and contains matching from accepted evidence gate.
- Require validation accepted binding to use structured CWI/sourceRequirement/normalizedSourceKey refs.
- Keep broad text matches only for supporting evidence and context selection.

Acceptance:

- A validation record with `targetRefs:["接口与转发： GET /healthz..."]` but no `cwi-026` / sourceRequirementId must not close `cwi-026`.
- A validation record with `targetRefs:["cwi-026","dod-...-item-001"]` must close `cwi-026`.
- Existing state_change evidence cannot bind by `coverageTargets` alone.

### P1. Canonicalize Runtime Step Evidence Scope

Scope:

- `internal/modules/kernel/orchestrator/biz/runtime_evidence_records.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_evidence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_focus_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`

Changes:

- Build canonical evidence scope from current authority focus before recording EvidenceRecord.
- Convert valid `validationEvidence.targetRefs` to structured refs when they match known current focus refs.
- If LLM returns natural language targetText, keep it in `coverageTargets` / context metadata but not accepted binding refs.
- Reject or downgrade evidence when `coverageWorkItemIds` conflicts with `validationEvidence.targetRefs`.

Acceptance:

- EvidenceRecord for current focus includes canonical `coverageItemRefs`.
- EvidenceRecord metadata separates `structuredTargetRefs` from `coverageTargets` / text refs.
- Mismatched `coverageWorkItemIds:["cwi-007"]` plus DoD text for `cwi-026` does not create cross-binding.

### P2. Re-Audit Inherited Accepted Evidence

Scope:

- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence_reuse.go`
- appservice activation/replan hydration tests.

Changes:

- During replan/restart/resume hydration, run all inherited accepted refs through the current structured binding gate.
- Demote invalid inherited accepted refs to supporting refs with reason `legacy_evidence_not_structurally_bound`.
- Preserve diagnostic lineage for audit, but exclude demoted refs from `acceptedEvidenceByKind`.

Acceptance:

- Legacy `list-workspace-root` validation cannot keep `version_control_snapshot` as `evidence_ready`.
- Source change coverage cannot be satisfied by read/list validation.
- Replan successor does not inherit weak accepted evidence as authority.

### P3. Prioritize Evidence-Ready Assessment

Scope:

- `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`
- `internal/modules/kernel/orchestrator/biz/runtime_acceptance_slices.go`
- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`

Changes:

- Make active slice/current focus evidence_ready assessment preempt next validation focus unless policy says otherwise.
- If multiple evidence_ready items exist in current active slice, select deterministic order by priority and source order.
- Do not ask LLM for broad follow-up when runtime can perform assessment gate deterministically or render narrow assessment contract.

Acceptance:

- After accepted validation completes current focus required evidence, next action is assessment for that focus.
- Open/failed later CWI does not preempt assessment of current focus unless active policy says batch validation.
- projection shows currentActionLabel as assessment when evidence_ready is pending assessment.

### P4. Narrow Contract Retry RequiredOutput

Scope:

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_contracts.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_blocks.go`

Changes:

- Contract retry prompt must include only the current allowed output shape.
- Diagnostic lineage should describe rejected shape but not expose old expectedContract as an alternative authority.
- `validate` retry must forbid plain dodAssessment.
- `assessment` retry must forbid executionGraph.

Acceptance:

- Test rejected `plain failed dodAssessment` during validation retry produces a second prompt that only asks for bounded validation graph.
- Test assessment-only retry cannot return executionGraph without being rejected and escalated by budget.
- Rejected output remains diagnostic-only and never writes accepted evidence.

### P5. Resource Amplification Guard

Scope:

- `internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go`
- `internal/modules/kernel/orchestrator/biz/runtime_managed_validation.go`
- event metadata/payload repository paths.
- projection/readmodel IO paths.

Changes:

- Tie ordinary shell validation scratch to resource lifecycle metadata, not only managed validation.
- Add per-run scratch TTL/size accounting and orphan reconciliation diagnostics.
- Avoid repeated full metadata scans for current projection queries where revision/index can answer.
- Sample or deduplicate progress checkpoint writes without weakening worker heartbeat recovery.

Acceptance:

- Long run live verification leaves no new orphan validation processes.
- Scratch directories for completed step-scoped validations are cleaned or marked retained with reason.
- event metadata scan count grows sublinearly with repeated snapshot polling.
- CPU/RSS stop growing without corresponding active provider/shell work.

### P6. Stage Packet Evidence Context Tightening

Scope:

- `internal/modules/kernel/orchestrator/biz/execution_context_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_llm_context.go`
- `internal/modules/blocks/llm/**`

Changes:

- Stage Packet must expose current focus structured refs prominently.
- Historical broad evidence must be summarized as diagnostic/supporting, not shown as accepted authority.
- Latest rejected diagnostics must explain why an evidence candidate was supporting-only or scope-mismatched.

Acceptance:

- LLM input for validation focus includes `coverageWorkItemIds` and structured `targetRefs`.
- LLM input does not present text-only targetRefs as accepted binding authority.
- Compact history cannot hide current focus requiredOutput or target refs.

## Implementation Order

1. P0: strict accepted evidence binding.
2. P1: canonical runtime step evidence scope.
3. P2: legacy inherited evidence re-audit.
4. P3: evidence_ready assessment priority.
5. P4: contract retry requiredOutput narrowing.
6. P6: Stage Packet evidence context tightening.
7. P5: resource amplification guard.

P0-P2 must land before further live quality claims. Resource work should follow after false positive binding is blocked, otherwise high IO will keep being amplified by unnecessary retry/follow-up loops.

## Verification Plan

### Unit Tests

- Evidence with natural language targetText only is supporting-only.
- Evidence with exact CWI/sourceRequirement structured refs is accepted.
- Mismatched `coverageWorkItemIds` and `validationEvidence.targetRefs` is rejected or supporting-only.
- Replan hydration demotes old weak accepted evidence.
- evidence_ready current focus yields assessment NextAction before next validation.
- validation contract retry rejects plain assessment and asks only for bounded validation graph.

### Integration Tests

- Build a run with:
  - one goal CWI,
  - one DoD CWI,
  - one validation evidence that mentions both by text but structurally binds one.
  - Verify only the structurally bound CWI changes to evidence_ready.
- Build a replan successor with legacy accepted refs.
  - Verify weak inherited evidence is demoted.
- Build a contract violation sequence.
  - Verify rejected diagnostic evidence is recorded and no accepted evidence/result/artifact is produced.

### Live Verification

Use a fresh workspace or clean run lineage after P0-P4:

- Record workspaceId/runId, projection revision, current focus, NextAction, accepted/rejected evidence refs.
- Verify each completed validation step either:
  - binds exactly to current CWI and changes ledger, or
  - becomes supporting/rejected with reason.
- Verify `evidence_ready` count does not grow without assessment or satisfied transition.
- Verify no new weak accepted evidence similar to `list-workspace-root -> version_control_snapshot`.
- Verify CPU/RSS and event metadata scan growth during repeated snapshot polling.

## Non-Regression Rules

- Do not add core special cases for `software-dev`, Go, `go test`, `main.go`, `config.example.yaml`, provider names, workspace title, or specific CWI IDs.
- Do not use natural language targetText fuzzy matching as accepted evidence authority.
- Do not let projection repair owner state.
- Do not let LLM output decide completion without coverage/evidence ledger gate.
- Do not treat provider retry success as proof of goal progress unless resulting evidence is structurally bound.
- Do not keep old repairActionPackage/currentFocus as authority after convergence authority refresh.

## Definition Of Done

This closure is complete when:

- accepted validation evidence can only close CWI through structured binding.
- inherited weak evidence is demoted or removed from current accepted ledger.
- evidence_ready current focus deterministically enters assessment/closure.
- contract retry output shape is narrow enough to reduce repeated assessment/validation graph confusion.
- live run shows fewer repeated validation follow-ups for already evidenced targets.
- resource metrics no longer grow rapidly due to projection/history/evidence scan amplification.
- all changes remain core business-neutral and template-specific strategy remains in workspaceTemplate/profile/Soul/policy.
