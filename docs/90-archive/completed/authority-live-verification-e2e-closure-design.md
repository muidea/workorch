# Authority Live Verification E2E Closure Design

Status: active

Type: closure-design

Last Updated: 2026-06-27

Related:

- `../../90-archive/superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `convergence-authority-owner-state-continuation-closure-plan.md`
- `projection-event-io-resource-closure-plan.md`
- `runtime-decision-eligibility-narrow-retry-closure-plan.md`

## Purpose

本文在 `Goal/DoD -> Coverage/Evidence Ledger -> Deterministic NextAction -> Stage Packet -> Contract Gate -> Evidence -> Projection/ViewModel` 长期设计基础上，固化下一阶段必须完成的 live/e2e 收口设计。

当前 P0/P1/P2 的局部代码项已有单测覆盖，但 live 观测仍显示两个 workspace 停在 `waiting_decision`，并且存在 ineffective retry、projection/detail 大对象和验证闭环未完成的问题。因此下一步不能继续只做单点补丁，必须把剩余偏差转成可复现的端到端验收合同。

本文只定义业务中立的 WorkOrch core、workorchd、projection、TUI/Web port/client 和测试验证范围。不得因为当前 workspace、模板、语言、文件名、命令文本、provider 或具体业务目标加入 core 特判。

## Contract Audit

- Scope:
  - live workspaces:
    - `ws-4e83810542ac4d65`
    - `ws-7ced70e98ea50c43`
  - workorchd scheduler continuation、decision/replan/resume、runtime contract retry、managed validation、projection/ViewModel、event IO/resource metrics、TUI/Web command/display path.
- Observed state:
  - `/readyz`:
    - service `ready`
    - projection `ready`
    - `sourceEventRevision=10879`
    - `projectedSourceRevision=10879`
    - `workspaceTotal=2`
    - watchdog timer registered once and ticking.
  - `/api/v1/workorch/workspaces`:
    - `ws-4e83810542ac4d65` is `waiting_decision`, prompt mode `decision`, active run `run-004`.
    - `ws-7ced70e98ea50c43` is also `waiting_decision`, prompt mode `decision`, active run `run-004-replan-...-r-replan-007`.
    - list ViewModel revisions are internally consistent per workspace.
  - `/metrics`:
    - `workorch_workspace_projection_rebuild_total{workspace="ws-7ced70e98ea50c43"} 14`
    - `workorch_workspace_projection_ineffective_retry_runs 2`
    - `workorch_event_record_metadata_scans_total 1104`
    - `workorch_event_record_metadata_records_scanned_total 131196`
    - `workorch_event_record_payload_hydrates_total 591`
    - `workorch_event_record_payload_hydrate_bytes_total 10545746`
  - local data:
    - `.workorch-data` is about `2.5G`
    - `.workorch-workspaces` is about `21M`
  - Code facts:
    - `runContinuationScheduler` records readback diagnostics and restart pending-work diagnostics.
    - `workspaceviewcore.Store` compacts workspace list overview and exposes projection revisions.
    - `runtime_managed_validation.go` normalizes `validation/managed_session` to bounded foreground `shell/command`.
    - projection emits authority diagnostics for stale focus/requiredOutput mismatches.
    - TUI command surface has canonical `/run resume`; no user-visible `/run continue` main path was found in the current search.
- Expected state:
  - Valid `replan` or `resume` must deterministically lead to one of:
    - successor run or refreshed current run with a dispatched step,
    - accepted evidence and next deterministic action,
    - governed wait with a newly valid decision/approval,
    - terminal failure with structured non-recoverable reason.
  - Invalid decision input must not close governance tickets.
  - LLM contract violations must be accepted as rejected diagnostic evidence, then bounded retry with a narrowed current contract; they must not execute or satisfy coverage.
  - Projection/list/prompt/status must consume compact ViewModel and same revision; detail/snapshot is explicitly on-demand.
  - Runtime resource usage must not grow due to normal projection/list/status refresh or every-event payload hydration.
- Deviation:
  - P3 live verification is still pending and not encoded as reproducible e2e tests.
  - Both live workspaces currently sit at `waiting_decision`, so latest service cannot yet be proven to move from valid replan/resume to positive owner progress.
  - `ineffective_retry_runs=2` indicates retry escalation can still end in governance without proving whether narrowed retry or managed validation was attempted and visible.
  - Metrics show high metadata scans and payload hydrates; compact list path improved, but event IO budget still lacks explicit thresholds and tests.
  - Detail snapshot endpoint can still return very large payloads, so debug/observability detail and normal UI paths need a stricter access contract.
- Owner:
  - WorkOrch core:
    - convergence authority
    - deterministic NextAction
    - Stage Packet
    - contract violation recovery
    - evidence ledger admission
    - managed validation normalization
  - workorchd:
    - EventHub command ingress
    - scheduler queue
    - continuation readback diagnostics
    - projection/ViewModel owner
    - HTTP/SSE command and read APIs
  - TUI/Web:
    - scoped command submission
    - projection display
    - prompt/input validation
  - Template/Soul/policy:
    - software-development-specific stage preference, validation preference, file-size guidance, DoD policy.
- Event path:
  - TUI/Web/CLI command -> workorchport/workorchclient DTO -> workorchd REST or embedded backend -> EventHub scoped command -> owner state -> scheduler queue -> convergence authority refresh -> dispatch/governed wait -> projection ViewModel -> TUI/Web.
  - LLM candidate -> parse/schema gate -> action-specific contract gate -> accepted or rejected evidence -> coverage ledger recompute -> NextAction -> projection.
- Contract gap:
  - Missing e2e fixtures that prove command -> owner -> authority -> dispatch/governed wait.
  - Missing live verification harness that records workspace/run/ticket/step/revision/evidence facts in a bounded format.
  - Missing resource budget gates for metadata scans, payload hydrates, projection rebuild duration and data growth.
  - Missing explicit managed validation live/request assertion that latest LLM contract includes `validation/managed_session` where validation lifecycle is needed.
  - Missing terminal/governance diagnostics that explain why repeated valid replan/resume still results in `waiting_decision`.

## Design Principles

1. Live verification must be a first-class contract, not a manual afterthought.
2. Every live/e2e assertion must record scoped IDs: workspace, run, ticket, step, command request id, projection revision and evidence refs.
3. `waiting_decision` is acceptable only when the decision is semantically necessary, newly valid, and not the result of retry drift or stale lineage.
4. Retry is bounded by current focus and current NextAction; rejected diagnostics narrow the next request but never become authority.
5. Normal UI/list/status paths must not hydrate full detail payloads.
6. Resource budgets are part of correctness for long-running multi-workspace execution.
7. Projection can expose diagnostics but cannot repair owner state.
8. Tests must fail on the previous drift class: stale focus, stale requiredOutput, closed ticket with old blocker, valid replan without owner progress, or large payload reads on list path.

## Target Verification Flow

```text
fresh service
  -> load all workspaces into projection
  -> capture baseline readyz/workspaces/metrics
  -> submit invalid decision input
  -> assert ticket remains open and projection prompt stays consistent
  -> submit valid decision replan
  -> assert command accepted
  -> assert owner readback advances or reports structured terminal/governed reason
  -> assert authority snapshot drives Stage Packet
  -> assert LLM violation becomes rejected evidence and bounded retry
  -> assert managed validation capability is visible when validation lifecycle is requested
  -> assert projection/list/status resource budget remains bounded
  -> persist verification report
```

## Required Work Items

### P0: Live Verification Harness

Owner: test/e2e, workorchd HTTP surface.

Status: code-closed.

Requirements:

- Add a deterministic verification script or Go e2e test that can run against a deployed `workorchd`.
- Capture:
  - git commit or binary version if available,
  - service start time,
  - `/readyz`,
  - `/api/v1/workorch/workspaces`,
  - `/metrics`,
  - scoped workspace/run/ticket IDs,
  - projection revisions before/after command,
  - scheduler diagnostics,
  - accepted/rejected evidence refs when available.
- Avoid full snapshot by default. Any detail read must be explicitly marked as `observability` or `debug`.
- Output a compact JSON or Markdown report under a verification artifact path.

Acceptance:

- The harness can distinguish `workspaceProgress` from final `goalConvergence`.
- The harness fails if projection is ready but source/projected revisions diverge after the allowed wait.
- The harness fails if a valid command is accepted but no owner progress, governed wait, or terminal reason is observed.

Closure evidence:

- Added `scripts/verify-authority-live-e2e-closure.sh`.
- Added `make verify-authority-live-e2e-closure`.
- Added `docs/60-verification/authority-live-e2e-closure-verification.md`.
- Script defaults to read-only `/readyz`, workspace list and `/metrics`; optional env vars enable explicit resume or decision command verification.
- The script avoids full snapshot/detail reads by default and writes compact `report.json`.

Likely code areas:

- `scripts/`
- `internal/modules/application/workorchd/service/server/**`
- `docs/60-verification/**`

### P1: Decision/Replan/Resume E2E Contract

Owner: workorchd command API, scheduler continuation, governance appservice, TUI/Web adapters.

Status: code-closed.

Requirements:

- Invalid `provide_missing_info`:
  - empty, whitespace, `/`, command prefix-only payloads are rejected.
  - decision remains open.
  - projection prompt mode remains `decision`.
- Valid `replan`:
  - resolution is persisted with scope.
  - scheduler continuation is enqueued.
  - completion is recorded only after readback proves successor run, old blocker cleared, dispatch, terminal state or governed wait.
- Valid `resume`:
  - first activation, restart-after-service-stop and decision/approval release use the same canonical intent.
  - no secondary `continue` flow exists in port/REST/projection.
- Valid `provide_missing_info`:
  - must contain non-empty semantic payload.
  - old `blockingTicketId` must be cleared or a structured reason must explain why it cannot be cleared.

Acceptance:

- Integration tests cover REST and embedded backend paths.
- TUI tests cover slash suggestions, prompt mode and command result rendering.
- Web tests or API tests cover decision resolution payload validation.
- Scheduler diagnostics cannot show `completed/no_action_needed` for the same unresolved blocker.

Closure evidence:

- `governanceports.ResolveDecisionView` rejects explicit non-semantic `provide_missing_info` payloads including empty, whitespace, `/`, and command prefix-only values before closing the ticket.
- REST decision resolve tests prove invalid missing-info payloads return `400`, write no resolution files and keep the decision ticket open.
- Existing replan HTTP tests prove valid `replan` resolution persists scoped command metadata and enqueues governance continuation.
- Existing TUI tests prove `/run resume` is the canonical run command, non-canonical run commands are rejected, and slash suggestions prefer `/run resume`.
- Target checks:
  - `go test ./internal/modules/application/workorchd/biz/appservice/governanceports -run 'ResolveDecisionView|BatchResolveDecisionViews' -count=1`
  - `go test ./internal/modules/application/workorchd/service/server -run 'DecisionResolveEndpoint|WorkorchD.*Resume|Scheduler|Replan' -count=1`
  - `go test ./internal/services/workorch/service/tui -run 'RunResume|NonCanonicalRunCommands|Decision|Slash' -count=1`

Likely code areas:

- `internal/modules/application/workorchd/service/server/http_test.go`
- `internal/modules/application/workorchd/biz/appservice/decision_service_test.go`
- `internal/services/workorch/service/tui/model_test.go`
- `web/workorch-console/src/**`

### P1: Authority Readback And Governance Diagnostics

Owner: orchestrator runtime lifecycle, scheduler diagnostics, projection.

Status: code-closed.

Requirements:

- Add a readback result structure that records:
  - old run state,
  - new run state,
  - old blocker,
  - new blocker,
  - successor run id,
  - latest step id,
  - terminal/governance state,
  - projection revision observed.
- Expose this readback outcome in scheduler diagnostics and projection diagnostics.
- When a valid replan/resume cannot progress, emit a structured reason:
  - `owner_readback_not_advanced`
  - `authority_no_dispatchable_action`
  - `governed_wait_required`
  - `terminal_failure`
  - `internal_contract_misalignment`
- Do not treat command accepted as completion.

Acceptance:

- A test with resolved decision but stale blocker fails readback completion.
- A test with successor run passes readback completion.
- A test with governed wait passes only when a new valid governance blocker exists.

Closure evidence:

- Added optional `ContinuationReadbackDiagnostic` to scheduler diagnostics DTOs.
- Scheduler continuation diagnostics now preserve readback state diff, blocker IDs and projection revisions.
- Remote backend and TUI diagnostics carry the readback structure.
- TUI status text displays readback action, advancement, reason, state diff, blocker and projection revision.

Likely code areas:

- `internal/modules/application/workorchd/service/server/http_run_continuation_scheduler.go`
- `internal/modules/application/workorchd/service/server/scheduler_diagnostics.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`

### P1: Managed Validation Contract Visibility

Owner: runtime contract renderer, managed validation normalizer, shell block adapter.

Status: code-closed.

Requirements:

- The validation requiredOutput for bounded lifecycle validation must include `validation/managed_session` as an accepted kind.
- The generated graph may declare `validation/managed_session`; runtime normalizes it to bounded foreground `shell/command`.
- The normalized node must record managed metadata:
  - original kind,
  - runtime managed,
  - auto cleanup,
  - cleanup policy,
  - scratch dir,
  - allocated port when requested,
  - validation evidence metadata.
- Live/e2e report must assert whether the latest validation retry request exposed the managed validation capability.

Acceptance:

- Unit test already covers managed validation normalization; add or retain request-rendering test proving requiredOutput exposes the accepted kind.
- Live verification can prove the deployed binary is rendering the current managed validation contract.

Closure evidence:

- `runtime_managed_validation.go` normalizes `validation/managed_session` into bounded foreground `shell/command` while preserving managed metadata, scratch dir and optional allocated port.
- `concreteValidationEvidenceRequiredOutputWithScratch` exposes `validation/managed_session` in `requiredOutput.acceptedKinds`.
- Runtime contract tests prove managed validation declaration is accepted and normalized, while unsafe file-mutating validation remains rejected.
- Shell adapter tests prove validation evidence and diagnostics remain visible in block output.
- Target checks:
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'ManagedValidation|ConcreteValidationEvidence|ValidationEvidence' -count=1`
  - `go test ./internal/modules/blocks/shell/biz -run 'Validation|Managed' -count=1`

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_managed_validation.go`
- `internal/modules/blocks/shell/biz/shell_runtime_adapter.go`

### P2: Projection And Detail IO Budget

Owner: workorchd projection/read model, HTTP APIs, TUI/Web clients.

Status: code-closed.

Requirements:

- Define three read classes:
  - `list`: compact ViewModel only, no full timeline payload hydration.
  - `detail`: bounded detail summary, large payloads by ref.
  - `observability`: explicit full debug/hydrated read.
- `/api/v1/workorch/workspaces` must remain list class.
- `/snapshot` must either become bounded by default or require an explicit `view=observability` / equivalent parameter for full hydration.
- Metrics must expose enough counters to prove list/status reads do not hydrate full payloads.
- TUI main/list/prompt must not fetch full snapshot during normal refresh.

Acceptance:

- Test: list projection omits context hints, evidence policies and large runtime payloads.
- Test: normal list/status read does not increment payload hydrate counters.
- Test: observability read can hydrate payloads and increments counters explicitly.
- Live budget for normal polling window:
  - no unbounded projection rebuild fan-out,
  - no repeated full snapshot hydration from TUI/Web,
  - metadata scans and payload hydrates remain within documented thresholds.

Closure evidence:

- `/api/v1/workorch/workspaces/:workspaceId/snapshot` is now a bounded `detail` read by default and reads from the workspace projection store.
- Explicit `?view=observability` or `?view=debug` is required for full/hydrated snapshot reads.
- Projection overlay merge for observability reads only supplements ViewModel, RunCommand and scheduler diagnostics; it does not compact or overwrite full observability payloads.
- 2026-06-27 live follow-up found the default snapshot path was bounded at the endpoint layer but still allowed large nested projection fields inside the stored detail:
  - `snapshot.step.output.contextBudget` could expose full budget/cache metadata in the normal detail response.
  - `taskDecompositions[].nodes[].input` and `taskPlans[].nodes[].input` could retain large runtime action/context maps.
  - `contextBundle.sections[].data` could retain typed Go slices/structs such as results, task graphs, decisions, execution sections and memory recall traces because the generic compaction helper only recognized JSON-like `map[string]any` / `[]any` values.
  - global projection status could report `projectedSourceRevision > sourceEventRevision` when a workspace-scoped progress event advanced an entry source revision without synchronizing the global source revision.
- The follow-up code closure tightens the projection layer itself:
  - `ProjectStepOutput` now applies bounded trace compaction to all allowed step output values.
  - `ProjectWorkspaceSnapshotDetail` projects top-level `Step`, `TaskProgress`, `TaskDecompositions` and `TaskPlans` through bounded view helpers before storing or returning default detail.
  - `compactTraceValue` now converts unknown typed structs/slices into JSON-compatible values before recursively applying the same map/slice/string limits, so context section data cannot bypass bounded projection due to static Go type.
  - `workspaceviewcore.Store` now derives global source revision from workspace entry source/projected revisions, so global status cannot lag behind a projected workspace event revision.
- Server tests prove:
  - default snapshot does not call the full snapshot reader,
  - default snapshot strips workspace context hints, evidence policies and run execution context,
  - default snapshot returns bounded projection detail rather than full workspace usage history,
  - explicit observability snapshot retains full heavy fields and full usage history.
- Read-model tests prove:
  - default workspace detail compacts top-level runtime step output and task graph node input,
  - typed context section data is converted to bounded JSON-compatible projection data,
  - task graph refs and diagnostic text are bounded,
  - global projection status source/projected revision remains internally consistent after workspace-scoped runtime progress.
- TUI and Web default clients continue to call the normal snapshot endpoint without observability/debug query parameters.
- Target checks:
  - `go test ./internal/modules/application/workorchd/service/server -run 'TestWorkspaceSnapshotEndpoint(ReturnsRelatedRunsAndBlockingItems|ReturnsCompactProjection|DefaultUsesProjectedDetailOnly|ObservabilityUsesExplicitFullRead)$' -count=1`
  - `go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -run 'Store|Manager|Snapshot|List|Compact' -count=1`
  - `go test ./internal/services/workorch/service/tui -run 'WorkspaceContext|Refresh|Prompt|Snapshot|RunResume' -count=1`

Likely code areas:

- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/**`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`
- `internal/modules/application/workorchd/service/server/http_workspace.go`
- `internal/services/workorch/service/tui/**`
- `web/workorch-console/src/**`

### P2: Persisted Request/Input Retention And Large Payload References

Owner: runtime request logging, repository/file backend, observability.

Status: partial-code-closed.

Requirements:

- Separate provider-facing request audit from normal runtime state needed for projection.
- Large `request.json`, `input.json`, tool output and snapshot bodies should be:
  - referenced by payloadRef/snapshotRef in normal state,
  - optionally compressed or retained with policy,
  - excluded from compact projection,
  - available through explicit observability/debug paths.
- Add metrics or diagnostics for persisted request/input byte growth per workspace/run.

Acceptance:

- `.workorch-data` growth from repeated runs can be attributed by category.
- Normal projection does not require reading provider request bodies.
- Retention policy can be configured without changing core business behavior.

Closure evidence:

- Runtime/LLM context compaction already projects large target snapshots and payloads by `snapshotRef` / `payloadRef` and marks omitted body content with `contentOmittedReason`.
- `GET /api/v1/workorch/runtime/payloads?ref=...` is the explicit observability path for raw runtime payload reads.
- Runtime raw content service tests cover:
  - LLM request files,
  - step and tool payload files,
  - virtual checkpoint/result detail files,
  - workspace-scoped virtual reads,
  - unsafe ref rejection.
- Event record IO metrics expose metadata scans, records scanned, payload hydrate count and payload hydrate bytes.

Remaining gap:

- Per workspace/run/category persisted byte attribution is not yet implemented as a maintained metric or diagnostics record.
- Retention policy configuration for request/input files remains a separate storage-policy item; it must not be implemented by adding request-time full directory scans to `/metrics`.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/**`
- `internal/modules/blocks/llm/**`
- `internal/pkg/repository/file/**`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`

### P2: Ineffective Retry Classification

Owner: convergence authority, contract violation recovery, projection diagnostics.

Status: code-closed.

Requirements:

- When `ineffective_retry_runs > 0`, projection and diagnostics must explain:
  - current focus,
  - violated rule signature,
  - retry budget scope,
  - latest rejected diagnostic evidence ref,
  - whether narrowed retry was attempted,
  - whether managed validation was available,
  - why governance is now required.
- `waiting_decision` caused by retry exhaustion must not look the same as a user-information decision.

Acceptance:

- Test: repeated same-rule violation records rejected diagnostics and exposes retry exhaustion reason.
- Test: decision created after retry exhaustion includes recoverability and required operator choices.
- Live verification report can classify `waiting_decision` as user-info, approval-like governance, retry exhaustion, or internal issue.

Closure evidence:

- `ContractViolation` now preserves neutral `budgetScope` diagnostics.
- `ConvergenceStatus` now exposes:
  - `decisionCause`,
  - latest rejected diagnostic ref,
  - retry attempt/max attempts/signature repeat count,
  - retry budget scope,
  - whether narrowed retry was attempted,
  - whether managed validation was available in the current contract.
- Projection classifies waiting-decision retry exhaustion as `decisionCause=retry_exhaustion` when the retry budget is exhausted.
- TUI and Web display these projection fields instead of inferring waiting-decision cause locally.
- Target checks:
  - `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'ConvergenceStatus|ContractRetry|RuntimeExecution' -count=1`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'ContractViolation|ManagedValidation|RuntimeExecutionContract' -count=1`
  - `go test ./internal/services/workorch/service/tui -run 'InspectStatus|Convergence|RuntimeStatus|RunResume' -count=1`

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`
- `internal/services/workorch/service/tui/**`
- `web/workorch-console/src/**`

## Verification Matrix

| Scenario | Expected Result | Required Evidence |
| --- | --- | --- |
| Invalid `/` decision input | rejected, ticket open | command rejected event, unchanged ticket state, projection revision |
| Valid decision `replan` | successor/dispatch/governed wait/terminal reason | resolution, scheduler diagnostic, run state diff, projection revision |
| Valid `/run resume` | canonical resume path | accepted command, no continue intent, owner readback |
| Repeated LLM violation | rejected diagnostic + narrowed retry or governed wait | violated rule, rejected evidence ref, retry budget scope |
| Managed validation needed | managed capability visible and normalizable | requiredOutput accepted kind, normalized graph metadata |
| Projection list refresh | compact and stable | list payload size, no full payload hydrate, same ViewModel revision |
| Detail observability read | explicit heavy read | observability marker, hydrate counters |
| Multi-workspace concurrent wait | isolated state and scope | separate workspace/run/ticket ids, no cross-workspace blocker leakage |

## Implementation Sequence

1. Add live verification harness and compact report format.
2. Add REST/embedded/TUI tests for invalid decision, valid replan and canonical resume.
3. Extend scheduler readback diagnostics with state-diff details.
4. Add managed validation request-rendering assertion.
5. Add projection IO read-class contract and tests.
6. Add ineffective retry diagnostics and classification.
7. Add retention/large payload reference design follow-up if IO metrics still exceed budget.
8. Run boundary search and targeted tests.
9. Run fresh deployment live verification and attach report.

## Boundary Rules

- Core may use generic concepts such as evidence kind, capability kind, validation session, retry signature, NextAction and projection revision.
- Core must not branch on `software-dev`, programming language, file names, command names, provider names, workspace title or live workspace ID.
- Software-development guidance belongs to workspaceTemplate/profile/Soul/skill.
- Web/TUI may display template/profile names returned by projection but cannot infer runtime state from them.
- Diagnostics can mention observed IDs in reports, but production logic cannot encode those IDs.

## Done Criteria

- P3 live verification from `convergence-authority-owner-state-continuation-closure-plan.md` is executable and passing.
- A valid replan/resume from `waiting_decision` either advances owner state or produces a structured, projection-visible reason.
- `waiting_decision` is classified by cause, not shown as an undifferentiated blocker.
- Managed validation capability is visible in current request contracts and validated by live/e2e evidence.
- Normal workspace list/status/TUI prompt paths do not hydrate full event payloads or full snapshot details.
- Resource metrics have documented budgets and do not show unbounded growth during normal observation.
- Projection diagnostics expose authority mismatch without repairing owner state.
- All changes remain business-neutral and pass boundary search.
