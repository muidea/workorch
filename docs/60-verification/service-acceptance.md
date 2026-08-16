# WorkOrch Service Acceptance

Status: active

Type: verification-contract

Last Verified: 2026-08-16

## Purpose

本文定义 workorchd HTTP/Web surface、file-backed persistence 与已部署实例的验收边界。它验证 transport、projection
和 owner integration，不用 API 投影替代 Runtime 状态机验证。Runtime transition、CAS、restart 与 terminal delivery
断言见 [Runtime Owner Convergence Matrix](runtime-owner-convergence-matrix.md)。

## Acceptance Layers

| Layer | Scope | Main evidence |
| --- | --- | --- |
| Static boundary | route/DTO/port、禁止依赖、owner-neutral transport | server package tests + `make verify-boundary-contract` |
| In-process HTTP | status code、scope、pagination、SSE、raw content、projection cache | `go test ./internal/modules/application/workorchd/service/server -count=1` |
| Service smoke | health/readiness/metrics/console/config/list endpoints | `make verify-service` |
| CI-safe business | template/Soul、mock LLM、MCP stdio、workspace activation | `make verify-ci-mvp-file` |
| Persistence/restart | owner repositories、EventRecord/SSE replay、Runtime re-drive | focused repository/server tests + restart scripts |
| Real-provider live | actual provider/MCP/weather/closure behavior | explicit `verify-real-*` / Runtime live targets |

Passing a higher layer does not excuse a lower-layer regression. Conversely, a unit test cannot close a deployed live
gate that explicitly requires provider, process restart or filesystem evidence.

Container, software-dev, governance-resolution and service-restart wrappers now exercise the current typed contracts. Focused
wrappers reject an empty `go test -run` selection, and the boundary verifier prevents removed routes/provider authority fields
from returning. Historical filenames do not grant compatibility behavior beyond the assertions documented here.

## Current Mutation Contract

The only execution mutation endpoint is:

```http
POST /api/v1/workorch/workspaces/:workspaceId/execution-activations
```

Acceptance must prove:

- only `resume` and `restart` are admitted; empty intent normalizes to `resume`;
- body fields are limited to `priority`, `requestId`, `reason`, `source`, `intent`;
- `runId`, Goal/DoD content, provider selection and raw Runtime contract fields are rejected;
- accepted response is `202` and carries normalized request metadata;
- asynchronous scheduler failure becomes an execution-observation/projection failure, not an owner state rewrite;
- the same workspace/request may be process-locally coalesced, while durable idempotency remains with the owners;
- removed run create/start/resume and `execution-restarts` routes return `404` and create no projected command.

Do not reintroduce tests for `POST .../runs`, `POST .../runs/:runId/start`, `POST .../runs/:runId/resume` or
`ExecutionContextPatch` as successful behavior. Their rejection is the compatibility guard.

## In-Process HTTP Baseline

Run:

```bash
go test ./internal/modules/application/workorchd/service/server -count=1
```

The package baseline covers at least:

- all supported routes are registered and explicitly removed mutating routes are absent;
- production transport imports neither Runtime session/EventHub nor root appservice facade;
- Workspace/Goal/DoD scope validation and owner-generated identity;
- workspace LLM selection completeness, CAS revision and scope isolation;
- execution admission, early `202`, request-id projection, rejected async outcome and restart intent;
- Decision/Approval single and batch resolution;
- Memory/MCP/Soul/Skill/Template management;
- workspace-scoped Run reads and child-resource pagination;
- status wait, terminal snapshot cache and typed owner-notification invalidation;
- SSE filters, projection-only default, replay by `Last-Event-ID`, persisted replay after restart and bounded buffering;
- Artifact URI / Runtime raw-ref allowlisting and content response headers;
- file-backed persistence, long safe IDs and workspace isolation;
- graceful shutdown deadline and pprof/console/static asset registration.

## Deployed Smoke

`make verify-service` runs `scripts/verify-workorch-http.sh` against an already running service. It does not build, start,
stop or restart the process.

```bash
WORKORCH_BASE_URL=http://127.0.0.1:8080 make verify-service
```

The script verifies:

- `/healthz`, `/readyz` and `/metrics`;
- embedded console HTML, CSS, entry bundle and lazy chunks;
- expected current navigation/interaction strings and absence of retired Web entries;
- configuration, workspace list, Run list and bounded EventRecord list wrappers.

Temporary evidence is removed on success. Set `WORKORCH_VERIFY_KEEP_ARTIFACTS=1` or
`WORKORCH_VERIFY_ARTIFACT_DIR=<dir>` to retain it. Connection and request timeouts are configurable through the script's
`WORKORCH_VERIFY_*` variables.

## CI-Safe Deployed Baseline

```bash
WORKORCH_BASE_URL=http://127.0.0.1:8080 make verify-ci-mvp-file
```

This aggregates:

- `verify-service`;
- `verify-workspace-template-soul`;
- mock-provider `verify-llm`;
- deterministic MCP stdio `verify-mcp-stdio`.

It is the deployed CI-safe baseline: no real provider credentials, weather network dependency or implicit service process
management. The `-file` aliases target the same deployed-instance contract; backend identity is discovered from the service.

## Persistence And Restart

File-backed acceptance must distinguish four data classes:

| Data | After process restart |
| --- | --- |
| Workspace/Goal/DoD/selection | owner repository reload, same identity/revision |
| Run lifecycle and opaque RuntimeStateRecord | Run owner reload; Orchestrator re-drive from aggregate only |
| EventRecord / SSE replay cursor | persisted query/replay remains ordered and workspace-scoped |
| application projection cache | may rebuild from owner snapshots/events; never becomes authority |

Required guards:

- no startup scan mutates inactive/running Run state through workorchd;
- no stale Step/watchdog/checkpoint replay drives Runtime;
- duplicate inventory/notification/re-drive is idempotent;
- non-current baseline fails closed with typed disposition and performs no provider/capability call;
- current-baseline Restart preserves `ReplacementIntent` and terminal delivery ordering before successor creation;
- repeated restart/resume cannot create duplicate successor, attempt, evidence or terminal delivery.

Relevant deployed targets include:

```bash
make verify-service-restart-recoverable-e2e
make verify-runtime-restart-recovery-live
make verify-run-resume-gap-confirmation-e2e
make verify-dual-workspace-resume-replan-e2e
```

Some filenames retain historical wording; acceptance follows the current workspace-scoped `resume|restart` contract, not
the retired implementation implied by a script name.

## Projection, Pagination And Raw Content

Acceptance should cover the following externally visible rules:

- workspace/run scope is resolved before repository query; duplicate IDs in other workspaces cannot leak;
- `limit` and `cursorAfterId` produce stable, non-duplicated pages;
- Run status `waitState` uses a bounded positive timeout and wakes on typed Run notification;
- terminal cache invalidation/warmup never treats Eventstream payload as owner state;
- default SSE view emits projection changes; explicit owner/event views remain filtered by workspace/run;
- replay resumes strictly after the supplied event ID and tolerates process restart when records are persisted;
- list/detail payloads carry bounded summaries; large provider/artifact content is exposed only by validated ref/URI;
- path traversal, wrong root, missing scope and cross-workspace ref access fail closed.

## Error Contract

Tests must preserve the mapping defined by the server:

| Error family | HTTP |
| --- | --- |
| invalid argument/body/query | `400` |
| not found/removed route | `404` |
| blocked/conflict/stale revision | `409` |
| admission wait canceled | `408` |
| internal/dependency failure | `500` |

Unknown JSON fields are invalid. Runtime Decision action conflicts additionally return structured remediation fields; tests
must check stable code/action metadata without matching volatile diagnostic prose.

## Real-Provider And Live Gates

Real-provider checks are explicit and excluded from CI:

```bash
make verify-real-llm
make verify-real-runtime-closure
make verify-real-mcp-stdio
make verify-real-vector-memory
make verify-real-weather
make verify-weather-ticket-approval
make verify-runtime-positive-convergence-live
```

They must record the deployed revision/config identity, workspace/run IDs, terminal or bounded-live disposition, provider
call evidence and filesystem/artifact evidence where applicable. A valid artifact without later convergence is positive
progress but not live closure.

## Change Rule

Any service interface change must update, in one change:

1. route registration and request/response DTO;
2. native route and behavior tests;
3. [Service API](../50-product-interface/service-api.md);
4. Web/TUI client contract when consumed there;
5. this acceptance matrix and the relevant deployed script.

Historical acceptance records belong under `docs/90-archive`; they must not remain in this active contract after their API or
execution path is removed.
