# workorch Runtime Expansion Plan

## Goal

This document defines the next active delivery scope after the closed service MVP
and completed next MVP acceptance items. The goal is to evolve `workorchd` from a
minimal in-process orchestration service into a runtime-capable execution service
with formal tool adapters, controlled shell execution, generalized patch handling,
block graph execution, and stronger observability.

This is an active delivery scope. It replaces the previous "out of current scope"
status for the items listed here.

## Scope

### R1. External Runtime And Tool Adapter Contract

Status: completed

Target:

- Define a formal runtime/tool adapter contract outside the legacy minimal
  adapter.
- Support adapter selection by step kind or block kind.
- Keep runtime output normalized into the existing evidence model:
  `ToolCall`, `Patch`, `ValidationResult`, and `RuntimeCheckpoint`.
- Keep adapter failures mapped to Run/Step failure state and validation evidence.

Acceptance:

- Existing legacy runtime remains supported through the new adapter boundary.
- At least one non-legacy adapter can be registered and executed in tests.
- Adapter output can produce tool calls, validation results, checkpoints, and
  patch requests without special-case orchestration code per adapter.

### R2. Controlled Shell Sandbox

Status: completed

Target:

- Add a shell execution adapter with an explicit working directory rooted at the
  Workspace working directory.
- Enforce command timeout and environment allowlist.
- Capture stdout, stderr, exit code, and duration into `ToolCall` evidence.
- Reject unsafe configuration by default.

Acceptance:

- A shell-backed Run can execute a simple command inside the Workspace working
  directory.
- Attempts to escape the Workspace directory are rejected.
- Timeout and non-zero exit are visible in Run snapshot evidence.

### R3. Generalized Diff Patch Apply Engine

Status: completed

Target:

- Extend the current runtime-requested workspace file write patch path into a
  generalized diff patch apply engine.
- Support unified diff application under the Workspace working directory.
- Record created/applied/failed patch state with target refs and summary.
- Preserve path safety checks.

Acceptance:

- A valid unified diff can be applied and recorded as `Patch` evidence.
- Invalid diff or unsafe target path fails the patch and Run deterministically.
- File backend restart preserves applied/failed patch evidence.

### R4. Block Execution Graph

Status: completed

Target:

- Move beyond linear `nextStep` dispatch into a formal block execution graph.
- Support block nodes, dependencies, branch conditions, and terminal states.
- Keep Run state as the external execution state while graph state remains an
  execution detail.
- Continue using Decision Ticket and Approval gates for human-in-the-loop blocks.

Acceptance:

- A graph-backed Run can execute multiple dependent blocks.
- Branch selection is visible in snapshot/evidence.
- Blocked decision/approval nodes resume from the correct graph position.

### R5. Observability Pipeline

Status: completed

Target:

- Promote current snapshot/event collection into a stronger runtime observability
  pipeline.
- Track run/step/tool/patch/checkpoint metrics and event streams.
- Keep observability derived from formal events and evidence, not separate
  mutable business state.

Acceptance:

- Runtime execution emits enough events/evidence to reconstruct current Run
  progress.
- HTTP health/ready remains stable while observability is enabled.
- Verification covers both memory and file repository backends.

## Delivery Order

1. `R1 External Runtime And Tool Adapter Contract`
2. `R2 Controlled Shell Sandbox`
3. `R3 Generalized Diff Patch Apply Engine`
4. `R4 Block Execution Graph`
5. `R5 Observability Pipeline`

The order is intentional. The adapter contract creates the stable extension
point required by the shell sandbox and future external tools. The diff patch
engine then consumes normalized tool output. The block graph should build on
stable adapter execution and patch semantics. Observability should be derived
from the resulting formal execution events and evidence.

## Current Active Item

- active: none. Runtime expansion delivery scope completed.

## Progress

- completed: `R1 External Runtime And Tool Adapter Contract`
  Runtime execution now uses a formal `StepExecutionRequest` /
  `StepExecutionResult` contract and a step-kind based adapter registry. The
  legacy minimal runtime remains available through the same contract, and tests
  cover dispatch to a registered non-legacy adapter.
- completed: `R2 Controlled Shell Sandbox`
  The `blocks/shell` runtime unit now registers a `shell/command` adapter through
  the runtime adapter registry and can execute a non-interactive command in the
  Workspace working directory. It executes commands through `exec.CommandContext`
  with explicit command/args, an environment allowlist, timeout control, and
  Workspace-rooted working directory. Successful execution, non-zero exit, and
  timeout all preserve stdout, stderr, exit code, duration, and status in Step
  output and `ToolCall` evidence. This is a controlled local command executor for
  MVP validation, not an OS-level isolation boundary.
- completed: `R3 Generalized Diff Patch Apply Engine`
  Runtime patch requests now support both `file_write` and `unified_diff`
  `patchType` values. Unified diff application is constrained to an explicit
  Workspace-relative `targetRef`, reuses the same path safety checks as file
  writes, records created/applied/failed `Patch` evidence, and fails the Run
  deterministically on invalid diff content or mismatched targets. Tests cover
  successful diff application, invalid diff failure without file mutation, and
  file backend reload preserving applied patch evidence.
- completed: `R4 Block Execution Graph`
  Runtime execution now supports a formal `BlockExecutionGraph` with ordered
  block nodes, dependencies, branch selection, and graph metadata carried through
  Step input/output. The orchestrator keeps Run state as the external execution
  state, derives graph progress from existing Step records, and records graph
  node/branch metadata into runtime checkpoints. Tests cover graph-backed
  dependent block execution, branch selection without executing the inactive
  branch, and correct resume position after both Decision Ticket and Approval
  gates.
- completed: `R5 Observability Pipeline`
  Runtime observability is now derived from existing events and evidence rather
  than separate mutable state. `RunObservability` exposes run progress, evidence
  counters, latest checkpoint, and a timeline composed from checkpoints and
  observed Workorch events. HTTP exposes
  `/api/v1/workorch/runs/:runId/observability`, `/metrics` tracks
  `run_observability` requests, and the embedded console shows selected run
  observability progress, evidence counters, latest checkpoint, and timeline.
  Verification covers memory and file repository backends.
