# Runtime Ledger Binding Projection Authority Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-29

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-current-live-convergence-improvement-plan.md`
- `runtime-validation-nextaction-convergence-improvement-plan.md`
- `runtime-finalization-contract-evidence-resource-closure-plan.md`
- `runtime-progress-quality-resource-closure-plan.md`

## Purpose

This document scopes the next closure pass after live verification of commit
`2b690fd9`. The latest runs show that WorkOrch still does not converge because
three neutral runtime surfaces are not fully driven by the same authority:

```text
owner run/governance state
  -> projection/ViewModel

coverage/evidence ledger
  -> accepted evidence binding
  -> deterministic NextAction

deterministic NextAction
  -> convergence contract
  -> requiredOutput
  -> repairActionPackage
  -> LLM candidate gate
```

The fix must preserve WorkOrch core business neutrality. No change may branch on
workspace id, workspace title, template id, Soul id, language, command, provider,
file name, or this live sample's business goal.

Current phase does not require compatibility. Existing tests or DTO fields that
encode the older split-authority behavior should be updated.

## Contract Audit

- Scope:
  - Live runs `ws-705249a8d7816330/run-005` and
    `ws-db040d24a190f6e5/run-005`.
  - Workspace list/detail projection, ViewModel prompt mode, coverage/evidence
    ledger binding, deterministic NextAction, requiredOutput rendering, and
    blocked-run resource lifecycle.
- Observed state:
  - workorchd is ready and projection source revision is caught up.
  - Both scoped workspace detail snapshots show `waiting_decision` and active
    run `blocked_on_decision`.
  - `/api/v1/workorch/workspaces` still shows both workspaces as `active` with
    no `blockingTicketId`.
  - `ws-705249a8d7816330/run-005` has 89 coverage items with
    `accepted=0`, `failed=33`, `in_progress=2`, `open=54`.
  - `ws-705249a8d7816330/run-005` has accepted validation evidence records
    carrying `metadata.coverageWorkItemIds` and `metadata.structuredTargetRefs`,
    but the matching coverage item keeps the same validation evidence under
    `supportingEvidenceByKind`, not `acceptedEvidenceByKind`.
  - `ws-705249a8d7816330/run-005` later produced a contract where
    `NextAction=acquire_evidence` and `state=need_evidence`, while current
    requiredOutput also forbade state-changing and patch/apply. The LLM then
    requested a human decision for a continuable candidate choice.
  - `ws-db040d24a190f6e5/run-005` repeatedly received
    `need_state_change` / `stateChangingOutputRequired=true` but the LLM
    returned read-only graphs. Runtime rejected them correctly and escalated
    after bounded retries.
  - Two blocked runs still appear in `workorch_workspace_projection_active_run_heartbeats`.
- Expected state:
  - Workspace list, detail, ViewModel prompt, TUI, and Web must be derived from
    the same owner state/projection revision. A blocked run with an open
    decision must never be displayed as `active` in list projection.
  - Accepted validation evidence that is focus-bound and satisfies the required
    evidence kind must become `acceptedEvidenceByKind.validation` for the
    matching coverage item, not only supporting evidence.
  - Supporting evidence can be visible for audit, but it must not replace the
    accepted binding rule. The ledger must explain why a focused accepted
    evidence record was not accepted for the current gap.
  - A deterministic NextAction must render a non-contradictory current contract.
    If the next action requires new evidence, the required output must expose a
    valid route to acquire it; if no valid route exists, runtime should choose a
    governed blocker or fail-fast internal contract diagnostic instead of asking
    LLM to choose.
  - Blocked runs must leave active heartbeat/resource loops and be represented
    as governed waits, not active execution.
- Deviation:
  - Projection has at least two display derivation paths: list and detail do not
    agree.
  - Evidence ledger and coverage ledger still have a non-closed binding path:
    accepted evidence can be downgraded to supporting evidence without a
    deterministic closure reason.
  - NextAction and requiredOutput can become mutually exclusive in evidence
    acquisition repair after repeated validation/assessment failures.
  - Blocked run lifecycle does not consistently remove active heartbeat state.
- Owner:
  - WorkOrch core owns runtime convergence, coverage/evidence ledger binding,
    governance/blocking state, projection/ViewModel, and resource lifecycle.
  - TUI/Web are projection consumers and scoped command submitters only.
  - WorkspaceTemplate/Soul/policy are not the owner of these specific defects.
- Event path:
  - LLM output -> contract gate -> accepted/rejected evidence record -> coverage
    ledger recompute -> deterministic NextAction -> convergence contract /
    requiredOutput -> dispatch or governed blocker -> projection/ViewModel.
  - Run/decision owner state -> projection builder -> workspace list/detail
    ViewModel -> REST/SSE/port DTO -> TUI/Web display.
- Contract gap:
  - There is no mandatory ledger-binding explanation when accepted evidence is
    not accepted by coverage.
  - Projection list is not using the same blocking-governance source as scoped
    detail.
  - Current action contract rendering does not validate that allowed and
    forbidden routes leave at least one executable/satisfying output shape.
  - Active heartbeat lifecycle is not strictly tied to executable run state.
- Boundary check:
  - All proposed changes use neutral concepts: run state, governance ticket,
    coverage item, evidence kind, accepted/rejected/supporting binding,
    NextAction kind, requiredOutput satisfiability, projection revision, and
    resource ownership.

## Design Alignment

### Goal/DoD NextAction Delivery State Machine

The architecture design requires:

- coverage ledger and evidence ledger are the authority;
- `satisfied` or accepted coverage can only come from accepted evidence;
- runtime derives the only deterministic NextAction;
- LLM violations become rejected diagnostic evidence;
- projection/ViewModel is display-only and does not own business state.

The live deviation is not that LLM output is imperfect. The deviation is that
accepted evidence, deterministic action, and projection are not fully connected
as one closed loop.

### Convergence Authority Simplification

The simplification design requires `gapBaseline`, `convergenceContract`,
`stagePacket`, and `repairActionPackage.requiredOutput` to be derived from the
same authority snapshot before dispatch.

The live deviation shows one remaining class not covered strongly enough:
authority snapshot can select a reasonable action, but downstream requiredOutput
can still have no satisfiable route after combining forbidden output constraints
with repair lineage.

## Root Causes

### R1: Projection List Uses A Stale Or Weaker Workspace Summary

Scoped detail sees the active run and blocking decision. Workspace list still
shows the workspace as `active` without `blockingTicketId`. This means list
projection either reads a workspace-level state that is not recomputed from
active run/governance state, or it falls back when compact run detail is missing.

### R2: Evidence Binding Lacks An Acceptance Decision Ledger

Evidence records can be `status=accepted` and carry focus metadata, but coverage
items may still keep them as supporting evidence. That can be valid only if the
ledger records a deterministic reason, for example wrong evidence role,
insufficient proof strength, stale definition revision, rejected assessment
shadowing, or focus mismatch. The current projection does not expose such a
reason, so runtime keeps retrying a missing validation gap despite having
focus-bound accepted validation records.

### R3: Evidence Acquisition Can Produce Unsatisfiable RequiredOutput

After repeated assessment/validation failures, runtime can select
`acquire_evidence` but also forbid state-changing and patch/apply while asking
for new validation/failure evidence. If no accepted output shape remains that
can satisfy the current gap, the runtime sends an impossible contract to LLM.
The LLM then asks for a decision, which is rejected as a continuable candidate
choice, exhausting repair budget.

### R4: State-Change Retry Is Correctly Rejected But Has No Stronger Recovery

For the state-change sample, runtime correctly rejects read-only graphs. The
remaining gap is not rejection strictness; it is recovery strategy. After
bounded same-contract retry, runtime escalates to a human decision even when the
root cause is "LLM ignored a clear state-change contract", not missing external
information. The governed blocker is acceptable, but it must be projected
consistently and must not leave active execution heartbeats.

### R5: Blocked Run Lifecycle Does Not Close Active Execution Signals

`active_run_heartbeats=2` while both runs are `blocked_on_decision` indicates
that active execution metrics/lifecycle are not closed when the run enters
governed wait. This contributes to CPU/resource ambiguity and incorrect
operator interpretation.

## Target Invariants

### I1: Projection Single Display Source

Workspace list, scoped detail, prompt mode, and stream ViewModel must be derived
from the same owner snapshot and projection revision.

If active run state is `blocked_on_decision`, list projection must expose:

- workspace display state `waiting_decision`;
- `blockingTicketId`;
- prompt mode `decision`;
- no active execution heartbeat.

### I2: Accepted Evidence Binding Must Be Explicit

For every accepted evidence record with focus metadata, coverage recompute must
make exactly one of these decisions:

- bind it as accepted evidence for the matching required evidence kind;
- mark it as supporting evidence with a structured non-acceptance reason;
- reject/supersede it with a structured reason.

No accepted focus-bound evidence may silently remain only supporting evidence.

### I3: Coverage Gap Must Use Binding Decisions, Not Raw Record Status

`missingEvidenceKinds` must be computed from accepted binding decisions, not only
from evidence record status. Supporting evidence is visible but non-satisfying
unless the evidence policy explicitly promotes it.

### I4: RequiredOutput Must Be Satisfiable

Before provider/block dispatch, runtime must check that the current
`NextAction`, `requiredOutput.acceptedKinds`, `forbiddenKinds`,
`allowedOutputShapes`, and `forbiddenOutputShapes` leave at least one satisfiable
route.

If no route remains:

- do not ask LLM to choose;
- recompute authority if stale;
- otherwise create a runtime internal contract diagnostic and either select a
  governed blocker with precise reason or fail-fast according to policy.

### I5: Decision Eligibility Must Require External Or Governance Dependency

An LLM decision request is valid only when it identifies a runtime-unavailable
external dependency, permission, credential, approval, user choice, or
governance policy. Candidate-choice questions such as "should I switch mode" are
runtime contract issues, not user decisions.

### I6: Governed Wait Is Not Active Execution

When a run enters `blocked_on_decision`, `blocked_on_approval`, terminal failed,
or terminal completed, active execution heartbeats and validation resources must
be finalized for that run scope.

## Closure Scope

### P0: Projection List/Detail/ViewModel Alignment

Code scope:

- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/**`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`
- `internal/modules/application/workorchd/service/server/**`
- `internal/pkg/workorchport/**`
- `internal/pkg/workorchclient/**`
- `internal/services/workorch/service/tui/**`

Implementation requirements:

- Build workspace list display state from compact active run/governance
  projection, not only workspace base state.
- Derive prompt mode from blocking governance first, then executable run state,
  then idle workspace state.
- Expose blocking ticket/approval ids in list DTOs and ViewModel DTOs.
- If the compact projection lacks active run/governance fields, report
  projection degraded rather than falling back to `active`.
- Ensure TUI prompt uses ViewModel prompt mode and does not derive decision
  state locally.

Acceptance:

- Regression test: workspace base state `active` + active run
  `blocked_on_decision` + open decision ticket produces list state
  `waiting_decision`, prompt mode `decision`, and matching `blockingTicketId`.
- Regression test: list, scoped detail, and stream ViewModel agree for the same
  projection revision.
- Live check: the two current workspaces no longer show `active` in list while
  detail says `waiting_decision`.

### P0: Evidence Binding Decision Ledger

Code scope:

- `internal/modules/kernel/orchestrator/biz/runtime_step_evidence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence_reuse.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_state.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_context.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence_quality*.go`
- projection code that surfaces coverage/evidence binding diagnostics.

Implementation requirements:

- Introduce a neutral `EvidenceBindingDecision` or equivalent structured record
  during coverage recompute.
- The decision must include evidence ref, coverage item id, evidence kind,
  status (`accepted_binding`, `supporting_only`, `rejected`, `superseded`), and
  reason.
- Accepted validation evidence with matching `coverageWorkItemIds` /
  `structuredTargetRefs`, current definition revision, valid quality, and
  required evidence kind must bind to `acceptedEvidenceByKind.validation`.
- Supporting-only records must carry a reason such as `role_mismatch`,
  `definition_revision_mismatch`, `proof_strength_insufficient`,
  `diagnostic_only`, `stale_focus`, or `assessment_rejected`.
- Projection must expose binding diagnostics for the current focus.

Acceptance:

- Regression fixture based on the current `cwi-001` sample proves a focus-bound
  accepted validation record is either accepted into coverage or emits a
  supporting-only reason. Silent supporting-only is invalid.
- Unit test proves rejected diagnostics with focus metadata remain
  non-satisfying.
- Unit test proves accepted validation closes `missingEvidenceKinds.validation`
  when policy requirements are met.

### P0: RequiredOutput Satisfiability Gate

Code scope:

- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation*.go`

Implementation requirements:

- Add a dispatch-time gate that computes the satisfiable output route set for
  the current NextAction.
- The gate must consider accepted kinds, forbidden kinds, allowed shapes,
  forbidden shapes, state-changing required/forbidden flags, patch
  required/forbidden flags, managed validation availability, and approval
  requirements.
- If route set is empty, classify as `runtime_internal_contract_unsatisfiable`
  and do not send the request to LLM.
- If authority is stale, refresh once and rebuild. If still empty, create a
  governed blocker or terminal failure based on policy.
- Remove "candidate mode switch" questions from LLM-visible decision paths.

Acceptance:

- Regression fixture based on the current `acquire_evidence + stateChangingForbidden`
  sample fails before provider dispatch with a structured internal diagnostic.
- Regression test proves `need_validation` with managed validation or bounded
  shell validation has a non-empty route.
- Regression test proves `need_state_change` does not carry
  `stateChangingForbidden=true`.

### P1: Decision Eligibility Gate Tightening

Code scope:

- `internal/modules/kernel/orchestrator/biz/runtime_decision_gate*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`

Implementation requirements:

- Decision requests emitted by LLM or recovery logic must cite one of:
  external dependency, missing user-provided information, permission,
  credential, approval/governance policy, destructive operation, or explicit
  user choice.
- Continuable candidate choices and mode switches are rejected as internal
  contract issues.
- Exhausted LLM contract retries can still create a decision, but the question
  must describe the governed blocker and allowed recovery actions, not ask the
  user to choose a runtime mode that should be deterministic.

Acceptance:

- Unit test proves "permission to switch to state-changing mode" is rejected as
  a continuable candidate choice when runtime owns the mode.
- Unit test proves a real missing credential or approval remains eligible for
  decision/approval.

### P1: Blocked Run Resource And Heartbeat Finalization

Code scope:

- `internal/modules/kernel/orchestrator/biz/runtime_run_state*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- `internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`
- metrics code for active run heartbeat.

Implementation requirements:

- Define executable run states separately from governed wait states.
- Finalize active execution heartbeat when run enters governed wait or terminal
  state.
- Finalize scoped validation resources on contract violation, decision
  escalation, approval wait, terminal failed, and terminal completed.
- Projection metrics must count active heartbeats only for executable in-flight
  runs.

Acceptance:

- Regression test: `blocked_on_decision` run does not increment active
  heartbeat gauge.
- Resource test: contract violation -> decision path finalizes validation
  resource ownership.
- Live check: blocked workspaces do not continue to appear as active execution
  heartbeats.

### P1: Recovery Strategy For Repeated State-Change Violations

Code scope:

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence_budget.go`

Implementation requirements:

- Keep strict rejection for read-only graphs under state-change contract.
- After repeated same-signature state-change violation, classify the blocker as
  `llm_contract_noncompliance_exhausted` with current focus, expected route,
  and rejected summaries.
- Do not pretend external information is missing unless the blocker cites a
  real external dependency.
- The governed blocker may allow `replan`, `resume same contract`, or
  `terminate`, but projection must show it consistently.

Acceptance:

- Regression fixture based on `ws-db...` read-only graph retries creates one
  governed blocker with consistent list/detail prompt state.
- Rejected read-only outputs remain rejected diagnostic evidence and do not
  produce state-changing side effects.

### P2: Live Verification Harness

Implementation requirements:

- Add a repeatable live verification note or script that captures:
  commit, binary path, pid/start time, workspace ids, run ids, projection
  revision, list/detail state, latest LLM request ids, latest evidence binding
  decisions, CPU/RSS, heartbeat metrics, and scratch/resource deltas.
- Verify one validation-gap workspace and one state-change-gap workspace.

Acceptance:

- Validation-gap workspace either closes validation evidence binding or exposes
  a structured supporting-only reason and a non-contradictory next action.
- State-change-gap workspace either produces state-changing candidate or reaches
  a governed blocker with consistent projection.
- CPU/RSS and heartbeat metrics stop growing after both runs are governed waits.

## Work Order

1. P0 projection list/detail/ViewModel alignment.
2. P0 evidence binding decision ledger.
3. P0 requiredOutput satisfiability gate.
4. P1 decision eligibility tightening.
5. P1 blocked run heartbeat/resource finalization.
6. P1 repeated state-change violation blocker classification.
7. P2 live verification harness.

Projection alignment is listed first because the current operator surface is
misleading even when runtime correctly blocks. It must not be used to hide the
P0 ledger/contract defects; all P0 items are required for positive convergence.

## Verification Commands

Targeted tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./internal/services/workorch/... ./internal/pkg/workorchport/... ./internal/pkg/workorchclient/... -count=1
```

Boundary search:

```bash
rg -n "ws-705249a8d7816330|ws-db040d24a190f6e5|轻量级 LLM 本地代理网关|LSP-Bridge MVP" internal/modules internal/services internal/pkg
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "acceptedEvidenceByKind|supportingEvidenceByKind|coverageWorkItemIds|structuredTargetRefs|requiredOutput|stateChangingForbidden|patchForbidden|blockingTicketId|promptMode" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg internal/services
```

Live verification:

```bash
curl -sS http://127.0.0.1:8080/readyz | jq '.projection,.diagnostics'
curl -sS http://127.0.0.1:8080/api/v1/workorch/workspaces | jq '.workspaces[] | {id,state,activeRunId,blockingTicketId,promptMode}'
curl -sS http://127.0.0.1:8080/api/v1/workorch/workspaces/<workspaceId>/snapshot | jq '.snapshot | {workspace,run,viewModel}'
curl -sS http://127.0.0.1:8080/metrics | rg 'workorch_workspace_projection_active_run_heartbeats|workorch_event_record_metadata|workorch_workspace_projection_rebuild'
```

## Done Criteria

- Workspace list/detail/ViewModel/prompt agree for governed waits.
- Focus-bound accepted validation evidence either closes the coverage gap or
  carries a structured supporting-only reason.
- No provider-facing request is emitted when requiredOutput has no satisfiable
  route.
- LLM decision requests for runtime-owned mode switching are rejected before
  they become user-facing blockers.
- Blocked runs do not keep active execution heartbeat/resource signals.
- Core remains business-neutral; live workspace ids appear only in this audit
  document and tests/fixtures that explicitly model the neutral failure class.
