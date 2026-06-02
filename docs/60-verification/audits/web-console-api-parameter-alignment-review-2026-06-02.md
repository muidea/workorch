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

### 2.2 MCP 能力编辑 Modal 漏字段 `schema` 与 `uri`

- **后端**：`MCPCapability`（`http_mcp.go:190`）含 `schema map[string]any` 与 `uri string` 两个字段。
- **前端**：`RuntimeControlPage.tsx:267-297` 的 `saveCapability` 通过 `...editingCapability` 保留旧值，但 Form 里**没有** `schema` / `uri` 的 Form.Item（`RuntimeControlPage.tsx:640-702`）。
- **影响**：编辑时只能"维持"旧值，无法修改或新建带 schema/uri 的能力。
- **修复建议**：补充 `Form.Item name="schema"`（JSON 编辑器）与 `name="uri"`，并把 `id / serverId / kind / name` 改为受控展示（`disabled` 锁死）。

### 2.3 WebSearch `headers` 字段被静默丢弃

- **后端**：`UpdateWebSearchProviderConfiguration` 含 `headers map[string]string`（`http_configuration.go:61`）。
- **前端**：类型 `UpdateWebSearchProviderConfiguration.headers?: Record<string,string>` 已声明（`types.ts:217`），但 `ConfigurationPage.tsx:156-187` 的 `saveWebSearchConfiguration` **没有**把 headers 放进 payload，Form 也没有对应输入（`ConfigurationPage.tsx:509-541`）。
- **影响**：后端支持的"自定义请求头"能力完全无法配置；保存一次会清掉已有 headers（如果 omitempty 路径不严格保留）。
- **修复建议**：在表单增加 `headers` 键值对编辑器，并在 `saveWebSearchConfiguration` 的 providers 映射里补 `headers: parsedHeaders ?? {}`。

### 2.4 `createRun` 类型与后端约束不一致

- **后端**：`http_run.go:392` 强制 `goalId` 与 `doDId` 非空。
- **前端**：`api.ts:337` 声明 `goalId?: string; doDId?: string`（可选），类型误导调用方。
- **影响**：当前无调用点，暂无线上故障；但任何后续按类型签名编写的代码都会踩 400。
- **修复建议**：把 `createRun` 的签名改为 `(workspaceId, runId, goalId, doDId)` 四个必填，或在 `WorkspaceExecutionActivationRequest` 风格的请求类型里把这两个标为必填。

---

## 三、中等问题（潜在功能偏差）

### 3.1 SSE 订阅不传 `workspaceId` / `runId` 过滤

- **后端**：`/api/v1/workorch/events/stream` 接受 `?workspaceId=&runId=&lastEventId=`（`http_events.go:31`）。
- **前端**：`api.ts:7-16` 的 `createWorkorchEventStream` 只把 `lastEventId` 传出（`useSSE.ts:41`）。
- **影响**：浏览器拉取的是全量事件流，多工作空间/多浏览器时事件压力被放大；与后端已经设计好的过滤能力不匹配。
- **修复建议**：在 `useSSE` 接受 `workspaceId?` / `runId?` 参数，透传到 EventSource URL。

### 3.2 续跑 `resumeRun` 不支持 `executionContextPatch`

- **后端**：`POST /api/v1/workorch/runs/:runId/resume` 与 `workspaces/:workspaceId/runs/:runId/resume` 均解析 `{ executionContextPatch }`（`http_run.go:476-491`、`http_run.go:691-706`）。
- **前端**：`api.ts:348` 的 `resumeRun` 只透传 `priority` query，没有 body 参数；`WorkspaceBoardPage.tsx:137` 与 `WorkspaceDetailPage.tsx:1270` 调用都没有 patch。
- **影响**：当前业务无场景，但运行续跑时无法向 LLM 注入"用户补充信息"。文档/契约上应补上。
- **修复建议**：把 `resumeRun(runId, priority?, workspaceId?, executionContextPatch?)` 拼到 body 里。

### 3.3 `listRuns` 没有 `workspaceId` 过滤

- **后端**：`GET /api/v1/workorch/runs` 支持 `?workspaceId=`（`http_run.go:15`）。
- **前端**：`api.ts:236` 的 `listRuns` 不接受参数，只能全量。
- **影响**：工作空间内的 run 列表走 `readWorkspaceSnapshot`（`api.ts:139`），不会触发 `listRuns`；所以 UI 无可见 bug，但前端有未使用的"全量拉取"调用，代码意图不清晰。
- **修复建议**：要么删除未使用的 `listRuns`，要么补上 `workspaceId` 参数透传。

### 3.4 `purge` 在 `deleteWorkspace` 上的 query/body 双轨不一致

- **后端**：`http_workspace.go:282-303` 同时支持 `?purge=true` query 与 body 的 `purge` 字段，二者均生效。
- **前端**：`api.ts:194-198` 只用 query 透传 `purge`；`workspaceId` 拼到 URL，body 为空。
- **影响**：当前 OK，但当后端需要从 body 取值时前端不会传。建议在 `api.ts` 注释中明确"只走 query"以避免未来混淆。

### 3.5 `MCPServer` 表单 `env` 三态编辑易踩坑

- `RuntimeControlPage.tsx:40-62` `mcpServerPayload`：当 `envMode === 'keep'` 时**不发** `env` 字段；后端 `MCPServer.env` 在 `MCPServer` 类型上是 `map[string]string`（`http_mcp.go:146`），PUT 请求会保留原值（`updateMCPServer` 走 `appservice.MCPServer`）。逻辑上 OK，但依赖后端 "不传则保留"的实现细节，不健壮。
- **修复建议**：在 `api.ts` 注释里写明，或者改成始终发送 `env`（用户清空用 `{}`）。

### 3.6 `MCPServer.enabled` 表单缺省为 `true`

- `RuntimeControlPage.tsx:48` `enabled: values.enabled !== false` —— `values.enabled` 为 `undefined` 时（理论上不会发生）会变成 `true`。
- **影响**：低，但 `undefined` 应判空而非默认 true，否则后端 `enabled bool`（非 omitempty）收到 `true` 也会被静默接受，掩盖前端 bug。
- **修复建议**：用 `values.enabled === true` 显式判别。

---

## 四、低风险 / 风格建议

### 4.1 `MCP` / `Soul` / `Template` 编辑器对只读字段缺乏强约束

- `RuntimeControlPage.tsx:267` `saveCapability` 用 `...editingCapability` 兜底再覆盖，掩盖"该字段是技术标识"的事实，编辑器可读性差。
- `helpers.tsx:2191` `saveSoul` 同理，技术标识 `id` 应在 Form 中可见且不可改。
- **修复建议**：加显式 `Form.Item name="..."` 锁死为 disabled 展示。

### 4.2 `appendWorkspaceSetupDraftMessage` 的 `content` 必填与前端逻辑

- `api.ts:180` 把 `content: string` 标为必填；调用点 `WorkspaceDetailPage.tsx:968-970` 在 `content` 非空时走 messages、空时走 regenerate，**无**误用风险。
- **修复建议**：在 `api.ts` 给 `regenerateWorkspaceSetupDraft` 的类型也明示 `content` 不应出现，避免后续重构时把 content 错塞进去。

### 4.3 `goals/:goalId` `PUT` 后端使用 `defineGoalRequest`（text 必填）

- 前端 `updateGoal` 总是传 `values.text`（`WorkspaceDetailPage.tsx:2630`），正常。但 form 上没有 `required` 校验，提交时若空字符串会被后端接受（`text string` 无 omitempty 但允许空串）。
- **修复建议**：在 Form 上加 `rules={[{ required: true, min: 1 }]}`。

### 4.4 `api.ts` 的 query 透传与 `boolean` 序列化

- `querySuffix` 与 `URLSearchParams` 对 `undefined / null / ''` 做空过滤（`api.ts:80-88`），行为正确。
- 但 `listMCPCapabilities` 的 `enabled` 字段是 `boolean`，会被 `String(true)` → `"true"`，后端 `mcpCapabilityQueryFromRequest` 用 `strconv.ParseBool` 接收，OK。但 `enabled=undefined` 被 `querySuffix` 漏掉，后端则视作"未传"，可空——和类型表达一致，没问题。

### 4.5 `WorkspaceExecutionActivationRequest` 的 `priority` 默认 0

- 表单初始化 `priority: 0`（`WorkspaceDetailPage.tsx:1233`），后端 `runPriorityFromRequest` 缺省返回 0，正常。Form 没显式标注 0 的含义（最低 vs 缺省），可在 placeholder 注明"0=队列默认"。

### 4.6 前端 SSE Hook 没有鉴权/重连退避上限

- `useSSE.ts:22` `maxRetries = 20`，失败后指数退避 30s 上限。`workspaceEventStream` 在后端 `http_events.go:31` 没有错误码限制，所以 20 次后即停。建议在 UI 给出"事件流已断开"提示。

### 4.7 后端 `goal-dod-quality-check` 端点无 UI 入口

- 后端 `POST /api/v1/workorch/workspaces/:workspaceId/goal-dod-quality-check`（`http_workspace.go:486`）支持对 Goal/DoD 文本做质量打分。
- 前端 `api.ts` 无对应封装，`workspaceSetup` 流程也没有调用。
- **影响**：后端能力闲置。
- **修复建议**：在 Workspace 创建/编辑流中接入"质量评分"按钮，结果以 Modal 展示。

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
| `GET /workspaces/:id/memory-recalls` | `?workspaceId/runId/stepId/stage/source/status/limit` | `workspaceId` 走 path | ⚠️ 期望后端能根据 path 兜底，需在 handler 处确认（已确认 handler 用 `memoryRecallTraceQueryFromValues`，path 注入 workspaceId） |
| `GET /workspaces/:id/soul-bindings` | — | — | ✅ |
| `PUT /workspaces/:id/soul-bindings` | `workspaceId/defaultSoulId/bindings/priorityPolicy/availableStages` | 同 | ✅ |
| `POST /workspaces/:id/execution-activations` | 11 字段全 `omitempty`，`priority` 可走 body / query / header | 自动注入 `dialogLanguage=navigator.language` | ✅ |
| `POST /workspaces/:id/goal-dod-quality-check` | `goalId/doDId/goalText/doDItems` | **未使用** | ⚠️ 4.7 |
| `GET /workspaces/:id/goals` | — | — | ✅ |
| `POST /workspaces/:id/goals` | `{ text }` | 同 | ✅ |
| `GET /workspaces/:id/dods` | — | — | ✅ |
| `POST /workspaces/:id/dods` | `{ items }` | 同 | ✅ |
| `PUT /goals/:goalId` | `{ text }` | 同（Form 无 required 校验） | ⚠️ 4.3 |
| `PUT /dods/:dodId` | `{ items }` | 同 | ✅ |
| `POST /workspace-setup-draft-sessions` | `templateId/title/description/deliverableDirectory/dialogLanguage` | 同 | ✅ |
| `POST /workspace-setup-draft-sessions/:id/messages` | `content` 必填 | 仅在 content 非空时调用 | ✅ |
| `POST /workspace-setup-draft-sessions/:id/regenerate` | body 可空 | 不传 content | ✅ |
| `GET /runs` | `?workspaceId=` | 不接受参数 | ⚠️ 3.3 |
| `GET /runs/:id` | `?workspaceId=` | `api.ts:runPath` 支持 | ✅ |
| `POST /workspaces/:id/runs` | `runId/goalId/doDId`，后两个**必填** | 类型写为可选 | ❌ 2.4 |
| `POST /runs/:id/start` & `/resume` & workspace-scoped | `?priority=` + body `executionContextPatch` | 仅 priority | ⚠️ 3.2 |
| `GET /runs/:id/{steps,tool-calls,runtime-checkpoints,results,patches,validation-results,artifacts}` | `?workspaceId/cursorAfterId/limit` | `api.ts:runPageSuffix` 支持 | ✅ |
| `GET /runs/:id/{snapshot,observability,context-bundle,task-progress,task-decompositions,memory-recalls}` | `?workspaceId=` | 支持 | ✅ |
| `GET /artifacts/content?uri=` | — | `${apiBase}/artifacts/content?uri=` | ✅ |
| `GET /runtime/payloads?ref=` | — | `${apiBase}/runtime/payloads?ref=` | ✅ |
| `GET /events` | `?workspaceId/runId/topic/subjectType/subjectId/cursorAfterId/occurredAfter/occurredBefore/limit` | `?workspaceId/runId/limit=` | ✅（其余字段未传，符合需求） |
| `GET /events/stream` | `?workspaceId/runId/lastEventId` | 只传 `lastEventId` | ⚠️ 3.1 |
| `GET /memory?workspaceId=&q=&tag=&type=&limit=` | `workspaceId` 必填 | `api.ts:475` 必传 | ✅ |
| `GET /memory/:id` / `PUT /memory/:id` / `DELETE /memory/:id` / `POST /workspaces/:id/memory` | `CreateMemoryItemCommand` | **未使用** | ⚠️ 后端能力闲置 |
| `GET/POST/PUT/DELETE /souls` | `id/name/soulMd` 必填 | Form 已 required | ✅ |
| `GET/POST/PUT/DELETE /workspace-templates` | `id/name` 必填 | `saveTemplate` 传全字段 | ✅ |
| `GET /skills` | `?workspaceId=` | 支持 | ✅ |
| `GET /mcp/servers` | — | — | ✅ |
| `POST /mcp/servers` | `id/name/transport/enabled` 必填 | 表单兜底 | ✅（注意 3.5/3.6） |
| `PUT /mcp/servers/:id` | 同上 | 同上 | ✅ |
| `GET /mcp/servers/:id/tools|resources|prompts` | — | — | ✅ |
| `GET /mcp/capabilities` | `?serverId/kind/category/stage/riskLevel/enabled/q` | 同 | ✅ |
| `POST /mcp/capabilities` | `id/serverId/kind/name/displayName/enabled` 必填 | **无 UI 入口** | ❌ 2.1 |
| `PUT /mcp/capabilities/:id` | 全字段 | 漏 `schema/uri` 表单 | ❌ 2.2 |
| `PUT /configuration/llm` | 嵌套 `providers[]` 含 `apiKey/inferenceProfile/contextProfile` | 全字段 | ✅ |
| `PUT /configuration/web-search` | `providers[]` 含 `headers` | **漏 headers** | ❌ 2.3 |
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

## 六、收口推荐顺序

1. **P0（功能缺失）**
   - 补 MCP 能力新建入口（2.1）
   - 补 MCP `schema/uri` 编辑（2.2）
   - 补 WebSearch `headers` 表单（2.3）
2. **P1（契约/类型）**
   - 修 `createRun` 类型（2.4）
   - SSE 透传 `workspaceId/runId`（3.1）
3. **P2（体验/防御）**
   - `resumeRun` 支持 `executionContextPatch`（3.2）
   - 删除或补参 `listRuns`（3.3）
   - MCP env 改全量发送（3.5）
   - 显式判别 `enabled` / `text required` 校验（3.6 / 4.3）
   - 接入 `goal-dod-quality-check` 与 `POST /memory/:id` 等闲置后端能力（4.7）

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
