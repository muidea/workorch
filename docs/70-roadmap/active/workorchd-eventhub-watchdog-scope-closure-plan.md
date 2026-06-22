# Workorchd EventHub / Watchdog Scope Closure Plan

## Purpose

This document freezes the current scope review for workorchd command flow, runtime event flow, watchdog health checks, TUI/Web interaction, and CPU/IO related recovery behavior.

The goal is to guide code closure item by item without expanding watchdog into a general scheduling fallback. WorkOrch core must remain business-neutral. Software-development-specific behavior must stay in workspaceTemplate, Soul, profile, skill, or other template-owned data.

## Confirmed Design Rules

1. workorchd owns all workspace state management, run state transitions, read-model projection, command scheduling, and runtime execution coordination.
2. TUI, Web, CLI, and remote clients only submit user intent and render workorchd-owned state. They must not maintain a second business state machine.
3. REST and TUI commands must enter workorchd through EventHub command events, then be accepted only after the scheduler owner has validated and queued the command.
4. EventHub `Send` may be used for command acknowledgement, but subscribed handlers must do only fast validation and enqueue work. Long execution must run outside the sending lane through managed background scheduling.
5. Watchdog is not normal scheduling, not startup recovery, and not an EventHub event-loss fallback.
6. Watchdog may only compensate external interactions that can fail to return within the expected time limit, and the compensation must be idempotent or backed by durable evidence.
7. Missing EventHub subscription, failed command enqueue, projection failure, owner command/result failure, or missing runtime notification is a design/implementation defect. These cases must be diagnosed and fixed on the main path, not silently recovered by watchdog.
8. Service restart does not automatically resume historical running runs. Recovery after restart should be explicit: manual `/run continue`, replan, restart, or a dedicated diagnostic/recovery command.

## Current Status

The first boundary adjustment has already been made locally:

- Startup automatic recovery was removed.
- Startup now only registers the watchdog timer.
- `workorchd` service startup now treats missing framework `EventHub` or
  `BackgroundRoutine` as assembly errors. Background scheduling does not create
  unmanaged goroutine fallback when the routine is absent or rejects a task.
- Periodic watchdog currently calls stale dispatched runtime-gap recovery only.
- Historical running-run recovery, resolved-governance recovery, and replan continuation recovery are no longer part of periodic startup recovery and should remain explicit/manual paths unless a later design explicitly changes that.
- Periodic stale runtime-gap recovery now enumerates only compact running-run
  candidates through run owner read command payload
  `states=[running], compact=true`; candidate runs omit `ExecutionContext`.
- Watchdog runtime-gap classification uses scoped step summaries and scoped
  runtime checkpoints. It does not read workspace snapshot, run snapshot,
  global event list, or raw event payload on the periodic path.
- Automatic recovery remains gated by neutral recovery safety metadata:
  `request_not_accepted` is diagnostic-only, terminal gaps require terminal
  result evidence, and in-progress/worker-lost gaps require explicit watchdog
  retry safety.
- Runtime event terminal/lifecycle publication failures are now observable.
  `runtimeevent` records accepted, terminal, progress, worker lifecycle,
  worker-lost, and heartbeat timer start failure counters, and `/metrics`
  exposes them as `workorch_runtime_event_publish_failures_total` plus
  `workorch_runtime_heartbeat_timer_start_failures_total`.

Remaining work is the scope below.

## P0 Closure Scope

### 1. Scheduler Command Enqueue Reliability

Files:

- `internal/modules/application/workorchd/service/server/workorchd_scheduler_commands.go`
- `internal/modules/application/workorchd/service/server/http_run_continuation_scheduler.go`
- `internal/modules/application/workorchd/service/server/http_run.go`
- `internal/modules/application/workorchd/service/server/http_workspace.go`
- `internal/modules/application/workorchd/service/server/http_governance.go`

Current issue:

- `handleWorkorchDSchedulerCommand` accepts scheduler events even when `scheduleWorkspaceActivation`, `scheduleWorkspaceRestart`, `scheduleRunContinuation`, `scheduleGovernanceRunResumeRefs`, or `scheduleReplanContinuationIfNeeded` fails to enqueue work.
- Several calls ignore the scheduling result with `_ = ...`.
- This can make REST/TUI receive an accepted response even though no actual work was queued.

Required closure:

- Scheduler command handlers must return an error when queueing fails.
- `publishWorkorchDSchedulerCommand` should only treat a command as accepted after the scheduler owner returns a valid accepted response.
- Workspace activation/restart, run start/resume, governance continuation, and replan continuation must all share this rule.
- Queueing must remain fast and lane-safe. Handler must not execute the long-running activation/resume/replan body inline.

Tests:

- Scheduler command returns error when background queue scheduling fails.
- REST activation/restart/start/resume returns non-accepted business error when scheduler enqueue fails.
- Governance decision/approval resolution does not claim continuation accepted when scheduler enqueue fails.

### 2. Watchdog Runtime-Gap Recovery Boundary

Files:

- `internal/modules/application/workorchd/service/server/http_run_continuation_scheduler.go`
- `internal/modules/application/workorchd/biz/appservice/run_recovery_service.go`
- `internal/modules/application/workorchd/biz/appservice/runports/recovery_orphan_step.go`
- `internal/modules/application/workorchd/biz/appservice/runports/recovery_orphan_step_candidates.go`
- `internal/modules/application/workorchd/biz/appservice/runports/recovery_runtime_gap.go`
- `internal/modules/application/workorchd/biz/appservice/runports/commands.go`
- `internal/modules/kernel/run/pkg/contracts/commands.go`
- `internal/modules/kernel/orchestrator/module.go`

Current issue:

- Runtime-gap classification can detect `request_not_accepted`, `accepted_no_progress`, `accepted_in_progress`, `worker_lost`, and `runtime_terminal_not_finalized`.
- The current recovery decision is not explicit enough about which gap types are safe for automatic watchdog recovery.
- `request_not_accepted` likely indicates a main-path EventHub dispatch/subscribe/queue defect and should not be silently recovered by periodic watchdog.

Required closure:

- Periodic watchdog must auto-recover only runtime gaps that have durable evidence proving the runtime request entered an external execution boundary.
- `request_not_accepted` must be diagnostic-only for periodic watchdog. It may be exposed in projection/status, but it must not be auto-recovered.
- `runtime_terminal_not_finalized` may be recovered only when terminal runtime result evidence exists.
- `worker_lost`, `accepted_no_progress`, and `accepted_in_progress` may be recovered only when recovery safety metadata says the step is idempotent or externally safe to retry/resume.
- Runtime-gap recovery command should carry the evidence used to justify recovery, including request id, queue key, latest step time, gap type, and recovery safety classification.

Tests:

- Stale `request_not_accepted` dispatched step is not recovered by periodic watchdog.
- Runtime terminal result not finalized is recovered only when terminal result evidence exists.
- Worker-lost or accepted-in-progress gap is not auto-recovered for a non-idempotent step.
- Worker-lost or accepted-in-progress gap is recovered for an explicitly recoverable external interaction.

### 3. Runtime Recovery Safety Metadata

Files:

- `internal/pkg/workorchruntimecontract/*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_event.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- `internal/modules/kernel/orchestrator/runtime_event_adapter.go`
- `internal/modules/blocks/pkg/runtimeevent/runtime_event.go`
- Runtime block packages under `internal/modules/blocks/*`

Current issue:

- Runtime execution request/accepted/checkpoint data includes request id, queue key, step kind, and timestamps, but not a clear recovery safety policy.
- Watchdog cannot reliably distinguish read-only/idempotent external work from state-changing or non-idempotent work.

Required closure:

- Add a business-neutral runtime recovery policy to the runtime contract.
- The policy should express generic semantics such as side-effect scope, idempotency, retryability, timeout behavior, and whether watchdog may recover a missing terminal response.
- Core must not infer this from template id, workspace title, natural-language goal text, programming language, or command text.
- Runtime blocks or step contracts should provide the policy; orchestrator and watchdog only consume the structured policy.

Possible neutral fields:

- `sideEffectPolicy`: `none`, `workspace_read`, `workspace_write`, `external_read`, `external_write`, `unknown`
- `idempotencyPolicy`: `idempotent`, `dedupe_key_required`, `non_idempotent`, `unknown`
- `watchdogRecovery`: `disabled`, `finalize_terminal_only`, `retry_external`, `manual_only`
- `idempotencyKey`

Tests:

- Policy is carried from dispatch request to runtime checkpoint/projection.
- Unknown or non-idempotent policy defaults to no automatic watchdog retry.
- Explicit idempotent external policy allows watchdog recovery for supported gap types.

### 4. Runtime Event Publication Reliability

Files:

- `internal/modules/blocks/pkg/runtimeevent/runtime_event.go`
- `internal/modules/blocks/pkg/runtimeevent/runtime_event_test.go`
- `internal/modules/blocks/pkg/runtimeevent/eventdefs/topics.go`
- `internal/pkg/events/event_contract.go`

Current issue:

- Runtime lifecycle notifications are often published with ignored errors.
- `runtime_worker_started`, `runtime_worker_heartbeat`, `runtime_worker_stopped`, `runtime_worker_lost`, completed, and failed events feed checkpoint/projection/watchdog evidence.
- If publication fails silently, workorchd can later diagnose from incomplete evidence and take the wrong recovery path.

Required closure:

- Accepted, completed, failed, and worker-lost publication failures must be observable and must not be silently swallowed.
- Heartbeat/progress publication failures may remain non-fatal, but must be rate-limited/logged or otherwise observable.
- Runtime terminal state should not be recorded as completed in local executor state before the terminal event is durably published, unless replay semantics are made explicit and tested.
- Runtime event lane policy has been reviewed for the audited path. Runtime accepted/result events use scoped state lanes, and owner notifications now use owner/workspace/run scoped state lanes; future topic-lane notifications require an explicit global-ordering reason.

Tests:

- Terminal publish failure is surfaced by runtime executor.
- Worker-lost publish failure is logged or returned through a test-observable hook.
- Runtime event ordering for accepted/progress/completed/failed remains deterministic per workspace/run/step.

Closure record:

- Code: `internal/modules/blocks/pkg/runtimeevent/metrics.go`,
  `internal/modules/blocks/pkg/runtimeevent/runtime_event.go`,
  `internal/modules/application/workorchd/service/server/http_response_helpers.go`,
  `scripts/observe-workorch-runtime-closure.sh`, `Makefile`.
- Behavior: runtime event publish failure is counted at the shared
  `publishRuntimeNotification` boundary. Terminal completed/failed publish
  failures increment the terminal counter and still avoid recording terminal
  replay state before durable publication succeeds. Accepted publish failures
  increment the accepted counter and still prevent worker start. Worker
  lifecycle progress publish failures increment progress and lifecycle
  counters, with `runtime_worker_lost` also incrementing the worker-lost
  counter. Heartbeat timer start failure is counted and logged.
- Metrics: `/metrics` exposes
  `workorch_runtime_event_publish_failures_total{category="accepted"}`,
  `terminal`, `progress`, `worker_lifecycle`, `worker_lost`, and
  `workorch_runtime_heartbeat_timer_start_failures_total`.
- Runtime observation: `observe-workorch-runtime-closure.sh` requires the new
  metrics and reports `runtimeEventPublishFailures` deltas. The optional
  `WORKORCH_OBSERVE_MAX_RUNTIME_EVENT_PUBLISH_FAILURES_DELTA` gate is set to
  zero by `make verify-runtime-idle-baseline`.
- Verification:
  `go test ./internal/modules/blocks/pkg/runtimeevent ./internal/modules/application/workorchd/service/server -run 'TestExecutor(TerminalPublishFailureDoesNotRecordCompletedTerminal|WorkerLostPublishFailureIsReturned|AcceptedPublishFailureDoesNotStartWorker)|TestMetricsEndpointReportsCounters' -count=1`.
  `WORKORCH_OBSERVE_DURATION_SECONDS=5 WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 make verify-runtime-idle-baseline`
  passed with artifacts in `/tmp/tmp.E6cSX08MH6/observe-runtime-closure`;
  `runtimeEventPublishFailures.totalPublishFailureDelta=0`,
  `projectionPendingRebuilds=0`, event payload hydrate delta `0`, and process
  read/write byte deltas `0`.

## P1 Closure Scope

### 5. EventHub Lane and Handler Audit

Files:

- `internal/pkg/events/magic_hub.go`
- `internal/pkg/events/event_contract.go`
- `internal/modules/kernel/**/module.go`
- `internal/modules/application/workorchd/**`
- `internal/modules/blocks/**`

Required audit:

- Command handlers using synchronous `Publish`/`Send` must be quick and must not wait for long-running work in the same lane.
- Owner state changes should use state lanes scoped to the business owner and object identity.
- Read/query commands should use query lanes.
- Runtime execution commands should not serialize unrelated workspaces unnecessarily.
- Any handler that must launch long work should enqueue it through managed background scheduling and return accepted only after enqueue succeeds.

Tests:

- Replan/continue for different workspaces can be queued independently.
- A long runtime execution does not block command acceptance for another workspace.

### 6. Workorchd Projection and Client Display Boundary

Files:

- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/**`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`
- `internal/modules/application/workorchd/service/server/http_workspace.go`
- `internal/pkg/workorchclient/**`
- `internal/pkg/workorchport/**`
- `internal/services/workorch/service/tui/**`

Required audit:

- workorchd must be the owner of workspace list/detail projection and prompt state facts.
- TUI must not infer whether a workspace is `decision`, `running`, `failed`, or `submitting` from stale local state after switching workspace.
- TUI and Web should consume projection revision/state and render degraded/refreshing status explicitly.
- Command submission state must be separate from workspace business state.

Tests:

- Switching workspace refreshes prompt state from workorchd projection.
- Resolving a decision updates the selected workspace view after projection event.
- A failed command submission does not permanently overwrite the workspace prompt state.

### 7. CPU/IO Load Boundary

Files:

- `internal/modules/application/workorchd/service/server/http_run_continuation_scheduler.go`
- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/**`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`
- `internal/modules/application/workorchd/biz/appservice/runports/**`
- `internal/services/workorch/service/tui/**`

Required audit:

- Periodic watchdog must not repeatedly scan and rebuild all workspace/run details when no external runtime gap is eligible.
- Projection should use incremental event-driven updates where possible.
- TUI should subscribe to server projection/event stream and avoid polling-heavy refresh loops when no remote client mode requires it.
- Runtime history access should prefer recent complete records plus compact historical summaries instead of repeated full reads.

Tests:

- Watchdog with no eligible gaps performs bounded reads.
- Projection rebuild is not repeatedly triggered by unchanged state.
- TUI remote mode does not start unnecessary refresh routines when no remote client is active.

## P2 Documentation and Verification Scope

Documents to update after code closure:

- `docs/30-module-architecture/eventhub-driven-workorchd-state-loop-design.md`
- `docs/30-module-architecture/runtime-execution-event-queue-state-loop-design.md`
- `docs/30-module-architecture/mvvp-workspace-viewmodel-architecture.md`
- `docs/60-verification/framework-eventhub-module-boundary-checklist.md`
- `docs/70-roadmap/active/workorchd-state-loop-and-projection-closure-plan.md`

Verification commands:

```bash
go test ./internal/modules/application/workorchd/service/server -count=1
go test ./internal/modules/application/workorchd/biz/appservice/... -count=1
go test ./internal/modules/blocks/pkg/runtimeevent -count=1
go test ./internal/pkg/events -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
```

Full regression after P0 closure:

```bash
go test ./... -count=1
make verify-ci-mvp-file
```

## Non-Goals

- Do not add template-specific or software-development-specific rules to core.
- Do not infer recovery safety from file names, command strings, language names, template ids, Soul ids, or goal text.
- Do not add periodic compensation for missing EventHub events.
- Do not make TUI/Web repair backend state by local inference.
- Do not restart historical running runs automatically on service startup.

## Closure Order

1. Fix scheduler enqueue acknowledgement semantics.
2. Restrict periodic watchdog to explicitly safe external runtime gaps.
3. Add runtime recovery safety metadata and checkpoint/projection support.
4. Make runtime terminal/lifecycle publication failures observable.
5. Re-audit EventHub lane policies and handler duration.
6. Re-audit projection/TUI state display after command submission and workspace switching.
7. Run targeted tests, then full regression.
