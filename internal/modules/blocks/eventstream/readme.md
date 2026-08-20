# Eventstream Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-20

## 组件类型

`blocks/eventstream` 是 Stateful Block：它只拥有不可变 EventRecord 审计流、全局追加位置、workspace-local revision / high-watermark，以及有界查询和 workspace 清理合同。它不组合多个 owner 推进业务闭环，也不拥有 SM-1～SM-4 的任何权威状态。

## 功能范围与权威边界

- EventRecord 只服务审计、投影、SSE 和展示，不是 Workspace、Run、Result 或治理状态的修复、合并、删除及重建入口。
- Goal/DoD canonical lineage、定义选择和 `RuntimeBootstrap` 只读取 Workspace owner 的 `DefinitionRevision` 与 lineage state；不得读取 EventRecord 的时间、revision、payload 或存在性。边界脚本对 authority 路径执行静态扫描。
- Eventstream 不订阅 Orchestrator Runtime topic，也不向 Orchestrator 提交 drive、evidence、resolution 或 lifecycle command。
- projection 可以用 persisted workspace high-watermark 判断自身是否追平，但 projection 与 EventRecord 都不得反向修改 owner state。

Eventstream 正式拥有两个不同的顺序身份：

- `StreamPosition`：Eventstream owner 在追加前签发的全局单调位置，是跨 workspace 持久化回放和 cursor 的顺序 SoT。允许因失败预留产生空洞，但已签发的位置不得在重启或 purge 后复用。
- `Revision`：workspace-local 的追加代次，用于该 workspace 的 projection high-watermark。完整 purge 后该 workspace 的 revision 回到 0，下一条记录从 1 开始；全局 `StreamPosition` 不回退。

`OccurredAt` 仅是来源时间元数据，不参与 cursor 顺序。旧数据若没有 `StreamPosition`，repository 将其置于新位置记录之前并使用确定性的 legacy 顺序；所有新追加必须具有正 `StreamPosition`，workspace 记录还必须具有正 `Revision`。

## 组件交互及具体合同

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| Workspace、Run、Result、Decision、Approval、Memory、Governance、LLM | owner → Eventstream | 各 owner 的具体 typed notification | 完整；每个 topic 校验具体 data、必要 identity 与精确 producer source |
| Approval | Approval → Eventstream | `RecordApprovalResolutionCommand` → `RecordApprovalResolutionResult` | 完整；Approval 终态提交所需的同步持久化 checkpoint，exact replay 返回同一 ack |
| `application/workorchd` | Application → Eventstream | `ListEventsCommand` → `ListEventsResult` | 完整；使用 owner-local `EventQuery`，默认 100、最大 5000 |
| `application/workorchd` | Application → Eventstream | `HighWatermarksCommand` → `HighWatermarksResult` | 完整；一次最多 500 个 workspace，结果为具体 workspace/revision 列表 |
| `application/workorchd` | Application → Eventstream | `PurgeWorkspaceCommand` → `PurgeWorkspaceResult` | 完整；仅接受 application appservice source，必须同步发送并携带 WorkspaceID |
| application projection / SSE | Eventstream → Application | `EventRecordAppendedData{Record EventRecordData}` | 完整；只读派生链路，不回写 owner |
| `kernel/orchestrator` | 无直接交互 | 无 | 完整；Runtime 权威状态不从审计流构造 |

查询命令不暴露 repository query 或 entity。`EventQuery` 在 owner 内完成 trim、去重、limit 校验和 repository query 映射；list、high-watermark 与 purge 分别使用不可混淆的具体 result，不使用可空字段联合 envelope。

所有命令 handler 都先要求非空同步 result，再执行读取或写入。因而误用 `PostEvent` 不会执行 purge，也不会执行 Approval 的同步审计 checkpoint。Purge 与 append 串行，memory backend 物理移除目标记录；file backend 压缩 Eventstream 自有的 `events/payloads/ids/deleted` 数据，同时保留同目录下其他 owner 状态和全局 stream-position 计数。重复 purge 返回 0。

## 来源、payload 与实时输出边界

- 每个订阅同时校验 topic、具体 data 类型、必要 typed identity/`Validate()` 与精确 `event.Source()`；Eventstream 不把来路不明的同形 DTO 改写成正式 owner 记录。
- stable Result、Decision、Approval notification 以 owner 派生的 `NotificationID` 作为 EventRecord ID。相同 ID 仅在持久化记录与预期 topic/source/scope/payload 完全一致时视为 exact replay；内容漂移仍是 conflict。
- typed payload encoder 不保存 Memory 正文、Goal/DoD 原文、Decision resolution material 或 Approval context 原文；这些材料只保存 hash、bytes、count 等审计摘要。
- `SanitizeAuditPayload` 是追加前和 owner 读出前的双重 guard：递归脱敏 secret-like key，并限制 payload 为 16 KiB、最多 32 个字段/数组项、单字符串 512 bytes、最大深度 4；无法安全容纳的内容退化为确定性 hash/size 摘要。旧版本已落库的 payload 也必须经过读边界清洗后才能进入 EventHub/SSE。
- `EventRecordData.Validate()` 要求完整 immutable identity/source、正 `StreamPosition`、workspace-local 正 revision，以及已经清洗且有界的 payload；Application 消费者还校验发布 source 必须是 `blocks/eventstream`。

## 追加、回放与失败语义

普通 owner notification 通过 EventHub `PostEvent` 发布，因此它没有可向原发送方返回的同步错误通道。当前合同明确将这类审计记录定义为 best-effort：Eventstream 对瞬时 append 错误立即重试最多 3 次，最终失败记录结构化日志，不发布虚假的 `EventRecordAppendedData`；任何业务完成、Runtime 恢复或 owner 幂等都不得依赖该记录存在。

需要业务步骤 fail-closed 的持久化审计必须定义独立的具体同步 command/result 或 owner outbox，不能把普通 Post notification 描述为同步持久化。当前唯一此类合同是 Approval resolution checkpoint。

完整 audit SSE 重连不依赖进程内 256 条 ring：server 先建立 live subscription，再按持久化 `StreamPosition` cursor 以每页 500 条读取直到追平，追赶期间的新追加进入 live queue。process-local ring 只用于非 EventRecord 的 projection frame 回放，不承担持久化 audit replay。

## 已覆盖回归

- topic/data/source/identity 校验、伪造 producer 拒绝与订阅释放；
- concrete result、默认/最大 limit、high-watermark 范围和 Post 强写拒绝；
- payload 脱敏、深度/长度/总量边界、旧记录读边界清洗；
- stable notification exact replay、conflicting/failed append 不发布虚假记录、瞬时失败重试；
- workspace revision 单调、跨 workspace clock skew、全局 `StreamPosition` cursor、重启与 purge 后计数语义；
- memory/file/workspace-scoped repository 的 scope、cursor、物理 purge、其他 owner 文件保留；
- HTTP audit replay 跨多页追平，以及 projection replay 与 audit replay 分离；
- authority 路径禁止依赖 Eventstream/EventRecord 的静态边界扫描。

当前组件合同没有待补的 code-closure 项；长时间运行的吞吐、磁盘压缩和断线重放规模只保留为非阻断 benchmark/soak 观测，不改变上述语义合同。
