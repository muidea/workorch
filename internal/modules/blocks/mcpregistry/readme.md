# MCP Registry Stateful Block

## 功能定位

`blocks/mcp-registry` 是 MCPServer 与 MCPCapability registry 的单一资源聚合 block。代码路径使用 `internal/modules/blocks/mcpregistry`，用于与 invocation 技术 block `blocks/mcp` 明确区分。

## 拥有边界

- 拥有 MCPServer 与 MCPCapability owner command/query。
- 只访问自身 MCPServer/MCPCapability repositories，不持有其他 module/block 的 service、repository 或 invocation adapter。
- 维护 capability kind、schema、refs 与 selection metadata。
- 不执行 MCP 调用，也不决定 runtime completion。

## 主要交互

- 为 `kernel/orchestrator` 和 LLM context 提供 declared capability catalog。
- 为 `blocks/mcp` 提供可调用能力的结构化声明来源。
- 被 `application/workorchd` context registry API 读写。
- 与 `kernel/eventstream` 协作发布 MCP registry 事件。
- 订阅 System 定义的 `/system/command/ensure-mcp-defaults`，在 MCP registry owner 内幂等加载内置 server/capability。

## 组件交互合同

- Command contract：调用方导入 `blocks/mcpregistry/pkg/events`，通过 magicCommon EventHub 投递具体 server/capability command/query，并接收 `MCPCommandResult`。
- Input contract：server/capability command 必须使用结构化 serverId/capabilityId/kind/schema 字段。
- Output contract：registry query result 返回 declared capability catalog、refs 和 bounded metadata。
- MCP block contract：`blocks/mcp` 只通过 EventHub 查询 registry 中存在且 policy 准入的 capability，不直接调用 registry 实现。
- Orchestrator contract：orchestrator 只消费 capability kind/schema/ref，不按 display text 分支。
- Application contract：context registry API 只能通过 EventHub 修改 registry metadata，不修改 runtime owner state。
- System defaults contract：只消费 `kernel/system/pkg/events` 定义的 `EnsureMCPDefaultsCommand` 并返回具体 result；System 不取得 MCP repositories。

## 已知缺口

- 当前盘点未发现 MCP Registry block 职责、状态所有权或 EventHub 合同缺失；server/capability schema 与 invocation block 的 pairwise 测试应持续补齐。

## 边界约束

- capability metadata 不能编码 runtime 状态决策。
- runtime core 只消费结构化 kind/schema/ref，不按业务含义分支。
- 未声明 capability 不能被 MCP block 作为捷径调用。
