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
| QC28 | StartRun loaded Run dispatch reuse | P1 | implemented | Orchestrator test verifies StartRun dispatch avoids a second Run repository Get; deployed batch `load-20260501073558` keeps 5/5 success |
| QC29 | Approval ensure loaded Approval reuse | P1 | implemented | Approval service test verifies EnsureApproved reuses the loaded Approval and avoids a second repository Get; deployed batch `load-20260501074909` keeps 5/5 success |
| QC30 | Decision ensure loaded Ticket reuse | P1 | implemented | Decision service test verifies EnsureResolvedTicket reuses the loaded Ticket and avoids a second repository Get; deployed governance regression batch `load-20260501081428` keeps 100/100 success |
| QC31 | Runtime decision recovery run-scoped Ticket read | P1 | implemented | Orchestrator test verifies resolved Decision recovery uses ListByRunID and avoids full Ticket scan; deployed batch `load-20260501083902` keeps 5/5 success |
| QC32 | Serial dispatch loaded Workspace reuse | P1 | implemented | Orchestrator test verifies StartRun dispatch reuses loaded Workspace and avoids a second Workspace Get; deployed batch `load-20260501084917` keeps 5/5 success |
| QC33 | Workspace activation governance history scoped Ticket read | P1 | implemented | Appservice test verifies activation governance history uses ListByWorkspaceID instead of full Ticket List; deployed batch `load-20260501090409` keeps 5/5 success |
| QC34 | Created Run lightweight compressed context | P1 | implemented | Appservice test verifies created Run compressed context can be built without full RunSnapshot; deployed batch `load-20260501090409` keeps 5/5 success |
| QC35 | Workspace activation RunSnapshot read-model reuse | P1 | implemented | Read-model test verifies WorkspaceSnapshot with cached projection RunSnapshot avoids a second Step ListByRun; deployed batch `load-20260501091619` keeps 5/5 success |
| QC36 | Workspace activation definition read reuse | P1 | implemented | Appservice test verifies activation context reuses selected Goal / DoD and avoids second definition Get; deployed batch `load-20260501092532` keeps 5/5 success |
| QC37 | DoD closeout evidence ref reuse | P1 | implemented | Orchestrator test verifies Result summary and deliverable manifest keep the same DoD evidence refs; local tests passed, deployed quantification pending next rebuild |
| QC38 | Workspace activation response RunSnapshot reuse | P1 | implemented | Appservice test verifies activation response reuses RunSnapshot Run and avoids the extra post-start Run Get; local tests passed, deployed quantification pending next rebuild |
| QC39 | LLM follow-up workspace manifest reuse | P1 | implemented | Orchestrator test verifies follow-up LLM input inherits the previous context package workspace manifest; local tests passed, deployed quantification pending next rebuild |
| QC40 | LLM provider runtime revision cache | P1 | implemented | LLM adapter test verifies provider config/client construction is reused until workorch config revision changes; deployed batch `load-20260501100345` keeps 5/5 success |
| QC41 | Lazy selected LLM provider client construction | P1 | implemented | LLM adapter test verifies only the selected provider client is constructed and reused until config revision changes; deployed batch `load-20260501101045` keeps 5/5 success but does not improve p95 |
| QC42 | Initial LLM step failure-context read skip | P1 | implemented | Orchestrator test verifies first LLM step does not scan same-run failed-step history; deployed batch `load-20260501101906` keeps 5/5 success |
| QC43 | LLM execution context independent read parallelization | P1 | implemented | Orchestrator test verifies Result / Artifact history context reads start concurrently; deployed batch `load-20260501105944` keeps 5/5 success |
| QC44 | Final deliverable manifest step-context reuse | P1 | implemented | Orchestrator tests verify no-patch closeout reuses step context manifest and patch closeout rescans workspace; deployed batch `load-20260501111034` keeps 5/5 success |
| QC45 | Activation WorkspaceSnapshot cached Workspace reuse | P1 | implemented | Appservice test verifies activation response workspace snapshot can reuse cached RunSnapshot workspace without extra Workspace Get; deployed batch `load-20260501112144` keeps 5/5 success |
| QC46 | Activation execution context independent read parallelization | P1 | implemented | Appservice test verifies Soul routing, Memory retrieval, and governance history reads start concurrently; deployed batch `load-20260501114230` keeps 5/5 success |
| QC47 | Activation created Run compressed-context reuse | P1 | implemented | Appservice test verifies activation writes created Run compressed context without the extra Run repository Get; deployed batch `load-20260501114230` keeps 5/5 success |
| QC48 | Closeout deliverable manifest prebuild | P1 | implemented | Orchestrator test verifies manifest content build can overlap summary artifact write while Result / Artifact write order stays unchanged; deployed batch `load-20260501120459` keeps 5/5 success |
| QC49 | Terminal status wait RunSnapshot in-flight warmup | P1 | implemented | HTTP service tests verify completed status wait warms and shares terminal RunSnapshot in-flight reads with following snapshot/readback requests; deployed batch `load-20260501122412` keeps 5/5 success |
| QC50 | Resume compressed-context lightweight projection | P1 | implemented | Read-model test verifies lightweight compressedContext matches full ContextBundle compressedContext; deployed batch `load-20260501130227` keeps 5/5 success but no stable performance gain |
| QC51 | Resume executionContext patch coalesced with Run state update | P1 | implemented | Appservice / run / orchestrator tests verify resumed and rejected runs retain compressedContext without a pre-resume Run write; deployed batch `load-20260501131543` keeps 5/5 success |
| QC52 | Resume recovery snapshot lightweight read | P1 | implemented | Read-model test verifies recovery snapshot compressedContext equals full bundle compressedContext while skipping display projections; deployed batch `load-20260501132551` keeps 5/5 success but no stable performance gain |
| QC53 | Graph batch loaded Workspace reuse | P1 | implemented | Orchestrator test verifies multi-node graph batch dispatch reuses loaded Workspace; deployed batch `load-20260501133118` keeps 5/5 success |
| QC54 | Closeout Artifact writes parallelization | P1 | implemented | Orchestrator test verifies final summary and deliverable manifest Artifact writes enter concurrently before accepted Result creation; deployed batch `load-20260501133916` keeps 5/5 success |
| QC55 | Terminal Run notification snapshot warmup | P1 | implemented | HTTP service test verifies terminal Run notification warms RunSnapshot cache; deployed batch `load-20260501135110` keeps 5/5 success |
| QC56 | Decision resolution read-model parallelization | P1 | implemented | Read-model test verifies multi-ticket resolution reads start concurrently; deployed batch `load-20260501140219` keeps 5/5 success but readback p95 benefit is not stable |
| QC57 | Terminal Run / Workspace completion write parallelization | P1 | implemented | Orchestrator test verifies final Run completed write and Workspace release write start concurrently while event order stays Run completed before Workspace completed; deployed batch `load-20260501141235` keeps 5/5 success but target closeout p95 benefit is not stable |
| QC58 | Activation Goal / DoD definition read parallelization | P1 | implemented | Appservice test verifies Goal and DoD definition selection reads start concurrently while selection rules stay unchanged; deployed batch `load-20260501142037` keeps 5/5 success but activation p95 benefit is not stable |
| QC59 | Loadtest slow sample diagnostics | P1 | implemented | Loadtest report includes per-workspace `timingByStage`, `slowestWorkspaces`, and capped per-stage `stageSamples`; deployed batch `load-20260501142530` keeps 5/5 success |
| QC60 | Resolved governance resume lightweight compressed context | P1 | implemented | Appservice tests verify resolved `provide_missing_info` Decision and approved Approval resume can skip full recovery snapshot while rejected / non-continuing paths keep full context; deployed batch `load-20260501143821` keeps 5/5 success and improves approval completion p95 |
| QC61 | Activation MCP context read parallelization | P1 | implemented | Appservice test verifies selected MCP Server and Capability reads start concurrently; deployed batch `load-20260501145030` keeps 5/5 success but activation p95 benefit is not stable because the first cold sample dominates |
| QC62 | Resume dispatch synced Workspace reuse | P1 | implemented | Orchestrator test verifies resumed dispatch reuses the Workspace already loaded during running-state sync; deployed batch `load-20260501150322` keeps 5/5 success and improves governance resume stage p95 |
| QC63 | Validating Run / Workspace write parallelization | P1 | implemented | Orchestrator test verifies final validation state writes enter concurrently while Run event then Workspace event publication order stays unchanged; deployed quantification pending next rebuild |
| QC64 | Completed Step evidence write parallelization | P1 | implemented | Orchestrator test verifies ToolCall completed and ValidationResult passed evidence writes enter concurrently before Step completed persistence/event publication; deployed quantification pending next rebuild |

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
| `load-20260501073558` | 0.05s | 2.00s + notify + StartRun loaded Run reuse | 5/5 | 535.64ms | 116.91 workspace/min | 25.51ms | 32.20ms | 125.37ms |
| `load-20260501074909` | 0.05s | 2.00s + notify + approval ensure loaded approval reuse | 5/5 | 461.98ms | 130.47 workspace/min | 28.89ms | 31.46ms | 103.68ms |
| `load-20260501081952` | 0.05s | 2.00s + notify + QC29/QC30 deployed baseline after data growth | 5/5 | 1520.14ms | 40.16 workspace/min | 236.29ms | 236.45ms | 355.17ms |
| `load-20260501083902` | 0.05s | 2.00s + notify + runtime decision recovery run-scoped ticket read | 5/5 | 1206.00ms | 61.42 workspace/min | 97.06ms | 131.70ms | 215.92ms |
| `load-20260501084917` | 0.05s | 2.00s + notify + serial dispatch loaded Workspace reuse | 5/5 | 1100.37ms | 69.55 workspace/min | 79.87ms | 102.90ms | 179.51ms |
| `load-20260501085437` | 0.05s | 2.00s + notify + deployed variance baseline before QC33/QC34 rebuild | 5/5 | 808.60ms | 77.24 workspace/min | 108.32ms | 87.97ms | 172.75ms |
| `load-20260501090409` | 0.05s | 2.00s + notify + scoped governance history + lightweight created Run context | 5/5 | 1015.83ms | 71.62 workspace/min | 77.95ms | 93.66ms | 181.53ms |
| `load-20260501091619` | 0.05s | 2.00s + notify + activation RunSnapshot read-model reuse | 5/5 | 1211.18ms | 69.58 workspace/min | 89.66ms | 100.69ms | 170.33ms |
| `load-20260501092532` | 0.05s | 2.00s + notify + activation definition read reuse | 5/5 | 1055.23ms | 69.69 workspace/min | 84.54ms | 92.20ms | 206.32ms |
| `load-20260501093211` | 0.05s | 2.00s + notify + DoD closeout evidence ref reuse | 5/5 | 1273.00ms | 64.48 workspace/min | 105.77ms | 117.51ms | 197.04ms |
| `load-20260501094044` | 0.05s | 2.00s + notify + activation response RunSnapshot reuse | 5/5 | 1131.81ms | 63.23 workspace/min | 92.12ms | 170.92ms | 230.30ms |
| `load-20260501094820` | 0.05s | 2.00s + notify + LLM follow-up workspace manifest reuse | 5/5 | 1169.41ms | 64.79 workspace/min | 81.59ms | 91.51ms | 215.01ms |
| `load-20260501100345` | 0.05s | 2.00s + notify + LLM provider runtime revision cache | 5/5 | 1124.63ms | 68.69 workspace/min | 95.23ms | 90.14ms | 191.38ms |
| `load-20260501101045` | 0.05s | 2.00s + notify + lazy selected LLM provider client construction | 5/5 | 1227.25ms | 64.17 workspace/min | 100.09ms | 110.92ms | 186.17ms |
| `load-20260501101906` | 0.05s | 2.00s + notify + initial LLM step failure-context read skip | 5/5 | 1211.85ms | 57.13 workspace/min | 96.61ms | 120.13ms | 222.98ms |
| `load-20260501105944` | 0.05s | 2.00s + notify + LLM context independent read parallelization | 5/5 | 1241.80ms | 62.66 workspace/min | 77.64ms | 113.23ms | 256.71ms |
| `load-20260501111034` | 0.05s | 2.00s + notify + final deliverable manifest step-context reuse | 5/5 | 1285.62ms | 62.54 workspace/min | 97.74ms | 120.14ms | 223.24ms |
| `load-20260501112144` | 0.05s | 2.00s + notify + activation WorkspaceSnapshot cached Workspace reuse | 5/5 | 1184.98ms | 65.74 workspace/min | 84.10ms | 96.38ms | 203.10ms |
| `load-20260501114230` | 0.05s | 2.00s + notify + activation context parallel reads + created Run context reuse | 5/5 | 1246.77ms | 59.70 workspace/min | 91.13ms | 127.57ms | 213.76ms |
| `load-20260501120459` | 0.05s | 2.00s + notify + closeout deliverable manifest prebuild | 5/5 | 1285.37ms | 59.29 workspace/min | 97.32ms | 142.16ms | 194.72ms |
| `load-20260501122412` | 0.05s | 2.00s + notify + terminal RunSnapshot in-flight warmup | 5/5 | 1157.26ms | 64.69 workspace/min | 101.13ms | 120.24ms | 218.41ms |

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
  and `go test ./... -count=1 -p 1`.
- 2026-05-01: Quantified deployed StartRun loaded Run dispatch reuse. Batch
  `load-20260501073558` kept 5/5 success with no HTTP / curl / LLM protocol /
  timeline runState errors, measured e2e p95 `535.64ms`, throughput `116.91`
  workspace/min, `activation_to_decision1` p95 `88.41ms`,
  `decision1->decision2` p95 `25.51ms`, `decision2->approval` p95 `32.20ms`,
  `approval->completed` p95 `125.37ms`, `snapshot_read` p95 `51.86ms`,
  `workspace_snapshot_read` p95 `40.87ms`, `observability_read` p95
  `35.81ms`, and `artifact_content_read` p95 `12.90ms`. Correctness remained
  stable, but this batch regressed versus `load-20260501070703` because local
  file backend/readback variance dominated the small removed Run read. Treat
  QC28 as a necessary cleanup with low standalone measurable impact; further
  optimization should target coalescing terminal readback calls rather than
  more single-record start-path reads.
- 2026-05-01: Added Approval ensure loaded Approval reuse. `EnsureApproved` and
  `EnsureRejected` already load the Approval to validate idempotent and
  opposite-terminal-state semantics, so they now reuse that loaded entity for
  the state transition instead of calling `Approve` / `Reject`, which re-read
  the same Approval. Public approval service behavior and events are unchanged:
  pending approvals still transition, already matching terminal approvals stay
  idempotent, and opposite terminal states still return conflict. This reduces
  one generic file backend Approval read on each approved/rejected governance
  step before Run resume. Local verification passed with:
  `go test ./internal/modules/blocks/approval/biz ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz -count=1`
  and `go test ./... -count=1 -p 1`.
- 2026-05-01: Quantified deployed Approval ensure loaded Approval reuse after
  rebuild/redeploy. Batch `load-20260501074909` kept 5/5 success with no HTTP /
  curl / LLM protocol / timeline runState errors, measured e2e p95
  `461.98ms`, throughput `130.47` workspace/min,
  `decision1->decision2` p95 `28.89ms`, `decision2->approval` p95 `31.46ms`,
  `approval->completed` p95 `103.68ms`, `snapshot_read` p95 `44.05ms`,
  `workspace_snapshot_read` p95 `25.27ms`, `observability_read` p95
  `25.20ms`, and `artifact_content_read` p95 `12.21ms`. Compared with
  `load-20260501073558`, e2e p95 improved by `73.66ms`, throughput improved by
  `13.56` workspace/min, and `approval->completed` p95 improved by `21.69ms`.
  The result confirms governance resolution read reuse is visible in this
  single-workspace mock governed path, while local file backend variance still
  requires tracking through repeated identical runs.
- 2026-05-01: Added Decision ensure loaded Ticket reuse. `EnsureResolvedTicket`
  already loads the Ticket to validate existing resolution idempotency, so the
  open-ticket transition now reuses that loaded entity instead of calling
  `ResolveTicket`, which re-read the same Ticket. Public decision service
  behavior and events are unchanged: open tickets still persist a Resolution
  and publish `ticket_resolved`, already matching resolutions remain
  idempotent, and mismatched resolutions still return conflict. This reduces
  one generic file backend Ticket read on each Decision resolution before Run
  resume. Local verification passed with:
  `go test ./internal/modules/kernel/decision/biz ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz -count=1`
  and `go test ./... -count=1 -p 1`.
- 2026-05-01: Verified the deployed QC29/QC30 build with full regression and
  governed concurrency before continuing single-workspace optimization.
  `make verify-deployed-full` passed against `http://127.0.0.1:8080`,
  including real LLM provider `miniMax`, real weather query, multi-round
  Ticket + Approval, real MCP stdio, and vector Memory recall. Governed mock
  concurrency batches `load-20260501081203` (`c10/t30`),
  `load-20260501081248` (`c20/t50`), and `load-20260501081428`
  (`c20/t100`) all passed with 100% success and no HTTP / curl / LLM protocol /
  timeline runState errors. The high-pressure baseline `load-20260501081428`
  measured throughput `68.13` workspace/min, e2e p95 `23.22s`,
  `approval->completed` p95 `5.31s`, `snapshot_read` p95 `1.69s`,
  `workspace_snapshot_read` p95 `1.56s`, and `observability_read` p95
  `1.39s`. This confirms the Decision / Approval loaded-entity reuse changes
  did not regress overall functionality or concurrency capacity; the remaining
  high-concurrency cost remains file backend write/read-model amplification.
- 2026-05-01: Added runtime Decision recovery run-scoped Ticket read. The
  orchestrator recovery path for a running Run with a failed terminal Step and
  a resolved Decision now calls `DecisionTicketRepository.ListByRunID(runID)`
  instead of scanning all Decision Tickets through `List()` before filtering by
  Run and Step. This keeps the same recovery semantics for
  `provide_missing_info`, `reject_current_path`, and `replan`, but avoids a
  full Ticket scan on generic Decision recovery. Local verification passed
  with:
  `go test ./internal/modules/kernel/orchestrator/biz ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/pkg/repository ./internal/pkg/repository/file -count=1`
  and `go test ./... -count=1 -p 1`.
- 2026-05-01: Quantified the deployed runtime Decision recovery run-scoped
  Ticket read after rebuild/redeploy. Batch `load-20260501083902` kept 5/5
  success with no HTTP / curl / LLM protocol / timeline runState errors,
  measured e2e p95 `1206.00ms`, throughput `61.42` workspace/min,
  `activation_to_decision1` p95 `333.12ms`, `decision1->decision2` p95
  `97.06ms`, `decision2->approval` p95 `131.70ms`,
  `approval->completed` p95 `215.92ms`, `snapshot_read` p95 `75.88ms`,
  `workspace_snapshot_read` p95 `46.84ms`, `observability_read` p95
  `23.04ms`, and `artifact_content_read` p95 `14.82ms`. Compared with the
  immediately preceding deployed single-workspace baseline
  `load-20260501081952` (`1520.14ms` e2e p95, `40.16` workspace/min,
  `236.29ms` decision1->decision2 p95, `236.45ms` decision2->approval p95,
  `355.17ms` approval->completed p95), e2e p95 improved by `314.14ms` and
  throughput improved by `21.26` workspace/min. Correctness remained stable;
  the next generic optimization target is the initial activation path and
  terminal Result / Artifact materialization path, not business-specific goal
  parsing.
- 2026-05-01: Added serial dispatch loaded Workspace reuse. `StartRun` already
  loads the Workspace to activate it, and each serial `dispatchStep` needs the
  same Workspace for LLM context enrichment, runtime execution working
  directory, blocking transitions, Result / Artifact manifest generation, and
  terminal Workspace projection. The orchestrator now passes the loaded
  Workspace through internal `dispatchStepWithWorkspace` /
  `dispatchStepSpecsWithWorkspace` calls for the single-step and serial
  follow-up path, avoiding repeated Workspace repository reads in the same Run
  chain. Graph batch nodes still use the existing independent dispatch path to
  avoid sharing mutable Workspace state across concurrent node execution.
  Public dispatch APIs and state-machine semantics are unchanged. Local
  verification passed with:
  `go test ./internal/modules/kernel/orchestrator/biz ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/pkg/repository ./internal/pkg/repository/file -count=1`
  and `go test ./... -count=1 -p 1`.
- 2026-05-01: Quantified the deployed serial dispatch loaded Workspace reuse
  after rebuild/redeploy. Batch `load-20260501084917` kept 5/5 success with no
  HTTP / curl / LLM protocol / timeline runState errors, measured e2e p95
  `1100.37ms`, throughput `69.55` workspace/min,
  `activation_to_decision1` p95 `340.63ms`, `decision1->decision2` p95
  `79.87ms`, `decision2->approval` p95 `102.90ms`,
  `approval->completed` p95 `179.51ms`, `snapshot_read` p95 `72.71ms`,
  `workspace_snapshot_read` p95 `52.29ms`, `observability_read` p95
  `25.25ms`, and `artifact_content_read` p95 `10.63ms`. Compared with
  `load-20260501083902`, e2e p95 improved by `105.63ms`, throughput improved
  by `8.13` workspace/min, `decision1->decision2` p95 improved by `17.19ms`,
  `decision2->approval` p95 improved by `28.80ms`, and
  `approval->completed` p95 improved by `36.41ms`. The initial activation path
  remains the dominant p95 segment under the current file backend data volume;
  the next optimization should target generic activation/start-path work and
  avoid any Goal / DoD business-content parsing.
- 2026-05-01: Re-ran the currently deployed build before the next activation
  optimization deployment. Batch `load-20260501085437` kept 5/5 success with no
  HTTP / curl / LLM protocol / timeline runState errors, measured e2e p95
  `808.60ms`, throughput `77.24` workspace/min,
  `activation_to_decision1` p95 `106.43ms`, `decision1->decision2` p95
  `108.32ms`, `decision2->approval` p95 `87.97ms`,
  `approval->completed` p95 `172.75ms`, `snapshot_read` p95 `64.68ms`,
  `workspace_snapshot_read` p95 `62.38ms`, `observability_read` p95
  `21.96ms`, and `artifact_content_read` p95 `11.57ms`. This batch does not
  include QC33 / QC34 because the service was not rebuilt after those code
  changes; use it only as deployed variance context.
- 2026-05-01: Added workspace-scoped governance history lookup during workspace
  execution activation. `workspaceGovernanceHistory` now calls
  `DecisionTicketRepository.ListByWorkspaceID(workspaceID)` instead of full
  `List()` and keeps the existing Run / Goal / DoD / Resolution filtering.
  This removes a data-growth-sensitive full Ticket scan from the start path
  while preserving governance recovery context and staying business-generic.
  Local verification passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz ./internal/pkg/repository ./internal/pkg/repository/file -count=1`
  and `go test ./... -count=1 -p 1`.
- 2026-05-01: Added lightweight compressed context generation for newly created
  Runs. For `created` Runs with no execution evidence yet, `attachCompressedContextToRun`
  now builds the deterministic recovery summary from the Run identity and
  milestone skeleton instead of constructing a full `RunSnapshot`. Resume and
  non-created Run paths still use the full snapshot-based compression so
  blockers, latest evidence, Result refs, and recovery checkpoints remain
  complete after execution has started. Local verification passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz ./internal/pkg/repository ./internal/pkg/repository/file -count=1`
  and `go test ./... -count=1 -p 1`.
- 2026-05-01: Quantified the deployed QC33 / QC34 activation optimizations
  after rebuild/redeploy. Batch `load-20260501090409` kept 5/5 success with no
  HTTP / curl / LLM protocol / timeline runState errors, measured e2e p95
  `1015.83ms`, throughput `71.62` workspace/min,
  `activation_to_decision1` p95 `304.62ms`, `decision1->decision2` p95
  `77.95ms`, `decision2->approval` p95 `93.66ms`,
  `approval->completed` p95 `181.53ms`, `snapshot_read` p95 `70.24ms`,
  `workspace_snapshot_read` p95 `49.33ms`, `observability_read` p95
  `27.09ms`, and `artifact_content_read` p95 `12.69ms`. Compared with
  `load-20260501084917`, e2e p95 improved by `84.54ms`, throughput improved by
  `2.07` workspace/min, `decision1->decision2` p95 improved by `1.92ms`, and
  `decision2->approval` p95 improved by `9.24ms`; `approval->completed`
  regressed by `2.02ms` and the first activation sample still dominates p95.
  Correctness remained stable. The next target should continue focusing on
  generic activation/start scheduling and status-read wait behavior.
- 2026-05-01: Added Workspace activation RunSnapshot read-model reuse. After a
  new workspace execution Run reaches its first blocked / terminal state,
  activation now builds that RunSnapshot once and passes it into the
  WorkspaceSnapshot projection instead of rebuilding the same projection Run's
  full evidence chain during activation response assembly. The response
  structure remains unchanged for Web consumers: `activation.run` and
  `activation.snapshot` are still returned. This is read-model reuse only and
  does not change Run state transitions, Decision / Approval, DoD validation,
  Result / Artifact, or business Goal interpretation. Local verification
  passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz ./internal/pkg/repository ./internal/pkg/repository/file -count=1`
  and `go test ./... -count=1 -p 1`.
- 2026-05-01: Quantified the deployed QC35 activation RunSnapshot read-model
  reuse after rebuild/redeploy. Batch `load-20260501091619` kept 5/5 success
  with no HTTP / curl / LLM protocol / timeline runState errors, measured e2e
  p95 `1211.18ms`, throughput `69.58` workspace/min,
  `activation_to_decision1` p95 `392.47ms`, `decision1->decision2` p95
  `89.66ms`, `decision2->approval` p95 `100.69ms`,
  `approval->completed` p95 `170.33ms`, `snapshot_read` p95 `60.10ms`,
  `workspace_snapshot_read` p95 `52.14ms`, `observability_read` p95
  `19.63ms`, and `artifact_content_read` p95 `10.96ms`. Compared with
  `load-20260501090409`, final `approval->completed` improved by `11.20ms`,
  `snapshot_read` improved by `10.14ms`, `observability_read` improved by
  `7.46ms`, and `artifact_content_read` improved by `1.73ms`; e2e p95 and
  activation p95 regressed because the first activation sample was slower.
  Correctness remained stable. The next generic optimization target is
  activation request setup read reuse, especially repeated Workspace / Goal /
  DoD reads before Run start.
- 2026-05-01: Added workspace activation definition read reuse. Activation now
  selects Goal and DoD entities through the existing workspace-scoped list read
  and reuses those loaded entities, plus the already loaded Workspace, when
  building the execution context. This removes the previous repeated
  Workspace / Goal / DoD repository reads from activation setup while keeping
  the same definition selection, DoD readiness validation, Soul / Skill / MCP
  / Memory context assembly, Run creation, and response shape. The generic
  `CreateRunWithSession` path still uses repository-backed validation because
  it does not already hold selected entities. Local verification passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz ./internal/pkg/repository ./internal/pkg/repository/file -count=1`
  and `go test ./... -count=1 -p 1`.
- 2026-05-01: Quantified the deployed QC36 activation definition read reuse
  after rebuild/redeploy. Batch `load-20260501092532` kept 5/5 success with no
  HTTP / curl / LLM protocol / timeline runState errors, measured e2e p95
  `1055.23ms`, throughput `69.69` workspace/min,
  `activation_to_decision1` p95 `319.58ms`, `decision1->decision2` p95
  `84.54ms`, `decision2->approval` p95 `92.20ms`,
  `approval->completed` p95 `206.32ms`, `snapshot_read` p95 `57.15ms`,
  `workspace_snapshot_read` p95 `48.12ms`, `observability_read` p95
  `29.19ms`, and `artifact_content_read` p95 `16.07ms`. Compared with
  `load-20260501091619`, e2e p95 improved by `155.95ms`,
  `activation_to_decision1` p95 improved by `72.89ms`,
  `decision1->decision2` p95 improved by `5.11ms`,
  `decision2->approval` p95 improved by `8.49ms`, and
  `snapshot_read` p95 improved by `2.95ms`; `approval->completed` regressed by
  `35.99ms`, so the next generic target shifts to closeout / Result /
  Artifact materialization after approval.
- 2026-05-01: Added DoD closeout evidence ref reuse. The terminal DoD
  assessment path now extracts normalized evidence refs once in
  `recordRunResultFromDoDAssessment` and passes the same refs into Result
  summary Artifact creation, workspace deliverable manifest creation, and
  Result summary detail assembly. This keeps Result / Artifact content and
  evidence references identical while removing repeated extraction work in the
  `approval->completed` closeout segment. The change is generic closeout
  materialization only; it does not alter DoD validation, Decision / Approval,
  workspace projection, artifact categories, or any business Goal parsing.
  Local verification passed with:
  `go test ./internal/modules/kernel/orchestrator/biz ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/pkg/repository ./internal/pkg/repository/file -count=1`
  and `go test ./... -count=1 -p 1`. Deployed single-workspace quantification
  is pending the next rebuild/redeploy; compare against
  `load-20260501092532` (`1055.23ms` e2e p95,
  `206.32ms` approval->completed p95).
- 2026-05-01: Quantified the deployed QC37 DoD closeout evidence ref reuse
  after rebuild/redeploy. Batch `load-20260501093211` kept 5/5 success with no
  HTTP / curl / LLM protocol / timeline runState errors, measured e2e p95
  `1273.00ms`, throughput `64.48` workspace/min,
  `activation_to_decision1` p95 `355.11ms`, `decision1->decision2` p95
  `105.77ms`, `decision2->approval` p95 `117.51ms`,
  `approval->completed` p95 `197.04ms`, `snapshot_read` p95 `76.32ms`,
  `workspace_snapshot_read` p95 `51.58ms`, `observability_read` p95
  `26.74ms`, and `artifact_content_read` p95 `14.96ms`. Compared with
  `load-20260501092532`, the targeted closeout segment improved by `9.28ms`
  while e2e p95 regressed by `217.77ms` because the first workspace activation
  sample took `420.55ms` and dominated p95. Correctness remained stable. The
  next target is generic activation/start cold-path and readback overhead, not
  changing multi-round Decision / Approval or DoD acceptance semantics.
- 2026-05-01: Added Workspace activation response RunSnapshot reuse. After
  creating and starting the Run, activation already builds a full RunSnapshot
  for the response and for WorkspaceSnapshot projection reuse. The start helper
  now returns the generated Run ID instead of reloading the Run immediately
  after `EnsureStarted`; the response Run is populated from the RunSnapshot.
  This removes one redundant appservice-level Run repository read from the
  activation path while preserving response shape, execution context,
  workspace projection, Decision / Approval blocking state, and DoD behavior.
  Local verification passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz ./internal/pkg/repository ./internal/pkg/repository/file -count=1`
  and `go test ./... -count=1 -p 1`. Deployed single-workspace quantification
  is pending the next rebuild/redeploy; compare against
  `load-20260501093211` (`1273.00ms` e2e p95,
  `355.11ms` activation_to_decision1 p95).
- 2026-05-01: Quantified the deployed QC38 activation response RunSnapshot
  reuse after rebuild/redeploy. Batch `load-20260501094044` kept 5/5 success
  with no HTTP / curl / LLM protocol / timeline runState errors, measured e2e
  p95 `1131.81ms`, throughput `63.23` workspace/min,
  `activation_to_decision1` p95 `313.69ms`, `decision1->decision2` p95
  `92.12ms`, `decision2->approval` p95 `170.92ms`,
  `approval->completed` p95 `230.30ms`, `snapshot_read` p95 `64.87ms`,
  `workspace_snapshot_read` p95 `73.12ms`, `observability_read` p95
  `23.62ms`, and `artifact_content_read` p95 `15.33ms`. Compared with
  `load-20260501093211`, e2e p95 improved by `141.19ms` and
  `activation_to_decision1` p95 improved by `41.41ms`; the closeout segment
  regressed by `33.26ms`, so the next target moves back to generic terminal
  completion read/write and snapshot materialization overhead while preserving
  Result / Artifact / DoD semantics.
- 2026-05-01: Added LLM follow-up workspace manifest reuse. Multi-round LLM
  execution already includes `workspaceFileManifest` in the execution context
  package, but follow-up LLM steps previously did not carry that manifest into
  the next step input, so each LLM context enrichment re-scanned the workspace
  directory. `nextStepInputFromOutput`, graph follow-up, and completion
  follow-up now inherit the previous step's workspace manifest when available,
  allowing the existing context package code to reuse it. Runtime repair keeps
  its explicit workspace manifest refresh path because repair can follow file
  write / patch changes and should not reuse stale directory state. This is a
  generic context-read optimization only; it does not alter LLM decisions,
  execution graph semantics, Decision / Approval, Result / Artifact, or DoD
  validation. Local verification passed with:
  `go test ./internal/modules/kernel/orchestrator/biz -count=1`,
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz ./internal/pkg/repository ./internal/pkg/repository/file -count=1`,
  and `go test ./... -count=1 -p 1`. Deployed single-workspace quantification
  is pending the next rebuild/redeploy; compare against
  `load-20260501094044` (`1131.81ms` e2e p95,
  `313.69ms` activation_to_decision1 p95,
  `230.30ms` approval->completed p95).
- 2026-05-01: Quantified the deployed QC39 LLM follow-up workspace manifest
  reuse after rebuild/redeploy. Batch `load-20260501094820` kept 5/5 success
  with no HTTP / curl / LLM protocol / timeline runState errors, measured e2e
  p95 `1169.41ms`, throughput `64.79` workspace/min,
  `activation_to_decision1` p95 `376.48ms`, `decision1->decision2` p95
  `81.59ms`, `decision2->approval` p95 `91.51ms`,
  `approval->completed` p95 `215.01ms`, `snapshot_read` p95 `56.60ms`,
  `workspace_snapshot_read` p95 `52.05ms`, `observability_read` p95
  `22.97ms`, and `artifact_content_read` p95 `16.59ms`. Compared with
  `load-20260501094044`, `decision1->decision2` improved by `10.53ms`,
  `decision2->approval` improved by `79.41ms`, `approval->completed`
  improved by `15.29ms`, `snapshot_read` improved by `8.27ms`, and
  `workspace_snapshot_read` improved by `21.07ms`; e2e p95 regressed by
  `37.59ms` because the first activation sample took `446.80ms` and dominated
  p95. Correctness remained stable. The next target remains the generic
  activation cold path and first response assembly overhead.
- 2026-05-01: Added LLM provider runtime revision cache. The workorchd LLM
  block now builds provider runtimes from `config/application.toml` once per
  `workorchconfig` revision and reuses the resulting provider clients across
  LLM steps. `UpdateLLMConfiguration` still persists and reloads the
  application config; `workorchconfig.Reload()` advances the process-local
  revision, so later LLM steps rebuild the cache after Web-side config changes.
  The existing dynamic loader constructor remains uncached for tests or callers
  that intentionally need per-call config reads. This removes repeated provider
  config loading and SDK / HTTP client construction from multi-round LLM
  execution without changing provider selection, Decision / Approval, DoD
  validation, Result / Artifact materialization, or any business Goal parsing.
  Local verification passed with:
  `go test ./internal/modules/blocks/llm/biz -count=1`,
  `go test ./internal/pkg/workorchconfig -count=1`,
  `go test ./internal/modules/blocks/llm/biz ./internal/pkg/workorchconfig ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz -count=1`,
  and `go test ./... -count=1 -p 1`. Deployed single-workspace quantification
  is pending the next rebuild/redeploy; compare against
  `load-20260501094820` (`1169.41ms` e2e p95,
  `376.48ms` activation_to_decision1 p95,
  `81.59ms` decision1->decision2 p95,
  `91.51ms` decision2->approval p95,
  `215.01ms` approval->completed p95).
- 2026-05-01: Quantified the deployed QC40 LLM provider runtime revision cache
  after rebuild/redeploy. Batch `load-20260501100345` kept 5/5 success with no
  HTTP / curl / LLM protocol / timeline runState errors, measured e2e p95
  `1124.63ms`, throughput `68.69` workspace/min,
  `activation_to_decision1` p95 `340.45ms`, `decision1->decision2` p95
  `95.23ms`, `decision2->approval` p95 `90.14ms`,
  `approval->completed` p95 `191.38ms`, `snapshot_read` p95 `88.21ms`,
  `workspace_snapshot_read` p95 `45.63ms`, `observability_read` p95
  `22.10ms`, and `artifact_content_read` p95 `12.33ms`. Compared with
  `load-20260501094820`, e2e p95 improved by `44.78ms`, throughput improved
  by `3.90` workspace/min, `activation_to_decision1` improved by `36.03ms`,
  `decision2->approval` improved by `1.37ms`, `approval->completed` improved
  by `23.63ms`, `workspace_snapshot_read` improved by `6.42ms`, and
  `artifact_content_read` improved by `4.26ms`; `decision1->decision2`
  regressed by `13.64ms` and `snapshot_read` regressed by `31.61ms`.
  Correctness remained stable. The first workspace still had a cold activation
  sample of `402.45ms`, so the next target remains provider / activation
  cold-path overhead.
- 2026-05-01: Added lazy selected LLM provider client construction. The
  revision cache now stores provider runtime entries for all configured
  providers, but each available provider is wrapped in a lazy client and only
  constructs the concrete SDK / HTTP provider on first use. This prevents a
  first LLM step that selects the mock or default provider from also building
  unrelated real provider clients such as OpenAI-compatible MiniMax or
  DeepSeek entries. The client is still reused for later calls within the same
  config revision, and `workorchconfig.Reload()` still forces a new lazy
  runtime set. This is a provider runtime construction optimization only; it
  does not change provider selection, streaming / embedding capability checks,
  Decision / Approval, DoD validation, Result / Artifact materialization, or
  business Goal parsing. Local verification passed with:
  `go test ./internal/modules/blocks/llm/biz -count=1`,
  `go test ./internal/modules/blocks/llm/biz ./internal/pkg/workorchconfig ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz -count=1`,
  and `go test ./... -count=1 -p 1`. Deployed single-workspace quantification
  is pending the next rebuild/redeploy; compare against
  `load-20260501100345` (`1124.63ms` e2e p95,
  `340.45ms` activation_to_decision1 p95,
  `95.23ms` decision1->decision2 p95,
  `90.14ms` decision2->approval p95,
  `191.38ms` approval->completed p95).
- 2026-05-01: Quantified the deployed QC41 lazy selected LLM provider client
  construction after rebuild/redeploy. Batch `load-20260501101045` kept 5/5
  success with no HTTP / curl / LLM protocol / timeline runState errors,
  measured e2e p95 `1227.25ms`, throughput `64.17` workspace/min,
  `activation_to_decision1` p95 `376.32ms`, `decision1->decision2` p95
  `100.09ms`, `decision2->approval` p95 `110.92ms`,
  `approval->completed` p95 `186.17ms`, `snapshot_read` p95 `64.39ms`,
  `workspace_snapshot_read` p95 `61.11ms`, `observability_read` p95
  `20.81ms`, and `artifact_content_read` p95 `19.68ms`. Compared with
  `load-20260501100345`, e2e p95 regressed by `102.62ms`,
  `activation_to_decision1` regressed by `35.87ms`,
  `decision1->decision2` regressed by `4.86ms`, `decision2->approval`
  regressed by `20.78ms`, and `workspace_snapshot_read` regressed by
  `15.48ms`; `approval->completed` improved by `5.21ms`,
  `snapshot_read` improved by `23.82ms`, and `observability_read` improved by
  `1.29ms`. Correctness remained stable, but this did not improve the current
  p95 bottleneck. The next target is first-step LLM context construction where
  empty same-run failure history is still read on the initial step.
- 2026-05-01: Added initial LLM step failure-context read skip. The execution
  context package still includes `recentFailureSummary` for repair and
  follow-up LLM steps, but the first step of a Run now skips the same-run
  failed-step history scan because no earlier step in that Run can exist before
  `run-id/step-1` is dispatched. This removes one unnecessary `Step.ListByRun`
  read from the synchronous `StartRun -> first LLM decision` path while
  preserving failure recovery context for every non-initial step. The change is
  generic context construction only; it does not change provider selection,
  Decision / Approval, DoD validation, Result / Artifact materialization, or
  business Goal parsing. Local verification passed with:
  `go test ./internal/modules/kernel/orchestrator/biz -count=1`,
  `go test ./internal/modules/blocks/llm/biz ./internal/pkg/workorchconfig ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz -count=1`,
  and `go test ./... -count=1 -p 1`. Deployed single-workspace quantification
  is pending the next rebuild/redeploy; compare against
  `load-20260501101045` (`1227.25ms` e2e p95,
  `376.32ms` activation_to_decision1 p95,
  `100.09ms` decision1->decision2 p95,
  `110.92ms` decision2->approval p95,
  `186.17ms` approval->completed p95).
- 2026-05-01: Quantified the deployed QC42 initial LLM step failure-context
  read skip after rebuild/redeploy. Batch `load-20260501101906` kept 5/5
  success with no HTTP / curl / LLM protocol / timeline runState errors,
  measured e2e p95 `1211.85ms`, throughput `57.13` workspace/min,
  `activation_to_decision1` p95 `373.13ms`, `decision1->decision2` p95
  `96.61ms`, `decision2->approval` p95 `120.13ms`,
  `approval->completed` p95 `222.98ms`, `snapshot_read` p95 `88.97ms`,
  `workspace_snapshot_read` p95 `62.96ms`, `observability_read` p95
  `34.46ms`, and `artifact_content_read` p95 `24.51ms`. Compared with
  `load-20260501101045`, activation improved slightly by `3.19ms` and
  `decision1->decision2` improved by `3.48ms`, while e2e p95 improved by
  `15.40ms`; closeout and readback regressed under local file backend
  variance. Correctness remained stable, so the next target is generic
  independent context read parallelization rather than business-specific
  Goal / DoD parsing.
- 2026-05-01: Added LLM execution context independent read parallelization.
  `executionContextPackage` now builds workspace file manifest, latest accepted
  Result context, current deliverables, and same-run failure summary
  concurrently because these are independent read-only inputs to the LLM step.
  This targets file backend latency in the `StartRun -> first LLM decision` and
  follow-up LLM context paths while preserving the exact same context fields,
  Decision / Approval flow, DoD validation, Result / Artifact materialization,
  recovery behavior, and business-generic orchestrator boundary. Local
  verification passed with:
  `go test ./internal/modules/kernel/orchestrator/biz -count=1`. Deployed
  single-workspace quantification is pending the next rebuild/redeploy; compare
  against `load-20260501101906` (`1211.85ms` e2e p95,
  `373.13ms` activation_to_decision1 p95,
  `96.61ms` decision1->decision2 p95,
  `120.13ms` decision2->approval p95,
  `222.98ms` approval->completed p95).
- 2026-05-01: Quantified the deployed QC43 LLM execution context independent
  read parallelization after rebuild/redeploy. Batch `load-20260501105944`
  kept 5/5 success with no HTTP / curl / LLM protocol / timeline runState
  errors, measured e2e p95 `1241.80ms`, throughput `62.66` workspace/min,
  `activation_to_decision1` p95 `408.50ms`, `decision1->decision2` p95
  `77.64ms`, `decision2->approval` p95 `113.23ms`,
  `approval->completed` p95 `256.71ms`, `snapshot_read` p95 `80.90ms`,
  `workspace_snapshot_read` p95 `51.69ms`, `observability_read` p95
  `27.58ms`, and `artifact_content_read` p95 `15.21ms`. Compared with
  `load-20260501101906`, `decision1->decision2` improved by `18.97ms`,
  `decision2->approval` improved by `6.90ms`, workspace snapshot/readback
  improved, and throughput improved by `5.53` workspace/min, while e2e and
  activation p95 regressed because the first activation sample reached
  `484.24ms`; `approval->completed` also regressed under closeout/file-backend
  variance. Correctness stayed stable, so the next optimization targets
  generic final deliverable manifest generation in the closeout path.
- 2026-05-01: Added final deliverable manifest step-context reuse. The closeout
  manifest generator now reuses the final step's
  `executionContextPackage.workspaceFileManifest` when the step output contains
  no patches, avoiding a redundant workspace directory scan during final
  Result / Artifact materialization. If the final step contains patch output,
  the generator still rescans the workspace directory so the deliverable
  manifest reflects newly written files. This is a generic data freshness
  rule based on runtime patch output, not Goal / DoD parsing, and preserves
  Result / Artifact schemas, evidence refs, DoD validation, and recovery
  semantics. Local verification passed with:
  `go test ./internal/modules/kernel/orchestrator/biz -count=1`. Deployed
  single-workspace quantification is pending the next rebuild/redeploy; compare
  against `load-20260501105944` (`1241.80ms` e2e p95,
  `408.50ms` activation_to_decision1 p95,
  `77.64ms` decision1->decision2 p95,
  `113.23ms` decision2->approval p95,
  `256.71ms` approval->completed p95).
- 2026-05-01: Quantified the deployed QC44 final deliverable manifest
  step-context reuse after rebuild/redeploy. Batch `load-20260501111034` kept
  5/5 success with no HTTP / curl / LLM protocol / timeline runState errors,
  measured e2e p95 `1285.62ms`, throughput `62.54` workspace/min,
  `activation_to_decision1` p95 `399.16ms`, `decision1->decision2` p95
  `97.74ms`, `decision2->approval` p95 `120.14ms`,
  `approval->completed` p95 `223.24ms`, `snapshot_read` p95 `72.70ms`,
  `workspace_snapshot_read` p95 `51.54ms`, `observability_read` p95
  `22.87ms`, and `artifact_content_read` p95 `20.05ms`. Compared with
  `load-20260501105944`, the targeted closeout segment improved by `33.47ms`
  and snapshot / observability reads also improved, while e2e p95 remained
  dominated by the first activation cold sample (`477.54ms`). Correctness
  stayed stable, so the next target is generic activation response read reuse.
- 2026-05-01: Added activation WorkspaceSnapshot cached Workspace reuse. The
  activation API already builds a RunSnapshot after starting the Run; the
  WorkspaceSnapshot projection can now reuse the Workspace projection from that
  RunSnapshot instead of performing another Workspace repository read for the
  same workspace. This is read-model construction reuse only and does not
  change Run state, Workspace state, Decision / Approval semantics, DoD
  validation, Result / Artifact generation, or Goal / DoD interpretation.
  Local verification passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestActivationResponseWorkspaceSnapshotReusesRunSnapshotWorkspace|TestActivationResponseReusesRunSnapshotRun' -count=1`.
  After rebuild/redeploy, governed mock `concurrency=1,total=5` batch
  `load-20260501112144` kept 5/5 success with no HTTP / curl / LLM protocol /
  timeline runState errors, measured e2e p95 `1184.98ms`, throughput `65.74`
  workspace/min, `activation_to_decision1` p95 `387.40ms`,
  `decision1->decision2` p95 `84.10ms`, `decision2->approval` p95 `96.38ms`,
  `approval->completed` p95 `203.10ms`, `snapshot_read` p95 `66.30ms`,
  `workspace_snapshot_read` p95 `50.50ms`, `observability_read` p95 `25.44ms`,
  and `artifact_content_read` p95 `10.83ms`. Compared with
  `load-20260501111034`, e2e p95 improved by `100.64ms`, throughput improved
  by `3.20` workspace/min, `activation_to_decision1` improved by `11.76ms`,
  `decision1->decision2` improved by `13.64ms`, `decision2->approval`
  improved by `23.76ms`, `approval->completed` improved by `20.14ms`,
  `snapshot_read` improved by `6.40ms`, and `artifact_content_read` improved
  by `9.22ms`. Correctness stayed stable, so the next safe target remains
  generic first activation and closeout read-path reduction.
- 2026-05-01: Added activation execution context independent read
  parallelization. The activation context builder now starts Soul routing,
  Memory retrieval, and governance history reads concurrently because they are
  independent read-only inputs before Skill/MCP binding. Skill auto-selection
  still waits for the resolved default Soul, and MCP auto-binding plus Soul
  capability validation still execute in the original order, so capability
  boundaries and business-generic orchestration semantics are unchanged. Local
  verification passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestBuildWorkspaceExecutionContextStartsIndependentReadsConcurrently|TestActivationResponseWorkspaceSnapshotReusesRunSnapshotWorkspace|TestActivateWorkspaceExecutionWithBackendAutoSelectsSkillAndBindsRequiredMCP|TestActivateWorkspaceExecutionWithBackendUsesSemanticMemoryRecall' -count=1`.
  After rebuild/redeploy with QC47, governed mock `concurrency=1,total=5`
  batch `load-20260501114230` kept 5/5 success with no HTTP / curl / LLM
  protocol / timeline runState errors, measured e2e p95 `1246.77ms`,
  throughput `59.70` workspace/min, `activation_to_decision1` p95 `405.03ms`,
  `decision1->decision2` p95 `91.13ms`, `decision2->approval` p95
  `127.57ms`, `approval->completed` p95 `213.76ms`, `snapshot_read` p95
  `83.02ms`, `workspace_snapshot_read` p95 `68.07ms`,
  `observability_read` p95 `32.11ms`, and `artifact_content_read` p95
  `17.37ms`. Compared with `load-20260501112144`, correctness stayed stable
  but p95 regressed by `61.79ms` and throughput dropped by `6.04`
  workspace/min, dominated by the first activation cold sample (`477.37ms`)
  and readback variance rather than a functional failure. The next optimization
  should target generic post-approval closeout and readback cost, where repeated
  segments still contribute `approval_to_completed` p95 `213.76ms` and
  `snapshot_read` p95 `83.02ms`.
- 2026-05-01: Added activation created Run compressed-context reuse.
  Workspace activation now constructs the just-created Run model in memory and
  passes it to the compressed-context attachment path, avoiding the previous
  immediate repository Get for the same newly persisted Run. Public
  `StartRun` / `ResumeRun` entry points still load the current persisted Run
  because they may operate on historical non-created states; this optimization
  is limited to the activation create-and-start path where the Run state and
  execution context are already known. The formal compressedContext payload,
  Run state-machine behavior, Decision / Approval flow, and DoD validation are
  unchanged. Local verification passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestActivationResponseReusesRunSnapshotRun|TestStartRunWithBackendFailsWhenDefaultLLMProviderUnavailable|TestBuildWorkspaceExecutionContextStartsIndependentReadsConcurrently' -count=1`.
  Deployed quantification was included in batch `load-20260501114230` above
  together with QC46. The removed same-Run repository Get was verified by
  unit test and preserved correctness, but its standalone impact is below the
  current file-backend variance floor in the deployed p95 measurement.
- 2026-05-01: Added closeout deliverable manifest prebuild. During accepted
  DoD closeout, the orchestrator now starts building the Workspace Deliverable
  Manifest content before writing the Result Summary artifact, so manifest file
  inspection can overlap the first artifact write. Artifact / Result
  persistence order remains unchanged: summary artifact is still written before
  deliverable manifest artifact, and the accepted Result is still created only
  after both artifact refs are known. This preserves failure ordering,
  published artifact/result semantics, DoD validation, and business-generic
  orchestration boundaries. Local verification passed with:
  `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_ResultManifestBuildStartsBeforeSummaryArtifactWriteCompletes|TestRuntimeOrchestrator_CompletesLLMRunWithSatisfiedDoDAssessment|TestRuntimeOrchestrator_ResultManifestReusesStepContextManifestWhenNoPatch|TestRuntimeOrchestrator_ResultManifestRescansWorkspaceWhenStepHasPatch' -count=1`.
  After rebuild/redeploy, governed mock `concurrency=1,total=5` batch
  `load-20260501120459` kept 5/5 success with no HTTP / curl / LLM protocol /
  timeline runState errors, measured e2e p95 `1285.37ms`, throughput `59.29`
  workspace/min, `activation_to_decision1` p95 `409.79ms`,
  `decision1->decision2` p95 `97.32ms`, `decision2->approval` p95
  `142.16ms`, `approval->completed` p95 `194.72ms`, `snapshot_read` p95
  `82.71ms`, `workspace_snapshot_read` p95 `52.88ms`,
  `observability_read` p95 `18.27ms`, and `artifact_content_read` p95
  `10.98ms`. Compared with `load-20260501114230`, the targeted closeout
  segment improved by `19.04ms`; workspace snapshot, observability, and
  artifact content reads also improved. e2e p95 still regressed by `38.60ms`
  because the first activation cold sample reached `488.07ms` and
  `decision2->approval` p95 regressed by `14.59ms`. Correctness stayed stable;
  next optimization should continue on generic closeout/readback work, not on
  business Goal / DoD parsing.
- 2026-05-01: Added terminal status wait RunSnapshot in-flight warmup. When
  `/runs/{runId}/status` with `waitState` observes a terminal Run state, the
  HTTP service now starts a terminal RunSnapshot warm read and exposes that
  in-flight read to the following `/snapshot` request instead of allowing an
  immediate duplicate file-backed read. This does not change the status
  response contract or Run state-machine semantics; it only prepares the same
  cached snapshot that `/snapshot`, `/workspaces/{workspaceId}/snapshot`,
  `/observability`, and artifact-content reads already know how to reuse. Local
  verification passed with:
  `go test ./internal/modules/application/workorchd/service/server -run 'TestRunStatusWaitWarmsTerminalRunSnapshotCache|TestRunSnapshotReadWaitsForTerminalStatusWarmupInflight|TestTerminalRunSnapshotCacheReusesSnapshotForReadViews|TestRunStatusWaitUsesStatusChangeNotification' -count=1`.
  Broader verification passed with:
  `go test ./internal/modules/blocks/llm/biz ./internal/pkg/workorchconfig ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz -count=1`
  and `go test ./... -count=1 -p 1`. After rebuild/redeploy, governed mock
  `concurrency=1,total=5` batch `load-20260501122412` kept 5/5 success with no
  HTTP / curl / LLM protocol / timeline runState errors, measured e2e p95
  `1157.26ms`, throughput `64.69` workspace/min,
  `activation_to_decision1` p95 `366.54ms`, `decision1->decision2` p95
  `101.13ms`, `decision2->approval` p95 `120.24ms`,
  `approval->completed` p95 `218.41ms`, `snapshot_read` p95 `48.95ms`,
  `workspace_snapshot_read` p95 `49.68ms`, `observability_read` p95 `20.45ms`,
  and `artifact_content_read` p95 `15.57ms`. Compared with
  `load-20260501120459`, correctness stayed stable, e2e p95 improved by
  `128.11ms`, throughput improved by `5.40` workspace/min, and targeted
  `snapshot_read` p95 improved by `33.76ms`. `approval_to_completed` regressed
  by `23.69ms`, so this optimization should be treated as post-completion
  readback improvement rather than execution closeout improvement.
- 2026-05-01: Added resume compressed-context lightweight projection. Resume no
  longer builds the full read-model `ContextBundle` just to obtain
  `compressedContext`; it reads the RunSnapshot and maps only the deterministic
  recovery summary fields required by LLM continuation and e2e verification.
  Full ContextBundle construction remains unchanged for read APIs and Web
  inspection. Local verification passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/kernel/run/biz ./internal/modules/kernel/orchestrator/biz ./internal/modules/application/workorchd/service/server -count=1`
  and `go test ./... -count=1 -p 1`. After rebuild/redeploy, governed mock
  `concurrency=1,total=5` batch `load-20260501130227` kept 5/5 success with no
  HTTP / curl / LLM protocol / timeline runState errors, measured e2e p95
  `1326.87ms`, throughput `56.39` workspace/min,
  `approval->completed` p95 `248.29ms`, `snapshot_read` p95 `68.24ms`,
  `workspace_snapshot_read` p95 `52.66ms`, `observability_read` p95 `24.93ms`,
  and `artifact_content_read` p95 `18.27ms`. Compared with the earlier
  deployed baseline `load-20260501124211` and intermediate no-gain batch
  `load-20260501125346`, this is correctness-preserving internal cleanup but
  should not be counted as a proven performance win.
- 2026-05-01: Coalesced resume `executionContext` patching with the Run state
  update. Appservice now passes a generic `ExecutionContextPatch` containing
  `compressedContext` into the Run service; the orchestrator merges it into
  the already loaded Run when transitioning from Decision / Approval block to
  `running`, or when closing rejected / superseded paths. This removes the
  previous pre-resume Run repository write while preserving the public
  `Run.executionContext.compressedContext` contract, recovery context, Decision
  / Approval governance, DoD validation, Result / Artifact generation, and
  business-generic module boundaries. Local verification passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/kernel/run/biz ./internal/modules/kernel/orchestrator/biz ./internal/modules/kernel/orchestrator/service ./internal/modules/kernel/orchestrator ./internal/modules/application/workorchd/service/server -count=1`
  and `go test ./... -count=1 -p 1`. After rebuild/redeploy, governed mock
  `concurrency=1,total=5` batch `load-20260501131543` kept 5/5 success with no
  HTTP / curl / LLM protocol / timeline runState errors, measured e2e p95
  `1288.11ms`, throughput `60.38` workspace/min,
  `activation_to_decision1` p95 `414.17ms`, `decision1->decision2` p95
  `112.03ms`, `decision2->approval` p95 `96.42ms`,
  `approval->completed` p95 `213.78ms`, `snapshot_read` p95 `78.75ms`,
  `workspace_snapshot_read` p95 `53.63ms`, `observability_read` p95
  `29.29ms`, and `artifact_content_read` p95 `10.09ms`. Compared with
  `load-20260501130227`, e2e p95 improved by `38.76ms`, throughput improved by
  `3.99` workspace/min, and targeted `approval->completed` p95 improved by
  `34.51ms`. `snapshot_read` regressed by `10.51ms`, so the next optimization
  should continue separating execution closeout cost from post-completion
  readback variance.
- 2026-05-01: Added resume recovery snapshot lightweight read. Resume
  compressed context now uses a recovery-scoped RunSnapshot projection instead
  of the full display snapshot. The recovery projection preserves workspace,
  Run, latest Step, formal Result / Artifact refs, governance, checkpoints,
  recent evidence, task progress, and recovery plan, while skipping display-only
  projections such as selected / used capability views, task decompositions,
  execution sections, observed snapshot, and full Web inspection structures.
  Local verification passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/kernel/run/biz ./internal/modules/kernel/orchestrator/biz ./internal/modules/application/workorchd/service/server -count=1`
  and `go test ./... -count=1 -p 1`. After rebuild/redeploy, governed mock
  `concurrency=1,total=5` batch `load-20260501132551` kept 5/5 success with no
  HTTP / curl / LLM protocol / timeline runState errors, measured e2e p95
  `1527.28ms`, throughput `53.38` workspace/min,
  `activation_to_decision1` p95 `490.98ms`, `decision1->decision2` p95
  `94.75ms`, `decision2->approval` p95 `171.08ms`,
  `approval->completed` p95 `238.12ms`, `snapshot_read` p95 `77.85ms`,
  `workspace_snapshot_read` p95 `63.40ms`, `observability_read` p95
  `21.92ms`, and `artifact_content_read` p95 `15.83ms`. Compared with
  `load-20260501131543`, correctness stayed stable but e2e p95 regressed by
  `239.17ms`; this should be treated as a correctness-preserving internal
  cleanup rather than a proven performance win.
- 2026-05-01: Added graph batch loaded Workspace reuse. Ready-node graph batch
  dispatch now passes the already loaded Workspace through multi-node graph
  dispatch, recursive graph continuation, graph follow-up, LLM completion
  follow-up, formal Result / Artifact materialization, and final
  validating / completed state synchronization. This removes repeated
  Workspace repository reads in a single execution round without changing
  RuntimeAdapter inputs, graph dependency rules, Decision / Approval behavior,
  DoD validation, Result / Artifact contracts, or the business-generic
  orchestrator boundary. Local verification passed with:
  `go test ./internal/modules/kernel/orchestrator/biz -count=1`,
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/kernel/run/biz ./internal/modules/application/workorchd/service/server -count=1`,
  and `go test ./... -count=1 -p 1`. After rebuild/redeploy, governed mock
  `concurrency=1,total=5` batch `load-20260501133118` kept 5/5 success with no
  HTTP / curl / LLM protocol / timeline runState errors, measured e2e p95
  `1344.50ms`, throughput `56.74` workspace/min,
  `activation_to_decision1` p95 `403.82ms`, `decision1->decision2` p95
  `122.40ms`, `decision2->approval` p95 `120.64ms`,
  `approval->completed` p95 `228.08ms`, `snapshot_read` p95 `61.39ms`,
  `workspace_snapshot_read` p95 `55.72ms`, `observability_read` p95
  `21.73ms`, and `artifact_content_read` p95 `14.54ms`. Compared with the
  immediate pre-change batch `load-20260501132551`, e2e p95 improved by
  `182.78ms`, throughput improved by `3.36` workspace/min,
  `activation_to_decision1` p95 improved by `87.17ms`,
  `decision2->approval` p95 improved by `50.44ms`, and `snapshot_read` p95
  improved by `16.45ms`. The closeout segment remains around `228ms`, so the
  next target should be generic Result / Artifact / checkpoint closeout cost or
  file-backend readback variance.
- 2026-05-01: Added closeout Artifact writes parallelization. The DoD-accepted
  closeout path still requires both formal Artifacts before accepted Result
  creation, but the final summary Artifact write and deliverable manifest
  Artifact write now execute concurrently after manifest content is available.
  The accepted Result is still created only after both Artifact writes finish,
  preserving complete `ArtifactRefs`, DoD validation, Result / Artifact
  contracts, event evidence, and the business-generic orchestrator boundary.
  Local verification passed with:
  `go test ./internal/modules/kernel/orchestrator/biz -count=1`,
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/kernel/run/biz ./internal/modules/application/workorchd/service/server -count=1`,
  and `go test ./... -count=1 -p 1`. After rebuild/redeploy, governed mock
  `concurrency=1,total=5` batch `load-20260501133916` kept 5/5 success with no
  HTTP / curl / LLM protocol / timeline runState errors, measured e2e p95
  `1367.97ms`, throughput `57.17` workspace/min,
  `activation_to_decision1` p95 `463.34ms`, `decision1->decision2` p95
  `125.43ms`, `decision2->approval` p95 `122.38ms`,
  `approval->completed` p95 `217.10ms`, `snapshot_read` p95 `72.69ms`,
  `workspace_snapshot_read` p95 `53.19ms`, `observability_read` p95
  `21.67ms`, and `artifact_content_read` p95 `12.68ms`. Compared with
  `load-20260501133118`, targeted `approval->completed` p95 improved by
  `10.98ms` and throughput improved by `0.43` workspace/min. e2e p95 regressed
  by `23.47ms` because the first activation cold sample reached `555.38ms`, so
  this should be counted as a closeout-local improvement rather than a
  full-path e2e win.
- 2026-05-01: Added terminal Run notification snapshot warmup. HTTP write
  paths that already receive a terminal Run now clear stale snapshot cache,
  start a background RunSnapshot warmup, and only then notify status waiters;
  governance background resume applies the same path when it returns a resumed
  terminal Run. This preserves the application service boundary: the HTTP
  service only warms its read-model cache and does not change Run state,
  Decision / Approval governance, DoD validation, Result / Artifact contracts,
  or orchestrator business behavior. Local verification passed with:
  `go test ./internal/modules/application/workorchd/service/server -run 'TestNativeBusinessRoutesInjectEventHub|TestRunStatusNotificationForTerminalRunWarmsSnapshotCache|TestRunStatusWaitWarmsTerminalRunSnapshotCache|TestRunSnapshotReadWaitsForTerminalStatusWarmupInflight|TestRunStatusWaitUsesStatusChangeNotification' -count=1`,
  `go test ./internal/modules/application/workorchd/service/server ./internal/modules/application/workorchd/biz/appservice ./internal/modules/kernel/orchestrator/biz ./internal/modules/kernel/run/biz -count=1`,
  and `go test ./... -count=1 -p 1`. After rebuild/redeploy, governed mock
  `concurrency=1,total=5` batch `load-20260501135110` kept 5/5 success with no
  HTTP / curl / LLM protocol / timeline runState errors, measured e2e p95
  `1482.98ms`, throughput `57.56` workspace/min,
  `activation_to_decision1` p95 `547.19ms`, `approval->completed` p95
  `214.28ms`, `snapshot_read` p95 `50.30ms`,
  `workspace_snapshot_read` p95 `47.78ms`, `observability_read` p95
  `18.75ms`, and `artifact_content_read` p95 `18.66ms`. Compared with
  `load-20260501133916`, targeted readback improved:
  `snapshot_read` p95 improved by `22.39ms`,
  `workspace_snapshot_read` p95 improved by `5.41ms`, and
  `approval->completed` p95 improved by `2.82ms`. e2e p95 regressed because
  the first activation cold sample was `663ms`, so this is counted as a
  readback-local improvement rather than a full-path stability win.
- 2026-05-01: Added Decision resolution read-model parallelization. Run and
  Workspace snapshot projection now read resolutions for resolved tickets in
  parallel before mapping Decision views, avoiding serial resolution reads for
  multi-round governance history. This remains inside the application
  read-model boundary: it does not change Ticket / Resolution persistence,
  Decision or Approval state transitions, Run scheduling, DoD validation, or
  Result / Artifact closeout behavior. Local verification passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestReadRunSnapshotReadsDecisionResolutionsConcurrently|TestReadRunSnapshotWithBackendReturnsBlockedDecisionReadModel|TestReadWorkspaceSnapshotWithBackendReturnsRelatedRunsAndBlockingItems|TestReadWorkspaceSnapshotWithRunSnapshotReusesProjectionSections' -count=1`,
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz ./internal/modules/kernel/run/biz -count=1`,
  and `go test ./... -count=1 -p 1`. Deployed quantification is pending the
  next rebuild; expected impact is limited to snapshot / observability readback
  for workspaces with multiple resolved Decision tickets.
- 2026-05-01: Quantified the deployed QC56 Decision resolution read-model
  parallelization with governed mock `concurrency=1,total=5` batch
  `load-20260501140219`. The batch kept 5/5 success with no HTTP / curl / LLM
  protocol / timeline runState errors, measured e2e p95 `1276.33ms`,
  throughput `61.73` workspace/min, `activation_to_decision1` p95
  `423.28ms`, `decision1->decision2` p95 `87.56ms`,
  `decision2->approval` p95 `118.09ms`, `approval->completed` p95
  `200.09ms`, `snapshot_read` p95 `60.04ms`,
  `workspace_snapshot_read` p95 `52.78ms`, `observability_read` p95
  `17.99ms`, and `artifact_content_read` p95 `14.09ms`. Compared with
  `load-20260501135110`, e2e p95 improved by `206.65ms`, throughput improved
  by `4.17` workspace/min, `activation_to_decision1` improved by `123.91ms`,
  `decision2->approval` improved by `~2.59ms`, and `approval->completed`
  improved by `14.19ms`; however `snapshot_read` regressed by `9.74ms` and
  `workspace_snapshot_read` regressed by `5.00ms`. Treat this as an overall
  successful regression batch with no stable readback-local win. The next
  optimization should target execution closeout or activation cold-path work
  rather than additional Decision read-model micro-optimizations.
- 2026-05-01: Added terminal Run / Workspace completion write
  parallelization. After a DoD-accepted Result and its Artifact refs have been
  persisted, the orchestrator now writes final Run `completed` state and
  Workspace release/completed state concurrently, then publishes events in the
  original external order: Run completed before Workspace completed. This
  keeps the Result / Artifact dependency, DoD validation gate, Run terminal
  semantics, Workspace active-run cleanup, and event ordering intact while
  reducing file-backend terminal closeout wait. Local verification passed with:
  `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_FinalRunAndWorkspaceCompletionWritesConcurrently|TestRuntimeOrchestrator_CompletesLLMRunWithSatisfiedDoDAssessment|TestRuntimeOrchestrator_StartRunPublishesRunAndStepEvents|TestRuntimeOrchestrator_ResultArtifactsWriteConcurrentlyBeforeAcceptedResult' -count=1`,
  `go test ./internal/modules/kernel/orchestrator/biz ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/run/biz -count=1`,
  and `go test ./... -count=1 -p 1`. Deployed quantification is pending the
  next rebuild; expected impact is limited to `approval_to_completed` /
  toolExecutionAndResultMaterialization.
- 2026-05-01: Quantified the deployed QC57 terminal Run / Workspace completion
  write parallelization with governed mock `concurrency=1,total=5` batch
  `load-20260501141235`. The batch kept 5/5 success with no HTTP / curl / LLM
  protocol / timeline runState errors, measured e2e p95 `1297.17ms`,
  throughput `59.79` workspace/min, `activation_to_decision1` p95
  `432.02ms`, `decision1->decision2` p95 `101.67ms`,
  `decision2->approval` p95 `136.53ms`, `approval->completed` p95
  `228.15ms`, `snapshot_read` p95 `41.25ms`,
  `workspace_snapshot_read` p95 `52.23ms`, `observability_read` p95
  `19.57ms`, and `artifact_content_read` p95 `10.42ms`. Compared with
  `load-20260501140219`, e2e p95 regressed by `20.84ms`, throughput regressed
  by `1.94` workspace/min, and targeted `approval->completed` p95 regressed by
  `28.06ms`; however `snapshot_read` improved by `18.79ms`,
  `status_read` improved by `15.50ms`, and `artifact_content_read` improved by
  `3.67ms`. Treat this as a correctness-preserving closeout refactor with no
  stable closeout-local performance win. The next optimization should target
  post-completion readback cache stability and activation cold-start variance,
  not additional terminal write parallelization.
- 2026-05-01: Added activation Goal / DoD definition read parallelization. The
  activation service now reads candidate Goal and DoD definitions concurrently
  before applying the existing selection rules. This stays inside the
  application service boundary: it does not change how Goal or DoD are chosen,
  does not infer business intent from Goal text, and does not change Run
  creation, LLM execution, Decision / Approval governance, or DoD validation.
  Local verification passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestActivationDefinitionSelectionReadsGoalAndDoDConcurrently|TestActivationContextUsesSelectedGoalAndDoDWithoutReload|TestActivationResponseReusesRunSnapshotRun|TestBuildWorkspaceExecutionContextStartsIndependentReadsConcurrently' -count=1`,
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz ./internal/modules/kernel/run/biz -count=1`,
  and `go test ./... -count=1 -p 1`. Deployed quantification is pending the
  next rebuild; expected impact is limited to `activation_to_decision1` cold
  path and should be evaluated against first-sample p95 variance.
- 2026-05-01: Quantified the deployed QC58 activation Goal / DoD definition
  read parallelization with governed mock `concurrency=1,total=5` batch
  `load-20260501142037`. The batch kept 5/5 success with no HTTP / curl / LLM
  protocol / timeline runState errors, measured e2e p95 `1408.92ms`,
  throughput `55.60` workspace/min, `activation_to_decision1` p95
  `463.82ms`, `decision1->decision2` p95 `105.39ms`,
  `decision2->approval` p95 `125.89ms`, `approval->completed` p95
  `275.31ms`, `snapshot_read` p95 `58.27ms`,
  `workspace_snapshot_read` p95 `59.61ms`, `observability_read` p95
  `32.69ms`, and `artifact_content_read` p95 `15.15ms`. Compared with
  `load-20260501141235`, e2e p95 regressed by `111.74ms`, throughput regressed
  by `4.19` workspace/min, and targeted `activation_to_decision1` regressed by
  `31.80ms`; only `decision2->approval` improved by `10.64ms`. Treat this as a
  correctness-preserving activation refactor with no stable activation-local
  performance win. The next optimization should reduce post-governance resume
  readback / response cost or improve deployed measurement granularity before
  adding more activation micro-optimizations.
- 2026-05-01: Added QC59 loadtest slow sample diagnostics to improve
  measurement granularity before additional single-workspace micro-optimizing.
  The governed loadtest summary now records per-workspace `timingByStage`,
  the slowest workspaces by e2e duration, and capped per-stage slow samples for
  activation, governance transitions, completion, and readback stages. This is
  a test/observability improvement only; it does not change Run scheduling,
  Decision / Approval governance, DoD validation, Result / Artifact materialize,
  RuntimeCheckpoint emission, or workspace business semantics. Verification
  passed with `python3 -m py_compile scripts/loadtest-governed-workspace-concurrency.py`
  and deployed governed mock `concurrency=1,total=5` batch
  `load-20260501142530`. The batch kept 5/5 success with no HTTP / curl / LLM
  protocol / timeline runState errors, measured e2e p95 `1057.53ms`,
  throughput `58.32` workspace/min, `activation_to_decision1` p95
  `115.97ms`, `decision1->decision2` p95 `105.23ms`,
  `decision2->approval` p95 `138.90ms`, `approval->completed` p95
  `285.84ms`, `snapshot_read` p95 `69.80ms`,
  `workspace_snapshot_read` p95 `66.50ms`, `observability_read` p95
  `22.60ms`, and `artifact_content_read` p95 `17.49ms`. The new report fields
  confirmed expected output in
  `/tmp/workorch-loadtest-load-20260501142530-_9fhwrde/summary.json` and
  `/tmp/workorch-loadtest-load-20260501142530-_9fhwrde/report.md`.
- 2026-05-01: Added QC60 resolved governance resume lightweight compressed
  context. `ResumeRunWithSession` can now build a lightweight deterministic
  compressed context for blocked runs only when the current blocker is known to
  continue execution: Decision Ticket state is `resolved` with
  `provide_missing_info`, or Approval state is `approved`. Pending,
  rejected, replan, invalid, or non-blocked runs still fall back to the full
  recovery snapshot path, preserving richer evidence for terminal / exceptional
  outcomes. This remains inside the application service boundary: it does not
  change Run state transitions, orchestrator recovery validation, Ticket /
  Approval resolution semantics, LLM step input rules, DoD validation, Result /
  Artifact creation, or RuntimeCheckpoint emission. Local verification passed
  with:
  `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestCompressedContextForRunStartUsesLightweightCreatedRunProjection|TestCompressedContextForBlockedRunResumeUsesLightweightGovernanceProjection|TestResumeRunWithBackendReturnsFailedRunAfterApprovalRejected|TestResumeRunWithBackendReturnsBlockedWhenDependencyPending|TestResumeRunWithBackendReturnsConflictForCreatedRun' -count=1`,
  `python3 -m py_compile scripts/loadtest-governed-workspace-concurrency.py`,
  and
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz ./internal/modules/kernel/run/biz -count=1`.
  Deployed quantification is pending the next rebuild; expected impact is
  limited to governance resume stages (`decision1_to_decision2`,
  `decision2_to_approval`, and `approval_to_completed`) by avoiding a full
  recovery snapshot build before continuing the already-resolved path.
- 2026-05-01: Quantified deployed QC60 with governed mock
  `concurrency=1,total=5` batch `load-20260501143821`. The batch kept 5/5
  success with no HTTP / curl / LLM protocol / timeline runState errors,
  measured e2e p95 `1353.92ms`, throughput `60.53` workspace/min,
  `activation_to_decision1` p95 `464.49ms`, `decision1->decision2` p95
  `98.55ms`, `decision2->approval` p95 `127.79ms`,
  `approval->completed` p95 `191.53ms`, `snapshot_read` p95 `51.95ms`,
  `workspace_snapshot_read` p95 `62.93ms`, `observability_read` p95
  `18.38ms`, and `artifact_content_read` p95 `10.38ms`. Compared with
  `load-20260501142530`, the targeted governance resume / closeout path
  improved: `decision1->decision2` by `6.67ms`, `decision2->approval` by
  `11.11ms`, and `approval->completed` by `94.31ms`; readback also improved
  for snapshot, observability, and artifact content reads. e2e p95 regressed by
  `296.39ms` because the first activation sample took `550.54ms` while the
  remaining activation samples were `73.92-120.30ms`. Treat QC60 as a
  successful target-stage optimization; the next service-side work should focus
  on activation cold-path variance and context assembly, not additional
  governance resume shortcuts.
- 2026-05-01: Added QC61 activation MCP context read parallelization. After
  Skill selection and required MCP / Capability auto-binding have produced the
  final ID sets, activation now reads selected MCP Servers and selected MCP
  Capabilities concurrently before building context and running Soul capability
  validation. This keeps all business semantics unchanged: Skill selection,
  required MCP binding, explicit user selections, disabled MCP rejection, Soul
  permission validation, Run creation, Decision / Approval governance, DoD
  validation, Result / Artifact materialization, and RuntimeCheckpoint emission
  are unchanged. Local verification passed with:
  `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestActivationContextReadsMCPServersAndCapabilitiesConcurrently|TestActivateWorkspaceExecutionWithBackendBindsSelectedContext|TestActivateWorkspaceExecutionWithBackendAutoSelectsSkillAndBindsRequiredMCP|TestBuildWorkspaceExecutionContextStartsIndependentReadsConcurrently' -count=1`,
  `go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/orchestrator/biz ./internal/modules/kernel/run/biz -count=1`,
  and `go test ./... -count=1 -p 1`. Deployed quantification is pending the
  next rebuild; expected impact is limited to activation context assembly for
  runs with selected or Skill-required MCP Servers and Capabilities.
- 2026-05-01: Quantified deployed QC61 with governed mock
  `concurrency=1,total=5` batch `load-20260501145030`. The batch kept 5/5
  success with no HTTP / curl / LLM protocol / timeline runState errors,
  measured e2e p95 `1571.67ms`, throughput `53.34` workspace/min,
  `activation_to_decision1` p95 `537.91ms`, `decision1->decision2` p95
  `129.86ms`, `decision2->approval` p95 `157.06ms`,
  `approval->completed` p95 `229.30ms`, `snapshot_read` p95 `50.71ms`,
  `workspace_snapshot_read` p95 `50.92ms`, `observability_read` p95
  `28.65ms`, and `artifact_content_read` p95 `18.51ms`. Compared with
  `load-20260501143821`, target activation p95 regressed by `73.42ms` because
  the first activation cold sample reached `643.04ms`; the remaining activation
  samples were `89.50-117.39ms`, close to the prior non-cold range. Treat QC61
  as a correctness-preserving context assembly improvement with no stable
  deployed p95 win. The next optimization should target resume / closeout
  path cost and avoid further activation micro-optimizations until cold-start
  attribution is available.
- 2026-05-01: Added QC62 resume dispatch synced Workspace reuse. When
  governance resume transitions a Run back to `running`, the orchestrator
  already loads and updates the Workspace while syncing Workspace projection
  state. Resumed dispatch now reuses that loaded Workspace instead of letting
  `dispatchStepWithWorkspace` read it again from the repository. This preserves
  Run state transitions, Workspace active-run validation, Decision / Approval
  resolution semantics, runtime execution input, DoD validation, Result /
  Artifact materialization, and event ordering; it only removes a redundant
  repository read from the normal resume dispatch path. Local verification
  passed with:
  `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_ResumeRunDispatchReusesSyncedWorkspace|TestRuntimeOrchestrator_StartRunDispatchReusesLoadedWorkspace|TestRuntimeOrchestrator_GraphBatchDispatchReusesLoadedWorkspace|TestRuntimeOrchestrator_ValidateCompleteReusesProvidedWorkspace' -count=1`
  and
  `go test ./internal/modules/kernel/orchestrator/biz ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./internal/modules/kernel/run/biz -count=1`.
  Deployed quantification is pending the next rebuild; expected impact is
  limited to governance resume stages (`decision1_to_decision2`,
  `decision2_to_approval`, `approval_to_completed`) on file backend.
- 2026-05-01: Quantified deployed QC62 with governed mock
  `concurrency=1,total=5` batch `load-20260501150322`. The batch kept 5/5
  success with no HTTP / curl / LLM protocol / timeline runState errors,
  measured e2e p95 `1399.72ms`, throughput `53.96` workspace/min,
  `activation_to_decision1` p95 `488.54ms`, `decision1->decision2` p95
  `111.12ms`, `decision2->approval` p95 `135.58ms`,
  `approval->completed` p95 `219.88ms`, `snapshot_read` p95 `52.82ms`,
  `workspace_snapshot_read` p95 `51.48ms`, `observability_read` p95
  `17.27ms`, and `artifact_content_read` p95 `20.79ms`. Compared with
  `load-20260501145030`, e2e p95 improved by about `171.94ms`,
  activation p95 improved by about `49.37ms`, `decision1->decision2` by
  about `18.74ms`, `decision2->approval` by about `21.48ms`,
  `approval->completed` by about `9.42ms`, and throughput improved by
  about `0.62` workspace/min. The remaining closeout hotspot is still
  `approval->completed` around `219.88ms`; next optimization should target
  terminal validating / completion writes while preserving Result / Artifact,
  RuntimeCheckpoint, and event ordering semantics.
- 2026-05-01: Added QC63 validating Run / Workspace write parallelization.
  Final closeout previously wrote Run `validating` first, emitted the Run
  event / RuntimeCheckpoint, then wrote Workspace `validating` and emitted the
  Workspace event. The write portion now updates Run and Workspace concurrently,
  while event publication still happens afterward in the same Run then
  Workspace order. This preserves DoD assessment, accepted Result creation,
  Artifact materialization, terminal completion, RuntimeCheckpoint semantics,
  Workspace active-run conflict checks, and Web-visible event order; it only
  removes serial file backend write latency from the validating state sync.
  Local verification passed with
  `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_ValidationRunAndWorkspaceWritesConcurrently|TestRuntimeOrchestrator_ValidateCompleteReusesProvidedWorkspace|TestRuntimeOrchestrator_FinalRunAndWorkspaceCompletionWritesConcurrently|TestRuntimeOrchestrator_ResumeRunDispatchReusesSyncedWorkspace' -count=1`.
  Deployed quantification is pending the next rebuild; expected impact is
  limited to the closeout segment, especially `approval_to_completed` on file
  backend.
- 2026-05-01: Added QC64 completed Step evidence write parallelization. After
  a runtime step succeeds, ToolCall `completed` and ValidationResult `passed`
  are independent evidence records that must both exist before Step completed
  persistence and event publication. The orchestrator now writes those two
  evidence records concurrently and preserves all downstream ordering: Step is
  still updated only after both evidence writes complete, Step completed event
  is still published after Step persistence, and RuntimeCheckpoint /
  Decision / Approval / Result / Artifact behavior is unchanged. Local
  verification passed with
  `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_CompletedStepEvidenceWritesConcurrently|TestRuntimeOrchestrator_StartRunToComplete_Mainline|TestRuntimeOrchestrator_CompletesLLMRunWithSatisfiedDoDAssessment|TestRuntimeOrchestrator_ValidationRunAndWorkspaceWritesConcurrently' -count=1`.
  Deployed quantification is pending the next rebuild; expected impact is
  distributed across each successful step segment and should be visible in
  governance transition p95 plus `approval_to_completed`.
- Completed items should be marked in this document with verification evidence.
- Broad future demand candidates remain in `docs/next-stage-demand-candidates.md`.
- Previously closed MVP scope remains in `docs/mvp-backlog.md` and should not be
  reopened unless the acceptance target itself changes.
