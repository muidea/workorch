# Storage And Read Model Architecture

Status: active

Type: module-architecture

Last Verified: 2026-08-16

## Purpose

本文说明 memory/file repository、workspace-scoped layout、Runtime aggregate persistence、raw content 和 derived read
model 的当前边界。Storage is physical ownership; it does not transfer semantic state-machine ownership.

## Directory Boundary

For a file-backed workspace:

- internal owner data: `<repository.dataDir>/internal/workspaces/<workspaceId>`;
- default deliverables: `<parent-of-dataDir>/.workorch-workspaces/<workspaceId>`;
- an explicit `deliverableDirectory` may replace the default deliverable path;
- workspace index records workspace identity, internal/deliverable paths, state and active Run identity.

Capability side effects target the allowed deliverable/internal scope declared by Workspace policy. Repository metadata and
Runtime raw content stay under the internal root. A path may never cross between them merely because both are on the same
filesystem.

## Owner Repository Groups

The repository initiator creates one backend and exposes owner-scoped groups. Each owner receives only its repositories:

- Workspace: Workspace, Goal, DoD, Template-related owner data;
- Run: Run/Step/ToolCall/Patch/Validation/Checkpoint and opaque RuntimeStateRecord slot;
- Result: Result/Artifact;
- Decision/Approval/Memory/Soul/Skill/MCP Registry/Eventstream: their respective aggregates;
- Orchestrator: EvidenceRecord, legacy coverage record/read surface and opaque candidate-request content carrier.

No application or sibling owner receives the full private repository set. File and memory backends implement the same
repository contracts and must detach mutable slices/maps/entities at their boundaries.

MCP Registry is one revisioned `RegistrySnapshot` aggregate containing Server and Capability metadata. Its repository exposes
only whole-snapshot `Load` plus `Commit(expectedRevision)`; a Server update and the corresponding allowlist reconciliation can
never commit as separate physical transactions.

## Runtime Aggregate Persistence

`RuntimeStateRecord` is semantically owned by Orchestrator and physically stored by Run owner. Run owner validates only:

- current schema envelope;
- workspace/run scope;
- expected revision/CAS monotonicity;
- no missing/downgraded record.

It does not decode authority/focus/candidate/evidence/governance/delivery content. Orchestrator validates the internal
invariants before commit and after reload. Every update carries expected revision; stale or absent CAS fails without overwrite.

Process recovery loads this record through the Run owner typed contract. Eventstream, Step, checkpoint and projection data
cannot reconstruct it.

## Itemized Evidence And Legacy Process Data

Run-scoped item repositories include Step, ToolCall, Patch, ValidationResult, RuntimeCheckpoint, EvidenceRecord, Result and
Artifact data. Their current role is audit, evidence/reference and product detail projection unless a current typed Runtime
contract explicitly consumes them. They are not a second scheduler or transition log.

`ArtifactContinuationRepository` remains in the storage/read surface, but the current Runtime production use is an opaque
carrier for one logical candidate-request complete document. Request session, sequence, budget, candidate allocation and
retry policy remain inside the Orchestrator aggregate. The carrier cannot choose retry/attempt/transition and its raw content
does not enter the default projection.

Historical incremental-artifact plan/chunk/apply state may still be readable for old records, but it is not an alternative
production output state machine.

## Eventstream

Eventstream persists bounded append-only `EventRecord` data with scope/topic/subject/time metadata and optional payload ref.
It supports query, SSE replay and projection rebuild input. It must not:

- repair Workspace/Run/Result/Decision/Approval state;
- infer Runtime next action or terminal state;
- replay a side effect;
- replace owner inventory or Runtime aggregate recovery.

High-watermark/cursor identity is display/audit ordering, not a business revision.

## Projection Architecture

workorchd owns one projection manager/store. It combines immutable owner snapshots, EventRecords, Runtime drive observations
and execution-admission observations into bounded Workspace/Run views.

| Level | Content |
| --- | --- |
| summary | identity, state, blocker/outcome/progress summary |
| detail | bounded evidence lists, observability, context and governance correlation |
| raw | explicitly opened validated payload/artifact content |

Projection revisions only order derived view updates. Projection may be rebuilt or deleted; it cannot write owner repositories
or make a stale view authoritative. Server transport receives read-only projection access; Biz-owned projection component is
the only derived writer/lifecycle owner.

## Scope And Identity

- workspace-scoped paths and queries validate WorkspaceID before resolving Run/resource ID;
- identical Run/resource IDs in different workspaces must not resolve through an unscoped first match;
- compatibility unscoped reads require a unique identity and return conflict/not-found otherwise;
- Run child IDs remain traceable to run/scope but storage path fallback must not redefine object identity;
- long path segments may use deterministic hash suffixes only as a filesystem representation.

## Pagination And Caching

- paged child resources use owner/repository `limit` + cursor contracts;
- list/detail views do not hydrate all raw payloads;
- terminal Run snapshot/status caches are invalidated/warmed by typed owner notification, not Eventstream payload;
- workspace list/detail and TUI/Web refresh use projection or bounded metadata queries, not whole-history scans;
- cache miss may trigger a read/rebuild, never owner mutation.

## Raw Content Safety

Large provider output, artifact content and event payload use bounded metadata plus a validated ref/URI. Opening raw content
requires:

- allowlisted storage root and ref scheme;
- canonical path and traversal/symlink protection;
- workspace/run/resource scope consistency;
- bounded read and safe content headers;
- no credential, prompt or unsafe provider diagnostic leak.

Missing/corrupt raw content may be diagnosed, but projection cannot manufacture replacement content or change owner state.

## Delete And Purge

Deleting a Workspace owner record and purging its managed internal/deliverable directories are distinct operations. Purge is
explicit, path-validated and limited to the resolved workspace layout. Repository code must never use unresolved broad roots,
environment variables or globs as destructive targets.

## Verification

- repository contract tests cover memory/file parity, scope isolation, clone safety and CAS;
- workorchd server tests cover file-backed restart, pagination, SSE replay, raw refs and terminal cache behavior;
- `make verify-boundary-contract` prevents production repository/service leakage;
- [Service Acceptance](../60-verification/service-acceptance.md) defines deployed checks;
- [Runtime Owner Convergence Matrix](../60-verification/runtime-owner-convergence-matrix.md) defines recovery/idempotency guards.

## Related Documents

- [Workspace / Goal / DoD](../10-domain-model/workspace-goal-dod.md)
- [Run / Step / Result / Artifact](../10-domain-model/run-step-result-artifact.md)
- [Event / Observability](../10-domain-model/event-observability.md)
- [Workspace / Run / Result Architecture](blocks-workspace-run-result.md)
- [Service API](../50-product-interface/service-api.md)
