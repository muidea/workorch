# Web Console 与后端 API 参数对齐 评审报告

> 核对日期：2026-06-02
> 核对范围：`web/workorch-console/src/**` 全部前端 HTTP 调用 与 `internal/modules/application/workorchd/service/server/**` 全部后端 handler
> 核对方法：逐条对照 `router.AddHandler` 注册的路径、方法、Query、Body 字段（基于 `http_query_helpers.go` / `http_types.go` / `appservice/*`）与 `api.ts` / `pages/*` / `helpers.tsx` / `hooks/useSSE.ts` 的真实调用
> 前置参考：`docs/50-product-interface/web-console/overview.md`、`docs/60-verification/audits/design-implementation-audit-2026-06-01.md`

---

## 一、总体结论

- **核心读写路径（workspaces / runs / goals / dods / souls / 治理流）参数对齐良好**。
- 主要差距集中在三块：
  1. **MCP 能力管理子模块**：后端已经支持 `schema` / `uri` 字段与 `POST /mcp/capabilities`，但前端既缺"新建能力"入口，也缺字段编辑。
  2. **WebSearch headers**：后端 `UpdateWebSearchProviderConfiguration.headers` 已被支持，前端类型声明了但 Form / payload 都没有体现，配置无法落地。
  3. **SSE 过滤与续跑 patch**：`/events/stream` 过滤参数没透传；`resumeRun` 不支持 `executionContextPatch`。
- 其余 6 条为低/中风险风格建议，不影响线上功能。

---

## 二、关键问题（会导致功能缺失或请求被拒）

### 2.1 `createMCPCapability` 完全没有 UI 入口

- **后端**：`POST /api/v1/workorch/mcp/capabilities`（`http_mcp.go:190`）支持新建能力，必填 `id / serverId / kind / name / displayName / enabled`。
- **前端**：`api.ts:461` 定义了 `createMCPCapability`，但 `web/` 下**无任何调用点**（`grep` 只命中定义处）。`RuntimeControlPage` 只能编辑既有能力，没有"新建能力"流程。
- **影响**：能力目录里只能看不能建，新接入 MCP 后需要后端脚本或手工注入。
- **修复建议**：在 `RuntimeControlPage` 增加"新建能力" Modal，复用 `MCPCapabilityFormValues` 并补充 `serverId / kind / name` 三个 Form.Item。
- **收口状态（2026-06-22）**：已修复。`RuntimeControlPage` 能力目录新增“新建能力”入口，复用同一个能力表单，支持填写 `id/serverId/kind/name/displayName/enabled` 后调用 `createMCPCapability`。

### 2.2 MCP 能力编辑 Modal 漏字段 `schema` 与 `uri`

- **后端**：`MCPCapability`（`http_mcp.go:190`）含 `schema map[string]any` 与 `uri string` 两个字段。
- **前端**：`RuntimeControlPage.tsx:267-297` 的 `saveCapability` 通过 `...editingCapability` 保留旧值，但 Form 里**没有** `schema` / `uri` 的 Form.Item（`RuntimeControlPage.tsx:640-702`）。
- **影响**：编辑时只能"维持"旧值，无法修改或新建带 schema/uri 的能力。
- **修复建议**：补充 `Form.Item name="schema"`（JSON 编辑器）与 `name="uri"`，并把 `id / serverId / kind / name` 改为受控展示（`disabled` 锁死）。
- **收口状态（2026-06-22）**：已修复。能力表单新增 `uri` 与 JSON object `schema` 编辑；编辑模式下 `id/serverId/kind/name` 锁定，新建模式下作为必填字段开放。

### 2.3 WebSearch `headers` 字段被静默丢弃

- **后端**：`UpdateWebSearchProviderConfiguration` 含 `headers map[string]string`（`http_configuration.go:61`）。
- **前端**：类型 `UpdateWebSearchProviderConfiguration.headers?: Record<string,string>` 已声明（`types.ts:217`），但 `ConfigurationPage.tsx:156-187` 的 `saveWebSearchConfiguration` **没有**把 headers 放进 payload，Form 也没有对应输入（`ConfigurationPage.tsx:509-541`）。
- **影响**：后端支持的"自定义请求头"能力完全无法配置；保存一次会清掉已有 headers（如果 omitempty 路径不严格保留）。
- **修复建议**：在表单增加 `headers` 键值对编辑器，并在 `saveWebSearchConfiguration` 的 providers 映射里补 `headers: parsedHeaders ?? {}`。
- **收口状态（2026-06-22）**：已修复。`ConfigurationPage` 的 Web Search Provider 表单新增 headers 多行编辑器，支持 `Header: value` / `Header=value` 格式校验；`helpers.tsx` 的 web search form value 映射保留已保存 headers；保存 payload 已透传 `headers` 到 `PUT /configuration/web-search`。

### 2.4 `createRun` 类型与后端约束不一致

- **后端**：`http_run.go:392` 强制 `goalId` 与 `doDId` 非空。
- **前端**：`api.ts:337` 声明 `goalId?: string; doDId?: string`（可选），类型误导调用方。
- **影响**：当前无调用点，暂无线上故障；但任何后续按类型签名编写的代码都会踩 400。
- **修复建议**：把 `createRun` 的签名改为 `(workspaceId, runId, goalId, doDId)` 四个必填，或在 `WorkspaceExecutionActivationRequest` 风格的请求类型里把这两个标为必填。
- **收口状态（2026-06-22）**：已修复。Web `workorchApi.createRun` 签名已收紧为 `goalId` / `doDId` 必填，避免调用方按可选类型提交后端必然拒绝的请求。

---

## 三、中等问题（潜在功能偏差）

### 3.1 SSE 订阅不传 `workspaceId` / `runId` 过滤

- **后端**：`/api/v1/workorch/events/stream` 接受 `?workspaceId=&runId=&lastEventId=`（`http_events.go:31`）。
- **前端**：`api.ts:7-16` 的 `createWorkorchEventStream` 只把 `lastEventId` 传出（`useSSE.ts:41`）。
- **影响**：浏览器拉取的是全量事件流，多工作空间/多浏览器时事件压力被放大；与后端已经设计好的过滤能力不匹配。
- **修复建议**：在 `useSSE` 接受 `workspaceId?` / `runId?` 参数，透传到 EventSource URL。
- **收口状态（2026-06-22）**：已修复。`useSSE` 已支持 `workspaceId` / `runId` 选项并透传到 `createWorkorchEventStream`；scope 变化时会重置 `lastEventId`。`WorkspaceDetailPage` 现在按当前 workspace 订阅 projection stream，保留 workspace 级刷新事件；全局看板和治理队列仍使用全局 projection stream。

### 3.2 续跑 `resumeRun` 不支持 `executionContextPatch`

- **后端**：`POST /api/v1/workorch/runs/:runId/resume` 与 `workspaces/:workspaceId/runs/:runId/resume` 均解析 `{ executionContextPatch }`（`http_run.go:476-491`、`http_run.go:691-706`）。
- **前端**：`api.ts:348` 的 `resumeRun` 只透传 `priority` query，没有 body 参数；`WorkspaceBoardPage.tsx:137` 与 `WorkspaceDetailPage.tsx:1270` 调用都没有 patch。
- **影响**：当前业务无场景，但运行续跑时无法向 LLM 注入"用户补充信息"。文档/契约上应补上。
- **修复建议**：把 `resumeRun(runId, priority?, workspaceId?, executionContextPatch?)` 拼到 body 里。
- **收口状态（2026-06-22）**：已修复。Web `workorchApi.resumeRun` 已支持可选 `executionContextPatch` 并写入 JSON body；`startRun` / `resumeRun` 均保留 `intent/requestId/reason/source` 追踪字段，且 `startRun` 的 body intent 已修正为 `start`。

### 3.3 `listRuns` 没有 `workspaceId` 过滤

- **后端**：`GET /api/v1/workorch/runs` 支持 `?workspaceId=`（`http_run.go:15`）。
- **前端**：`api.ts:236` 的 `listRuns` 不接受参数，只能全量。
- **影响**：工作空间内的 run 列表走 `readWorkspaceSnapshot`（`api.ts:139`），不会触发 `listRuns`；所以 UI 无可见 bug，但前端有未使用的"全量拉取"调用，代码意图不清晰。
- **修复建议**：要么删除未使用的 `listRuns`，要么补上 `workspaceId` 参数透传。
- **收口状态（2026-06-22）**：已修复。Web `workorchApi.listRuns` 已支持可选 `workspaceId` 参数，并通过 `?workspaceId=` 透传到后端。

### 3.4 `purge` 在 `deleteWorkspace` 上的 query/body 双轨不一致

- **后端**：`http_workspace.go:282-303` 同时支持 `?purge=true` query 与 body 的 `purge` 字段，二者均生效。
- **前端**：`api.ts:194-198` 只用 query 透传 `purge`；`workspaceId` 拼到 URL，body 为空。
- **影响**：当前 OK，但当后端需要从 body 取值时前端不会传。建议在 `api.ts` 注释中明确"只走 query"以避免未来混淆。
- **收口状态（2026-06-22）**：已修复。`deleteWorkspace` 已补充代码注释，明确 Web 端采用 query contract，后端 body 路径保留给非浏览器客户端。

### 3.5 `MCPServer` 表单 `env` 三态编辑易踩坑

- `RuntimeControlPage.tsx:40-62` `mcpServerPayload`：当 `envMode === 'keep'` 时**不发** `env` 字段；后端 `MCPServer.env` 在 `MCPServer` 类型上是 `map[string]string`（`http_mcp.go:146`），PUT 请求会保留原值（`updateMCPServer` 走 `appservice.MCPServer`）。逻辑上 OK，但依赖后端 "不传则保留"的实现细节，不健壮。
- **修复建议**：在 `api.ts` 注释里写明，或者改成始终发送 `env`（用户清空用 `{}`）。
- **收口状态（2026-06-22）**：已修复。`RuntimeControlPage` 已在 `mcpServerPayload` 标注三态语义：`clear` 发送 `{}`、`set` 发送解析后的 env、`keep` 有意省略 `env` 以保持后端现值。

### 3.6 `MCPServer.enabled` 表单缺省为 `true`

- `RuntimeControlPage.tsx:48` `enabled: values.enabled !== false` —— `values.enabled` 为 `undefined` 时（理论上不会发生）会变成 `true`。
- **影响**：低，但 `undefined` 应判空而非默认 true，否则后端 `enabled bool`（非 omitempty）收到 `true` 也会被静默接受，掩盖前端 bug。
- **修复建议**：用 `values.enabled === true` 显式判别。
- **收口状态（2026-06-22）**：已修复。`mcpServerPayload` 已改为 `values.enabled === true`，MCP Server 启用字段表单已加 required 校验。

---

## 四、低风险 / 风格建议

### 4.1 `MCP` / `Soul` / `Template` 编辑器对只读字段缺乏强约束

- `RuntimeControlPage.tsx:267` `saveCapability` 用 `...editingCapability` 兜底再覆盖，掩盖"该字段是技术标识"的事实，编辑器可读性差。
- `helpers.tsx:2191` `saveSoul` 同理，技术标识 `id` 应在 Form 中可见且不可改。
- **修复建议**：加显式 `Form.Item name="..."` 锁死为 disabled 展示。
- **收口状态（2026-06-22）**：已修复。MCP 能力编辑、新建表单已显式展示并在编辑时锁定 `id/serverId/kind/name`；Soul 与 Workspace Template 表单已有 `id` 字段且编辑时 disabled；MCP Server 配置在选中已有服务时锁定服务 ID，点击“新建草稿”会清除选中态后再开放 ID 输入。

### 4.2 `appendWorkspaceSetupDraftMessage` 的 `content` 必填与前端逻辑

- `api.ts:180` 把 `content: string` 标为必填；调用点 `WorkspaceDetailPage.tsx:968-970` 在 `content` 非空时走 messages、空时走 regenerate，**无**误用风险。
- **修复建议**：在 `api.ts` 给 `regenerateWorkspaceSetupDraft` 的类型也明示 `content` 不应出现，避免后续重构时把 content 错塞进去。
- **收口状态（2026-06-22）**：已修复。Web `api.ts` 已拆分 `WorkspaceSetupDraftMessageRequest` 与 `WorkspaceSetupDraftGenerationRequest`，`appendWorkspaceSetupDraftMessage` 必须携带 `content`，`regenerateWorkspaceSetupDraft` 的请求类型不再允许携带 `content`。

### 4.3 `goals/:goalId` `PUT` 后端使用 `defineGoalRequest`（text 必填）

- 前端 `updateGoal` 总是传 `values.text`（`WorkspaceDetailPage.tsx:2630`），正常。但 form 上没有 `required` 校验，提交时若空字符串会被后端接受（`text string` 无 omitempty 但允许空串）。
- **修复建议**：在 Form 上加 `rules={[{ required: true, min: 1 }]}`。
- **收口状态（2026-06-22）**：已修复。Workspace setup 与 Goal 定义/修改表单均已增加 `required + whitespace + min` 校验，避免空白目标提交。

### 4.4 `api.ts` 的 query 透传与 `boolean` 序列化

- `querySuffix` 与 `URLSearchParams` 对 `undefined / null / ''` 做空过滤（`api.ts:80-88`），行为正确。
- 但 `listMCPCapabilities` 的 `enabled` 字段是 `boolean`，会被 `String(true)` → `"true"`，后端 `mcpCapabilityQueryFromRequest` 用 `strconv.ParseBool` 接收，OK。但 `enabled=undefined` 被 `querySuffix` 漏掉，后端则视作"未传"，可空——和类型表达一致，没问题。

### 4.5 `WorkspaceExecutionActivationRequest` 的 `priority` 默认 0

- 表单初始化 `priority: 0`（`WorkspaceDetailPage.tsx:1233`），后端 `runPriorityFromRequest` 缺省返回 0，正常。Form 没显式标注 0 的含义（最低 vs 缺省），可在 placeholder 注明"0=队列默认"。
- **收口状态（2026-06-22）**：已修复。Workspace 执行激活表单的 priority 输入提示已改为 `0 = 队列默认优先级`，与后端缺省语义对齐。

### 4.6 前端 SSE Hook 没有鉴权/重连退避上限

- `useSSE.ts:22` `maxRetries = 20`，失败后指数退避 30s 上限。`workspaceEventStream` 在后端 `http_events.go:31` 没有错误码限制，所以 20 次后即停。建议在 UI 给出"事件流已断开"提示。
- **收口状态（2026-06-22）**：已修复。`useSSE` 新增 `onExhausted` 回调，在达到重连上限后由页面提示事件流已断开；工作空间总览、工作空间详情、决策队列、审批队列会同步触发一次主动刷新，端侧不额外维护业务状态机。

### 4.7 后端 `goal-dod-quality-check` 端点无 UI 入口

- 后端 `POST /api/v1/workorch/workspaces/:workspaceId/goal-dod-quality-check`（`http_workspace.go:486`）支持对 Goal/DoD 文本做质量打分。
- 前端 `api.ts` 无对应封装，`workspaceSetup` 流程也没有调用。
- **影响**：后端能力闲置。
- **修复建议**：在 Workspace 创建/编辑流中接入"质量评分"按钮，结果以 Modal 展示。
- **收口状态（2026-06-22）**：已修复。Web 已新增 `checkGoalDoDQuality` API wrapper 和 `GoalDoDQualityCheck` DTO；Workspace 详情页的 Goal / DoD 定义列表增加“质量评分”操作，调用后端 workspace-scoped 端点并以 Modal 展示评分、可执行/可验证状态、缺失约束、建议和问题明细。由于该端点要求真实 `workspaceId`，创建工作空间前不伪造 scope，创建前仍以草案和表单校验为准。

---

## 五、接口对照表

| 接口 | 后端期望 | 前端实际 | 状态 |
|---|---|---|---|
| `GET /workspaces` | — | — | ✅ |
| `POST /workspaces` | `title/description/deliverableDirectory/templateId`（`workspaceId` 由后端生成） | 同 | ✅ |
| `DELETE /workspaces/:id` | `?purge=true` 或 body | `?purge=true` | ✅ |
| `GET /workspaces/:id/snapshot` | — | — | ✅ |
| `GET /workspaces/:id/task-progress` | — | — | ✅ |
| `GET /workspaces/:id/task-decompositions` | — | — | ✅ |
| `GET /workspaces/:id/memory-recalls` | `?workspaceId/runId/stepId/stage/source/status/limit` | `workspaceId` 走 path | ✅ 已确认 handler 用 `memoryRecallTraceQueryFromValues`，path 注入 workspaceId |
| `GET /workspaces/:id/soul-bindings` | — | — | ✅ |
| `PUT /workspaces/:id/soul-bindings` | `workspaceId/defaultSoulId/bindings/priorityPolicy/availableStages` | 同 | ✅ |
| `POST /workspaces/:id/execution-activations` | 11 字段全 `omitempty`，`priority` 可走 body / query / header | 自动注入 `dialogLanguage=navigator.language` | ✅ |
| `POST /workspaces/:id/goal-dod-quality-check` | `goalId/doDId/goalText/doDItems` | Workspace 详情页 Goal / DoD 质量评分 | ✅ |
| `GET /workspaces/:id/goals` | — | — | ✅ |
| `POST /workspaces/:id/goals` | `{ text }` | 同 | ✅ |
| `GET /workspaces/:id/dods` | — | — | ✅ |
| `POST /workspaces/:id/dods` | `{ items }` | 同 | ✅ |
| `PUT /goals/:goalId` | `{ text }` | 表单已校验非空白文本 | ✅ |
| `PUT /dods/:dodId` | `{ items }` | 同 | ✅ |
| `POST /workspace-setup-draft-sessions` | `templateId/title/description/deliverableDirectory/dialogLanguage` | 同 | ✅ |
| `POST /workspace-setup-draft-sessions/:id/messages` | `content` 必填 | 仅在 content 非空时调用 | ✅ |
| `POST /workspace-setup-draft-sessions/:id/regenerate` | body 可空 | 不传 content | ✅ |
| `GET /runs` | `?workspaceId=` | 支持可选 `workspaceId` | ✅ |
| `GET /runs/:id` | `?workspaceId=` | `api.ts:runPath` 支持 | ✅ |
| `POST /workspaces/:id/runs` | `runId/goalId/doDId`，后两个**必填** | 类型已改为必填 | ✅ |
| `POST /runs/:id/start` & `/resume` & workspace-scoped | `?priority=` + body `executionContextPatch` | `resumeRun` 已支持可选 body patch；`startRun` 保留 command metadata | ✅ |
| `GET /runs/:id/{steps,tool-calls,runtime-checkpoints,results,patches,validation-results,artifacts}` | `?workspaceId/cursorAfterId/limit` | `api.ts:runPageSuffix` 支持 | ✅ |
| `GET /runs/:id/{snapshot,observability,context-bundle,task-progress,task-decompositions,memory-recalls}` | `?workspaceId=` | 支持 | ✅ |
| `GET /artifacts/content?uri=` | — | `${apiBase}/artifacts/content?uri=` | ✅ |
| `GET /runtime/payloads?ref=` | — | `${apiBase}/runtime/payloads?ref=` | ✅ |
| `GET /events` | `?workspaceId/runId/topic/subjectType/subjectId/cursorAfterId/occurredAfter/occurredBefore/limit` | `?workspaceId/runId/limit=` | ✅（其余字段未传，符合需求） |
| `GET /events/stream` | `?workspaceId/runId/lastEventId` | `useSSE` 支持 workspace/run scope，详情页按 workspace 过滤 | ✅ |
| `GET /memory?workspaceId=&q=&tag=&type=&limit=` | `workspaceId` 必填，搜索关键字使用 `q` | `workspaceId` 必传，搜索关键字已改用 `q` | ✅ |
| `GET /memory/:id` / `PUT /memory/:id` / `DELETE /memory/:id` / `POST /workspaces/:id/memory` | `CreateMemoryItemCommand` | Workspace 详情页工作空间记忆管理 | ✅ |
| `GET/POST/PUT/DELETE /souls` | `id/name/soulMd` 必填 | Form 已 required | ✅ |
| `GET/POST/PUT/DELETE /workspace-templates` | `id/name` 必填 | `saveTemplate` 传全字段 | ✅ |
| `GET /skills` | `?workspaceId=` | 支持 | ✅ |
| `GET /mcp/servers` | — | — | ✅ |
| `POST /mcp/servers` | `id/name/transport/enabled` 必填 | 表单兜底 | ✅（注意 3.5/3.6） |
| `PUT /mcp/servers/:id` | 同上 | 同上 | ✅ |
| `GET /mcp/servers/:id/tools|resources|prompts` | — | — | ✅ |
| `GET /mcp/capabilities` | `?serverId/kind/category/stage/riskLevel/enabled/q` | 同 | ✅ |
| `POST /mcp/capabilities` | `id/serverId/kind/name/displayName/enabled` 必填 | RuntimeControlPage 新建能力表单已支持 | ✅ |
| `PUT /mcp/capabilities/:id` | 全字段 | 表单已支持 `schema/uri`，技术标识编辑时锁定 | ✅ |
| `PUT /configuration/llm` | 嵌套 `providers[]` 含 `apiKey/inferenceProfile/contextProfile` | 全字段 | ✅ |
| `PUT /configuration/web-search` | `providers[]` 含 `headers` | 表单与 payload 已支持 headers | ✅ |
| `PUT /configuration/server` | `defaultDialogLanguage` | 同 | ✅ |
| `GET /decisions` | `?workspaceId/runId/cursorAfterId/limit` | 同 | ✅ |
| `GET /decisions/:id` | — | — | ✅ |
| `POST /decisions/:id/resolve` | `resolutionType` 必填，`resolutionPayload` 可空 | `resolveDecision/rejectDecision` 均传 | ✅ |
| `POST /decisions/resolve-batch` | `ticketIds/resolutionType/resolutionPayload` | 同 | ✅ |
| `GET /approvals` | `?workspaceId/runId/cursorAfterId/limit` | 同 | ✅ |
| `GET /approvals/:id` | — | — | ✅ |
| `POST /approvals/:id/approve|reject` | 无 body | 无 body | ✅ |
| `POST /approvals/resolve-batch` | `approvalIds/action` | 同 | ✅ |

---

## 六、收口验证摘要

### 6.1 当前结论（2026-06-22）

本审计文档列出的 Web Console 与后端 REST/SSE 参数对齐问题已经按 P0/P1/P2 完成代码收口。当前保留的接口行为差异均为显式契约差异，例如 `deleteWorkspace` 由 Web 端固定采用 query contract，后端 body contract 保留给非浏览器客户端。

### 6.2 实现证据

- Web API 封装：`web/workorch-console/src/api.ts`
  - `createRun` 的 `goalId/doDId` 已改为必填。
  - `resumeRun` 已支持 `executionContextPatch` body。
  - `listRuns` 已支持 `workspaceId` query。
  - `createWorkorchEventStream` 已支持 `workspaceId/runId/lastEventId/view`。
  - `checkGoalDoDQuality` 已补齐 workspace-scoped API wrapper。
- Web 表单与 ViewModel：`RuntimeControlPage.tsx`、`ConfigurationPage.tsx`、`WorkspaceDetailPage.tsx`、`WorkspaceBoardPage.tsx`、`helpers.tsx`、`useSSE.ts`
  - MCP 能力新建、`schema/uri` 编辑、只读标识锁定、`enabled` 显式判别、env 三态语义已落地。
  - Web Search `headers` 表单、解析、校验与保存 payload 已落地。
  - SSE scope 变化时会重置 `lastEventId`，断连耗尽后触发页面主动刷新，不引入端侧业务状态机。
- 后端契约源：`internal/modules/application/workorchd/service/server/http_*.go`
  - `GET /runs?workspaceId=`、workspace-scoped run routes、SSE projection stream、Goal/DoD quality check、MCP capabilities、configuration web-search headers 等 handler 与前端调用保持一致。

### 6.3 复验命令

```bash
npm run build
go test ./internal/modules/application/workorchd/service/server -run 'TestWorkorchDHTTPRoutes|TestWorkorchDNativeAPI|TestWorkorchDWorkspaceRunCommand|TestDecisionResolveEndpointProjectsReplanRunCommand|TestWorkorchDSchedulerWorkspaceRunCommand|TestEventStream|TestWorkorchDGoalDoDQuality|TestWorkorchDMemory' -count=1 -timeout=240s
```

2026-06-22 复验结果：

- `npm run build`：通过，并刷新嵌入式 console assets。
- 上述 `go test`：通过。

---

## 七、涉及文件清单

- `web/workorch-console/src/api.ts`
- `web/workorch-console/src/types.ts`
- `web/workorch-console/src/pages/RuntimeControlPage.tsx`
- `web/workorch-console/src/pages/ConfigurationPage.tsx`
- `web/workorch-console/src/pages/WorkspaceDetailPage.tsx`
- `web/workorch-console/src/pages/WorkspaceBoardPage.tsx`
- `web/workorch-console/src/helpers.tsx`
- `web/workorch-console/src/hooks/useSSE.ts`

后端参考（不需改动，但作为契约源）：

- `internal/modules/application/workorchd/service/server/http_workspace.go`
- `internal/modules/application/workorchd/service/server/http_run.go`
- `internal/modules/application/workorchd/service/server/http_run_cache.go`
- `internal/modules/application/workorchd/service/server/http_governance.go`
- `internal/modules/application/workorchd/service/server/http_catalog.go`
- `internal/modules/application/workorchd/service/server/http_mcp.go`
- `internal/modules/application/workorchd/service/server/http_memory.go`
- `internal/modules/application/workorchd/service/server/http_events.go`
- `internal/modules/application/workorchd/service/server/http_configuration.go`
- `internal/modules/application/workorchd/service/server/workspace_setup_draft.go`
- `internal/modules/application/workorchd/service/server/http_query_helpers.go`
- `internal/modules/application/workorchd/service/server/http_types.go`
