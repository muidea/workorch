# Runtime Contract Retry Batch 2 Todo / ExecutionGraph Audit

## Scope

This is the Batch 2 audit required by `runtime-contract-retry-full-chain-audit-plan.md`.

Goal:

- Confirm how LLM todo / `executionGraph` is created, persisted, dispatched, retried, rejected, and projected.
- Identify the gap between raw runtime progress and a stable planned-task ViewModel.

This document audits projection and read-model behavior only. It does not encode any software-development-specific rule in core.

## Expected Todo / ExecutionGraph Lifecycle

| Lifecycle Node | Expected Model | Expected Evidence |
| --- | --- | --- |
| LLM emits executable plan | `executionGraph.nodes[]` with node id, kind, dependencies, coverage targets, and validation/patch contract metadata | LLM step output and output contract validation |
| Plan accepted for dispatch | first-class run task plan revision | plan revision record or projected task plan |
| Node dispatched | task node state changes from `planned` to `dispatched/running` | `/step/dispatched`, `graphRootStepId`, `graphNodeId` |
| Node terminal result | task node becomes `completed`, `failed`, or `rejected` | step result, tool call, validation, patch, contract violation |
| Retry generated | retry task links back to failed task/node and violation type | contract violation events and retry step input/output |
| Decision opened | task plan becomes `blocked_on_decision` with ticket id | ticket event and projection |
| Projection consumed | TUI/Web receive planned-task ViewModel only | `taskPlans`, `taskDecompositions`, `taskProgress` |

## Implementation Evidence

| Evidence | Implementation Finding |
| --- | --- |
| `ev:code/projection/task-derived/001` | `MapTaskDecompositions` iterates `snapshot.Steps` and extracts `step.Output["executionGraph"]`; there is no independent persisted task-plan owner. |
| `ev:code/projection/task-node-mapping/001` | Node execution state is inferred by matching dispatched steps through `graphRootStepId` and `graphNodeId`. |
| `ev:code/projection/task-plan/001` | `TaskPlan` is projected from task decomposition, grouped by anchor step, and labeled `SourceProjection=task_decomposition`. |
| `ev:code/projection/progress-mixed-items/001` | `TaskProgressItems` mixes steps, tool calls, patches, validation results, results, artifacts, decisions, and approvals in one list. |
| `ev:ws4e/run4/ctr-validation-003/output/001` | A valid-looking controlled-substitute execution graph existed, but projection cannot represent it as a blocked planned task separate from raw failed retry step. |
| `ev:ws7c/run4/ctr-statechange-005/output/001` | Final retry had no execution graph and returned DoD assessment, but the projection needs to show this as retry exhaustion on the original planned task. |

Closure status: the findings below record the original audited gaps. The
2026-06-21 closure records later in this file close the ViewModel/projection
portion by adding projected `TaskPlans` lifecycle metadata and workspace/client
transport plumbing. A separate persisted task-plan owner remains a conditional
future option only if later runtime verification proves snapshot-derived
projection insufficient.

## Current Projection Behavior

Original audited code derived task projection from observed runtime artifacts:

1. `MapTaskDecompositions(snapshot)` scans `snapshot.Steps`.
2. `taskDecompositionFromStep` extracts `executionGraph` from `step.Output`.
3. `taskExecutionStepsByRoot` maps later dispatched steps back to the graph by `graphRootStepId` and `graphNodeId`.
4. `MapTaskPlans` converts decompositions into `TaskPlan` revisions.
5. `TaskProgressItems` separately emits raw step/tool/patch/validation/result/artifact/decision/approval items.

Before the 2026-06-21 closure, this was useful as a derived read model, but it
did not fully satisfy the target design from the convergence plan:

- planned tasks are not first-class before dispatch
- retry tasks are not first-class with violation metadata
- rejected outputs are not consistently shown as rejected plan revisions
- raw progress items and planned todo items are not clearly separated for clients

## Batch 2 Findings

### B2-TASK-001: Todo Exists, But Is Derived From Step Output

- Claim: WorkOrch can identify execution graphs from LLM outputs.
- Expected behavior: accepted LLM execution graph should become a stable planned task model before or at dispatch.
- Actual behavior: the read model derives task decomposition from `step.Output["executionGraph"]` after that step is already present in the snapshot.
- Evidence: `ev:code/projection/task-derived/001`.
- Deviation type: `planned_task_projection_gap`.
- Owner: projection / read model, with possible core event support.
- Closure batch: 2.
- Required action: define whether task plans are persisted as owner events or remain derived projections with stronger lifecycle metadata.
- Closure status: code-closed for the audited ViewModel requirement by keeping task plans as projection-owned read model data and adding stronger lifecycle metadata. Persisted owner/event storage is not required unless later verification proves projection cannot recover a required transition.

### B2-TASK-002: Node State Is Inferred From Dispatched Steps

- Claim: node state is currently inferred rather than explicitly owned.
- Expected behavior: every graph node should have a stable node state: `planned`, `dispatched`, `running`, `completed`, `failed`, `rejected`, `retrying`, or `blocked_on_decision`.
- Actual behavior: node state derives from matching `graphRootStepId` / `graphNodeId` on later steps. Nodes with no matching dispatched step remain pending even if blocked by contract rejection before dispatch.
- Evidence: `ev:code/projection/task-node-mapping/001`.
- Deviation type: `task_node_state_inference_gap`.
- Owner: projection / read model, possibly orchestrator task-plan event source.
- Closure batch: 2.
- Required action: add explicit rejected/blocked/retrying metadata from contract violation events into task node projection.
- Closure status: code-closed through projected node lifecycle metadata for rejected, retrying, retry-exhausted, and blocked-on-decision states.

### B2-TASK-003: Raw Progress Items Are Mixed With Planned Work

- Claim: current `TaskProgress.Items` is a raw activity rollup, not a clean planned todo list.
- Expected behavior: UI should render planned task state from a planned-task ViewModel, and render raw timeline separately.
- Actual behavior: steps, tool calls, patches, validations, results, artifacts, decisions, and approvals are appended into one progress item list.
- Evidence: `ev:code/projection/progress-mixed-items/001`.
- Deviation type: `planned_task_projection_gap`.
- Owner: projection / ViewModel.
- Closure batch: 2.
- Required action: keep `TaskProgress.Items` as activity if still useful, but expose `TaskPlans` / `TaskDecompositions` as the primary todo surface with stable states.
- Closure status: code-closed; `TaskProgress.Items` remains activity/timeline, while `TaskPlans` is exposed through run snapshot, workspace snapshot, port/client DTOs, embedded backend, and TUI display cache.

### B2-TASK-004: Retry And Contract Violation Are Not First-Class Plan Revisions

- Claim: retry work exists in raw events and steps, but is not modeled as a first-class plan revision with violation type and target scope.
- Expected behavior: a retry should link to the source plan/node, violation type, target scope, attempt number, and resulting task state.
- Actual behavior: retry can appear as `contract-retry-*` step and contract violation events, but the projection does not consistently model the retry as a task revision.
- Evidence: `ev:ws4e/events/all-runs/001`, `ev:ws7c/events/all-runs/001`, `ev:code/projection/task-plan/001`.
- Deviation type: `retry_task_projection_gap`.
- Owner: projection / orchestrator event metadata.
- Closure batch: 2 and 4.
- Required action: add retry metadata to task plan projection, sourced from contract violation/retry events and retry step lineage.
- Closure status: code-closed for projection; retry metadata is projected from neutral contract violation and retry lineage fields without template-specific rules.

### B2-TASK-005: Contract-Rejected ExecutionGraph Needs Rejected Plan State

- Claim: an execution graph can be generated but rejected by runtime contract before meaningful dispatch.
- Expected behavior: the generated graph should be visible as `rejected` with violation type, not only as a failed LLM step.
- Actual behavior: `ws-4e` generated an 11-node controlled-substitute graph, but the visible terminal state is a failed LLM retry step and decision blocker.
- Evidence: `ev:ws4e/run4/ctr-validation-003/output/001`.
- Deviation type: `rejected_plan_projection_gap`.
- Owner: core contract enforcement and projection.
- Closure batch: 2 and 3.
- Required action: when output contract rejects a graph, project the graph as rejected diagnostic task evidence without executing it or accepting it as result/artifact.
- Closure status: code-closed for projection; rejected execution graphs project as rejected diagnostic task plans/nodes and do not become accepted result/artifact state.

### B2-TASK-006: Missing Graph Retry Needs Original Plan Link

- Claim: a retry can return no `executionGraph` while still being semantically tied to a prior failed planned task.
- Expected behavior: the retry exhaustion should update the original plan/node to `blocked_on_decision` or `retry_exhausted`.
- Actual behavior: `ws-7c` final retry returned no graph and three DoD items, so projection cannot naturally attach it to a todo node without explicit lineage metadata.
- Evidence: `ev:ws7c/run4/ctr-statechange-005/output/001`.
- Deviation type: `retry_lineage_projection_gap`.
- Owner: core retry metadata and projection.
- Closure batch: 2 and 4.
- Required action: carry source step/node/target scope through retry output and projection.
- Closure status: code-closed for projection; retry steps without a new graph create a contract-violation retry plan revision tied to the original anchor and can project `blocked_on_decision`.

## Planned Task ViewModel Requirements

Status: code-closed for the audited ViewModel path.

The 2026-06-21 closure defines the business-neutral planned task ViewModel
through `TaskPlan` and `TaskDecompositionNode`. The table below remains as the
contract checklist for future regressions; it is not an open request for a new
task-plan owner unless later runtime verification proves snapshot-derived
projection cannot recover a required lifecycle transition.

| Field | Required Semantics |
| --- | --- |
| `workspaceId` | workspace scope; implemented on `TaskPlan` and `TaskDecompositionNode`'s parent `TaskDecomposition` |
| `runId` | run scope; implemented on `TaskPlan` and `TaskDecompositionNode`'s parent `TaskDecomposition` |
| `planId` | stable plan id; implemented as `TaskPlan.ID` |
| `revision` | monotonic plan revision; implemented as `TaskPlan.Revision` |
| `sourceStepId` | LLM step that produced or revised the plan; implemented as `AnchorStepID` / `RevisionStepID` |
| `sourceOutputState` | accepted, rejected, retrying, blocked; implemented as `SourceOutputState` |
| `nodeId` | graph node id; implemented as `TaskDecompositionNode.ID` |
| `nodeKind` | runtime block kind; implemented as `TaskDecompositionNode.Kind` |
| `nodeState` | planned/dispatched/running/completed/failed/rejected/retrying/blocked_on_decision; implemented as `TaskDecompositionNode.State` |
| `coverageTargets` | declared targets from node input/output; implemented as `CoverageTargets` |
| `evidenceRefs` | direct step/tool/patch/validation/result refs; implemented as `EvidenceRefs` |
| `violationType` | contract violation type when rejected or retrying; implemented as `ViolationType` |
| `retryOf` | source step/node ref when retrying; implemented as `RetryOf` |
| `blockingTicketId` | governance blocker when blocked; implemented as `BlockingTicketID` |

## Batch 2 Closure Gates

### Gate B2-G1: First-Class Planned Task Projection

- Chain node: accepted LLM `executionGraph` output to planned task projection.
- Deviation type: `planned_task_projection_gap`.
- Evidence refs: `ev:code/projection/task-derived/001`, `ev:code/projection/task-plan/001`.
- Owner: projection / read model, with possible orchestrator event support.
- Expected behavior: planned tasks are visible independently from raw step timeline.
- Actual behavior: planned tasks are derived from step outputs and mixed activity.
- Proposed change: introduce a business-neutral task-plan lifecycle contract owned by orchestrator/runtime metadata and consumed by workorchd projection. When an `executionGraph` is accepted, rejected, retried, or blocked, emit or persist scoped plan revision metadata with `workspaceId`, `runId`, `sourceStepId`, `planId`, `revision`, node refs, node state, coverage targets, and evidence refs. Keep the existing derived projection as a compatibility-free implementation detail only if it is fed by the same lifecycle metadata; TUI/Web must consume the projected `TaskPlans` surface, not infer tasks from raw progress.
- Why this is not business-specific: applies to any execution graph regardless of template or tool kind.
- Required tests: accepted `executionGraph` appears as a task plan before or at first node dispatch; raw progress items remain separate; a client-facing ViewModel can render planned tasks without reading step output details.

### Gate B2-G2: Rejected And Retry Task State

- Chain node: contract violation / retry to task plan state.
- Deviation type: `retry_task_projection_gap`.
- Evidence refs: `ev:ws4e/run4/ctr-validation-003/output/001`, `ev:ws7c/run4/ctr-statechange-005/output/001`.
- Owner: projection and orchestrator retry metadata.
- Expected behavior: rejected graphs and retry exhaustion are represented as task states with violation metadata.
- Actual behavior: clients mainly see failed retry steps and decision blockers.
- Proposed change: carry contract-violation and retry lineage into the same task-plan lifecycle metadata. A rejected graph should project `sourceOutputState=rejected` and node state `rejected` with `violationType`, `failureClass`, rejected evidence refs, and no accepted result/artifact side effect. A retry should project `nodeState=retrying` or `retry_exhausted`, `retryOf`, target scope, attempt number, and blocking ticket id when escalation opens a decision. If a retry returns no new graph, update the source plan/node through lineage rather than creating an unrelated raw failed step display.
- Why this is not business-specific: contract violation and retry metadata are core runtime concepts.
- Required tests: rejected `executionGraph` projects as rejected task; retry-exhausted without a new graph updates the source plan to blocked/retry_exhausted; rejected diagnostic output does not appear as accepted evidence/result/artifact or long-lived memory.

## Closure Records

### 2026-06-21 B2-G1 / B2-G2 Planned Task Projection ViewModel

- Closed gates: `B2-G1`, `B2-G2`.
- Closed deviations: `planned_task_projection_gap`, `retry_task_projection_gap`, `rejected_plan_projection_gap`, `retry_lineage_projection_gap`.
- Implementation refs:
  - `internal/modules/application/workorchd/biz/appservice/readmodelcore/task_types.go`
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_decomposition_node.go`
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_decomposition_node_accumulator.go`
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_decomposition_projection.go`
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_lifecycle_metadata.go`
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_node_clone.go`
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_plan_item_projection.go`
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_plan_projection.go`
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_retry_plan_projection.go`
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/run_snapshot_test.go`
- Actual change:
  - Extended `TaskPlan` and `TaskDecompositionNode` with first-class lifecycle metadata: `sourceOutputState`, `nodes`, `retryOf`, `retryAttempt`, `violationType`, `violatedRule`, `failureClass`, `failureSignature`, `targetScope`, `blockingTicketId`, and `coverageTargets`.
  - Kept `TaskProgress.Items` as raw activity/timeline, while `TaskPlans` now carries the planned todo ViewModel surface that TUI/Web can render without parsing raw step outputs.
  - Rejected execution graphs are projected as rejected plans/nodes with contract violation metadata and diagnostic evidence refs; no accepted result/artifact side effect is created by projection.
  - Contract violation retries that return no new `executionGraph` now create a `contract_violation_retry` plan revision linked to the original anchor through structured retry lineage.
  - Retry exhaustion with an open decision projects as `blocked_on_decision` with `blockingTicketId`.
- Boundary result:
  - The change is entirely in workorchd readmodel/ViewModel projection and DTOs.
  - Projection consumes generic runtime metadata (`executionGraph`, step state, `contractViolation`, `contractViolationRecovery`, decision scope, coverage target fields).
  - No branch depends on workspace template ID, Soul ID, live workspace ID, provider, programming language, LSP, concrete filenames, or business-specific text.
  - Runtime execution semantics and owner state were not changed in this pass; future persistence/event ownership can be introduced only if Batch 1/EventHub gates require it.
- Tests run:
  - `gofmt -w internal/modules/application/workorchd/biz/appservice/readmodelcore/task_types.go internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_decomposition_projection.go internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_decomposition_node_accumulator.go internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_decomposition_node.go internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_plan_item_projection.go internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_plan_projection.go internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_lifecycle_metadata.go internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_node_clone.go internal/modules/application/workorchd/biz/appservice/readmodelprojection/task_retry_plan_projection.go internal/modules/application/workorchd/biz/appservice/readmodelprojection/run_snapshot_test.go`
  - `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'TestRunSnapshotProjectsRejectedExecutionGraphAsTaskPlan|TestRunSnapshotProjectsContractRetryWithoutNewGraphAsPlanRevision' -count=1`
  - `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1`
  - `go test ./internal/modules/application/workorchd/biz/appservice/... -count=1`
  - `go test ./internal/modules/application/workorchd/service/server -count=1`
  - `go test ./... -count=1 -p 1`
  - `go test ./... -count=1`
  - Evidence reference integrity check over `docs/70-roadmap/active/runtime-contract-retry-*.md`: `evidence refs consistent`.
- Remaining risk:
  - This pass makes the ViewModel first-class from current snapshot data; it does not yet add a separate persisted task-plan owner/event stream.
  - If later runtime verification proves projection cannot recover a lifecycle transition from snapshots, the follow-up should be an EventHub lifecycle event under Batch 1/B2, not a TUI/Web inference patch.
- Next gate:
  - Continue with Batch 7 deployed e2e, live workspace, and resource-envelope verification for the combined Batch 2-6 changes.

### 2026-06-21 B2-G1 Workspace Snapshot / Remote / TUI Consumption Follow-Up

- Closed gate: transport and client consumption portion of `B2-G1`.
- Closed deviation: `planned_task_projection_gap`.
- Gap found during follow-up:
  - `RunSnapshot` carried `taskPlans`, but workspace-level snapshots and remote/TUI DTOs did not expose the same field.
  - That meant a remote TUI/Web consumer could still receive `TaskDecompositions` or raw activity while missing the primary planned-task ViewModel surface described by this audit.
- Implementation refs:
  - `internal/modules/application/workorchd/biz/appservice/readmodelcore/workspace_snapshot_types.go`
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_snapshot_run_merge.go`
  - `internal/pkg/workorchclient/types.go`
  - `internal/pkg/workorchport/observability.go`
  - `internal/pkg/workorchport/workspace.go`
  - `internal/services/workorch/biz/backends/eventhub.go`
  - `internal/services/workorch/service/tui/model.go`
  - `internal/services/workorch/service/tui/workspace_state.go`
  - `internal/services/workorch/service/tui/view.go`
- Actual change:
  - Added `taskPlans` to workspace snapshot read model, client DTO, port DTO, and embedded backend resource map.
  - Merged active run `TaskPlans` into workspace snapshot projection together with `TaskDecompositions`.
  - TUI display cache now stores `TaskPlans`, and the main view renders `Task Plans` before falling back to `Task Decomposition`.
  - Web console types now include `TaskPlan` and lifecycle metadata, and the workspace detail view renders `taskPlans` as the primary planned-task card before falling back to `taskDecompositions`.
  - Existing raw `TaskProgress.Items` behavior is unchanged and remains an activity/timeline surface.
- Tests run:
  - `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection ./internal/services/workorch/biz/backends ./internal/services/workorch/service/tui ./internal/pkg/workorchclient ./internal/pkg/workorchport -count=1`
  - `npm run build` in `web/workorch-console`
- Boundary result:
  - The change is DTO/ViewModel plumbing only.
  - No logic branches on workspace IDs, templates, provider names, programming languages, filenames, or business text.

## Batch 2 Verdict

Before this closure, the implementation already had the pieces needed to reconstruct a partial todo view:

- LLM outputs can contain `executionGraph`
- dispatched steps carry `graphRootStepId` / `graphNodeId`
- projection has `TaskDecomposition` and `TaskPlan` DTOs

This closure makes the MVVP-facing planned-task surface explicit:

- `TaskPlans` and `TaskDecompositions` carry planned/rejected/retrying/blocked lifecycle metadata.
- `TaskProgress.Items` remains a raw activity timeline instead of the primary todo surface.
- Contract rejection and retry exhaustion are first-class projected plan states with neutral violation metadata.

Runtime core still only emits or exposes neutral lifecycle metadata; workorchd owns the ViewModel; software-development-specific task meaning remains in workspaceTemplate/Soul and coverage targets. A separate persisted task-plan owner/event stream remains a future option only if Batch 1/B2 runtime verification proves snapshot-derived projection cannot recover required lifecycle transitions.
