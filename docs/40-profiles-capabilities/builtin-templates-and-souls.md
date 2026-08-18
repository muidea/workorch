# Built-In Templates And Souls

Status: active

Type: profile-capability

Last Verified: 2026-08-18

## Current Inventory

Embedded defaults live under `internal/pkg/workorchdefaults/defaults`.

The current inventory is exact, not a minimum:

- 6 templates: `generic-governed`, `weather-query`, `software-dev`, `code-review`, `doc-generator`, `data-analysis`;
- 36 Souls: one per template for each of `planning`, `execution`, `decision`, `approval`, `validation`, `artifact`;
- each template `defaultSoulId` equals its `execution` binding.

## Ownership And Startup

`internal/pkg/workorchdefaults` loads immutable owner-neutral default DTOs. At startup, `kernel/system` coordinates concrete
ensure commands; Workspace owner persists templates and Soul owner persists Souls. System does not define those command schemas
or write their repositories.

MCP defaults follow the same pattern through MCP Registry owner.

All built-ins are seed-once, user-editable owner resources. Ensure creates a built-in ID only when it is absent and never
replaces an existing Soul, WorkspaceTemplate, MCP Server or MCP Capability. Deleting a built-in allows the next startup to
recreate it from the then-current embedded declaration. An embedded-default change does not migrate existing resources; any
future forced refresh requires a separate versioned and auditable owner migration contract.

## Template Contract

A built-in template may declare:

- ID, name, description, starter Goal and DoD items;
- default Soul and role bindings;
- context/evidence/capability policies that the activation compiler understands;
- profile evidence requirements and execution constraints.

Template declarations are inputs to bootstrap compilation. Stage-like labels and descriptive policy fields remain profile
semantics; they cannot directly select SM-2 convergence state, seal dispatch or write evidence.

## Soul Contract

Each Soul exposes owner-managed metadata and full `soulMd` content. A Soul may constrain allowed Skill/MCP/capability/tool
references and guide model behavior. It cannot grant a capability missing from the owner catalog or bypass governance.

Fresh Start/Restart resolves Soul content and bindings into an immutable activation snapshot. Resume/recovery uses that sealed
snapshot and does not reread a newly edited Soul into the active Run.

## Resource Layout

```text
internal/pkg/workorchdefaults/defaults/
  templates.json
  souls.json
  souls/<template>/<role>.md
  mcp_servers.json
  mcp_capabilities.json
```

`souls.json` contains metadata and a validated `soulMDRef`; bodies are embedded from the referenced Markdown files. The
reference must stay below `souls/`, use the `<template>/<role>.md` shape and resolve to non-empty content.

## Compatibility Boundary

The activation compiler may read declarative template fields retained for compatibility, but only typed Runtime Definition,
Profile, Evidence and Execution contracts can enter authority. Active default resources use semantic-plan, operations,
binding, owner-receipt, evidence and assessment language; a removed graph/next-action schema is rejected by static tests.

## Verification

- `go test ./internal/pkg/workorchdefaults -count=1`
- `make verify-workspace-template-soul`
- `make verify-runtime-profile-software-dev`

Verification must cover exact inventory, reference safety, non-empty Soul content, owner ensure idempotency, binding validity and
the absence of removed authority vocabulary from active provider-facing guidance.

Default-resource tests cover inventory/reference/ensure behavior and explicitly reject retired provider authority vocabulary.
