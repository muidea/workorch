# WorkOrch Runtime Storage JSON Control Design

## Purpose

This document defines how WorkOrch should prevent single JSON files from growing into large runtime bottlenecks.

The current file repository stores many run-level resources as full JSON maps, for example:

```text
step.json
tool_call.json
event_record.json
runtime_checkpoint.json
```

For long autonomous workflows, a small append/update can force a full load, clone, marshal, and rewrite of tens of MB. This causes high CPU, high memory usage, slow read-model APIs, and possible partial-read parse errors during concurrent reads.

Current phase does not require compatibility. The implementation may replace the storage layout directly.

## Goals

- Eliminate large run-level JSON map rewrites.
- Keep per-update IO bounded by the changed item or append segment.
- Separate metadata/index data from large raw payloads.
- Make read models projection-based and paginated.
- Preserve full evidence through references without injecting raw payloads by default.
- Keep storage rules framework-neutral.

## Non-Goals

- Do not preserve the old map JSON files for new data.
- Do not introduce SQL or external storage as a prerequisite.
- Do not expose raw local file paths in API responses.
- Do not store raw stream deltas inside primary event records.

## Root Layout

Runtime state lives under the workspace internal directory:

```text
.workorch-data/internal/workspaces/<workspace-id>/
  workspace.json
  goals/
  dods/
  memory/
  context/
  runs/
  indexes/
```

Each run has an itemized layout:

```text
runs/<run-id>/
  run.json
  index.json

  steps/
    <step-id-path>/
      metadata.json
      input.json
      output.json

  tool-calls/
    <tool-call-id-path>/
      metadata.json
      arguments.json
      output.json

  patches/
    <patch-id-path>.json

  validations/
    <validation-id-path>.json

  checkpoints/
    <checkpoint-id-path>.json

  decisions/
    tickets/
      <ticket-id-path>.json
    resolutions/
      <resolution-id-path>.json
    approvals/
      <approval-id-path>.json

  results/
    <result-id-path>.json

  artifacts/
    <artifact-id-path>.json
    blobs/

  events/
    events-000001.jsonl
    events-000002.jsonl
    payloads/
      <event-id-path>.json

  llm-calls/
    <step-id-path>/
      request.json
      response.sse
      response.json
      usage.json
```

`<id-path>` must be a readable, filesystem-safe projection of the logical ID that preserves structural hierarchy where useful.

Rules:

- logical IDs must remain the source of truth;
- `/` in logical IDs should normally become directory hierarchy in file backend storage;
- each path segment must be sanitized to a safe readable segment;
- default storage must not encode the whole ID as opaque base64 or hash file names;
- pure base64/hash names are allowed only as an extreme fallback for segment overflow or unavoidable collision, and even then should keep a readable prefix.

## Storage Rules

### Item-Level Files

Mutable item records such as steps and tool calls must be stored as item-level files.

For a step:

```text
steps/<step-id-path>/metadata.json
steps/<step-id-path>/input.json
steps/<step-id-path>/output.json
```

`metadata.json` contains list/detail fields:

```json
{
  "id": "...",
  "runId": "...",
  "kind": "...",
  "state": "...",
  "createdAt": "...",
  "updatedAt": "...",
  "inputRef": "...",
  "outputRef": "..."
}
```

Large input/output payloads are stored separately. Updating output should not rewrite all steps in a run.

### Append-Only Events

Events must be append-only JSONL:

```text
events/events-000001.jsonl
events/events-000002.jsonl
```

Each line contains event metadata plus optional `payloadRef`:

```json
{"id":"...","topic":"...","workspaceId":"...","runId":"...","stepId":"...","occurredAt":"...","payloadRef":"events/payloads/<event-id-path>.json"}
```

Small payloads may be embedded only under a strict size threshold. Large payloads must be written to `payloads/`.

### LLM Calls

Raw LLM call data must not be stored in step/tool-call/event primary payloads.

Store it under:

```text
llm-calls/<step-id-path>/request.json
llm-calls/<step-id-path>/response.sse
llm-calls/<step-id-path>/response.json
llm-calls/<step-id-path>/usage.json
```

Step/tool-call/event records should reference these files through logical refs.

### Runtime Indexes

Indexes should be small and projection-oriented:

```text
runs/<run-id>/index.json
indexes/workspace-run-index.json
indexes/open-decision-index.json
indexes/latest-run-index.json
```

Indexes may be rebuilt from item files and JSONL event segments. They should not be the only durable source of truth.

## Size Thresholds

Initial recommended thresholds:

| Resource | Threshold | Behavior |
| --- | ---: | --- |
| Embedded event payload | 8KB | larger payload becomes `payloadRef` |
| Step input/output | 64KB | store as separate payload file, summary in metadata |
| Tool call arguments/output | 64KB | store separately, summary in metadata |
| JSONL event segment | 8MB | rotate to next segment |
| Index file | 2MB target | split/rebuild projection if exceeded |
| API default event page | 100 items | explicit cursor required for more |

These thresholds are implementation defaults, not business rules.

## Atomic Write Rules

All overwrite writes must be atomic:

```text
write temp file
fsync temp file
rename temp file to target
fsync parent directory where practical
```

Append-only event writes must hold a per-run event append lock. If segment rotation is needed, rotation must be atomic.

Readers must never observe half-written JSON files.

## Read Model Projection

Read APIs must distinguish projections:

```text
summary
detail
raw
```

### Summary

Default for workspace and run list views.

Includes:

- IDs;
- states;
- kind;
- created/updated times;
- current step summary;
- result summary;
- latest error summary;
- counts.

Excludes:

- full event payloads;
- full step input/output;
- full tool arguments/output;
- stream deltas;
- raw LLM requests/responses.

### Detail

Default for a selected run detail page.

Includes paginated:

- step metadata;
- tool-call metadata;
- event metadata;
- validation/result/artifact metadata;
- selected summaries and refs.

Still excludes raw payloads unless explicitly requested.

### Raw

Explicit ref-based load only.

Examples:

```text
GET /api/v1/workorch/runtime/payloads?ref=workorch://workspaces/<workspace-id>/runs/<run-id>/llm-calls/<step-id-path>/response.json
GET /runs/<run-id>/events?limit=100&cursor=...
GET /runs/<run-id>/steps/<step-id>/raw-output
```

## Context Integration

LLM context builders must consume projection data, not raw repository files by default.

Repair input should include:

- current failure packet;
- current target snapshots or refs;
- latest bounded verification output;
- invalid strategy summaries;
- raw refs where needed.

It should not include:

- full event streams;
- full superseded run histories;
- raw LLM streams;
- large tool outputs without a specific reason.

## Repository Contract Changes

The file repository implementation should move from map-file storage to itemized storage.

Recommended repository behavior:

- `Create` writes a new item directory/file.
- `Get` reads only the requested item.
- `Update` overwrites only the requested item files.
- `ListByRun` scans item metadata files or uses a rebuildable index.
- `EventRecord.Append` appends one JSONL line and optional payload file.
- `EventRecord.List` requires limit/cursor defaults and scans only needed segments where possible.

The public repository interfaces can remain similar initially, but implementations must avoid full run-level JSON map rewrites.

## API Contract Changes

Workspace/run APIs must not default to all raw data.

Required changes:

- event list requires default `limit`;
- workspace snapshot does not include raw `eventRecords`;
- run snapshot defaults to summary/detail projection;
- raw payloads are loaded by ref;
- console uses pagination and lazy loading.

## Testing Requirements

Add tests for:

1. Item update does not rewrite unrelated step/tool-call files.
2. Event append increases only the active JSONL segment and optional payload file.
3. Segment rotation occurs at threshold.
4. Large payload becomes a ref.
5. Reader never observes partial JSON after overwrite.
6. Workspace summary API does not load raw event payloads.
7. Run detail API paginates events.
8. Repair context builder receives refs/summaries instead of full streams.
9. Synthetic large run remains within bounded CPU/memory during summary read.

Suggested synthetic large-run test:

```text
1000 events
200 steps
200 tool calls
20 large LLM responses
```

Expected behavior:

- summary read does not load raw payload files;
- event append is O(1) relative to number of previous events in current segment;
- single step update does not marshal all steps.

## Implementation Order

0. Separate deliverable and internal workspace roots.
   - Status: completed.
   - Runtime files now live under `.workorch-data/internal/workspaces/<workspace-id>/`.
   - User deliverables now live under `.workorch-workspaces/<workspace-id>/`.
1. Add storage layout resolver under application/runtime session layer.
   - Status: completed for current workspace/run storage roots.
   - Workspace creation resolves a separate deliverable root and WorkOrch internal root through the runtime session repository backend.
   - For the file backend, default deliverables live under `.workorch-workspaces/<workspace-id>` beside the configured data root, while runtime state lives under `.workorch-data/internal/workspaces/<workspace-id>`.
   - Explicit deliverable directories are respected as user project roots, while the internal runtime root remains managed under the repository data root.
   - Contract tests verify the resolver roots, internal subdirectories, and absence of the old `working/` level for new workspaces.
2. Implement itemized step/tool-call file repositories.
   - Status: completed for file backend.
   - `StepRepository` now stores each step under `steps/<step-id-path>/metadata.json`, `input.json`, and `output.json`.
   - `ToolCallRepository` now stores each tool call under `tool-calls/<tool-call-id-path>/metadata.json`, `arguments.json`, and `output.json`.
   - Item storage now keeps readable ID hierarchy by default; IDs containing `/` expand into nested directories under the collection root.
   - Create/update/delete now touch only the target item directory instead of rewriting a run-wide `step.json` or `tool_call.json`.
3. Implement JSONL event repository with pagination.
   - Status: completed for append-only storage, payload splitting, and bounded HTTP list queries.
   - `EventRecordRepository` now appends to `events/events-000001.jsonl` segments instead of rewriting `event_record.json`.
   - Large event payload maps are stored under `payloads/<event-id-path>.json` and rehydrated by repository reads.
   - Existing `List` keeps cursor/limit behavior at the repository contract level; `GET /api/v1/workorch/events` applies a default `limit=100`, accepts `cursorAfterId`, and clamps oversized limits to the server maximum.
4. Split LLM raw request/response/stream data into refs.
   - Status: completed for runtime LLM block outputs and console raw-ref access.
   - `StepExecutionRequest` now carries `WorkspaceInternalDirectory` so runtime blocks can write internal evidence without touching deliverables.
   - The LLM runtime block writes provider request, raw response, stream SSE, repair response when present, and usage summary under `runs/<run-id>/llm-calls/<step-id-path>/`.
   - Step output keeps `llmCallRefs`, `streamDiagnostics`, `contextBudget`, and `llmUsage`; raw `streamEvents` are no longer embedded in primary step output by the LLM block.
   - Runtime timeline read models can render bounded stream summaries from `streamDiagnostics`; legacy in-output `streamEvents` remain readable only as historical input.
   - `GET /api/v1/workorch/runtime/payloads?ref=...` exposes explicit raw-ref reads for whitelisted LLM call files under workspace internal storage.
   - The same runtime payload endpoint also supports non-LLM lazy details through logical refs for Step input/output, ToolCall arguments/output, RuntimeCheckpoint data, and Result DoD/detail payloads.
   - Embedded Web Console step, timeline, tool-call, checkpoint, and result detail views expose direct raw-ref actions without embedding raw payloads in list responses.
5. Update read model projections.
   - Status: completed for current high-risk run detail collections.
   - Run and workspace snapshots now load bounded event record projections with `limit=100`.
   - Snapshot event payloads keep only compact metadata fields needed by timeline/status/summary views and omit raw `input`, `output`, `context`, `content`, and generic nested payload data.
   - Run snapshot HTTP responses now embed only bounded Step, ToolCall, RuntimeCheckpoint, Result, Patch, ValidationResult, and Artifact projections for initial detail rendering; complete lists must be read through their paginated endpoints.
   - LLM usage summary is computed from separately filtered `run.llm_usage_recorded` events, so usage aggregation does not require exposing full event payloads in snapshots.
   - Step list detail reads are available through `/api/v1/workorch/runs/<run-id>/steps?limit=100&cursorAfterId=...` and return a bounded step output projection instead of raw input/output.
   - Tool-call list detail reads are available through `/api/v1/workorch/runs/<run-id>/tool-calls?limit=100&cursorAfterId=...` and return metadata, state, summary, and error fields without raw arguments/output payloads.
   - Runtime checkpoint list detail reads are available through `/api/v1/workorch/runs/<run-id>/runtime-checkpoints?limit=100&cursorAfterId=...` and return metadata without raw checkpoint `data`.
   - Result list detail reads are available through `/api/v1/workorch/runs/<run-id>/results?limit=100&cursorAfterId=...` and return metadata, summary, source refs, and evidence refs without raw DoD assessment/detail payloads.
   - Patch list detail reads are available through `/api/v1/workorch/runs/<run-id>/patches?limit=100&cursorAfterId=...` and return patch metadata, state, target refs, approvals, and line stats without raw diff/content payloads.
   - ValidationResult list detail reads are available through `/api/v1/workorch/runs/<run-id>/validation-results?limit=100&cursorAfterId=...` and return validation metadata, state, summaries, and evidence refs.
   - Artifact list detail reads are available through `/api/v1/workorch/runs/<run-id>/artifacts?limit=100&cursorAfterId=...` and return artifact metadata, URI, summary, and evidence refs without raw metadata maps or artifact content.
   - Decision list reads are available through `/api/v1/workorch/decisions?workspaceId=...&runId=...&limit=100&cursorAfterId=...` and return paginated audit projections without ticket `context` or resolution payloads.
   - Approval list reads are available through `/api/v1/workorch/approvals?workspaceId=...&runId=...&limit=100&cursorAfterId=...` and return paginated audit projections without approval `context`.
   - Decision/Approval full payloads remain available only through explicit detail reads: `/api/v1/workorch/decisions/<ticket-id>` and `/api/v1/workorch/approvals/<approval-id>`.
   - Workspace and Run snapshot detail projections now also bound and project Decision/Approval audit collections.
   - Coverage includes a synthetic run snapshot test with more events than the snapshot projection limit and a late LLM usage event.
   - Coverage also includes a synthetic large run snapshot projection test that exceeds the default Step, ToolCall, RuntimeCheckpoint, Result, Patch, ValidationResult, Artifact, Decision, and Approval detail limits and verifies raw payload fields are omitted from the projected detail response.
   - Remaining work: none for current bounded detail projection coverage; measured CPU/memory pressure can drive future benchmark thresholds.
6. Update console to use paginated detail/raw endpoints.
   - Status: completed for current high-risk run detail collections.
   - LLM raw evidence refs are opened explicitly through the runtime payload endpoint.
   - Web Console run detail now has an independent Event tab backed by `/api/v1/workorch/events?runId=...&limit=100&cursorAfterId=...`, with refresh and load-more actions.
   - Web Console run detail Step tab now uses `/api/v1/workorch/runs/<run-id>/steps?limit=100&cursorAfterId=...`, with refresh and load-more actions.
   - Web Console run detail ToolCall tab now uses `/api/v1/workorch/runs/<run-id>/tool-calls?limit=100&cursorAfterId=...`, with refresh and load-more actions.
   - Web Console run detail RuntimeCheckpoint tab now uses `/api/v1/workorch/runs/<run-id>/runtime-checkpoints?limit=100&cursorAfterId=...`, with refresh and load-more actions.
   - Web Console run detail Result tab now uses `/api/v1/workorch/runs/<run-id>/results?limit=100&cursorAfterId=...`, with refresh and load-more actions.
   - Web Console run detail Patch tab now uses `/api/v1/workorch/runs/<run-id>/patches?limit=100&cursorAfterId=...`, with refresh and load-more actions.
   - Web Console run detail ValidationResult tab now uses `/api/v1/workorch/runs/<run-id>/validation-results?limit=100&cursorAfterId=...`, with refresh and load-more actions.
   - Web Console run detail Artifact tab now uses `/api/v1/workorch/runs/<run-id>/artifacts?limit=100&cursorAfterId=...`, with refresh and load-more actions.
   - Web Console governance queue now loads Decision and Approval audit projections through paginated endpoints with refresh/load-more actions.
   - Decision resolution and Approval confirmation modals lazy-load the selected full detail record before rendering context or resolution payloads.
   - Remaining work: none for Step/ToolCall/RuntimeCheckpoint/Result/Patch/ValidationResult/Artifact/Decision/Approval pagination.
7. Add synthetic large-run performance tests.
   - Status: completed for current bounded projection regression coverage.
   - Step pagination projection is covered by a page/cursor/projection test.
   - Tool-call pagination projection is covered by a page/cursor/projection test.
   - RuntimeCheckpoint, Result, Patch, ValidationResult, and Artifact pagination projections are covered by page/cursor/projection tests.
   - Decision and Approval audit pagination projections are covered by page/cursor/projection tests that also verify explicit detail reads retain full payloads.
   - Synthetic large run snapshot projection is covered across Step, ToolCall, RuntimeCheckpoint, Result, Patch, ValidationResult, Artifact, Decision, and Approval collections, including raw payload omission checks.
   - Remaining work: none for regression coverage required by this design. Dedicated benchmark thresholds should be added only after collecting stable runtime baselines.
8. Remove old map JSON repository paths from new workspace creation.
   - Status: completed for current run-scoped runtime and governance collections.
   - Completed for `event_record.json`, `step.json`, `tool_call.json`, `patch.json`, `validation_result.json`, `runtime_checkpoint.json`, `result.json`, `artifact.json`, `ticket.json`, `resolution.json`, and `approval.json`.
   - `PatchRepository`, `ValidationResultRepository`, `RuntimeCheckpointRepository`, `ResultRepository`, and `ArtifactRepository` now store new records as item-level JSON files under `patches/`, `validations/`, `checkpoints/`, `results/`, and `artifacts/`.
   - `DecisionTicketRepository`, `DecisionResolutionRepository`, and `ApprovalRepository` now store new records as item-level JSON files under `decisions/tickets/`, `decisions/resolutions/`, and `approvals/`.
   - Create/update/delete now touch only the target item file for those run evidence and governance collections instead of rewriting a run-wide map.

## Acceptance Criteria

The storage redesign is complete when:

1. New runs no longer create monolithic `step.json`, `tool_call.json`, or `event_record.json` maps.
2. Event append does not rewrite historical events.
3. Step/tool-call update rewrites only that item.
4. Large payloads are referenced, not embedded in list views.
5. Workspace/run summary APIs stay responsive for large runs.
6. Raw evidence remains available by ref.
7. No reader can observe partially-written JSON.
8. The deliverable directory remains free of WorkOrch internal storage.

## Summary

The target model is:

```text
item-level metadata
+ append-only events
+ raw payload references
+ projection-based read models
+ atomic writes
```

This directly addresses the CPU, memory, and partial-read risks observed in large WorkOrch runs while preserving full evidence for audit and repair. It also keeps runtime storage readable enough for human inspection, recovery debugging, and run/step correlation, instead of hiding the primary hierarchy behind opaque base64/hash file names.
