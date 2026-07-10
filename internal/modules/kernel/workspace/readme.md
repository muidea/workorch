# Workspace Module

## 功能定位

`kernel/workspace` 是 Workspace、Goal、DoD、workspace definition revision 与 workspace binding 的 owner module。它负责工作空间定义的创建、更新、删除、模板关联、Goal/DoD 定义与对应业务事件发布。

## 拥有边界

- 拥有 Workspace、Goal、DoD、WorkspaceTemplate 的 owner command/query 处理。
- 负责维护 workspace 当前定义、definition revision、lineage 与状态变更事件。
- 不负责 Run 执行、Result 接受、runtime repair 或 block 调度。

## 主要交互

- 被 `kernel/run` 在激活 Run 时读取最新 workspace definition。
- 被 `kernel/orchestrator` 读取 Goal/DoD 快照以构建 runtime stage 与 coverage 合同。
- 向 `kernel/eventstream` 发布 workspace/goal/dod 业务事件。
- 被 `application/workorchd` 通过 EventHub owner ports 读写，并投影到 Web/TUI/API。

## 组件交互合同

- Input contract: Workspace、Goal、DoD 命令必须携带 workspace scope 或明确创建上下文。
- Output contract: Goal/DoD 变更必须产生可追溯 definition revision、lineage 字段和正式业务事件。
- Run activation contract: `kernel/run` 激活时只能使用 workspace owner 提供的最新定义快照。
- Projection contract: `application/workorchd` 只能消费 owner command/query 响应或正式事件，不直接拼装定义权威。
- Event contract: workspace/goal/dod 事件必须携带 source module、subject、workspaceId 和定义 payload。

## 已知缺口

- 当前盘点未发现 Workspace module 职责或 definition-change 交互合同缺失。
- definition 变更重新打开 terminal/superseded workspace 时会清理失效 `ActiveRunID`；workspace-state event 与 definition event 的发布数量已有回归测试约束，后续修改时必须保持该 owner invariant。

## 边界约束

- Goal/DoD 变更不得静默弱化历史约束。
- definition 变更导致 workspace 重新进入 active 时，不能保留过期 active run 作为当前权威。
- 不直接启动 runtime，也不决定 Run 完成或失败。
