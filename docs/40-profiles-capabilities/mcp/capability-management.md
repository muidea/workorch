# MCP Capability Management

Status: active

Type: profile-capability

Last Verified: 2026-08-21

## Purpose

MCP capability management separates declarative catalog/selection metadata from concrete Runtime execution. The split is a
single-owner boundary, not two representations of the same mutable state.

## Owner Split

| Concern | Owner |
| --- | --- |
| MCP server definition, discovered catalog metadata and `MCPCapability` availability/provenance | `blocks/mcpregistry` |
| Workspace/profile/Soul/Skill references used to select allowed capabilities | their respective state owners; compiled by workorchd |
| Frozen per-Run MCP metadata | `RuntimeBootstrap.Activation`, sealed by Run owner |
| Concrete `mcp/tool_call`, `mcp/resource_read`, `mcp/prompt_get` operation | `blocks/mcp` |
| Candidate/sealed dispatch/evidence acceptance | `kernel/orchestrator` |
| UI list/detail/usage view | workorchd projection only |

MCP Registry does not execute tools. MCP execution does not mutate catalog definitions. Orchestrator does not inspect MCP
implementation or call a registry/service facade.

## Catalog Contract

An `MCPCapability` provides bounded metadata such as:

- identity, server identity, `kind`, technical name and display name;
- description/category/tags/stage hints;
- input/output summary and optional URI/schema metadata;
- risk level, approval policy and the single authoritative `enabled` availability flag;
- Runtime operation kind, effect scope, idempotency class and validation proof applicability.

Server and Capability are one revisioned Registry aggregate. Writes use whole-aggregate CAS, list queries are bounded and
carry a stable `RegistryRevision`, and Runtime/activation consumers use owner-issued consistent snapshot commands rather than
joining separately observed Server and Capability lists. Capability provenance is the owner-issued `source`, `sourceRef` and
monotonic item `revision`; there is no separate draft/active/deprecated lifecycle state.

Effect scope, idempotency and `direct | auxiliary | not_declared` proof applicability are explicit Registry-owner metadata.
Runtime must not infer them from tool name, description, category, risk label or DoD text. MCP execution independently compiles
technical `ready | prerequisite_required | unavailable` from the detached registry snapshot and sealed input facts; only
`ready + direct` may enter provider-visible validation recipes. A ready auxiliary tool such as the default `lsp_status` remains
callable/auditable but cannot directly close CWI coverage.

The management API exposes server/capability list/create/read/update plus server tool/resource/prompt discovery. Exact routes
are listed in [Service API](../../50-product-interface/service-api.md).

Server environment configuration is reference-only (`environmentRefs`). `env://ENV_NAME` is resolved by MCP execution at the
last responsible moment; supported workspace placeholders contain no secret. Plaintext secret values are rejected before
Registry persistence or EventHub publication.

## Activation And Selection

For a fresh activation, workorchd resolves Workspace/profile defaults, Skill requirements and effective Soul allowlists,
then freezes allowed server IDs and `RuntimeMCPCapabilityContext` values in `RuntimeActivationContext`. The public execution
request cannot inject MCP server/capability IDs.

Selection and use are different facts:

- sealed activation metadata defines what may be considered;
- a sealed dispatch names one admitted concrete operation;
- capability owner result proves what was actually attempted/effected;
- accepted operation evidence proves what Orchestrator admitted into convergence.

Resume/recovery reuses the sealed activation/catalog observation. Workspace or registry changes affect a later fresh
activation and cannot rewrite an active Run.

SM-2 catalog acquisition and feasibility are strictly static. They may read one Registry aggregate, validate the bounded
input schema and inspect local HTTP/stdio launch prerequisites, but they do not contact an MCP endpoint, start a process,
sample tool behavior, create a receipt or otherwise execute an operation. A `safe_to_replay` declaration does not turn a
feasibility probe into SM-3 execution authority.

## Runtime Execution

`blocks/mcp` owns the concrete `ExecuteOperationCommand/Result` topic and performs transport/session/tool/resource/prompt
work. Orchestrator's typed event adapter seals the complete persisted operation together with the selected owner-catalog
snapshot, exact Capability and operation policy; there is no independently mutable execution `Data` copy. Before transport,
the owner verifies Orchestrator source, current claimed execution key/fence, payload hash, current Registry revision, exact
enabled Server/Capability/allowlist/schema/effect/idempotency and any exact operation-scoped approval grant. A missing receipt
is executable only when Orchestrator's receipt query has already proved that this exact execution is active and claimed.

The result crosses EventHub only as one `RuntimeOperationReceipt`. Provider output and stream events are bounded and stored as
owner-private content-addressed evidence; the receipt carries a safe summary, SHA-256 and opaque evidence/capability refs, not
the raw response, endpoint, credential or transport diagnostic. A durable capability must return a typed `workorchEffect`
observation with matching scope, changed/no-op revisions and bounded resource changes. The owner never derives `Changed=true`
from Registry metadata alone.

It must not:

- choose Runtime action, focus or evidence acceptance;
- create/resolve Approval or Decision state;
- update MCP Registry metadata as a side effect of execution;
- receive a generic Orchestrator operation envelope or `map[string]any` authority payload.

## Governance

Risk and approval policy are declarative inputs. Whether a specific dispatch needs authorization is decided by the sealed
Runtime/governance contract. Approval owner records the authorization result; MCP owner consumes only the typed immutable
fact needed for the operation. The UI may correlate approval and capability projections but cannot decide authorization.

The MCP requirement binds the exact Capability, complete operation payload hash, catalog/Registry/capability revision,
effect and idempotency. A stale, reused or differently scoped grant fails before any network/process action. The owner may
return `governance_required`; it cannot create, approve, reject or resolve the Approval itself.

## Projection Rules

Catalog pages may show display name, kind, server, risk, approval policy, stage hints, enabled and provenance. Run pages should
distinguish:

- allowed/selected capability snapshot;
- actually used capability and operation receipt;
- accepted evidence/result/artifact;
- related governance fact.

HTTP/stdio output is limited to 1 MiB and 128 stream/notification events; invalid or oversized protocol data is rejected
rather than silently truncated. HTTP redirects are bounded and same-origin only, so operation payloads are not forwarded to a
different host. Large MCP payloads use validated owner-private raw refs. Projection summaries and Eventstream records must
not contain credentials, endpoints, raw provider errors, unbounded tool output or live client/session objects.

## Verification

- registry CRUD/discovery and scope tests live with `blocks/mcpregistry` and workorchd server;
- typed operation/admissibility tests live with `blocks/mcp`;
- `make verify-mcp-stdio` is the deterministic deployed baseline;
- `make verify-real-mcp-stdio`, `make verify-real-weather` and `make verify-weather-ticket-approval` are explicit live gates;
- Runtime candidate/dispatch/evidence assertions follow
  [Runtime Owner Convergence Matrix](../../60-verification/runtime-owner-convergence-matrix.md).

## Related Documents

- [Kernel Context / Capability](../../30-module-architecture/kernel-context-capability.md)
- [Capability Adapter Architecture](../../30-module-architecture/blocks-runtime-adapters.md)
- [Service API](../../50-product-interface/service-api.md)
- [Service Acceptance](../../60-verification/service-acceptance.md)
