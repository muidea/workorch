# Runtime Contract Retry Batch 1 State Model Audit

## Scope

This is the Batch 1 audit required by `runtime-contract-retry-full-chain-audit-plan.md`.

Goal:

- Confirm the expected state chain from activation to decision.
- Confirm the EventHub topics and owner boundaries used by each transition.
- Identify state-loop deviations before any code closure starts.

This document audits the current implementation only. It does not prescribe business-specific behavior for the live software-development workspaces.

## Expected State Chain

| Chain Node | Expected Owner | Expected Input | Expected Output / Event | Projection Impact |
| --- | --- | --- | --- | --- |
| Workspace activation / resume request | workorchd scheduler facade | REST/TUI/Web command | `/workorchd/command/workspace-run/requested` accepted response | command accepted marker; no backend state claim yet |
| Run creation / ensure | run owner through EventHub command | workspace scoped activation command | `/run/created` | run appears in projection |
| Run start / resume / replan | orchestrator owner | `/orchestrator/command/*` | `/run/state/changed` | run state and workspace state refresh |
| Step dispatch request | orchestrator owner | execution graph node / step spec | `/orchestrator/dispatch-step/requested` | dispatch is queued by step idempotency key |
| Step persisted / dispatched | run/orchestrator owner | dispatch command | `/step/dispatched`, `/step/state/changed` | step appears as runtime progress item |
| Runtime block accepted | runtime block owner | `/runtime/block/command/execute/<kind>` | `/runtime/block/execute/accepted` | runtime checkpoint and heartbeat can start |
| Runtime block terminal result | runtime block owner | accepted execution | `/runtime/block/execute/completed` or `/runtime/block/execute/failed` | orchestrator finalizes step |
| Contract violation | orchestrator owner | invalid LLM/runtime output | `/orchestrator/contract_violation/recorded`, optional retry/escalated | retry or decision state appears |
| Governance decision | decision owner | escalated runtime blocker | `/ticket/created`, `/ticket/resolved` | workspace/run becomes waiting decision or resumes |
| Projection update | workorchd projection owner | owner events and detail read model | `/workorchd/workspace/projection/changed` | TUI/Web render ViewModel only |

## Implementation Evidence

| Evidence | Implementation Finding |
| --- | --- |
| `ev:code/scheduler/workspace-run-request/001` | HTTP/TUI-facing run commands publish `/workorchd/command/workspace-run/requested`; scheduler returns accepted response only after queueing/scheduling succeeds. |
| `ev:code/orchestrator/dispatch-request/001` | Runtime orchestrator can publish `/orchestrator/dispatch-step/requested` with workspace/run/step scope and idempotency key. |
| `ev:code/runtime/block-executor/001` | Runtime block executor acknowledges accepted work before asynchronous execution and later publishes terminal result events. |
| `ev:code/eventhub/magic-publish/001` | `MagicHub.Publish` uses magicCommon `Send`, binds a lane key, and therefore synchronously waits for handler response; `MagicHub.PublishAsync` uses magicCommon `Post` and does not wait for a send response. |
| `ev:code/eventhub/lane-policy/001` | State-changing commands use state lanes; runtime execute commands use execute lanes; runtime accepted/result topics use runtime state/orchestrator state lanes; projection changed and owner notify topics use owner/workspace/run state lanes. |
| `ev:code/projection/manager/001` | workorchd owns workspace projection through `workspaceviewcore.Manager`, and `/events/stream?view=projection` subscribes to projection-changed events. |

## State Model Audit Result

### B1-SM-001: Command Entry Is EventHub-Driven

- Claim: Web/TUI-facing workspace run commands enter the backend through EventHub command topics.
- Expected behavior: REST/TUI should produce command events, not directly mutate owner state.
- Actual behavior: Workspace run start/resume/replan/restart commands publish `/workorchd/command/workspace-run/requested`; the scheduler handler chooses activation, restart, replan, or run continuation.
- Evidence: `ev:code/scheduler/workspace-run-request/001`.
- Deviation type: none for command entry.
- Owner: workorchd scheduler facade.
- Closure batch: 1.
- Required action: keep this as the canonical command entry path.

### B1-SM-002: Scheduler ACK Is Queue Acceptance, Not Run Completion

- Claim: The scheduler accepted response means "accepted for scheduling", not "run state already changed".
- Expected behavior: UI may show submitting/accepted transient state but must wait for projection state events for actual running/decision/completed state.
- Actual behavior: `projectAcceptedSchedulerCommand` updates projection with accepted command metadata immediately after queue acceptance.
- Evidence: `ev:code/scheduler/workspace-run-request/001`.
- Deviation type: client/projection semantics risk.
- Owner: projection / ViewModel.
- Closure batch: 1 and 2.
- Required action: future UI state should distinguish command accepted, run scheduled, run dispatched, and run state changed. Do not let clients infer running from accepted ACK.

### B1-SM-003: Dispatch Is Event-Driven And Idempotent

- Claim: Orchestrator dispatch now uses an event-driven request path.
- Expected behavior: graph node dispatch should be represented as a scoped event and idempotent per workspace/run/step/kind.
- Actual behavior: `requestDispatchStep` publishes `/orchestrator/dispatch-step/requested` with a `workspace|run|step|kind` idempotency key when event-driven dispatch is enabled.
- Evidence: `ev:code/orchestrator/dispatch-request/001`.
- Deviation type: none for dispatch entry.
- Owner: core orchestrator.
- Closure batch: 1.
- Required action: retain this as the only event-driven dispatch path.

### B1-SM-004: Runtime Execution Uses ACK + Async Terminal Events

- Claim: Runtime block execution follows accepted/progress/completed/failed events.
- Expected behavior: the caller must not assume the runtime result is available at command ACK time; terminal state is carried by result events.
- Actual behavior: the runtime executor publishes accepted response first, starts background execution, then publishes progress and terminal result.
- Evidence: `ev:code/runtime/block-executor/001`.
- Deviation type: none for runtime block event contract.
- Owner: runtime block infrastructure.
- Closure batch: 1.
- Required action: keep state finalization driven by terminal events.

### B1-SM-005: Synchronous Publish Can Backpressure Notify Paths

- Claim: `MagicHub.Publish` is synchronous and uses magicCommon `Send` for every envelope.
- Expected behavior: state-changing commands that need a result can use synchronous send; notify/projection events should not accidentally block owner state transitions on heavy observers.
- Actual behavior: `Publish` uses `Send`; `PublishAsync` uses `Post`; local wildcard dispatch still runs before both magic hub delivery modes. Missing observer is ignored, but synchronous `Publish` observers can backpressure the publish path. Projection changed notifications are published through `AsyncHub` when available.
- Evidence: `ev:code/eventhub/magic-publish/001`.
- Deviation type: event delivery/backpressure risk, with projection notification subpath covered and an explicit topic delivery policy now available.
- Owner: event infrastructure.
- Closure batch: 1.
- Required action: keep `EventHubDeliveryModeForTopic` and `PublishByContract` as the canonical delivery policy. Command/facade/execute/read/write topics require synchronous response semantics; notify topics are async-preferred observer updates. Run, workspace, approval, decision, governance, and orchestrator event-subsystem notifications now use the contract-aware publisher. Any further call-site migration remains owner-specific and must prove idempotency, ordering, and replay safety before replacing `Publish` with async delivery.

### B1-SM-006: Scoped Notify Lane Policy Is State-Oriented

- Claim: lane selection is structured by topic contracts.
- Expected behavior: stateful owner commands and owner state notifications must serialize by owner/workspace/run/subject scope; stateless or independent execution may use execute lanes.
- Actual behavior: owner write commands use state lanes; runtime execute commands use execute lanes; runtime accepted uses runtime state lane; runtime completed/failed use orchestrator state lane. Projection changed uses projection owner/workspace state lane. Owner notify topics now use state lanes so same-scope ordering is preserved without serializing unrelated workspaces on a global topic lane.
- Evidence: `ev:code/eventhub/lane-policy/001`.
- Deviation type: closed lane granularity risk.
- Owner: event infrastructure and owner module event contracts.
- Closure batch: 1.
- Required action: keep owner notifications on state lanes unless a future topic explicitly proves global ordering is required. `TestMagicHubUsesStateLanesForScopedNotificationEvents` and `TestEventHubLaneKeySeparatesNotificationLanesByWorkspaceScope` cover the current decision.

### B1-SM-007: Projection Is Backend-Owned

- Claim: workorchd owns the workspace projection; Web/TUI should consume projection DTOs.
- Expected behavior: workorchd projection manager loads all workspace state, rebuilds workspace snapshots, and emits projection-changed events.
- Actual behavior: server projection is managed by `workspaceviewcore.Manager`; projection stream subscribes to `/workorchd/workspace/projection/changed`.
- Evidence: `ev:code/projection/manager/001`.
- Deviation type: none for owner placement.
- Owner: workorchd projection.
- Closure batch: 1.
- Required action: keep TUI/Web consuming workorchd projection DTOs; current TUI/backend regression tests verify accepted command metadata, workspace switch, prompt state, stale blocker clearing, event-driven refresh, and task plan decoding without a second client-side business state machine.

## State Transition Matrix

| From | Trigger | Expected Event | To | Audit Status |
| --- | --- | --- | --- | --- |
| idle workspace | workspace run requested / resume without run id | `/workorchd/command/workspace-run/requested` then run owner events | running or waiting activation | supported |
| blocked_on_decision | replan command | `/workorchd/command/workspace-run/requested` with `intent=replan` | queued replan, then run resume/replan | supported, but projection semantics need clear accepted-vs-running state |
| running step | runtime accepted | `/runtime/block/execute/accepted` | dispatched/running with checkpoint | supported |
| running step | runtime completed | `/runtime/block/execute/completed` | step completed and next dispatch | supported |
| running step | runtime failed | `/runtime/block/execute/failed` | step failed, repair/retry/decision | supported |
| repeated contract violation | retry budget available | `/orchestrator/contract_violation/retry_requested` | retry step queued | supported |
| repeated contract violation | budget exhausted | `/ticket/created` | blocked_on_decision / waiting_decision | supported |

## Batch 1 Closure Gates

### Gate B1-G1: Publish Semantics

- Chain node: EventHub publish for commands, state events, runtime events, and projection notifications.
- Deviation type: `event_publish_backpressure_risk`.
- Evidence refs: `ev:code/eventhub/magic-publish/001`, `ev:code/eventhub/lane-policy/001`.
- Owner: event infrastructure.
- Expected behavior: commands needing return values use synchronous response; notify/state projection events should not block unrelated work or lose events.
- Actual behavior: current `Publish` is synchronous; `PublishAsync` is available and uses `Post`. The workspace projection manager already uses `PublishAsync` for projection changed notifications when the hub supports it.
- Closure status: code-closed for delivery policy, projection changed async publication, and the audited run/workspace/approval/decision/governance/orchestrator event-subsystem notify publishers.
- Closure evidence: `TestMagicHubPublishAsyncUsesPostWithoutSendResponseWait` proves async delivery uses magicCommon `Post` instead of `Send`; `TestManagerPublishesProjectionChangedThroughAsyncHub` proves projection changed notifications use the async branch; `TestEventHubDeliveryModeForTopicClassifiesCommandsAndNotifications` proves command/read/write/facade/runtime-execute topics are sync, runtime terminal notify results and projection notifications are async-preferred, facade result topics keep response semantics, and unknown topics remain conservative sync; `TestPublishByContractUsesAsyncForNotifyWhenAvailable`, `TestPublishByContractFallsBackToSyncWhenAsyncUnavailable`, `TestPublishByContractIgnoresNotifyResponseWhenFallingBackToSync`, `TestPublishByContractUsesSyncForCommandTopics`, and `TestPublishByContractPreservesCommandResponseWhenUsingSync` prove the shared publisher honors the policy and hub capability, ignores response envelopes only for notify fallback, and preserves command response semantics; `TestService_PublishesRunNotificationsThroughAsyncHub`, `TestGoalDoDManager_PublishesWorkspaceNotificationsThroughAsyncHub`, `TestGoalDoDManager_PublishesDefinitionNotificationsThroughAsyncHub`, `TestService_PublishesApprovalNotificationsThroughAsyncHub`, `TestService_PublishesDecisionNotificationsThroughAsyncHub`, `TestPublishGovernanceEventUsesAsyncHubForNotifications`, `TestEventSubsystemPublishesNotificationsThroughAsyncHub`, `TestRecordMemoryRecallTracePublishesRecordedNotificationThroughAsyncHub`, `TestPublishMemoryEventPublishesNotificationThroughAsyncHub`, `TestRuntimeOrchestrator_StepInputMemoryTraceRecordsExcludedProviderRetryMemory`, and `TestRuntimeOrchestratorCreateApprovalPublishesNotificationThroughAsyncHub` prove audited owner notifications use contract-aware async publication when available.
- Proposed change: no automatic behavior migration from this gate. Future owner-specific migrations may use `EventHubDeliveryModeForTopic`, but must document that the topic is idempotent, replay-safe, and does not require immediate response semantics.
- Why this is not a business-specific core rule: applies to all WorkOrch events.
- Required tests: lane response tests, notify backpressure tests, no lost projection event tests.

### Gate B1-G2: Accepted Command Projection

- Chain node: scheduler accepted command to projection state.
- Deviation type: `accepted_command_state_semantics`.
- Evidence refs: `ev:code/scheduler/workspace-run-request/001`.
- Owner: projection / ViewModel.
- Expected behavior: accepted command state is separate from run actual state.
- Actual behavior: projection records accepted command metadata in `runCommand`; run state still comes from owner-derived workspace/run state and active run fields.
- Closure status: closed for the current ViewModel path.
- Closure evidence: `TestDecodeSnapshotInfoPreservesRunCommandStatus` proves remote snapshot decoding preserves accepted command metadata; `TestModelWorkspaceContextDoesNotTreatAcceptedRunCommandAsRunning` proves a workspace context containing accepted command metadata without a live run does not keep stale submitting state or render as running; Web `WorkspaceSnapshot` now declares `runCommand` as `RunCommandStatus` and the workspace detail page renders it as independent "调度 ACK" metadata while keeping actual workspace/run state derived from owner projection fields.
- Proposed change: no further code change for this gate unless future UI code introduces a separate scheduler state display. If that happens, keep `lastAcceptedCommand` / scheduler metadata distinct from owner-derived `runState`.
- Why this is not business-specific: applies to all workspace command sources.
- Required tests: command accepted without run transition must not display as running. Covered by the closure evidence above.

## Batch 1 Verdict

The state model is broadly aligned with the EventHub-driven architecture:

- command entry is EventHub-driven
- dispatch is event-driven and scoped
- runtime block execution uses accepted/progress/terminal events
- projection is owned by workorchd

The remaining risks are architectural, not live-workspace-specific, and are conditional owner migration items rather than the first retry-convergence implementation target:

- synchronous publish semantics can backpressure notify paths when a notify call site still uses raw `Publish`; the canonical topic policy and shared `PublishByContract` are now available, and run/workspace/approval/decision/governance/orchestrator event-subsystem notifications have been migrated. Further migrations must be owner-specific
- owner notify lane policy now uses state-scoped lanes; future topic-lane notifications require explicit global-ordering rationale
- accepted scheduler command projection must remain distinct from actual run state; current ViewModel/backend behavior is covered by regression tests
- client projection ownership is covered by TUI/backend regression tests for accepted command metadata, workspace switch refresh, stale decision prompt clearing, running-state projection override, and task plan decoding; Web consumes the same `runCommand` metadata only as ACK display data and does not use it to infer running state

These risks are now covered by the Batch 1 closure gates and regression
evidence above for the audited paths. Reopen this batch only when a future
owner notification, topic lane, scheduler ACK, or client projection change
introduces new evidence that the accepted-command/state-transition boundary is
ambiguous again.
