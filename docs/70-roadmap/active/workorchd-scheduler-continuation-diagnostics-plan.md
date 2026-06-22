# Workorchd Scheduler Continuation Diagnostics Plan

## Purpose

This document defines the remaining follow-up scope after the EventHub /
watchdog closure. It is the design source for code changes that improve
workorchd scheduler continuation observability without turning watchdog into a
scheduler fallback.

The core decision is:

- Scheduler pending work remains process-local and is not automatically
  recovered after workorch restart.
- Replan continuation does not enter periodic watchdog as an automatic
  compensation path.
- EventHub command/result loss is a main-path defect and must surface as an
  explicit error or diagnostic, not as a hidden watchdog recovery.
- TUI prompt state is rendered from the current active workspace server
  projection/view model. Local TUI caches are not authoritative for blocker,
  progress, run activity, or scheduler continuation state.
- If a continuation cannot be scheduled or cannot run after being accepted, the
  server must expose a structured reason and an explicit manual next action.

## Confirmed Boundaries

1. Scheduler command `accepted` means the current process has validated the
   command and successfully submitted the work item to the managed
   `BackgroundRoutine`.
2. Scheduler command `accepted` does not mean the command has been durably
   persisted across process restart.
3. After workorch restart, process-local scheduler pending maps are empty by
   design. The server must not replay historical process-local pending work.
4. If users still want execution, they must explicitly submit a new command:
   run continue, run start/resume, workspace activation/restart, decision
   resolve, approval resolve, or a future manual diagnostic recovery command.
5. Periodic watchdog may diagnose continuation anomalies, but must not create
   replan continuation from business state.
6. Periodic watchdog may recover only runtime execution gaps already allowed by
   `RuntimeRecoveryPolicy` and durable runtime evidence. That behavior remains
   separate from scheduler continuation diagnostics.
7. TUI/Web/CLI may display diagnostics and recommended manual commands. They
   must not locally replay resume/replan/restart.
8. TUI prompt and blocker/progress labels are display projections of the
   current active workspace returned by workorchd. Local UI feedback may show
   command submission pending/error, but it must not override server-owned
   workspace/run state.

## Non-Goals

- Do not add a durable scheduler queue in this scope.
- Do not persist scheduler pending maps for automatic replay.
- Do not auto-activate a workspace after service restart.
- Do not auto-resume governance or replan continuation from periodic watchdog.
- Do not use watchdog to compensate missing EventHub subscribers, missing
  `events.Response`, owner command failure, projection failure, or scheduler
  enqueue failure.
- Do not infer recovery behavior from workspace title, template id, goal text,
  file names, language stack, command text, or domain-specific content.
- Do not let TUI local `blockingDecision`, `blockingApproval`, `progress`,
  `progressText`, `ResolvedTickets`, or `ResolvedApprovals` mask a server
  projection that still reports an open blocker or a non-live workspace state.

## Current Accepted Semantics

The current scheduler command path is intentionally short:

```text
REST/TUI/CLI command
  -> EventHub scheduler command Send/Publish
    -> workorchd scheduler handler validates payload and scope
      -> scheduleBackground / BackgroundRoutine.AsyncFunction
        -> accepted response only after enqueue success
```

Immediate failures must remain synchronous command failures:

| Failure | Required response |
| --- | --- |
| Missing subscriber | non-accepted business error |
| Missing `events.Response` | non-accepted business error |
| Unexpected response topic/source/payload | non-accepted business error |
| Invalid payload/scope | non-accepted business error |
| BackgroundRoutine enqueue failure | non-accepted business error |

After accepted, the long-running body executes asynchronously. Failures in that
body must be projected as continuation diagnostics rather than being silently
logged only.

## Restart Semantics

Workorch restart must not recover process-local pending scheduler work.

Required display behavior:

- Do not show old command submission as still pending after restart.
- Do not claim a workspace/run is being activated solely because a previous
  process accepted a command.
- If formal owner state indicates a run/workspace still needs user action, show
  that state as server-owned projection, not as local TUI state.
- If the server can determine that execution is not active after restart, expose
  a diagnostic such as:

```json
{
  "state": "not_scheduled",
  "reason": "service_restarted_pending_scheduler_work_not_recovered",
  "recoverability": "manual_retry_required",
  "recommendedActions": ["run_continue", "workspace_restart"]
}
```

This diagnostic is informational. It must not cause automatic activation,
resume, restart, or replan.

Implementation note: because process-local pending work is intentionally not
durable, the server cannot reconstruct the exact pre-restart pending command.
Diagnostics should describe the current observable condition: no active
process-local scheduler work and a user-triggered command is required to
continue.

## Continuation Diagnostics Model

Add a server-side diagnostic projection, not a formal kernel owner state.

Recommended neutral shape:

```json
{
  "workspaceId": "ws-...",
  "runId": "run-...",
  "kind": "workspace_activation | workspace_restart | run_continuation | governance_continuation | replan_continuation",
  "state": "not_scheduled | accepted | running | failed | completed | no_action_needed",
  "reason": "scheduler_enqueue_failed | background_execution_failed | owner_command_failed | run_slot_unavailable | service_shutting_down | workspace_not_eligible | run_not_eligible | replan_successor_exists | service_restarted_pending_scheduler_work_not_recovered",
  "detail": "human-readable bounded detail",
  "recoverability": "manual_retry_required | no_action_needed | inspect_required",
  "recommendedActions": ["run_continue", "workspace_restart"],
  "dedupeKey": "workspace/run/action",
  "attempt": 1,
  "occurredAt": "RFC3339Nano",
  "source": "workorchd/scheduler"
}
```

Rules:

- `kind` is scheduler continuation kind, not business-domain meaning.
- `state` describes scheduler/diagnostic state only; it is not run/workspace
  formal state.
- `reason` must be machine-readable and stable.
- `detail` must be bounded and safe for projection.
- `recommendedActions` must be explicit user commands, never implicit
  automatic behavior.
- `dedupeKey` must include workspace scope when available.

## Required Reason Codes

Initial required reason codes:

| Reason | Meaning | Recoverability |
| --- | --- | --- |
| `scheduler_enqueue_failed` | Command handler could not submit work item to `BackgroundRoutine` | `manual_retry_required` after service health is restored |
| `background_execution_failed` | Work item started but returned an execution error not represented by owner state | `inspect_required` or `manual_retry_required` |
| `owner_command_failed` | Kernel/appservice owner command returned an error | `inspect_required` |
| `run_slot_unavailable` | Continuation could not acquire run slot | `manual_retry_required` or retry by explicit command |
| `service_shutting_down` | Background context was canceled or scheduler is closing | `manual_retry_required` after restart |
| `workspace_not_eligible` | Workspace state no longer needs the requested continuation | `no_action_needed` |
| `run_not_eligible` | Run state no longer allows the requested continuation | `no_action_needed` |
| `replan_successor_exists` | Replan successor already exists | `no_action_needed` |
| `service_restarted_pending_scheduler_work_not_recovered` | Previous process-local pending work is not recoverable by design | `manual_retry_required` |

Reason names are intentionally generic. Do not add software-development,
template, language, test-framework, or command-text-specific reason codes in
workorchd core.

## Replan Continuation Rule

Replan continuation remains a main-path scheduler command:

```text
decision/approval resolved
  -> publish governance/replan scheduler command
    -> accepted only after work item enqueue succeeds
      -> background work checks successor/run/workspace eligibility
        -> create or continue successor through owner command
        -> publish projection/diagnostic result
```

Forbidden behavior:

- Periodic watchdog must not scan resolved decisions and create replan
  continuation automatically.
- Periodic watchdog must not infer missing replan from workspace state and
  create a successor run.
- TUI/Web must not replay replan because the UI still sees a blocked or waiting
  projection.

If replan cannot proceed, the server must expose a continuation diagnostic:

```json
{
  "kind": "replan_continuation",
  "state": "failed",
  "reason": "owner_command_failed",
  "recoverability": "manual_retry_required",
  "recommendedActions": ["run_continue", "workspace_restart"]
}
```

If replan no longer needs work because a successor exists:

```json
{
  "kind": "replan_continuation",
  "state": "no_action_needed",
  "reason": "replan_successor_exists",
  "recoverability": "no_action_needed",
  "recommendedActions": []
}
```

## Event Loss Policy

Event loss must be treated as a defect.

The following must not be compensated by watchdog:

- scheduler command missing subscriber
- scheduler command missing accepted response
- scheduler accepted response with wrong topic/source/payload
- owner command missing response
- runtime lifecycle event publish failure
- projection changed event publish failure
- EventHub queue timeout or lane misconfiguration

Expected behavior:

- synchronous command path returns an error when loss is detected before
  accepted;
- asynchronous work path records a bounded continuation diagnostic when failure
  happens after accepted;
- metrics and diagnostics make the failure visible;
- tests prove no handler silently swallows these failures.

## Projection And Client Display

workorchd remains the server projection owner. TUI/Web display only consumes
server state and diagnostics.

Client rules:

- Command submission pending/error is local UI feedback only.
- Local UI feedback must not overwrite workspace/run business state.
- After workspace switch, old command feedback and old workspace diagnostics
  must not pollute the new active workspace view.
- TUI `/status` may display latest continuation/watchdog diagnostics.
- TUI `/status` must not trigger resume, replan, restart, or activation.

## TUI Prompt Projection Authority

The TUI prompt must have a single authoritative source: the current active
workspace projection returned by workorchd, optionally enriched with matching
decision/approval detail from the same active workspace snapshot.

Current risk areas to close:

- Prompt rendering currently checks local `activeBlockingDecision()` /
  `activeBlockingApproval()` before checking the workspace overview. A stale
  local blocker can therefore make the prompt disagree with
  `WorkspaceInfo.State`, `BlockingTicketID`, or `BlockingApprovalID`.
- Workspace display cache currently stores and restores authoritative-looking
  fields: `Progress`, `ProgressText`, `BlockingDecision`, `BlockingApproval`,
  `Decisions`, `Approvals`, `Running`, and `LastError`. After a workspace
  switch, this can restore old state before the backend snapshot arrives.
- Workspace overview normalization currently uses local `ResolvedTickets` /
  `ResolvedApprovals` to optimistically clear server `BlockingTicketID` /
  `BlockingApprovalID`. If the server still owns an open blocker, this hides
  the real waiting state.
- Decision/approval resolved markers currently risk being written to the
  active workspace instead of the workspace carried by the async message.
- The deprecated decision endpoint
  `/api/v1/workorch/workspaces/{workspaceId}/decisions` must not be used.
  Clients must use `/api/v1/workorch/decisions?workspaceId=...` or decisions
  embedded in the workspace snapshot.

Required prompt rules:

1. Derive prompt suffix only from the active workspace server projection:
   `WorkspaceInfo.State`, `BlockingTicketID`, `BlockingApprovalID`,
   `ActiveRunID`, and the active run/snapshot view model returned by workorchd.
2. Use local decision/approval detail only as metadata for the blocker that the
   server projection already identifies. Local detail may improve label text;
   it must not create or clear a blocker by itself.
3. Do not use local `progress` or `progressText` as prompt authority. They may
   remain status-line feedback for command submission, refresh retries, or
   transient UI messages.
4. If the active workspace server projection is missing or still refreshing,
   render a neutral prompt such as workspace id plus `refreshing`; do not
   restore an old workspace's blocker/running/submitting suffix.
5. After restart, the prompt must show the server projection as inactive or
   waiting for manual retry. It must not show a previous process' accepted
   scheduler command as still active.

Required cache rules:

| Local value | Allowed cache behavior |
| --- | --- |
| Scroll position, cursor, input draft, viewport state | May be cached per workspace |
| Transcript, result text, artifacts, patches, memory display | May be cached as display-only content until fresh snapshot arrives |
| `Progress`, `ProgressText`, `Running`, active run control | Must not be restored as authoritative state |
| `BlockingDecision`, `BlockingApproval`, `Decisions`, `Approvals` | Must not be restored as authoritative blocker state; may be replaced only by current snapshot/detail response for the same workspace |
| `ResolvedTickets`, `ResolvedApprovals` | May record submission feedback, but must not clear server blocker ids |
| Scheduler diagnostics | Must be scoped to workspace/run and replaced by server projection |

Required workspace switch flow:

```text
save display-only cache for previous workspace
  -> clear authoritative blocker/progress/run-control fields
  -> select new active workspace id
  -> render neutral refreshing state
  -> fetch workorchd workspace snapshot/status
  -> apply response only when response workspaceId still matches active workspace
  -> derive prompt from applied server projection
```

Async event and response scoping:

- Every async response that carries `workspaceId` must update that workspace's
  local entry, not whichever workspace is currently active.
- If a response has only `runId`, the TUI may map it to a workspace only when a
  current server projection already associates that run with exactly one
  workspace; otherwise it must not mutate authoritative local blocker state.
- Decision/approval resolved handlers must accept an explicit workspace id or
  use the event payload workspace id. They must not call a helper that writes to
  `activeWorkspaceID` implicitly.
- Stale responses for inactive workspaces may refresh that workspace's cached
  display data, but must not update prompt, status line authority, or active
  blocker state.

## Metrics And Diagnostics

Add metrics for observability only:

```text
workorch_scheduler_commands_total{kind,result}
workorch_scheduler_command_enqueue_failures_total{kind,reason}
workorch_scheduler_continuation_failures_total{kind,reason}
workorch_scheduler_continuation_no_action_total{kind,reason}
workorch_watchdog_ticks_total
workorch_watchdog_last_error_timestamp_seconds
workorch_watchdog_stale_runtime_gap_recoveries_total{gap,safety}
```

Readiness or diagnostics output should include:

- latest watchdog tick time;
- latest watchdog duration;
- latest watchdog error summary;
- latest continuation diagnostic summary;
- count of recent continuation failures by kind/reason.

These fields must not affect readiness success unless an existing readiness
contract explicitly chooses to fail on degraded diagnostics.

## Code Closure Scope

### P0

- Add continuation diagnostic data model and bounded projection.
- Record diagnostics for accepted background work failures.
- Ensure restart display does not retain old process-local pending/submitting
  state.
- Ensure replan continuation failure/no-action reasons are projected.
- Ensure periodic watchdog does not auto-create replan continuation.
- Make TUI prompt derive blocker/running/submitting state only from the active
  workspace server projection/view model.
- Stop restoring authoritative blocker/progress/run-control fields from
  workspace display cache after workspace switch.
- Stop optimistic local clearing of server `BlockingTicketID` /
  `BlockingApprovalID` based on `ResolvedTickets` / `ResolvedApprovals`.
- Scope decision/approval resolved markers by message workspace id instead of
  implicit `activeWorkspaceID`.
- Remove any remaining usage of
  `/api/v1/workorch/workspaces/{workspaceId}/decisions`.

### P1

- Add metrics for scheduler command enqueue and continuation failures.
- Add diagnostics endpoint or extend existing ready/status diagnostics with
  latest watchdog and continuation summaries.
- Add TUI `/status` rendering for server-provided diagnostics.
- Add TUI tests for prompt authority, workspace switch cache behavior, resolved
  marker workspace scoping, and stale inactive workspace responses.

### P2

- Extend `observe-runtime-closure` to validate resolved governance continuation
  main path: accepted or explicit diagnostic, never hidden watchdog recovery.
- Extend real-runtime preflight to sample watchdog interval, run slot
  capacity, and continuation diagnostic metrics.
- Update architecture/checklist docs after implementation.

## Verification Plan

Required tests:

- Scheduler enqueue failure still returns non-accepted REST/TUI error.
- Accepted background execution failure records a continuation diagnostic.
- Service restart clears local command-submission state and does not auto-run
  scheduler pending work.
- Resolved replan decision with missing successor does not get auto-recovered by
  periodic watchdog.
- Replan no-action path records `replan_successor_exists`.
- Owner command failure records `owner_command_failed`.
- TUI `/status` displays diagnostics without triggering commands.
- Inactive workspace diagnostic events do not pollute active workspace view.
- TUI prompt uses active workspace server projection, not stale local
  `blockingDecision`, `blockingApproval`, `progress`, or `progressText`.
- TUI workspace switch does not restore authoritative blocker/progress/run
  control from display cache before backend snapshot refresh.
- TUI local resolved ticket/approval feedback does not hide a server projection
  that still reports `BlockingTicketID` or `BlockingApprovalID`.
- Decision/approval resolved async handlers update the workspace carried by the
  message, not the workspace active at handler time.
- No code path calls `/api/v1/workorch/workspaces/{workspaceId}/decisions`.

## Implementation Notes

### 2026-06-22 Closure Pass

Closed or re-verified items:

- TUI prompt authority now uses the active workspace server projection for
  command eligibility as well as rendering. `ensureActiveBlockingDecisionFromOverview`
  and `ensureActiveBlockingApprovalFromOverview` no longer fall back to local
  `blockingDecision` / `blockingApproval` when the active workspace projection
  is live, terminal, missing, or otherwise not reporting the matching blocker.
- Added TUI regressions proving stale local decision/approval blockers cannot
  submit `/decision replan` or `/approval approve` when the current server
  projection reports the workspace as running.
- Startup inactive running run recovery now records a bounded
  `not_scheduled` continuation diagnostic with reason
  `service_restarted_pending_scheduler_work_not_recovered` and manual
  `run_resume` / `workspace_restart` actions. This exposes restart semantics
  without replaying process-local scheduler pending work.
- Scheduler queue submission failure now has regression coverage proving that
  the command returns a non-accepted error and records a workspace/run scoped
  `scheduler_enqueue_failed` continuation diagnostic with manual retry
  recoverability.
- TUI decision/approval resolved markers now update only the matching
  workspace/run scoped local item. Duplicate ticket or approval ids in another
  workspace remain untouched, so local submission feedback cannot move or clear
  another workspace's authoritative blocker display.
- TUI `/status` now has regression coverage proving scheduler continuation
  diagnostics and watchdog registration are rendered from the current
  ViewModel without triggering resume, replan, approval, restart, or any other
  backend command.
- TUI workspace context application now treats workspace snapshot diagnostics
  as the authoritative full projection and treats run status diagnostics as a
  partial override only when present. A run status payload that omits
  scheduler diagnostics no longer clears the active workspace snapshot
  diagnostics, and inactive workspace diagnostics cannot pollute the active
  workspace status view.
- Added a direct server regression proving that executing the registered
  periodic watchdog timer does not scan workspace governance state and does
  not activate replan continuations. Replan continuation recovery remains an
  explicit/manual recovery path, while the watchdog timer is scoped to stale
  dispatched runtime gap recovery.
- Re-verified that the periodic watchdog timer calls only stale dispatched run
  recovery. It does not call replan continuation recovery.
- Re-verified that source code does not call the deprecated
  `/api/v1/workorch/workspaces/{workspaceId}/decisions` endpoint; remaining
  matches are this document's requirement text.
- `verify-boundary-contract` now enforces the deprecated workspace-scoped
  decision endpoint ban across `internal`, `web`, and `scripts`, excluding
  tests, built console assets, node modules, and docs. This turns the previous
  manual `rg` evidence into a reusable regression gate.

Validation evidence:

```bash
go test ./internal/services/workorch/service/tui -count=1 -timeout=180s
go test ./internal/services/workorch/service/tui -run 'TestModel(ResolvedMarkersAreScopedAcrossDuplicateIDs|DecisionReplanUsesServerProjectionEvenAfterOptimisticResolvedMark|DecisionResolvedDoesNotHideServerOpenProjection|GovernanceResolvedEventRefreshesWorkspaceState|LiveRunStateEventClearsStaleDecisionPrompt|LiveStateEventClearsStaleDecisionPrompt)' -count=1 -timeout=180s
go test ./internal/services/workorch/service/tui -run 'TestModel(StatusRendersSchedulerDiagnosticsWithoutTriggeringCommands|InspectCommandsRenderCurrentState)|TestSchedulerDiagnosticsStatusShowsWatchdogTimerRegistration' -count=1 -timeout=180s
go test ./internal/services/workorch/service/tui -run 'TestModelWorkspaceSwitchThenProjectionRefreshUpdatesMainViewAndStatusBar|TestModelStatusRendersSchedulerDiagnosticsWithoutTriggeringCommands' -count=1 -timeout=180s
go test ./internal/modules/application/workorchd/service/server -count=1 -timeout=180s
go test ./internal/modules/application/workorchd/service/server -run 'TestWorkorchDSchedulerCommandReturnsErrorWhenRunContinuationQueueFails|TestWorkorchDRESTCommandsReturnConflictWhenSchedulerQueueFails|TestSchedulerRecommendedActionsUseResumeTerminology' -count=1 -timeout=180s
go test ./internal/modules/application/workorchd/service/server -run 'TestServerStateWatchdogDoesNotRecoverReplanContinuations|TestServerStateRecoverReplanContinuationsSchedulesWaitingDecisionActiveSupersededRun|TestHTTPServerStartupOnlySchedulesWatchdogTimer' -count=1 -timeout=180s
make verify-boundary-contract
rg -n "/api/v1/workorch/workspaces/\\{workspaceId\\}/decisions|workspaces/.*/decisions|/workorch/workspaces/.*/decisions" internal web scripts docs -g '!internal/modules/application/workorchd/service/server/consoleassets/dist/**'
```

Suggested commands:

```bash
go test ./internal/modules/application/workorchd/service/server -count=1
go test ./internal/modules/application/workorchd/biz/appservice/... -count=1
go test ./internal/services/workorch/service/tui -count=1
go test ./internal/pkg/events -count=1
```

Full regression after implementation:

```bash
go test ./... -count=1 -p=1
make verify-ci-mvp-file
```

## Acceptance Criteria

- No automatic scheduler pending replay after restart.
- No periodic watchdog replan compensation.
- All scheduler command pre-accepted failures return explicit errors.
- All accepted asynchronous continuation failures produce bounded diagnostics.
- Projection and TUI/Web show manual next actions without mutating formal
  business state.
- TUI prompt blocker/running/submitting suffix always matches the active
  workspace server projection or a neutral refreshing/no-workspace state.
- TUI local display cache does not authoritatively create, clear, or move
  blocker/progress state across workspaces.
- Metrics/diagnostics expose failures enough to distinguish provider/runtime
  gap, projection stale, scheduler enqueue failure, and continuation execution
  failure.
