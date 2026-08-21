# Workspace Soul Routing

Status: normative

Type: profile-capability

Last Verified: 2026-08-18

## Purpose

Soul routing resolves Workspace/system declarations into immutable typed Soul content for a fresh Runtime activation. Soul
text guides model behavior; it is not an authorization token or a Runtime state owner.

## Owner And Resolution Boundary

- Soul definitions and versions are owned by `blocks/soul`;
- Workspace `defaultSoulId` and stage bindings are owned by `blocks/workspace`;
- workorchd reads both owners through concrete EventHub contracts and compiles `RuntimeBootstrap.Activation`;
- Run owner seals the bootstrap with the Run;
- Orchestrator and LLM consume the sealed typed content and do not re-read Soul/Workspace state on Resume;
- changing Workspace bindings affects only a later fresh activation/Restart, not the active Run.

The public workspace execution request does not accept Run-level Soul overrides. Internal activation helpers retain a bounded
run-override input for trusted application composition, but it is not an HTTP/TUI authority surface.

## Resolution Order

For a fresh activation, the implementation resolves:

1. trusted run-stage binding, when explicitly supplied by the application use case;
2. Workspace stage binding;
3. trusted run default Soul;
4. Workspace default Soul;
5. configured system default Soul;
6. explicit `unbound` when none exists.

Stage binding wins over a default for that stage. An unresolved referenced Soul is an error; it cannot silently fall back to
a different identity.

## Typed Activation Contract

The result is not an `executionContext` map. It is represented by:

- `RuntimeActivationContext.DefaultSoul` as a frozen `RuntimeSoulContext`;
- `RuntimeActivationContext.StageSoulBindings[]` with `stage`, `source` and frozen Soul content;
- Soul allowlists inside each `RuntimeSoulContext`;
- `RuntimeActivationContext.Skills`, `MCPServerIDs` and `MCPCapabilities`, validated against effective Soul policy.

Each frozen Soul context includes identity, descriptive fields, instructions and the structured Skill/MCP/capability/tool
allowlists needed for later admission. `soulMd` text alone cannot grant access.

## Capability Boundary

- Soul routing selects instruction/policy content; it does not execute File/Git/Shell/SSH/Web/MCP operations;
- capability selection must satisfy Workspace/profile, Skill requirements and effective Soul allowlists;
- Orchestrator core remains business-neutral and cannot choose Soul by parsing Goal/DoD text;
- LLM owner receives the selected bounded context but cannot modify routing or owner state;
- Resume/recovery uses the sealed activation snapshot; no mutable binding lookup or patch is allowed.

## Product Projection

Management surfaces:

- `GET /workspaces/:workspaceId/soul-bindings`;
- `PUT /workspaces/:workspaceId/soul-bindings`;
- Workspace projection fields `defaultSoulId` and `soulBindings`;
- Run context-bundle/observability summaries derived from the sealed activation.

The UI should show effective Soul identity and source as bounded context. It must not expose raw Runtime aggregate content or
imply that changing Workspace settings mutates a running Run.

## Verification

- binding normalization and priority: `internal/modules/application/workorchd/biz/appservice/appmodelcore` tests;
- owner lookup and typed activation snapshot: `activationports` tests;
- Workspace owner CRUD and reference validation: `workspaceports` tests;
- deployed template/Soul flow: `make verify-workspace-template-soul`;
- full execution context packaging: mock/real LLM workspace execution verifiers.

## Related Documents

- [Instruction Assets](../../10-domain-model/instruction-assets.md)
- [Capability Catalog](../../10-domain-model/capability-catalog.md)
- [Memory](../../10-domain-model/memory.md)
- [Context Assembly Flow](../../00-core/runtime-request-context-assembly.md)
- [Block Boundaries](../../30-module-architecture/block-boundaries.md)
- [Service API](../../50-product-interface/service-api.md)
- [Built-in Default Resources](builtin-default-resources.md)
