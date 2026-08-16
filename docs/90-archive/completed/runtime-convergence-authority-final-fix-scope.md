# Runtime Convergence Authority Final Fix Scope

Status: active

Type: closure-scope

Last Updated: 2026-06-25

Related:

- `../superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `runtime-coverage-convergence-state-machine-closure-plan.md`
- `runtime-contract-retry-convergence-closure-plan.md`
- `goal-dod-nextaction-iterative-delivery-plan.md`

## Purpose

This document is the final fix scope for the latest live deviation where two workspaces reached `waiting_decision` without stable positive convergence after replan/resume.

The goal is not to make the LLM always obey contracts. The goal is to make WorkOrch converge when LLM output is unstable:

```text
Coverage/Evidence Ledger
  -> deterministic NextAction
  -> current Stage Packet
  -> LLM/runtime contract
  -> candidate validation
  -> accepted/rejected evidence
  -> ledger recompute
```

No code change should start outside this document's scope unless a new Contract Audit adds a new deviation here.

## Contract Audit

- Scope: live workspaces `ws-7ced70e98ea50c43` and `ws-4e83810542ac4d65`, active runtime convergence and projection paths.
- Observed state:
  - `ws-7ced70e98ea50c43` is `waiting_decision`, active run `run-004-replan-001`, latest failed step `graph-004/001-patch-load-config`, failure `patch text_replace oldText not found`, latest event `/governance/auto/escalated` with reason `replan_lineage_budget_exhausted`.
  - `ws-4e83810542ac4d65` is `waiting_decision`, active run `run-003-replan-001-replan-001-replan-001-replan-001-replan-001-r-replan-014`, latest failed step `contract-retry-001-of-planning-001-2`, failure `decision requests a continuable candidate choice`, latest event `/governance/auto/escalated` with reason `replan_lineage_budget_exhausted`.
  - `ws-4e83810542ac4d65` persisted state is internally inconsistent: `coverageWorkItemContext.currentFocus = cwi-001/evidence_ready/need_assessment`, while `coverageFocus = focus-cwi-039`, `convergenceBudget.focusId = focus-cwi-039`, and `convergenceContract = need_evidence/acquire_concrete_failure_evidence`.
  - Both live projections show `gapBaseline.nextAction = schedule_step`, even when current focus implies `need_assessment`, `need_state_change`, or governed waiting.
- Expected state:
  - Runtime owner recomputes one authoritative NextAction from current ledger and policy.
  - Replan/resume/decision resolution refreshes GapBaseline, focus, convergence budget, convergence contract, Stage Packet, and projection from the same ledger snapshot.
  - LLM contract violations are recorded as rejected diagnostic evidence and retried only when the current deterministic NextAction still requires LLM-generated content.
- Deviation:
  - Runtime persists generic `schedule_step`, and older repair/focus/budget fields can override current ledger-derived state.
  - Projection prefers persisted stale gap baseline, so TUI/Web can display a misleading next action even when projection revision is current.
  - Replan successor can inherit stale `repairObjective`, `repairActionPackage`, `coverageFocus`, and exhausted budget.
- Owner: WorkOrch core. The issue is in runtime convergence state, coverage ledger synchronization, Stage Packet generation, and read model projection. It is not a template/Soul-specific rule, not a provider issue, and not a TUI local-state issue.
- Event path:
  - `resume/replan/decision resolve` command -> workorchd owner -> runtime scheduler -> orchestrator convergence -> run execution context -> step input -> LLM/block output -> contract gate -> evidence ledger -> coverage ledger -> projection.
- Contract gap:
  - Missing single owner function that recomputes coverage focus, NextAction, convergence contract, budget scope, and gap baseline atomically.
  - Runtime NextAction is still generic and not aligned with `goal-dod-nextaction-delivery-state-machine-design.md`.
  - Old repair continuation data can act as state authority instead of diagnostic lineage.
- Proposed change:
  - Implement a neutral convergence authority layer in orchestrator core.
  - Replace generic runtime `schedule_step` with deterministic NextAction classification.
  - Make replan/resume/dispatch paths refresh current Stage Packet from ledger before scheduling.
  - Make projection detect or normalize stale persisted baseline conflicts without becoming a second state machine.
- Boundary check:
  - Core must only consume structured fields such as status, required evidence kinds, accepted/rejected evidence refs, current focus, budget, ticket/approval state, and contract violation metadata.
  - Do not add template IDs, language names, file names, command names, provider names, or workspace IDs as behavior conditions.
- Verification:
  - Add focused unit tests for evidence-ready assessment, state-change gaps, focus/budget reset, replan successor refresh, projection baseline conflict, and bounded LLM contract violation retry.
  - Run boundary search for business-specific strings in core.
  - After code closure, run targeted orchestrator/readmodel tests, then full validation/e2e as requested.

## Final Fix Scope

### F1: Runtime Authoritative Convergence Refresh

Problem:

- Runtime stores `coverageWorkItemContext`, `coverageFocus`, `convergenceBudget`, `convergenceContract`, and `gapBaseline` as separately maintained fields.
- Live evidence proves these fields can drift across replan and retry.

Required closure:

- Add one orchestrator-owned refresh path that recomputes the current convergence snapshot from the current run ledger.
- The refresh must update the following fields from one source snapshot:
  - `coverageWorkItemContext`
  - `coverageFocus`
  - `convergenceBudget`
  - `convergenceContract`
  - `gapBaseline`
  - current Stage Packet inputs used by LLM/runtime dispatch
- The refresh must run before dispatch and after any step/evidence update that can change ledger state.

Likely code locations:

- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence_budget.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_context.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_event.go`

Acceptance:

- A run cannot persist `coverageWorkItemContext.currentFocus` for one item while `coverageFocus` and `convergenceBudget.focusId` reference another item.
- A run cannot persist `convergenceContract=need_evidence` when the current focus is `evidence_ready` with no missing evidence kinds.

### F2: Replace Generic Runtime `schedule_step`

Problem:

- Runtime `gapNextActionForRun` still returns `schedule_step` for non-terminal runs with open coverage.
- Design requires deterministic NextAction types such as `state_change`, `validate`, `assessment`, `request_decision`, `request_approval`, `complete`, and `fail`.

Required closure:

- Replace runtime authority `schedule_step` with structured NextAction derived from coverage state and governance state.
- Keep any legacy display string out of owner state. If a generic display label is needed, projection can derive it from the structured action.

Action mapping:

| Current ledger condition | NextAction |
| --- | --- |
| pending approval | `request_approval` |
| open decision with real external/governance dependency | `request_decision` |
| retryable rejected diagnostic and current action still needs LLM content | `repair` |
| no gap baseline | `assess_gap` |
| missing `state_change` | `state_change` |
| missing `validation` after state change exists | `validate` |
| evidence complete but not assessed | `assessment` |
| all required evidence accepted and assessed | `complete` |
| unrecoverable or exhausted by current focus budget | `fail` or governed blocked |

Likely code locations:

- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_state.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/coverage_summary.go`

Acceptance:

- Runtime tests prove `evidence_ready` maps to `assessment`, not `schedule_step` or `need_evidence`.
- Runtime tests prove missing `state_change` maps to `state_change`.
- Runtime tests prove pending decision/approval maps to governed wait actions.

### F3: Replan/Resume Stage Packet Rebuild

Problem:

- Replan can inherit stale `repairObjective`, `repairActionPackage`, old focus, and old budget.
- This makes the next LLM request ask for the wrong output even when the ledger has advanced.

Required closure:

- Treat `replan` as current-run replanning over current ledger, not as a continuation of old repair input.
- Treat `resume` as the canonical activation command for first run, restart recovery, decision/approval release, and recoverable active runs.
- Before scheduling after replan/resume, rebuild:
  - latest GapBaseline
  - current NextAction
  - current Stage Packet
  - current requiredOutput
  - current allowed/forbidden actions
- Old decision text, old execution graph, old repair package, and old rejected output may be included only as diagnostic lineage, not as current authority.

Likely code locations:

- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_run_state.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`

Acceptance:

- A replan successor with current focus `evidence_ready` must produce an assessment Stage Packet.
- A replan successor must not inherit `acquire_concrete_failure_evidence` when current ledger no longer needs evidence acquisition.
- `continue` must not reappear as a separate canonical path; any compatibility text must route to `resume`.

### F4: Focus/Budget Scope Reset

Problem:

- Live state shows `coverageWorkItemContext.currentFocus`, `coverageFocus`, and `convergenceBudget.focusId` can disagree.
- Budget exhaustion can then apply to the wrong focus and force premature decision escalation.

Required closure:

- Make focus change a scoped state transition.
- When current focus changes, reset budget counters that are scoped to the previous focus.
- Preserve only neutral budget limits such as max rounds; do not preserve previous focus attempts, no-delta counters, repeated signatures, or last decision reason.

Likely code locations:

- `internal/modules/kernel/orchestrator/biz/runtime_convergence_budget.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_context.go`
- `internal/modules/kernel/orchestrator/biz/runtime_acceptance_slices.go`

Acceptance:

- Tests prove focus switch from `focus-cwi-039` to `cwi-001` updates budget scope and clears stale escalation counters.
- Tests prove budget escalation is evaluated only for the current focus.

### F5: Contract Generation From NextAction

Problem:

- `BuildConvergenceContract` can still be driven by old input fields and continuation packages before the current ledger state is fully considered.
- This allows stale `repairObjective` to override current deterministic action.

Required closure:

- Make deterministic NextAction the primary input for convergence contract generation.
- Use old repair/continuation/rejected output only as diagnostics, retry hints, source refs, or target evidence.
- Only generate `acquire_concrete_failure_evidence` when the current NextAction is evidence acquisition and no current accepted evidence satisfies the required kind.
- Only generate executable state-changing required output when the current NextAction is `state_change`.
- Only generate assessment-only required output when the current NextAction is `assessment`.

Likely code locations:

- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation_recovery*.go`

Acceptance:

- Tests prove LLM asking for more evidence does not override `assessment` when ledger evidence is already ready.
- Tests prove contract violation retry keeps the selected action narrow and does not mutate C0/C1/C2 request partitions.

### F6: Projection Conflict Visibility

Problem:

- Projection currently prefers persisted `gapBaseline` even when it conflicts with current focus.
- TUI/Web can display stale next action despite up-to-date projection revision.

Required closure:

- Projection remains read-only and does not mutate owner state.
- Projection must either:
  - display owner-provided current structured NextAction when fixed by F1/F2, or
  - expose a diagnostic when persisted baseline conflicts with derived focus state.
- TUI/Web must continue consuming projection; they must not infer state locally.

Likely code locations:

- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/coverage_summary.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/run_snapshot_derived_read_models.go`
- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore`
- `internal/pkg/workorchport`

Acceptance:

- Projection test proves stale persisted `schedule_step` does not silently hide `evidence_ready -> assessment`.
- Workspace snapshot exposes enough diagnostic detail to audit mismatch without TUI logic changes.

### F7: Bounded LLM Violation Recovery

Problem:

- LLM output will continue to violate contracts.
- If runtime keeps asking the LLM to restate a deterministic transition, convergence can loop forever.

Required closure:

- Retry only when the selected NextAction requires LLM-generated candidate content.
- If ledger can determine the next action without new LLM content, advance deterministically.
- Invalid LLM output must be persisted only as rejected diagnostic evidence.
- Rejected diagnostics must not enter accepted evidence, artifact/result, or high-value memory.

Likely code locations:

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_llm_context.go`

Acceptance:

- Tests prove wrong wrapper but safe assessment evidence is either normalized safely or retried once under assessment contract.
- Tests prove continuable `blocked_on_decision` is rejected unless it has a structured external/governance dependency.
- Tests prove repeated same-rule violation escalates with current focus scope and does not pollute accepted evidence.

### F8: Live Sample Regression Fixtures

Problem:

- The latest live deviations are reproducible through persisted snapshots and should become regression samples.

Required closure:

- Add compact fixtures or focused test builders for:
  - `ws-4e...`: `evidence_ready` current focus with stale evidence-acquisition contract.
  - `ws-7ced...`: patch conflict and exhausted repair lineage should show a real governed blocker.
- Tests must not hardcode live workspace IDs into production logic. IDs may appear only in test names, fixtures, or documentation.

Likely code locations:

- `internal/modules/kernel/orchestrator/biz/runtime_*_test.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/*_test.go`

Acceptance:

- Tests fail on the current deviation and pass only after F1-F7 are implemented.
- Test assertions verify structured state, not business text or template-specific content.

## Explicit Non-Scope

Do not solve this closure by:

- Adding template ID, Soul ID, language, file name, command, provider, or workspace ID branches to core.
- Weakening contract validation so invalid LLM output can execute.
- Expanding prompts to ask the LLM to self-manage state transitions.
- Making TUI/Web infer decision, approval, coverage, or NextAction state locally.
- Using watchdog recovery to mask owner-state drift.
- Treating old decision resolution, old graph, old memory, or old repair package as current authority.

## Implementation Order

1. F8 test fixtures for the two observed classes.
2. F2 deterministic NextAction classification in runtime owner.
3. F1 convergence refresh that writes aligned owner state.
4. F4 focus/budget scoped reset.
5. F3 replan/resume Stage Packet rebuild.
6. F5 contract generation from NextAction.
7. F6 projection conflict visibility.
8. F7 bounded LLM violation recovery refinements.
9. Boundary search and targeted tests.
10. Full validation and e2e after local tests pass.

## Required Verification Commands

Run targeted tests first:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
```

Then run boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "stateChangingOutputRequired|repairObjective|providerTimeoutStateChangingRetry|assessmentOnly|runtimeRepairFollowUp|currentProgressDigest|memoryRecall|Cache-Control|cache_control" internal/modules/blocks/llm internal/modules/kernel
rg -n "definitionChangeImpact|coverageTargetSet|repairActionPackage|processEvidenceTimeline|requiredOutput|blocked_on_decision|contextGate" internal/modules/kernel internal/modules/blocks/llm
```

After targeted closure:

```bash
go test ./... -count=1
```

Full e2e/live verification should be run only after code and unit tests close the targeted deviations.

## Closure Gate

Each implementation commit must update this checklist:

| Item | Status | Evidence |
| --- | --- | --- |
| F1 runtime convergence refresh | code-closed | `ensureGapBaselineBeforeDispatch` now refreshes convergence authority before dispatch; `TestRefreshRunConvergenceAuthorityAlignsFocusAndBudgetToCurrentCoverage` covers focus/budget alignment. |
| F2 deterministic NextAction | code-closed | Runtime `gapNextActionForRun` no longer emits generic `schedule_step`; tests cover `validate`, `assessment`, decision, and approval mappings. |
| F3 replan/resume Stage Packet rebuild | code-closed | Replan successor budget reset is protected through `replanBudgetResetPending`; gap baseline is refreshed before dispatch; full `go test ./... -count=1` passed after the change. |
| F4 focus/budget scope reset | code-closed | Focus changes reset scoped counters while preserving neutral limits; replan successor first update starts from reset budget. |
| F5 contract generation from NextAction | code-closed | `BuildConvergenceContract` now lets structured NextAction preempt stale evidence/state-change fields; `TestBuildConvergenceContractNextActionAssessmentPreemptsStaleEvidenceAcquisition` covers the live drift class. |
| F6 projection conflict visibility | code-closed | Read model normalizes stale/generic persisted `gapBaseline.nextAction` and exposes `stale_persisted_next_action` diagnostics without mutating owner state. |
| F7 bounded LLM violation recovery | code-closed | Contract-violation retries are bounded, repeated signatures exhaust the retry budget, and rejected diagnostics stay out of accepted evidence. Coverage exists in `runtime_contract_violation_test.go`; full `go test ./... -count=1` passed. |
| F8 live sample regression fixtures | code-closed | Regression coverage now includes evidence-ready assessment, stale persisted projection normalization, and patch-conflict / governed blocker cases in focused tests. |
| Boundary search | code-closed | Production core scan found no template-specific business strings entering kernel/orchestrator/appservice logic; remaining hits were expected generic/runtime symbols or tests/docs. |
| Targeted tests | passed | `go test ./internal/modules/kernel/orchestrator/biz -count=1`; `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1`; `go test ./internal/modules/application/workorchd/biz/appservice -count=1`. |
| Full tests/e2e | passed | `go test ./... -count=1` |

## Done Criteria

The closure is complete only when:

- Runtime no longer persists generic `schedule_step` as the authoritative next action.
- Current focus, coverage focus, convergence budget, convergence contract, gap baseline, and Stage Packet are aligned for every dispatch.
- Replan/resume rebuilds the current Stage Packet from ledger and does not let stale repair packages override current state.
- LLM contract violations are tolerated through rejected diagnostic evidence and bounded retry without allowing invalid side effects.
- Projection exposes the same authoritative state that runtime owns, with diagnostics for any detected conflict.
- Tests cover the two observed live deviation classes.
- Boundary search proves core remains business-neutral.
