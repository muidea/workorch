# MCP Capability Management

Status: active

Type: profile-capability

Last Updated: 2026-05-30

## Purpose

本文件承接 MCP capability 的统一识别、管理、绑定和运行时展示设计。

## Scope

- MCP Server 与 Capability 的关系
- tool / resource / prompt 的业务化展示
- Workspace / Soul / Skill / Run 对 capability 的引用

## Boundary

- capability catalog 是 profile/capability 层
- core runtime 不按具体 tool 名称做业务判断

## Stable Rules

### Core Concepts

- `MCPServer` 负责连接、会话和默认运行策略
- `MCPCapability` 负责表达单个 `tool` / `resource` / `prompt` 的正式能力项
- Workspace、Soul、Skill、Run activation 和运行证据都应引用 capability，而不是只依赖裸技术白名单字符串

### Capability Shape

稳定 capability 至少应包含：

- `id`
- `serverId`
- `kind`
- `name`
- `displayName`
- `description`
- `category`
- `tags`
- `usageHint`
- `inputSummary`
- `outputSummary`
- `stageHints`
- `riskLevel`
- `schema` 或 `uri`
- `enabled`
- `approvalPolicy`
- `status`

### Catalog And Binding

- capability 目录应支持按 `kind`、server、category、stage、risk 筛选
- 目录应优先展示 `displayName`、说明、输入/输出摘要和适用阶段
- Skill 可声明 `requiredCapabilityRefs`
- Soul 可声明 `allowedCapabilityRefs`、`allowedCapabilityCategories`、`allowedMCPServerRefs`
- Workspace / Run activation 可记录 `selectedCapabilityRefs`

### Runtime Boundary

- `kernel/orchestrator` 只能基于 Run context、Skill/Soul/Workspace 策略和 capability 元数据形成候选集合
- `blocks/mcp` 只执行已经被策略允许并完成审批的 capability
- `soul.md` 文本不能隐式授予 capability 权限
- core runtime 不得按具体 tool/resource/prompt 名称做业务判断

### Observability

运行过程应优先展示：

- capability `displayName`
- 原始技术标识
- 所属 MCP server
- 用途与结果摘要

而不是只显示裸技术标识。

## Step Kind / Adapter Mapping

MCP capability 当前稳定承接以下 step kind：

| Step Kind | Adapter Block | 说明 |
| --- | --- | --- |
| `mcp/tool_call` | `blocks/mcp` | 执行 MCP tool 并返回结构化动作证据 |
| `mcp/resource_read` | `blocks/mcp` | 读取 MCP resource，默认属于只读证据 |
| `mcp/prompt_get` | `blocks/mcp` | 读取 MCP prompt / prompt template |

约束：

- `tool` / `resource` / `prompt` 的业务解释属于 capability 元数据和 profile 规则，不属于 core runtime。
- 是否允许某个 step kind 被当前 Run 选中，仍取决于 Soul / Skill / Workspace / activation 的结构化绑定。

## Read-Model And API Mapping

MCP catalog 和运行态展示应优先落到以下产品面：

- `GET /mcp/servers`
- `GET /mcp/servers/:serverId`
- `GET /mcp/servers/:serverId/tools`
- `GET /mcp/servers/:serverId/resources`
- `GET /mcp/servers/:serverId/prompts`
- `GET /mcp/capabilities`
- `GET /mcp/capabilities/:capabilityId`

运行态投影面应复用：

- `run snapshot`
  - 查看 selected capabilities / result / artifact
- `run observability`
  - 查看 used capabilities / tool evidence / closure trace
- `tool-calls` detail page
  - 查看具体 MCP 动作证据

## Governance And Approval Mapping

MCP capability 的审批边界应通过结构化字段进入治理层：

- `riskLevel`
- `approvalPolicy`
- `enabled`
- stage hints / category hints

稳定方向：

- catalog 元数据定义“该能力是否高风险”
- runtime 根据当前 contract 判断“本次是否需要 Approval”
- 审批结果最终投影到 `approvals`、`tool-calls`、`run observability`

## Verification Mapping

当前与 MCP capability 直接相关的验证入口包括：

- `make verify-mcp-stdio`
- `make verify-real-mcp-stdio`
- `make verify-real-weather`
- `make verify-weather-ticket-approval`
- `../../60-verification/service-acceptance.md`

这些入口分别验证：

- registry 与 deployed server 读写
- LLM graph 通过 MCP stdio 执行工具
- capability 风险边界与治理路径
- 运行证据和产品展示是否仍复用统一 read model

## Approval Matrix

| Capability Kind / Risk | 默认审批方向 | 说明 |
| --- | --- | --- |
| `resource_read` + low risk | 不需要 | 默认只读资源 |
| `prompt_get` + low risk | 不需要 | 提示词读取 |
| `tool_call` + low risk | 视 profile/policy 决定 | 常规工具调用，可自动执行 |
| `tool_call` + medium/high risk | 需要 | 由 `approvalPolicy`、`riskLevel`、stage hints 共同决定 |
| disabled capability | 禁止 | 不应进入候选集 |
| server 未允许 / Soul 未允许 / Skill 未声明 | 禁止 | 属于结构化能力边界拦截 |

## Product Examples

典型产品展示方向：

- capability catalog 页面
  - `displayName`、`kind`、`category`、`stageHints`、`riskLevel`、`approvalPolicy`
- MCP server detail
  - `tools/resources/prompts` 分组展示
- run snapshot / observability
  - `selectedCapabilities` / `usedCapabilities`
- tool-calls / approvals
  - 具体 capability 的执行证据和审批链

更具体的跨产品示例：

- Console capability catalog
  - 先按 `kind/category/stageHints/riskLevel/enabled` 过滤
- Run 详情页
  - 在 `selectedCapabilities` 中展示“本次候选集”
  - 在 `usedCapabilities` 中展示“本次真正执行过的 capability”
- Approval 抽屉/列表
  - 将 `approval.reason/action/context` 与 capability `displayName/riskLevel` 关联展示

## Read-Model / UI Field Matrix

| 产品面 | 关键字段 | 当前方向 |
| --- | --- | --- |
| capability catalog | `displayName` / `kind` / `category` / `stageHints` / `riskLevel` / `approvalPolicy` / `enabled` | 目录浏览与筛选 |
| MCP server detail | `server.id` / `name` / `transport` / `endpoint` / `defaultApprovalPolicy` / `sessionPolicy` | server 级配置基线 |
| tool/resource/prompt lists | `name` / `uri` / `serverId` | server 暴露资源清单 |
| run snapshot | `selectedCapabilities` / `usedCapabilities` / `results` / `artifacts` | capability 选择与结果聚合 |
| run observability | `usedCapabilities` / `tool-calls` / `closureTraces` | capability 使用时间线 |
| approvals | `reason` / `action` / `context` / `state` | capability 风险审批链 |

## Activation Contract Field Mapping

MCP capability 在 activation 阶段的关键字段映射应优先围绕：

| Activation / Context 字段 | 来源 | 当前方向 |
| --- | --- | --- |
| `mcpServerIds` | Workspace / Soul / Skill / activation request | 当前 run 允许接入的 MCP server |
| `capabilityIds` | Workspace / Soul / Skill / activation request | 当前 run 候选 capability 集 |
| `selectedCapabilities` | executionContext / read model projection | 被最终选中的 capability |
| `usedCapabilities` | runtime evidence projection | 实际被调用过的 capability |
| `soulBindings` / `soulRouting` | Soul 解析结果 | capability allowlist 的最终校验边界 |

约束：

- `selectedCapabilities` 不等于 `usedCapabilities`
- `capabilityIds` 只是 activation 输入面，最终仍需经过 Soul/Skill/policy 校验

## Cross-Layer Mapping

- kernel registry / policy：
  `../../30-module-architecture/kernel-context-capability.md`
- adapter 分发：
  `../../30-module-architecture/blocks-runtime-adapters.md`
- API 契约：
  `../../50-product-interface/service-api.md`
- 服务级验收：
  `../../60-verification/service-acceptance.md`

## Source Documents

- `../../50-product-interface/service-api.md`
