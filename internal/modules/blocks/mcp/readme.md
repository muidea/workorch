# MCP 能力组件

## 功能说明

`blocks/mcp` 调用已注册的 MCP tool、resource 和 prompt，并把外部调用结果规范化为 runtime execution result。

## 组件交互及合同

- `kernel/orchestrator` 通过 Orchestrator 定义的 `ExecuteRuntimeOperationCommand/Result` 同步投递 tool_call、resource_read 或 prompt_get；本组件直接订阅 EventHub，不再启动通用 runtime executor 或发布 禁止的 执行通知。
- Runtime capability preflight 通过 Orchestrator 定义的 typed EventHub command 查询本 block 实际提供的 tool、resource 和 prompt kinds。
- Runtime MCP operation 不会通过 Envelope 或 `map[string]any` 传递跨组件结果。
- 本 block 导入 `blocks/mcpregistry/pkg/events`，通过 EventHub 查询 MCP Server 和 Capability；不注入 registry service/repository。
- request 必须携带 workspace/run/authority/dispatch/operation identity、sealed server/capability ref 和结构化参数；
  result 以 `RuntimeOperationReceipt.Outcome` 为唯一结果，并返回 source refs、bounded summary、truncation
  metadata、owner stable factual category、完整安全 original diagnostic 和 evidence/ref，不返回跨组件 Status 双权威。

## 当前内聚收口切片（2026-08-16）

- `RuntimeAdapter` 仍是唯一 MCP capability adapter；原 1,389 行混合文件已按 core ingress/catalog、HTTP transport、
  short/long-lived stdio operation、long-lived session manager、stdio process session、JSON-RPC codec、operation
  contract 与 result normalization 分成同 package 的 owner-private 文件。没有新增 Block、registry facade、
  EventHub round trip、receipt owner、通用 capability executor 或第二 session authority。
- MCP Registry 仍通过 typed EventHub query 独立拥有 Server/Capability；MCP adapter 只保存原有一个 `sessionMu` 和
  一个 workspace/server-keyed `sessions` map。短生命周期 stdio 每次关闭自身 session；长生命周期 stdio 继续由
  同一 manager 做 create-or-reuse、idle eviction、failure eviction 和 adapter close。
- HTTP/stdio 都继续消费同一 operation contract、JSON argument codec、bounded response parser、tool `isError`
  detection 与 result normalization；transport helper 不解释 effect scope、coverage、retry、governance 或 terminal。
- C22-05 MCP 切片完成后，非测试 Go 为 24 个文件/2,799 LOC，测试为 9 个文件/2,183 LOC，package 保持 4；
  最大生产文件从 `mcp_runtime_adapter.go` 1,389 行降为 `owner.go` 776 行，至少 500 行生产文件从 2 个降为 1 个，
  生产 `CC>30` 继续为 0。`BenchmarkRuntimeAdapterHTTPToolCall` 的 50 次、3 轮样本为
  `317,444～349,474 ns/op`、`12,375～12,694 B/op`、`112～113 allocs/op`，计数器确认每次 operation
  恰好一次 HTTP transport call。

## 相关约束

- 未注册或不匹配的 Server/Capability 不得调用。
- 参数和输出必须有界，并受 runtime policy 与 approval 准入约束。
- credential 和 secret 只能使用安全引用，不得进入事件或日志。
- MCP 输出只作为 context/evidence candidate，不能直接创建 accepted Result。
- 对 `mcp/tool_call`，Orchestrator 仅依据 activation 已密封的 exact capability `effectScope` 判定 canonical state
  相容性；本 block 不得以 risk label 或 tool 名称补推断为 read-only/session/durable。
- 本 owner 私有 code 只在 MCP Biz 边界映射为 factual category；不得选择 correction、retry、governance、terminal
  或修改 Runtime aggregate。unknown code 保留原 code/message 并 typed fail-fast。
- CWI reconciliation 将 endpoint/catalog/tool-surface readiness 与 behavior proof 分离；没有 binding 声明的可执行
  tool probe 和完整输入约束时返回 `unverifiable/behavior_probe_unavailable`，连接成功或 tool description 不能关闭 CWI。
