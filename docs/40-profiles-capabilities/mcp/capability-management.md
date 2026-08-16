# MCP Capability Management

Status: active

Type: profile-capability

Last Verified: 2026-08-16

## Purpose

MCP capability management separates declarative catalog/selection metadata from concrete Runtime execution. The split is a
single-owner boundary, not two representations of the same mutable state.

## Owner Split

| Concern | Owner |
| --- | --- |
| MCP server definition, discovered catalog metadata and `MCPCapability` lifecycle | `blocks/mcpregistry` |
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
- risk level, approval policy, enabled/status;
- Runtime operation kind, effect scope, idempotency class and validation proof applicability.

Effect scope, idempotency and `direct | auxiliary | not_declared` proof applicability are explicit Registry-owner metadata.
Runtime must not infer them from tool name, description, category, risk label or DoD text. MCP execution independently compiles
technical `ready | prerequisite_required | unavailable` from the detached registry snapshot and sealed input facts; only
`ready + direct` may enter provider-visible validation recipes. A ready auxiliary tool such as the default `lsp_status` remains
callable/auditable but cannot directly close CWI coverage.

The management API exposes server/capability list/create/read/update plus server tool/resource/prompt discovery. Exact routes
are listed in [Service API](../../50-product-interface/service-api.md).

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

## Runtime Execution

`blocks/mcp` owns the concrete `ExecuteOperationCommand/Result` topic and performs transport/session/tool/resource/prompt
work. It validates scope, selected capability, effect/idempotency metadata and approval fact required by the sealed dispatch.
It returns factual resource/operation outcomes and safe diagnostics only.

It must not:

- choose Runtime action, focus or evidence acceptance;
- create/resolve Approval or Decision state;
- update MCP Registry metadata as a side effect of execution;
- receive a generic Orchestrator operation envelope or `map[string]any` authority payload.

## Governance

Risk and approval policy are declarative inputs. Whether a specific dispatch needs authorization is decided by the sealed
Runtime/governance contract. Approval owner records the authorization result; MCP owner consumes only the typed immutable
fact needed for the operation. The UI may correlate approval and capability projections but cannot decide authorization.

## Projection Rules

Catalog pages may show display name, kind, server, risk, approval policy, stage hints and enabled/status. Run pages should
distinguish:

- allowed/selected capability snapshot;
- actually used capability and operation receipt;
- accepted evidence/result/artifact;
- related governance fact.

Large MCP payloads use validated raw refs. Projection summaries and Eventstream records must not contain credentials,
unbounded tool output or live client/session objects.

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
