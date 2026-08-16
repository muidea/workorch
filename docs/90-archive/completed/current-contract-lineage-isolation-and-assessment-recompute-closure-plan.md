# Current Contract Lineage Isolation And Assessment Recompute Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-27

Related:

- `../superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `convergence-authority-single-source-closure-plan.md`
- `runtime-decision-eligibility-narrow-retry-closure-plan.md`
- `projection-event-io-resource-closure-plan.md`

## Purpose

This document records the latest live audit conclusion and the remaining closure
scope for WorkOrch runtime convergence. The target is to prevent historical
contract lineage from competing with the current convergence authority, and to
ensure `coverage_assessment_incomplete` is handled by runtime authority recompute
instead of repeated LLM assessment-only retries.

The closure must keep WorkOrch core business-neutral. Software-development
specific strategy remains in workspaceTemplate/profile/Soul/policy data. Core
changes may only use structured convergence state, evidence kinds, contract
rules, governance state, retry budget, and scoped runtime events.

## Contract Audit

- Scope:
  - `ws-4e83810542ac4d65 / run-005 / contract-retry-001-of-planning-001-5`
  - `ws-7ced70e98ea50c43 / run-004-...-replan-010 / repair-009-of-planning-001`
  - Runtime convergence, decision gate, contract violation recovery, Stage
    Packet rendering, and assessment-only retry handling.
- Observed state:
  - Service projection is aligned and ready.
  - Both live workspaces are `waiting_decision`; TUI/Web state matches backend
    projection, so the active deviation is not a projection refresh issue.
  - `ws-4e838...` opened a decision because the LLM claimed a contradiction
    between forced-narrow validation evidence and state-changing patch output.
  - The actual request carried both the current forced-narrow validation
    correction contract and historical `expectedContract` state-changing
    contract. The LLM treated historical expected contract as current authority.
  - `ws-7ced...` repeated ten `verification /
    complete_dod_goal_coverage_assessment / assessmentOnly=true` calls for
    `coverage_assessment_incomplete`, then escalated to decision. The coverage
    summary still had 62 `evidence_ready` items and unresolved deliverable
    state-change gaps.
- Expected state:
  - Current `NextAction`, `convergenceContract`, `repairActionPackage.requiredOutput`,
    and Stage Packet are derived only from the latest convergence authority
    snapshot.
  - Historical `expectedContract`, old repair package, old graph, previous
    decision, rejected output, and contract violation recovery details are
    diagnostic lineage only.
  - If current required output is mechanically continuable and no real external
    or runtime dependency is declared, LLM `requiredDecision` must not create an
    open ticket.
  - `coverage_assessment_incomplete / recompute_authority` must make runtime
    recompute authority and dispatch the next deterministic action. It must not
    keep submitting the same global assessment-only request to LLM.
- Deviation:
  - Historical `expectedContract` can still influence `BuildConvergenceContract`
    through contract-violation recovery and pull the current contract back to
    `need_state_change`.
  - Decision gate can accept a model-created blocker caused by stale or
    diagnostic-only contract data.
  - Assessment retry loop still lets LLM own the state transition that should be
    owned by the runtime convergence authority.
- Owner: WorkOrch core.
- Event path:
  - LLM output or contract violation -> contract gate -> rejected diagnostic
    evidence -> runtime convergence authority recompute -> Stage Packet ->
    next LLM/block dispatch -> owner state -> projection.
- Contract gap:
  - Missing hard isolation between current authoritative contract and diagnostic
    historical contract.
  - Missing decision gate check that compares LLM blocker against current
    authority continuability.
  - Missing runtime-owned recompute path for repeated
    `coverage_assessment_incomplete`.
- Boundary:
  - No core logic may branch on template ID, Soul ID, workspace title, language,
    tool name, file name, command text, provider brand, or live workspace ID.

## Design Rules

### Rule 1: Current Authority Is The Only Dispatch Authority

Before provider or block dispatch, the following fields must come from the same
current authority snapshot:

- current focus
- deterministic `NextAction`
- `convergenceContract`
- Stage Packet
- `repairActionPackage.requiredOutput`
- top-level required output fields
- gap baseline

Any stale field from previous step input, replan continuation, contract retry,
provider retry, rejected output, decision history, or memory recall must be
excluded from current authority and retained only under diagnostic lineage.

### Rule 2: Historical Expected Contract Is Diagnostic-Only

`contractViolationRecovery.expectedContract` is useful for audit and retry
diagnostics, but it must not override the current correction contract or current
authority-derived required output.

Required behavior:

- The request renderer must mark old expected contract as diagnostic lineage or
  otherwise make it unambiguous that it is not authoritative.
- `BuildConvergenceContract` must not use `expectedContract` to select
  `need_state_change` when the current correction contract or current authority
  selected validation/evidence/assessment.
- Contract retry must preserve stage semantics from current authority, not from
  stale expected contract.

### Rule 3: Decision Tickets Require Real Non-Continuability

LLM `requiredDecision` is admitted only when current authority cannot continue
automatically or when the decision declares a real external/runtime dependency.

Reject or convert to repair when:

- current contract is `need_state_change` and accepted state-changing kinds are
  available;
- current contract is `need_validation` and bounded validation evidence can be
  produced;
- current contract is `need_evidence` and bounded evidence acquisition can be
  produced;
- current correction contract explicitly forbids
  `decision_request_when_runtime_can_continue`;
- the blocker is based on contradictory or stale diagnostic fields rather than
  the current required output.

### Rule 4: Assessment Incomplete Triggers Runtime Recompute

`coverage_assessment_incomplete` with `recoverability=recompute_authority` is a
runtime instruction, not an LLM instruction.

Required behavior:

- Runtime recomputes current focus and deterministic `NextAction` from the
  coverage/evidence ledger.
- If the ledger has missing `state_change`, dispatch state-changing action.
- If the ledger has missing `validation`, dispatch bounded validation action.
- If the focus has evidence ready and only needs assessment, dispatch a bounded
  focus-scoped assessment.
- If the same assessment output shape fails repeatedly, do not issue another
  identical global assessment-only request. Recompute, narrow focus, switch
  action, or escalate only when current authority truly requires governance.

### Rule 5: Retry Budget Is Bound To Rule, Focus, And Output Shape

Bounded retry remains necessary because LLM output is not guaranteed to follow
the contract. The retry budget must be scoped by:

- workspaceId
- runId
- current authority focus
- violated rule
- output shape or failure signature

After the scoped budget is exhausted, runtime must not blindly create a decision
ticket. It first recomputes authority. A decision ticket is valid only if the
recomputed authority is governance or no automatic/managed narrowed path exists.

## Required Code Closure Items

### C1. Isolate Historical Expected Contract

- Target:
  - `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
  - LLM request rendering paths that include `contractViolationRecovery`
- Change:
  - Make `contractViolationRecovery.expectedContract` diagnostic-only for
    contract building and dispatch.
  - Ensure `contractViolationRecoveryStateChangingContract` cannot promote a
    stale expected contract to current `need_state_change` when current
    correction/authority selected validation, evidence, assessment, governance,
    complete, or fail.
  - Preserve expected contract in diagnostics with an explicit non-authoritative
    marker.
- Verification:
  - Regression sample matching `ws-4e838...`: current correction requires
    bounded validation evidence, historical expected contract requires state
    change, and LLM returns decision claiming contradiction. Runtime must not
    open decision.

### C2. Strengthen Decision Gate Against Stale-Contract Blockers

- Target:
  - `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_blocks.go`
- Change:
  - Before creating a decision ticket, compare the blocker with current
    authority continuability.
  - Treat blockers based on stale/diagnostic expected contract as rejected
    diagnostic evidence.
  - Keep real governance blockers for external dependency, runtime
    configuration, approval requirement, or explicitly unresolvable missing
    information.
- Verification:
  - Unit tests for state-change, validation, and evidence contracts where LLM
    asks for human choice but current authority can continue.
  - Unit test for real runtime/external dependency that must still create a
    decision ticket.

### C3. Runtime-Owned `coverage_assessment_incomplete` Recompute

- Target:
  - `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`
  - coverage authority / Stage Packet construction helpers
- Change:
  - Convert `coverage_assessment_incomplete / recompute_authority` into a
    runtime authority refresh before the next dispatch.
  - If recompute selects validation/state-change/evidence, clear stale
    `assessmentOnly` and stale assessment required output.
  - If recompute selects assessment, bound it to the current focus rather than
    global coverage.
- Verification:
  - Regression sample matching `ws-7ced...`: repeated incomplete assessment with
    evidence-ready backlog and source-change gap must not produce another
    identical global assessment-only retry.

### C4. Stage Packet Narrowing And Diagnostic Namespace

- Target:
  - Stage Packet builders
  - LLM input/request rendering
  - context budget compaction
- Change:
  - Render only current action contract as authoritative.
  - Put previous contract/rejected output/decision history under a diagnostic
    namespace with non-authoritative semantics.
  - Keep current Goal/DoD, coverage ledger, current focus, accepted/rejected
    evidence refs, and required output protected from truncation.
- Verification:
  - Request snapshot tests prove current `requiredOutput` appears once as the
    dispatch authority, while old `expectedContract` is diagnostic-only.
  - Stable prefix hash must not change due to dynamic retry/diagnostic lineage.

### C5. Live Verification And Resource Observation

- Target:
  - e2e/live verification scripts or documented manual audit steps
  - projection/resource observability
- Change:
  - Add verification steps for both live failure shapes.
  - Observe projection revision, event revision, process RSS, and IO deltas
    during retry loops.
- Verification:
  - No repeated same-shape assessment-only loop beyond scoped retry budget.
  - No invalid human decision for mechanically continuable contract.
  - Projection remains aligned and TUI/Web prompt mode reflects owner state.
  - IO growth is bounded by actual state transitions, not by repeated identical
    retry loops.

### C6. Keep `nextAction` And `convergenceContract` Atomic

- Target:
  - `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
  - dispatch input preparation and validation
- Change:
  - Applying a recomputed convergence contract must also update
    `convergenceAuthority.nextAction` and `gapBaseline.nextAction`.
  - If a stale or broad `state_change` action is refined into prerequisite
    evidence acquisition, the dispatch input must carry `acquire_evidence` or
    `acquire_target_snapshot`, not the old `state_change` action.
  - Dispatch validation must recognize evidence-acquisition actions as
    `need_evidence`.
- Verification:
  - Regression sample matching the post-deploy `ws-4e838...` failure:
    `nextAction=state_change` plus recomputed `contractState=need_evidence`
    must not fail with `runtime_internal_contract_misalignment`.
  - Regression sample matching `ws-7ced...`: recompute must clear stale
    assessment action when the current contract selects non-assessment.

## Acceptance Criteria

- `ws-4e838...` class:
  - Current validation/evidence correction contract is not overridden by stale
    state-changing expected contract.
  - LLM stale-contract contradiction decision is rejected or converted to
    runtime repair, not opened as a user decision.
- `ws-7ced...` class:
  - `coverage_assessment_incomplete` performs runtime authority recompute.
  - Runtime does not issue ten identical global assessment-only calls for the
    same focus/rule/output shape.
  - If coverage ledger requires state-change or validation, next dispatch
    matches that requirement.
- General:
  - Current authority remains the single source for dispatch.
  - `nextAction`, `convergenceContract`, Stage Packet, and required output are
    applied as one atomic dispatch contract.
  - Historical lineage remains available for audit but cannot drive state.
  - WorkOrch core remains business-neutral.
  - Tests cover both regression samples before live verification.

## Non-Goals

- Do not loosen completion gates to accept incomplete DoD coverage.
- Do not add template, language, file, command, or provider-specific branches in
  core.
- Do not solve this by increasing prompt size or asking LLM to choose the state
  transition.
- Do not use watchdog or compensation as the primary fix for deterministic
  contract recompute defects.

## Closure Progress

### 2026-06-27

- C1 code-closed:
  - `contractViolationRecovery.expectedContract` is now treated as diagnostic
    lineage for convergence state selection when the current correction contract
    narrows to validation, evidence, assessment, recompute, or JSON-shape output.
  - `contractViolationRecoveryStateChangingContract` no longer promotes a stale
    expected state-changing contract over the current non-state-changing
    correction contract.
- C2 code-closed:
  - Added a regression case proving a stale expected-contract contradiction does
    not create a decision when the current validation contract can continue.
  - Existing real external/runtime dependency decision behavior remains covered.
- C3 code-closed:
  - `coverage_assessment_incomplete / recompute_authority` can preempt stale
    `nextAction=assessment` when the current coverage work item still has
    actionable evidence gaps.
  - Added a regression case proving assessment-only is cleared when recompute
    selects a non-assessment contract.
- C4 partially closed:
  - `contractViolationRecovery` now carries
    `expectedContractRole=diagnostic_lineage` and
    `expectedContractAuthoritative=false` when an expected contract is present.
  - Retry guidance now explicitly names `correctionContract` and current
    `repairActionPackage.requiredOutput` as authoritative, and expected contract
    as diagnostic lineage.
- C6 code-closed:
  - `ApplyConvergenceContract` now synchronizes `convergenceAuthority.nextAction`
    and `gapBaseline.nextAction` from the applied contract state.
  - Dispatch validation now recognizes `repair`, `acquire_evidence`, and
    `acquire_target_snapshot` as `need_evidence` actions.
  - Added regression coverage for `state_change -> need_evidence` recompute and
    dispatch validation.
- Verification completed:
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'Test(BuildConvergenceContractDoesNotPromoteDiagnosticExpectedContractOverNarrowValidation|CoverageAssessmentIncompleteRecomputePreemptsStaleAssessmentAuthorityWithEvidenceGap|ShouldCreateDecisionTicketBlocksMechanical|ShouldCreateDecisionTicketKeeps|RebuildDispatchInputUsesAuthorityAfterCoverageAssessmentIncomplete|CoverageAssessmentIncompleteSkipsContractRetryForRuntimeRepair|ContractViolation)'`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'Test(CoverageAssessmentIncompleteRecomputePreemptsStaleAssessmentAuthorityWithEvidenceGap|CoverageAssessmentIncompleteStateChangeRecomputeSyncsEvidenceNextAction|BuildConvergenceContractDoesNotPromoteDiagnosticExpectedContractOverNarrowValidation|BuildConvergenceContractStateChangeRequiresTargetSnapshotAcquisition|BuildConvergenceContractStateChangeUsesExistingTargetSnapshot)'`
  - `go test ./internal/modules/kernel/orchestrator/biz`

## Suggested Implementation Order

1. Add regression tests for `ws-4e838...` stale expected-contract decision.
2. Fix expected-contract isolation and decision gate.
3. Add regression tests for `ws-7ced...` repeated assessment incomplete.
4. Fix runtime-owned `coverage_assessment_incomplete` recompute and stale
   assessment field clearing.
5. Keep `nextAction`, `convergenceContract`, Stage Packet, and required output
   synchronized after every recompute.
6. Add request rendering assertions for diagnostic-only old contracts.
7. Run targeted orchestrator tests, appservice projection tests, build, and live
   verification.
