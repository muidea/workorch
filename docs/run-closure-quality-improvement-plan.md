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
| QC11 | Performance regression scenario for concurrent governance Runs | P1 | planned | Load test with ticket and approval |

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
- Completed items should be marked in this document with verification evidence.
- Broad future demand candidates remain in `docs/next-stage-demand-candidates.md`.
- Previously closed MVP scope remains in `docs/mvp-backlog.md` and should not be
  reopened unless the acceptance target itself changes.
