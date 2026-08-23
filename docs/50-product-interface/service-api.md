# WorkOrch Service Command API

Status: normative

Type: product-interface

Last Verified: 2026-08-23

## Purpose And Authority

本文件是 HTTP/SSE surface 的稳定入口，并只展开 mutation/command contract。GET、SSE、pagination、raw-content 与
read-model 规则见 [Service Read API](service-read-api.md)。实际 route registration、DTO 和 handler tests 是协议实现事实源；
owner 状态机仍由 owner component contract 定义。

所有业务路径以 `/api/v1/workorch` 为前缀。HTTP payload 必须先转换为接收 owner `pkg/events` 的 concrete typed
command/result；handler 不得把 JSON DTO、display text 或 projection 当作 Runtime authority。

## Workspace Execution Mutation

唯一 workspace execution 写入口是：

```http
POST /api/v1/workorch/workspaces/:workspaceId/execution-activations
```

```json
{
  "intent": "resume",
  "requestId": "optional-request-id",
  "reason": "optional-reason",
  "source": "optional-source"
}
```

- `intent` 空值规范化为 `resume`，只允许 `resume|restart`；
- execution admission 不接受 body/query/header priority；Run 顺序与 owner activation identity 不由 caller priority 控制；
- caller 不能注入 RunID、Goal/DoD、provider/Soul/Skill/MCP/Memory selection、Runtime context、target state 或 successor identity；
- fresh Start/Restart 必须绑定 exact `WorkspaceActivationAdmission=eligible`；pending/missing/mismatch 返回
  `definition_clarification_pending`，零 Run/replacement intent；
- Resume 只复用 existing Run 的 original sealed startup input，不吸收 newer candidate/revision；
- Restart 使用 fresh eligible bootstrap；Run owner先 seal predecessor `ReplacementIntent`，terminal delivery 完成后才创建 successor；
- handler 同步等待并校验 exact Workspace owner result。只有 `reconciliation_requested|replacement_pending` 返回
  `202`；`restart_required|fresh_activation_required|definition_changed_requires_restart|governance_required|data_reset_required`
  返回 typed `409`。两类响应都原样携带 `workspaceId/runId/requestId/intent/outcome/reason`，不存在
  application-local queue/queued acknowledgement；`requestReason` 与 owner-authored `reason` 分离。

响应中的 `accepted`、`state=accepted|rejected`、HTTP status 和 display diagnostic 只由共享 pure admission classifier
从已校验 owner outcome 生成。HTTP、projection、client、Web/TUI 不得各自重判 outcome；projection 还保留 exact
`outcome/admissionReason`，SSE 只发布 immutable projection invalidation。

以下 mutation 永久不属于当前 API：run-addressed create/start/resume、`execution-restarts`、
`ExecutionContextPatch`、caller-selected target state 或 successor。

## Mutation Inventory

### Configuration

| Method | Path | Command semantics |
| --- | --- | --- |
| `PUT` | `/configuration/server` | update supported persisted server fields |
| `PUT` | `/configuration/runtime` | update Runtime configuration source |
| `PUT` | `/configuration/llm` | update persisted ai-proxy selection/key/budget for next active revision |
| `POST` | `/configuration/llm/models/refresh` | request LLM-owner catalog refresh when revision permits |
| `PUT` | `/configuration/web-search` | update Web-search provider configuration |

LLM configuration save does not mutate the running LLM owner snapshot or any sealed Run selection；read side exposes
`restartRequired` when persisted and active revisions differ。

### Workspace And Definition

| Method | Path | Command semantics |
| --- | --- | --- |
| `POST` | `/workspaces` | create owner-identified Workspace |
| `DELETE` | `/workspaces/:workspaceId` | delete; optional typed purge request |
| `PUT` / `DELETE` | `/workspaces/:workspaceId/llm-selection` | owner-CAS update/clear selection |
| `PUT` | `/workspaces/:workspaceId/soul-bindings` | update owner-managed Soul routing |
| `POST` | `/workspaces/:workspaceId/goals` | submit Goal authoring change through admission candidate |
| `PUT` | `/workspaces/:workspaceId/goals/:goalId` | submit scoped Goal revision candidate |
| `POST` | `/workspaces/:workspaceId/dods` | submit DoD authoring change through admission candidate |
| `PUT` | `/workspaces/:workspaceId/dods/:dodId` | submit scoped DoD revision candidate |
| `POST` | `/workspaces/:workspaceId/goal-dod-quality-check` | bounded non-mutating assessment of supplied/referenced text |
| `POST` | `/workspaces/:workspaceId/execution-activations` | submit `resume|restart` intent |

Workspace creation accepts title、optional description、optional directory and template ID；owner generates identity。
Blank directory resolves to `.workorch-workspaces/<workspace-id>` before candidate promotion。普通 setup 不要求 raw
context/evidence/target/recipe/check/metric/artifact JSON。

Goal/DoD mutation先提交 immutable `DefinitionAdmissionCandidate` 并执行 pure admission。只有 `admitted_all_of`
可以 promotion；`definition_one_of_unsupported` 或 declared ambiguity 返回 typed `BlockingTicketID`。Corrected
candidate 仍需 Decision resolution 与 Workspace acknowledgement 才能变为 eligible。

必须区分两类 Definition clarification：

- pre-seal ticket由新candidate的all-of admission、专用Decision resolution和Workspace acknowledgement收口；不存在
  “接受one-of后继续”的payload；
- Run-scoped runtime-definition ticket不提供公共continue/fail resolution。用户只提交修正后的candidate与显式`restart` intent；
  Run owner在持久化matching `ReplacementIntent`后，通过
  `ResolveRuntimeDefinitionClarificationByReplacementCommand/Result`与Decision owner收口。HTTP handler、Web/TUI和
  workorchd均不得代发该owner-to-owner command或伪造replacement binding。

不存在独立的`/runtime-definition-clarifications/:id/continue`、直接ticket-close或Definition patch route。普通Goal/DoD写入、
promotion和execution activation仍分别经过各自owner合同，任何一步失败都保持零新Run/provider/dispatch/effect。

### Governance

| Method | Path | Command semantics |
| --- | --- | --- |
| `POST` | `/decisions/:ticketId/resolve` | submit typed resolution for one Decision ticket |
| `POST` | `/decisions/resolve-batch` | resolve exact ticket IDs with one typed resolution |
| `POST` | `/approvals/:approvalId/approve` | submit approval fact |
| `POST` | `/approvals/:approvalId/reject` | submit rejection fact |
| `POST` | `/approvals/resolve-batch` | approve/reject exact approval IDs |

Resolution只提交对应 owner fact并刷新 projection。Application 不发送 Resume；ordinary Runtime governance 经
Orchestrator typed resolution收口。通用Decision resolve route必须拒绝pre-seal与runtime两类Definition clarification ticket；
前者只由corrected admission专用合同收口，后者只由Run owner发起的dedicated replacement-resolution contract收口。

### Resource Registries

| Resource | Mutation routes |
| --- | --- |
| Soul | `POST /souls`; `PUT, DELETE /souls/:soulId` |
| Workspace Template | `POST /workspace-templates`; `PUT, DELETE /workspace-templates/:templateId` |
| Skill | `POST /skills` |
| MCP server | `POST /mcp/servers`; `PUT /mcp/servers/:serverId` |
| MCP capability | `POST /mcp/capabilities`; `PUT /mcp/capabilities/:capabilityId` |
| Memory | `POST /workspaces/:workspaceId/memory`; `PUT, DELETE /workspaces/:workspaceId/memory/:memoryId`; compatibility `PUT, DELETE /memory/:memoryId` |

Registry DTO 只表达 declarative owner resource。MCP Registry owns metadata；MCP capability Block owns execution。
`validationProofApplicability=direct|auxiliary|not_declared` 是 Registry declaration，technical readiness 不是 API 可写字段。
Server DTO只接受`environmentRefs`而不接受environment value；supported ref由MCP execution最后解析，plaintext credential在
persistence/EventHub前拒绝。Capability以`enabled`作为唯一availability字段；owner-issued
`source/sourceRef/revision`只读provenance不能作为mutation authority。

### Optional Setup Draft

| Method | Path | Semantics |
| --- | --- | --- |
| `POST` | `/workspace-setup-draft-sessions` | create process-local optional authoring session |
| `POST` | `/workspace-setup-draft-sessions/:draftSessionId/messages` | append message and generate bounded draft |
| `POST` | `/workspace-setup-draft-sessions/:draftSessionId/regenerate` | regenerate current draft |

Draft endpoint不写 Workspace owner state、不直接 promotion/start，也不把 `one_of` 提供为合法选项。采用 draft 必须重新
提交普通 admission candidate。

## Command Scope And Responses

- path scope wins over body scope；unknown JSON fields or cross-workspace redirect are rejected；
- owner-generated identity/revision/hash 必须从 concrete result 返回，handler 不自行生成替代 authority；
- created=`201`、accepted execution=`202`、successful update=`200`、empty delete=`204`；
- invalid=`400`、not found=`404`、blocked/stale/conflict=`409`、admission wait canceled=`408`、unexpected dependency=`500`；
- credentials、raw prompt/provider body、unsafe diagnostic 和 internal aggregate 不进入 response。

## Verification

- route/DTO/removed-route guards：`internal/modules/application/workorchd/service/server/http_*.go` and tests；
- deployed transport/owner integration：[Service Acceptance](service-acceptance.md)；
- Runtime/owner invariants：[Core Runtime Verification](../00-core/runtime/verification.md)。

Route change必须在同一变更中更新 route tests、command/read contract、Web/TUI client contract 与 service acceptance。
