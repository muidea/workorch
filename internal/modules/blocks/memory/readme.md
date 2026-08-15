# Memory Stateful Block

## 功能说明

`blocks/memory` 管理 MemoryItem 与 MemoryRecallTrace 聚合，负责记忆的创建、查询、搜索、更新、删除以及 recall 选择轨迹的持久化和查询。

## 组件交互及合同

- MemoryItem 与 RecallTrace command/query 由 `blocks/memory/pkg/events` 定义，并返回 `MemoryCommandResult`。
- `MemorySearchQuery` 与 `MemoryRecallTraceQuery` 是 owner event DTO；module 内部才映射到 repository query，并 clone vector/tags/types 等可变 slice。repository type 不得进入 event contract。
- MemoryItem create/update/delete 成功后由本 block 单点发布 `MemoryChangedData`。
- RecallTrace upsert 成功后由本 block单点发布 `MemoryRecallRecordedData`。
- `application/workorchd`、`kernel/orchestrator` 和 `kernel/governance` 通过 EventHub 查询、搜索或写入 Memory；LLM 只消费 Orchestrator 组织后的有界记忆上下文。
- Eventstream 和 projection 按需订阅 Memory 的具体通知。

## Owner 内部职责边界

- `owner.go` 只保留 Memory Biz 的构造、订阅与生命周期；`command_router.go` 是唯一 command ingress，并静态穷尽地区分 MemoryItem 与 MemoryRecallTrace 两个命令族，未知 Topic fail closed。
- `item_commands.go` 内聚 item CRUD/search，`recall_trace_commands.go` 内聚 recall trace upsert/query/delete；两者仍共享同一 Memory Biz、repository group、workspace context fence 和 notification authority。
- `memory_notification.go` 只在对应 owner repository 操作成功后发布 item lifecycle 或 recall recorded fact，不允许外围组件补发、重建或代写 Memory 状态。

## 相关约束

- 所有 MemoryItem 和 RecallTrace 操作必须携带 workspace scope；delete 前必须读取对象并校验 WorkspaceID。
- 本 block 只访问 MemoryItem 与 MemoryRecallTrace repository，不得直接访问 Run、Workspace、Result 或 LLM 实现。
- 记忆内容进入 LLM context 前必须经过相关性、价值、来源和 context budget 筛选。
- 外围组件不得重复发布 Memory lifecycle 或 recall 通知。
