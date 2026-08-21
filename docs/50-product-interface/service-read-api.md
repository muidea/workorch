# WorkOrch Service Read API

Status: normative

Type: product-interface

Last Verified: 2026-08-21

## Purpose

本文只定义 workorchd 的 GET、SSE、pagination、raw-content 和 read-model surface。所有返回均为 owner fact 的
detached projection；读取、等待、缓存或 replay 不能修改 owner state、触发 Resume 或重建 Runtime authority。

## Operational And Configuration Reads

| Method | Path | Projection |
| --- | --- | --- |
| `GET` | `/healthz`, `/readyz`, `/metrics`, `/debug/pprof/**` | process health/readiness/metrics/debug |
| `GET` | `/configuration` | redacted persisted configuration and active-revision status |
| `GET` | `/configuration/llm/models` | active LLM-owner catalog summary |
| `GET` | `/console`, `/console/`, `/console/**` | embedded Web console assets |

## Workspace And Run Reads

| Method | Path | Projection |
| --- | --- | --- |
| `GET` | `/workspaces`; `/workspaces/:workspaceId` | Workspace list/detail |
| `GET` | `/workspaces/:workspaceId/snapshot`; `/runtime-view` | public and Runtime-oriented Workspace projection |
| `GET` | `/workspaces/:workspaceId/soul-bindings` | canonical routing projection |
| `GET` | `/workspaces/:workspaceId/goals`; `/workspaces/:workspaceId/goals/:goalId` | scoped Goal resources |
| `GET` | `/workspaces/:workspaceId/dods`; `/workspaces/:workspaceId/dods/:dodId` | scoped DoD resources |
| `GET` | `/runs?workspaceId=...`; `/workspaces/:workspaceId/runs` | Run summaries |
| `GET` | `/workspaces/:workspaceId/runs/:runId` | Run detail |
| `GET` | `/workspaces/:workspaceId/runs/:runId/{snapshot,status,observability}` | detailed/status/observability views |
| `GET` | `/workspaces/:workspaceId/runs/:runId/{steps,runtime-checkpoints,task-decompositions}` | legacy diagnostic projections only |
| `GET` | `/workspaces/:workspaceId/runs/:runId/{tool-calls,patches,validation-results,artifacts,results}` | evidence/outcome projections |
| `GET` | `/workspaces/:workspaceId/runs/:runId/{memory-recalls,task-progress,context-bundle}` | recall/progress/context projections |

`Run` exposes scope、lineage、Definition revision、lifecycle、blocker、timestamps and sealed activation/LLM status。
It never exposes `ExecutionContext`。Step/checkpoint/nextAction-named compatibility fields are display-only and cannot
drive scheduling or recovery。

`snapshot` / `runtime-view` / `status`必须能够只读投影：

- current canonical phase与governance/recoverable-wait/terminal disposition；
- recoverable wait identity、safe reason、recovery condition、next eligible drive/deadline和guard/budget摘要；
- coverage=`uncovered|partially_covered|covered`、assessment=`unassessed|satisfied|unsatisfied|indeterminate`、
  evidence=`candidate|accepted|rejected|stale_or_superseded`，按requirement/CWI逐项绑定；
- execution-budget各dimension的limit、confirmed、unknown/reserved、remaining和exhausted；
- Runtime state revision、Definition revision和实际依赖的owner revision set/hash。

这些字段均从owner/aggregate facts派生；API不提供clear wait、补预算、改coverage/assessment/evidence或写owner revision
set的通用mutation。三轴内容必须一一对应
[`RuntimeCoverageView`](../10-domain-model/result-closeout.md#projection-contract)，terminal Result必须一一对应
`RuntimeResultView/RuntimeResultCloseout`；budget必须一一对应
[`RuntimeExecutionBudgetView`](../00-core/runtime/execution-budget.md#projection)。不得提供删减axis、revision、reason或lineage的
第二套summary DTO作为正式结果。

## Governance And Resource Reads

| Resource | Read routes |
| --- | --- |
| Decision | `GET /decisions`; `GET /decisions/:ticketId` |
| Approval | `GET /approvals`; `GET /approvals/:approvalId` |
| Soul | `GET /souls`; `GET /souls/:soulId` |
| Template | `GET /workspace-templates` |
| Skill | `GET /skills`; `GET /skills/:skillId` |
| MCP server | `GET /mcp/servers`; detail/tools/resources/prompts below one server |
| MCP capability | `GET /mcp/capabilities`; `GET /mcp/capabilities/:capabilityId` |
| Memory | `GET /memory?workspaceId=...`; `GET /memory/:memoryId`; scoped `GET /workspaces/:workspaceId/memory/:memoryId`; Workspace recall query |

Unscoped unique-ID Memory detail remains compatibility read access only；scope collision must fail closed。Secrets、large
content and owner-private fields are omitted or exposed only through validated refs。

Decision projection必须区分ordinary governance、pre-seal Definition clarification与Run-scoped runtime-definition
clarification，并安全展示scope、typed violation/gap、required action和binding state。runtime-definition clarification的
`RequiredActionKind`固定为`revise_definition_and_restart`；read DTO不得把它投影为`continue`、通用answer或Resume可用状态，
也不得暴露raw provider diagnostic、replacement hash内部字节或owner-private authority。

## Event, SSE And Raw Content

| Method | Path | Semantics |
| --- | --- | --- |
| `GET` | `/events` | bounded persisted EventRecord query |
| `GET` | `/events/stream` | filtered SSE live/replay |
| `GET` | `/runtime/payloads?ref=...` | allowlisted Runtime raw ref |
| `GET` | `/artifacts/content?uri=...` | allowlisted Artifact content |

SSE accepts workspace/run/view/lastEventId and `Last-Event-ID`, emits heartbeat and typed envelopes。Projection view只显示
derived changes；EventRecord/SSE replay从不修复 owner state。Raw reader validates root/scope/ref/URI before open；
content type/disposition来自 validated record，而不是 caller path。

## Query, Pagination And Wait

- paged Run children use `limit + cursorAfterId`；
- Decision/Approval use workspace/run filters and owner-supported cursor；
- Run status supports bounded positive `waitState + waitTimeoutMillis`；这是HTTP long-poll条件，不是
  `recoverable_runtime_wait` authority或其恢复trigger；
- Event query supports workspace/run/topic/subject/time/limit/cursor；
- Memory supports scoped text/tag/type/limit filters；MCP capability supports server/kind/category/stage/risk/enabled/text；
- cursor order is stable and pages neither duplicate nor leak cross-workspace resources。

## Response And Error Boundary

Response wrappers are resource-specific (`workspace`, `run`, `snapshot`, `decisions`, `memory`, `eventRecords`, catalog
resources) and detach mutable slices/maps before crossing owner/application boundary。Unknown query/body fields、invalid scope
or unsafe raw ref=`400`；missing=`404`；bounded wait cancellation=`408`；owner conflict/stale=`409`；unexpected dependency=`500`。

Read handlers may cache/rebuild projection and coalesce waiters, but cache invalidation only consumes typed owner notification。
Projection failure cannot rewrite sealed terminal or active owner state。

## Related Contracts

- [Service Command API](service-api.md)
- [Storage And Read Model](../30-module-architecture/storage-readmodel.md)
- [Service Acceptance](service-acceptance.md)
