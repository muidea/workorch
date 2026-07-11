# MCP Block

## 功能定位

`blocks/mcp` 是 runtime MCP invocation block。它负责调用已声明的 MCP tool、resource、prompt，并把调用结果规范化为 runtime execution result。

## 拥有边界

- 拥有 MCP invocation adapter 与 runtimeevent executor。
- 只执行对已声明 MCP capability 的结构化调用。
- 不拥有 MCP registry 状态，不决定 DoD 满足或 Result 接受。

## 主要交互

- 从 `kernel/orchestrator` 接收 `mcp/tool_call`、`mcp/resource_read`、`mcp/prompt_get` execution request。
- 依赖 `blocks/mcp-registry` 提供的 MCP server/capability registry。
- 调用结果被 `kernel/orchestrator` 分类为 evidence candidate、diagnostic 或 blocker。
- 可与 `blocks/approval` 间接协作处理高风险外部能力调用。

## 组件交互合同

- Input contract: request 必须引用 registry 中声明的 server/capability，并携带结构化 params 与 run/step scope。
- Output contract: 返回 normalized MCP result/error、source refs、raw ref 和 truncation metadata。
- Registry contract: `blocks/mcp-registry` 是 capability 权威；block 不自行创建或猜测 capability。
- Orchestrator contract: MCP output 只作为 evidence candidate 或 context material，accepted 由 orchestrator 判定。
- Approval contract: 高风险工具调用必须在执行前准入，或返回 structured approval blocker。
- EventHub contract: 直接订阅 `/runtime/block` 下 tool_call/resource_read/prompt_get topic 的 `runtimeevent.ExecuteCommand`，返回 `ExecuteCommandResult`，并向 `/runtime/orchestrator` 投递具体的 accepted/progress/completed/failed data；顶层载荷禁止使用 Envelope/map/JSON 转换。

## 已知缺口

- 当前盘点未发现 MCP block 职责缺失；registry capability schema 与 invocation params 的 pairwise 测试需要持续扩展。
- 当前未发现与 orchestrator 的 runtime execute 交互合同缺失。
- MCP registry 查询由 block 内部 EventHub getter 完成，只导入 `blocks/mcp-registry/pkg/events` 与其公开模型，不注入 registry service/repository。

## 边界约束

- 未注册 capability 不能被调用。
- 参数必须结构化、bounded，并受 runtime policy 约束。
- MCP 输出本身不是 accepted Result。
