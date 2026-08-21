# WorkOrch Product Interface Overview

Status: normative

Type: product-interface-index

Last Verified: 2026-08-21

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
- [Service Command API](service-api.md)
- [Service Read API](service-read-api.md)
- [Service Acceptance](service-acceptance.md)
- [Deployed Runtime Convergence Scenarios](deployed-convergence-scenarios.md)
- [Governed Concurrency Load Test](loadtest-baseline.md)
- [TUI](tui/overview.md)
- [Web Console](web-console.md)

One-off UI optimization measurements and dated bundle reports are not design contracts and are not stored in `docs/`.

## Shared Rules

- Web/TUI/client never submit target Run state, successor ID, next action or authority patch.
- Decision/Approval resolution uses the corresponding owner API; the interface does not auto-Resume afterward.
- Definition clarification不是可由通用Decision answer继续的治理项：pre-seal票据只引导修正all-of candidate；Runtime票据
  还要求显式Restart，并由Run/Decision owner专用typed command/result绑定replacement intent。
- command acknowledgement means accepted for processing; final state comes from owner projection/SSE.
- projection分别展示coverage/assessment/evidence、recoverable wait reason/condition和execution-budget view；客户端不得
  合并为authority status、解除wait或修改消费。
- local and remote TUI expose the same business semantics through different transports.
- projection may be rebuilt or refreshed but cannot repair owner state.
- raw internal paths, credentials, provider bodies and opaque aggregate content are not public authority fields.
