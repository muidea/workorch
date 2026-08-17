# MCP Registry Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-17

## 功能范围与边界

`blocks/mcpregistry` 管理 MCPServer 与 MCPCapability registry 聚合，负责 server/capability 的创建、查询、列表、更新和结构化能力元数据。

## 组件交互及合同

- MCP Server 与 Capability command/query 由 `blocks/mcpregistry/pkg/events` 定义，并返回 `MCPCommandResult`。
  Capability create/update command 只携带字段化配置意图，不接收完整 registry entity；lifecycle `Status` 由
  MCPRegistry owner 创建时派生、更新时保留，调用方不能指定。
- `AllowedTools`、`AllowedResources` 与 `AllowedPrompts` 只派生通用的 owner capability 默认值（工具、资源、提示词
  加原始名称）。领域显示名、分类、标签、阶段提示和用途只能来自显式 capability 配置或上层 template/profile 声明；
  Registry 不根据 tool/resource/prompt 名称推断业务语义。
- MCP Server 与 Capability query/result 使用 owner-local detached DTO；Capability Result 复用该 owner 的
  canonical `pkg/models.Capability` 值，不再创建同构 alias。所有 slice、map/payload 在 EventHub 边界显式复制，
  不向调用方泄漏 repository model 或共享可变引用；handler 必须使用 `CloneCapability/CloneCapabilities`。
- Event Contract owner、receiver 和 state lane identity 统一为 `mcp-registry`；它与执行 MCP operation 的
  `blocks/mcp` / owner `mcp` 严格分离，不共享 owner identity。
- `blocks/mcp` 通过 EventHub 查询已注册 server/capability 后执行调用，不注入 registry service 或 repository。
- `application/workorchd` 通过 EventHub 获取有界 capability metadata 和 server refs，并在 fresh activation 中封存选择结果；`kernel/orchestrator` 不直接查询 Registry。
- `kernel/system` 发送 `EnsureMCPDefaultsCommand`；本 block 在自身 repository 内幂等初始化默认 server/capability。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `application/workorchd` | Application → Registry | Server/Capability create/get/list/update | 完整 |
| `blocks/mcp` | MCP execution → Registry | detached Server/Capability get/list snapshot | 完整；执行 block 不取得 repository |
| `kernel/system` | System → Registry | `EnsureMCPDefaultsCommand` 及具体 result | 完整 |
| `kernel/orchestrator` | 无直接 Registry I/O | 只消费 activation selection 和 MCP execution owner facts | 完整 |

## 相关约束

- 本 block 只访问 MCPServer 与 MCPCapability repository，不执行 MCP tool/resource/prompt 调用。
- Capability 必须绑定存在的 Server，并保持 ID、kind、schema、risk 和 provenance 一致。
- 可执行 tool capability 的 `effectScope` 与 `idempotencyClass` 是本 block owner 明确维护的运行时元数据；缺失值在 activation 中只能密封为 `unknown`。`riskLevel`、tool 名称（包括初始化名称）不得被用来推断 effect 或 replay 安全性。
- schema、stage hints、effect、replay class 和 `direct | auxiliary | not_declared` validation proof applicability 的持久化定义归本 owner；
  proof applicability 不得从 tool 名称、描述、category、risk 或 DoD 推断。但 `ready | prerequisite_required | unavailable`
  validation readiness 由执行该调用的 `blocks/mcp` 基于同一次 detached registry snapshot 编译。Registry 不执行 probe、
  不生成 input binding、不修改 Runtime aggregate，也不选择 provider-visible recipe。
- secret 和 credential 只能使用安全引用，不得进入事件、投影或日志。
- 未注册或无效 capability 不能被 runtime invocation 使用。

## 待完善测试点

- [ ] 覆盖 Capability 绑定不存在、禁用或跨 Server 的场景，确认 create/update 均 fail-closed。
- [ ] 覆盖 `effectScope`、`idempotencyClass`、`validationProofApplicability` 的往返持久化与未知值拒绝。
- [ ] 覆盖所有 detached DTO 的深拷贝，调用方修改 schema、tags、stage hints 后不得污染 Registry。
- [ ] 覆盖 defaults ensure 不覆盖用户配置，以及 Server 默认派生能力不推断领域语义或 direct proof。
- [ ] 增加边界测试，确认 Registry 不打开 MCP transport、不生成 input binding，也不向 Orchestrator 暴露 repository。
