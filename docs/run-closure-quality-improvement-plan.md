# workorch Run Closure Quality Improvement Plan

## Purpose

This document tracks the next focused improvement round for `workorch`: make the
Goal -> Run -> DoD acceptance -> Result / Artifact closure loop stable,
efficient, and easy to verify.

The scope is not to add broad new business domains. The scope is to improve the
quality of the current orchestration loop after issues observed in real
workspace runs:

- Workspace state, task progress, Web outcome, and run observability can diverge
  when multiple historical runs exist.
- Historical failed repair steps can incorrectly dominate the final acceptance
  view even after a later accepted Result exists.
- Resume / repair / decision recovery must not leave a Run hanging in an
  unrecoverable `running` state.
- Final deliverables need stronger structure than a generic summary.

## Current Baseline

The current implementation already supports:

- Workspace, Goal, DoD, Soul, Skill, MCP, Memory, Run, Decision, Approval,
  ToolCall, Patch, ValidationResult, RuntimeCheckpoint, Result, and Artifact
  models.
- LLM-driven Workspace execution through `llm/workspace_execution`.
- Evidence-backed DoD assessment gate before accepted Result creation.
- Result / Artifact read models in Run snapshot and Workspace snapshot.
- Web Console views for business overview, workspace detail, decision queue,
  runtime evidence, and final outcome.
- Runtime recovery for Decision / Approval, repair attempts, and graph
  follow-up.

This improvement round treats the above as the product baseline and focuses on
closure quality.

## Principles

1. Final business success is decided by DoD-backed accepted Result, not by the
   absence of historical failed intermediate steps.
2. Historical failed Step / ToolCall / ValidationResult records remain evidence,
   but they must not override the final accepted outcome.
3. A completed Run without accepted Result is not a validated delivery.
4. Workspace, Run, TaskProgress, Observability, and Web must use one consistent
   outcome projection.
5. Decision / Approval resolution must either enqueue a real continuation or
   transition the Run to a clear terminal failure.
6. Kernel / orchestrator must stay business-generic and must not parse concrete
   workspace goal text to decide behavior.

## Improvement Areas

### Q1. Unified Outcome Projection

Goal: eliminate inconsistent state views across API and Web.

Planned work:

- Introduce a shared `RunOutcome` projection for:
  - `created`
  - `running`
  - `blocked_on_decision`
  - `blocked_on_approval`
  - `validating`
  - `accepted`
  - `failed`
  - `cancelled`
  - `superseded`
- Introduce a `WorkspaceOutcome` projection derived from active Run, accepted
  Result, and latest meaningful Run.
- Use the same projection in:
  - Workspace list
  - Workspace snapshot
  - Workspace task progress
  - Run task progress
  - Run observability
  - Web business overview
  - Web workspace detail
- Keep raw Run state visible for diagnosis, but make business-facing state use
  the projection.

Acceptance criteria:

- A workspace with a latest failed historical Run and a newer or selected
  accepted Result does not show final outcome as failed.
- A Run with accepted Result shows final business outcome as accepted even if
  historical repair steps failed before recovery.
- A completed Run without accepted Result is clearly displayed as incomplete or
  not accepted.
- Unit tests cover active Run, latest failed Run, accepted historical Result,
  and no-Result completed Run combinations.

### Q2. Strict DoD Acceptance

Goal: make Goal acceptance explicitly traceable to DoD items and evidence.

Planned work:

- Normalize DoD assessment into a stable read model:
  - item text
  - item state
  - evidence refs
  - failure reason
  - optional confidence / source notes
- Require every satisfied DoD item to reference evidence.
- Require final accepted Result to include:
  - Goal ID and summary
  - DoD ID and item-level result
  - source Step
  - evidence refs
  - Artifact refs
  - residual risks
- Add validation that accepted Result cannot be created from incomplete DoD
  assessment.
- Make Web show DoD item status from formal Result / assessment instead of
  relying only on raw Step output.

Acceptance criteria:

- Missing DoD assessment prevents accepted Result creation.
- Satisfied DoD item without evidence refs is rejected.
- Web shows each DoD item as satisfied / failed / blocked with evidence refs.
- E2E scripts verify accepted Result, Artifact, DoD items, and evidence refs.

### Q3. Runtime Recovery Stability

Goal: avoid stuck or ambiguous Runs during repair, Decision, Approval, and
resume.

Planned work:

- Promote recovery point to a first-class checkpoint field:
  - current stage
  - current step
  - continuation action
  - blocker ID
  - failure class
  - retry signature
- Ensure Decision / Approval resolution performs one of:
  - enqueue continuation
  - mark blocked if still waiting
  - fail / supersede / reject with explicit reason
- Add stuck-run guard:
  - `running` with no queue item, no in-flight execution, no blocker, and no
    continuation checkpoint must transition to recoverable inspection or failed
    state.
- Add failure classification:
  - provider unavailable
  - provider schema drift
  - provider timeout
  - workspace path conflict
  - shell command failed
  - patch apply failed
  - MCP capability denied
  - MCP runtime failed
- Add duplicate failure signature handling to avoid repeating the same repair
  loop.

Acceptance criteria:

- Resolving a Decision never leaves a Run permanently stuck in `running`.
- Repeated identical failures enter Decision instead of repeating ineffective
  repair attempts.
- Stuck-run guard has unit tests and deployed-instance verification.
- Recovery state is visible in Run task progress and observability.

### Q4. Execution Efficiency

Goal: reduce unnecessary LLM calls, repeated failed actions, and serial waiting.

Planned work:

- Provide stronger execution context before LLM planning:
  - workspace file manifest
  - current deliverables
  - latest accepted Result
  - recent failure summary
  - current MCP capability catalog
  - selected Skill summary
- Use failure signatures to suppress repeated invalid plans.
- Keep ready-node graph dispatch concurrent, but add clearer per-node timeout
  and cancellation behavior.
- Cache stable read-only context within a Run where safe:
  - Goal / DoD / Soul content
  - Skill definitions
  - MCP capability metadata
  - Memory retrieval result
- Avoid full snapshot scans where a Run-scoped read is enough.

Acceptance criteria:

- Repeated path-conflict or same shell-error repair loops are reduced.
- Concurrent graph execution still produces deterministic final Result.
- Load tests cover multi-run, multi-decision, and approval paths with stable
  queue and in-flight metrics.
- File backend performance remains acceptable under configured concurrent Runs.

### Q5. Process Completeness

Goal: make task decomposition, execution, governance, validation, and summary
read as one complete process.

Planned work:

- Add or formalize `WorkPlan` / `ExecutionSection` projection:
  - section ID
  - title
  - related DoD items
  - planned steps
  - actual evidence refs
  - state
- Link execution graph nodes to WorkPlan sections where possible.
- Keep Decision and Approval connected to the section or step they unblock.
- Make task progress milestones reflect:
  - planning
  - execution
  - governance
  - validation
  - artifact generation
  - final acceptance

Acceptance criteria:

- Web can show which section each failure, Decision, Approval, ToolCall, Patch,
  and Result belongs to.
- A completed workspace can be reviewed from plan to final Artifact without
  reading raw JSON.
- E2E verification can assert planned section count, completed section count,
  and DoD coverage.

### Q6. Result And Artifact Summary Quality

Goal: make final deliverables complete enough for user acceptance and later
integration.

Planned work:

- Expand Result summary schema:
  - goal outcome
  - DoD item table
  - implementation summary
  - verification summary
  - evidence refs
  - artifact refs
  - known limitations
  - residual risks
  - next actions
- Expand Artifact manifest categories:
  - source
  - configuration
  - documentation
  - test report
  - runtime evidence
  - final summary
- Store Artifact content with internal `workorch://` URI only; do not expose
  local absolute paths.
- Ensure final summary uses the configured dialog language when available.

Acceptance criteria:

- Artifact manifest is not just a file list; it explains file purpose and
  relation to DoD.
- Result summary clearly states what is accepted, what was verified, and what
  remains risky.
- Web target outcome view can show final Result and Artifact without opening raw
  runtime evidence.

## Work Breakdown

| ID | Item | Priority | Status | Verification |
| --- | --- | --- | --- | --- |
| QC1 | Shared `RunOutcome` / `WorkspaceOutcome` projection | P0 | implemented | Unit tests plus workspace with mixed successful and failed Runs |
| QC2 | Accepted Result overrides historical repair failures in business progress | P0 | implemented | Unit tests and Web workspace detail check |
| QC3 | Completed Run without accepted Result is not displayed as accepted delivery | P0 | implemented | Read-model tests |
| QC4 | DoD item-level accepted Result schema | P0 | implemented | Unit tests, Web snapshot display, pending deployed real LLM e2e |
| QC5 | Decision / Approval resolution always enqueues or terminates | P0 | implemented | Unit tests, service endpoint tests, pending deployed multi-round ticket / approval e2e |
| QC6 | Stuck-run guard for no in-flight running Runs | P1 | implemented | Appservice recovery test and HTTP startup validation test |
| QC7 | Failure classification and duplicate failure signatures | P1 | implemented | Runtime adapter and orchestrator tests |
| QC8 | Stronger LLM execution context package | P1 | implemented | Orchestrator context package tests; pending deployed real workspace repair regression |
| QC9 | WorkPlan / ExecutionSection projection | P1 | implemented | Web build and API snapshot test |
| QC10 | Rich Result / Artifact summary schema | P1 | implemented | Orchestrator, read-model, LLM adapter tests and Web build |
| QC11 | Performance regression scenario for concurrent governance Runs | P1 | implemented | Governed load test controller with pass gate, failure classes, Result/Artifact integrity assertions |
| QC12 | Single-workspace LLM step persistence reduction | P1 | implemented | Orchestrator unit test verifies context injection without redundant intermediate Step update |
| QC13 | Single-workspace graph scheduling read reuse | P1 | implemented | Orchestrator unit test verifies graph follow-up evidence reuses loaded Step list |
| QC14 | LLM repair context manifest reuse | P1 | implemented | Orchestrator unit test verifies execution context package reuses input workspace manifest |
| QC15 | Quantified single-workspace wait latency measurement | P1 | implemented | Governed load test supports configurable status polling interval and records it in reports |
| QC16 | Run status short server-side wait | P1 | implemented | HTTP status endpoint test verifies `waitState` blocks until target state |
| QC17 | Run status wait notification wake-up | P1 | implemented | HTTP service test verifies status wait is woken by runtime status change notification before poll interval |
| QC18 | Accepted Run terminal closeout repository read reuse | P1 | implemented | Orchestrator tests verify validating/completed state transitions remain intact; deployed quantification pending next rebuild |
| QC19 | Accepted Run checkpoint StepID reuse | P1 | implemented | Orchestrator tests verify run closeout still emits validating/completed evidence; deployed quantification pending next rebuild |
| QC20 | Terminal Run snapshot read-model reuse | P1 | implemented | HTTP service test verifies terminal snapshot/status/observability share one cached snapshot and cache invalidates on run status notification |
| QC21 | Workspace snapshot projection RunSnapshot reuse | P1 | implemented | Appservice test verifies workspace projection sections can reuse provided RunSnapshot; deployed batch `load-20260501061918` keeps 5/5 success |
| QC22 | Run / Workspace read-model repository reads parallelization | P1 | implemented | Appservice / server tests pass; deployed batch `load-20260501062554` keeps 5/5 success |
| QC23 | Run status read-model repository reads parallelization | P1 | implemented | Appservice / server / full tests pass; deployed batch `load-20260501063222` keeps 5/5 success |
| QC24 | LLM execution context workspace read reuse | P1 | implemented | Orchestrator test verifies provided workspace is reused without repository Get; deployed batch `load-20260501063946` keeps 5/5 success |
| QC25 | Result deliverable manifest workspace read reuse | P1 | implemented | Orchestrator test verifies Result manifest reuses provided workspace without repository Get; deployed batch `load-20260501065532` keeps 5/5 success |
| QC26 | Artifact content RunSnapshot artifact metadata reuse | P1 | implemented | Appservice / server / full tests pass; deployed batch `load-20260501070159` keeps 5/5 success |
| QC27 | Run state transition loaded Workspace reuse | P1 | implemented | Orchestrator tests verify Decision / Approval / completion state transitions reuse provided Workspace without repository Get; deployed batch `load-20260501070703` keeps 5/5 success |
| QC28 | StartRun loaded Run dispatch reuse | P1 | implemented | Orchestrator test verifies StartRun dispatch avoids a second Run repository Get; deployed quantification pending next rebuild |

## Recommended Delivery Order

1. Complete `RunOutcome` / `WorkspaceOutcome` and replace ad-hoc projection
   logic across read model and Web.
2. Tighten DoD item-level Result validation and expose it in Web.
3. Harden recovery checkpoint and stuck-run handling.
4. Add failure classification and duplicate-failure suppression.
5. Improve WorkPlan / section projection and Web process visualization.
6. Expand Result / Artifact summary quality.
7. Re-run full local MVP, real LLM, governance, weather, MCP, memory, and
   concurrency verification.

## Verification Matrix

| Scenario | Required check |
| --- | --- |
| Completed accepted Run after repair failures | Workspace outcome accepted; task progress final result completed; historical failed steps remain visible |
| Latest failed Run after accepted historical Result | Web must clearly show selected/projection Run and not silently hide latest failure |
| Completed Run without Result | Workspace not accepted; final result milestone failed or incomplete |
| Multi-round Decision | Ticket resolution resumes the correct recovery point and produces final Result |
| Approval gate | Approval completion resumes in background and does not block Web |
| LLM provider timeout | Run fails with provider timeout class and no fake Result |
| Path conflict | LLM receives manifest and conflict diagnostics; repeated same conflict enters Decision |
| Concurrent governance Runs | Queue metrics stable; every Run terminal state matches Result / failure evidence |
| Artifact download | `workorch://` URI resolves through API and does not leak absolute path |

## Non-Goals

- Adding unrelated new business modules.
- Letting kernel/orchestrator parse concrete Goal text for business-specific
  behavior.
- Treating ToolCall, Patch, or ValidationResult as a replacement for formal
  Result / Artifact.
- Hiding failed historical evidence after a successful repair.
- Using CI to call real external LLM providers.

## Tracking Notes

- This document is the active quality-improvement tracker for the next closure
  round.
- 2026-04-30: Implemented `RunOutcome` / `WorkspaceOutcome` read-model
  projection for Run snapshots, Run observability, Workspace snapshots, and Web
  business-facing status display. Raw Run state remains visible for diagnosis.
- 2026-04-30: Added read-model tests for accepted Result overriding historical
  repair failures, stale terminal active Run projection, latest failed Run
  projection, and completed Run without accepted Result.
- 2026-04-30: Added formal DoD assessment fields to accepted Result, persisted
  item-level state and evidence refs, exposed the structure through Run
  snapshot read models, and updated Web target outcome display to prefer formal
  Result DoD assessment over raw Step output.
- 2026-04-30: Hardened Decision / Approval resolution closure. Governance
  resolution endpoints now schedule server-side background Run resume; Web no
  longer performs a separate resume call after Decision or Approval actions.
  Resume failures after entering `running` now transition the Run to explicit
  `failed` terminal state and clear blockers instead of leaving ambiguous
  `running` or relying on a repeated manual resume.
- 2026-04-30: Added service-level stuck-running guard. On HTTP service startup
  validation, persisted `running` Runs with no blocker and no continuation
  recovery checkpoint are moved to explicit `failed` state with evidence via
  the Run service. Runs with a recovery continuation checkpoint are left
  untouched for later recovery.
- 2026-04-30: Added generic runtime failure classification and duplicate
  failure signatures. Failed Step output, runtime checkpoints, repair input,
  and task recovery projection now carry `failureClass` and
  `failureSignature`; repeated repairable runtime failures are converted into a
  Decision Ticket instead of replaying the same ineffective repair loop.
- 2026-04-30: Added a formal LLM `executionContextPackage` assembled by the
  orchestrator before `llm/workspace_execution` steps. The package includes the
  workspace file manifest, latest accepted Result, current deliverables,
  selected Skill / MCP / Memory context, governance history, and recent failure
  summaries without requiring the LLM block to read repositories directly.
- 2026-04-30: Added formal `ExecutionSection` read-model projection for Run,
  Workspace, and observability snapshots. Execution sections link graph nodes
  to Step, ToolCall, Patch, ValidationResult, Decision, Approval, Result, and
  Artifact evidence, and the Web console now exposes these sections as a
  process visualization distinct from the raw task decomposition graph.
- 2026-04-30: Expanded accepted Result with structured `SummaryDetail`
  containing goal outcome, DoD item table, implementation summary,
  verification summary, evidence refs, Artifact refs, limitations, residual
  risks, and next actions. Artifact records now carry formal categories, the
  generated final summary uses the configured dialog language where available,
  and the workspace deliverable manifest groups files by category with DoD
  relation and evidence refs without exposing local absolute paths.
- 2026-04-30: Hardened the governed workspace concurrency load test controller
  for performance regression use. Reports now include success-rate pass gates,
  HTTP / curl failure totals, failure class totals, and per-workspace checks
  for formal Result `summaryDetail`, Artifact categories, `workorch://` URI
  discipline, downloadable final summary content, Result / Artifact linkage,
  governance records, MCP evidence, checkpoints, and timeline `runState`.
- 2026-05-01: Reduced single-workspace LLM stage persistence overhead. The
  orchestrator now injects `executionContextPackage` into the in-memory Step
  input before runtime execution and relies on the normal terminal Step update
  to persist it, avoiding one redundant Step repository write per LLM stage
  without changing runtime input, state transitions, checkpoints, DoD
  validation, Result, or Artifact behavior. The same pass also reuses the
  workspace working directory already loaded for runtime dispatch when applying
  runtime patches, avoiding an extra workspace repository read on patch-heavy
  steps.
- 2026-05-01: Added lifecycle tracking for HTTP governance resume background
  tasks so server shutdown and HTTP endpoint tests wait for asynchronous
  Decision / Approval continuation work before closing shared resources or
  removing file-backend temporary workspaces.
- 2026-05-01: Reduced single-workspace graph scheduling read amplification.
  Ready-node calculation and graph follow-up evidence generation now reuse the
  Step list already loaded in the same scheduling round. This avoids repeated
  `ListByRun` calls while preserving graph dependency, branch, evidence, DoD
  gate, Result, and Artifact semantics.
- 2026-05-01: Reused `workspaceFileManifest` already present in LLM repair or
  retry Step input when building `executionContextPackage`, avoiding a second
  workspace directory scan for the same repair context.
- 2026-05-01: Added quantified single-workspace measurement controls to the
  governed load test. `WORKORCH_LOAD_POLL_INTERVAL_SECONDS` is now recorded in
  the report, allowing test-controller wait latency to be separated from
  workorch runtime latency. Against deployed `http://127.0.0.1:8080`, mock
  `concurrency=1,total=5` measured:

| Batch | Poll interval | Status wait | Success | e2e p95 | Throughput | decision1->decision2 p95 | decision2->approval p95 | approval->completed p95 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `load-20260501052418` | 1.00s | off | 5/5 | 3433.33ms | 19.82 workspace/min | 1030.53ms | 1038.12ms | 1033.24ms |
| `load-20260501052615` | 0.10s | off | 5/5 | 745.65ms | 84.65 workspace/min | 127.81ms | 151.86ms | 134.40ms |
| `load-20260501052658` | 0.05s | off | 5/5 | 647.53ms | 96.87 workspace/min | 86.80ms | 92.69ms | 163.54ms |
| `load-20260501053641` | 0.05s | 2.00s | 5/5 | 556.24ms | 117.62 workspace/min | 64.95ms | 65.19ms | 116.83ms |
| `load-20260501054445` | 0.05s | 2.00s + notify | 5/5 | 405.28ms | 150.18 workspace/min | 21.99ms | 25.52ms | 100.96ms |
| `load-20260501055018` | 0.05s | 2.00s + notify | 5/5 | 493.58ms | 129.79 workspace/min | 25.09ms | 31.91ms | 119.59ms |
| `load-20260501055646` | 0.05s | 2.00s + notify + closeout reuse | 5/5 | 509.27ms | 130.06 workspace/min | 18.92ms | 30.89ms | 131.40ms |
| `load-20260501060145` | 0.05s | 2.00s + notify + closeout/checkpoint reuse | 5/5 | 459.33ms | 136.44 workspace/min | 25.89ms | 24.45ms | 116.39ms |
| `load-20260501061155` | 0.05s | 2.00s + notify + terminal snapshot cache | 5/5 | 497.39ms | 130.13 workspace/min | 28.56ms | 33.09ms | 111.78ms |
| `load-20260501061918` | 0.05s | 2.00s + notify + terminal/workspace snapshot cache | 5/5 | 461.08ms | 135.20 workspace/min | 24.97ms | 33.76ms | 116.38ms |
| `load-20260501062554` | 0.05s | 2.00s + notify + read-model parallel reads | 5/5 | 436.56ms | 140.17 workspace/min | 22.59ms | 27.60ms | 103.37ms |
| `load-20260501063222` | 0.05s | 2.00s + notify + status read parallel reads | 5/5 | 491.98ms | 136.18 workspace/min | 23.92ms | 31.00ms | 110.44ms |
| `load-20260501063946` | 0.05s | 2.00s + notify + LLM context workspace reuse | 5/5 | 440.52ms | 144.21 workspace/min | 17.47ms | 26.30ms | 97.84ms |
| `load-20260501065532` | 0.05s | 2.00s + notify + result manifest workspace reuse | 5/5 | 434.15ms | 139.60 workspace/min | 25.73ms | 31.63ms | 101.95ms |
| `load-20260501070159` | 0.05s | 2.00s + notify + artifact metadata snapshot reuse | 5/5 | 438.55ms | 140.02 workspace/min | 20.56ms | 27.05ms | 94.23ms |
| `load-20260501070703` | 0.05s | 2.00s + notify + state transition workspace reuse | 5/5 | 453.07ms | 140.05 workspace/min | 23.43ms | 29.56ms | 113.99ms |

  The 1s poll baseline overstated single-workspace e2e latency by roughly
  2.7-2.8s. The 0.05s measurement is the current service-side working baseline
  for mock governed multi-round Decision + Approval closure.
- 2026-05-01: Added generic Run status short wait support. Clients may call
  `/api/v1/workorch/runs/{runId}/status?waitState=<state>&waitTimeoutMillis=<ms>&waitIntervalMillis=<ms>`
  to reduce external polling round trips while preserving the same Run status
  response contract. The governed load test can use this through
  `WORKORCH_LOAD_STATUS_WAIT_TIMEOUT_SECONDS`. Against deployed
  `http://127.0.0.1:8080`, mock `concurrency=1,total=5`, `poll=0.05s`, and
  `statusWait=2s` reduced e2e p95 from `647.53ms` to `556.24ms` and improved
  throughput from `96.87` to `117.62` workspace/min while preserving 5/5
  success, formal Result / Artifact checks, governance records, MCP evidence,
  checkpoints, and timeline `runState`.
- 2026-05-01: Added HTTP service-level Run status wait notification wake-up.
  Status wait still reads persisted Run status as the source of truth, but
  workspace activation, direct start/resume, and governance background resume
  completion now broadcast a generic notification to wake pending status waits
  before their next polling interval. This is intentionally generic and does
  not parse Goal / DoD content or change Run state-machine semantics.
  After rebuild/redeploy, governed mock `concurrency=1,total=5` measured e2e
  p95 `405.28ms` and throughput `150.18` workspace/min, with 5/5 success and
  no HTTP / curl / LLM protocol / timeline runState errors. Compared with the
  status-wait-only baseline, e2e p95 improved by `150.96ms` and throughput
  improved by `32.56` workspace/min. The remaining largest mandatory runtime
  segment is `approval->completed` p95 `100.96ms`, which includes approval
  continuation, graph tool execution, completion LLM follow-up, DoD assessment,
  formal Result / Artifact materialization, validation, and terminal state
  persistence.
- 2026-05-01: Re-ran the deployed notification-wake baseline before the next
  closeout optimization. Batch `load-20260501055018` kept 5/5 success with no
  HTTP / curl / LLM protocol / timeline runState errors, measured e2e p95
  `493.58ms`, throughput `129.79` workspace/min, and
  `approval->completed` p95 `119.59ms`. The variance confirms that the next
  optimization should be tracked against the same governed mock scenario rather
  than a single previous best run.
- 2026-05-01: Added accepted Run terminal closeout read reuse in the
  orchestrator. The internal DoD-accepted path now performs the formal
  `validating -> completed` transitions in one closeout helper and reuses the
  already loaded Run plus Workspace between the two transitions, avoiding the
  previous extra Run reload, extra Workspace reload, and repeated terminal
  release check. The public `ValidateRun` and `CompleteRun` APIs are unchanged,
  and validating / completed run events, workspace projection events, runtime
  checkpoints, Result, and Artifact semantics are preserved. Deployed
  quantification is pending the next rebuild/redeploy.
- 2026-05-01: Quantified the deployed terminal closeout read-reuse change after
  rebuild/redeploy. Batch `load-20260501055646` kept 5/5 success with no HTTP /
  curl / LLM protocol / timeline runState errors, measured e2e p95 `509.27ms`,
  throughput `130.06` workspace/min, and `approval->completed` p95 `131.40ms`.
  This did not show a stable deployed improvement against `load-20260501055018`
  (`493.58ms` e2e p95, `119.59ms` approval->completed p95), so the saved
  repository reads are not the dominant runtime cost for this scenario.
- 2026-05-01: Added accepted Run checkpoint StepID reuse. The DoD-accepted
  internal closeout path now passes the known terminal Step ID into the
  validating / completed run-state checkpoint writer, avoiding two additional
  `ListByRun` scans that were previously used only to rediscover the latest
  Step. The public run-state event contract, runtime checkpoint records,
  Result, Artifact, and Workspace projection behavior remain unchanged.
  Deployed quantification is pending the next rebuild/redeploy.
- 2026-05-01: Quantified the deployed checkpoint StepID reuse change after
  rebuild/redeploy. Batch `load-20260501060145` kept 5/5 success with no HTTP /
  curl / LLM protocol / timeline runState errors, measured e2e p95 `459.33ms`,
  throughput `136.44` workspace/min, and `approval->completed` p95 `116.39ms`.
  Compared with `load-20260501055646`, e2e p95 improved by `49.94ms` and
  `approval->completed` p95 improved by `15.01ms`, but observed variance still
  requires continued tracking across identical governed mock runs.
- 2026-05-01: Added terminal Run snapshot read-model reuse in the HTTP service.
  Completed / failed / cancelled / superseded Run snapshots are cached for a
  short TTL and invalidated by run status notifications. Snapshot, status, and
  observability views for the same terminal Run can reuse the same formal read
  model instead of rebuilding it repeatedly during immediate validation reads.
  This is service-read optimization only; repository data, Run state machine,
  Result, Artifact, and observability contracts are unchanged. Deployed
  quantification is pending the next rebuild/redeploy.
- 2026-05-01: Quantified the deployed terminal Run snapshot cache after
  rebuild/redeploy. Batch `load-20260501061155` kept 5/5 success with no HTTP /
  curl / LLM protocol / timeline runState errors, measured e2e p95 `497.39ms`,
  throughput `130.13` workspace/min, `approval->completed` p95 `111.78ms`,
  `snapshot_read` p95 `53.43ms`, and `observability_read` p95 `30.25ms`.
  This did not produce a stable end-to-end improvement because the validation
  chain still performs an independent `workspace_snapshot` projection that
  rebuilds the same selected Run read model.
- 2026-05-01: Added Workspace snapshot projection RunSnapshot reuse. The
  appservice workspace snapshot builder now accepts an optional cached
  RunSnapshot for its selected projection Run, and the HTTP service passes the
  terminal Run snapshot cache into workspace snapshot reads when available.
  This targets the validation read chain after a Run completes, where
  `run/snapshot` is immediately followed by `workspace/snapshot`. Repository
  data, Run state machine, Workspace outcome semantics, Result, Artifact, and
  read API contracts are unchanged.
- 2026-05-01: Quantified the deployed Workspace snapshot projection cache after
  rebuild/redeploy. Batch `load-20260501061918` kept 5/5 success with no HTTP /
  curl / LLM protocol / timeline runState errors, measured e2e p95 `461.08ms`,
  throughput `135.20` workspace/min, `approval->completed` p95 `116.38ms`,
  `snapshot_read` p95 `51.53ms`, `workspace_snapshot_read` p95 `28.87ms`, and
  `observability_read` p95 `27.65ms`. Compared with
  `load-20260501061155`, e2e p95 improved by `36.31ms`, throughput improved by
  `5.07` workspace/min, and `workspace_snapshot_read` p95 improved by
  `4.42ms`; the core `approval->completed` path remains the largest runtime
  segment and requires continued generic optimization.
- 2026-05-01: Added Run / Workspace read-model repository read
  parallelization. Independent repository reads inside `RunSnapshot` and
  `WorkspaceSnapshot` are executed concurrently, then projected through the
  existing read-model mapping. This is limited to application read-model
  construction and does not change repository data, orchestration state,
  Decision / Approval semantics, DoD validation, Result / Artifact contracts,
  or business Goal interpretation.
- 2026-05-01: Quantified deployed Run / Workspace read-model parallel reads.
  Batch `load-20260501062554` kept 5/5 success with no HTTP / curl / LLM
  protocol / timeline runState errors, measured e2e p95 `436.56ms`,
  throughput `140.17` workspace/min, `approval->completed` p95 `103.37ms`,
  `snapshot_read` p95 `54.82ms`, `workspace_snapshot_read` p95 `28.90ms`, and
  `observability_read` p95 `22.29ms`. Compared with
  `load-20260501061918`, e2e p95 improved by `24.52ms`, throughput improved by
  `4.97` workspace/min, and `approval->completed` p95 improved by `13.01ms`.
  `snapshot_read` p95 still shows variance from local file backend and client
  curl overhead, so it remains a tracked metric rather than a guaranteed
  monotonic per-batch improvement.
- 2026-05-01: Added Run status read-model repository read parallelization.
  `RunStatus` now reads steps, Decision Tickets, and Approvals concurrently
  before applying the same status projection. This targets status waits used by
  multi-round Decision / Approval flow closure and keeps the status API
  response contract unchanged.
- 2026-05-01: Quantified deployed Run status read-model parallel reads. Batch
  `load-20260501063222` kept 5/5 success with no HTTP / curl / LLM protocol /
  timeline runState errors, measured e2e p95 `491.98ms`, throughput `136.18`
  workspace/min, `approval->completed` p95 `110.44ms`, `snapshot_read` p95
  `44.50ms`, `workspace_snapshot_read` p95 `23.59ms`, and
  `observability_read` p95 `26.49ms`. Compared with
  `load-20260501062554`, final validation reads improved in some segments, but
  e2e p95 regressed because the first sample had slower workspace / goal /
  activation path latency. Treat this batch as a correctness-preserving
  measurement with local file backend variance, not as a stable throughput
  improvement.
- 2026-05-01: Added LLM execution context workspace read reuse. `dispatchStep`
  already needs the Workspace for runtime execution, so the same loaded
  Workspace is now passed into LLM execution context package construction to
  avoid a second Workspace repository read while building the workspace file
  manifest. This is an orchestrator efficiency change only; Soul, Skill, MCP,
  Decision / Approval, DoD validation, Result, and Artifact semantics are
  unchanged.
- 2026-05-01: Quantified deployed LLM execution context workspace read reuse.
  Batch `load-20260501063946` kept 5/5 success with no HTTP / curl / LLM
  protocol / timeline runState errors, measured e2e p95 `440.52ms`,
  throughput `144.21` workspace/min, `approval->completed` p95 `97.84ms`,
  `snapshot_read` p95 `47.79ms`, `workspace_snapshot_read` p95 `25.52ms`, and
  `observability_read` p95 `25.27ms`. Compared with
  `load-20260501063222`, e2e p95 improved by `51.46ms`, throughput improved by
  `8.03` workspace/min, and `approval->completed` p95 improved by `12.60ms`.
  This confirms the optimization removed useful repeated Workspace reads from
  the LLM execution path, while local file backend and curl overhead still
  create per-batch variance in read-only validation stages.
- 2026-05-01: Added Result deliverable manifest workspace read reuse. The final
  Result / Artifact generation path can reuse the Workspace already loaded by
  `dispatchStep` when building the deliverable manifest, instead of issuing an
  additional Workspace repository read. The graph-batch completion path keeps
  the existing fallback read because it does not currently hold a loaded
  Workspace. Result / Artifact schemas, DoD validation, event publication, and
  workspace file manifest content are unchanged.
- 2026-05-01: Quantified deployed Result deliverable manifest workspace read
  reuse after rebuild/redeploy. Batch `load-20260501065532` kept 5/5 success
  with no HTTP / curl / LLM protocol / timeline runState errors, measured e2e
  p95 `434.15ms`, throughput `139.60` workspace/min,
  `approval->completed` p95 `101.95ms`, `snapshot_read` p95 `49.13ms`,
  `workspace_snapshot_read` p95 `31.37ms`, `observability_read` p95
  `21.37ms`, and `artifact_content_read` p95 `12.73ms`. Compared with
  `load-20260501063946`, e2e p95 improved by `6.37ms` and correctness stayed
  stable; throughput variance remains dominated by local file backend and
  client-side curl scheduling, so the improvement is tracked as useful but not
  a guaranteed monotonic gain.
- 2026-05-01: Added Artifact content RunSnapshot artifact metadata reuse. The
  artifact content HTTP endpoint now first checks the terminal RunSnapshot
  cache by artifact URI and, when available, passes that snapshot into the
  application read service so artifact metadata can be reused instead of
  listing artifacts by Run again. File content is still read from the file
  backend path resolved from the `workorch://workspaces/{workspace}/artifacts`
  URI, so repository data, Result / Artifact schemas, DoD validation, evidence,
  and permission boundaries are unchanged. Local verification passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server -count=1`
  and `go test ./... -count=1 -p 1`.
- 2026-05-01: Quantified deployed Artifact content RunSnapshot artifact
  metadata reuse after rebuild/redeploy. Batch `load-20260501070159` kept 5/5
  success with no HTTP / curl / LLM protocol / timeline runState errors,
  measured e2e p95 `438.55ms`, throughput `140.02` workspace/min,
  `approval->completed` p95 `94.23ms`, `snapshot_read` p95 `38.91ms`,
  `workspace_snapshot_read` p95 `31.53ms`, `observability_read` p95
  `20.13ms`, and `artifact_content_read` p95 `19.92ms`. Compared with
  `load-20260501065532`, the core `approval->completed` path improved by
  `7.72ms`, while `artifact_content_read` showed local variance; correctness
  remained stable and the next optimization should continue targeting generic
  run state transition reads rather than business-specific logic.
- 2026-05-01: Added Run state transition loaded Workspace reuse. During
  `dispatchStep`, the orchestrator already loads the Workspace for runtime
  execution. Decision blocking, Approval blocking, and validating/completed
  transitions now reuse that loaded Workspace through internal state transition
  helpers instead of issuing another Workspace repository read. Public
  `BlockOnDecision`, `BlockOnApproval`, and `validateAndCompleteRun` behavior
  is unchanged; callers that do not already hold a Workspace still use the
  repository-backed path. This reduces generic file backend reads in
  multi-round governance closure and keeps Run state machine, Workspace
  projection, Decision / Approval semantics, DoD validation, Result / Artifact,
  checkpoints, and event publication unchanged. Local verification passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz -count=1`
  and `go test ./... -count=1 -p 1`.
- 2026-05-01: Quantified deployed Run state transition loaded Workspace reuse.
  Batch `load-20260501070703` kept 5/5 success with no HTTP / curl / LLM
  protocol / timeline runState errors, measured e2e p95 `453.07ms`,
  throughput `140.05` workspace/min, `activation_to_decision1` p95 `69.29ms`,
  `decision1->decision2` p95 `23.43ms`, `decision2->approval` p95 `29.56ms`,
  `approval->completed` p95 `113.99ms`, `snapshot_read` p95 `51.17ms`,
  `workspace_snapshot_read` p95 `31.72ms`, `observability_read` p95
  `23.42ms`, and `artifact_content_read` p95 `15.35ms`. Compared with
  `load-20260501070159`, the initial activation path improved by `6.57ms`,
  but the final completion and readback stages showed local file backend
  variance. Correctness stayed stable; the next generic optimization targets
  removing the extra Run read in `StartRun` dispatch.
- 2026-05-01: Added StartRun loaded Run dispatch reuse. `StartRun` already
  loads and mutates the Run before dispatching the initial step, so it now
  computes dispatch specs from that loaded Run instead of calling
  `DispatchNextStep(runID)`, which previously re-read the same Run. The public
  `DispatchNextStep` API remains unchanged for external callers. This reduces
  one generic file backend Run read on the start-to-first-step path while
  preserving Run / Workspace state transitions, step dispatch semantics,
  Decision / Approval, DoD validation, Result / Artifact, checkpoints, and
  event publication. Local verification passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz -count=1`
  and `go test ./... -count=1 -p 1`. Deployed quantification is pending the
  next rebuild/redeploy.
- Completed items should be marked in this document with verification evidence.
- Broad future demand candidates remain in `docs/next-stage-demand-candidates.md`.
- Previously closed MVP scope remains in `docs/mvp-backlog.md` and should not be
  reopened unless the acceptance target itself changes.
