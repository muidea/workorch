# WorkOrch Web Console Overview

Status: active

Type: product-interface

Last Verified: 2026-08-16

## Purpose

本文记录 `web/workorch-console/src` 当前实际提供的页面、HTTP/SSE 依赖和产品边界。Web Console 是远程控制与
projection 观察面，不是第二套业务实现或 Runtime owner。

## Navigation

当前主导航由 `main.tsx` 注册：

| Entry | Page | Current responsibility |
| --- | --- | --- |
| 业务总览 | `WorkspaceBoardPage` | Workspace 聚合列表、当前运行摘要、创建入口和执行提交 |
| 工作空间 | `WorkspaceDetailPage` | 单 Workspace 的定义、Run、证据、上下文、资源和诊断详情 |
| 治理队列 | `GovernanceQueuePage` | Decision / Approval 分页、详情及单个/批量处理 |
| 运行控制 | `RuntimeControlPage` | MCP server/capability 管理、发现结果和 metrics 摘要 |
| 系统配置 | `ConfigurationPage` | 支持的 server、LLM、Web Search、Soul 与 Template 配置 |
| 服务状态 | `ServicePage` | health、ready 和 metrics |

路由采用前端 hash/navigation state；生产静态资源由 workorchd 在 `/console/**` 提供。

## Page Contracts

### Business Board

`WorkspaceBoardPage` 的主读入口是 `GET /api/v1/workorch/workspaces` 返回的聚合列表与 projection status。它不会
为了绘制每一张卡片分别读取 Goal、DoD 或 Run child pages。

用户提交执行时，页面按需读取 Workspace snapshot：

1. 无 active Run、active Run 已终态或 owner 已标记 `fresh_activation_required` 时，主操作提交 `restart`；
2. current-baseline 非终态 Run 的主操作提交 `resume`，同时提供独立的显式 `restart`；
3. Resume 只读展示 Run 已封存 selection，不读取模型目录、不更新 Workspace preference；
4. 只有显式 Restart 弹窗允许先通过 Workspace owner API 保存模型/reasoning 与 Soul，再提交 immutable intent；
5. activation body 只发送 `intent`、`priority`、`requestId`、`reason`、`source`。

activation form 不提供 Goal/DoD、Skill、MCP 或 Memory 的 per-run override；这些定义和策略只从 owner 当前状态编译。
Workspace 创建使用共享的 `WorkspaceSetupModal`，正常流程要求 Template、Goal 与 DoD。工作目录允许留空；创建时由
application 生成 `.workorch-workspaces/<workspace-id>`，显式输入只作为目录覆盖。

### Workspace Detail

`WorkspaceDetailPage` 先读取 Workspace list/detail projection，再按当前选择加载：

- Workspace snapshot、Soul bindings、服务配置、Soul、Skill、MCP catalog 与 Memory list；
- 当前 Run snapshot、observability 与 context bundle；
- 按需分页的 Step、ToolCall、RuntimeCheckpoint、Result、Patch、ValidationResult 与 Artifact；
- workspace/run-scoped EventRecord 诊断列表。

页面支持 Goal/DoD define/update、quality check、Workspace LLM/Soul selection、execution activation 与 Workspace delete。
这些表单只组装已公开的 HTTP DTO；owner 校验、revision CAS 和状态转换全部留在服务端。

`steps`、`runtime-checkpoints`、`nextAction`-named fields、`taskProgress`、`taskDecompositions` 和
`artifactContinuations` 若仍出现在 projection/type 中，只是历史/诊断兼容数据。它们不能驱动按钮可用性、恢复
Runtime authority 或替代 formal Result/evidence。

### Governance Queue

Decision 和 Approval 页面分别使用分页 list，显式打开单项 detail，并提交：

- `POST /decisions/:ticketId/resolve` 或 `/decisions/resolve-batch`；
- `POST /approvals/:approvalId/approve|reject` 或 `/approvals/resolve-batch`。

页面可以帮助组织 resolution payload，但不得推导 Run 下一状态，也不得在 resolve 成功后自行发送 Resume。通用
Decision UI 中的历史 `replan` 选择会由服务端合同规范化；它不授权 application 创建 successor。

### Runtime Control

`RuntimeControlPage` 管理 MCP Registry 的 server/capability metadata，并按用户操作读取 tools、resources、prompts
发现结果。Server 表单只接受 `KEY=env://ENV_NAME` 或受支持 workspace placeholder 形式的 environment reference，不接收
或回显 credential 明文；Capability 使用 `enabled` 作为唯一可用性字段并展示 owner provenance。它不直接发起 Runtime MCP
operation；具体执行仍由 sealed dispatch 进入 `blocks/mcp` owner。

### Configuration

`ConfigurationPage` 读取统一 configuration projection，并仅写当前 UI 支持的字段：

- supported server configuration；
- ai-proxy LLM model/key/budget configuration；
- Web Search provider configuration；
- Soul CRUD 与 Workspace Template CRUD。

页面展示的 runtime/git/LLM-output 配置摘要不等于它支持修改全部 runtime 字段。LLM save 只更新 persisted
configuration；active owner snapshot 仍需进程重启才变化。

### Service Status

`ServicePage` 只读取 `/healthz`、`/readyz` 与 `/metrics`。它不把 process readiness 推导为某个 Run 已完成。

## Refresh And Event Model

Board、Workspace detail、Decision 和 Approval 页面订阅 `GET /api/v1/workorch/events/stream`：

- SSE 是刷新提示，不是 owner state；
- board/detail 主要消费 workspace projection-changed notification；
- governance pages 只对相关事件重新读取分页数据；
- revision gap、重连或 retry exhaustion 触发受影响的 HTTP projection 回读；
- Workspace detail 另有显式 EventRecord 查询用于诊断。

页面必须按 workspace ID 过滤事件并对 burst 做有界 debounce。SSE payload、浏览器 cache 或当前选中 workspace
都不能成为跨 Workspace 的隐式全局状态。

## Pagination And Raw Content

Run child lists 使用服务端 `limit` / `cursorAfterId`；“加载更多”不能依赖一次全量读取。大正文只通过用户显式动作
打开：

- `GET /api/v1/workorch/artifacts/content?uri=...`；
- `GET /api/v1/workorch/runtime/payloads?ref=...`。

URI/ref 由服务端 allowlist 校验。前端不得根据 filesystem path 自行拼接下载地址，也不得把 raw payload 作为默认
页面状态或 Runtime authority。

## Form And State Boundary

所有 create/update form 统一遵守：

- 只收集输入、做最小字段校验、调用正式 API，并在成功后回读 list/detail/snapshot；
- 失败时保留输入并展示服务端错误，不在浏览器构造替代结果；
- 不复刻 appservice、DoD quality、governance 或 owner reducer 规则；
- 不直接执行 Shell、Git、File、Web、MCP 或 provider side effect；
- 不维护并行的 Workspace/Run/Decision/Approval 状态机；
- 不构造或转发已删除的 `executionContext` / `ExecutionContextPatch`；
- 不从 projection compatibility fields 创建 successor、选择 action 或接纳 evidence。

Web 与 TUI 共享同一 HTTP/projection 语义：TUI 偏 agent-first 本地交互，Web 偏多 Workspace 观察、治理和配置。

## Source And Verification

- API client：`web/workorch-console/src/api.ts`
- navigation：`web/workorch-console/src/main.tsx`
- pages：`web/workorch-console/src/pages`
- types：`web/workorch-console/src/types.ts`
- service contract：[Service API](../service-api.md)
- acceptance：[Service Acceptance](../../60-verification/service-acceptance.md)
- 历史 bundle/UI 优化记录：[archive record](../../90-archive/records/web-console-bundle-ui-optimization-record-2026-08-16.md)

变更页面调用时必须同步更新 API client/types、对应 server DTO/route test、本文件和服务验收；历史性能截图或 bundle
清单只进入 archive record。
