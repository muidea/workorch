# workorch Next MVP Acceptance

## Goal

The next MVP milestone closes the gap between the current deployable `workorchd` service and the WorkOrch product planning baseline under `/home/fedquery/aispace/clawKnowledge/xiaozhua/workorch`.

Current service MVP remains valid and closed. This document defines only the next MVP acceptance scope, not every deviation from the long-term product planning baseline.

## Product Baseline

Authoritative planning inputs:

- `/home/fedquery/aispace/clawKnowledge/xiaozhua/workorch/product/workorch-functional-definition-2026-03-18.md`
- `/home/fedquery/aispace/clawKnowledge/xiaozhua/workorch/implementation/workorch-p0-object-model-minimal-closure-checklist-v1-2026-03-19.md`
- `/home/fedquery/aispace/clawKnowledge/xiaozhua/workorch/architecture/workorch-state-machine-design-2026-03-18.md`
- `/home/fedquery/aispace/clawKnowledge/xiaozhua/workorch/application/workorch-runservice-orchestrator-interface-draft-v1.md`
- `/home/fedquery/aispace/clawKnowledge/xiaozhua/workorch/infra/workorch-repository-interface-draft-v1.md`
- `/home/fedquery/aispace/clawKnowledge/xiaozhua/workorch/implementation/workorch-event-topic-payload-spec-v1-2026-03-19.md`

## Current Coverage

Already covered by the closed service MVP:

- Workspace creation, listing, detail, and snapshot.
- Goal and DoD definition under Workspace.
- Run creation, listing, detail, snapshot, status, start, and resume.
- Decision Ticket list/detail/resolve and blocked Run resume.
- Approval list/detail/approve/reject and blocked Run resume.
- Memory and file repository backends with restart-read verification.
- Embedded Ant Design Pro style management console.
- `magicCommon/framework/application`, plugin module lifecycle, `magicCommon/event.Hub`, and `magicEngine/http` route registry integration.

## Next MVP Scope

### N1. Execution Evidence And Run Snapshot

Status: completed

Target:

- Add formal `ToolCall`, `Patch`, `ValidationResult`, and `RuntimeCheckpoint` models under `kernel/run`.
- Add repository interfaces and memory/file implementations for these objects.
- Include repository contract tests for create/get/list/update semantics where applicable.
- Extend Run snapshot to include execution evidence grouped by `runId` and `stepId`.
- Keep `Step` as Run aggregate internal entity; evidence objects remain execution evidence, not primary business state.
- Expose the evidence repositories through the workorch runtime assembly exports so appservice read models can consume them.
- Record runtime execution evidence automatically from the orchestrator execution path:
  `ToolCall` for step execution, `ValidationResult` for runtime step result, and
  `RuntimeCheckpoint` for run/step state changes. `Patch` is recorded when the
  runtime emits a formal workspace file patch request.

Acceptance:

- `go test ./internal/pkg/repository ./internal/pkg/repository/memory ./internal/pkg/repository/file -count=1` passes.
- File backend persists each execution evidence object type into its own JSON file.
- Mutating returned objects must not mutate repository state.
- `GET /api/v1/workorch/runs/{runId}/snapshot` can return steps plus tool calls, patches, validation results, and checkpoints when present.
- Starting a workspace execution through `workorchd` creates runtime evidence without tests or callers manually inserting evidence records.
- File backend restart preserves and returns the same evidence view.

### N2. Run State Machine And Workspace Projection

Status: completed

Target:

- Align the current simplified states with the product planning state machine without over-expanding the service.
- Add the next necessary states for `Run` and `Workspace` projection, especially validating, cancelled, and superseded.
- Keep `Workspace.state` as a projected work state, not a duplicate of `Run.state`.

Acceptance:

- Workspace snapshot clearly reflects active Run blocked, running, completed, or failed projection.
- Run state transitions reject invalid moves with stable conflict errors.
- Existing service verification remains green.

### N3. Decision Ticket Structure And Resolution Semantics

Status: completed

Target:

- Enhance Decision Ticket structure enough to support actionable resolution.
- Expand Decision Resolution beyond approve/reject paths to at least cover:
  `replan`, `reject_current_path`, and `provide_missing_info`.
- Keep batch decision out of scope until single-ticket resolution semantics are stable.

Acceptance:

- A decision can drive continue, fail, or supersede behavior through explicit resolution type.
- Resolution payload is persisted and visible in decision detail.

### N4. Control Plane MVP Views

Status: completed

Target:

- Evolve the embedded console from resource pages into the planned control-plane views:
  Workspace Board, Decision Queue, and Workspace Detail.
- Keep the existing pages if useful, but primary navigation should match the product workflow.

Acceptance:

- User can see active Workspace status from a board-like view.
- User can process pending Decision Tickets from a queue-like view.
- User can inspect Workspace Detail with Goal, DoD, Run, Decision, Approval, and execution evidence context.

## Non-Goals For Next MVP

- Any long-term product-planning deviation not explicitly listed in the four Next MVP scope items.
- Full knowledge asset, retrieval, template marketplace, enterprise collaboration, multi-tenant governance, and Sub-Agent orchestration.
- Batch decision handling before single-ticket resolution semantics are stable.
- Distributed scheduling, external runtime marketplace, external worker fleets,
  and multi-tenant execution quota.

## Progress

- completed: `N1 Execution Evidence And Run Snapshot`
  Runtime execution now writes `ToolCall`, `ValidationResult`, and
  `RuntimeCheckpoint` records automatically; `Patch` is recorded when runtime
  output contains a formal workspace file patch request.
- completed: `N2 Run State Machine And Workspace Projection`
- completed: `N3 Decision Ticket Structure And Resolution Semantics`
- completed: `N4 Control Plane MVP Views`

## Post-Completion Feature Progress

- completed: `Runtime Requested Workspace File Patch`
  When runtime step output includes formal `patches` entries, the orchestrator
  writes the target file under the Workspace working directory and records an
  applied `Patch` linked to the runtime `ToolCall`. Invalid absolute or escaping
  paths fail the Run and record failed Patch evidence.
- completed: `Runtime Expansion`
  Controlled shell execution, generalized diff patch apply, formal block
  execution graph, external runtime adapter contract, and runtime observability
  are tracked as completed in `docs/runtime-expansion-plan.md`.
- completed: `LLM Graph Closure`
  Real-provider LLM decisions can emit validated `executionGraph` output; graph
  leaf completion dispatches an LLM follow-up for evidence-backed DoD
  assessment.
- completed: `Advanced Runtime And Scheduling`
  MCP resources, prompts, streaming evidence, long-lived stdio session reuse,
  and priority-aware local run scheduling are tracked as completed in
  `docs/advanced-runtime-scheduling-plan.md`.
- completed: `Decision Batch Resolution`
  Decision Queue now supports resolving multiple open Decision Tickets in one
  formal batch operation. The API validates all requested tickets before
  mutation, rejects duplicate or already resolved tickets, preserves request
  order in the response, and applies the same resolution semantics as single
  ticket resolution.
