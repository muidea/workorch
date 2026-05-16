# WorkOrch Workspace Deliverable Boundary Design

## Purpose

This document defines the workspace data boundary for the next WorkOrch implementation phase.

The confirmed direction is:

- remove the extra `working/` level;
- treat the workspace deliverable directory itself as the user project root;
- move all WorkOrch runtime, evidence, context, memory, event, repair, and LLM-call data into WorkOrch-managed internal directories;
- do not preserve compatibility with the previous `.workorch-data/workspaces/<workspace-id>/working` layout.

This document supersedes the `working/` assumptions in `docs/workspace-data-layout-plan.md` for new implementation work.

## Goals

- Keep user deliverables clean and directly usable.
- Prevent WorkOrch internal runtime data from polluting project output.
- Make workspace export/package operations straightforward.
- Give WorkOrch freedom to evolve internal storage without changing user project layout.
- Keep core framework functionally neutral: the boundary is about directory responsibility, not business semantics.

## Non-Goals

- Do not retain old `workingDirectory` compatibility in this phase.
- Do not store runtime events, steps, tool calls, LLM prompts, repair context, memory, or internal indexes in the deliverable directory.
- Do not let LLM/tool execution decide where WorkOrch internal evidence should be stored.
- Do not expose local absolute internal paths through public API responses.

## Directory Model

Recommended root layout:

```text
.workorch-workspaces/
  <workspace-id>/
    <user deliverable files directly here>

.workorch-data/
  internal/
    workspaces/
      <workspace-id>/
        workspace.json
        config/
        goals/
        dods/
        memory/
        context/
        runs/
        events/
        artifacts/
        llm-calls/
        indexes/
```

The deliverable directory:

```text
.workorch-workspaces/<workspace-id>/
```

is the project root passed to shell/file/patch/MCP tools as the workspace working directory.

The internal directory:

```text
.workorch-data/internal/workspaces/<workspace-id>/
```

is the WorkOrch-managed state root for runtime repositories and read models.

## Workspace Model

The workspace model should use explicit directory fields:

```json
{
  "id": "ws-...",
  "deliverableDirectory": ".workorch-workspaces/ws-...",
  "internalDirectory": ".workorch-data/internal/workspaces/ws-...",
  "dataPolicy": {
    "deliverableDirectoryPurpose": "user_project_root",
    "internalDirectoryPurpose": "workorch_runtime_state",
    "runtimeDataManagedBy": "workorch"
  }
}
```

`workingDirectory` is replaced by `deliverableDirectory` in new code paths. Runtime adapters receive `WorkspaceDeliverableDirectory` so tool execution can stay bound to the user project root.

## Responsibility Boundary

### Deliverable Directory

The deliverable directory may contain:

- source code;
- tests;
- configuration files that belong to the user project;
- documentation and project assets;
- build outputs if the project naturally produces them;
- workspace-local skill assets only if they are part of the user deliverable contract.

The deliverable directory must not contain:

- run records;
- step/tool-call records;
- event streams;
- memory items or recall traces;
- runtime checkpoints;
- LLM request/response logs;
- repair context packets;
- WorkOrch indexes;
- internal governance tickets/resolutions;
- internal artifact metadata unless explicitly exported as user deliverable content.

### Internal Directory

The internal directory contains:

- workspace metadata;
- goals and DoDs;
- workspace configuration;
- soul bindings;
- runtime runs;
- steps, tool calls, patches, validation results, checkpoints;
- tickets, resolutions, approvals;
- event stream records;
- memory and recall traces;
- LLM call records;
- context projections;
- repair packets;
- internal artifacts and indexes.

## Runtime Tool Rules

Runtime tool execution must follow these rules:

| Component | Directory Used | Rule |
| --- | --- | --- |
| `shell/command` | `deliverableDirectory` | Commands execute in the user project root. |
| `file/read` / `file/edit` / `patch/apply` | `deliverableDirectory` | File operations target deliverable files only. |
| LLM provider runtime logs | `internalDirectory` | Raw request/response and stream data are internal. |
| Event stream | `internalDirectory` | Events are internal state, not user deliverables. |
| Memory | `internalDirectory` | Memory items and recall traces are internal. |
| Result artifacts | both, by type | User-facing deliverables live in deliverable root; internal evidence artifacts live internally. |

WorkOrch core should pass explicit roots instead of inferring them from path names.

## Artifact Boundary

Artifact classification must distinguish:

```text
deliverable_artifact
runtime_evidence_artifact
```

`deliverable_artifact` is user-facing and may point to files under the deliverable directory.

`runtime_evidence_artifact` is WorkOrch internal and should point to internal storage refs.

Public APIs should expose stable logical URIs:

```text
workorch://workspaces/<workspace-id>/deliverables/<path>
workorch://workspaces/<workspace-id>/internal/artifacts/<artifact-id>
```

They should not expose local absolute paths.

## Workspace Creation Contract

On workspace creation, WorkOrch must:

1. Allocate a workspace ID.
2. Create `deliverableDirectory`.
3. Create `internalDirectory`.
4. Persist `workspace.json` under internal storage.
5. Initialize empty internal subdirectories needed by repositories.
6. Bind runtime tools to `deliverableDirectory`.
7. Bind repository/read-model storage to `internalDirectory`.

If directory creation fails, workspace creation must fail atomically enough that partially-created directories can be cleaned up or safely retried.

## Read Model Contract

Workspace and run read models must present two separate views:

1. Deliverable view:
   - files, artifacts, final outputs, project status.
2. Runtime view:
   - runs, events, steps, tool calls, memory, context, repair evidence.

Default workspace detail APIs should not load raw runtime payloads. Runtime details must be paginated and ref-based.

## Migration Policy

Current phase does not require compatibility.

Implementation may:

- ignore old `working/` directories;
- require clean data directories;
- create only the new layout for new workspaces;
- update tests to use the new layout exclusively.

If a later migration is required, it should be a separate explicit migration tool, not hidden compatibility logic inside the core runtime path.

## Acceptance Criteria

The boundary implementation is complete when:

1. New workspaces have no `working/` child directory by default.
2. Tool execution uses `.workorch-workspaces/<workspace-id>/` as cwd/project root.
3. WorkOrch runtime repositories store data under `.workorch-data/internal/workspaces/<workspace-id>/`.
4. No runtime event/step/tool/LLM/memory data is written into the deliverable directory.
5. Workspace API exposes deliverable and runtime views separately.
6. Public API responses do not expose local absolute internal paths.
7. Tests assert that runtime data and deliverable files are physically separated.

## Implementation Status

Status: partially implemented.

Completed in the first收口阶段:

- workspace model and public API now use `deliverableDirectory` and `internalDirectory`;
- default file-backend deliverables are created under `.workorch-workspaces/<workspace-id>/`;
- default file-backend internal state is created under `.workorch-data/internal/workspaces/<workspace-id>/`;
- workspace, goal, DoD, memory, run, step, tool-call, patch, validation, checkpoint, result, artifact, event, ticket, resolution, and approval repositories are rooted under the internal workspace directory;
- shell/file/patch/SSH runtime requests use `WorkspaceDeliverableDirectory`, so tools operate on the deliverable root instead of internal storage;
- purge deletes both the internal workspace state and the deliverable directory;
- tests assert the new physical separation and `go test ./...` passes.

Still pending:

- API/read-model projection cleanup for hiding local internal paths from public responses;
- console text and form naming cleanup from old “working directory” wording;
- runtime storage JSON-size redesign from `workorch-runtime-storage-json-control-design.md`.

## Implementation Notes

Prefer conservative renaming:

- External model/API: `deliverableDirectory`.
- Internal repository root: `internalDirectory`.
- Runtime adapter field has been renamed to `WorkspaceDeliverableDirectory` in the current implementation.

Do not encode software-development-specific behavior in the core framework. The same boundary must support code, documents, data analysis output, and other workspace templates.
