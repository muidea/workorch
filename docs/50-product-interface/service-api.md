# WorkOrch Service API

Status: active

Type: product-interface

Last Verified: 2026-08-16

## Purpose And Authority

本文记录 `internal/modules/application/workorchd/service/server` 当前注册的 HTTP/SSE surface。实际 route registration、request DTO、
projection DTO 和 handler test 是接口事实源；本文件不重新定义 owner 状态机。

所有 API 路径以 `/api/v1/workorch` 为业务前缀。HTTP DTO 只用于 transport；跨 owner 调用必须转换为对应 owner
`pkg/events` 中的具体 typed command/result，不能把 HTTP payload 当作 Runtime authority。

## Runtime Mutation Boundary

当前唯一 workspace execution 写入口是：

```http
POST /api/v1/workorch/workspaces/:workspaceId/execution-activations
```

请求只接受用户可控的调度字段：

```json
{
  "intent": "resume",
  "priority": 0,
  "requestId": "optional-idempotency-key",
  "reason": "optional-reason",
  "source": "optional-source"
}
```

- `intent` 为空时按 `resume` 处理，只允许 `resume` 或 `restart`；
- Goal、DoD、provider、Soul、Skill、MCP、Memory 和 capability selection 从 Workspace owner 当前 canonical state
  编译，不接受调用方在本请求中注入；
- `resume` 保留当前 Run aggregate、definition 与 sealed provider selection；
- `restart` 使用 fresh bootstrap，但 successor lifecycle 归 Run owner：先持久化 predecessor 的
  `ReplacementIntent`，再经 `Result -> Workspace -> Run` terminal delivery 后由 Run-private reducer 幂等创建 successor；
- 请求被 admission 接受时返回 `202`。accepted 只表示 typed owner command 已接收，不表示 Run 已终态；
- 非当前 development baseline 返回 owner 定义的 `data_reset_required`，不会创建 replacement intent。

以下旧 mutation surface 已移除，不是兼容 API：

- `POST /workspaces/:workspaceId/runs`；
- `POST /workspaces/:workspaceId/runs/:runId/start`；
- `POST /workspaces/:workspaceId/runs/:runId/resume`；
- `POST /workspaces/:workspaceId/execution-restarts`；
- 任何携带 `executionContext` / `executionContextPatch`、目标 Run state 或 successor identity 的请求。

## Endpoint Inventory

### Operational And Configuration

| Method | Path | Result |
| --- | --- | --- |
| `GET` | `/healthz` | process liveness |
| `GET` | `/readyz` | dependency readiness |
| `GET` | `/metrics` | Prometheus text metrics |
| `GET` | `/debug/pprof/**` | Go pprof surface |
| `GET` | `/configuration` | redacted persisted configuration plus active LLM restart status |
| `PUT` | `/configuration/server` | update supported server fields for next configuration state |
| `PUT` | `/configuration/runtime` | update Runtime configuration |
| `PUT` | `/configuration/llm` | update persisted ai-proxy model/key/budget selection |
| `GET` | `/configuration/llm/models` | active LLM owner catalog summary |
| `POST` | `/configuration/llm/models/refresh` | refresh catalog when active/persisted revision permits it |
| `PUT` | `/configuration/web-search` | update Web search provider configuration |

LLM saves do not mutate the running LLM owner's frozen active snapshot. A mismatched persisted revision is surfaced as
`restartRequired`; an already sealed Run selection never drifts because configuration changed.

### Workspace And Definition

| Method | Path | Semantics |
| --- | --- | --- |
| `GET` / `POST` | `/workspaces` | list or create Workspace |
| `GET` / `DELETE` | `/workspaces/:workspaceId` | read or delete Workspace; delete supports optional `purge` query/body |
| `GET` | `/workspaces/:workspaceId/snapshot` | public aggregate projection |
| `GET` | `/workspaces/:workspaceId/runtime-view` | Runtime-oriented workspace projection |
| `PUT` / `DELETE` | `/workspaces/:workspaceId/llm-selection` | update/clear canonical selection via owner CAS; reads come from Workspace projection |
| `GET` / `PUT` | `/workspaces/:workspaceId/soul-bindings` | query/update canonical Soul routing |
| `GET` / `POST` | `/workspaces/:workspaceId/goals` | list/create Goal |
| `GET` / `PUT` | `/workspaces/:workspaceId/goals/:goalId` | read/update Goal in workspace scope |
| `GET` / `POST` | `/workspaces/:workspaceId/dods` | list/create DoD |
| `GET` / `PUT` | `/workspaces/:workspaceId/dods/:dodId` | read/update DoD in workspace scope |
| `POST` | `/workspaces/:workspaceId/goal-dod-quality-check` | assess supplied or referenced Goal/DoD without mutating them |
| `POST` | `/workspaces/:workspaceId/execution-activations` | submit `resume` or `restart` intent |

Workspace creation fields are `title`, optional `description`, `deliverableDirectory` and `templateId`; identity is
owner-generated. The owner-neutral wire DTO remains usable by administrative clients, while normal TUI/Web setup requires a
selected template and an explicit workspace directory, followed by Goal and DoD. Callers are not required to author raw
context/evidence/target/recipe/check/artifact JSON.

### Run Read Model

| Method | Path | Projection |
| --- | --- | --- |
| `GET` | `/runs?workspaceId=...` | Run summaries, optionally workspace-filtered |
| `GET` | `/workspaces/:workspaceId/runs` | workspace-scoped Run summaries |
| `GET` | `/workspaces/:workspaceId/runs/:runId` | one Run projection |
| `GET` | `/workspaces/:workspaceId/runs/:runId/snapshot` | detailed Run snapshot |
| `GET` | `/workspaces/:workspaceId/runs/:runId/status` | status, optionally long-polled |
| `GET` | `/workspaces/:workspaceId/runs/:runId/observability` | observability projection |
| `GET` | `/workspaces/:workspaceId/runs/:runId/steps` | legacy/non-authoritative process projection |
| `GET` | `/workspaces/:workspaceId/runs/:runId/tool-calls` | tool-call evidence projection |
| `GET` | `/workspaces/:workspaceId/runs/:runId/runtime-checkpoints` | diagnostic checkpoint projection |
| `GET` | `/workspaces/:workspaceId/runs/:runId/results` | result projections |
| `GET` | `/workspaces/:workspaceId/runs/:runId/patches` | patch evidence projections |
| `GET` | `/workspaces/:workspaceId/runs/:runId/validation-results` | validation evidence projections |
| `GET` | `/workspaces/:workspaceId/runs/:runId/artifacts` | artifact projections |
| `GET` | `/workspaces/:workspaceId/runs/:runId/memory-recalls` | recall trace projections |
| `GET` | `/workspaces/:workspaceId/runs/:runId/task-progress` | derived progress projection |
| `GET` | `/workspaces/:workspaceId/runs/:runId/context-bundle` | bounded context projection |
| `GET` | `/workspaces/:workspaceId/runs/:runId/task-decompositions` | diagnostic task decomposition |

`Run` exposes identity/scope/lineage/definition revision/state/blocker/timestamps plus sealed LLM and activation status.
It does not expose `executionContext`. Step, checkpoint, recovery and `nextAction`-named display fields—where still present—
are diagnostics only and cannot drive production scheduling or rebuild authority.

### Governance

| Method | Path | Semantics |
| --- | --- | --- |
| `GET` | `/decisions` | query tickets; supports workspace/run/page filters |
| `GET` | `/decisions/:ticketId` | read one ticket |
| `POST` | `/decisions/:ticketId/resolve` | submit `resolutionType` + optional `resolutionPayload` |
| `POST` | `/decisions/resolve-batch` | submit `ticketIds`, shared resolution and optional payload |
| `GET` | `/approvals` | query approvals; supports workspace/run/page filters |
| `GET` | `/approvals/:approvalId` | read one approval |
| `POST` | `/approvals/:approvalId/approve` | approve with optional payload |
| `POST` | `/approvals/:approvalId/reject` | reject with optional payload |
| `POST` | `/approvals/resolve-batch` | submit `approvalIds`, `action=approve|reject` and optional payload |

Resolve endpoints only commit owner facts and refresh projection. Application does not emit Resume after Decision/Approval
resolution. Runtime governance resolution returns through the typed Orchestrator contract and Run-private continuation.

### Registry And Context

| Method | Path | Semantics |
| --- | --- | --- |
| `GET` / `POST` | `/souls` | list/create Soul |
| `GET` / `PUT` / `DELETE` | `/souls/:soulId` | read/update/delete Soul |
| `GET` / `POST` | `/workspace-templates` | list/create template |
| `PUT` / `DELETE` | `/workspace-templates/:templateId` | update/delete template |
| `GET` / `POST` | `/skills` | list/create Skill |
| `GET` | `/skills/:skillId` | read Skill |
| `GET` / `POST` | `/mcp/servers` | list/create MCP server definition |
| `GET` / `PUT` | `/mcp/servers/:serverId` | read/update MCP server definition |
| `GET` | `/mcp/servers/:serverId/tools` | discover tools |
| `GET` | `/mcp/servers/:serverId/resources` | discover resources |
| `GET` | `/mcp/servers/:serverId/prompts` | discover prompts |
| `GET` / `POST` | `/mcp/capabilities` | query/create MCP capability metadata |
| `GET` / `PUT` | `/mcp/capabilities/:capabilityId` | read/update capability metadata |
| `GET` | `/memory?workspaceId=...` | list/search workspace memory |
| `GET` / `PUT` / `DELETE` | `/memory/:memoryId` | unscoped unique-ID compatibility access |
| `POST` | `/workspaces/:workspaceId/memory` | create workspace memory |
| `GET` / `PUT` / `DELETE` | `/workspaces/:workspaceId/memory/:memoryId` | scoped memory access |
| `GET` | `/workspaces/:workspaceId/memory-recalls` | workspace recall traces |

Registry requests carry declarative definitions only. MCP Registry owns catalog/selection metadata; the MCP capability block
owns concrete Runtime execution. Capability create/update/read DTOs expose the Registry-owned
`validationProofApplicability` value (`direct | auxiliary | not_declared`); technical Runtime readiness remains an MCP execution
fact and is not writable through this API. Secrets and large content must not be copied into EventHub notifications or projections.

### Setup Draft, Events And Raw Content

| Method | Path | Semantics |
| --- | --- | --- |
| `POST` | `/workspace-setup-draft-sessions` | create an in-process draft session |
| `POST` | `/workspace-setup-draft-sessions/:draftSessionId/messages` | append message and revise draft |
| `POST` | `/workspace-setup-draft-sessions/:draftSessionId/regenerate` | regenerate from current session state |
| `GET` | `/events` | bounded persisted EventRecord query |
| `GET` | `/events/stream` | SSE live/replay stream |
| `GET` | `/runtime/payloads?ref=...` | open allowlisted Runtime raw ref |
| `GET` | `/artifacts/content?uri=...` | open allowlisted Artifact URI |
| `GET` | `/console`, `/console/`, `/console/**` | embedded Web console |

SSE supports `workspaceId`, `runId`, `view`, `lastEventId` and the `Last-Event-ID` header. It sends a connection comment,
15-second heartbeat and typed envelopes. `view=projection` filters to workspace projection changes. SSE and EventRecord
replay update displays; neither can repair owner state or reconstruct Runtime authority.

Raw payload and artifact readers validate their ref/URI roots before opening content. HTTP `Content-Disposition` and content
type are derived from the validated content record, not arbitrary caller paths.

## Query And Pagination

- paged Run child resources use `limit` and `cursorAfterId`;
- Decision/Approval queries accept `workspaceId`, `runId`, `limit` and cursor fields supported by their page mapper;
- Run status accepts `waitState` and positive `waitTimeoutMillis` for bounded wait;
- Event queries accept workspace/run/topic/subject/time filters, `limit` and `cursorAfterId`;
- Memory queries accept workspace identity plus supported text/tag/type/limit filters;
- MCP capability queries accept server/kind/category/stage/risk/enabled/text filters.

Unknown JSON fields are rejected. Path scope wins over body scope; a body cannot redirect a mutation to another workspace,
Run or resource.

## Response And Error Contract

JSON responses use explicit wrapper families such as `workspace`, `run`, `snapshot`, `status`, `decisions`, `approvals`,
`memory`, `eventRecords` or catalog resources, together with service/backend metadata where applicable. Mutable slices/maps
are detached before crossing the owner/application boundary.

| Condition | Status |
| --- | --- |
| successful query/update | `200` |
| resource created | `201` |
| execution command accepted | `202` |
| deletion without response body | `204` |
| invalid body/query/scope | `400` |
| missing resource | `404` |
| blocked, stale revision or transition conflict | `409` |
| canceled while waiting for execution admission | `408` |
| unexpected application/dependency error | `500` |

The baseline error body is `{ "error": "..." }`. Unsupported Runtime Decision action conflicts additionally expose a
stable `code`, ticket identity, requested/allowed actions and remediation. Credentials, raw provider bodies, prompts and
unsafe diagnostics must not be returned.

## Verification

- Route registration and supported/removed route guards live in
  `internal/modules/application/workorchd/service/server/http_*.go` and `http_test.go`.
- DTO and handler tests live beside the server implementation.
- Deployed behavior is covered by [Service Acceptance](../60-verification/service-acceptance.md).
- Owner and Runtime semantics are covered by
  [Runtime Owner Convergence Matrix](../60-verification/runtime-owner-convergence-matrix.md).

When a route changes, update route tests, this file, the Web/TUI client contract and service acceptance in the same change.
