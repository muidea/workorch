# workorch Result And Event Stream Development Plan

## Goal

This document promotes two next-stage requirements into the active development
backlog:

- formal business Result / Artifact model
- persistent Workorch event stream

The target remains a locally deployable `workorchd` service. This round should
improve user-facing outcome confirmation and audit-grade runtime replay without
expanding into distributed worker fleets, multi-tenant governance, or an
external runtime marketplace.

## Scope

### RE1. Formal Business Result And Artifact Model

Status: in_progress

Target:

- Add a formal business result model so the final Workspace outcome is not
  exposed only through `dodAssessment.summary`.
- Represent the accepted outcome of a Run as a first-class Result linked to
  Workspace, Run, Goal, and DoD.
- Represent deliverable files, generated outputs, external references, and
  evidence-backed attachments as Artifacts linked to Result, Run, Step,
  ToolCall, Patch, or ValidationResult.
- Keep ToolCall, Patch, ValidationResult, RuntimeCheckpoint, Memory, and events
  as evidence or context. They must not be relabeled as final business outcome
  by Web or runtime code.
- Automatically create the initial Result from a completed LLM-driven Run with
  a satisfied `dodAssessment`.

Acceptance:

- Memory and file repositories persist Result and Artifact records.
- `GET /api/v1/workorch/runs/{runId}/snapshot` exposes the formal Result when a
  Run completes with a satisfied DoD assessment.
- Workspace detail and Workspace snapshot expose current Results without
  requiring users to inspect raw Step output.
- Web Console shows business-facing final outcome from Result / Artifact, while
  Step output remains runtime evidence.
- Existing weather-query and server-config e2e checks can assert final outcome
  through Result instead of only `dodAssessment.summary`.

### RE2. Persistent Workorch Event Stream

Status: planned

Target:

- Persist formal Workorch business events emitted through the existing
  `EventHub` boundary.
- Keep the existing `EventHub` as runtime dispatch infrastructure; event
  persistence must be a subscriber/collector concern, not a state transition
  owner.
- Add query APIs for audit and replay by workspace, run, topic, subject, and
  cursor/time range.
- Allow Run observability and Workspace detail to read from persisted event
  stream when available, while preserving current evidence-driven state truth.
- File backend restart must preserve event stream records.

Acceptance:

- Memory and file repositories persist normalized Workorch event records.
- Events emitted for Workspace, Goal, DoD, Run, Step, Decision, Approval,
  Result, and Artifact can be queried after the request that emitted them.
- File-backed service restart preserves event stream query results.
- Run observability can include persisted event timeline items without
  synthesizing missing events from repository state.
- The event persistence path does not introduce business-specific fallback
  logic into `kernel/orchestrator`, `kernel/run`, `blocks/llm`, `blocks/mcp`,
  or Web outcome rendering.

## Module Boundary

Use the current `go-multi-module-dev` layout and existing workorch terminology.

- Result / Artifact business model should live under `internal/modules/kernel`
  because it is a formal business outcome model tied to Workspace and Run
  closure.
- Persistent event stream should live under `internal/modules/kernel` or the
  application observed boundary depending on implementation detail, but the
  persisted event repository and query contract must remain separate from
  `internal/pkg/events` dispatch primitives.
- `internal/pkg/events` continues to own envelope construction, topic constants,
  payload helpers, and adapter-facing event types.
- `internal/modules/application/workorchd/biz/appservice` owns HTTP use-case
  composition and read-model assembly.
- `internal/modules/application/workorchd/service/server` owns route handlers.
- Web Console consumes only HTTP APIs; it must not infer final outcome from
  weather-specific fields, tool names, provider payloads, or concrete Goal text.

## Delivery Order

1. `RE1.1 Result / Artifact model and repositories`
2. `RE1.2 Result creation from completed Run DoD assessment`
3. `RE1.3 Result / Artifact read APIs and snapshots`
4. `RE1.4 Web final-outcome view backed by Result / Artifact`
5. `RE2.1 Persistent event record model and repositories`
6. `RE2.2 EventHub persistence subscriber and lifecycle wiring`
7. `RE2.3 Event stream query APIs and observability integration`
8. `RE2.4 File-backed restart and e2e replay verification`

The order is intentional. Result / Artifact should first establish the formal
business outcome surface. Event persistence then records the new outcome events
alongside existing Workspace, Run, Step, Decision, and Approval events.

## Non-Goals

- Distributed scheduling and external worker fleets.
- Multi-tenant authorization, user identity, and quota.
- External runtime/tool adapter marketplace.
- Replacing ToolCall / Patch / ValidationResult / RuntimeCheckpoint evidence.
- Replacing Memory with a knowledge asset platform in this round.
- Inferring business success from concrete domain terms such as weather, city,
  host configuration, tool payload fields, or provider-specific response shape.

## Verification Baseline

Each delivered slice must keep the existing baseline green unless the slice
explicitly updates the verification contract:

```bash
go test ./... -count 1
npm --prefix web/workorch-console run build
make build
make verify-boundary-contract
make verify-mvp
make verify-real-mvp
```

New acceptance scripts should continue to verify the currently deployed service
through `WORKORCH_BASE_URL`; they must not start or stop `workorchd` internally.

CI-safe checks should use mock providers. Local MVP validation should keep using
the configured real LLM provider where the test depends on actual LLM behavior.

## Progress

- completed: `RE1.1 Result / Artifact model and repositories`
  Result and Artifact now have formal kernel models, common ID/state types,
  repository interfaces, memory/file repository implementations, shared
  repository contract coverage, file-backed reload verification, and
  runtime/bootstrap repository export wiring.
- completed: `RE1.2 Result creation from completed Run DoD assessment`
  The orchestrator now creates an accepted Result only after the formal
  LLM-driven DoD assessment gate passes. The Result is linked to Workspace,
  Run, Goal, DoD, source Step, evidence refs, and a text Artifact that exposes
  the accepted summary as a formal deliverable without inspecting
  business-specific goal terms, provider payloads, or tool names.
- completed: `RE1.3 Result / Artifact read APIs and snapshots`
  Run and Workspace snapshots now expose Result / Artifact read models through
  the existing workorchd read-model boundary, including file-backed reload
  coverage for formal outcomes and linked artifacts.
- completed: `RE1.4 Web final-outcome view backed by Result / Artifact`
  The Web Console target-outcome area now prioritizes formal Result summaries
  and Artifact deliverables. DoD assessment, Patch, ToolCall, ValidationResult,
  and RuntimeCheckpoint remain visible as verification and runtime evidence.
- completed: `RE2.1 Persistent event record model and repositories`
  EventRecord is now a formal kernel event stream model with memory/file
  repositories and shared repository contract coverage.
- completed: `RE2.2 EventHub persistence subscriber and lifecycle wiring`
  The `kernel/eventstream` module subscribes to formal Workorch events and
  appends normalized records through the EventRecord repository. The module is
  wired into both bootstrap runtime and plugin-based `workorchd` loading.
- completed: `RE2.3 Event stream query APIs and observability integration`
  Run and Workspace snapshots expose persisted event records, Run observability
  uses persisted event timeline items when available, and
  `GET /api/v1/workorch/events` supports query by workspace, run, topic,
  subject, cursor, and limit.
- planned: `RE2.4 File-backed restart and e2e replay verification`
