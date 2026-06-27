# Resolved Decision Continuation Owner State Closure Plan

Status: closed

Type: closure-plan

Last Updated: 2026-06-27

Latest Closure Update: 2026-06-27

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `convergence-authority-owner-state-continuation-closure-plan.md`
- `workorchd-scheduler-continuation-diagnostics-plan.md`
- `authority-live-verification-e2e-closure-design.md`

## Purpose

本文固化 2026-06-27 live verification 中发现的决策恢复链路缺陷，并作为后续代码逐项收口依据。

目标是修复以下通用问题：

- run/workspace 仍被已经 resolved 的 decision ticket 阻塞。
- `replan` 没有确定性生成 successor run 或明确失败诊断。
- 无效 `provide_missing_info` resolution 关闭了 ticket，但没有产生可恢复语义。
- scheduler continuation 的完成状态没有严格以 owner state readback 为准。
- TUI/Web prompt 展示了 decision 状态，但 workorchd 已没有 open decision。

本文只定义 WorkOrch core、workorchd、port/client、projection/TUI/Web 的业务中立能力收口项。不得因为当前 live workspace、模板、语言、文件名、命令文本、provider 或具体业务目标加入 core 特判。

## Contract Audit

- Scope:
  - live workspace `ws-4e83810542ac4d65`
  - run `run-004`
  - decision ticket `run-004-contract-retry-001-of-planning-001-2-repair-exhausted-decision`
  - decision resolution, governance continuation, run owner state, workspace projection, TUI/Web prompt mode.
- Observed state:
  - workspace list shows `state=waiting_decision`.
  - active run is `run-004`.
  - run state is `blocked_on_decision`.
  - run keeps `blockingTicketId=run-004-contract-retry-001-of-planning-001-2-repair-exhausted-decision`.
  - ticket detail shows `state=resolved`.
  - workspace decision list has `open=0`.
  - resolution type is `provide_missing_info`.
  - resolution payload contains `answer="/"`.
  - resolution has empty `SuccessorRunID`, empty `ResumeRunID`, and empty `ContinuationAction`.
  - run events show `/ticket/resolved`, followed by `/run/state/changed state=blocked_on_decision` with the same resolved ticket.
  - workspace events also contain a later `/workspace/state/changed state=running`, while the run remains blocked on the resolved decision.
- Expected state:
  - A run in `blocked_on_decision` must reference an open decision ticket in the same workspace/run.
  - Once the blocking decision is resolved, owner state must deterministically choose one outcome:
    - resume current run and dispatch the next step,
    - create or confirm a replan successor run,
    - close the run as failed/cancelled/superseded with a structured reason,
    - reopen or reject the resolution before it is accepted, if the resolution is semantically invalid.
  - A workspace in `waiting_decision` must only be projected when the active run has an open blocking decision.
  - TUI/Web prompt mode must be derived from workorchd projection and must not locally infer decision state from stale cached blockers.
- Deviation:
  - owner state allows `blocked_on_decision` with a resolved ticket.
  - invalid `provide_missing_info` persisted as a resolved ticket without useful recovery data.
  - continuation scheduling did not produce successor run, resumed dispatch, terminal state, or durable failed diagnostic for this run.
  - projection exposes decision prompt state even though there is no open decision.
- Owner:
  - core owner state: run/ticket/resolution invariant, resume/replan lifecycle, state transition consistency.
  - workorchd scheduler: EventHub command ingress, continuation scheduling, readback diagnostics.
  - projection/ViewModel: read-only display of owner state, stale blocker diagnostics.
  - TUI/Web: scoped command submission and projection rendering only.
- Event path:
  - TUI/Web decision command -> workorchport/workorchclient DTO -> workorchd REST/EventHub -> decision owner resolves ticket -> governance continuation event -> scheduler -> runtime owner resume/replan -> run/workspace state events -> projection ViewModel -> TUI/Web prompt.
- Contract gap:
  - No hard invariant that a blocked decision run must reference an open ticket.
  - No deterministic reconciliation for persisted resolved blockers at service startup and projection rebuild time.
  - No single resolution outcome contract that binds resolution type to `resume`, `replan`, terminal, or rejection.
  - No readback-confirmed guarantee that accepted continuation actually advanced owner state.
  - No projection diagnostic for impossible owner state.

## Design Principles

1. Owner state is authoritative. Projection and TUI/Web must not repair or reinterpret owner state.
2. A resolved decision ticket is not a blocker. It is recovery input for the next owner transition.
3. `replan` is explicit. It must not be inferred from arbitrary missing-info text after resolution has been persisted.
4. `provide_missing_info` must contain semantic recovery information. Empty, slash-only, command-prefix-only, or whitespace-only payloads are invalid.
5. Accepted scheduler work is not success. Success requires owner readback proving state advancement, governed wait on a new open blocker, successor creation, or terminal closure.
6. Startup reconciliation may repair impossible owner states, but it must use only generic run/ticket/resolution state and scoped IDs.
7. No code path may depend on workspace ID, title, template ID, Soul ID, language, file path, command text, provider, or business goal text.

## Target State Machine

### Decision Block Invariant

```text
run.state == blocked_on_decision
  requires run.blockingTicketId != ""
  requires ticket.workspaceId == run.workspaceId
  requires ticket.runId == run.id
  requires ticket.state == open
```

If any requirement is false, the run is in an inconsistent owner state and must not be projected as a normal waiting decision.

### Resolution Outcome Matrix

| Ticket state | Resolution state | Resolution type | Required owner outcome |
| --- | --- | --- | --- |
| `open` | none | n/a | keep `blocked_on_decision`; projection prompt mode `decision` |
| `resolved` | present | `replan` | create/confirm successor run, or record failed replan diagnostic and expose manual retry |
| `resolved` | present | `provide_missing_info` with valid payload | resume current run using resolution input, or terminal failed with structured owner reason |
| `resolved` | present | `provide_missing_info` with invalid payload | mark resolution invalid and terminal/reopen according to owner policy; do not keep old blocker |
| `resolved` | present | `reject_current_path` | close run as failed/cancelled; clear old blocker |
| `resolved` | missing | unknown | mark data integrity failure; clear normal waiting decision projection and expose diagnostic |
| `dismissed` | any | any | clear normal blocker; continue only if owner policy explicitly allows |

### Workspace Projection Mapping

```text
active run blocked_on_decision + open ticket
  -> workspace waiting_decision
  -> promptMode decision

active run blocked_on_decision + resolved/missing ticket
  -> workspace diagnostic/inconsistent or terminal after owner reconciliation
  -> promptMode status/run, not decision

active run running/validating
  -> workspace running
  -> promptMode run

active run failed/completed/cancelled/superseded
  -> workspace active/idle equivalent projection
  -> promptMode run/status
```

The exact workspace state names must use existing common states where possible. Do not add a new public state unless the current state model cannot represent the transition.

## Required Work Items

### P0: Owner Invariant Guard for Blocked Decision Runs

Owner: orchestrator runtime lifecycle / run state subsystem.

Status: closed for readback/projection; owner lifecycle guard verified by targeted tests.

Requirements:

- Add a reusable neutral guard that checks whether `run.BlockingTicketID` still references an open ticket.
- The guard must run before:
  - resume loaded run,
  - replan loaded run,
  - owner state event publication,
  - startup/recovery scan,
  - projection snapshot creation where owner state is loaded.
- A run must not remain `blocked_on_decision` when the ticket is resolved, dismissed, missing, or scoped to another workspace/run.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_lifecycle.go`
- `internal/modules/kernel/orchestrator/biz/runtime_run_state_subsystem.go`
- `internal/modules/kernel/orchestrator/module.go`
- `internal/modules/application/workorchd/service/server/http_run_continuation_scheduler.go`

Acceptance:

- Unit test: `blocked_on_decision` + resolved ticket is not treated as normal governed wait.
- Unit test: `blocked_on_decision` + missing ticket produces structured owner diagnostic or terminal state.
- Unit test: ticket scoped to another workspace/run is rejected as inconsistent.
- Boundary search proves no template/language/file-name logic is added.

Closure evidence:

- `TaskProgressRecovery` only keeps decision/approval blockers when the scoped ticket/approval is still open.
- Workspace detail/list projection normalizes stale resolved/missing blockers out of normal prompt-driving fields.
- Scheduler governance continuation uses owner readback before recording completion.
- Targeted tests passed:
  - `go test ./internal/modules/kernel/orchestrator/biz ./internal/modules/kernel/decision/biz ./internal/modules/application/workorchd/service/server ./internal/modules/application/workorchd/biz/appservice/readmodelprojection ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -count=1`
  - `go test ./internal/services/workorch/service/tui ./internal/services/workorch/biz/backends ./internal/pkg/workorchport ./internal/pkg/workorchclient -count=1`

### P0: Resolved Blocker Reconciliation on Startup and Rebuild

Owner: workorchd startup/recovery, orchestrator owner state.

Status: closed for scheduler startup scan and projection rebuild visibility.

Requirements:

- On service startup and explicit projection rebuild, scan active runs for impossible blockers:
  - blocked decision with resolved/dismissed/missing ticket,
  - waiting workspace whose active run has no open decision,
  - workspace running while active run is blocked.
- Reconciliation must produce one of:
  - owner resume/replan command accepted and readback advanced,
  - terminal owner state with failure reason,
  - structured diagnostic requiring manual retry.
- Reconciliation must not silently mutate projection only.
- Reconciliation must not replay old process-local scheduler pending work. It may only act on durable owner state and resolution records.

Likely code areas:

- `internal/modules/application/workorchd/service/server/http_run_continuation_scheduler.go`
- `internal/modules/application/workorchd/service/server/http_types.go`
- `internal/modules/application/workorchd/biz/bootstrap/runtime.go`
- `internal/modules/kernel/orchestrator/biz/runtime_lifecycle.go`

Acceptance:

- Unit test: service startup finds resolved blocking decision and does not leave workspace `waiting_decision`.
- Unit test: resolved replan without successor schedules or reports replan continuation diagnostic.
- Unit test: invalid resolved missing-info does not keep prompt mode `decision`.
- Live check: `ws-4e83810542ac4d65` no longer shows waiting decision unless a new open ticket exists.

Closure evidence:

- Resolved governance-blocked run recovery schedules scoped resume only when the decision/approval is resolved.
- Governance resume pending work is not cleared when readback still shows the old resolved blocker.
- Projection rebuild no longer presents resolved/missing decision blockers as normal `waiting_decision` prompt state.
- Unit coverage:
  - `TestServerStateRecoversResolvedDecisionBlockedRun`
  - `TestServerStateRecoversResolvedDecisionBlockedRunWithWorkspaceScope`
  - `TestServerStateGovernanceResumeDoesNotCompleteWhenReadbackStillBlockedOnResolvedDecision`
  - `TestStoreDoesNotUseResolvedDecisionForPromptMode`

### P0: Resolution Outcome Contract

Owner: governance appservice, decision module, runtime lifecycle.

Status: verified existing implementation.

Requirements:

- Persisted resolution must explicitly expose:
  - resolution type,
  - semantic intent,
  - validity,
  - continuation action: `resume`, `replan`, `terminal`, `none`.
- `replan` resolution must be the only resolution type that creates a replan successor.
- `provide_missing_info` must not create a replan successor.
- Invalid `provide_missing_info` must be rejected before persistence. For already persisted invalid resolutions, reconciliation must mark the run as inconsistent or terminal rather than keeping the old blocker.
- `reject_current_path` must close the run and clear old blocker.

Likely code areas:

- `internal/pkg/workorchport/governance.go`
- `internal/pkg/workorchclient/decision.go`
- `internal/modules/kernel/decision/biz/decision_service.go`
- `internal/modules/application/workorchd/biz/appservice/governanceports/decision_resolve_views.go`
- `internal/modules/application/workorchd/biz/appservice/governanceports/decision_batch_resolve_views.go`

Acceptance:

- Unit test: slash-only missing info is rejected and no resolution file is written.
- Unit test: `/decision replan` persists `resolutionType=replan` and `ContinuationAction=replan`.
- Unit test: explicit `provide_missing_info` with valid text resumes current run, not replan.
- Unit test: historical invalid missing-info resolution is surfaced as invalid owner state during reconciliation.

Closure evidence:

- Existing decision/governance tests cover slash-only missing-info rejection, explicit replan persistence, and continuation action projection.
- Targeted tests passed:
  - `go test ./internal/modules/application/workorchd/biz/appservice -run 'Test.*Decision|Test.*Governance|Test.*Resolve' -count=1`
  - `go test ./internal/modules/application/workorchd/biz/appservice/governanceports -count=1`

### P1: Readback-Confirmed Continuation Completion

Owner: workorchd scheduler diagnostics.

Status: closed.

Requirements:

- `publishGovernanceContinuationRequested` and scheduler handlers must not record `completed` unless readback proves one of:
  - old blocker cleared,
  - successor run exists,
  - target run is terminal,
  - new open blocker exists and differs from the resolved blocker,
  - owner explicitly reports no action needed for a terminal/superseded state.
- If readback still shows the same resolved blocker, record:

```json
{
  "kind": "governance_continuation",
  "state": "failed",
  "reason": "blocked_on_resolved_decision",
  "recoverability": "manual_retry_required"
}
```

- Diagnostics must retain enough recent entries per workspace/run so one workspace does not overwrite another workspace's failed continuation evidence.

Likely code areas:

- `internal/modules/application/workorchd/service/server/http_run_continuation_scheduler.go`
- `internal/modules/application/workorchd/service/server/scheduler_diagnostics.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelcore/scheduler_diagnostics_types.go`
- `internal/pkg/workorchport/scheduler_diagnostics.go`

Acceptance:

- Unit test: accepted resume with same resolved blocker records failed diagnostic, not completed.
- Unit test: replan continuation completed only after successor exists or predecessor is superseded/terminal.
- Unit test: diagnostics retain entries for two workspaces independently.
- `/readyz` exposes latest and recent continuation diagnostics for the affected workspace.

Closure evidence:

- `runContinuationConfirmedAdvanced` is reused for governance resume completion.
- `replanContinuationConfirmedAdvanced` confirms successor creation, target terminal state, or records owner readback failure.
- Continuation diagnostics include readback for failed and completed continuation records.
- Unit coverage:
  - `TestServerStateRunContinuationDoesNotCompleteWhenReadbackStillBlockedOnResolvedDecision`
  - `TestServerStateGovernanceResumeDoesNotCompleteWhenReadbackStillBlockedOnResolvedDecision`
  - `TestServerStateRecoverReplanContinuationsSchedulesWaitingDecisionActiveSupersededRun`

### P1: Projection Inconsistency Diagnostic

Owner: read model projection / workspace view core.

Status: closed.

Requirements:

- Projection must derive decision prompt mode only from open decision blockers.
- If owner state is inconsistent, projection must expose a bounded diagnostic instead of rendering normal `waiting_decision`.
- List view, detail snapshot, TUI prompt, and Web workspace card must agree on:
  - workspace state,
  - prompt mode,
  - blocking ticket/approval,
  - projection revision.
- Projection must not hydrate full detail payloads to decide the prompt mode.

Likely code areas:

- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/store.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/convergence_status.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_snapshot_detail.go`
- `internal/services/workorch/service/tui/workspace_state.go`
- `internal/services/workorch/service/tui/view.go`
- `web/workorch-console/src/**`

Acceptance:

- Unit test: resolved ticket does not produce prompt mode `decision`.
- Unit test: open decision still produces prompt mode `decision`.
- Unit test: list and snapshot report the same prompt mode and blocker fields.
- Live check: TUI prompt for `ws-4e83810542ac4d65` does not show `decision >` unless a new open decision exists.

Closure evidence:

- `workspacePromptMode` derives `decision`/`approval` only from open scoped blockers.
- Workspace list uses normalized snapshot data when detail is available, so list/detail prompt mode and blocker fields agree.
- Unit coverage:
  - `TestWorkspaceGovernanceProjectionClearsResolvedDecisionBlocker`
  - `TestWorkspaceGovernanceProjectionKeepsOpenDecisionBlocker`
  - `TestStoreDoesNotUseResolvedDecisionForPromptMode`

### P1: EventHub Main Path for Decision Commands

Owner: TUI/Web/backend command path, workorchd REST/EventHub.

Status: closed.

Requirements:

- TUI and Web decision commands must submit scoped command payloads only.
- All command results must carry enough scope:
  - workspaceId,
  - runId,
  - decisionId,
  - requestId,
  - resolutionType,
  - continuation intent.
- REST command acceptance must fail synchronously if EventHub publish/send cannot enqueue or cannot return an accepted response.
- Async continuation failure must be visible through scheduler diagnostics and projection command status.

Likely code areas:

- `internal/services/workorch/biz/backends/remote.go`
- `internal/pkg/workorchclient/decision.go`
- `internal/pkg/workorchport/governance.go`
- `internal/modules/application/workorchd/service/server/http_governance.go`
- `internal/modules/application/workorchd/service/server/workorchd_scheduler_commands.go`

Acceptance:

- Unit test: TUI `/decision replan` calls backend with `resolutionType=replan`.
- Unit test: Web/REST resolve response includes command/continuation metadata.
- Unit test: EventHub publish/send failure returns non-accepted error.
- E2E: valid replan produces command accepted plus successor/run progress or explicit failed diagnostic.

Closure evidence:

- TUI `/decision replan` submits `resolutionType=replan` with workspace/ticket scope.
- REST decision resolve publishes scoped scheduler command metadata: workspaceId, runId, decisionId, requestId, source, reason and intent.
- Scheduler command accept path requires an accepted response; missing response or queue failure returns conflict and projects rejected command status.
- Existing unit coverage:
  - `TestModelDecisionReplanSlashSubcommandUsesExplicitResolutionType`
  - `TestDecisionResolveEndpointProjectsReplanRunCommand`
  - `TestWorkorchDSchedulerCommandRequiresAcceptedResponse`
  - `TestWorkorchDGovernanceResolutionReturnsConflictWhenSchedulerQueueFails`

### P2: Live Verification Harness Update

Owner: verification scripts/e2e.

Status: closed.

Requirements:

- Extend live verification to detect:
  - blocked run with resolved/missing decision,
  - workspace waiting decision with no open decision,
  - resolved replan without successor or failed diagnostic,
  - prompt mode decision without open decision,
  - continuation completed while old resolved blocker remains.
- Verification must avoid full snapshot unless explicitly requested.
- Output compact evidence:
  - workspace ID,
  - run ID,
  - ticket ID,
  - ticket state,
  - resolution type,
  - successor/resume run ID,
  - latest run/workspace events,
  - scheduler diagnostics,
  - projection revision.

Likely code areas:

- `scripts/verify-authority-live-e2e-closure.sh`
- `docs/60-verification/authority-live-e2e-closure-verification.md`
- `internal/modules/application/workorchd/service/server/http_test.go`

Acceptance:

- Script fails on the current `ws-4e83810542ac4d65` stale resolved blocker state.
- Script passes when the stale blocker is reconciled or exposed as structured terminal/manual-retry diagnostic.
- Script distinguishes `ws-7ced70e98ea50c43` valid open decision from `ws-4e83810542ac4d65` stale resolved blocker.

Closure evidence:

- `scripts/verify-authority-live-e2e-closure.sh` now performs compact authority readback using decisions/runs list endpoints only; it does not call workspace snapshot/detail endpoints by default.
- The report includes `authorityChecks`, `authorityReadbackWorkspaceCount`, enriched `waitingDecisions`, and `authority-readback-after.json` artifacts.
- The verifier fails when it detects:
  - workspace `waiting_decision` or prompt `decision` without an open scoped decision,
  - active run `blocked_on_decision` with resolved/dismissed/missing decision,
  - resolved replan without successor evidence or failed continuation diagnostic,
  - completed continuation readback while the old resolved blocker remains.

## Non-Goals

- Do not add workspace-template-specific recovery behavior.
- Do not infer replan from arbitrary answer text after resolution persistence.
- Do not make TUI/Web repair owner state.
- Do not use projection as a hidden owner-state reconciliation layer.
- Do not replay process-local scheduler pending work after service restart.
- Do not add hardcoded handling for any live workspace ID, file name, language, command, provider, or DoD text.

## Implementation Order

1. Add tests for the stale resolved blocker state before changing owner logic.
2. Add owner invariant guard and reconciliation decision matrix.
3. Wire startup/rebuild reconciliation to durable owner state.
4. Tighten scheduler readback completion semantics.
5. Add projection inconsistency diagnostics and prompt-mode guard.
6. Align TUI/Web command metadata and display rules.
7. Extend live verification harness.
8. Run boundary search and targeted tests.

## Verification Plan

Targeted tests:

```bash
GOCACHE=/tmp/workorch-go-build go test \
  ./internal/modules/kernel/orchestrator/biz \
  ./internal/modules/kernel/decision/biz \
  ./internal/modules/application/workorchd/service/server \
  ./internal/modules/application/workorchd/biz/appservice/readmodelprojection \
  ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore \
  ./internal/services/workorch/service/tui \
  ./internal/services/workorch/biz/backends \
  ./internal/pkg/workorchport \
  ./internal/pkg/workorchclient \
  -count=1
```

Boundary search:

```bash
rg -n "ws-4e83810542ac4d65|ws-7ced70e98ea50c43|software-dev|go test|npm test|cmd/lsp-bridge|config.yaml" \
  internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg internal/services/workorch \
  -g "*.go"
```

Live checks:

```bash
curl -sS http://127.0.0.1:8080/readyz
curl -sS http://127.0.0.1:8080/api/v1/workorch/workspaces
curl -sS 'http://127.0.0.1:8080/api/v1/workorch/decisions?workspaceId=<workspaceId>'
curl -sS 'http://127.0.0.1:8080/api/v1/workorch/workspaces/<workspaceId>/runs'
```

Expected live result after closure:

- No workspace is `waiting_decision` without an open decision.
- No run is `blocked_on_decision` with a resolved/missing ticket.
- Valid `replan` creates successor progress or a failed diagnostic with reason.
- Invalid missing-info input is rejected before persistence.
- TUI prompt mode matches workorchd projection.

## Open Questions

1. For already persisted invalid `provide_missing_info` resolutions, should reconciliation close the run as `failed` or reopen a new decision ticket with an explicit `invalid_resolution_payload` reason?
2. Should scheduler diagnostics keep a fixed count per workspace/run, or a global ring plus workspace index?
3. Should `ContinuationAction` become a stored resolution field for all resolution types, or remain derived by workorchd at command time?

Until these are resolved, implementation should choose the smallest business-neutral behavior that prevents stale resolved blockers from being projected as normal waiting decisions.
