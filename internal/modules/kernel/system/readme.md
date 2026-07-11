# System Module

## 功能定位

`kernel/system` 是内置默认资源的启动协调 module。它组合 Soul catalog、WorkspaceTemplate catalog 与 MCP registry 三个独立组件合同，在各 owner 完成订阅后通过 EventHub 触发幂等 ensure。

## 拥有边界

- 不拥有 Soul、WorkspaceTemplate、MCPServer、MCPCapability repository 或正式状态。
- 只管理“本次启动默认资源协调是否完成”的瞬时流程，不保存跨 owner 状态副本。
- 不处理 runtime completion、governance、evidence 或健康状态查询。

## 主要交互

- 向 `blocks/soul` 投递 `/system/command/ensure-soul-defaults`。
- 向 `blocks/workspace` 投递 `/system/command/ensure-workspace-template-defaults`。
- 向 `blocks/mcp-registry` 投递 `/system/command/ensure-mcp-defaults`。
- 各消费组件只访问自己的 repository，并同步返回 System 定义的具体 result。

## 组件交互合同

- Event ownership contract: 三组 command/result 均由 `kernel/system/pkg/events` 定义，消费组件导入并订阅。
- Lifecycle contract: Soul、Workspace、MCP 必须先完成订阅，System 再执行同步 ensure；因此 System lifecycle weight 位于三个 owner 之后、Orchestrator 之前。
- Persistence contract: System 不接收 repository、service、adapter 或 writer callback；默认数据的加载/upsert 发生在消费 owner 内部。
- Failure contract: 任一 owner ensure 失败时 System 启动失败，不能将部分初始化视为完整成功。

## 已知缺口

- X03 多 owner repository 直连已关闭。
- C15 最终命名与入口复核已完成：组件保持 `kernel/system`，由 `application/workorch/main.go` 显式启用；三个被协调的 stateful block 均在其之前完成订阅。
- 当前未发现功能职责、状态所有权或 EventHub 合同缺失。

## 边界约束

- 不得承载业务完成判断或 owner 状态查询。
- 不得恢复 Soul/Template/MCP repository 注入，也不得变成跨 owner service registry。
- 新增默认资源类型时必须新增投递方具体事件合同，并由对应 owner 消费。
