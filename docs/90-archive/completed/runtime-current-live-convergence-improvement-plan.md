# Runtime Current Live Convergence Improvement Plan

Status: code-closure-applied

Type: closure-plan

Last Updated: 2026-06-29

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-validation-nextaction-convergence-improvement-plan.md`
- `runtime-finalization-contract-evidence-resource-closure-plan.md`
- `runtime-validation-contract-gate-managed-session-closure-plan.md`
- `runtime-managed-validation-resource-lifecycle-closure-plan.md`
- `projection-event-io-resource-closure-plan.md`

## Purpose

This document scopes the next closure pass after the latest live verification.
The observed workspaces are not failing because WorkOrch needs more
business-specific rules. They are failing because the neutral convergence
contract still has inconsistent authority surfaces:

```text
Coverage/Evidence Ledger
  -> Deterministic NextAction
  -> Convergence Contract
  -> Stage Packet / requiredOutput
  -> Provider-facing runtimeCapabilities
  -> Contract Gate
  -> Evidence Ledger
  -> Projection/ViewModel
```

All items in this plan must preserve WorkOrch core business neutrality. No fix
may branch on workspace id, template id, Soul id, programming language, command,
file name, provider, workspace title, or the current live workspace goal.

Current phase does not require compatibility. If existing behavior or tests
encode the older inconsistent contract, update them to match this document.

## Contract Audit

- Scope:
  - Active live runs `ws-705249a8d7816330/run-004-replan-001` and
    `ws-db040d24a190f6e5/run-004`.
  - Runtime capability catalog, current action contract rendering,
    requiredOutput synchronization, evidence binding, projection/ViewModel
    summary, and validation resource lifecycle.
- Observed state:
  - Both scoped workspace detail endpoints report `waiting_decision` and their
    active runs report `blocked_on_decision`.
  - The workspace list endpoint still reports the same workspaces as
    `active` with `promptMode=workspace`, while scoped detail carries the
    blocking decision ids.
  - `ws-705249a8d7816330/run-004-replan-001` reached a validation gap for
    focus `cwi-085`. Its requiredOutput accepts and examples
    `validation/managed_session`, but provider-facing
    `runtimeCapabilities.blockKinds` does not declare that kind. The LLM tried
    shell validation and was repeatedly rejected for process-control policy
    violations.
  - `ws-db040d24a190f6e5/run-004` is in `need_state_change` and the stage
    requiredOutput requires state-changing output, but
    `repairActionPackage.repairObjective` still says
    `acquire_concrete_failure_evidence` and
    `repairActionPackage.evidenceAcquisitionRequired=true`. The LLM returned a
    read/build graph with no `patch/apply`, which the runtime correctly
    rejected.
  - Rejected validation evidence records exist, but several records lack
    `coverageWorkItemIds`, `structuredTargetRefs`, `targetRefs`, and
    `observedClaims`, weakening retry diagnostics and coverage ledger reuse.
  - `/tmp/workorch-validation-scratch` contains multiple historical scratch
    trees and stale validation processes, showing that validation resource
    cleanup is still incomplete.
- Expected state:
  - Provider-facing `runtimeCapabilities.blockKinds`,
    requiredOutput accepted kinds, runtime evidence capability gap checks, and
    execution contract validation must come from one neutral capability source.
  - Current `NextAction` owns `convergenceContract`,
    `stagePacket.requiredOutput`, top-level `requiredOutput`, and
    `repairActionPackage.requiredOutput`. Old repair/evidence-acquisition
    fields are diagnostic lineage only.
  - A validation gap may use `validation/managed_session` when managed
    validation is advertised and contract-valid. If the LLM still returns
    invalid shell process-control output, rejected diagnostics must guide a
    bounded retry without contradicting the validation contract.
  - A state-change gap must not expose stale failure-evidence objective fields
    in the current action package.
  - Workspace list, main view, prompt mode, scoped detail, and SSE projection
    must represent the same owner state revision.
  - Runtime-owned validation scratch/process resources must be released
    idempotently on completed, failed, contract-violation, governed-blocked,
    and terminal paths.
- Deviation:
  - Capability declaration is split between kernel convergence and LLM adapter.
  - Current action synchronization clears stale fields only in some forced
    narrow paths, not in the general authority-to-dispatch path.
  - Projection list summary and scoped detail snapshot can disagree about
    blocking governance state.
  - Evidence records and validation resources do not yet form a closed loop for
    failed managed/bounded validation attempts.
- Owner:
  - WorkOrch core for runtime capability, convergence authority, contract gate,
    evidence ledger, resource lifecycle, and projection/ViewModel.
  - TUI/Web remain projection consumers and scoped command submitters.
  - WorkspaceTemplate/Soul/policy are not the owner of these defects.
- Event path:
  - LLM step request -> provider-facing capability catalog -> candidate graph
    -> adapter schema gate -> runtime execution contract -> accepted/rejected
    evidence -> coverage recompute -> deterministic NextAction -> projection.
  - Owner state snapshot -> workspace projection store -> REST/SSE/port DTO
    -> TUI/Web list, main view, and prompt.
- Contract gap:
  - The current action is not yet rendered from one authority snapshot all the
    way to provider capabilities and requiredOutput.
  - Diagnostic lineage fields can still appear as current action fields.
  - Projection summary can preserve stale workspace state while detail is
    correct.
  - Resource ownership metadata is recorded but not fully finalized.
- Boundary check:
  - Proposed changes use only capability kind, evidence kind, action kind,
    focus binding, governance state, resource ownership, and projection
    revision. They do not introduce template, language, provider, or
    workspace-specific behavior.

## Design Alignment

### Goal/DoD NextAction Delivery State Machine

The architecture document requires runtime to determine the next action from
ledger state:

1. missing `state_change` evidence means `state_change`;
2. accepted state change with missing `validation` evidence means `validate`;
3. sufficient evidence then leads to `assessment` or `complete`;
4. contract violations become rejected diagnostics and bounded retries, not
   accepted evidence.

The current live failures violate the handoff between steps 1/2 and provider
dispatch. The ledger-derived action is visible in some fields, but provider
capability and `repairActionPackage` do not always match it.

### Convergence Authority Simplification

The simplification design states that `gapBaseline`, `convergenceContract`,
`stagePacket`, and `repairActionPackage.requiredOutput` are derived fields.
They must be rebuilt from the same current authority snapshot before dispatch.

The current live failures show two remaining authority leaks:

- provider-facing runtime capability is maintained separately from runtime
  evidence capability;
- stale `repairActionPackage` diagnostic lineage can remain visible alongside a
  newer state-changing contract.

## Root Causes

### R1: Runtime Capability Catalog Is Not Single Source

Kernel convergence declares `validation/managed_session` as available runtime
evidence capability, and validation requiredOutput recommends it. The LLM
adapter still builds `runtimeCapabilities.blockKinds` from its own hand-written
catalog and rejects graph nodes before kernel managed-validation normalization
can run.

This turns an intended runtime-managed validation route into an impossible
contract: the LLM is told it may use `validation/managed_session`, but the
provider-facing schema says it cannot.

### R2: Current Action Package Is Not Fully Normalized On General Dispatch

Forced-narrow state-changing recovery clears stale evidence-acquisition fields.
The general convergence-authority path can still leave:

```text
convergenceContract.state = need_state_change
repairObjective = produce_state_changing_output
repairActionPackage.repairObjective = acquire_concrete_failure_evidence
repairActionPackage.evidenceAcquisitionRequired = true
```

The validator correctly rejects read-only graphs, but the LLM-visible contract
is noisy and contradictory, so repeated retries keep producing low-value
inspection graphs.

### R3: Rejected Evidence Is Not Rich Enough For Retry Guidance

Rejected validation records can lack focus-binding metadata. The rejected
diagnostic exists, but it is not always reusable by the coverage/evidence
ledger as a precise "do not repeat this for this focus" signal.

### R4: Projection Summary And Detail Can Diverge

The scoped workspace detail is correct, but the workspace list summary can still
show `active/ready/workspace prompt` while the owner state is
`waiting_decision` with an open blocking ticket. This breaks MVVP: TUI/Web will
display a projection-owned view, but the projection itself is internally
inconsistent.

### R5: Validation Resource Lifecycle Still Leaks

Scratch directories and process resources are runtime-owned. They must be
released even when a validation graph is rejected, converted to decision,
fails contract gate, or reaches terminal state before shell execution finishes.

## Target Invariants

### I1: Capability Single Source

One neutral runtime capability source must drive:

- provider-facing `runtimeCapabilities.blockKinds`;
- adapter graph kind validation;
- convergence capability gap checks;
- requiredOutput accepted kinds;
- execution contract accepted-kind validation.

`validation/managed_session` is a core runtime-managed validation capability
and must be declared wherever a managed validation node can be requested or
accepted.

### I2: Current Action Contract Single Source

Before provider/block dispatch, the runtime must ensure these fields are
derived from the same current authority snapshot:

- `convergenceAuthority.nextAction`;
- `convergenceContract`;
- `stagePacket.requiredOutput`;
- top-level `requiredOutput`;
- `repairActionPackage.requiredOutput`;
- `repairObjective`;
- `stateChangingOutputRequired`;
- validation/evidence acquisition flags.

For `need_state_change`, stale failure-evidence flags must be removed or moved
under diagnostic lineage. For `need_validation`, stale state-changing or
failure-evidence output contracts must be removed or moved under diagnostic
lineage.

### I3: Adapter Must Not Reject Runtime-Managed Nodes Before Normalization

If a node kind is a declared runtime-managed pseudo block, adapter validation
must either:

- declare it in provider capabilities and allow it through to runtime
  normalization; or
- normalize it before generic graph-kind validation.

The chosen implementation must be single-source and business-neutral.

### I4: Rejected Diagnostics Must Carry Focus Metadata

Rejected validation/state-change diagnostics must preserve available focus
metadata:

- `coverageWorkItemIds`;
- `structuredTargetRefs`;
- `targetRefs`;
- `observedClaims` when supplied;
- violated rule id and actual graph summary.

Rejected evidence still cannot satisfy coverage, but it must be precise enough
to guide the next bounded retry.

### I5: Projection ViewModel Is The Display Authority

Workspace list, scoped detail, prompt mode, current run state, and blocking
governance must come from the same projected owner snapshot or expose a
degraded/stale diagnostic. Projection may not show `active/workspace prompt`
when the active run is `blocked_on_decision` with a blocking ticket.

### I6: Validation Resources Are Idempotently Finalized

Every runtime-owned validation resource must reach terminal resource state on:

- completed step;
- failed step;
- contract violation;
- decision/approval conversion;
- run terminal;
- service restart observation of orphaned resources.

Cleanup is scoped by resource ownership metadata, not by template or language.

## Closure Scope

### P0: Runtime Capability Catalog Single Source

Code scope:

- `internal/modules/blocks/llm/biz/runtime_adapter.go`
- `internal/modules/blocks/llm/biz/runtime_adapter_test.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_managed_validation.go`
- shared model/package if a neutral capability catalog needs to move out of
  one package to avoid import cycles.

Implementation requirements:

- Introduce or expose one neutral declared runtime block capability catalog.
- Include `validation/managed_session` with input shape for managed
  observation, validation evidence metadata, scratch/timeout/port allocation,
  and cleanup policy.
- Make adapter graph kind validation use the same catalog that requiredOutput
  and execution contract use.
- Ensure managed validation graph nodes can pass adapter schema validation and
  reach runtime normalization/contract validation.

Acceptance:

- Unit test proves provider request `runtimeCapabilities.blockKinds` includes
  `validation/managed_session`.
- Unit test proves an execution graph containing `validation/managed_session`
  is not rejected as an undeclared kind by the adapter.
- Unit test proves requiredOutput accepted kinds are a subset of declared
  runtime capabilities, except for explicitly modeled non-executable output
  markers.

### P0: Current Action Package Synchronization

Code scope:

- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`

Implementation requirements:

- Add a generic current-action normalization path that runs for every dispatch,
  not only forced-narrow contract retry.
- For `need_state_change`, set the current objective to
  `produce_state_changing_output`, set state-changing required output, and
  remove stale `evidenceAcquisitionRequired`,
  `concreteFailureEvidenceRequired`, evidence-acquisition exit criteria, and
  old failure-evidence output contracts from the current package.
- For `need_validation`, set the current objective to
  `produce_concrete_validation_evidence`, set concrete validation required
  output, and remove stale state-changing/failure-evidence contracts.
- Keep old package contents only under diagnostic lineage fields when useful.
- Add dispatch gate checks that fail fast on contradictory combinations such as
  `need_state_change` plus `repairActionPackage.repairObjective =
  acquire_concrete_failure_evidence`.

Acceptance:

- Regression fixture based on `ws-db040d24a190f6e5/run-004` proves stale
  evidence-acquisition flags are absent from provider-visible current action
  input when current authority is `need_state_change`.
- Regression fixture proves `need_validation` does not carry
  `mustProduceConcreteFailureEvidence`.
- Dispatch gate test proves contradictory current action fields are rejected
  before provider dispatch.

### P0: Validation Contract And Managed Session Retry Closure

Code scope:

- `internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_managed_validation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_blocks.go`

Implementation requirements:

- Render managed validation examples only when the capability is declared in
  the current capability catalog.
- Ensure validation examples use current authority focus metadata, not
  placeholder `current_coverage_focus`, when focus metadata is known.
- Preserve a non-destructive bounded shell validation route, but prefer or make
  available managed validation when process lifecycle, port allocation, or
  scratch cleanup is required.
- Keep process-control rejection strict. Do not relax shell policy to allow
  background `&`, `kill`, `pkill`, `killall`, `setsid`, or `nohup`.

Acceptance:

- Regression fixture based on `ws-705249a8d7816330/run-004-replan-001` proves
  validation requiredOutput and provider-facing capabilities agree on
  `validation/managed_session`.
- Unit test proves validation requiredOutput examples carry
  `coverageWorkItemIds` and `structuredTargetRefs`.
- Contract gate test proves destructive shell validation remains rejected and
  becomes rejected diagnostic evidence, not accepted validation.

### P1: Evidence Binding For Rejected And Accepted Validation

Code scope:

- `internal/modules/kernel/orchestrator/biz/runtime_step_evidence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence_reuse.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_focus_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence_quality*.go`

Implementation requirements:

- Normalize focus binding for raw `validation/managed_session` nodes before
  and after managed validation normalization.
- When recording rejected validation diagnostics, copy available
  `coverageWorkItemIds`, `structuredTargetRefs`, `targetRefs`, and
  `observedClaims` from node metadata, requiredOutput, or current authority
  focus.
- Keep rejected diagnostics rejected. Do not let focus metadata make rejected
  records satisfy coverage.

Acceptance:

- Unit test proves rejected validation evidence keeps focus metadata.
- Unit test proves accepted validation evidence binds the same focus metadata
  and can be reused by coverage ledger.
- Unit test proves rejected diagnostics remain non-satisfying.

### P1: Projection List/Detail/ViewModel Alignment

Code scope:

- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/store.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`
- `internal/modules/application/workorchd/service/server/workspace_projection.go`
- `internal/modules/application/workorchd/biz/appservice/cli_workspace_commands.go`
- `internal/services/workorch/service/tui/**`
- `internal/pkg/workorchport/**`
- `internal/pkg/workorchclient/**`

Implementation requirements:

- Workspace list projection must derive workspace display state and prompt mode
  from the same active run/blocking governance snapshot as scoped detail.
- If list summary lacks the active run detail needed to determine blocking
  state, it must use a compact run/governance projection rather than falling
  back to `active/workspace`.
- TUI prompt mode must consume the server ViewModel/projection. It may not
  clear blocking ticket ids because a local overview appears "live".
- Full detail hydration remains explicit detail/status behavior, not the main
  list/prompt refresh path.

Acceptance:

- Projection test proves workspace list reports `waiting_decision` and
  `promptMode=decision` when active run is `blocked_on_decision`.
- TUI model test proves switching to a waiting-decision workspace updates
  prompt from projection without local state inference.
- REST/SSE test proves list, scoped detail, and event stream expose the same
  blocking ticket id for the same projection revision.

### P1: Validation Resource Lifecycle Closure

Code scope:

- `internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_run_state*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_managed_validation.go`
- service-start recovery code if orphan observation is implemented there.

Implementation requirements:

- Introduce one idempotent resource finalizer for validation resources owned by
  a workspace/run/step.
- Call it on completed, failed, contract violation, governed blocker, and run
  terminal paths.
- On service start, identify owned orphan resources from persisted ownership
  metadata and mark them recoverable/cleanup-pending or clean them up
  according to policy.
- Do not scan all `/tmp` on a hot loop. Cleanup must be scoped and bounded.

Acceptance:

- Unit/resource test proves contract violation path releases process group and
  scratch_dir records.
- Unit/resource test proves decision conversion path releases resources.
- Live verification shows no new stale validation processes after a failed
  validation run reaches decision.

### P2: Live Verification And Resource Regression

Implementation requirements:

- Add a scripted or documented live verification checklist for two-workspace
  runs.
- Record commit, service pid/start time, workspace ids, run ids, latest
  projection revision, latest LLM request ids, and terminal or governed state.
- Capture CPU/RSS and scratch growth before and after live validation.

Acceptance:

- `ws-705...`-style validation gap either produces accepted validation evidence
  or a governed blocker with consistent managed-validation capability
  diagnostics.
- `ws-db...`-style state-change gap either produces a state-changing candidate
  or a governed blocker with no stale evidence-acquisition current contract.
- Workspace list, scoped detail, TUI prompt, and Web status agree.
- Scratch/resource growth is bounded for the run scope.

## Work Order

1. P0 capability catalog single source.
2. P0 current action package synchronization.
3. P0 validation managed-session contract agreement.
4. P1 evidence binding for validation diagnostics.
5. P1 projection list/detail/ViewModel alignment.
6. P1 validation resource lifecycle closure.
7. P2 regression and live verification.

Do not start P1 projection cleanup as a substitute for P0 runtime contract
fixes. A correct UI over contradictory runtime contracts still will not improve
goal convergence.

## Verification Commands

Targeted tests after code closure:

```bash
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./internal/services/workorch/... ./internal/pkg/workorchport/... ./internal/pkg/workorchclient/... -count=1
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|city|workspace title|ws-705249a8d7816330|ws-db040d24a190f6e5" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/modules/blocks internal/pkg internal/services
rg -n "validation/managed_session|runtimeCapabilities|acceptedKinds|repairObjective|evidenceAcquisitionRequired|promptMode|blockingTicketId|validationScratchDir" internal/modules internal/pkg internal/services
```

Live verification:

```text
1. Start workorchd from the target commit.
2. Record /readyz projection revision and process resource usage.
3. Resume or replan both workspaces.
4. For each active run, inspect latest request.json:
   - runtimeCapabilities includes every current requiredOutput accepted kind;
   - convergenceContract, stagePacket, top requiredOutput, and repairActionPackage agree.
5. Inspect evidence records:
   - accepted records close coverage only when status accepted;
   - rejected records preserve focus metadata but do not satisfy coverage.
6. Inspect projection:
   - /workspaces list, scoped detail, SSE, TUI prompt, and Web state agree.
7. Inspect validation resources:
   - no newly orphaned scratch/process resources remain after terminal/governed state.
```

## Done Criteria

- Provider-facing runtime capabilities and runtime contract accepted kinds are
  generated from one neutral source.
- `validation/managed_session` is either consistently declared and accepted, or
  consistently absent from requiredOutput. The target design is declared and
  accepted.
- Current action package fields cannot contradict deterministic NextAction at
  dispatch.
- State-change retries do not expose stale failure-evidence current contracts.
- Validation retries do not expose stale state-change or failure-evidence
  current contracts.
- Rejected diagnostics carry focus metadata without satisfying coverage.
- Workspace list/detail/ViewModel/TUI prompt agree on blocking governance.
- Validation resources are finalized on all runtime terminal/governed paths.
- All closure remains business-neutral.

## Closure Notes - 2026-06-29

Implemented in this closure pass:

- Declared `validation/managed_session` as a neutral runtime block kind shared
  by the provider-facing LLM adapter and kernel managed-validation
  normalization.
- Added adapter regressions proving managed validation is advertised and is not
  rejected as an undeclared execution graph kind.
- Normalized current action packages for `need_state_change` and
  `need_validation` so stale evidence-acquisition or state-changing fields do
  not remain visible beside a newer deterministic NextAction.
- Added a dispatch gate that rejects contradictory current-action packages
  before provider dispatch.
- Enriched validation evidence scope extraction from requiredOutput,
  stagePacket, repairActionPackage, and current focus metadata.
- Preserved snapshot governance authority in workspace projection list/detail
  alignment, including run-level blocked governance state while avoiding
  resolved decision/approval blockers.
- Made validation resource finalization idempotent for active resources and
  covered scratch cleanup with a resource lifecycle regression.

Remaining live-only verification:

- Re-run the two-workspace live scenario and confirm request payloads,
  projection list/detail/TUI prompt, evidence records, and validation scratch
  growth match the Live verification checklist above.
