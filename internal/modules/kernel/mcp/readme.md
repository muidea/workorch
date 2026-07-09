# MCP Registry Module

## 功能定位

`kernel/mcp` 是 MCPServer 与 MCPCapability registry 的 owner module。它负责 MCP server、tool/resource/prompt capability 的注册、查询、更新和运行期能力元数据。

## 拥有边界

- 拥有 MCPServer 与 MCPCapability owner command/query。
- 维护 capability kind、schema、refs 与 selection metadata。
- 不执行 MCP 调用，也不决定 runtime completion。

## 主要交互

- 为 `kernel/orchestrator` 和 LLM context 提供 declared capability catalog。
- 为 `blocks/mcp` 提供可调用能力的结构化声明来源。
- 被 `application/workorchd` context registry API 读写。
- 与 `kernel/eventstream` 协作发布 MCP registry 事件。

## 组件交互合同

- Input contract: server/capability command 必须使用结构化 serverId/capabilityId/kind/schema 字段。
- Output contract: registry 查询返回 declared capability catalog、refs 和 bounded metadata。
- MCP block contract: `blocks/mcp` 只能调用 registry 中存在且 policy 准入的 capability。
- Orchestrator contract: orchestrator 只消费 capability kind/schema/ref，不按 display text 分支。
- Application contract: context registry API 只能修改 registry metadata，不修改 runtime owner state。

## 已知缺口

- 当前盘点未发现 MCP Registry module 职责缺失；server/capability schema 与 block invocation 的 pairwise 测试应持续补齐。

## 边界约束

- capability metadata 不能编码 runtime 状态决策。
- runtime core 只消费结构化 kind/schema/ref，不按业务含义分支。
- 未声明 capability 不能被 MCP block 作为捷径调用。
