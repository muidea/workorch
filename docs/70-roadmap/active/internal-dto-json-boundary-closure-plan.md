# Internal DTO / JSON Boundary Closure Plan

## Contract Audit

- Scope: WorkOrch internal interaction paths between TUI/local backend, embedded EventHub backend, workorchd appservice CLI dispatcher, readmodel projection, and `internal/pkg/workorchport` DTOs.
- Observed state:
  - Live pprof on `2026-06-29` showed CPU samples concentrated in `CLICommandDispatcher.handleWorkspaceCommand -> clicore.ResourceMap -> typedWorkspaceSnapshotInfo`, with most cost under `encoding/json.Marshal` / `encoding/json.Unmarshal`.
  - `internal/modules/application/workorchd/biz/appservice/clicore/resources.go` converts `readmodelcore.WorkspaceSnapshot` to `workorchport.WorkspaceSnapshotInfo` by JSON round-trip.
  - `internal/services/workorch/biz/backends/eventhub.go` decodes EventHub command results by JSON round-trip when `TypedResource` is absent or not matched.
  - `internal/services/workorch/biz/backends/snapshot.go` decodes snapshot payloads by JSON round-trip.
  - `clicore.DecodeOrZero` and `eventhub.decodeOrZero` use JSON to decode command payload maps into DTOs.
- Expected state:
  - `encoding/json.Marshal` / `encoding/json.Unmarshal` is allowed only at real serialization boundaries: REST/SSE HTTP server, REST client, explicitly JSON-backed config/files, and external command output that is actually JSON.
  - TUI local/embedded, EventHub command/result, appservice facade, projection, and `workorchport` DTO exchange must use typed DTOs and explicit field mappers.
  - workorchd remains the owner of workspace state and ViewModel. TUI/Web consume projection/DTOs and do not infer owner state.
- Deviation:
  - Internal appservice/backend/TUI paths currently use JSON as a generic type conversion mechanism.
  - Full snapshot conversion creates avoidable CPU and allocation load, and it obscures the DTO contract between readmodel and port layers.
  - Generic JSON fallbacks make unsupported result shapes silently accepted instead of failing at the typed boundary.
- Owner: core interaction boundary across `workorchd appservice`, `workorchport`, and `internal/services/workorch` local/embedded backend. This is framework-neutral and not workspaceTemplate/Soul policy.
- Event path:
  - TUI embedded command -> backend EventHub command -> workorchd CLI dispatcher -> readmodel/view owner -> CLI result -> backend DTO decode -> TUI ViewModel.
  - Current path sometimes uses `map[string]any` + JSON round-trip in the middle. Target path uses typed command/result payloads and explicit readmodel-to-port DTO mappers.
- Contract gap:
  - Missing explicit `readmodelcore` -> `workorchport` mapping package.
  - CLI/EventHub command result still carries both untyped `Resource` maps and optional `TypedResource`, allowing fallback JSON decode.
  - Payload map decoding uses generic JSON rather than explicit typed command builders.
  - There is no boundary test preventing new internal JSON round-trip conversions.
- Proposed change:
  - Introduce explicit DTO mappers for all appservice -> port DTOs needed by TUI/local backend.
  - Make embedded EventHub backend consume typed result payloads first and remove JSON fallback for internal DTO decoding.
  - Replace generic command payload JSON decoding with explicit field readers or typed command payload structs.
  - Keep JSON at REST/SSE and file/external JSON boundaries only.
- Boundary check:
  - The change is business-neutral: it does not add template, language, provider, file-name, or workspace-specific rules.
  - The change strengthens MVVP boundaries: workorchd remains ViewModel/state owner; TUI/local backend only receives DTOs.
- Verification:
  - Unit tests for readmodel-to-port mappers.
  - Embedded backend tests proving typed resources are decoded without JSON fallback.
  - Static boundary scan for forbidden `encoding/json` use in internal DTO paths.
  - pprof/live verification showing idle TUI/workorchd no longer spends material CPU in JSON round-trip snapshot conversion.

## Boundary Rules

### JSON Allowed

`encoding/json` remains allowed in these cases:

- REST API request/response and SSE envelope serialization under `internal/modules/application/workorchd/service/server`.
- Remote REST client implementation under `internal/pkg/workorchclient`.
- CLI stdout formatting when the user explicitly requests JSON output.
- Explicit JSON file/config persistence or managed config generation.
- Parsing external command output that is contractually JSON, for example a third-party CLI status response.
- Tests that verify HTTP JSON payloads or JSON file compatibility.

### JSON Forbidden

`encoding/json.Marshal` / `encoding/json.Unmarshal` must not be used for internal conversion in these paths:

- `readmodelcore` -> `workorchport` DTO projection.
- `workorchport` DTO -> `map[string]any` CLI/EventHub resource conversion.
- EventHub command result decode inside `internal/services/workorch/biz/backends/eventhub.go`.
- TUI local/embedded workspace snapshot decode.
- appservice `clicore` generic command payload decode.
- readmodel projection helpers that only clone or narrow already-typed internal structures.

### Replacement Standard

Every internal conversion must be one of:

- Direct typed DTO handoff.
- Explicit field-by-field mapper.
- Explicit typed command builder with small helper functions for primitive `map[string]any` values.
- Structured clone helper for `map[string]any` payloads when the payload is intentionally dynamic and stays dynamic.

Generic JSON round-trip fallback is not allowed.

## Current Findings

| File | Current JSON Use | Classification | Required Action |
| --- | --- | --- | --- |
| `internal/modules/application/workorchd/biz/appservice/clicore/resources.go` | `typedWorkspaceSnapshotInfo` JSON converts `readmodelcore.WorkspaceSnapshot` to `workorchport.WorkspaceSnapshotInfo`; `ResourceMapAlias` JSON converts unknown resources to map | Forbidden internal DTO conversion | Replace with explicit mapper and remove fallback alias |
| `internal/modules/application/workorchd/biz/appservice/clicore/payload_values.go` | `DecodeOrZero[T]` JSON converts payload map to command DTO | Forbidden internal command decode | Remove generic decoder; use explicit command builders |
| `internal/services/workorch/biz/backends/eventhub.go` | `resourceMap`, `resourceMapAlias`, `decodeResultResource`, `decodeResultResources`, `decodeOrZero` use JSON fallback | Forbidden embedded backend DTO conversion | Use typed resource payloads and explicit fallback errors |
| `internal/services/workorch/biz/backends/snapshot.go` | `decodeSnapshotInfo` JSON converts any to `WorkspaceSnapshotInfo` | Forbidden local snapshot decode | Replace with typed assertion and explicit mapper if needed |
| `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_snapshot_detail.go` | `cloneMapValue` JSON clones generic values | Needs review | Replace with structural clone unless it handles persisted JSON boundary |
| `internal/modules/application/workorchd/biz/appservice/readmodelprojection/acceptance_projection.go` | JSON copies typed values into projection structs | Needs review | Replace with explicit mapper or typed copy helper |
| `internal/modules/application/workorchd/biz/appservice/runtimecore/compressed_context.go` | JSON copies compressed context maps | Needs review | Replace with structural clone unless preserving external JSON schema |
| `internal/modules/application/workorchd/biz/appservice/readmodelprojection/runtime_execution_state.go` | JSON copies contract violation-like projection data | Needs review | Replace with explicit DTO mapper |
| `internal/modules/application/workorchd/biz/appservice/contentports/runtime_virtual_reader.go` | JSON renders virtual file content | Allowed boundary | Keep; this produces user-readable virtual JSON content |
| `internal/modules/application/workorchd/biz/appservice/workspacecore/managed_config_json.go` | JSON reads/writes managed config | Allowed file boundary | Keep |
| `internal/pkg/workorchclient/**` | HTTP client JSON encoding/decoding | Allowed REST boundary | Keep |
| `internal/modules/application/workorchd/service/server/**` | HTTP/SSE JSON encoding/decoding | Allowed REST/SSE boundary | Keep |
| `internal/services/workorch/service/cli/root.go` | CLI JSON output | Allowed user output boundary | Keep |

## Target Architecture

### Typed Projection Mapper

Add an appservice-level mapper package or file that converts `readmodelcore` to `workorchport` without JSON:

- `PortWorkspaceInfo(readmodelcore.Workspace) workorchport.WorkspaceInfo`
- `PortRunInfo(readmodelcore.Run) workorchport.RunInfo`
- `PortWorkspaceSnapshotInfo(readmodelcore.WorkspaceSnapshot) workorchport.WorkspaceSnapshotInfo`
- `PortDecisionInfo(readmodelcore.Decision) workorchport.DecisionInfo`
- `PortApprovalInfo(readmodelcore.Approval) workorchport.ApprovalInfo`
- equivalent mappers for result, artifact, patch, usage, context budget, event record, task progress, heartbeat, convergence, scheduler diagnostics, and acceptance projection.

Mapper rules:

- Preserve all fields already exposed by `workorchport`.
- Use exact ID types from `common`.
- Do not infer state from display text.
- Do not drop projection revision, blocking IDs, active run IDs, or current ViewModel status.
- Dynamic maps remain maps and are structurally cloned, not JSON cloned.

### Typed CLI/EventHub Result

CLI/EventHub result payload should treat typed DTO as the primary contract:

- Appservice handlers return typed resources for embedded/local consumers.
- `Resource` maps are optional display/resource compatibility only and must not be used by embedded backend as the primary DTO.
- If a backend expects `T` and `TypedResource` is missing or mismatched, return a typed boundary error instead of JSON-decoding the map.
- Bulk results use `TypedResources []any` with every item typed; mixed or missing typed resources fail fast.

### Typed Command Payloads

Command payload creation should use explicit DTOs wherever the caller is internal:

- TUI/local backend should construct typed command requests already defined in `workorchport`.
- EventHub command payload may still be `map[string]any` only at the event-envelope compatibility boundary, but appservice must decode it through explicit field readers, not JSON generic decoding.
- Remove generic `DecodeOrZero[T]`; keep primitive helpers like string/int/bool/string-slice/map readers.

### Structural Clone Helper

For intentionally dynamic maps, use one shared structural clone helper:

- Supports `map[string]any`, `[]map[string]any`, `[]any`, primitive values, and nil.
- Has a bounded depth.
- Does not serialize through JSON.
- Lives in the narrow layer that needs it, or in a small shared appservice helper if multiple appservice packages need it.

## Iterative Closure Tasks

### P0-1: Add ReadModel -> Port DTO Mapper

Files:

- Add mapper near `internal/modules/application/workorchd/biz/appservice/readmodelports` or a dedicated `portprojection` package under appservice.
- Update mapper tests in the same package.

Acceptance:

- `WorkspaceSnapshotInfo` can be built from `readmodelcore.WorkspaceSnapshot` without `encoding/json`.
- Test covers workspace state, projection revisions, ViewModel, active run, blockers, acceptance current focus, runtime execution, convergence, run command status, decisions, approvals, and event records.
- Mapper is business-neutral and contains no template/language/tool special cases.

### P0-2: Remove `clicore.typedWorkspaceSnapshotInfo` JSON Round Trip

Files:

- `internal/modules/application/workorchd/biz/appservice/clicore/resources.go`

Changes:

- Replace `typedWorkspaceSnapshotInfo` with the explicit mapper.
- `workspaceSnapshotResourceMap` attaches the typed mapped DTO under `events.CLIResultTypedResourceKey`.
- Remove `encoding/json` import if no longer needed.

Acceptance:

- `ResourceMap(readmodelcore.WorkspaceSnapshot{})` returns a typed `workorchport.WorkspaceSnapshotInfo`.
- No `json.Marshal` / `json.Unmarshal` remains in `clicore/resources.go`.

### P0-3: Remove `clicore.ResourceMapAlias`

Files:

- `internal/modules/application/workorchd/biz/appservice/clicore/resources.go`
- Appservice CLI command handlers that currently rely on implicit fallback.

Changes:

- Replace generic fallback with explicit typed cases for every command resource currently returned.
- Unknown resource type returns nil or explicit error at call site; do not JSON-convert it.

Acceptance:

- Static scan shows no `ResourceMapAlias`.
- CLI command unit tests still cover workspace, run, goal, DoD, decision, approval, template, memory, config result mapping.

### P0-4: Remove Embedded Backend JSON Result Decode

Files:

- `internal/services/workorch/biz/backends/eventhub.go`

Changes:

- `decodeResultResource[T]` only accepts `TypedResource`.
- `decodeResultResources[T]` only accepts `TypedResources`.
- If typed resource is missing, return an internal typed-boundary error with command/resource context.
- Remove `resourceMapAlias` and JSON fallback decode.

Acceptance:

- Embedded backend tests prove workspace snapshot/run/decision/approval/list paths use typed resources.
- A test with missing typed resource fails clearly instead of silently JSON-decoding `Resource`.
- No `json.Marshal` / `json.Unmarshal` remains for internal DTO decode in `eventhub.go`.

### P0-5: Remove `decodeSnapshotInfo` JSON Round Trip

Files:

- `internal/services/workorch/biz/backends/snapshot.go`

Changes:

- Replace `decodeSnapshotInfo(any)` with typed assertion:
  - accept `WorkspaceSnapshotInfo`
  - accept `*WorkspaceSnapshotInfo`
  - reject everything else

Acceptance:

- No `encoding/json` import remains in `snapshot.go`.
- Snapshot tests cover typed value and pointer cases plus invalid input.

### P1-1: Remove Generic Command Payload JSON Decoders

Files:

- `internal/modules/application/workorchd/biz/appservice/clicore/payload_values.go`
- `internal/modules/application/workorchd/biz/appservice/clicore/commands.go`
- `internal/services/workorch/biz/backends/eventhub.go`

Changes:

- Delete `DecodeOrZero[T]` and `decodeOrZero[T]`.
- Replace remaining usage with explicit field readers.
- Add missing primitive readers only when needed, for example optional string, string slices, nested map, bool, int.

Acceptance:

- Static scan shows no `DecodeOrZero` / `decodeOrZero`.
- Command helper tests cover every command builder without JSON.

### P1-2: Review ReadModel Projection JSON Clones

Files:

- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_snapshot_detail.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/acceptance_projection.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/runtime_execution_state.go`
- `internal/modules/application/workorchd/biz/appservice/runtimecore/compressed_context.go`

Changes:

- Replace JSON clone/copy with structural clone or explicit mapper.
- If any use is truly an external JSON schema boundary, document it with a short code comment and keep it scoped.

Acceptance:

- No JSON round-trip remains for internal projection cloning.
- Tests show projection compaction preserves key fields and does not mutate source values.

### P1-3: Add Static Boundary Test

Files:

- Add a test under an appropriate package, for example `internal/services/workorch/biz/backends` or a repository-level script/test.

Rule:

- Forbidden patterns:
  - `encoding/json` in `internal/services/workorch/biz/backends/eventhub.go`
  - `encoding/json` in `internal/services/workorch/biz/backends/snapshot.go`
  - `encoding/json` in `internal/modules/application/workorchd/biz/appservice/clicore/resources.go`
  - generic `DecodeOrZero` helpers
  - JSON round-trip functions named `*Alias`, `decode*Resource`, or `typed*Info` in internal DTO paths

Allowed list:

- REST/SSE server.
- `internal/pkg/workorchclient`.
- CLI explicit JSON output.
- managed config JSON files.
- virtual JSON content rendering.
- tests that decode HTTP/file JSON.

Acceptance:

- Test fails when a new forbidden internal JSON conversion is added.
- Allowed boundary files remain permitted.

### P2-1: Reduce Full Snapshot Dependence in TUI

Files:

- `internal/services/workorch/service/tui/model.go`
- `internal/services/workorch/service/tui/commands.go`
- local/remote backend adapters as needed

Changes:

- Keep TUI main view on projection/ViewModel events where possible.
- Fetch full snapshot only for explicit details or when projection revision indicates missing active detail.
- This task should happen after DTO cleanup so profiling reflects real remaining costs.

Acceptance:

- Idle TUI with active workspace no longer repeatedly triggers full `WorkspaceSnapshot` conversion.
- pprof no longer shows `CLICommandDispatcher.handleWorkspaceCommand -> ResourceMap -> json` as a material hotspot.

## Validation Plan

Run after each phase:

```bash
rg -n "json\\.(Marshal|Unmarshal)|encoding/json|DecodeOrZero|ResourceMapAlias|resourceMapAlias|typedWorkspaceSnapshotInfo|decodeSnapshotInfo" \
  internal/modules/application/workorchd/biz/appservice/clicore \
  internal/services/workorch/biz/backends \
  -g '*.go'

go test ./internal/modules/application/workorchd/biz/appservice/... -count=1
go test ./internal/services/workorch/... -count=1
go test ./internal/pkg/workorchport/... -count=1
```

Run before live verification:

```bash
go test ./... -count=1
```

Live verification:

1. Start workorchd and TUI embedded/local mode.
2. Open a workspace that is running or waiting for decision.
3. Observe CPU for at least 60 seconds without user input.
4. Capture pprof for 10 seconds.
5. Confirm JSON functions are not material CPU consumers in internal TUI/EventHub snapshot flow.
6. Confirm TUI prompt, workspace list, and main view still match workorchd projection state.

## Non-Goals

- Do not change REST API response schema in this closure unless a mapper exposes an existing mismatch.
- Do not change workspace execution state machine in this closure.
- Do not add template/Soul/software-development-specific logic.
- Do not optimize LLM provider JSON request rendering; that is an external provider boundary and separate from internal DTO exchange.

## Closure Status

- P0-1 done: added explicit readmodel-to-port DTO mapper under `readmodelports` with mapper tests.
- P0-2 done: `clicore.ResourceMap` now attaches typed `WorkspaceSnapshotInfo` without JSON round-trip.
- P0-3 done: removed `ResourceMapAlias` fallback and replaced known resource conversions with explicit typed cases.
- P0-4 done: embedded EventHub backend consumes `TypedResource` / `TypedResources` and fails fast on missing typed DTOs.
- P0-5 done: snapshot boundary now accepts only typed `WorkspaceSnapshotInfo` values or pointers.
- P1-1 done: removed generic `DecodeOrZero` / `decodeOrZero` command payload decoders in the internal CLI/EventHub path.
- P1-2 done: readmodel projection and runtime compressed context cloning use explicit field mappers or structural copies instead of JSON clone.
- P1-3 done: added static boundary test covering forbidden internal DTO JSON patterns.
- P2-1 done: TUI list refresh now requests full workspace snapshot only when detail is missing, detail revision is older than overview, or the active workspace changed; list responses no longer queue a duplicate snapshot while one is already in flight.

## Completion Criteria

- Internal TUI/EventHub/appservice DTO exchange no longer depends on JSON round-trip conversion.
- REST/SSE and remote client JSON boundaries remain intact.
- All removed fallbacks are replaced by typed DTO mappers or explicit command builders.
- Static boundary test prevents regression.
- Live pprof no longer identifies internal snapshot JSON conversion as a CPU hotspot.
