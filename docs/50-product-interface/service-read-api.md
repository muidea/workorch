# WorkOrch Service Read API

Status: normative

Type: product-interface

Last Verified: 2026-09-05

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
| `GET` | `/workspaces/:workspaceId/runs/:runId/{steps,runtime-checkpoints,task-decompositions}` | bounded diagnostic projections only |
| `GET` | `/workspaces/:workspaceId/runs/:runId/{tool-calls,patches,validation-results,artifacts,results}` | evidence/outcome projections |
| `GET` | `/workspaces/:workspaceId/runs/:runId/{memory-recalls,task-progress,context-bundle}` | recall/progress/context projections |

`Run` exposes scope、lineage、Definition revision、lifecycle、blocker、timestamps and sealed activation/LLM status。
It never exposes `ExecutionContext`。Step/checkpoint are diagnostic only；Runtime read DTOs do not expose a command-like
`nextAction` authority。

Workspace list/detail 的 `state` 与 `activeRunId` 是 Workspace owner canonical fact；Workspace JSON 不含
`blockingTicketId|blockingApprovalId`。Run 的 `state` 和 blocker refs 只来自 Run owner，TaskProgress、Decision/Approval 或
terminal presentation 都不得覆盖它们。`snapshot`、Run snapshot/observability 与 `runtime-view` 只使用
`displayOutcome`（没有并行 `outcome` 字段）表示非权威展示摘要，并保留独立 `result/results` 作为 formal Result projection。
`governanceDisplay` 只关联 open Decision/Approval 与 canonical Run scope；`consistencyDiagnostic` 只报告 owner/projection
handoff 偏差。三者均不能生成、阻断或选择 Resume、Restart、Drive、resolution target 或任何 mutation。
`runtime-view.blockingDecision|blockingApproval` 只在 canonical active Run 的 matching blocked state 与 exact blocker ref
命中同 scope 的真实 open/pending owner resource 时返回；同 Run 的另一个未决资源与 `governanceDisplay` ID 均不能替代该 ref。
TaskProgress、ContextBundle 与 compressed context 共享同一个 `RecoveryPlan.blockingResource` 事实引用，逐字段包含 owner、
resource、workspace/run scope、state 及 owner 原样提供的 `requiredAction/allowedUserActions`。`suggestedNavigation` 只供展示；
上述字段与兼容 blocking ID 不得进入 mutation constructor。没有 exact canonical 交集时只能输出 diagnostic，不能给出 action。

Acceptance 的 `budget` 只包含 SM-2 convergence/repair 与 prerequisite refresh；SM-4 provider transport、operation recovery、
CAS reload 和 terminal delivery 计数只在独立 `recoveryBudget`。`RuntimeAuditRepair` 不得包含
`terminalDeliveryAttempts`；该计数仅在 sealed terminal audit 中出现。nonterminal aggregate 携带 terminal delivery 计数属于
当前 owner shape 违规，owner load 必须失败，不由 projection 清零、隐藏或转换成兼容状态。

`RuntimeAudit.coverageBindings` 对 managed validation 必须只读保留 exact proof-requirement hash、Definition check refs 与
owner-opaque `validationCheckAttestations`；matching evidence/receipt 必须保留 `observationProtocolRef` 和 owner-native check
outcome。审计消费者通过该映射核对 Definition check 与 native outcome，不能直接把两种检查标识作字符串比较，也不能把该
投影回放为 receipt、operation 或 coverage authority。

Workspace `snapshot.runCommand` 只保存最新 immutable Workspace-owner admission observation：request/workspace/run/intent
identity、separate caller `requestReason`、`source`、exact `outcome`、`admissionReason`、derived `diagnostic` 与
`accepted|rejected`。accepted 只携带 `acceptedAt`，rejected 只携带 `rejectedAt`；字段集合固定为
`workspaceId/runId/state/intent/requestId/requestReason/source/outcome/admissionReason/diagnostic/acceptedAt|rejectedAt/revision`。
不暴露重复 `disposition`、歧义 `reason`、`error`、`definitionRevision` 或 `decisionId`。同 request identity 的
duplicate/stale observation 不改写投影或 revision。transport/context failure 不伪造 owner rejection，read model 不接收
caller priority，也不从 HTTP status 或 caller reason 反推 owner outcome/state。`revision` 由 projection Store 与该
runCommand observation 在同一事务中分配，transport/caller 不提供该值。

`llmUsageTraces` 原样暴露 optional `logicalRequestId`/`physicalCallId`，并以
`physicalResponseMode=streaming|non_streaming` 和 `providerSubmitted` 作为物理调用审计主事实。Runtime usage 的
两级身份必须同时存在，且 `physicalCallId=requestId`；generic、非 Runtime usage 可以两者都没有。只出现一级、空白、
correlation 不一致、缺少 physical fact 或非法 mode 的记录不得进入 projection。公共 DTO 不再包含 `streaming` 布尔字段；
Web/TUI 也不得从 provider、response bytes、usage 或 request bytes 推断身份、提交或响应模式。

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
| Memory | `GET /memory?workspaceId=...`; `GET /workspaces/:workspaceId/memory/:memoryId`; Workspace recall query |

Memory item detail/update/delete always carries the Workspace scope in the path；there is no unscoped item route or query-parameter
scope adapter。Secrets、large content and owner-private fields are omitted or exposed only through validated refs。

Decision projection必须区分ordinary governance、pre-seal Definition clarification与Run-scoped runtime-definition
clarification。`Decision.definitionClarification`是从Decision owner binding无损复制的bounded typed DTO：它包含
`class/bindingState/requiredAction/violationOrGapCode/offendingRefs`，并保留candidate、canonical signature、runtime
`definitionRevision/authoritySnapshotId/clarificationDispatchId/focusRef`及resolution/replacement lineage identity。
所有slice（特别是`offendingRefs`）必须deep-clone。runtime-definition clarification的`requiredAction`固定为
`revise_definition_and_restart`；read DTO不得把它投影为`continue`、通用answer、Resume可用状态或admission eligibility，
也不得暴露raw provider diagnostic、raw LLM text或owner-private authority aggregate。binding中的bounded ID/hash仅供审计，
不能决定Resume/Restart。

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
Projection failure cannot rewrite sealed terminal or active owner state。Run state invalidation只接受`source=blocks/run`且
通过`RunStateChangedData.Validate`的通知；Eventstream replay或source-forged/invalid通知不更新该cache。

## Related Contracts

- [Service Command API](service-api.md)
- [Storage And Read Model](../30-module-architecture/storage-readmodel.md)
- [Service Acceptance](service-acceptance.md)
