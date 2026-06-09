# WorkOrch Web Console Overview

Status: active

Type: product-interface

Last Updated: 2026-05-30

## Purpose

本文件定义 WorkOrch Web Console 的正式产品入口、页面分区、读模型依赖和交互边界。

## Product Positioning

Web Console 是 WorkOrch 的远程控制与观察面，主要服务：

- 浏览多个 Workspace / Run 的当前状态
- 处理 Decision / Approval 治理队列
- 查看 runtime evidence、Result / Artifact、TaskProgress 和 ContextBundle
- 管理 Soul / Workspace Template / MCP / 配置等控制面资源

它不是：

- 本地 TUI 的替代品
- 第二套业务实现
- 直接执行 shell/MCP/provider 的客户端

所有业务动作都必须继续经由 `workorchd` HTTP API 进入 appservice。

## Main Navigation

当前控制台主导航包括：

- `业务总览`
- `工作空间`
- `治理队列`
- `运行控制`
- `系统配置`
- `服务状态`

其中：

- `业务总览`
  - 面向多 workspace 总览、当前运行、阻塞与启动入口
- `工作空间`
  - 面向单 workspace 的 detail、Goal / DoD / Run / Result / Artifact / Context / Governance 聚合
- `治理队列`
  - 面向 Decision / Approval 的处理
- `运行控制`
  - 面向 MCP server / capability / metrics 等运行控制面
- `系统配置`
  - 面向 server / llm / web-search / Soul / Workspace Template 配置
- `服务状态`
  - 面向 health / ready / metrics

## Page To API Mapping

### 1. Business Board

主要页面：

- `WorkspaceBoardPage`

主要 API：

- `GET /api/v1/workorch/workspaces`
- `GET /api/v1/workorch/workspaces/:workspaceId/snapshot`
- `GET /api/v1/workorch/workspaces/:workspaceId/goals`
- `GET /api/v1/workorch/workspaces/:workspaceId/dods`
- `POST /api/v1/workorch/workspaces/:workspaceId/execution-activations`
- `POST /api/v1/workorch/workspaces/:workspaceId/runs/:runId/resume`

主要读模型：

- `Workspace`
- `WorkspaceSnapshot`
- `Run`
- `WorkspaceOutcome`
- `TaskProgress`

主要字段关注：

- `workspace.id` / `title` / `deliverableDirectory`
- `workspace.activeRunId`
- `snapshot.outcome`
- `snapshot.taskProgress`
- `snapshot.runs`

### 2. Workspace Detail

主要页面：

- `WorkspaceDetailPage`

主要 API：

- `GET /api/v1/workorch/workspaces/:workspaceId/snapshot`
- `GET /api/v1/workorch/workspaces/:workspaceId/memory-recalls`
- `GET /api/v1/workorch/workspaces/:workspaceId/soul-bindings`
- `PUT /api/v1/workorch/workspaces/:workspaceId/soul-bindings`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/snapshot`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/observability`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/context-bundle`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/task-progress`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/task-decompositions`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/memory-recalls`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/steps`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/tool-calls`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/runtime-checkpoints`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/results`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/patches`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/validation-results`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/artifacts`
- `GET /api/v1/workorch/events`

主要读模型：

- `WorkspaceSnapshot`
- `RunSnapshot`
- `RunObservability`
- `ContextBundle`
- `TaskProgress`
- `TaskDecomposition`
- `MemoryRecallTrace`
- `Decision`
- `Approval`

主要字段关注：

- workspace 级：
  - `snapshot.workspace`
  - `snapshot.goals`
  - `snapshot.dods`
  - `snapshot.runs`
  - `snapshot.artifactContinuations`（若存在，只作为 active continuation metadata 摘要）
  - `snapshot.decisions` / `snapshot.approvals`
- run 级：
  - `runSnapshot.results` / `artifacts` / `patches`
  - `runSnapshot.artifactContinuations`（默认只展示 state/target/progress/hash 摘要，不内嵌正文）
  - `runObservability.timeline` / `closureTraces`
  - `contextBundle.sections` / `recentEvidence`
  - `taskProgress.repairLoop`

### 3. Governance Queue

主要页面：

- `DecisionQueuePage`
- `ApprovalQueuePage`
- `GovernanceQueuePage`

主要 API：

- `GET /api/v1/workorch/decisions`
- `GET /api/v1/workorch/decisions/:ticketId`
- `POST /api/v1/workorch/decisions/:ticketId/resolve`
- `POST /api/v1/workorch/decisions/resolve-batch`
- `GET /api/v1/workorch/approvals`
- `GET /api/v1/workorch/approvals/:approvalId`
- `POST /api/v1/workorch/approvals/:approvalId/approve`
- `POST /api/v1/workorch/approvals/:approvalId/reject`
- `POST /api/v1/workorch/approvals/resolve-batch`

主要读模型：

- `DecisionPage`
- `ApprovalPage`
- `Decision`
- `Approval`
- `AutoGovernanceSummary`

主要字段关注：

- `decisions[].reason` / `question` / `state`
- `decisions[].resolutionType` / `resolutionPayload`
- `approvals[].reason` / `action` / `state`
- `autoGovernance.status` / `action` / `reason`

### 4. Runtime Control

主要页面：

- `RuntimeControlPage`

主要 API：

- `GET /api/v1/workorch/mcp/servers`
- `POST /api/v1/workorch/mcp/servers`
- `PUT /api/v1/workorch/mcp/servers/:serverId`
- `GET /api/v1/workorch/mcp/servers/:serverId/tools`
- `GET /api/v1/workorch/mcp/servers/:serverId/resources`
- `GET /api/v1/workorch/mcp/servers/:serverId/prompts`
- `GET /api/v1/workorch/mcp/capabilities`
- `POST /api/v1/workorch/mcp/capabilities`
- `PUT /api/v1/workorch/mcp/capabilities/:capabilityId`
- `GET /metrics`

主要读模型：

- `MCPServer`
- `MCPCapability`
- `MCPTool`
- `MCPResource`
- `MCPPrompt`

主要字段关注：

- server：
  - `transport` / `endpoint` / `command` / `args`
  - `defaultApprovalPolicy` / `sessionPolicy`
- capability：
  - `displayName` / `kind` / `category`
  - `stageHints` / `riskLevel` / `approvalPolicy`
  - `enabled` / `status`

### 5. Configuration

主要页面：

- `ConfigurationPage`

主要 API：

- `GET /api/v1/workorch/configuration`
- `PUT /api/v1/workorch/configuration/server`
- `PUT /api/v1/workorch/configuration/llm`
- `PUT /api/v1/workorch/configuration/web-search`
- `GET /api/v1/workorch/souls`
- `POST /api/v1/workorch/souls`
- `PUT /api/v1/workorch/souls/:soulId`
- `DELETE /api/v1/workorch/souls/:soulId`
- `GET /api/v1/workorch/workspace-templates`
- `POST /api/v1/workorch/workspace-templates`
- `PUT /api/v1/workorch/workspace-templates/:templateId`
- `DELETE /api/v1/workorch/workspace-templates/:templateId`

主要读模型：

- `ServiceConfiguration`
- `Soul`
- `WorkspaceTemplate`
- provider / capability configuration DTOs

主要字段关注：

- `configuration.server`
- `configuration.repository`
- `configuration.runtime`
- `configuration.llm`
- `configuration.webSearch`
- `souls[].allowed*`
- `workspaceTemplates[].goalText` / `doDItems` / `defaultSoulId` / `soulBindings`

### 6. Service Status

主要页面：

- `ServicePage`

主要 API：

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`

主要字段关注：

- `status`
- metrics 文本

## Event And Refresh Model

Web Console 当前通过 SSE 建立轻量事件刷新：

- `GET /api/v1/workorch/events/stream`

稳定规则：

- SSE 只负责通知“需要刷新哪些页面数据”
- 主数据仍通过普通 read API 重读
- 断线补偿应回退到 `GET /api/v1/workorch/events` 或重新读取 snapshot/observability
- Web Console 不应把 SSE 当作唯一事实源

当前页面刷新方向：

- `WorkspaceBoardPage`
  - SSE 到达后重新读取 workspace list + snapshot
- `WorkspaceDetailPage`
  - SSE 到达后只刷新当前选中 workspace 的上下文与运行详情
- `GovernanceQueuePage`
  - 只在 `decision` / `approval` / `run.*` 相关 topic 到达时刷新

## Raw Content Boundary

Web Console 当前可显式打开：

- `GET /api/v1/workorch/artifacts/content`
- `GET /api/v1/workorch/runtime/payloads`

稳定规则：

- raw content 只在用户显式展开时打开
- 默认页面应优先展示 summary / snapshot / observability / page list
- raw payload / artifact content 不应替代主阅读路径
- `artifactContinuations` 在 board/detail/observability 中默认只展示 metadata 和进度摘要；
  plan/chunk/assembled 正文只允许通过显式 raw/detail 打开，不直接嵌入页面主内容

## Low-Frequency Interaction Boundary

以下低频交互当前应明确保持“显式打开、显式提交、显式回读”的产品边界。

### 1. Workspace Create / Goal / DoD Drafting

- 创建工作空间只负责组织创建期字段：
  - `title`
  - `description`
  - `deliverableDirectory`
  - `templateId`
- 定义 Goal / DoD 只负责调用正式写接口，不在前端预判：
  - 目标是否足够完整
  - DoD 是否可满足
  - 是否应自动生成 run
- Goal / DoD 质量诊断应继续通过 `POST /api/v1/workorch/workspaces/:workspaceId/goal-dod-quality-check`
  返回的结构化结果呈现，而不是由前端自行复刻规则

### 2. Governance Detail / Batch Resolution

- `GovernanceQueuePage` 默认展示 open/pending 队列和 compact list
- 只有用户显式展开或进入处理弹窗时，才读取：
  - `GET /api/v1/workorch/decisions/:ticketId`
  - `GET /api/v1/workorch/approvals/:approvalId`
- 决策处理弹窗可以组织：
  - structured payload fields
  - optional answer/note
  - optional advanced JSON payload
  - operator
- 这些输入只用于组成 `resolutionPayload` 或 approve/reject action，不在前端推导 run 下一状态
- 批量处理只组织选中对象 ID 与 action，不在前端做“是否应批量可处理”的业务裁决

### 3. Runtime Detail Drill-Down

- `WorkspaceDetailPage` 默认优先展示 snapshot / observability / compact sections
- `steps` / `tool-calls` / `runtime-checkpoints` / `results` / `patches` /
  `validation-results` / `artifacts` 属于按需 deep view
- “加载更多”必须依赖对应 detail page 的 `cursorAfterId` / `limit`，而不是假设一次全量加载
- JSON panel 只作为诊断补充：
  - decision context
  - resolution payload
  - auto-governance summary
  - selected raw structs

### 4. Result / Artifact / Raw Payload Open

- artifact 正文通过 `artifactContentURL(uri)` 这类受控链接显式打开
- runtime raw payload 通过 `runtimePayloadURL(ref)` 这类受控链接显式打开
- 打开 raw/detail 不应反向改变当前页面的 summary state、governance state 或 active run 选择
- raw/detail 下载或展开失败时，前端只展示接口错误，不自行构造替代内容

## Create / Update Form Boundary

Web Console 的 create/update forms 当前应统一遵守以下规则。

### Form Responsibility

- form 负责：
  - 收集用户输入
  - 做最小字段级校验
  - 组装 request body
  - 调用正式 API
  - 成功后回读 list/detail/snapshot
- form 不负责：
  - 在前端实现 appservice command 规则
  - 推导审批/决策/运行状态机
  - 复刻 verifier 或 DoD 质量规则

### Configuration / Registry Forms

- `ConfigurationPage` 只承接：
  - server configuration
  - llm configuration
  - web-search configuration
  - Soul CRUD
  - Workspace Template CRUD
- `RuntimeControlPage` 只承接：
  - MCP server create/update
  - MCP capability filter/update
- registry/configuration forms 可以做：
  - required field 校验
  - enum/tag/list normalization
  - JSON text validity 校验
- 但不应在前端把：
  - Soul allowlist
  - template DoD validity
  - capability approval policy fallback
  - provider/runtime compatibility
  判定为本地真值

### Workspace / Governance Forms

- workspace create form 不负责自动绑定后续 run；是否进入 activation/run 由用户显式动作决定
- decision resolution form 不负责推导 `replan` 后 successor run 的具体状态
- approval confirm/reject form 中的 comment 当前只服务界面确认，不应被文档表述成正式审批模型字段

### Save / Refresh Contract

- 成功写入后，页面必须回读受影响的 list/detail/snapshot，而不是只在前端局部打补丁
- 失败时保留表单输入、展示结构化错误，并以服务端错误为准
- 前端 toast/message 只表达操作结果，不应宣称未被服务端返回证明的业务结论

## Product Boundary

Web Console 必须保持以下边界：

- 不直接构造业务状态
- 不绕过 appservice 执行 shell/MCP/provider 动作
- 不在前端维持并行 run/decision/approval 状态机
- 不把 `executionContext` 原始 JSON 作为默认主视图
- 不把 `selectedCapabilities` / `memoryRecallTraces` / `closureTraces` 全量噪声堆到首页

交互补充边界：

- 页面上的“启动执行”“继续运行”“批准/拒绝”“重新规划”等动作都必须调用现有 API，而不是在前端推导状态
- create/update forms 只组织请求体，不在前端重演 appservice 的业务规则
- 展开 JSON 面板只用于诊断补充，不是默认阅读路径

## Relationship To TUI

- TUI 偏本地 agent-first 体验
- Web Console 偏远程控制与只读/治理/配置聚合面
- 两者都必须复用同一组 read model、API 和治理对象

## Source Documents

- `../service-api.md`
- `bundle-ui-optimization-record.md`
- `../../60-verification/service-acceptance.md`
