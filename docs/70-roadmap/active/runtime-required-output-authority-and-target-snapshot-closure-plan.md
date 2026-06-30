# Runtime RequiredOutput Authority And Target Snapshot Closure Plan

Status: code-closed; pending live verification

Type: closure-plan

Last Updated: 2026-06-30

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-target-snapshot-authority-and-projection-closure-plan.md`
- `runtime-current-action-projection-convergence-closure-plan.md`

## Purpose

This plan closes the remaining implementation gaps between the Goal/DoD
NextAction state machine design and the convergence authority simplification
design.

The target is not to add another local compatibility patch. The target is to
make current requiredOutput and target snapshot acquisition obey a single,
business-neutral authority path:

```text
coverage/evidence ledger
  -> deterministic NextAction
  -> convergenceContract
  -> requiredOutput renderer
  -> repairActionPackage.requiredOutput
  -> stagePacket.requiredOutput
  -> currentActionContract.requiredOutput
  -> top-level requiredOutput mirror or diagnostic lineage
  -> dispatch gate
```

## Contract Audit

- Scope:
  - Runtime convergence dispatch.
  - Current action contract rendering.
  - Graph follow-up and contract retry input rebuilding.
  - Target snapshot evidence acquisition.
- Observed state:
  - Live dispatch can select `NextAction=acquire_target_snapshot` and correctly
    render read-only `repairActionPackage.requiredOutput` and
    `stagePacket.requiredOutput`.
  - The same follow-up input can still carry old top-level `requiredOutput` and
    old `currentActionContract.requiredOutput` with state-changing
    `patch/apply` requirements.
  - The dispatch gate then fails with
    `runtime_internal_contract_misalignment: requiredOutput state-changing mismatch`.
  - Another live path reaches `waiting_decision` because LLM returned a
    deliverable-writing shell node under an `acquire_target_snapshot` read-only
    contract.
- Expected state:
  - `currentActionContract.requiredOutput`, `repairActionPackage.requiredOutput`,
    `stagePacket.requiredOutput` and top-level `requiredOutput` are all rendered
    from the same current authority snapshot before dispatch.
  - Old top-level `requiredOutput`, old `currentActionContract`, old
    failed-step input, old follow-up input and retry input are diagnostic
    lineage only.
  - `acquire_target_snapshot` is a deterministic read-only evidence acquisition
    action whenever neutral runtime capabilities can obtain the required
    snapshot.
- Deviation:
  - Current code still lets some old requiredOutput fields compete with the
    authority-rendered action contract.
  - Target snapshot acquisition still relies too much on LLM graph generation.
- Owner: WorkOrch core runtime. This is a neutral convergence, contract and
  evidence acquisition issue. It is not owned by a workspaceTemplate, Soul,
  programming language, provider, file name or live workspace ID.
- Event path:
  - scheduler/event dispatch -> authority refresh -> convergence contract ->
    requiredOutput renderer -> stage packet -> current action contract ->
    dispatch gate -> block/LLM -> contract gate -> evidence ledger.
- Contract gap:
  - Missing single requiredOutput renderer.
  - Missing hard rule that top-level `requiredOutput` is not an authority input.
  - Missing runtime-managed target snapshot acquisition path.
- Boundary check:
  - All changes must use generic action kinds, evidence kinds, capability kinds,
    source refs, target refs, coverage focus and diagnostics.
  - No template ID, Soul ID, language, command, provider, file name, workspace
    title or live workspace ID branch is allowed.

## RequiredOutput Authority Rules

### Authority Sources

Allowed current authority sources:

- current convergence authority snapshot
- current deterministic NextAction
- current convergenceContract
- current coverage focus
- current accepted/rejected evidence refs
- current policy/capability refs

Diagnostic-only sources:

- old top-level `requiredOutput`
- old `currentActionContract`
- old `repairActionPackage`
- old stagePacket
- old failed-step input
- old follow-up input
- old contract retry input
- old replan continuation
- rejected LLM output
- previous decision text
- memory recall

### Renderer Contract

Introduce or consolidate a single renderer that produces one requiredOutput map
from the current authority snapshot and action state. That renderer must be the
only source for:

- `repairActionPackage.requiredOutput`
- `stagePacket.requiredOutput`
- `currentActionContract.requiredOutput`
- top-level `requiredOutput`, if retained

The renderer may receive diagnostic lineage for retry hints, but it must not use
lineage to select the current contract shape.

### Top-Level RequiredOutput

Top-level `requiredOutput` has only two valid states:

1. Current mirror: byte-stable equivalent to the current renderer output after
   normalization.
2. Diagnostic lineage: explicitly moved under a diagnostic field and not read by
   current action contract, stage packet, convergence contract or dispatch gate.

It must not be the first source selected by current action contract or dispatch
semantics.

## Target Snapshot Acquisition Rules

`acquire_target_snapshot` is a read-only evidence acquisition action. Runtime
should avoid asking LLM to invent a graph when neutral runtime capabilities can
deterministically obtain the snapshot.

Expected behavior:

- Build a bounded target snapshot request from current coverage focus and
  structured target refs.
- Use read-only file/git/tool capabilities to collect current manifest and
  target snapshot evidence.
- Record accepted evidence with sourceRef, targetRef, coverageWorkItemIds,
  structuredTargetRefs, observedClaims and completeness.
- For partial or missing targets, record partial evidence and let NextAction
  recompute the next deterministic action.
- If read-only acquisition lacks external information, permission, credential or
  approval, request governance with a structured blocker.

LLM may still be used for candidate target interpretation only when runtime
cannot identify candidate target refs from structured context. Even then, the
LLM contract remains read-only and any state-changing output is rejected
diagnostic evidence.

## Code Scope

### P0 RequiredOutput Single Renderer

Files to inspect and adjust:

- `internal/modules/kernel/orchestrator/biz/runtime_current_action_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_event.go`

Acceptance:

- `currentActionContract.requiredOutput` never prefers old top-level
  `requiredOutput`.
- `firstRequiredOutputForSemantics` or equivalent logic uses the current
  renderer output, not arbitrary map order or old top-level fields.
- `ApplyConvergenceContract` and dispatch rebuild always leave the four
  requiredOutput mirrors aligned or explicitly diagnostic-only.

Implementation status: closed in code.

- `activeRequiredOutputForCurrentAction` no longer prefers old top-level
  `requiredOutput` before authority mirrors.
- Dispatch rebuild synchronizes `repairActionPackage.requiredOutput`,
  `stagePacket.requiredOutput`, `currentActionContract.requiredOutput` and
  top-level `requiredOutput` from the current authority contract.
- Stale mirrors are recorded as authority diagnostics before replacement, so
  lineage remains auditable without becoming current authority.

### P0 Follow-Up And Retry Input Cleanup

Files to inspect and adjust:

- `internal/modules/kernel/orchestrator/biz/runtime_graph*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation*.go`

Acceptance:

- graph follow-up input cannot inherit old state-changing requiredOutput when
  current NextAction is read-only acquisition, validation or assessment.
- contract retry input preserves rejected diagnostics but re-renders current
  requiredOutput from authority.
- replan continuation cannot restore old requiredOutput as current contract.

Implementation status: closed for the current-authority rebuild path; pending
live verification across replan and contract retry workspaces.

- Rebuilt dispatch input now normalizes requiredOutput mirrors before current
  action contract attachment.
- RequiredOutput semantics selection prefers current action-package and stage
  mirrors before the top-level diagnostic mirror.

### P0 Target Snapshot Runtime-Managed Acquisition

Files to inspect and adjust:

- `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- existing target snapshot or evidence recording helpers

Acceptance:

- `acquire_target_snapshot` can complete through deterministic read-only
  runtime/block capability when target refs are available.
- accepted target snapshot evidence is recorded with coverage binding and
  completeness metadata.
- LLM graph drift is only used as rejected diagnostic evidence and does not
  directly open decision unless a structured external blocker exists.

Implementation status: closed in code.

- Runtime-managed acquisition is dispatched before the LLM/runtime adapter when
  the current action is `acquire_target_snapshot` and the requiredOutput is
  read-only.
- The runtime synthesizes bounded `file/read` graph nodes only from readable
  structured target refs and explicit target sets.
- The path rejects abstract refs, absolute paths, URL refs, parent traversal and
  NUL-containing paths.
- Regression coverage verifies that root LLM execution is skipped, the read-only
  graph node is dispatched, and accepted target snapshot evidence is recorded.

### P1 Dispatch Gate And Diagnostics

Acceptance:

- dispatch gate first rebuilds from authority, then validates.
- runtime internal contract misalignment is only emitted after rebuild fails.
- diagnostics identify which old field was downgraded or rejected.

Implementation status: closed in code for stale requiredOutput mirror rebound
diagnostics; pending live verification for projection visibility.

### P1 Projection And ViewModel

Acceptance:

- projection exposes authority diagnostics when stale persisted mirrors are
  present.
- projection does not repair owner state.
- TUI/Web remain projection consumers only.

Implementation status: not changed by this closure except for diagnostics
produced by owner state. Projection-only exposure remains a follow-up if live
verification needs a dedicated UI diagnostic surface.

## Test Plan

Targeted unit tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'CurrentAction|RequiredOutput|TargetSnapshot|Convergence|StagePacket|Dispatch|FollowUp|ContractViolation' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore ./internal/modules/application/workorchd/service/server -run 'ViewModel|Projection|Prompt|WorkspaceGetEndpoint' -count=1
```

Required fixtures:

- stale top-level `requiredOutput` is state-changing, current
  `repairActionPackage.requiredOutput` is read-only acquisition, expected
  current action contract is read-only acquisition.
- stale follow-up input carries old state-changing requiredOutput, current
  NextAction is `acquire_target_snapshot`, expected dispatch input is read-only
  aligned.
- target snapshot acquisition with existing structured target refs produces
  accepted snapshot evidence without LLM graph.
- LLM returns state-changing graph under read-only acquisition, expected rejected
  diagnostic evidence plus retry or runtime-managed acquisition, not accepted
  output.

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|city|ssh|provider|ws-" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "requiredOutput|currentActionContract|repairActionPackage|stagePacket|acquire_target_snapshot|target_snapshot" internal/modules/kernel/orchestrator/biz -g "*.go"
```

Live verification:

1. Start workorchd and record commit, pid and start time.
2. Trigger two workspaces through resume/replan.
3. Verify every LLM dispatch input has aligned currentActionContract,
   repairActionPackage, stagePacket and top-level requiredOutput.
4. Verify `acquire_target_snapshot` produces accepted target snapshot evidence
   or a structured blocker.
5. Verify no workspace enters waiting decision solely because runtime could have
   performed read-only snapshot acquisition.

## Done Criteria

- RequiredOutput has one current renderer and no stale source can preempt it.
- Top-level `requiredOutput` is a mirror or diagnostic-only field.
- `currentActionContract.requiredOutput` is always current-authority rendered.
- Target snapshot acquisition is deterministic when neutral read-only
  capabilities are sufficient.
- Rejected LLM graph drift remains diagnostic-only.
- Core remains business-neutral.
