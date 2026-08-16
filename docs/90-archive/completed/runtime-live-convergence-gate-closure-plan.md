# Runtime Live Convergence Gate Closure Plan

Status: partially-closed

Type: closure-plan

Last Updated: 2026-06-28

Related:

- `../superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-nextaction-validation-to-statechange-closure-plan.md`
- `runtime-convergence-authority-final-fix-scope.md`
- `goal-dod-nextaction-s4-5-completion-gate-reads-coverage-ledger.md`
- `goal-dod-nextaction-s5-5-missing-state-change-nextaction.md`
- `goal-dod-nextaction-s5-6-missing-validation-nextaction.md`
- `goal-dod-nextaction-s5-8-all-satisfied-complete-nextaction.md`
- `runtime-managed-validation-resource-lifecycle-closure-plan.md`

## Purpose

本文基于 2026-06-28 live verification 结果，重新收口 WorkOrch runtime
在真实 workspace 执行中仍暴露出的两个核心偏差：

1. 当前 focus 已被 rejected validation 证明缺少有效实现或测试证据时，runtime
   仍可能继续派发 `validate` / `produce_concrete_validation_evidence`，
   而没有切换到 `state_change` 或 `repair(state_change)`。
2. 只完成 `current_focus` 局部 assessment 时，run/workspace 仍可能进入
   `completed`，即 completion gate 没有严格以全局 coverage/evidence ledger
   作为唯一完成权威。

本文只约束 WorkOrch core 的业务中立收敛能力、projection diagnostics 和资源
生命周期。软件开发相关的验证偏好、文件粒度和工具策略仍由
workspaceTemplate/profile/Soul/policy 承载，core 不得加入模板、语言、命令、
文件名、provider、workspace title 或 live workspace ID 特判。

## Contract Audit

- Scope:
  - `ws-7ced70e98ea50c43` / `run-009`
  - `ws-4e83810542ac4d65` / `run-009`
  - runtime convergence authority, deterministic NextAction, requiredOutput
    generation, completion gate, projection diagnostics and validation resource
    lifecycle.
- Observed state:
  - `ws-7ced70e98ea50c43` is running and dispatch is alive.
  - Current focus is `cwi-003`, whose target requires usage/cost visibility
    evidence.
  - `graph-005` / `graph-006` / `graph-007` all failed on usage visibility
    validation.
  - Latest validation failed with `no usage-focused tests discovered in current
    source`.
  - Current source still has static zero-value metrics/stats output and no
    usage/token/cache focused tests.
  - Despite that no accepted state-change evidence closes this focus, the
    current repair input still uses `repairObjective=produce_concrete_validation_evidence`
    and `requiredOutput.forbiddenKinds` includes `patch/apply`.
  - `ws-4e83810542ac4d65` / `run-009` is marked `completed`.
  - Its snapshot still reports `acceptedSlices: 0`, `openSlices: 11`,
    `evidence_ready: 39`, `satisfied: 1`.
  - Its final artifact explicitly states that the assessment only covers the
    current focus and does not equal whole Goal/DoD acceptance, but
    `/run/state/changed completed` and `/workspace/state/changed completed`
    were emitted.
  - Resource observations show high write amplification and stale validation
    scratch/process state: workorch RSS around 1.7GB, write bytes above 600MB
    for the live process, `/tmp/workorch-validation-scratch` around 6.4GB, and
    old validation processes from prior runs still alive.
- Expected state:
  - `Coverage/Evidence Ledger -> Current Focus -> Deterministic NextAction ->
    Stage Packet -> Candidate Gate -> Evidence Ledger -> Completion Gate` is
    the only authoritative convergence loop.
  - If a focus has repeated rejected validation and no new accepted
    state-changing evidence, runtime must not keep dispatching ordinary
    validation graphs for the same gap.
  - When implementation evidence is missing or rejected validation proves the
    current source lacks the target behavior, NextAction must become
    `state_change` or `repair(state_change)`, and requiredOutput must allow
    state-changing candidate output.
  - `completed` can only be emitted when latest definition revision coverage is
    fully satisfied or explicitly governed waived.
  - Projection may expose diagnostics for authority/gate mismatch, but must not
    repair owner state.
- Deviation:
  - The NextAction interpreter / authority refresh still lets a validation-only
    route win even after the validation route proves no current source evidence
    exists.
  - requiredOutput is not strictly derived from the effective NextAction; it can
    forbid `patch/apply` while the actual ledger gap requires state change.
  - Completion gate can be bypassed or applied only to current-focus assessment
    instead of whole-run coverage readiness.
  - Resource lifecycle does not reliably clean validation scratch state and
    child processes after run/step completion or replacement.
- Owner:
  - Primary: WorkOrch core runtime convergence authority, NextAction interpreter,
    requiredOutput builder, completion gate and validation resource lifecycle.
  - Secondary: projection/read model for read-only diagnostics.
  - Not owner: TUI/Web local state, provider behavior, or target workspace
    business implementation.
- Event path:
  - Runtime validation failure / rejected diagnostic -> evidence ledger ->
    coverage ledger recompute -> `RefreshConvergenceAuthority` ->
    deterministic NextAction -> requiredOutput / Stage Packet -> dispatch ->
    candidate gate -> accepted/rejected evidence -> completion gate ->
    projection/ViewModel.
- Contract gap:
  - Missing no-delta validation loop breaker tied to focus/evidence kind.
  - Missing strict NextAction-to-requiredOutput consistency gate.
  - Missing global completion gate enforcement before run/workspace completed
    events.
  - Missing lifecycle cleanup verification for managed validation scratch and
    spawned processes.
- Boundary check:
  - Core may inspect structured coverage status, evidence kind, accepted/rejected
    evidence refs, diagnostic classes, retry/no-delta counters, NextAction type,
    requiredOutput allowed/forbidden kinds and lifecycle ownership.
  - Core must not inspect strings like usage, token, Go, test names, file names
    or provider names to make convergence decisions.

## Target Flow

```text
validation rejected for current focus
  -> record rejected diagnostic evidence
  -> recompute coverage/evidence ledger
  -> compare focus-scoped delta:
       new accepted state_change? no
       new accepted validation? no
       same focus/evidence kind repeated? yes
  -> derive deterministic NextAction:
       missing state_change evidence     -> state_change
       rejected validation proves no work -> repair(state_change)
       environment prerequisite missing  -> controlled validation / governance
       retry budget exhausted            -> governed blocked / fail
  -> rebuild requiredOutput from NextAction
  -> dispatch or governed wait
```

```text
candidate assessment/result says current focus satisfied
  -> record assessment candidate only after contract gate
  -> update coverage ledger for that focus
  -> run global completion gate:
       all required coverage satisfied/waived?
       all required evidence kinds accepted?
       open decisions/approvals/blockers absent?
       unhandled rejected diagnostics absent?
       final artifacts/results traceable?
  -> if yes: completed
  -> if no: recompute NextAction for remaining coverage
```

## Required Closure Items

### LCG-01. Focus No-Delta Validation Loop Breaker

Status: closed

Add a focus-scoped no-delta guard before deriving another validation or
validation-repair action.

Rules:

- Key by `workspaceId + runId + focusId + requiredEvidenceKind`.
- Count rejected validation attempts since the latest accepted evidence for the
  same focus.
- If the count increases without accepted `state_change`, `validation` or
  explicitly waived evidence, the next ordinary validation action is not allowed.
- The guard must return a structured reason such as
  `validation_loop_without_state_change`.
- The guard is generic; it does not parse stderr/stdout for business keywords.

Acceptance:

- Fixture: focus lacks validation evidence, has three rejected validation refs,
  and no accepted state-change delta. Expected NextAction is not `validate`.
- Fixture: focus has a new accepted state_change after rejected validation.
  Expected NextAction may be `validate`.
- Projection exposes no-delta diagnostics read-only.

Closure note:

- Implemented in `runtime_next_action.go` via a focus-scoped rejected diagnostic
  counter after the latest accepted `state_change` / `validation` evidence
  delta.
- The guard emits structured reason `validation_loop_without_state_change` and
  does not inspect stderr/stdout business text.
- Repeated concrete validation `executionGraph` follow-up expansion is now
  bounded by the same graph expansion limit semantics. After the limit is hit,
  runtime routes to an assessment-only graph-limit follow-up instead of allowing
  the validation graph to bypass convergence indefinitely.
- Regression coverage:
  `TestNextActionForRunNoDeltaValidationLoopRequestsStateChange` and
  `TestNextActionForRunNoDeltaValidationLoopResetsAfterAcceptedStateChange`.
  Graph follow-up coverage:
  `TestRuntimeOrchestrator_GraphFollowUpExpansionLimitAppliesToRepeatedConcreteValidationEvidenceGraph`.

### LCG-02. Validation Failure To StateChange Promotion

Status: closed

Strengthen deterministic NextAction so validation rejection can promote the
focus back to state-changing output when ledger evidence proves current
deliverable state is insufficient.

Rules:

- If current focus still lacks accepted state_change evidence required by policy,
  NextAction must be `state_change` before `validate`.
- If current focus has state_change evidence but repeated validation failure
  produces no accepted validation and no accepted evidence delta, NextAction
  must become `repair(state_change)` unless the diagnostic is classified as
  environment/external dependency.
- If policy does not require state_change for the focus, the no-delta guard may
  choose `request_decision`, `request_approval`, `fail` or controlled validation.
- The decision must be based on structured evidence requirements and diagnostic
  classes, not business strings.

Acceptance:

- Live-shaped fixture for `cwi-003`: missing validation, repeated rejected
  validation, no usage-specific string matching in core, no accepted state
  change for the focus. Expected NextAction is `state_change` or
  `repair(state_change)`.
- Environment failure fixture remains validation repair or governed wait.

Closure note:

- Implemented as deterministic NextAction promotion to `state_change` when
  repeated structured validation diagnostics make no accepted evidence delta.
- Existing implementation-gap routing remains intact; external/provider/runtime
  transient classes are not promoted by the generic no-delta guard.

### LCG-03. RequiredOutput Must Be Derived From NextAction

Status: closed

Make `requiredOutput` an executable contract generated from the effective
NextAction, with a final pre-dispatch consistency gate.

Rules:

- `state_change` / `repair(state_change)`:
  - `acceptedKinds` must include a state-changing output kind such as
    `patch/apply` or another generic state-change capability allowed by current
    policy.
  - `forbiddenKinds` must not include the accepted state-changing kind.
- `validate`:
  - workspace-mutating actions remain forbidden.
  - controlled substitute writes must be confined to validation scratch.
- `assessment`:
  - executable or patch output must be absent unless the current NextAction
    explicitly requires it.
- If `NextAction` and `requiredOutput` disagree, dispatch must fail fast with
  `required_output_mismatch_next_action`, not call LLM/provider.

Acceptance:

- Fixture: NextAction is `state_change`, requiredOutput forbids `patch/apply`.
  Expected result is pre-dispatch internal contract failure.
- Fixture: NextAction is `validate`, requiredOutput allows workspace mutation.
  Expected result is pre-dispatch internal contract failure.
- LLM request C3/C4 contains only one effective requiredOutput generated from
  current authority.

Closure note:

- Implemented in `validateConvergenceDispatchRequiredOutputSemantics`.
- `state_change` now requires at least one non-forbidden accepted kind when
  `mustIncludeStateChangingNode=true`.
- `validate` rejects `patch/apply` accepted output before provider dispatch.
- Mismatches append `authorityDiagnostics` with code
  `required_output_mismatch_next_action`.
- Regression coverage:
  `TestValidateConvergenceDispatchInputRejectsStateChangeRequiredOutputForbidsAcceptedKind`
  and
  `TestValidateConvergenceDispatchInputRejectsValidationRequiredOutputAcceptsPatchApply`.

### LCG-04. Global Completion Gate Enforcement

Status: closed

Ensure run/workspace `completed` events can only be emitted after the global
coverage ledger passes completion readiness.

Rules:

- Current-focus assessment can update that focus only; it cannot complete the
  whole run unless all required coverage is satisfied or waived.
- Completion gate must check the latest definition revision, all required
  coverage items, required evidence kinds, open governance objects, unresolved
  rejected diagnostics and final artifact/result traceability.
- If an LLM result says "this only covers current focus", runtime must treat it
  as partial assessment evidence and recompute NextAction.
- If completion was attempted but gate fails, record a rejected diagnostic or
  completion-gate failure event and keep run active/governed, not completed.

Acceptance:

- Fixture: `acceptedSlices=0`, `openSlices>0`, `satisfied=1`, assessment scope is
  current focus. Completion attempt is rejected.
- Fixture: all required coverage satisfied with accepted refs. Completion is
  allowed.
- Regression: run state cannot transition to `completed` while global completion
  gate reports unresolved coverage.

Closure note:

- `CompleteRun` and `validateAndCompleteRunWithWorkspace` both call the global
  coverage readiness gate before emitting completed state.
- `coverageSubsystem.validateReadyForAcceptance` now hydrates owner repository
  projection before validation when repository-backed coverage exists.
- Regression coverage:
  `TestRuntimeOrchestrator_CompleteRunRejectsOpenCoverageLedger`.

### LCG-05. Projection Authority Diagnostics

Status: closed

Expose read-only diagnostics when projection detects live authority/gate drift.

Diagnostic examples:

- `validation_loop_without_state_change`
- `required_output_mismatch_next_action`
- `completion_gate_failed_but_run_completed`
- `stale_validation_resource_lifecycle`

Rules:

- Projection must not mutate owner state.
- TUI/Web should display these diagnostics from ViewModel/projection only.
- Diagnostics must carry workspace/run/focus scope and source revision.

Acceptance:

- Projection test exposes diagnostics when owner snapshot contains inconsistent
  completion or requiredOutput state.
- TUI/Web does not infer these conditions locally.

Closure note:

- Runtime now appends `authorityDiagnostics` for
  `required_output_mismatch_next_action`.
- No-delta state-change routing carries `validationNoDeltaReason` and rejected
  attempt count in the current requiredOutput.
- `graphExpansionLimitFollowUp` and `assessmentOnlyContractCorrection` now keep
  a local `need_assessment` contract during dispatch input rebuild. They strip
  run-level `convergenceAuthority` / `gapBaseline` from the local request so a
  global `validate` nextAction cannot overwrite the assessment-only correction
  step.
- Readmodel projection now emits read-only diagnostics for completed/open
  coverage drift and stale/failed validation resource lifecycle records already
  present in step output.
- Regression coverage:
  `TestAuthorityDiagnosticsFromRunSnapshotReportsCompletedOpenCoverageDrift`
  and
  `TestAuthorityDiagnosticsFromRunSnapshotReportsStaleValidationResources`.

### LCG-06. Validation Resource Lifecycle Cleanup

Status: partially-closed

Close the resource leak surface observed during live verification.

Rules:

- Each validation step must own its scratch directory, managed process handles
  and cleanup lifecycle.
- Step completion, run completion, run cancellation, replacement run and service
  restart recovery must reconcile owned validation resources.
- Cleanup must be idempotent and scoped to WorkOrch-owned validation resources.
- Core must not kill arbitrary user processes by command name; it must use
  recorded ownership metadata.

Acceptance:

- Unit/integration fixture creates a managed validation process and proves it is
  stopped on step/run finalization.
- Restart recovery marks orphaned owned validation resources stale and cleans or
  reports them without affecting unrelated processes.
- Resource projection reports owned stale scratch/process state.

Closure note:

- Managed validation dispatch now persists planned owner resource records in
  step input before runtime execution is accepted. Records carry
  workspace/run/step scope, resource kind, cleanup policy, created timestamp and
  runtime-dispatch source.
- Readmodel resource health now reads output records first and falls back to
  planned input records when output is absent, so async/restart gaps remain
  observable without filesystem or process scanning.
- Terminal steps with planned resources but no release output are projected as
  `stale_validation_resource_lifecycle`; running pending records remain
  observable resource health, not an authority error.
- Code cleanup that kills processes by command name or scans arbitrary scratch
  trees is intentionally not added because it would violate scoped/idempotent
  ownership rules.
- Remaining work: implement scoped cleanup/reconciliation from these durable
  owner records for step/run finalization and restart recovery.

#### LCG-06 Contract Audit: Durable Owner Metadata

- Scope: managed validation `validation/managed_session` normalized to
  `shell/command`, runtime dispatch input, step input persistence and
  readmodel projection resource health.
- Observed state: shell runtime output can emit `validationResources` after the
  command returns, but async dispatch or service restart before completion has
  no durable owner-side resource metadata visible from the step snapshot.
- Expected state: before runtime execution is accepted, the owner persists
  planned WorkOrch-owned validation resources with workspace/run/step scope;
  projection can report these records even if output never arrives.
- Deviation: projection can only see completed output records, so lost/stale
  validation resources cannot be scoped or diagnosed without unsafe filesystem
  or process scanning.
- Owner: WorkOrch core runtime dispatch and readmodel projection.
- Event path: dispatch step -> prepare validation scratch -> attach planned
  resource ownership -> persist step input -> runtime execution -> output
  resource release records -> projection.
- Contract gap: missing planned/pending resource ownership records in the
  authoritative step snapshot.
- Proposed change: attach `validationResources` to step input for managed
  validation before runtime request; projection reads output records first and
  input records only when output is absent.
- Boundary check: records are keyed only by workspace/run/step, resource kind,
  cleanup policy and scratch path supplied by the runtime contract; no template,
  language, command, provider or workspace-id special cases.
- Verification: add dispatch persistence test and readmodel resource health /
  stale-owner projection tests.

Closure note:

- Implemented by `attachManagedValidationResourceOwnership` during runtime
  dispatch and readmodel `validationResourceRecordsForStep` fallback.
- Regression coverage:
  `TestRuntimeOrchestrator_DispatchPersistsManagedValidationResourceOwnershipBeforeRuntimeRequest`,
  `TestRuntimeResourceHealthFromStepsUsesPlannedInputResourcesWhenOutputMissing`
  and
  `TestAuthorityDiagnosticsFromRunSnapshotReportsTerminalPlannedValidationResourcesWithoutOutput`.

## Implementation Scope

Primary code areas to audit before patching:

- `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_validation*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_completion*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_adapter*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_managed_validation*.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`
- `internal/pkg/workorchport/**`
- `internal/pkg/workorchclient/**`

Secondary policy/template areas:

- `internal/modules/application/workorchd/biz/bootstrap/defaults/templates.json`
- `internal/modules/application/workorchd/biz/bootstrap/defaults/souls.json`
- project skills and workspaceTemplate policy documents when software-dev
  strategy needs data-only refinement.

## Verification Plan

Targeted unit tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*(NoDeltaValidation|ValidationLoop|RequiredOutputNextAction|CompletionGate|ManagedValidationResource)' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'Test.*AuthorityDiagnostics' -count=1
```

Package regressions:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./internal/services/workorch/... ./internal/pkg/workorchport/... ./internal/pkg/workorchclient/... -count=1
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|usage|token|main.go|main_test.go|provider|workspace title" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "requiredOutput|NextAction|completionGate|validateReadyForAcceptance|validationFailureDiagnostic|managedValidation|scratch" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
```

Live verification:

1. Start latest workorchd and record pid, binary, git commit and start time.
2. Resume or replan two workspaces.
3. Confirm command accepted -> authority refresh -> deterministic NextAction.
4. For repeated rejected validation without accepted evidence delta, confirm the
   next action is not ordinary validation.
5. Confirm state-changing NextAction has state-changing requiredOutput.
6. Confirm current-focus assessment does not complete the whole run unless
   global completion gate passes.
7. Confirm projection shows authority diagnostics for any remaining mismatch.
8. Confirm owned validation scratch/process resources are cleaned or reported
   after step/run finalization.

## Latest Verification

Date: 2026-06-28

Passed:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*(NoDeltaValidation|ValidationLoop|RequiredOutput.*NextAction|CompletionGate|CompleteRunRejectsOpenCoverageLedger|ValidationRequiredOutput|StateChangeRequiredOutput)|TestNextActionForRun|TestDeriveMissingValidation|TestValidateConvergenceDispatchInput|TestRuntimeOrchestrator_CompleteRun|TestCoverageWorkItemsAcceptanceGate' -count=1
```

Known remaining package failures observed in full
`go test ./internal/modules/kernel/orchestrator/biz -count=1`:

- None in the latest package run.

Additional passed verification:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run '^(TestRuntimeOrchestrator_RunningResumeReplansFromResolvedDecisionOnFailedNonTerminalStep|TestRuntimeOrchestrator_GraphFollowUpExpansionLimitAllowsConcreteValidationEvidenceGraph|TestRuntimeOrchestrator_GraphFollowUpExpansionLimitAppliesToRepeatedConcreteValidationEvidenceGraph|TestGraphExpansionLimitFollowUpRestoresCoverageValidationConvergenceContract|TestReconcile_Idempotent|TestEnsureGapBaselineHydratesCoverageContextFromOwnerRepository|TestReconcile_GraphLeafFuzzyMatchBinds)$' -count=1 -timeout 60s
go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout 120s
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'TestAuthorityDiagnosticsFromRunSnapshot' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./internal/modules/application/workorchd/biz/appservice -count=1 -timeout 120s
go test ./internal/services/workorch/... ./internal/pkg/workorchport/... ./internal/pkg/workorchclient/... -count=1 -timeout 120s
go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_Dispatch(PersistsManagedValidationResourceOwnershipBeforeRuntimeRequest|PreparesValidationScratchBeforeRuntimeRequest)' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'Test(RuntimeResourceHealthFromSteps|AuthorityDiagnosticsFromRunSnapshot)' -count=1
go test ./internal/modules/blocks/shell/biz -run 'TestRuntimeAdapter_ExecuteRuntimeStepCarriesManagedValidationMetadata' -count=1
```

## Done Criteria

- A focus cannot loop through ordinary validation indefinitely without accepted
  evidence delta.
- Missing or insufficient implementation evidence leads to `state_change` or
  `repair(state_change)` through structured ledger/diagnostic rules.
- `requiredOutput` cannot contradict effective NextAction at provider dispatch.
- `completed` cannot be emitted while latest coverage ledger still has required
  open or evidence-ready items without governed waiver.
- Projection reports mismatches instead of hiding them or mutating owner state.
- Validation scratch/process lifecycle is scoped, idempotent and observable.
- Core remains business-neutral.
