# MCP 能力组件

## 功能说明

`blocks/mcp` 调用已注册的 MCP tool、resource 和 prompt，并把外部调用结果规范化为 runtime execution result。

## 组件交互及合同

- `kernel/orchestrator` 通过 Orchestrator 定义的 `ExecuteRuntimeOperationCommand/Result` 同步投递 tool_call、resource_read 或 prompt_get；本组件直接订阅 EventHub，不再启动通用 runtime executor 或发布 V1 执行通知。
- Runtime V2 capability preflight 通过 Orchestrator 定义的 typed EventHub command 查询本 block 实际提供的 tool、resource 和 prompt kinds。
- Runtime V2 MCP operation 不会通过 Envelope 或 `map[string]any` 传递跨组件结果。
- 本 block 导入 `blocks/mcpregistry/pkg/events`，通过 EventHub 查询 MCP Server 和 Capability；不注入 registry service/repository。
- request 必须携带 workspace/run/step scope、server/capability ref 和结构化参数；result 返回状态、source refs、摘要、truncation metadata、错误和 raw ref。

## 相关约束

- 未注册或不匹配的 Server/Capability 不得调用。
- 参数和输出必须有界，并受 runtime policy 与 approval 准入约束。
- credential 和 secret 只能使用安全引用，不得进入事件或日志。
- MCP 输出只作为 context/evidence candidate，不能直接创建 accepted Result。
