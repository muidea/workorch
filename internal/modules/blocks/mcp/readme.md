# MCP 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-16

## 功能说明

`blocks/mcp` 调用已注册的 MCP tool、resource 和 prompt，并把外部调用结果规范化为 runtime execution result。

## 组件交互及合同

- `kernel/orchestrator` 通过 MCP execution owner 在 `blocks/mcp/pkg/events` 定义的 `ExecuteOperationCommand/Result`
  同步投递 tool_call、resource_read 或 prompt_get；本组件直接订阅 EventHub，不启动通用 runtime executor 或第二套执行通知。
- Runtime capability、operation-admissibility、operation-feasibility、semantic-proof feasibility 与 CWI reconciliation
  均使用 MCP execution owner 定义的 concrete typed command/result。
- Runtime MCP operation 不会通过 Envelope 或 `map[string]any` 传递跨组件结果。
- 本 block 导入 `blocks/mcpregistry/pkg/events`，通过 EventHub 查询 MCP Server 和 Capability；不注入 registry service/repository。
- operation-admissibility 在一次 registry snapshot 中同时取得 enabled server 与 tool capability，并由本 execution owner
  编译 `RuntimeValidationSurface`。完整 capability 仍是 callable inventory；只有 non-durable、`safe_to_replay`、
  schema 合法、已有 owner-sealed input binding，且 registry owner 显式声明 `direct` proof applicability 的 surface 才进入 provider-visible
  `ValidationRecipeRefs`。catalog acquisition 不打开 MCP transport。
- request 必须携带 workspace/run/authority/dispatch/operation identity、sealed server/capability ref 和结构化参数；
  result 以 `RuntimeOperationReceipt.Outcome` 为唯一结果，并返回 source refs、bounded summary、truncation
  metadata、owner stable factual category、完整安全 original diagnostic 和 evidence/ref，不返回跨组件 Status 双权威。

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
- 无 required input 的安全 validation tool 使用 owner 生成的 empty-input binding，技术 readiness 为 `ready`；只有 proof
  applicability 同时为 `direct` 才能进入 semantic provider。owner 声明为 `auxiliary` 的 health/status surface 即使
  empty-input ready 也只能用于审计/诊断，不能替代直接 DoD 行为证明。schema 含 required input 时保持 tool 可调用，但 validation surface 为
  `prerequisite_required/input_binding_required`，不得出现在 semantic provider schema/C3，也不得用空参数调用 endpoint。
  durable/unsafe replay 或非法 schema 的 surface 为 `unavailable`。这些分类只返回事实，不选择 retry、focus、terminal 或替代 tool。
