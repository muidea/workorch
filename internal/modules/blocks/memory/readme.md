# Memory Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-21

## 功能范围与所有权

`blocks/memory` 是 MemoryItem 与 MemoryRecallTrace 的唯一资源 owner，负责 item CRUD/search 以及 recall 轨迹的不可变记录、读取、分页和删除。它不拥有 SM-1～SM-4 状态，不选择 Runtime action，也不能用 recall 事实关闭 coverage。

Module 根只桥接生命周期；Biz 嵌入共享 Base，并且只持有本 owner 的 MemoryItem、MemoryRecallTrace repository 与 EventHub。Configure 必须同时取得两类 repository，缺少 RecallTrace repository 时即 fail closed，不允许启动后到首次命令才暴露依赖缺失。

## EventHub 合同

十个 command topic 各自使用具体 result，不存在跨命令的可空字段联合 envelope：

| 命令族 | command → result |
| --- | --- |
| MemoryItem 写 | `CreateMemoryCommand → CreateMemoryResult`、`UpdateMemoryCommand → UpdateMemoryResult`、`DeleteMemoryCommand → DeleteMemoryResult` |
| MemoryItem 读 | `GetMemoryCommand → GetMemoryResult`、`ListMemoryByWorkspaceCommand → ListMemoryByWorkspaceResult`、`SearchMemoryCommand → SearchMemoryResult` |
| RecallTrace 写 | `UpsertMemoryRecallTraceCommand → UpsertMemoryRecallTraceResult`、`DeleteMemoryRecallTraceCommand → DeleteMemoryRecallTraceResult` |
| RecallTrace 读 | `GetMemoryRecallTraceCommand → GetMemoryRecallTraceResult`、`ListMemoryRecallTracesCommand → ListMemoryRecallTracesResult` |

`MemorySearchQuery` 与 `MemoryRecallTraceQuery` 是 owner-neutral query value，由具体 Memory command 承载；repository type 不进入 EventHub 合同。Owner 会归一化并复制 query，搜索的 tags/types/vector 以及所有 result/model 可变集合都在边界深拷贝。

## Scope、查询与重放语义

- 每个 item/trace command 都必须显式携带 WorkspaceID。唯一 command ingress 使用 checked binder 将其绑定到 EventHub context；已有不同 workspace context 返回 conflict，不能被覆盖。
- get/update/delete 在操作前读取并核对对象归属。空 scope 和错误 workspace 均 fail closed；相同局部 ID 可由 memory/file 的 workspace-scoped repository 在不同 Workspace 独立解析。
- item workspace list 默认 100、单页最多 200；search 默认 20、最多 100；trace list 默认 100、单页最多 200。item 与 trace list 都返回 `NextCursor`，批量 purge/recovery 必须逐页读取并检测游标循环/页数上限。
- search 拒绝非有限 minScore/vector、超长 vector 和未知 memory type；create/update 同样拒绝非有限或超过 16384 维的 vector。
- RecallTrace 的公开“upsert”语义实际是 create-once：首次写入返回 `recorded`；相同 scope、TraceID 与完整持久化内容的重放零写入并返回 `duplicate`；任一内容漂移返回 conflict，不能改写历史 recall 证据。

## Runtime recall 证据链

Application activation 先读取最多 200 个候选，再按稳定顺序应用以下确定性选择合同：最终最多 8 项、最多 16 KiB、估算最多 4096 token，并执行 owner-neutral 的 memory type quota 与可见性规则。它把 candidate、selected、excluded、type quota、逐项 score/reason、retrieved hit、byte/token budget 以及 semantic fallback 证据封入 `RuntimeActivationContext.MemorySelection`。

这份 evidence 只描述上下文选择，不是 SM-2 authority。Run owner 在创建 Run 后使用稳定的 Run CreatedAt 将同一 sealed evidence 交给 Memory owner；若进程在 Run 提交与该交接之间崩溃，Run 启动扫描会从自身持久化 bootstrap 重放同一 create-once fact。Memory 持久化完整轨迹，projection 与 Eventstream 都可读取该轨迹。Orchestrator/LLM 只能消费已经封存的有界 Runtime memory context，不能从 Memory 反向重建 authority 或选择下一动作。

## 通知与交互边界

- MemoryItem create/update/delete 仅在 owner repository 成功后发布对应 `MemoryChangedData`。
- RecallTrace 仅在首次 `recorded` 后发布一次 `MemoryRecallRecordedData`；exact duplicate 不再次发布，冲突重放不写入也不发布。activation 通知携带完整 bounded selection evidence，Eventstream 以 typed payload 保存其可解释轨迹。
- repository 中的 MemoryRecallTrace 是权威记录。通知是提交后的 projection/audit trigger，不是持久化 ack 或跨 owner transaction；当前合同不承诺 outbox 式补发。
- `application/workorchd`、`blocks/run`、`kernel/orchestrator` 与 `kernel/governance` 只通过上述具体 EventHub command/result 访问 Memory，不取得 Memory repository/service。Eventstream/projection 只消费通知或执行 scoped read，不拥有 Memory 状态。

## 交互合同结论

| 交互组件 | 方向 | 合同结论 |
| --- | --- | --- |
| `application/workorchd` | Application → Memory | scoped item read/search/delete 与 trace query；候选、排除、quota、score、budget 不再在 activation bridge 丢失 |
| `blocks/run` | Run → Memory | stable create-once recall record；duplicate 零写入、零重复通知，conflicting replay fail closed |
| `kernel/orchestrator` / `kernel/governance` | consumer → Memory | workspace context 必填；读取使用具体 bounded result，Orchestrator 的全量恢复/清理逐页且有 hard guard |
| Eventstream / projection | Memory → consumer | 首次提交后的 typed fact 保留完整 selection audit；正式状态仍以 Memory repository 为准 |

## 回归基线

当前测试覆盖：

- Configure 缺少任一 repository、重复 Start/Stop；
- item/trace 空 scope、context mismatch、跨 Workspace get/delete 与局部重号隔离；
- RecallTrace 首次记录、exact replay、冲突 replay、单次通知；
- repository 失败不发布，恢复后成功提交并各发布一次；
- item/search/trace 默认与最大 limit、cursor、多页 workspace purge、Orchestrator trace 分页；
- Run 已提交但 RecallTrace 尚未交接时的启动补写，以及再次启动恢复的 exact replay；
- 非法 score/vector、candidate 上限、type quota、可见性、byte/token context budget；
- command/result、model、projection、CLI/Eventstream selection evidence 深拷贝及 nil/wrong concrete result 拒绝。

精确 tokenizer 计数与 durable notification outbox 不属于当前 Memory owner 合同；如未来产品要求二者，必须新增独立 typed policy/交付合同，不能把 projection 通知提升为 Runtime authority。
