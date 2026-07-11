# Memory Stateful Block

## 功能说明

`blocks/memory` 管理 MemoryItem 与 MemoryRecallTrace 聚合，负责记忆的创建、查询、搜索、更新、删除以及 recall 选择轨迹的持久化和查询。

## 组件交互及合同

- MemoryItem 与 RecallTrace command/query 由 `blocks/memory/pkg/events` 定义，并返回 `MemoryCommandResult`。
- MemoryItem create/update/delete 成功后由本 block 单点发布 `MemoryChangedData`。
- RecallTrace upsert 成功后由本 block单点发布 `MemoryRecallRecordedData`。
- `application/workorchd`、`kernel/orchestrator` 和 `kernel/governance` 通过 EventHub 查询、搜索或写入 Memory；LLM 只消费 Orchestrator 组织后的有界记忆上下文。
- Eventstream 和 projection 按需订阅 Memory 的具体通知。

## 相关约束

- 所有 MemoryItem 和 RecallTrace 操作必须携带 workspace scope；delete 前必须读取对象并校验 WorkspaceID。
- 本 block 只访问 MemoryItem 与 MemoryRecallTrace repository，不得直接访问 Run、Workspace、Result 或 LLM 实现。
- 记忆内容进入 LLM context 前必须经过相关性、价值、来源和 context budget 筛选。
- 外围组件不得重复发布 Memory lifecycle 或 recall 通知。
