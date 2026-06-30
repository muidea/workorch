# Runtime Live Authority Loop Full Chain Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-30

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-required-output-authority-and-target-snapshot-closure-plan.md`
- `runtime-current-action-projection-convergence-closure-plan.md`
- `runtime-target-snapshot-authority-and-projection-closure-plan.md`
- `runtime-progress-quality-resource-closure-plan.md`

## Purpose

本文承接 2026-06-30 live verification 结果，固定当前两个 workspace
无法稳定正向推进的全链路收口范围。

目标不是继续针对某个 live workspace 或某次 LLM 输出做局部补丁，而是把
当前暴露出的 runtime authority、local follow-up、target snapshot、budget、
projection、evidence 和资源热路径统一收敛到长期设计要求：

```text
Coverage/Evidence Ledger
  -> Deterministic NextAction
  -> single current action authority snapshot
  -> Stage Packet / RequiredOutput / CurrentActionContract from same snapshot
  -> Candidate generation or runtime-managed action
  -> Contract gate
  -> accepted/rejected evidence
  -> bounded retry / blocker / fail-fast / next deterministic action
  -> Projection/ViewModel
```

当前阶段不考虑兼容性。若现有实现、测试或端侧行为与本文冲突，应按本文
逐项收口。

## Contract Audit

- Scope:
  - Live workspaces:
    - `ws-db040d24a190f6e5` / `run-010`
    - `ws-705249a8d7816330` / `run-010-replan-001-replan-001`
  - Runtime convergence authority, dispatch input rebuild, LLM enrichment,
    currentActionContract, graph expansion follow-up, target snapshot
    acquisition, retry budget, resource hot path and projection boundary.
- Observed state:
  - Both workspaces are `running`, projection is ready, and there is no
    decision or approval blocker.
  - Coverage does not converge:
    - `ws-db040d24a190f6e5`: total 40, satisfied 0, failed 32, open 8.
    - `ws-705249a8d7816330`: total 89, satisfied 0, failed 41, open 48.
  - Recent steps repeat:
    - read-only `file/list` / `file/read`
    - `llm_execution_graph_plan`
    - `graph follow-up expansion limit exceeded`
    - failed `llm_dod_assessment`
    - repair
    - another read-only graph
  - `ws-db040d24a190f6e5` reached repeated repair numbers above 20 for the same
    focus and same graph expansion failure family.
  - `ws-705249a8d7816330` produced a step input where:
    - `currentActionContract.nextAction.kind = state_change`
    - `convergenceContract.state = need_evidence`
    - `convergenceContract.runtimeStageMode = target_snapshot_acquisition`
    - active `requiredOutput` is read-only and forbids `patch/apply`
  - Run owner context for both workspaces still reports
    `convergenceAuthority.nextAction.kind = state_change`, while dispatched
    step input can be downgraded to target snapshot evidence acquisition.
  - CPU and memory are unhealthy:
    - `workorch` process observed around 246% CPU.
    - RSS observed around 3.4 GB.
    - `.workorch-data` continues growing through repeated failed steps.
- Expected state:
  - A single current authority snapshot must drive NextAction,
    convergenceContract, stagePacket, repairActionPackage.requiredOutput,
    currentActionContract and top-level requiredOutput mirror.
  - If current authority says `state_change`, provider-facing requiredOutput
    must expose a state-changing route, or dispatch must fail-fast as runtime
    internal contract misalignment before LLM execution.
  - If runtime selects `acquire_target_snapshot`, currentActionContract must
    say `acquire_target_snapshot`, and the action must be bounded.
  - Repeated failure with no coverage delta must be converted to a governed
    blocker, fail-fast, or deterministic next action within budget.
  - TUI/Web/CLI must only display projection and submit scoped commands.
  - Projection and resource paths must not amplify failure loops through full
    snapshot or full run JSON scans.
- Deviation:
  - Dispatch input can contain mutually inconsistent authority fields.
  - Local assessment and graph-limit follow-up can remove owner authority and
    later be realigned into repair/target-snapshot semantics.
  - target snapshot acquisition repeats on abstract refs such as `cwi-*`,
    `goal:*`, `deliverable:*` and `source_change`.
  - Budget tracking is configured but does not terminate repeated no-delta
    failure loops.
  - Hot paths repeatedly list full run steps, load JSON input/output and clone
    large run execution context.
- Owner:
  - Primary owner is WorkOrch core runtime.
  - Projection and TUI/Web remain secondary owners for display and command
    boundary only.
  - Software development behavior remains template/Soul/policy owned and must
    not be hardcoded in core.
- Event path:
  - TUI/Web/REST command
  - workorchd owner/EventHub
  - runtime scheduler
  - authority refresh
  - dispatch input rebuild
  - LLM enrichment
  - final dispatch gate
  - runtime block/LLM
  - contract gate/finalization
  - evidence ledger
  - coverage recompute
  - projection/ViewModel
- Contract gap:
  - Current action contract can still be assembled from mixed sources.
  - Final dispatch gate does not fully validate the post-enrichment input as one
    current authority snapshot.
  - Graph-limit and local assessment paths can become a second action selector.
  - Target snapshot acquisition has no bounded result for abstract target refs.
  - Retry budget is not anchored to focus + failure signature + coverage delta.
- Boundary check:
  - This plan uses only generic runtime concepts:
    - workspace/run/step scope
    - coverage work item
    - evidence kind
    - NextAction
    - requiredOutput
    - targetRef
    - failure signature
    - projection ViewModel
  - Core fixes must not branch on template ID, Soul ID, language, command,
    provider, file name, workspace title or live workspace ID.

## Coverage Of This Audit

### Covered

- Run owner convergence authority.
- Dispatch input rebuild.
- Stage Packet and requiredOutput mirrors.
- CurrentActionContract derivation and validation.
- LLM enrichment post-processing.
- Graph expansion limit follow-up.
- Target snapshot acquisition behavior for readable and abstract target refs.
- Retry/budget symptoms.
- CPU/RSS and repository hot path evidence.

### Not Yet Fully Covered

- TUI/Web/CLI command ingress for `resume`, `restart`, `replan`, decision and
  approval.
- EventHub delivery and owner consumption after command ingress.
- Projection list/detail/ViewModel same-source invariants.
- Evidence ledger metadata completeness for all accepted/rejected paths.
- Result/artifact/completion gate final authority.
- Provider request rendering and cache partition stability.
- Managed validation resource lifecycle.
- workspaceTemplate/Soul/policy consistency for software-dev task granularity.
- Repository/read-model indexes needed to remove hot full JSON reads.

The current evidence is sufficient to fix the P0 runtime loop blocker, but not
enough to claim the whole WorkOrch execution lifecycle is fully audited.

## Root Cause

### R1: CurrentActionContract Can Read A Different Authority Than The Step

`currentActionContractFromInput` can fall back to `run.ExecutionContext` when
the step input lacks `convergenceAuthority` or `gapBaseline`.

This causes a split-brain contract:

- step input has `convergenceContract.state = need_evidence`
- step input has read-only target snapshot requiredOutput
- currentActionContract is rebuilt with run-level `nextAction = state_change`

The provider-facing input then carries contradictory instructions.

### R2: Final Dispatch Gate Does Not Treat CurrentActionContract As A Full Mirror

The final dispatch gate checks requiredOutput mirrors, but it does not fully
validate `currentActionContract.nextAction` and
`currentActionContract.requiredOutput` as part of the same semantic set.

If `convergenceNextActionKind(nil, input)` returns empty or a different source,
the gate can miss a `currentActionContract` contradiction.

### R3: Enrichment Can Mutate Authority Fields After Rebuild

`enrichLLMStepInputContext` runs after the step is created. It can call
target-snapshot realignment and reapply convergence contracts.

Any post-rebuild mutation of:

- `convergenceContract`
- `repairActionPackage`
- `stagePacket`
- `requiredOutput`
- `currentActionContract`

must be followed by one final authority mirror rebuild and dispatch gate.

### R4: Local Assessment Follow-Up Can Become A Second Action Selector

Graph expansion limit creates assessment-only follow-up inputs. The local
assessment path can strip authority fields. Later runtime enrichment can realign
that input into target snapshot acquisition.

That mixes:

- local assessment semantics
- target snapshot acquisition semantics
- run-level state_change semantics

inside one step.

### R5: Target Snapshot Acquisition Is Unbounded For Abstract Targets

Runtime-managed snapshot acquisition only handles readable file refs. Abstract
refs such as `cwi-*`, `goal:*`, `dod:*`, `deliverable:*` and `source_change`
are intentionally not readable.

The current loop still asks LLM to produce read-only graph output for these
abstract refs. That produces root listing and file reading, not target evidence
that can unlock state change.

### R6: Budget Gate Does Not Consume The Live Failure Loop

The run budget contains values such as:

- `maxRepeatedFailureSignature = 3`
- `maxRepairAttemptsPerFocus = 4`
- `maxNoCoverageDeltaRounds = 3`

Live evidence exceeds those limits. The likely reason is that budget counting
is keyed by changing step IDs, repair windows, graph roots or local follow-up
shape rather than:

```text
runId + currentFocus coverageWorkItemIds + failure signature + no coverage delta
```

### R7: Resource Hot Paths Amplify The Loop

The repeated loop triggers expensive paths:

- full `StepRepository.ListByRun`
- loading step `input.json` / `output.json`
- large run execution context clone/update
- coverage text matching with LCS
- repeated projection/event updates

This turns a logical convergence bug into sustained CPU, memory and IO pressure.

## Closure Design

### Principle 1: One Current Authority Snapshot Per Dispatch

Every provider-facing LLM step must carry exactly one active authority snapshot.

Allowed active fields:

- `currentActionContract`
- compact trace refs to the authority snapshot
- current focus
- accepted/rejected evidence refs for this focus
- latest rejected diagnostic

Diagnostic-only fields may exist, but they must be under explicit diagnostic
lineage and must not select the current action.

### Principle 2: CurrentActionContract Must Not Fallback To Run State

`currentActionContract` is a step input contract. It must be derived from the
post-rebuild, post-enrichment step input.

Rules:

- Do not read `run.ExecutionContext` inside `currentActionContractFromInput`
  to choose `nextAction`.
- If the step input lacks current authority, derive from the step
  `convergenceContract`.
- If derivation is impossible, omit `currentActionContract` and fail the final
  dispatch gate for LLM workspace execution.

### Principle 3: Final Dispatch Gate Is The Last Writer Boundary

After enrichment, before dispatch to runtime/LLM:

```text
normalize current authority
  -> synchronize requiredOutput mirrors
  -> rebuild currentActionContract
  -> validate all mirrors and action semantics
  -> persist step input
```

No code after this point may mutate active contract fields.

### Principle 4: Graph Limit Follow-Up Is Assessment-Only Or Authority-Recomputed

Graph expansion limit follow-up has only two valid outcomes:

1. Assessment-only closure:
   - no owner authority is mixed in
   - no state change
   - no target snapshot acquisition
   - result is accepted assessment evidence, rejected diagnostic evidence, or
     governed blocker
2. Authority recompute:
   - abort the local assessment contract
   - refresh owner authority
   - generate a new step from current NextAction

It must not produce a step that is both local assessment and target snapshot
repair.

### Principle 5: Target Snapshot Acquisition Must Be Finite

For target snapshot acquisition:

- readable target refs:
  - runtime-managed acquisition should read bounded targets and write evidence.
- abstract target refs:
  - runtime must not repeatedly ask LLM to list/read root.
  - runtime must choose one deterministic result:
    - derive readable candidates from existing manifest/evidence if available;
    - convert to coverage-scoped greenfield state change when policy allows;
    - request governance if external information is missing;
    - fail-fast as internal contract gap if the target is impossible to bind.

### Principle 6: Budget Gate Is Focus-Scoped And Delta-Aware

Budget gate must evaluate before scheduling a new LLM step:

```text
focusKey = runId + sorted(coverageWorkItemIds)
failureKey = failureClass + normalizedFailureSignature
deltaKey = coverageLedgerHashBefore == coverageLedgerHashAfter
```

If the same focus and failure key repeats without accepted evidence delta, the
runtime must not dispatch another equivalent LLM step.

### Principle 7: Resource Hot Paths Must Use Summaries Or Indexes

Runtime scheduling and failure loops must not repeatedly load all step
input/output JSON.

Allowed hot-path data:

- step summaries
- indexed latest active step
- indexed latest failure signature by focus
- evidence ledger records
- coverage ledger summaries
- bounded recent full step data by explicit need

## Code Scope

### P0.1 CurrentActionContract Authority

Target files:

- `internal/modules/kernel/orchestrator/biz/runtime_current_action_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`

Tasks:

1. Remove run-level nextAction fallback from `currentActionContractFromInput`.
2. Derive current action from step input only:
   - `convergenceAuthority.nextAction`
   - `gapBaseline.nextAction`
   - `convergenceContract` state/mode
3. If `convergenceContract.state = need_evidence` and mode is
   `target_snapshot_acquisition`, currentActionContract nextAction must be
   `acquire_target_snapshot`.
4. Include `currentActionContract.requiredOutput` in semantic mirror validation.
5. Add a dispatch-gate test for:
   - currentActionContract says `state_change`
   - convergenceContract says `need_evidence`
   - requiredOutput is read-only
   - expected result is runtime contract misalignment.

Acceptance:

- No LLM step can be dispatched with conflicting currentActionContract and
  convergenceContract.
- `currentActionContract` cannot be resurrected from stale run state after
  local authority stripping.

### P0.2 Post-Enrichment Final Gate

Target files:

- `internal/modules/kernel/orchestrator/biz/execution_context_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`

Tasks:

1. Introduce one helper for final LLM dispatch authority normalization.
2. Call it after `enrichLLMStepInputContext`.
3. Ensure it:
   - synchronizes requiredOutput mirrors
   - rebuilds currentActionContract
   - validates dispatch input
4. Ensure no later code mutates active authority fields before runtime dispatch.

Acceptance:

- Any enrichment-induced authority change is reflected in all mirrors.
- Final persisted step input is the same input validated by the dispatch gate.

### P0.3 Graph Limit Follow-Up State Boundary

Target files:

- `internal/modules/kernel/orchestrator/biz/runtime_graph.go`
- `internal/modules/kernel/orchestrator/biz/runtime_graph_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_coverage.go`

Tasks:

1. Mark graph-limit follow-up as either local assessment or authority recompute.
2. For local assessment:
   - block target snapshot realignment
   - block repairActionPackage state mutation
3. For authority recompute:
   - discard local assessment contract
   - rebuild from owner authority
4. Add tests for graph-limit follow-up not being converted into
   target snapshot acquisition in the same step.

Acceptance:

- Graph-limit assessment-only step never carries target snapshot acquisition
  requiredOutput.
- Graph-limit repair path always starts from refreshed owner authority.

### P0.4 Target Snapshot Acquisition Finite Outcome

Target files:

- `internal/modules/kernel/orchestrator/biz/runtime_target_snapshot_acquisition.go`
- `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`

Tasks:

1. Classify target refs:
   - readable resource ref
   - abstract coverage ref
   - invalid/unsafe ref
2. For readable refs, keep runtime-managed acquisition.
3. For abstract refs, do not dispatch another generic read-only LLM graph.
4. Add deterministic transition:
   - candidate derivation from manifest/evidence if available;
   - coverage-scoped state_change if allowed;
   - governed blocker if external choice is required;
   - fail-fast if the contract is internally impossible.
5. Add tests for abstract `cwi-*` and `source_change` refs.

Acceptance:

- Abstract refs do not produce infinite `file/list` loops.
- Runtime either moves to state_change, blocker, or fail-fast within one bounded
  transition.

### P0.5 Focus-Scoped Budget Gate

Target files:

- `internal/modules/kernel/orchestrator/biz/runtime_repair_budget.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_specs.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`

Tasks:

1. Add no-delta budget calculation keyed by:
   - run ID
   - current focus coverageWorkItemIds
   - failure class/signature
2. Count graph-limit failures and failed assessments under the same focus.
3. Check the budget before scheduling a new LLM workspace step.
4. On budget exhaustion, produce a deterministic blocker/fail-fast output.

Acceptance:

- A repeated graph expansion loop cannot exceed configured limits.
- Budget is not reset by changing repair step IDs or graph step IDs.

### P1.1 Evidence Ledger Binding

Target files:

- `internal/modules/kernel/orchestrator/biz/runtime_step_evidence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence_reuse.go`
- `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_reconciler.go`

Tasks:

1. Ensure rejected graph-limit and assessment failures bind:
   - focus coverageWorkItemIds
   - failure signature
   - target refs
   - no-delta marker
2. Ensure accepted target snapshot evidence binds:
   - sourceRef
   - targetRef
   - structuredTargetRefs
   - observedClaims
   - completeness
3. Use evidence ledger for budget and NextAction recompute.

Acceptance:

- Coverage delta and failure repetition can be computed from ledger data, not
  full step JSON scans.

### P1.2 Projection And Command Boundary Audit

Target files:

- `internal/modules/application/workorchd/**`
- `internal/services/workorch/service/tui/**`
- `internal/pkg/workorchport/**`
- `internal/pkg/workorchclient/**`

Tasks:

1. Confirm TUI/Web/CLI commands only submit scoped commands.
2. Confirm ViewModel prompt/list/detail use owner projection.
3. Confirm no local state machine remains for decision/approval/run/coverage.
4. Add projection tests for running, blocked, failed and budget-exhausted states.

Acceptance:

- Projection displays the same blocker/nextAction/prompt mode as owner state.

### P1.3 Resource Hot Path Closure

Target files:

- `internal/pkg/repository/file/file.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- `internal/modules/kernel/orchestrator/biz/runtime_graph.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_reconciler.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dod.go`
- `internal/modules/kernel/orchestrator/biz/execution_context_package.go`

Tasks:

1. Replace hot-path `ListByRun` with `ListByRunSummary` or indexes where full
   input/output is not required.
2. Add latest active LLM step lookup without loading every step body.
3. Add focus/failure budget index or bounded recent summary.
4. Cache coverage text normalization/LCS by definition revision.
5. Avoid cloning full run execution context on repeated progress-only updates.

Acceptance:

- Idle or blocked workspaces do not trigger sustained full JSON reads.
- A repeated failed loop is stopped by P0 gates before it can create sustained
  CPU/IO pressure.

### P1.4 Provider Request And Cache Boundary

Target files:

- `internal/modules/blocks/llm/**`
- `internal/modules/kernel/orchestrator/biz/runtime_llm_context.go`
- request log / cache plan tests

Tasks:

1. Ensure currentActionContract is the only active provider-facing action
   contract.
2. Move old repair packages and old requiredOutput under diagnostic lineage or
   trace refs.
3. Verify C0 stable prefix is not polluted by runtime dynamic fields.

Acceptance:

- Dynamic runtime retry changes do not change C0 stable prefix.
- Provider-facing context no longer contains competing active contracts.

### P2 Template/Soul/Policy Review

Target files:

- `internal/modules/application/workorchd/biz/bootstrap/defaults/templates.json`
- `internal/modules/application/workorchd/biz/bootstrap/defaults/souls.json`
- `.agents/skills/**`
- software-dev profile/policy fixtures

Tasks:

1. Review whether software-dev constraints still over-constrain LLM output.
2. Ensure file granularity and task granularity guidance is template-owned.
3. Keep core business-neutral.

Acceptance:

- Software-dev specific guidance is not encoded in kernel/orchestrator.
- Template policy enters workspace binding and run execution context
  traceably.

## Verification Plan

### Unit Tests

Required P0 tests:

- currentActionContract cannot fallback to stale run authority.
- currentActionContract nextAction/requiredOutput mismatch fails dispatch gate.
- post-enrichment authority mutation is re-normalized before dispatch.
- graph-limit assessment-only cannot become target snapshot acquisition.
- abstract target refs do not enter infinite read-only acquisition.
- focus-scoped budget stops repeated graph expansion failures.

### Integration Tests

Required:

- state_change authority produces state-changing requiredOutput or fail-fast.
- target snapshot acquisition with readable refs creates bounded evidence.
- target snapshot acquisition with abstract refs reaches deterministic outcome.
- repeated no-delta failure reaches blocker/fail-fast before another LLM step.
- projection reflects budget-exhausted or blocked owner state.

### Resource Tests

Required:

- scheduling duplicate/in-flight checks do not load all step bodies.
- repeated failed run uses bounded summaries.
- coverage LCS cache prevents repeated expensive recomputation for unchanged
  definition revision.

### Live Verification

Use a fresh deployment and record:

- git commit
- binary path and PID
- workspace ID and run ID
- starting coverage summary
- latest currentActionContract
- latest requiredOutput mirrors
- budget counters
- accepted/rejected evidence refs
- CPU/RSS samples
- final state after bounded observation

Expected live result:

- No step input contains conflicting currentActionContract and
  convergenceContract.
- No repeated graph expansion loop exceeds configured focus budget.
- Either files are changed through accepted state-changing evidence, or runtime
  reaches a structured blocker/fail-fast with clear reason.
- CPU/RSS does not grow unbounded during repeated failed steps.

## Completion Criteria

This closure is complete only when all P0 items are code-closed and verified by
unit/integration tests, and a live run demonstrates one of:

- positive state-changing progress with accepted evidence and coverage delta;
- governed blocker with scoped reason;
- runtime fail-fast for internal contract impossibility.

It is not complete if the workspace remains `running` while producing repeated
read-only graph repairs without coverage delta.

## 2026-06-30 Live Regression Reopen

The previous P0 closure is not sufficient as final evidence. The latest live
verification shows both active workspaces reached governed waiting states, but
the waiting states were produced by convergence-loop exhaustion rather than by a
clear Goal/DoD blocker:

- `ws-705249a8d7816330` / `run-011-replan-001`
  - State: `waiting_decision`.
  - Blocking ticket:
    `run-011-replan-001-repair-001-of-follow-up-graph-limit-follow-up-graph-follow-up-graph-follow-up-2-repair-exhausted-decision`.
  - Current focus: `cwi-085`, target `source_change`.
  - Required evidence: `validation`, `state_change`.
  - Latest blocker: `repair_focus_mismatch`; the state-changing contract was
    present, but the LLM candidate did not bind state-changing or validation
    actions to `focus-cwi-085`.
  - Important positive signal: the latest step input had target snapshots and a
    state-changing requiredOutput. The remaining gap is retry/budget and focus
    binding recovery, not target snapshot acquisition.
- `ws-db040d24a190f6e5` / `run-011-replan-001`
  - State: `waiting_decision`.
  - Blocking ticket:
    `run-011-replan-001-repair-005-of-follow-up-graph-limit-follow-up-graph-follow-up-graph-follow-up-repair-exhausted-decision`.
  - Current focus: `cwi-039`, target `version_control_snapshot`.
  - Required evidence: `validation`, `state_change`.
  - Latest blocker: repeated `dod_coverage_missing` and read-only repair.
  - Critical contradiction: `currentActionContract.nextAction.kind =
    acquire_evidence`, `convergenceContract.state = need_evidence` and
    requiredOutput forbids state-changing output, while the active focus still
    lacks `state_change`.

Runtime and projection resource evidence from the same observation:

- `.workorch-data` was about `1.2G`.
- `/tmp/workorch-validation-scratch` was about `7.8G`.
- `workorch_event_record_metadata_scans_total` was above `1300`.
- `workorch_event_record_metadata_records_scanned_total` was above `128000`.
- `workorch_workspace_projection_ineffective_retry_runs` still reported `0`,
  even though both workspaces had visible no-delta retry loops.

### Reopened Contract Audit

- Scope:
  - runtime authority selection for missing `state_change`;
  - target snapshot acquisition as a prerequisite under state change;
  - focus binding contract retry;
  - convergence budget escalation;
  - projection ineffective retry metrics.
- Observed state:
  - A focus missing `state_change` can still be dispatched with
    `stateChangingForbidden=true`.
  - A state-changing retry can fail focus binding and then become a generic
    repair-exhausted decision rather than a bounded same-contract retry or
    deterministic blocker.
  - Projection metrics do not report the ineffective retry condition.
- Expected state:
  - Per
    `goal-dod-nextaction-delivery-state-machine-design.md`, if open coverage
    lacks state-changing evidence, the deterministic NextAction remains
    `state_change`.
  - Per `convergence-authority-simplification-design.md`, target snapshot
    acquisition is a deterministic prerequisite or sub-action; it must not
    replace the high-level state-changing authority.
  - LLM focus-binding violations are rejected diagnostic evidence and can be
    retried only under the same state-changing contract within budget.
  - Budget exhaustion produces a structured blocker/fail-fast that names the
    exact focus and failure signature.
- Deviation:
  - `acquire_evidence` is being used as the current action for a focus whose
    mandatory missing evidence includes `state_change`.
  - Focus mismatch is treated as repair exhaustion rather than a narrow
    focus-binding retry or deterministic terminal blocker.
  - Ineffective retry metrics are blind to waiting-decision outcomes caused by
    no-delta repair exhaustion.
- Owner: WorkOrch core runtime. The fix remains business-neutral and must not
  introduce template, Soul, language, command, provider, filename, workspace
  title or live workspace ID branches.
- Event path:
  - coverage/evidence ledger
  - convergence authority refresh
  - state_change prerequisite selection
  - Stage Packet / requiredOutput render
  - LLM candidate
  - contract gate
  - rejected diagnostic evidence
  - focus-scoped budget
  - blocker/fail-fast or next deterministic action
  - projection metrics/ViewModel
- Contract gap:
  - Missing distinction between high-level `state_change` NextAction and
    internal `target_snapshot_acquisition` prerequisite.
  - Missing invariant that a `state_change`-missing focus cannot have a
    provider-facing read-only-only current action after prerequisite budget is
    exhausted or after sufficient target evidence exists.
  - Missing focus-mismatch retry classification and budget accounting.
  - Missing projection/read-model representation of ineffective retry caused by
    decision tickets.

## Reopened Closure Plan

### P0.6 Preserve StateChange Authority Across Prerequisites

Target files:

- `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_current_action_contract.go`

Tasks:

1. Keep deterministic high-level NextAction as `state_change` whenever the
   current focus has missing `state_change` evidence and no governance object
   blocks automatic progress.
2. Represent target snapshot acquisition as a sub-action/prerequisite of
   `state_change`, for example with:
   - `currentActionContract.nextAction.kind = state_change`;
   - `currentActionContract.prerequisite.kind = acquire_target_snapshot`;
   - `convergenceContract.state = need_state_change`;
   - `convergenceContract.prerequisiteState = need_evidence`.
3. If the provider-facing contract is intentionally read-only for prerequisite
   execution, it must still carry the parent state-changing authority and a
   finite exit rule.
4. Add a dispatch invariant:
   - active focus missing `state_change`
   - current action is read-only-only
   - no parent state-change prerequisite metadata
   - result: `runtime_internal_contract_misalign`.

Acceptance:

- A missing-state-change focus cannot be silently downgraded to generic
  `acquire_evidence`.
- Read-only target snapshot acquisition is visible as a finite prerequisite of
  state change, not as a competing current action.

### P0.7 Runtime-Owned Target Snapshot Prerequisite Exit

Target files:

- `internal/modules/kernel/orchestrator/biz/runtime_target_snapshot_acquisition.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_evidence.go`

Tasks:

1. Make target snapshot acquisition runtime-owned before LLM graph generation
   whenever refs are readable or can be derived from the workspace manifest,
   previous accepted state-changing evidence, or failure target evidence.
2. For abstract refs such as coverage IDs, deliverable IDs or source-scope
   tokens, do not ask LLM to repeatedly list/read the workspace root.
3. Produce one of four deterministic prerequisite outcomes:
   - accepted target snapshot evidence;
   - `state_change` with coverage-scoped greenfield or expansion contract;
   - governed blocker when external information, permission, credential or user
     choice is missing;
   - internal fail-fast when the contract cannot be bound.
4. Once accepted target snapshot evidence exists, forbid another equivalent
   target snapshot acquisition for the same focus and target hash unless the
   workspace definition revision or target content hash changes.

Acceptance:

- `cwi-*`, `deliverable:*`, `source_change` and similar abstract refs do not
  create repeated read-only LLM graph loops.
- `ws-db040d24a190f6e5`-style version snapshot focus either receives a
  state-changing evidence route, a scoped blocker, or fail-fast without another
  generic read-only repair cycle.

### P0.8 Focus Binding Violation Retry Contract

Target files:

- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence_budget.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`

Tasks:

1. Classify `repair_focus_mismatch` as a focus-binding contract violation,
   separate from generic runtime failure.
2. Record rejected diagnostic evidence with:
   - focus coverageWorkItemIds;
   - expected focus ID;
   - provided or missing focus fields;
   - requiredOutput hash;
   - parent NextAction.
3. Retry only the same state-changing contract with a narrower focus-binding
   correction if budget remains.
4. On repeated focus mismatch, stop LLM dispatch and produce a deterministic
   structured blocker/fail-fast. Do not create generic repair-exhausted
   decision unless the blocker genuinely needs user input.

Acceptance:

- `ws-705249a8d7816330`-style focus mismatch cannot become an unscoped generic
  repair exhaustion loop.
- Rejected focus-binding outputs never become accepted evidence, artifact,
  result or memory.

### P0.9 Ineffective Retry Budget And Projection Signal

Target files:

- `internal/modules/kernel/orchestrator/biz/runtime_convergence_budget.go`
- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_specs.go`
- `internal/modules/application/workorchd/service/server/workspace_projection.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`

Tasks:

1. Calculate no-delta retry budget with:

   ```text
   workspaceId
   runId
   definitionRevision
   sorted coverageWorkItemIds
   parent NextAction
   requiredOutput hash
   normalized failure class/signature
   coverageLedgerHash before/after
   ```

2. Count repeated graph-limit assessment failures, read-only target snapshot
   repeats, focus mismatch violations and `dod_coverage_missing` retries under
   the same focus budget.
3. Before scheduling another LLM step, stop dispatch when the focus budget is
   exhausted.
4. Persist budget exhaustion as owner state and evidence diagnostic so
   projection can show it without scanning all step bodies.
5. Update projection metrics so `ineffective_retry_runs` includes:
   - running no-delta loops;
   - waiting-decision caused by retry exhaustion;
   - blocked/fail-fast caused by focus-scoped budget exhaustion.

Acceptance:

- A repeated no-delta loop cannot exceed configured budget because step IDs or
  graph root IDs changed.
- Projection metrics report the current ineffective retry condition.

### P1.5 Resource Guard For Reopened Loop

Target files:

- `internal/modules/kernel/orchestrator/biz/execution_context_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_llm_context.go`
- `internal/modules/kernel/orchestrator/biz/runtime_graph.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`
- repository indexes used by run/step/event reads

Tasks:

1. For repeated no-delta retries, use compact focus diagnostics and recent
   bounded summaries instead of full step input/output hydration.
2. Ensure projection metrics for ineffective retry are computed from indexed
   owner/budget state, not from repeated full event payload scans.
3. Add scratch lifecycle cleanup evidence for terminal/blocker paths without
   business-specific process handling.

Acceptance:

- Waiting, blocked or fail-fast workspaces do not keep increasing JSON scan
  counters, memory or scratch usage.

## Reopened Verification Matrix

Required unit tests:

- Missing `state_change` focus preserves parent `state_change` authority when
  target snapshot acquisition is selected as a prerequisite.
- Read-only prerequisite without parent state-change metadata fails dispatch
  gate.
- Abstract target refs produce finite target snapshot prerequisite outcomes.
- Accepted target snapshot evidence prevents repeated equivalent acquisition.
- `repair_focus_mismatch` produces focus-binding rejected diagnostic evidence
  and same-contract retry.
- Repeated focus mismatch exhausts budget into blocker/fail-fast, not generic
  repair-exhausted decision.
- Ineffective retry budget is keyed by focus + requiredOutput hash + normalized
  failure signature, not step ID.
- Projection ineffective retry metric includes waiting-decision retry
  exhaustion.

Required package tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'StateChange|TargetSnapshot|FocusMismatch|ConvergenceBudget|ConvergenceDispatch|CurrentActionContract' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
```

Required full verification:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./... -count=1
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
```

Required live verification:

1. Deploy a fresh binary and record git commit, PID, start time and data root.
2. Trigger resume/replan for two workspaces.
3. Confirm every LLM step has one parent current action and optional
   prerequisite, not competing active contracts.
4. Confirm a focus missing `state_change` does not dispatch a generic
   read-only-only current action.
5. Confirm focus mismatch retries either produce accepted state-changing
   evidence or stop with structured blocker/fail-fast within budget.
6. Confirm projection shows the same next action/blocker in workspace list,
   main view and prompt.
7. Confirm CPU/RSS, `.workorch-data`, scratch usage and event metadata scans do
   not grow through repeated ineffective retries.

## Updated Completion Criteria

The closure is not complete merely because unit tests pass. It is complete only
when live verification proves all of the following:

- no current focus missing `state_change` is dispatched as generic
  `acquire_evidence`;
- target snapshot acquisition is finite and runtime-owned when possible;
- focus-binding violations are bounded and visible;
- ineffective retry is reflected in owner state and projection metrics;
- the workspace either makes accepted state-changing progress, reaches a scoped
  governed blocker, or fails fast with an internal contract reason.

## Implementation Status

### 2026-06-30 P0 Code Closure

Historical implementation record. This closure passed local tests at the time,
but the `2026-06-30 Live Regression Reopen` section above supersedes it as the
current active closure scope. Do not treat this record as final completion
evidence until P0.6-P0.9 and the reopened live verification matrix pass.

Implemented:

- P0.1 CurrentActionContract authority:
  - `currentActionContract` no longer falls back to `run.ExecutionContext` to
    select `nextAction`.
  - If step input lacks owner authority, current action is derived from the
    step-local `convergenceContract`.
  - `need_evidence` + `target_snapshot_acquisition` derives
    `acquire_target_snapshot`.
  - `currentActionContract.requiredOutput` participates in requiredOutput
    semantic and focus alignment validation.
- P0.2 Post-enrichment final gate:
  - LLM dispatch now runs final authority normalization after enrichment and
    before step persistence/runtime dispatch.
  - Final normalization synchronizes requiredOutput mirrors, rebuilds
    `currentActionContract`, scrubs stale diagnostics and validates the final
    dispatch input.
- P0.3 Graph-limit follow-up boundary:
  - assessment-only graph-limit follow-up is not target-snapshot realigned.
  - local assessment semantics remain assessment-only for that step.
- P0.4 Target snapshot finite outcome:
  - readable target refs still use runtime-managed bounded `file/read` graph.
  - abstract/unreadable refs produce deterministic runtime contract fail-fast
    output instead of another generic LLM read-only graph.
- P0.5 Focus-scoped budget gate:
  - convergence budget scope now includes `coverageWorkItemIds`.
  - escalated convergence budget blocks further dispatch through
    `gapBaselineDispatchGate`.

Verification:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'ConvergenceBudget|CurrentActionContract|TargetSnapshot|GraphLimitAssessmentOnly|ConvergenceDispatch' -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
```

Both commands passed locally.

### 2026-06-30 Reopened P0.6-P0.9 Code Closure

Implemented:

- P0.6 Preserve state_change authority across prerequisites:
  - target snapshot acquisition is now represented as a prerequisite of the
    parent `state_change` action.
  - `currentActionContract` carries `nextAction.kind = state_change` plus
    `prerequisite.kind = acquire_target_snapshot`.
  - read-only target snapshot requiredOutput is accepted only when prerequisite
    metadata is present; malformed read-only `state_change` inputs fail the
    dispatch gate.
- P0.7 Runtime-owned target snapshot prerequisite exit:
  - runtime-managed target snapshot acquisition accepts the prerequisite model.
  - runtime-owned read-only graph output is allowed only for
    `runtimeManagedTargetSnapshotAcquisition` prerequisite execution.
  - abstract target refs still fail fast with a deterministic internal contract
    outcome instead of generating another generic LLM read graph.
- P0.8 Focus binding violation retry contract:
  - `repair_focus_mismatch` is classified as `focus_binding_mismatch`.
  - contract violation metadata now records rule and recoverability for
    inferred violations, not only structured violation errors.
  - correction profile narrows retry to the same current action and current
    coverage focus binding.
- P0.9 Ineffective retry budget and projection signal:
  - run heartbeat marks convergence budget escalation as ineffective retry.
  - waiting-decision retry exhaustion is surfaced as `IneffectiveRetry` while
    preserving `waiting_decision` activity state.
  - projection metrics can count ineffective retry from heartbeat without
    scanning full step bodies.

Verification:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'FocusMismatch|Focus|ConvergenceBudget|CurrentActionContract|TargetSnapshot|StateChange|ConvergenceDispatch' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'RunHeartbeat|ConvergenceStatus' -count=1
```

Both commands passed locally.

Remaining:

- Full package and repository verification.
- Live verification against current workspaces after redeploy.

### 2026-06-30 P1.5 Resource Guard Code Closure

Implemented:

- Runtime dispatch hot path:
  - `nextDispatchSpecs` now uses `StepRepository.ListByRunSummary` for active
    and terminal step selection.
  - Full step bodies are loaded by ID only after a specific active or terminal
    step has been selected.
- In-flight duplicate checks:
  - runtime repair duplicate check uses step summaries;
  - active LLM workspace execution check uses step summaries;
  - graph dispatch duplicate check uses step summaries.
- Tests:
  - added coverage proving in-flight checks do not call full `ListByRun`;
  - added coverage proving `nextDispatchSpecs` selects via summary and only
    performs bounded step body loads.

Verification:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'RuntimeInFlightChecksUseStepSummaries|NextDispatchSpecsUsesStepSummaries|Dispatch|NextDispatch|ConvergenceBudget|CurrentActionContract|TargetSnapshot|StateChange|FocusMismatch|ConvergenceDispatch' -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
```

All commands passed locally.

Remaining:

- Full repository verification.
- Live verification of CPU/RSS, event metadata scans, `.workorch-data` growth
  and scratch lifecycle after redeploy.
