# Runtime Graph Terminal Continuation Closure Plan

## 1. Background

Latest live verification exposed a core orchestrator gap in `ws-705249a8d7816330/run-073`:

- All graph batch nodes completed.
- Run stayed `running`.
- There was no active step, blocker, approval, decision, or terminal outcome.
- Coverage authority already required `state_change`, but derived execution fields still rendered an evidence-acquisition contract.

This document scopes one closure target only: harden graph terminal continuation so a graph batch cannot finish into an ambiguous running state.

Do not use this plan to change external dependency blocker behavior, template policy, Soul wording, provider retry, or live workspace deliverables.

## 2. Contract Audit

- Scope: `ws-705249a8d7816330/run-073`, last graph step `run-073/graph-001/002-build`.
- Observed state:
  - `run.state=running`.
  - `Run.CurrentStepID` in owner state is empty.
  - `/status` still projects latest completed graph step as current step.
  - Last event is `/step/state/changed` for completed graph node.
  - No blocker, approval, decision, terminal result, or follow-up dispatch exists.
  - `convergenceAuthority.nextAction.kind=state_change`.
  - `convergenceContract.state=need_evidence`.
  - `stagePacket.nextAction.type=acquire_evidence`.
  - `requiredOutput` forbids `patch/apply`.
  - `currentActionContract=null`.
- Expected state:
  - Once all graph nodes are terminal and no graph work remains, runtime must deterministically continue to exactly one next outcome.
  - The outcome must be one of: next graph node dispatch, current-action follow-up dispatch, assessment dispatch, explicit blocker, run completion, or contract drift failure/blocker.
- Deviation:
  - Graph terminal state currently allows `running + no active step + no blocker + no terminal`.
  - Derived authority fields can remain inconsistent after accepted evidence changes the ledger.
- Owner: `core orchestrator`.
- Event path:
  - runtime result -> graph step finalization -> evidence record/coverage ledger update -> convergence authority recompute -> current action contract -> dispatch/block/complete -> projection.
- Contract gap:
  - Graph terminal continuation is not a single enforced state-machine gate.
  - Restart/watchdog can classify the state as `no_action_needed`.
  - Projection can obscure owner state by presenting the latest terminal step as the current step.
- Boundary check:
  - This plan is graph/ledger/contract generic.
  - No template ID, Soul ID, language, command, file name, or business-output text may be introduced into core logic.
- Verification:
  - Add focused regression tests for graph terminal continuation, authority-derived-field consistency, restart recovery, idempotency, and projection accuracy.

## 3. Target Invariant

The following state must be impossible:

```text
run.state = running
Run.CurrentStepID = empty
no blockingTicketId
no blockingApprovalId
all graph nodes in the active graph root are terminal
no ready graph node
no active graph work
no terminal run outcome
```

If this state is detected during finalization, restart recovery, watchdog continuation, or read-model recovery, runtime must resolve it through the graph terminal continuation gate.

## 4. Single Authority Rule

Graph terminal continuation must derive the next executable contract from the latest coverage ledger and convergence authority.

Allowed derivation order:

1. Persist or reload latest accepted/rejected evidence for the just-completed graph step.
2. Recompute coverage ledger for current run definition revision.
3. Recompute `convergenceAuthority`.
4. Rebuild:
   - `convergenceContract`
   - `stagePacket`
   - `requiredOutput`
   - `currentActionContract`
5. Validate derived consistency before any dispatch.
6. Produce exactly one continuation outcome.

Old `step.Input`, previous graph contract, previous rejected output, memory, and old current action fields may be used only as evidence/context. They must not override the latest ledger-derived authority.

## 5. Continuation Outcomes

Graph terminal continuation must return exactly one of the following outcomes.

| Priority | Outcome | Condition |
| --- | --- | --- |
| 1 | Dispatch ready graph node | Same graph root has ready, undispatched nodes. |
| 2 | Dispatch current action for `state_change` | Ledger authority requires state-changing evidence. |
| 3 | Dispatch current action for `validation` | Ledger authority requires validation evidence. |
| 4 | Dispatch assessment-only follow-up | Ledger is ready for DoD/goal coverage assessment. |
| 5 | Create blocker | Missing external information, approval, capability, runtime config, or unrecoverable contract context. |
| 6 | Complete run | Coverage and accepted assessment satisfy completion gate. |
| 7 | Contract drift failure/blocker | Authority fields cannot be made consistent. |

Returning `nil` without changing run state or dispatching a continuation is not allowed once graph terminal state is detected.

## 6. Implementation Checklist

### P0. Terminal State Detector

- Add a single helper in graph continuation code to detect graph terminal state.
- It must consider:
  - graph root identity
  - graph node IDs
  - step states
  - active dispatched/running graph work
  - ready next graph specs
  - run terminal/blocking state
- It must be independent of template, language, command, or output text.

### P0. Unified Continuation Entry

Route all graph terminal paths through one continuation function:

- runtime result finalization graph batch branch
- `continueGraphBatchAfterCompletedStep`
- `resumeCompletedGraphBatchStepContinuation`
- workspace restart continuation
- watchdog continuation
- graph dispatch conflict recovery

Each caller may pass different context, but the final state-machine decision must be centralized.

### P0. Ledger-First Authority Rebuild

At graph terminal:

- reload hydrated run and steps if needed
- ensure latest step evidence is recorded before deriving next action
- persist coverage updates before dispatch
- rebuild derived fields from latest ledger
- reject or repair stale derived fields before dispatch

Required consistency examples:

- `authority=state_change` implies `convergenceContract=need_state_change`.
- `authority=state_change` implies `stagePacket.nextAction.type=state_change` or equivalent state-changing action.
- `authority=state_change` implies `requiredOutput.acceptedKinds` includes state-changing kind and does not forbid all valid state-changing kinds.
- `currentActionContract.nextAction.kind` must match the authority.

### P0. No Ambiguous Running Return

After graph terminal handling, assert one of:

- a new step was dispatched
- run became blocked
- run became terminal
- an explicit contract drift diagnostic/blocker was recorded

If none is true, return an error and record diagnostic evidence rather than silently leaving the run running.

### P1. Restart And Watchdog Recovery

When restart/watchdog sees a run with:

- `running`
- no active current step
- no blocker
- terminal latest graph step
- no active graph work

it must call graph terminal continuation. It must not report `no_action_needed`.

### P1. Projection Alignment

Run status and task progress must distinguish:

- owner current step: `Run.CurrentStepID`
- latest terminal step: latest completed/failed step
- continuation gap: running with no current step and no blocker

Do not project latest terminal step as active current step. If a continuation gap remains, projection should expose a diagnostic state such as `awaiting_continuation` or `contract_drift`, not a misleading completed current step.

### P2. Observability

Add concise diagnostics for graph terminal decisions:

- graph root ID
- completed node IDs
- ready node IDs
- selected continuation outcome
- authority kind before/after rebuild
- derived consistency result

Detailed payloads should remain available in observability artifacts, but action logs should stay compact unless an error occurs.

## 7. Test Plan

### Required Unit/Integration Tests

1. All graph nodes completed without DoD assessment does not leave run running without blocker.
2. Graph terminal with latest ledger requiring `state_change` dispatches a state-changing current action contract.
3. Authority `state_change` with stale `need_evidence/acquire_evidence` fields is rebuilt before dispatch.
4. Restart recovery resumes graph terminal continuation instead of returning `no_action_needed`.
5. Watchdog recovery resumes graph terminal continuation idempotently.
6. Re-running graph terminal continuation does not duplicate an already-dispatched follow-up.
7. Projection/status does not present latest completed step as active current step when owner `Run.CurrentStepID` is empty.

### Suggested Test Commands

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'Graph|Authority|Convergence|Continuation|Restart|Watchdog' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/... -run 'RunStatus|TaskProgress|Projection|Continuation' -count=1
make fmt-check
git diff --check
```

## 8. Acceptance Criteria

The closure is complete only when all are true:

- No graph terminal path can return without dispatch/block/complete/diagnostic.
- `state_change` authority cannot coexist with `need_evidence/acquire_evidence` derived execution fields at dispatch time.
- Restart/watchdog can recover a running run whose active graph has already finished.
- Status/projection no longer mislabels latest terminal step as active current step.
- Focused regression tests pass.
- No core code introduces template, language, command, file-name, or business-string special cases.

## 9. Out Of Scope

The following must be handled separately:

- External dependency blocker behavior.
- Provider retry policy.
- CPU/IO read-model compaction.
- Template/Soul validation strategy.
- Live workspace deliverable edits.
- `docs-product/` updates.
