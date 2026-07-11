# Workspace Stateful Block

## 功能定位

`blocks/workspace` 是 Workspace、Goal、DoD、WorkspaceTemplate、definition revision 与 workspace binding 的单一资源聚合 block。它可以管理这组 workspace-definition 正式状态，但不组合其他组件来完成跨组件编排。

## 拥有边界

- 拥有 Workspace、Goal、DoD、WorkspaceTemplate 的 owner command/query 处理。
- 负责维护 workspace 当前定义、definition revision、lineage 与状态变更事件。
- 只通过自身 repository 管理 workspace-definition 聚合状态，不持有其他 module/block 的 service、repository 或 adapter。
- 不负责 Run 执行、Result 接受、runtime repair 或 block 调度。

## 主要交互

- 被 `kernel/run` 在激活 Run 时读取最新 workspace definition。
- 被 `kernel/orchestrator` 读取 Goal/DoD 快照以构建 runtime stage 与 coverage 合同。
- 向 `kernel/eventstream` 发布 workspace/goal/dod 业务事件。
- 被 `application/workorchd` 通过 EventHub owner ports 读写，并投影到 Web/TUI/API。
- 订阅 System 定义的 `/system/command/ensure-workspace-template-defaults`，在 WorkspaceTemplate owner 内幂等加载内置模板。

## 组件交互合同

- Command contract：调用方导入 `blocks/workspace/pkg/events`，通过 magicCommon EventHub 向 `blocks/workspace` 投递具体 Workspace、Goal、DoD、WorkspaceTemplate command，并接收 `WorkspaceCommandResult`。
- Input contract：Workspace、Goal、DoD 命令必须携带 workspace scope 或明确创建上下文；禁止通过 service/repository 旁路写入。
- Output contract：Workspace created/state-changed/deleted 使用 `WorkspaceChangedData`，Goal/DoD 定义变化分别使用 `GoalDefinedData` / `DoDDefinedData`；必须携带可追溯 definition revision 与 lineage 字段。
- Run activation contract：`kernel/run` 激活时只能通过 EventHub 使用 workspace block 提供的最新定义快照。
- Projection contract：`application/workorchd` 只能消费 command/query result 或正式事件，不直接拼装定义权威。
- Event contract：workspace/goal/dod 通知直接通过组件 EventHub 投递到 `/workspace/consumers/#`，消费方导入 `blocks/workspace/pkg/events` 的具体 data；禁止 Envelope、map/any 或 JSON/reflect 转换桥。
- System defaults contract：只消费 `kernel/system/pkg/events` 定义的 `EnsureWorkspaceTemplateDefaultsCommand`，并返回 `EnsureWorkspaceTemplateDefaultsResult`；System 不取得 WorkspaceTemplate repository。

## 已知缺口

- 当前盘点未发现 Workspace block 职责、状态所有权或 definition-change 交互合同缺失。
- definition 变更重新打开 terminal/superseded workspace 时会清理失效 `ActiveRunID`；workspace-state event 与 definition event 的发布数量已有回归测试约束，后续修改时必须保持该 owner invariant。
- Workspace owner 的生产通知路径已完成 typed 收口；`kernel/eventstream` 按 topic 消费具体 data，并仅在内部审计投影边界恢复 payload。

## 边界约束

- Goal/DoD 变更不得静默弱化历史约束。
- definition 变更导致 workspace 重新进入 active 时，不能保留过期 active run 作为当前权威。
- 不直接启动 runtime，也不决定 Run 完成或失败。
