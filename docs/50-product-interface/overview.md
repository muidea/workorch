# WorkOrch Product Interface Overview

Status: active

Type: product-interface-index

Last Verified: 2026-08-16

## Interfaces

| Surface | Current role | Authority boundary |
| --- | --- | --- |
| Binary/TUI | default local interactive mode; optional remote backend | submits typed intent and consumes projection |
| HTTP/Web | `--server` mode, REST/SSE and embedded console | validates protocol and composes use cases; no Runtime drive |
| Client/agent ports | local or remote backend abstraction | no alternate lifecycle API |
| Deployment/config | process, repository, provider and policy setup | config mutation does not mutate active owner snapshot silently |

## Current Execution Surface

Users do not create/start/resume a client-selected Run. The only execution mutation is workspace-scoped:

```http
POST /api/v1/workorch/workspaces/:workspaceId/execution-activations
{"intent":"resume"}
```

or `{"intent":"restart"}`. Workspace and Run owners resolve the current Run and perform the owner-local transaction. Read-only
Run endpoints remain available for status, snapshots and diagnostics.

## Documents

- [Deployment](deployment.md)
- [Service API](service-api.md)
- TUI: [requirements](tui/requirements.md), [interaction](tui/interaction-contract.md),
  [display consistency](tui/display-refresh-consistency-design.md)
- [Web Console](web-console/overview.md)

Historical UI optimization measurements are archived under `docs/90-archive/records` and do not define current bundle limits.

## Shared Rules

- Web/TUI/client never submit target Run state, successor ID, next action or authority patch.
- Decision/Approval resolution uses the corresponding owner API; the interface does not auto-Resume afterward.
- command acknowledgement means accepted for processing; final state comes from owner projection/SSE.
- local and remote TUI expose the same business semantics through different transports.
- projection may be rebuilt or refreshed but cannot repair owner state.
- raw internal paths, credentials, provider bodies and opaque aggregate content are not public authority fields.
