# Runtime Validation Evidence And Context Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-27

Related:

- `../superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-coverage-convergence-state-machine-closure-plan.md`
- `projection-event-io-resource-closure-plan.md`
- `workspace-goal-convergence-acceleration-design.md`

## Purpose

This plan captures the closure scope from the latest live verification where one
workspace kept progressing but did not converge efficiently, while another
workspace remained blocked by a contradictory repair contract.

The goal is to close the remaining gap between the target design and runtime
behavior:

```text
Coverage/Evidence Ledger
  -> deterministic NextAction
  -> narrow Stage Packet
  -> candidate generation or runtime validation
  -> accepted/rejected evidence
  -> ledger recompute
  -> complete / governed blocked / failed
```

This plan is business neutral. Core changes must not branch on template id,
workspace title, programming language, command text, tool name, provider, file
name, or live workspace id. Software-development validation preferences must be
provided through workspaceTemplate/profile/Soul/policy data and consumed by
generic runtime logic.

## Contract Audit

- Scope:
  - Live workspace `ws-4e83810542ac4d65`, run
    `run-006-replan-001-replan-001`.
  - Live workspace `ws-7ced70e98ea50c43`, run
    `run-005-replan-001-replan-001`.
  - Runtime coverage/evidence ledger, deterministic NextAction, validation
    dispatch, Stage Packet rendering, LLM request/context persistence and
    repeated repair contract eligibility.
- Observed state:
  - `ws-4e83810542ac4d65` was still `running`, projection was `ready`, and
    `sourceEventRevision == projectedSourceRevision`.
  - The workspace had real positive progress: patches, build/test steps,
    runtime validation steps and MCP/tool validation steps were accepted.
  - Coverage reached `26 evidence_ready / 40 total`, leaving `14 open`.
  - Runtime repeatedly generated follow-up validation graphs for individual
    coverage items even after earlier validation evidence was accepted.
  - Step inputs grew to roughly `0.5MB-1MB`; LLM request logs were roughly
    `1.3MB-1.6MB`; event payloads reached roughly `100KB-250KB`.
  - A later validation attempt for stability-style checks was rejected because
    the generated shell graph attempted process termination, which violates the
    non-destructive bounded validation contract.
  - `ws-7ced70e98ea50c43` remained `waiting_decision` with a
    `patch_apply_contract_failed` blocker. The effective contradiction was
    `strategyReassessment.required=true` plus `shift_root_cause_target` while
    the currently actionable repair still targeted the original safe snapshot.
- Expected state:
  - Accepted validation evidence should be reusable across all matching
    coverage items before dispatching another validation request.
  - Runtime should derive whether validation can be satisfied by existing
    evidence, deterministic runtime validation, LLM-generated validation,
    template-bound managed validation, deferred evidence, governance, or fail.
  - Destructive or environment-dependent validation should not be repeatedly
    handed to the LLM as ordinary shell graph generation.
  - Stage Packet and step input should carry the current focus and referenced
    ledger/evidence summaries, not repeatedly persist the full execution
    context and full coverage ledger for every step.
  - Repeated failed-surface repair should only force a distinct root-cause
    target when such a target is grounded and the current focus allows that
    shift.
- Deviation:
  - Runtime still asks the LLM to generate similar validation graphs per
    coverage item instead of first applying evidence reuse and deterministic
    validation routing.
  - Current validation NextAction lacks an explicit non-destructive / managed /
    deferred / governed validation decision.
  - Stage Packet persistence and LLM context rendering still cause large
    repeated writes during long validation loops.
  - The root-cause shift repair mode can become stricter than the current
    actionable repair contract and can block a valid same-target repair.
- Owner:
  - Core: coverage/evidence attribution, deterministic NextAction,
    validation dispatch routing, Stage Packet context budget, completion gate
    and repair-mode eligibility.
  - Template/Profile/Soul/Policy: concrete software-development validation
    preferences, destructive validation capability, managed harness allowance,
    CI/deferred evidence rules and tool ordering preferences.
- Event path:
  - Runtime step result -> accepted/rejected evidence record -> coverage ledger
    recompute -> deterministic NextAction -> Stage Packet -> validation/LLM
    dispatch -> contract gate -> evidence record -> projection ViewModel.
- Contract gap:
  - Missing evidence reuse and multi-target attribution before validation
    dispatch.
  - Missing validation NextAction subtypes for existing-evidence reuse,
    deterministic runtime validation, LLM validation graph, managed validation,
    deferred validation, governed blocker and fail.
  - Missing policy-driven destructive/environment validation classification.
  - Missing narrow Stage Packet persistence for validation steps.
  - Missing eligibility guard for forced root-cause target shift.
- Boundary check:
  - Core must only operate on structured fields such as evidence kind,
    evidence target refs, evidence tags, validation safety class, policy refs,
    NextAction type, focus ids and idempotency keys.
  - Any software-development-specific rule must be represented as
    workspaceTemplate/profile/Soul/policy data and bound into workspace/run
    context.
- Verification:
  - Add unit tests for evidence reuse, validation routing, Stage Packet
    slimming and root-cause shift eligibility.
  - Add boundary searches proving no new template/language/tool/file/provider
    special cases entered core.
  - Run live verification on at least two workspaces and record projection
    revision, accepted/rejected evidence refs, request size trend, CPU/IO
    trend and final governed state.

## Closure Scope

### S1: Evidence Reuse And Attribution

Problem:

- Accepted validation evidence is stored, but runtime still dispatches repeated
  validation actions for similar coverage items.
- A single validation command/tool result can often satisfy several coverage
  items when it carries matching structured scope.

Target:

- Before creating a new validation NextAction, runtime must search accepted
  evidence for the current open coverage item.
- Evidence matching must use structured metadata only:
  - `requiredEvidenceKinds`
  - `targetRefs`
  - `coverageTargetIds`
  - `sourceRequirementIds`
  - `evidenceTags`
  - `policyRef`
  - `validationScope`
  - `quality`
- Evidence can satisfy multiple coverage items only when policy allows reuse
  and the evidence scope covers each item.
- Reused evidence must record an audit edge so completion remains traceable.

Expected implementation areas:

- Runtime coverage/evidence ledger update logic.
- Evidence recorder and evidence query helpers.
- Coverage work item recompute path.
- Projection coverage summary if it needs to display reused refs.

Acceptance:

- A fixture with one accepted validation evidence record and three matching
  open coverage items recomputes all three to evidence-ready without another
  LLM call.
- A non-matching evidence record is not reused.
- Reused evidence remains accepted evidence, not memory or compact summary.

### S2: Deterministic Validation NextAction Routing

Problem:

- Missing validation evidence currently tends to produce another LLM-generated
  validation graph.
- Runtime does not consistently distinguish validation that can be reused,
  runtime-executed, LLM-generated, managed by template harness, deferred, or
  blocked.

Target:

Extend validation NextAction with business-neutral routing:

```text
validate_existing_evidence
validate_runtime_action
validate_llm_graph
validate_managed_harness
validate_deferred
request_decision
fail
```

Routing order:

1. Existing accepted evidence reuse.
2. Deterministic runtime validation already described by a safe structured
   action.
3. Template/profile-declared managed validation harness.
4. LLM-generated validation graph only when new candidate graph generation is
   truly needed.
5. Deferred/external evidence when local environment cannot safely prove the
   requirement.
6. Governance or fail when policy disallows automatic continuation.

Expected implementation areas:

- NextAction interpreter.
- Stage Contract builder.
- Validation dispatch admission gate.
- Contract violation recovery budget, so invalid validation graphs do not
  cause unbounded LLM retries when routing can be determined.

Acceptance:

- Missing validation with reusable evidence does not call LLM.
- Missing validation with a policy-declared runtime action dispatches that
  action directly.
- Missing validation requiring LLM still carries a narrow contract.
- Destructive validation requests do not retry as ordinary shell graphs.

### S3: Template-Bound Managed And Deferred Validation Policy

Problem:

- Some validation items require operations that are destructive, expensive,
  environment-dependent or better suited to CI.
- Core correctly rejects destructive shell termination, but lacks a structured
  alternative path besides retrying LLM output.

Target:

Add template/profile/Soul/policy data that can describe validation handling
without core special cases:

```text
ValidationPolicy
  id
  appliesToEvidenceKinds[]
  validationSafetyClass      # safe | destructive | external | ci_only | manual
  allowedExecutionModes[]    # runtime_action | llm_graph | managed_harness | deferred | decision
  managedHarnessRef
  deferredEvidenceKind
  governanceReason
  maxRetries
```

Core behavior:

- Core reads policy refs and applies generic safety/routing rules.
- Core never infers behavior from natural language text such as a specific
  command, language, tool, workspace title or DoD wording.
- Destructive validation without an allowed managed harness must become
  deferred evidence or governed blocker.

Expected implementation areas:

- Workspace template/profile binding data.
- Run execution context policy hydration.
- Generic validation policy interpreter.
- Projection ViewModel fields for deferred/governed validation.

Acceptance:

- A policy fixture marks a validation item as `ci_only`; runtime emits
  deferred validation evidence requirement instead of an LLM shell graph.
- A policy fixture marks a validation item as `managed_harness`; runtime
  dispatches only the referenced harness.
- A generic template without policy keeps the existing safe validation behavior.

### S4: Narrow Stage Packet And Step Input Persistence

Problem:

- Long validation runs persist full execution context and full coverage ledger
  into each step input and LLM request.
- This causes CPU/IO and memory pressure during multi-step validation loops.

Target:

Dispatch-time Stage Packet should persist:

- Full current focus.
- Current requiredOutput and allowed/forbidden actions.
- Recent accepted/rejected evidence relevant to current focus.
- Coverage summary and full ledger reference.
- Historical compact summary reference.
- Snapshot/payload refs for large objects.

Dispatch-time Stage Packet should not persist by default:

- Full `executionContext.coverageWorkItems` for every step.
- Full historical event payloads.
- Full unrelated accepted/rejected evidence payloads.
- Full tool outputs not required for current action.

Observability/debug APIs may hydrate full details explicitly.

Expected implementation areas:

- Stage Packet renderer.
- Runtime step input builder.
- LLM C3/C4/C5 rendering.
- Request log persistence.
- Observability/debug detail APIs.

Acceptance:

- A validation step input with a 40-item ledger carries current focus and
  `ledgerRef` rather than the full ledger body.
- Required current Goal/DoD, focus, requiredOutput, current failure, accepted
  and rejected evidence refs remain present.
- Request size does not grow linearly with every completed validation step.

### S5: Completion And Assessment Gate From Ledger

Problem:

- Runtime continues asking LLM for validation/follow-up actions while enough
  structured evidence may already exist for several items.
- Large DoD can slow convergence even when evidence is already present.

Target:

- Completion and assessment gates must run after every evidence update.
- If all required evidence for the current focus is accepted, runtime should
  move to assessment or next open focus deterministically.
- If all required coverage is satisfied or explicitly waived/deferred by
  policy, runtime should complete or enter governed completion review without
  another LLM self-assessment.
- `large DoD` only affects slicing order; it must not force repeated
  validation for already covered items.

Expected implementation areas:

- Coverage ledger recompute.
- Completion gate.
- Assessment NextAction.
- Projection coverage summary and next-action display.

Acceptance:

- After evidence recompute marks all required items satisfied, the next action
  is `complete` without another LLM provider call.
- A large coverage set with accepted evidence does not produce a large-DoD
  blocker.
- Deferred/governed items are displayed explicitly and do not masquerade as
  satisfied evidence.

### S6: Root-Cause Shift Eligibility Guard

Problem:

- Forced `shift_root_cause_target` can conflict with a valid same-target repair
  when the current safe target snapshot is still the actionable repair surface.

Target:

`shift_root_cause_target` is eligible only when all of the following are true:

- A distinct root-cause candidate is grounded by structured diagnostics or
  current target snapshots.
- The current focus permits shifting away from the failed surface.
- The required output can still satisfy the current coverage focus.
- The shifted target has sufficient snapshot/readiness metadata for the patch
  mode required by the contract.

If these are not true:

- Keep same-target repair valid when current snapshot is safe and actionable.
- Or route to a structured read-required / governance blocker when no safe
  target exists.
- Do not escalate to decision solely because the LLM patched the current
  actionable target.

Expected implementation areas:

- Strategy reassessment / repeated target repair mode.
- Repair action package builder.
- Contract gate for patch/apply.
- Contract retry correction profile.

Acceptance:

- A fixture with only one grounded safe failure target does not force
  `shift_root_cause_target`.
- A fixture with a grounded distinct root-cause target can still require
  `shift_root_cause_target`.
- Same-target valid patch is accepted when current focus and snapshot allow it.

## Sequencing

Recommended development order:

1. S1 Evidence Reuse And Attribution.
2. S2 Deterministic Validation NextAction Routing.
3. S4 Narrow Stage Packet And Step Input Persistence.
4. S3 Template-Bound Managed And Deferred Validation Policy.
5. S5 Completion And Assessment Gate From Ledger.
6. S6 Root-Cause Shift Eligibility Guard.

S1 and S2 should be completed first because they directly reduce repeated LLM
validation and IO amplification. S3 must be implemented without hard-coding the
current software-development or LSP scenario into core. S6 can be developed in
parallel if the blocked same-target repair fixture is isolated.

## Code Scope Inventory

Likely core areas:

- `internal/modules/kernel/orchestrator/biz/runtime_coverage*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract*.go`
- `internal/modules/blocks/llm/biz/prompt_messages.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`

Likely template/profile areas:

- `internal/modules/application/workorchd/biz/bootstrap/defaults/templates.json`
- `internal/modules/application/workorchd/biz/bootstrap/defaults/souls.json`
- Template-bound policy binding and workspace activation code.

The exact code paths must be confirmed with source inspection before each
change. This inventory is a starting point, not permission to patch all listed
files.

## Verification Plan

Targeted tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'Evidence|Coverage|NextAction|Validation|StagePacket|Repair|Contract' -count=1
go test ./internal/modules/blocks/llm/biz -run 'Prompt|Context|StagePacket|Cache|Request' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|LSP|lsp|python|golang|kill|pkill|killall|fuser|main.go|contract_test" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "coverageWorkItems|ledgerRef|payloadRef|snapshotRef|compactSummaryRef|processEvidenceTimeline|stagePacket|requiredOutput|acceptedEvidenceRefs|rejectedEvidenceRefs" internal/modules/kernel internal/modules/blocks/llm internal/modules/application/workorchd/biz/appservice
```

Live verification after code closure:

1. Record binary path, git commit, process pid and service start time.
2. Resume or replan at least two workspaces.
3. Confirm projection remains `ready` and ViewModel revisions advance.
4. Confirm accepted evidence count increases without repeated equivalent
   validation graphs.
5. Confirm validation that is destructive or environment-dependent becomes
   managed/deferred/governed according to policy.
6. Confirm per-step input/request size stays bounded across repeated
   validation steps.
7. Confirm CPU/IO does not spike from repeated full context persistence.
8. Confirm final state is complete, governed blocked, or failed with a
   structured reason.

## Done Criteria

- Accepted validation evidence can satisfy all matching coverage items without
  another LLM call.
- Validation NextAction deterministically routes existing evidence, runtime
  action, LLM graph, managed harness, deferred evidence, decision and fail.
- Destructive or environment-dependent validation behavior is declared in
  template/profile/Soul/policy and interpreted generically by core.
- Stage Packet and step input carry current-focus data plus refs, not the full
  execution context for every step.
- Completion/assessment is driven by ledger state, not LLM self-assertion.
- Forced root-cause target shift only applies when grounded and compatible with
  current focus.
- Core remains business-neutral and passes boundary search.

## Closure Update 2026-06-27

Implemented in this closure pass:

- S1 accepted evidence reuse:
  - Runtime now reconciles accepted `EvidenceRecord` entries into the coverage
    ledger before deriving coverage evidence NextAction.
  - Matching uses structured refs only: coverage item refs, coverage work item
    ids, coverage target ids, source requirement ids and target refs.
  - Reuse writes `evidenceReuseRefs` audit edges on the coverage work item.
  - Rejected, superseded, non-matching or invalid-quality evidence is ignored.
- S2 deterministic validation route metadata:
  - Validation NextAction now carries `validationRoute` in `requiredOutput`.
  - Supported route values are:
    `validate_runtime_action`, `validate_llm_graph`,
    `validate_managed_harness`, `validate_deferred`, `request_decision` and
    `fail`.
  - Route selection is driven by run/template policy metadata such as
    `validationSafetyClass`, `allowedExecutionModes`, `managedHarnessRef`,
    `deferredEvidenceKind`, `sourceRequirementIds`, `coverageTargetIds` and
    `targetRefs`.
- S3 policy interpreter baseline:
  - Core reads generic policy fields from `validationPolicies` and
    `evidencePolicies`.
  - No template id, language, command, provider or tool-specific branch was
    added to core.
- S4 narrow baseline persistence:
  - `gapBaseline.openCoverageWorkItems` now carries current focus items rather
    than the full open ledger.
  - The full ledger remains addressable through `ledgerRef` and
    `ledgerSummaryRef`.
  - `openCoverageWorkItemCount` and `openCoverageItemsTruncated` preserve
    observability without repeating large bodies in every step input.
- S5 ledger-driven assessment continuation:
  - `NextActionForRun` now derives an `assessment` action for
    `evidence_ready` coverage items after required evidence is accepted.
  - This prevents evidence reuse from collapsing into an empty action before
    completion assessment.

Verification completed:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'Evidence|Coverage|NextAction|Validation|StagePacket|Repair|Contract' -count=1
go test ./internal/modules/blocks/llm/biz -run 'Prompt|Context|StagePacket|Cache|Request' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./...
```

Boundary result:

- Full boundary search still reports historical hits in tests, defaults and
  template/Soul content.
- The new and modified core files in this closure pass do not introduce
  business-specific template ids, programming languages, commands, providers,
  LSP rules or live workspace ids.
