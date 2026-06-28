# Runtime Evidence Proof Strength And Assessment Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-28

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-managed-validation-resource-lifecycle-closure-plan.md`
- `runtime-nextaction-validation-to-statechange-closure-plan.md`
- `runtime-validation-evidence-context-closure-plan.md`
- `projection-event-io-resource-closure-plan.md`

## Purpose

本文用于收口最新 live verification 暴露出的主要偏差：WorkOrch runtime 已能通过 EventHub、projection、convergence authority、contract retry 和 evidence ledger 持续调度 workspace，但仍可能接受证明力不足的 validation evidence，使 coverage item 进入 `evidence_ready`，后续却无法稳定满足 Goal/DoD 并完成收敛。

本计划的目标是把“形式有效的验证输出”和“足以支撑当前 Goal/DoD coverage 的证据”分离，确保 runtime 只根据业务中立的结构化证据质量、target alignment、observed claims 和 assessment 结果推进状态机。

本文不引入模板、语言、命令、provider、workspace ID 或具体业务文本特判。软件开发场景的交付粒度、验证偏好和 required evidence policy 只能通过 workspaceTemplate / profile / Soul / policy / skill 绑定进入 runtime。

## Contract Audit

- Scope:
  - `ws-4e83810542ac4d65/run-009`
  - `ws-7ced70e98ea50c43/run-008`
  - runtime evidence quality, coverage ledger acceptance, deterministic assessment trigger, state_change / validation ordering, high-frequency progress IO.
- Observed state:
  - 两个 workspace 均能保持 `running`，说明 command ingress、EventHub 主路径、projection 刷新、replan/resume 触发链路不是当前主 blocker。
  - `ws-4e83810542ac4d65` 已有较多 accepted evidence，coverage 中多个 CWI 进入 `evidence_ready`，但 `satisfied=0`，仍持续推进后续验证和 follow-up。
  - `ws-7ced70e98ea50c43` 交付文件仍较薄，例如代理能力、usage/cost、interaction archive、provider fallback 等目标没有真实充分实现，但 generic test fallback / all-tests fallback 产生的 validation evidence 被记录为目标 CWI 的证据。
  - runtime 没有把上述弱证据误标为最终 `satisfied`，但允许它们进入 `evidence_ready`，导致后续 NextAction 继续推进而不是先做质量 assessment / downgrade / state_change。
  - workorch 进程仍存在较高 CPU、RSS 和 IO 压力，说明 high-frequency progress、checkpoint、projection/detail hydration 或 history payload 写入仍有资源风险。
- Expected state:
  - `Goal/DoD Definition -> Coverage/Evidence Ledger -> Current Focus -> Deterministic NextAction -> Candidate Gate -> Accepted/Rejected Evidence -> Ledger Recompute` 是唯一收敛闭环。
  - validation output 通过 schema 和命令成功只代表 `formal_valid`，不能自动代表 `coverage_sufficient`。
  - 只有 target-aligned、claim-supported、quality-sufficient 的 evidence 才能作为当前 CWI required evidence 入账。
  - `evidence_ready` 必须及时进入 deterministic assessment / downgrade / reject / state_change，而不能长期积累弱证据并继续扩图。
  - 高频 progress 不应触发完整 projection fan-out、detail hydration 或大量持久化 IO。
- Deviation:
  - evidence quality gate 对 proof strength 和 target alignment 的要求不足。
  - coverage ledger 接受 required evidence 时主要看 evidence kind 与形式质量，缺少对 `targetRefs`、`observedClaims`、`proofKind`、`coverageAlignment` 的硬性约束。
  - deterministic assessment 触发节奏不足，`evidence_ready` 可以积累而未及时被确认、降级或拒绝。
  - 对软件开发类目标，template/profile 未足够结构化地要求功能型 DoD 同时具备 `state_change`、`validation`、`assessment`。
  - progress/projection/history IO 仍可能放大长会话多 workspace 的资源消耗。
- Owner:
  - Core: evidence proof-strength model, evidence quality derivation, coverage ledger acceptance gate, assessment trigger, NextAction ordering, progress IO coalescing, projection hydration boundary.
  - Template/Soul/policy: software-dev required evidence kinds, validation preference, state_change before validation policy, target-aligned validation guidance.
  - LLM/provider: 只负责候选内容和 bounded retry，不拥有 coverage acceptance 或 completion 判定。
- Event path:
  - LLM/block output -> contract gate -> evidence quality derivation -> coverage acceptance gate -> coverage/evidence ledger recompute -> deterministic NextAction -> stage packet -> projection.
  - Progress IO path: LLM stream progress -> runtime progress event -> checkpoint/projection/read model -> TUI/Web display.
- Contract gap:
  - 缺少 `formal_valid` 与 `coverage_sufficient` 的分层模型。
  - 缺少 proof-strength / target-alignment 在 coverage 入账前的统一门禁。
  - 缺少 evidence_ready 后的 deterministic assessment cadence。
  - 缺少 template-bound evidence policy 对软件开发 DoD 的强约束绑定。
  - 缺少 progress event persistence / projection hydration 的明确限频合同。
- Boundary check:
  - 本计划只使用结构化字段和中立状态转移。
  - Core 不允许根据 Go、TypeScript、测试命令、文件名、provider、workspace title、workspace ID 或具体 stderr/stdout marker 推断业务结果。
  - 需要场景策略时，放入 workspaceTemplate / profile / Soul / policy，并在 workspace 创建或 run activation 时固化到 execution context。

## Target Model

### Evidence Proof Strength

Evidence 需要区分两个层次：

```text
formal_valid
  schema passed
  command/runtime result parsed
  validationEvidence metadata present
  side effect admitted or blocked correctly

coverage_sufficient
  required evidence kind matched
  targetRefs aligned to current coverage item
  observedClaims support acceptance criteria
  proofKind accepted by policy
  evidenceQuality not inconclusive / diagnostic_only
  assessment or gate has not rejected the evidence
```

`formal_valid` 是进入 evidence ledger 的最低条件，但不是满足 coverage 的充分条件。

### Neutral Evidence Fields

Core 可以消费以下业务中立字段：

```text
validationEvidence
  mode
  observationType
  proofKind
  coverageAlignment
  targetRefs[]
  observedClaims[]
  controlledSubstitute
  limitations[]
  sourceRefs[]

evidenceQuality
  level              # valid | weak | inconclusive | invalid | diagnostic_only
  reason
  tags[]
  confidence
```

示例 tags 只表达通用质量，不表达业务语义：

- `generic_test_fallback`
- `target_unaligned`
- `controlled_substitute_unproven`
- `discovery_only`
- `missing_observed_claims`
- `missing_target_refs`
- `diagnostic_only`

### Coverage Acceptance Gate

Coverage ledger 只接受满足当前 CWI required evidence kind 的 evidence：

```text
CoverageAcceptanceGate(currentFocus, evidence)
  evidence.kind in currentFocus.requiredEvidenceKinds
  evidence.targetRefs intersects currentFocus.targetRefs or currentFocus.id
  evidence.observedClaims cover at least one acceptance criterion or policy-required claim
  evidence.proofKind allowed by currentFocus.policyRef
  evidenceQuality.level in accepted levels
  evidence is not diagnostic-only, rejected, stale, truncated, or unrelated
```

未通过 gate 的 evidence 仍可保留在 evidence ledger，但只能作为 diagnostic、supporting 或 global health evidence，不能补齐当前 coverage missing evidence。

### Deterministic Assessment Cadence

Runtime 必须避免长期积累 `evidence_ready`。以下情况应触发 assessment 或 downgrade：

- 当前 focus required evidence 已形式齐备。
- 同一 run 中 `evidence_ready` 数量达到配置阈值。
- 即将从一个 goal section 切换到另一个 major section。
- evidence 带有 weak / fallback / substitute / inconclusive 质量标签。
- 当前 CWI 已多次验证但没有 target-aligned state_change 或 observed claims。

Assessment 的职责是确认 accepted evidence 是否真正支撑 coverage item；不能替代缺失的 state_change 或 validation。

### StateChange Before Validation For Deliverable Goals

Core 不判断“软件开发目标”，只读取结构化 `requiredEvidenceKinds` 和 policy。

软件开发 template/profile 应对功能型 DoD 声明：

```text
requiredEvidenceKinds:
  - state_change
  - validation
  - assessment
```

当 current focus 缺少 `state_change` evidence，NextAction 应优先 `state_change`；当存在 state_change 但缺少 validation，NextAction 才进入 validation；当二者都具备但未确认质量，NextAction 进入 assessment。

### Progress IO Policy

高频 LLM stream progress 应拆分为：

- in-memory progress: 给 TUI/Web 轻量刷新，不强制持久化每个 chunk。
- persisted milestone: started, first_token, periodic heartbeat, completed, failed。
- observability trace: 只有诊断接口或显式调试请求才按 ref 读取完整 trace。

普通 projection 不应 hydrate 高频 progress payload。workspace list、prompt、main view 只依赖 owner state、current step、latest milestone 和 projection revision。

## Closure Items

### E1. Evidence Proof-Strength Model

Status: implemented in current code pass.

Implementation:

- Added `coverageEvidenceAcceptanceOptions` as the neutral proof-strength context for coverage acceptance.
- Runtime validation steps now carry target refs, observed claims, proof kind, coverage alignment, evidence quality and tags into the coverage gate.
- Weak or insufficient evidence is retained as `supportingEvidenceByKind` / `supportingEvidenceRefs` instead of being accepted as coverage evidence.

Add or consolidate neutral proof-strength fields for runtime evidence.

Required changes:

- Define `coverageAlignment`, `proofKind`, `targetRefs`, `observedClaims`, `limitations` and `evidenceQuality.level` semantics.
- Ensure existing validation outputs can be normalized without business-specific parsing.
- Preserve weak / inconclusive evidence as diagnostic/supporting evidence instead of dropping it.

Acceptance:

- Unit test: schema-valid validation output without targetRefs is `formal_valid` but not `coverage_sufficient`.
- Unit test: discovery-only output becomes diagnostic/supporting evidence.
- Boundary search proves core has no template/language/command/provider string branch.

### E2. Evidence Quality Derivation Gate

Status: implemented in current code pass.

Implementation:

- Validation evidence admitted from runtime steps and reconciled validation results now requires target alignment and observed claims before it can satisfy coverage.
- Generic, target-unaligned, discovery-only or diagnostic-only tags force proof-strength evaluation and prevent automatic coverage acceptance.
- Command success remains formal runtime evidence but is not sufficient to close a target CWI without structured proof metadata.

Strengthen quality derivation before coverage ledger acceptance.

Required changes:

- Default nil/empty quality to `inconclusive`, not accepted coverage evidence, when evidence lacks target alignment.
- Treat generic fallback, all-tests fallback, discovery-only and unproven controlled substitute as weak or inconclusive unless target-aligned observed claims are present.
- Distinguish command success from target proof.

Acceptance:

- Unit test: `exitCode=0` with generic tests cannot satisfy a specific CWI validation requirement.
- Unit test: target-aligned command with observed claims can be accepted.
- Existing rejected diagnostic behavior remains unchanged.

### E3. Coverage Ledger Acceptance Gate

Status: implemented in current code pass.

Implementation:

- Added `coverageApplyWorkItemEvidenceWithOptions` and routed runtime step / validation result reconciliation through it.
- Accepted validation evidence must now satisfy evidence kind, sufficient quality, target alignment and observed claims before moving a CWI to `evidence_ready`.
- Target-unaligned validation remains visible as supporting evidence and does not reduce `missingEvidenceKinds`.

Apply proof-strength before a CWI can move to `evidence_ready`.

Required changes:

- `coverageApplyWorkItemEvidence` must require evidence kind, target alignment, observed claims and sufficient quality.
- Non-sufficient evidence remains visible in evidence ledger and projection diagnostics.
- Missing evidence calculation must not treat diagnostic/supporting evidence as fulfilling required evidence kinds.

Acceptance:

- Unit test: generic fallback validation does not move CWI to `evidence_ready`.
- Unit test: target-aligned state_change + validation moves CWI to `evidence_ready`.
- Unit test: rejected diagnostic never satisfies coverage.

### E4. Deterministic Assessment Trigger

Status: implemented in current code pass.

Implementation:

- `NextActionForRun` now derives `assessment` before `complete`.
- `coverageWorkItemCompletionReady` no longer treats `evidence_ready` as complete-ready; only `satisfied` / `closed` / `completed` can reach complete.
- Existing NextAction tests were updated so `evidence_ready` requests deterministic assessment.

Adjust NextAction so evidence quality is assessed before the run drifts across too many CWI.

Required changes:

- Trigger assessment when current focus becomes evidence_ready.
- Add threshold-based assessment for accumulated evidence_ready items.
- Trigger downgrade/reject when evidence carries weak/inconclusive tags.
- Ensure assessment cannot replace missing state_change or validation.

Acceptance:

- Unit test: current focus evidence_ready derives assessment before selecting unrelated next CWI.
- Unit test: weak evidence_ready derives assessment/downgrade path.
- Unit test: missing state_change still derives state_change, not assessment.

### E5. Template/Profile Required Evidence Policy

Status: implemented by existing template/profile binding; verified in current audit.

Implementation:

- Built-in `software-dev` template already declares `profileContracts.software-dev.evidenceRequirements` and `validationPolicies`.
- Workspace activation merges template/workspace context hints into execution contract, and core reads only structured `evidenceRequirements` through `profileEvidenceKindRequirements`.
- Current code pass did not add core template-name branches.

Move software-development-specific convergence expectations into template/profile data.

Required changes:

- Add or refresh template/profile policy for deliverable DoD required evidence kinds.
- Bind policy at workspace creation or run activation into execution context.
- Core consumes only structured policy fields.

Acceptance:

- Unit test: software-dev template binds required evidence kinds to workspace/run context.
- Unit test: a non-software template is not forced into software-dev evidence policy.
- Boundary search proves kernel/orchestrator has no `software-dev`, language or command-specific branch.

### E6. StateChange / Validation Ordering Enforcement

Status: implemented and verified by existing template/profile binding plus current NextAction ordering.

Implementation:

- Existing NextAction ordering still prioritizes missing `state_change` before missing validation when required evidence kinds declare both.
- `evidence_ready` now routes to assessment before complete.
- Built-in template defaults already bind software-dev `templateStagePolicies`, `profileContracts.evidenceRequirements`, `validationPolicies` and `evidencePolicies` through workspace context hints.
- Activation tests verify template-bound profile contracts are merged into execution context, while non-software templates do not inherit software-dev-specific strategy keys.

Ensure NextAction respects required evidence order from policy.

Required changes:

- If current focus lacks required `state_change`, derive `state_change`.
- If state_change exists but required validation is missing, derive validation.
- If required evidence exists but quality is not assessed, derive assessment.
- If validation failure proves implementation gap through structured diagnostic, route back to state_change.

Acceptance:

- Fixture: validation-only evidence cannot close a focus requiring state_change.
- Fixture: implementation-gap diagnostic switches from validation repair to state_change.
- Fixture: target-aligned state_change followed by validation enters assessment.

### E7. Validation Graph Contract Narrowing

Status: implemented in current code pass.

Implementation:

- Discovery-only, target-unaligned, missing-target and missing-claim evidence tags prevent validation evidence from satisfying coverage.
- Target-aligned evidence with observed claims can still be accepted through the same neutral gate.
- Execution graph contract now requires every shell/command or mcp/tool_call validation producer to declare `validationEvidence.mode`, `validationEvidence.observationType`, `validationEvidence.targetRefs`, and `validationEvidence.observedClaims` before execution.
- Contract violation retry hints, requiredFields, and validation node examples now include target refs and observed claims.

Prevent LLM validation graph fallback from becoming target coverage evidence without proof.

Required changes:

- Discovery nodes are support evidence only.
- all-tests/generic fallback is global health evidence unless targetRefs and observedClaims prove alignment.
- controlled substitute must declare limitation and proof relationship to target.
- Contract retry should ask for target-aligned evidence, not broader validation.

Acceptance:

- Unit test: discovery-only validation graph is not accepted as target validation.
- Unit test: controlled substitute without proof relationship is inconclusive.
- Unit test: target-aligned substitute with policy allowance can be accepted.

### E8. Progress Event IO Coalescing

Status: implemented in current code pass and verified by existing projection query tests.

Implementation:

- Runtime internal progress recorder now uses a 30s non-terminal persistence interval, aligned with provider stream milestones.
- Terminal progress still persists immediately.
- Read-model projection event queries use compact projection mode and explicitly exclude `TopicRunLLMStreamProgress`, while observability queries remain payload-inclusive for explicit diagnostics.
- Existing query tests verify projection excludes stream progress payloads and observability keeps the complete event timeline.
- Explicit external progress API remains unthrottled by design; it may represent owner-state progress rather than provider stream heartbeat and must not be coalesced without a separate owner-state contract.

Reduce CPU/IO pressure caused by high-frequency progress events.

Required changes:

- Do not persist every stream progress chunk as a full event/checkpoint payload.
- Persist milestones and rate-limited heartbeat only.
- Projection should read lightweight latest progress state without hydrating full trace.
- Full trace remains accessible through explicit observability refs.

Acceptance:

- Unit test: repeated progress updates are coalesced into bounded persisted milestones.
- Projection test: workspace list/main view do not hydrate progress payload.
- Live verification: repeated LLM streaming no longer produces unbounded checkpoint/projection writes.

### E9. Context And Evidence Compaction

Status: implemented in current code pass.

Implementation:

- `executionContextPackage` now includes a compact `evidenceLedger` sourced from runtime `EvidenceRecord` metadata for the current run.
- The compact ledger keeps recent complete evidence plus historical summary refs, preserving `evidenceRef`, `sourceRef`, `payloadRef`, `kind`, `status`, `evidenceQuality`, `targetRefs`, `coverageItemRefs`, `observedClaims`, proof/coverage metadata and concise reasons.
- Raw payload fields such as stdout/stderr/content/raw output are not copied into the ledger; large outputs remain addressable through `payloadRef` / `sourceRef`.
- LLM prompt rendering exposes the ledger as volatile `C4.evidence-ledger`, not as C0 stable prefix content.
- Provider context budget treats `evidenceLedger` as high-priority dynamic input and preserves its refs/claims under aggressive compaction.

Keep LLM input complete but bounded.

Required changes:

- LLM context should pass recent complete evidence plus historical compact summaries and refs.
- Accepted/rejected evidence should be represented by source refs, quality, target alignment and concise observed claims.
- Large raw output remains ref-addressable, not repeatedly injected.

Acceptance:

- Unit test: critical Goal/DoD/current focus/requiredOutput/evidence refs survive context budget.
- Unit test: raw large evidence payload is replaced by summary + ref when over budget.
- Cache/request test: dynamic evidence compaction does not pollute C0 stable prefix.

## Implementation Order

1. E1 Evidence proof-strength model.
2. E2 Evidence quality derivation gate.
3. E3 Coverage ledger acceptance gate.
4. E4 Deterministic assessment trigger.
5. E5 Template/profile required evidence policy.
6. E6 StateChange / validation ordering enforcement.
7. E7 Validation graph contract narrowing.
8. E8 Progress event IO coalescing.
9. E9 Context and evidence compaction.

This order intentionally closes the authority path before optimizing context and IO. If E1-E4 are incomplete, later context or prompt changes can hide the symptom but will not fix convergence.

## Verification Plan

### Tests

- Evidence model and quality derivation unit tests.
- Coverage ledger acceptance fixtures for target-aligned and target-unaligned evidence.
- NextAction fixtures for evidence_ready assessment, missing state_change, missing validation and weak evidence downgrade.
- Template policy binding tests proving software-dev policy is data-driven and non-target templates are unaffected.
- Projection/progress IO tests for coalesced progress persistence.
- LLM context budget tests proving current focus, requiredOutput, accepted/rejected evidence refs and observed claims are preserved.

### Boundary Search

Run boundary search before and after implementation:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|TypeScript|provider|fallback|workspace title|ws-" workorch/internal/modules/kernel workorch/internal/modules/application/workorchd/biz/appservice workorch/internal/pkg
rg -n "targetRefs|observedClaims|proofKind|coverageAlignment|evidenceQuality|evidence_ready|state_change|validation" workorch/internal/modules/kernel workorch/internal/modules/blocks
rg -n "llm_stream/progress|progress|checkpoint|projection" workorch/internal/modules workorch/internal/services workorch/internal/pkg
```

If the first command finds business strings in core paths, every hit must be justified as test fixture or moved to template/profile/policy.

### Live Verification

After code closure:

- Start fresh service and record binary version, process PID and start time.
- Re-run two representative workspaces.
- Confirm each run produces target-aligned state_change evidence before validation for deliverable CWI.
- Confirm generic fallback validation no longer moves target CWI to `evidence_ready`.
- Confirm weak evidence triggers assessment/downgrade instead of long drift.
- Confirm `satisfied` only appears after target-aligned accepted evidence and assessment.
- Observe CPU/RSS/IO for at least one long LLM streaming step and compare persisted event/checkpoint growth.

## Non-Goals

- Do not loosen completion gate to mark weak evidence as satisfied.
- Do not solve low-quality convergence by adding prompt text alone.
- Do not parse business terms, command names, filenames or provider names in core.
- Do not make projection repair owner state.
- Do not hide resource pressure by increasing timeouts or suppressing diagnostics.

## Done Criteria

This closure plan is complete only when:

- `formal_valid` and `coverage_sufficient` are separate runtime concepts.
- Coverage ledger cannot enter `evidence_ready` from target-unaligned validation evidence.
- Current focus evidence_ready triggers deterministic assessment before broad drift.
- Software-dev required evidence policy is template/profile-bound and consumed by core as structured data.
- High-frequency progress IO is coalesced and projection hydration is bounded.
- Tests and boundary searches demonstrate WorkOrch core remains business-neutral.
