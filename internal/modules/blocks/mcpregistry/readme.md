# MCP Registry Stateful Block

## 功能说明

`blocks/mcpregistry` 管理 MCPServer 与 MCPCapability registry 聚合，负责 server/capability 的创建、查询、列表、更新和结构化能力元数据。

## 组件交互及合同

- MCP Server 与 Capability command/query 由 `blocks/mcpregistry/pkg/events` 定义，并返回 `MCPCommandResult`。
- `blocks/mcp` 通过 EventHub 查询已注册 server/capability 后执行调用，不注入 registry service 或 repository。
- `kernel/orchestrator` 和 `application/workorchd` 通过 EventHub 获取有界 capability metadata 和 server refs。
- `kernel/system` 发送 `EnsureMCPDefaultsCommand`；本 block 在自身 repository 内幂等初始化默认 server/capability。

## 相关约束

- 本 block 只访问 MCPServer 与 MCPCapability repository，不执行 MCP tool/resource/prompt 调用。
- Capability 必须绑定存在的 Server，并保持 ID、kind、schema、risk 和 provenance 一致。
- secret 和 credential 只能使用安全引用，不得进入事件、投影或日志。
- 未注册或无效 capability 不能被 runtime invocation 使用。
