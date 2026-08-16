# Runtime Finalization Contract Evidence Resource Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-29

Related:

- `../superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-managed-validation-resource-lifecycle-closure-plan.md`
- `runtime-validation-contract-gate-managed-session-closure-plan.md`
- `projection-event-io-resource-closure-plan.md`

## Purpose

This document scopes the remaining closure work found during live/runtime audit:

1. runtime step finalization can mark a step completed before decision or approval gates finish;
2. contract-violation retry exhaustion can continue as retry instead of landing on a governed blocker or terminal state;
3. managed validation is accepted by requiredOutput and contract validation but is missing from the runtime capability catalog;
4. managed validation evidence can fail to bind to current coverage work items because focus metadata is not injected consistently;
5. TUI/CLI snapshot paths can still hydrate detail projections and amplify JSON/read-model cost.

The closure direction follows the architecture contract from
`goal-dod-nextaction-delivery-state-machine-design.md`:

```text
Goal/DoD Definition
  -> Coverage/Evidence Ledger
  -> Current Focus
  -> Deterministic NextAction
  -> Derived Stage Contract / Stage Packet
  -> Candidate Generation or Runtime Action
  -> Contract Gate
  -> Accepted/Rejected Evidence
  -> Ledger Recompute
  -> Projection/ViewModel
```

It also follows `convergence-authority-simplification-design.md`: only owner
state, coverage/evidence ledger, governance state and deterministic NextAction
are authoritative. Derived fields such as `gapBaseline`,
`convergenceContract`, `stagePacket` and `repairActionPackage.requiredOutput`
must be rebuilt from the same current authority snapshot and cannot drive
state transitions by themselves.

Current phase does not require compatibility. If existing implementation or
tests conflict with this document, adjust implementation and tests to match
this document.

## Contract Audit

- Scope:
  - Runtime dispatch finalization, contract-violation recovery, managed
    validation capability and evidence binding, validation resource lifecycle,
    and TUI/CLI projection snapshot access.
- Observed state:
  - `runtime_dispatch_execution.go` marks a step `completed`, records accepted
    runtime evidence, updates coverage, persists and publishes step state, then
    handles `decisionBlockSpec` and `approvalBlockSpec`.
  - Contract violation recovery can choose another
    `contract_violation_retry` after retry budgets are exhausted when the
    decision-eligibility fallback branch is active.
  - `requiredOutput.acceptedKinds` and execution contract validation already
    allow or require `validation/managed_session`, but
    `currentRuntimeEvidenceBlockKinds()` does not declare that kind.
  - Required-output examples still use the weak placeholder
    `current_coverage_focus`; managed validation raw graph nodes are not
    consistently treated as focus-binding nodes before normalization.
  - CLI snapshot requests still call `ReadWorkspaceViewSnapshot` and project
    full detail before converting to a resource map. Projection snapshot loading
    has partial query reduction but is still detail-shaped in key paths.
- Expected state:
  - A step is finalized as completed only after all gate decisions prove that
    the candidate output is accepted and no governance/blocker path owns the
    step outcome.
  - Contract retry exhaustion is deterministic. When retry budget is exhausted,
    runtime lands in exactly one of governed blocker or terminal failure; it
    does not dispatch another ordinary LLM retry.
  - Runtime capability catalog, requiredOutput accepted kinds and execution
    contract validator share one business-neutral capability source.
  - Validation evidence that claims to close a coverage work item carries
    `coverageWorkItemIds`, `structuredTargetRefs`, `targetRefs` and
    `observedClaims` in a form accepted by the evidence ledger.
  - TUI/Web/CLI main views consume compact projection/ViewModel state. Full
    detail snapshots are used only for explicit detail/status requests.
- Deviation:
  - Completion, failure, rejected diagnostics, decision and approval can be
    published for the same logical candidate in the wrong order.
  - Retry exhaustion tests still encode the older "one more retry with
    decisionEligibilityDenied" behavior, which conflicts with deterministic
    blocker/terminal closure.
  - Managed validation exists as a contract route but is not consistently
    visible as a runtime capability or coverage-binding evidence route.
  - Projection/resource reads can still pay detail-snapshot cost for interactive
    flows.
- Owner:
  - WorkOrch core for runtime state machine, contract gate, evidence ledger,
    managed validation neutral capability and read-model projection boundaries.
  - TUI/Web remain display and command clients only. No template, language,
    provider, command, workspace title or live workspace-specific behavior is
    introduced.
- Event path:
  - `dispatch step -> candidate output -> contract/gate processing -> accepted
    or rejected evidence -> coverage/evidence ledger -> next action -> owner
    state -> projection/ViewModel`.
  - `TUI/CLI/Web command -> scoped command/EventHub or REST -> owner state ->
    projection/ViewModel`.
- Contract gap:
  - Gate outcome is not the single authority before completed finalization.
  - Contract exhaustion and decision eligibility are still coupled in a way that
    can re-open retry after hard budget exhaustion.
  - Runtime capability, requiredOutput and validator are hand-maintained in
    multiple places.
  - Focus binding metadata is not treated as required structured input for all
    validation evidence routes.
  - Interactive read paths do not yet fully separate compact projection from
    full detail snapshot.
- Boundary check:
  - All proposed changes are business-neutral and operate on state kinds,
    evidence kinds, capability kinds, governance state and projection shape.
  - No template ID, Soul ID, programming language, provider, command output
    marker or workspace-specific rule is added to core.

## Target Rules

### R1: Gate Before Completed Finalization

Runtime must not mark a step completed until the candidate output has passed all
state-changing, decision, approval, graph continuation and completion gates that
can change the step outcome.

Allowed terminal outcomes for one candidate:

| Outcome | Step state | Evidence status | Next owner action |
| --- | --- | --- | --- |
| accepted completed output | `completed` | accepted evidence only after final gate | recompute ledger / dispatch next action |
| decision required | blocked/governed via ticket | rejected or diagnostic result if applicable | wait for decision |
| approval required | blocked/governed via approval | no accepted side effect until approval | wait for approval |
| contract violation | `failed` | rejected diagnostic evidence | bounded retry or blocker/terminal |
| runtime/block failure | `failed` | rejected diagnostic evidence | retry, blocker or terminal |

Completed evidence must never be recorded for a candidate that is later
converted into repair/failure.

### R2: Hard Retry Exhaustion Is Deterministic

Contract violation handling is split into two gates:

1. budget gate: can another LLM retry be dispatched;
2. governance gate: when budget is exhausted, should runtime create a governed
   blocker or fail terminally.

After the budget gate says exhausted, runtime must not dispatch an ordinary
`contract_violation_retry` step. The decision eligibility branch may still decide
whether the final outcome is a decision ticket or terminal failure, but it cannot
create another LLM retry.

### R3: Capability Source Is Single

The following surfaces must read from one neutral capability source:

- `runtimeCapabilities.blockKinds`;
- profile evidence runtime capability gap;
- `requiredOutput.acceptedKinds`;
- execution contract accepted-kind validation.

`validation/managed_session` is a runtime-managed validation capability. It is
not a template-specific or software-development-specific rule.

### R4: Validation Evidence Must Bind Focus Structurally

Any evidence route intended to satisfy validation coverage must carry structured
focus metadata:

- `coverageWorkItemIds`;
- `structuredTargetRefs`;
- `targetRefs`;
- `observedClaims`;
- validation mode and observation type.

`validation/managed_session` raw nodes must be checked as focus-binding
validation nodes before normalization to `shell/command`.

Required-output examples must not use `current_coverage_focus` as a weak
placeholder when current focus metadata is known. Examples should be rendered
from the current authority focus or omit the example rather than suggesting an
unmatchable placeholder.

### R5: Validation Resource Lifecycle Is Idempotent

Managed validation resources are lifecycle-owned by runtime, regardless of
whether execution reaches the shell adapter.

The following paths must finalize pending validation resources idempotently:

- completed step;
- failed step;
- contract violation path;
- decision/approval conversion path;
- run terminal state;
- service restart recovery that observes pending resources from previous runs.

Scratch cleanup must be scoped by workspace/run/step resource ownership and
must not rely on template-specific behavior.

### R6: Interactive Projection Is Compact By Default

TUI/Web/CLI main views consume compact projection or ViewModel state:

- workspace list;
- current workspace main panel;
- prompt/status line;
- event stream updates.

Full detail snapshots are allowed only for explicit detail flows:

- `/status`;
- detail page;
- manual refresh/debug;
- observability diagnostics.

Projection resource optimization must happen in read-model/appservice/TUI
boundaries, not through orchestrator watchdog compensation.

## Implementation Plan

### P0.1 Finalization Ordering Closure

Scope:

- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- evidence recording helpers used by completed runtime steps
- step state publish path

Tasks:

- Split candidate handling into `gateOutcome` and `finalizeAcceptedStep`.
- Move `decisionBlockSpec` and `approvalBlockSpec` handling before
  `step.State = completed`.
- Ensure converted decision repair calls failure handling before any accepted
  evidence or completed step event is recorded.
- Ensure patch preview/apply/revert behavior remains ordered by side-effect
  admission: no completed finalization until governance gates are resolved.
- Add an explicit guard that rejects attempts to record completed runtime
  evidence for a step already classified as failed or governed blocked.

Acceptance:

- A converted decision candidate never publishes a completed step event first.
- A decision ticket path produces governed wait state and rejected/diagnostic
  result as needed, but no accepted completion evidence.
- Existing accepted completion path still records completed evidence once.

### P0.2 Contract Exhaustion Deterministic Closure

Scope:

- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_specs.go`
- contract violation retry tests

Tasks:

- Replace the current exhaustion decision-eligibility retry branch with an
  explicit `contract_violation_decision_or_terminal` outcome after budget
  exhaustion.
- Preserve bounded retry only while `contractViolationRetryWithinBudget` is
  true.
- Make repeated same-rule/signature exhaustion land in a stable blocker or
  terminal state with a visible reason.
- Remove or update tests that expect "budget exhausted -> one more retry with
  decisionEligibilityDenied".
- Verify dispatch recovery from a failed last step cannot return a dispatch spec
  after hard exhaustion.

Acceptance:

- No run can remain `running` with current failed step, no dispatchable recovery
  spec and no blocker/ticket.
- Contract exhaustion creates exactly one scoped decision ticket when user input
  is genuinely required, otherwise terminal failure.
- Rejected LLM output remains rejected diagnostic evidence and never becomes
  accepted evidence/result/artifact.

### P0.3 Capability Contract Single Source

Scope:

- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_blocks.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`

Tasks:

- Introduce a neutral runtime capability catalog helper for evidence block
  kinds.
- Include `validation/managed_session` in that catalog.
- Route `currentRuntimeEvidenceBlockKinds()` through the shared catalog.
- Ensure required-output accepted kinds and execution contract validation use
  the same source for capability visibility checks.
- Add tests that fail if requiredOutput asks for a kind missing from the
  capability catalog.

Acceptance:

- `validation/managed_session` appears in runtime capability gap available
  kinds.
- Managed validation requiredOutput no longer conflicts with capability gap
  diagnostics.
- Adding/removing a managed validation capability requires changing one source,
  not multiple hand-written lists.

### P0.4 Managed Validation Focus Binding Closure

Scope:

- `internal/modules/kernel/orchestrator/biz/runtime_repair_focus_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_contracts.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_evidence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence_records.go`

Tasks:

- Treat raw `validation/managed_session` graph nodes as focus-binding validation
  nodes before normalization.
- Render validation examples from current focus metadata when available:
  `coverageWorkItemIds`, `structuredTargetRefs`, `targetRefs`,
  `observedClaims`.
- Remove weak `current_coverage_focus` placeholders from runtime-required
  output examples where the example could be interpreted as accepted payload.
- Ensure managed validation normalization copies target/focus metadata from
  the original node input/observation into the normalized `shell/command`
  input.
- Ensure `recordValidationResult` and evidence-record enrichment preserve
  coverage item refs and structured target refs for accepted validation
  evidence.

Acceptance:

- A managed validation node for `cwi-005` produces an accepted validation
  evidence record with `CoverageItemRefs` containing `cwi-005`.
- Evidence ledger matching succeeds by `coverage_item_ref` or structured target
  ref without relying on natural-language summaries.
- Validation evidence missing focus metadata is rejected as contract violation
  before it can satisfy coverage.

### P1.1 Managed Validation Resource Lifecycle Closure

Scope:

- `internal/modules/kernel/orchestrator/biz/runtime_managed_validation.go`
- runtime failure/finalization paths
- shell adapter output resource metadata
- run terminal/fail paths

Tasks:

- Add an idempotent runtime helper to mark validation resources as released,
  failed, skipped or cleanup_failed.
- Call the helper from completed, failed, contract violation, converted
  decision/approval and run terminal paths.
- Keep shell adapter cleanup as the primary cleanup for actually executed shell
  processes.
- For resources that never reached shell adapter, cleanup only scratch dirs that
  are under the runtime validation scratch root and owned by the current
  workspace/run/step.
- Add audit metadata: `finalizedAt`, `finalizedBy`, `cleanupStatus`,
  `cleanupError` when applicable.

Acceptance:

- Pending validation resources do not remain pending after any terminal
  candidate outcome.
- Cleanup is safe, scoped and idempotent.
- Tests cover completed, failed, converted decision repair and run terminal
  paths.

### P1.2 Projection/Snapshot Resource Closure

Scope:

- `internal/modules/application/workorchd/biz/appservice/cli_workspace_commands.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelports/workspace_snapshot.go`
- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/**`
- `internal/services/workorch/service/tui/**`
- `internal/pkg/workorchport/**`

Tasks:

- Introduce or reuse a compact typed workspace ViewModel for TUI main view and
  prompt rendering.
- Change embedded TUI refresh to consume projection/ViewModel events by default.
- Reserve `CommandWorkspaceSnapshot` detail hydration for explicit status/detail
  command paths.
- Make `WorkspaceProjectionSnapshot` summary/projection-shaped instead of
  detail-shaped, or rename the detail function so call sites cannot confuse the
  two.
- Keep observability/detail read paths explicit and auditable.

Acceptance:

- Main TUI refresh and prompt update do not call detail projection or ResourceMap
  conversion repeatedly.
- `/status` or explicit detail command can still request detail data.
- pprof/live observation shows idle or governed workspaces do not cause
  repeated full snapshot JSON conversion.

## Verification Plan

Targeted unit tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'Finalization|Decision|Approval|ContractViolation|ManagedValidation|EvidenceRecord|FocusBinding' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/... -run 'WorkspaceProjection|WorkspaceSnapshot|CLI|ViewModel' -count=1
go test ./internal/services/workorch/... ./internal/pkg/workorchport/... -run 'TUI|WorkspaceSnapshot|ViewModel|Prompt' -count=1
```

Package regression tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/... -count=1
go test ./internal/services/workorch/... ./internal/pkg/workorchport/... ./internal/pkg/workorchclient/... -count=1
```

Full regression:

```bash
go test ./... -count=1
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|provider|workspace title" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "current_coverage_focus|validation/managed_session|currentRuntimeEvidenceBlockKinds|acceptedKinds|recordCompletedRuntimeStepEvidence|contractViolationExhaustionDecisionNotEligible|ReadWorkspaceViewSnapshot|CommandWorkspaceSnapshot" internal/modules internal/services internal/pkg docs
```

Live verification:

1. Record git commit, binary path, workorchd PID and start time.
2. Use at least two workspaces to verify multi-workspace behavior.
3. Trigger one normal resume/replan path and one contract-violation path.
4. Confirm event stream order:
   - candidate gate result before completed finalization;
   - no completed event before converted repair/failure;
   - contract exhaustion produces a blocker or terminal event.
5. Confirm managed validation evidence binds to current coverage item.
6. Confirm TUI workspace list, main view and prompt use the same projection
   revision and do not perform repeated detail snapshot hydration during idle or
   waiting states.

## Done Criteria

- Completed step finalization is impossible before decision, approval and
  contract gates resolve the candidate as accepted.
- Contract violation exhaustion cannot dispatch another ordinary retry after
  hard budget exhaustion.
- `validation/managed_session` is visible in runtime capability catalog,
  requiredOutput and execution contract through one neutral source.
- Managed validation evidence accepted by runtime is structurally bound to
  coverage work items and reusable by the evidence ledger.
- Managed validation resources are finalized idempotently across completed,
  failed, converted-governance and terminal paths.
- TUI/Web/CLI main interaction paths consume compact projection/ViewModel state;
  detail snapshots are explicit.
- Core remains business-neutral and contains no template, language, provider,
  command-output or workspace-specific branches.

## Closure Checklist

| ID | Item | Status | Evidence |
| --- | --- | --- | --- |
| P0.1 | Gate before completed finalization | done | `finalizeAcceptedRuntimeStep`, `finalizeGovernedRuntimeStep` and rejected-continuation finalization move accepted evidence after governance/contract gates; graph single-node and multi-node continuation now share the same post-dispatch gate path; targeted and package runtime tests pass |
| P0.2 | Contract exhaustion deterministic blocker/terminal | done | Contract violation recovery no longer dispatches an ordinary retry after hard exhaustion; contract violation tests pass |
| P0.3 | Single capability source includes managed validation | done | Runtime evidence block kind catalog includes `validation/managed_session`; capability/requiredOutput targeted tests pass |
| P0.4 | Managed validation focus/evidence binding | done | Managed validation focus metadata is injected, normalized and preserved into validation evidence records; focus/evidence targeted tests pass |
| P1.1 | Managed validation resource lifecycle | done | Accepted, governed, failed, rejected-continuation, late-result and run terminal paths finalize pending validation resources idempotently; terminal run/workspace state close is authoritative and is not blocked by best-effort resource cleanup read failures |
| P1.2 | TUI/CLI compact projection path | done | `WorkspaceProjectionSnapshot` now uses summary projection by default; `?view=summary` remains projection-only; explicit REST `/snapshot`, `CommandWorkspaceSnapshot`, status and detail paths retain compact detail relations; appservice/server/TUI/backend tests pass |

## Closure Evidence

Validated in this closure pass:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'Finalization|Decision|Approval|ContractViolation|ManagedValidation|EvidenceRecord|FocusBinding' -count=1
go test ./internal/modules/kernel/orchestrator/biz -run 'ManagedValidation|ValidationResource|Finalization|EvidenceRecord|FocusBinding|ContractViolation' -count=1
go test ./internal/modules/kernel/orchestrator/biz -run 'DispatchesLLMFollowUpAfterExecutionGraphCompletes|DispatchStepGraphFollowUpReusesLoadedStepsForEvidence|DispatchesConcreteGraphReturnedByExecutionLLMNode|GraphBatch|GraphFollowUp' -count=1
go test ./internal/modules/kernel/orchestrator/biz -run 'StartRunFailsRunWhenInitialDispatchSpecBuildFails' -count=1 -v
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/... -run 'WorkspaceProjection|WorkspaceSnapshot|CLI|ViewModel' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/... -count=1
go test ./internal/modules/application/workorchd/service/server -run 'TestWorkspaceSnapshotEndpointDefaultUsesExplicitCompactDetail|TestWorkspaceSnapshotEndpointSummaryDoesNotHydrateDetail|TestWorkspaceSnapshotEndpointReturnsCompactProjection' -count=1
go test ./internal/modules/application/workorchd/service/server -count=1
go test ./internal/services/workorch/... ./internal/pkg/workorchport/... -run 'TUI|WorkspaceSnapshot|ViewModel|Prompt|EventHub' -count=1
go test ./internal/services/workorch/... ./internal/pkg/workorchport/... ./internal/pkg/workorchclient/... -count=1
go test ./application/workorch -run 'TestWorkorchServerModeSupportsFileRepositoryBackend|TestWorkorchServerModeRetainsFileBackedTerminalStateAcrossRestart' -count=1 -v
go test ./application/workorch -count=1
git diff -U0 -- internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg | rg -n "^\+.*(software-dev|weather-query|code-review|go test|npm test|no tests to run|city|workspace title|provider)"
```
