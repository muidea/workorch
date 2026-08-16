# Convergence Authority Dispatch Closure Checklist

Status: active

Type: closure-checklist

Last Updated: 2026-06-26

Related:

- `../superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `../../60-verification/convergence-authority-dispatch-live-verification.md`
- `convergence-authority-simplification-design.md`
- `convergence-authority-focus-drift-closure-plan.md`
- `runtime-convergence-authority-final-fix-scope.md`
- `goal-dod-nextaction-iterative-delivery-plan.md`

## Purpose

This document records the latest live verification deviation and turns it into a
bounded closure checklist. It is intentionally separate from the architecture
documents: the architecture documents remain the design source of truth, while
this file tracks what still needs implementation closure.

The closure target is:

```text
Goal/DoD Definition
  -> Coverage/Evidence Ledger
  -> Current Focus
  -> deterministic NextAction
  -> Stage Packet
  -> LLM/runtime candidate
  -> Contract Gate
  -> accepted/rejected evidence
  -> ledger recompute
  -> projection/ViewModel
```

Current stage does not require backward compatibility. If existing code, tests,
or UI behavior conflicts with this checklist and the related architecture
documents, close toward the architecture documents.

## Contract Audit

- Scope: latest live workspaces `ws-7ced70e98ea50c43` and
  `ws-4e83810542ac4d65`, workorchd runtime convergence authority, dispatch
  input rendering, projection detail, and TUI/Web ViewModel consumption.
- Observed state:
  - Service is ready and projection is ready. Workspace list shows two
    workspaces and correct high-level states.
  - `ws-7ced70e98ea50c43` is `running`, with active run
    `run-004-replan-001-replan-001-replan-001`. It has real LLM calls,
    contract retries, and patch activity, so this is not a pure provider or TUI
    submission failure.
  - `ws-7ced70e98ea50c43` persisted run context is internally inconsistent:
    `gapBaseline.currentFocus=cwi-008`, `gapBaseline.nextAction.kind=validate`,
    but `convergenceContract.state=need_state_change`,
    `repairObjective=produce_state_changing_output`, and
    `stateChangingOutputRequired=true`. `stagePacket` is `null`.
  - `ws-4e83810542ac4d65` is `waiting_decision`, with active run
    `run-003-replan-001-replan-001-replan-001-replan-001-replan-001-r-replan-016`.
    The latest decision was opened after repeated runtime contract violations.
    The gate rejecting incomplete coverage is reasonable, but the run still
    depends on fragmented runtime fields instead of one current Stage Packet.
  - Workspace list projection has active run IDs, but detail snapshot currently
    exposes `activeRun`, `currentFocus`, and `nextAction` as empty while still
    exposing `coverageSummary`. This means detail projection is not yet a
    complete ViewModel for TUI/Web.
  - Metrics show no runtime EventHub publish failures and no projection schedule
    drops, but high metadata scan and payload hydrate counts. CPU pressure is a
    parallel symptom of large event/payload JSON processing and repeated
    context activity.
- Expected state:
  - Before every dispatch, replan, resume, provider retry, contract retry, and
    follow-up, workorchd/orchestrator must refresh one convergence authority
    snapshot.
  - `gapBaseline`, `convergenceContract`, `stagePacket`, and
    `repairActionPackage.requiredOutput` must be derived from that same
    authority snapshot.
  - LLM sees exactly one current focus, one NextAction, and one requiredOutput.
  - Projection detail exposes the same owner-derived current focus, next action,
    active run, and governance blocker consumed by TUI/Web.
- Deviation:
  - Runtime still allows derived fields to drift. The same run can say
    "validate current focus" and "must produce state-changing output" at the
    same time.
  - `stagePacket` is not guaranteed to exist for each LLM/runtime step, even
    though it is the designed stage handoff carrier.
  - Contract retry/replan paths can preserve stale failed-step or old run
    dynamic fields unless explicitly reconciled before dispatch.
  - Projection detail is incomplete and cannot be the only display source for
    TUI/Web yet.
- Owner: WorkOrch core. The primary issue is neutral runtime convergence
  authority, dispatch contract building, evidence/coverage recompute, and
  projection. It is not a workspaceTemplate-specific rule, not a software
  development special case, not a provider-only issue, and not a TUI-owned
  state machine issue.
- Event path:
  - TUI/Web/REST command -> workorchd scoped command/EventHub -> owner state ->
    runtime scheduler -> orchestrator convergence authority refresh -> Stage
    Packet and step input -> LLM/block output -> contract gate ->
    accepted/rejected evidence -> coverage ledger recompute ->
    projection/ViewModel -> TUI/Web display.
- Contract gap:
  - Missing enforced single dispatch authority gate.
  - Missing mandatory Stage Packet generation before each LLM/runtime step.
  - Missing hard reconciliation between gap baseline, convergence contract,
    repair required output, and current NextAction before provider call.
  - Missing detail projection completeness for active run/current focus/next
    action.
- Boundary check:
  - All closures must remain business neutral. Core may consume structured
    status, evidence kind, focus ID, NextAction, governance object, budget scope,
    and violation metadata.
  - Core must not branch on template ID, Soul ID, programming language, command
    name, file name, provider brand, workspace title, or live workspace ID.
- Verification:
  - Each code closure must add or update a failing sample test or projection
    test that proves the old inconsistent state is rejected or rebuilt before
    dispatch.
  - Boundary search must prove no new business-specific branch was added in
    core.
  - After targeted tests pass, run live verification against two workspaces and
    confirm positive progress, projection detail consistency, and CPU/IO trend.

## Closure Order

The checklist is ordered by blocker level. Do not start P1/P2 optimization as a
substitute for P0 authority closure.

### P0: Single Runtime Authority

#### P0-1: Single Authority Refresh Before Dispatch

Status: closed in 2026-06-26 implementation pass

Owner: WorkOrch core orchestrator.

Required closure:

- Route dispatch, replan, resume, provider retry, contract retry, and follow-up
  through one authority refresh path.
- The refresh must derive the current focus, deterministic NextAction, budget
  scope, convergence contract, gap baseline, and Stage Packet seed from the same
  coverage/evidence ledger snapshot.
- Old repair packages, old failed-step input, old replan continuation, old
  governance history, and rejected LLM output must be diagnostic lineage only.

Acceptance:

- A run cannot reach provider dispatch when
  `gapBaseline.nextAction.kind=validate` and
  `convergenceContract.state=need_state_change`.
- A run cannot dispatch with mismatched current focus across
  `convergenceAuthority`, `gapBaseline`, `coverageFocus`,
  `convergenceBudget`, and `repairActionPackage.requiredOutput`.

Closure notes:

- `ensureGapBaselineBeforeDispatch` now refreshes coverage ledger,
  convergence authority, gap baseline, and run-level derived convergence
  fields together.
- LLM dispatch specs are rebuilt from the current convergence authority before
  provider execution when an authority/gap baseline snapshot is present.
- Old repair packages and stale state-changing flags are removed from current
  validate/assessment/governance actions when they are not derived from the
  current authority.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence_budget.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_context.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_run_state.go`

#### P0-2: Dispatch Integrity Gate

Status: closed in 2026-06-26 implementation pass

Owner: WorkOrch core orchestrator.

Required closure:

- Add a provider-before-dispatch integrity gate for LLM/runtime step inputs.
- The gate validates alignment between current focus, NextAction, convergence
  contract, Stage Packet, and required output.
- If misaligned, the runtime must refresh authority and rebuild derived fields.
  If still misaligned, fail fast as an internal runtime contract error instead
  of sending contradictory instructions to LLM.

Acceptance:

- Tests cover an input with `validate` NextAction plus
  `stateChangingOutputRequired=true`; the input is corrected before dispatch or
  rejected before provider call.
- The error is categorized as runtime internal contract misalignment, not as a
  normal LLM contract violation.

Closure notes:

- `dispatchStepWithWorkspace` is now the final provider-before-dispatch guard
  for LLM workspace execution steps.
- `DispatchRequestedStep` also rebuilds event-driven commands before invoking
  the shared dispatch path, so TUI/Web/EventHub command paths cannot bypass the
  integrity gate.
- `validateConvergenceDispatchInput` rejects contradictory NextAction,
  convergence contract, state-changing flag, and Stage Packet combinations as
  internal dispatch contract conflicts.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/blocks/llm/biz/runtime_adapter.go`

#### P0-3: Mandatory Stage Packet

Status: closed for LLM dispatch paths in 2026-06-26 implementation pass

Owner: WorkOrch core orchestrator and LLM context renderer.

Required closure:

- Every LLM/runtime step must have a non-empty Stage Packet.
- Stage Packet must be the only structured stage handoff carrier for current
  action, current focus, required output, allowed outputs, forbidden actions,
  accepted/rejected evidence refs, and target snapshot references.
- LLM C3/C4 may render Stage Packet and current diagnostics, but must not rebuild
  the stage contract from older scattered fields.

Acceptance:

- Unit tests prove Stage Packet is generated for `state_change`, `validate`,
  `assessment`, governed wait, and retry paths.
- Live request logs show current focus and required output are present in Stage
  Packet and do not conflict with `gapBaseline` or `convergenceContract`.

Closure notes:

- Authority-bound dispatch input is passed through `attachStagePacketToStepInput`
  after convergence contract application.
- LLM dispatch paths without convergence authority still receive a scoped Stage
  Packet without having their explicit existing contract rewritten.
- Remaining live-log verification is tracked under P2-2.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/blocks/llm/biz/prompt_messages.go`
- `internal/modules/blocks/llm/biz/provider_context_budget.go`

#### P0-4: Retry/Replan Must Not Preserve Old Authority

Status: closed for dispatch input reconciliation in 2026-06-26 implementation pass

Owner: WorkOrch core orchestrator.

Required closure:

- Contract retry and replan may preserve old output only as rejected diagnostic
  evidence or trace refs.
- They must not preserve old `gapBaseline`, `repairObjective`,
  `stateChangingOutputRequired`, `repairActionPackage.requiredOutput`, budget
  scope, or coverage focus unless those fields are re-derived from current
  authority.
- Replan is a current-run recovery over the latest ledger, not a replay of old
  failed-step authority.

Acceptance:

- Tests that previously preserved failed-step/run gap baseline are adjusted to
  require reconciliation with latest authority before dispatch.
- A replan from `waiting_decision` can continue when the current NextAction is
  actionable, but it dispatches with a fresh Stage Packet.

Closure notes:

- `prepareGapBaselineDispatchSpecs` and the final LLM dispatch guard both rebuild
  authority-bound input from current owner state, so stale failed-step fields can
  remain only as diagnostic lineage.
- Event-driven replan/resume dispatch commands are normalized by the same guard
  before runtime execution.
- End-to-end live replan verification remains tracked under P2-2.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`

### P1: Convergence Quality

#### P1-1: Evidence And Coverage Ledger Loop

Status: closed by verification in 2026-06-26 implementation pass

Owner: WorkOrch core evidence and coverage state.

Required closure:

- Accepted state-change, validation, assessment, approval, and artifact evidence
  must bind to coverage work items by structured evidence kind and source ref.
- Rejected diagnostic evidence can guide bounded retry but cannot satisfy
  coverage, cannot become accepted result/artifact, and cannot become high-value
  memory.
- `evidence_ready -> assessment -> satisfied/open` should be runtime-gated when
  sufficient structured evidence exists.

Acceptance:

- Tests prove missing validation on current focus maps to `validate`.
- Tests prove evidence-ready items map to `assessment` instead of another state
  change.
- Tests prove rejected evidence never satisfies coverage.

Closure notes:

- Coverage state derives missing evidence strictly from accepted evidence by
  structured evidence kind and required counts.
- Failed step and failed assessment updates record rejected evidence lineage
  without satisfying coverage.
- Evidence-ready work items deterministically map to assessment, and acceptance
  requires satisfied coverage items.
- Runtime memory filters reject contract violation and diagnostic-only rejected
  output records, so rejected diagnostic evidence does not become high-value
  memory.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_coverage_state.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_completion*.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection`

#### P1-2: LLM Context Partition And Size

Status: closed by verification in 2026-06-26 implementation pass

Owner: LLM context renderer and context budget.

Required closure:

- Keep C0 stable and free of runtime dynamic fields.
- Keep C3/C4 focused on the current Stage Packet, current focus, current
  required output, recent rejected diagnostics, and critical evidence refs.
- Replace repeated full-history injection with recent complete records plus
  compacted historical summary plus ledger authority.
- Continue tracking cache plan. Compatible providers may not support cache
  read/write, but stable prefix must remain stable and measurable.

Acceptance:

- Request logs show no contradictory stage contract fields.
- Cache usage records show stable prefix remains stable when only runtime
  dynamic fields change.
- Context budget tests prove Goal/DoD, required output, current failure,
  accepted/rejected evidence refs, and target snapshot refs are protected.

Closure notes:

- Prompt context is partitioned into stable C0/C1/C2 prefix, C3 stage bundle,
  C4 volatile current-action signals, and C5 large references.
- Dynamic runtime fields such as `repairObjective`,
  `stateChangingOutputRequired`, contract violation recovery, Stage Packet, and
  snapshots are covered by volatile/stage-bundle prompt tests and do not alter
  the stable prefix hash.
- Context budget tests protect Goal/DoD, Stage Packet, current focus, required
  output, target snapshots, current progress, coverage work items, rejected
  diagnostics, and cache plan diagnostics.

Likely code areas:

- `internal/modules/blocks/llm/biz/prompt_messages.go`
- `internal/modules/blocks/llm/biz/provider_context_budget.go`
- `internal/modules/blocks/llm/biz/provider_cache_plan.go`
- `internal/modules/kernel/orchestrator/biz/runtime_context*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_memory*.go`

#### P1-3: Projection Detail Completeness

Status: closed in 2026-06-26 implementation pass

Owner: workorchd projection/ViewModel.

Required closure:

- Detail snapshot must expose active run, current focus, deterministic
  NextAction, authority diagnostics, blocking ticket/approval, and projection
  revision from owner state.
- Projection may surface stale-derived-field diagnostics, but must not repair
  owner state or become a second state machine.
- TUI/Web must display projection fields only and must not infer runtime state
  locally.

Acceptance:

- The same projection revision drives workspace list, main detail, and TUI
  prompt state.
- Snapshot for a running workspace includes active run and current next action.
- Snapshot for a governed workspace includes blocking ticket/approval and
  current command affordances.

Closure notes:

- `workspaceviewcore.Store.List()` now attaches the projection-owned
  `ViewModel` to each workspace overview, including prompt mode, revision,
  source event revision, projected source revision, and section revisions.
- `workspaceviewcore.Store.GetSnapshot()` continues to attach the same
  projection-owned `ViewModel` to detail snapshots. Running snapshots expose the
  active run through the merged run snapshot and expose current focus/NextAction
  through `GapBaseline` and `Acceptance`.
- The port/client/backend DTOs now preserve workspace overview `ViewModel` in
  remote and embedded paths.
- TUI prompt mode now prefers the server-projected `ViewModel` and only falls
  back to workspace overview state when no ViewModel is available. This keeps
  list, detail, prompt, and command suggestions aligned with the projection
  owner instead of local blocker cache inference.

Likely code areas:

- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection`
- `internal/modules/application/workorchd/service/server/http_workspace.go`
- `internal/pkg/workorchport`
- `internal/pkg/workorchclient`
- `internal/services/workorch`

### P2: Resource Envelope And Long-Run Stability

#### P2-1: Projection And Event IO Pressure

Status: closed in 2026-06-26 implementation pass

Owner: workorchd projection and event record IO.

Required closure:

- Reduce projection rebuilds triggered by high-frequency LLM stream progress.
- Avoid hydrating large payloads for list projection and normal snapshot paths.
- Use cursor/window reads for timeline and diagnostics instead of repeated full
  event scans.
- Keep watchdog scope limited to idempotent external-call health recovery; do
  not use watchdog to hide main-path event or owner-state defects.

Acceptance:

- Metrics show bounded growth for metadata scans, payload hydrates, and hydrate
  bytes during long-running two-workspace execution.
- CPU no longer stays high when there is no active provider call or projection
  rebuild backlog.
- No EventHub publish failure or projection schedule drop is introduced.

Closure notes:

- `run.llm_stream_progress` is now applied as an in-memory projection heartbeat
  update without publishing a `workspace_projection_changed` fan-out event.
  TUI already consumes the original stream progress event directly, so high
  frequency stream progress no longer forces list/detail refetches.
- Projection and normal run snapshot event queries now exclude high-frequency
  `run.llm_stream_progress` records from hydrated detail timelines. Dedicated
  observability queries still keep the complete event timeline, including stream
  progress, while LLM usage queries continue to hydrate
  `run.llm_usage_recorded` payloads because usage/cache metrics are required
  read-model data.
- Event record repositories now support a neutral `ExcludeTopics` query field
  for both file and memory backends.
- File event record listing already uses reverse bounded latest-window reads
  when `Limit` is set and no cursor is provided; this implementation pass keeps
  that path intact and avoids adding watchdog-based masking.

Likely code areas:

- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection`
- `internal/modules/application/workorchd/service/server/workspace_projection.go`
- `internal/pkg/repository`

#### P2-2: Regression And Live Verification

Status: closed in 2026-06-26 live verification pass

Owner: verification harness.

Required closure:

- Add focused failure-sample tests before implementation changes.
- Add e2e coverage for two workspaces, replan/resume, contract violation retry,
  projection detail, TUI prompt, and CPU/IO observation.
- After redeploy, verify live workspaces by recording run IDs, step IDs,
  latest evidence refs, projection revision, LLM usage/cache stats, and CPU/IO
  metrics.

Acceptance:

- Targeted tests pass before full test suite.
- Boundary search proves no business-specific logic entered core.
- Live verification proves workspaces either positively progress toward Goal/DoD
  with accepted evidence or enter a correct governed blocked state with a
  precise reason.

Closure notes:

- 2026-06-26 automated regression covered the projection/observability split:
  normal run snapshot reads exclude high-frequency stream progress, while run
  observability reads keep the complete timeline. The prior failing
  `TestReadRunObservabilityWithSessionReturnsDerivedRuntimeProgress` now passes.
- 2026-06-26 live verification harness was fixed as
  `make verify-convergence-authority-dispatch-live`. It wraps
  `scripts/observe-workorch-runtime-closure.sh` with a minimum two-workspace
  threshold, workspace-progress/governed-blocker gate, optional
  goal-convergence gate, process resource sampling, projection drop threshold,
  and runtime EventHub publish failure threshold.
- The live verification artifact contract is now documented in
  `../../60-verification/convergence-authority-dispatch-live-verification.md`.
- Live verification evidence must be recorded with the deployed process,
  workspace IDs, run IDs, projection revision, evidence refs, usage/cache stats,
  and CPU/IO trend.
- 2026-06-26 deployed verification passed with artifact directory
  `/tmp/workorch-convergence-authority-live-20260626104725`.
  - Observed workspaces:
    `ws-4e83810542ac4d65` and `ws-7ced70e98ea50c43`.
  - `workspaceProgress.pass=true` for both workspaces. Both were running with
    active heartbeats, accepted evidence already present, no open decisions, no
    pending approvals, no stalled heartbeat, and no ineffective retry marker.
  - `displayContract.pass=true`; workspace list/detail projection fields were
    displayable, and ready workspace snapshots had aligned source/projected
    revisions.
  - Runtime EventHub publish failure delta was `0`, projection schedule dropped
    delta was `0`, pending projection rebuilds were `0`, and pprof checks found
    no recursive dispatch, no runtime adapter observer-lane execution, and no
    synchronous runtime EventHub Send bridge stack.
  - Final Goal/DoD convergence was intentionally not required for this in-flight
    observation; both active runs still had open coverage gaps and remained
    running.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/*_test.go`
- `internal/modules/application/workorchd/**/*_test.go`
- `internal/services/workorch/**/*_test.go`
- `scripts/`
- `docs/60-verification/`

## Explicit Non-Goals

- Do not make core software-development specific.
- Do not loosen completion gates just to finish a live workspace.
- Do not hide contradictory runtime fields in projection.
- Do not treat LLM/provider instability as the sole cause when the runtime input
  itself is contradictory.
- Do not optimize cache or CPU/IO before the dispatch authority conflict is
  closed.

## Tracking Checklist

- [x] P0-1 single authority refresh before dispatch.
- [x] P0-2 dispatch integrity gate.
- [x] P0-3 mandatory Stage Packet.
- [x] P0-4 retry/replan does not preserve old authority.
- [x] P1-1 evidence and coverage ledger loop.
- [x] P1-2 LLM context partition and size.
- [x] P1-3 projection detail completeness.
- [x] P2-1 projection and event IO pressure.
- [x] P2-2 regression and live verification.
