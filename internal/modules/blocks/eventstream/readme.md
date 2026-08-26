# Eventstream Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-26

## 组件类型

`blocks/eventstream` 是 Stateful Block：它只拥有不可变 EventRecord 审计流、全局追加位置、workspace-local revision / high-watermark、有界查询，以及永久的 workspace Eventstream 删除 tombstone。它不组合多个 owner 推进业务闭环，也不拥有 SM-1～SM-4 的任何权威状态。

## 功能范围与权威边界

- EventRecord 只服务审计、投影、SSE 和展示，不是 Workspace、Run、Result 或治理状态的修复、合并、删除及重建入口。
- Goal/DoD canonical lineage、定义选择和 `RuntimeBootstrap` 只读取 Workspace owner 的 `DefinitionRevision` 与 lineage state；不得读取 EventRecord 的时间、revision、payload 或存在性。边界脚本对 authority 路径执行静态扫描。
- Eventstream 不订阅 Orchestrator Runtime topic，也不向 Orchestrator 提交 drive、evidence、resolution 或 lifecycle command。
- projection 可以用 persisted workspace high-watermark 判断自身是否追平，但 projection 与 EventRecord 都不得反向修改 owner state。

Eventstream 正式拥有两个不同的顺序身份：

- `StreamPosition`：Eventstream owner 在追加前签发的全局单调位置，是跨 workspace 持久化回放和 cursor 的顺序 SoT。允许因失败预留产生空洞，但已签发的位置不得在重启或 purge 后复用。
- `Revision`：workspace-local EventRecord 的追加代次，用于该 workspace 的 projection high-watermark。workspace 一旦进入 Eventstream 删除 tombstone 就永久禁止再次追加，因此不存在 purge 后 revision 重置或重新从 1 开始的语义。

删除 tombstone 自身另有 owner-private revision：`fenced=1`、`purged=2`。它与 EventRecord revision 是两个不同状态域，不得互相推导。

`OccurredAt` 仅是来源时间元数据，不参与 cursor 顺序。每条持久化记录必须具有正 `StreamPosition`，workspace 记录还必须
具有正 `Revision`；缺失/零值不会被重新排序或补号，读取直接失败。

## 组件交互及具体合同

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| Workspace、Run、Result、Decision、Approval、Memory、Governance、LLM | owner → Eventstream | 各 owner 的具体 typed notification | 完整；每个 topic 校验具体 data、必要 identity 与精确 producer source |
| Approval | Approval → Eventstream | `RecordApprovalResolutionCommand` → `RecordApprovalResolutionResult` | 完整；Approval 终态提交所需的同步持久化 checkpoint，exact replay 返回同一 ack |
| `application/workorchd` | Application → Eventstream | `ListEventsCommand` → `ListEventsResult` | 完整；使用 owner-local `EventQuery`，默认 100、最大 5000 |
| `application/workorchd` | Application → Eventstream | `HighWatermarksCommand` → `HighWatermarksResult` | 完整；一次最多 500 个 workspace，结果为具体 workspace/revision 列表 |
| Workspace | Workspace → Eventstream | `SealWorkspaceEventStreamDeletionCommand` → `SealWorkspaceEventStreamDeletionResult` | 完整；只接受 `blocks/workspace` source，以完整删除 Scope 建立永久 fence |
| Workspace | Workspace → Eventstream | `PurgeWorkspaceCommand` → `PurgeWorkspaceResult` | 完整；只接受 `blocks/workspace` source，只能在同一 `mode=purge` fence 上执行 exact physical purge |
| application projection / SSE | Eventstream → Application | `EventRecordAppendedData{Record EventRecordData}` | 完整；只读派生链路，不回写 owner |
| `kernel/orchestrator` | 无直接交互 | 无 | 完整；Runtime 权威状态不从审计流构造 |

查询命令不暴露 repository query 或 entity。`EventQuery` 在 owner 内完成 trim、去重、limit 校验和 repository query 映射；list、high-watermark、seal 与 purge 分别使用不可混淆的具体 result，不使用可空字段联合 envelope。

`biz.Biz` 只向 `module.go` 暴露标准构造与 Start/Stop 生命周期；repository-facing list/high-watermark/deletion 实现均为
owner-private helper，不存在可被 sibling 直接调用的 `Service/NewService*` 或 `PurgeWorkspaceEvents` facade。workspace 删除只有
一个 concrete typed command ingress；该 ingress 根据 seal/purge topic 解码精确 payload，并在校验 `blocks/workspace` source、
完整 scope 和同步 result 后进入 Eventstream owner-private dispatcher。purge command adapter 只能提交
`purge_requested`；只有 dispatcher 可以调用 exact-fenced `PurgeWorkspaceEventRecords`，成功后再生成私有
`purge_completed` 并 CAS tombstone。外部或同包误投 `purge_completed` 均拒绝。

所有命令 handler 都先要求非空同步 result，再执行读取或写入。因而误用 `PostEvent` 不会建立删除 fence、不会执行 purge，也不会执行 Approval 的同步审计 checkpoint。

## Workspace 删除生命周期

Eventstream owner 内归一化的删除 Scope 固定由
`WorkspaceID/DeletionID/DeletionGeneration/Mode/QuiescenceID/RequestHash` 六个字段组成。seal command 不暴露目标状态，
只携带 `DataDispositionIntent` 并由 owner 映射为 Scope 的数据保留/清理模式；purge command 不携带 Mode，其具体命令语义
在 owner 内固定为 `mode=purge`。Eventstream 不接收简化 WorkspaceID purge，也不从其他 owner 状态补造其余 scope 字段。

- seal 通过 owner-private internal event 将不存在的 tombstone 转换为 `fenced/revision=1`，返回稳定
  `FenceID/AckHash`。同 scope replay 返回相同结果；任一 scope 字段变化均 conflict。
- `mode=retain` 在 fence 后完成：物理 EventRecord 保留，但 append、Get/List、global list、high-watermark、recovery/read
  均 fail-closed 或隐藏；retain scope 永远不能进入 purge。
- `mode=purge` 先建立同一 fence，再由唯一的 exact `PurgeWorkspaceEventRecords` sink 清理目标 workspace 的物理
  EventRecord；sink 必须同时匹配完整 persisted fenced aggregate，而不是只匹配 WorkspaceID 或 Scope。随后同一次
  owner-private dispatcher transaction 才能生成 `purge_completed` 并转为 `purged/revision=2`，返回稳定
  `PurgeID/AckHash/PurgeProofHash`。崩溃发生在物理清理与 reducer CAS 之间时，重放从空集 fixed point 继续完成。
- tombstone 无论 retain 还是 purge 都永久保留。purge 不删除 tombstone、不允许 workspace 再次追加，也不删除或回退
  全局 StreamPosition floor；已签发的位置永不重置、复用或重新编号。system/global EventRecord 不属于 workspace fence，
  仍可继续追加和读取。
- `TopicWorkspaceDeleted` 的 concrete `WorkspaceDeletedData` 是所有 owner seal/purge 完成后的 Workspace-owned
  immutable transport receipt。Eventstream 不订阅、不尝试
  将它追加到已封印的 audit lineage；Application transport/SSE 等 EventHub consumer 仍直接接收该事实。这样不会为了记录
  completed receipt 而重开 fence，也不会制造预期内的 late-append error。

append 与 tombstone 建立共享同一原子 guard：Biz 的 `appendMu` 负责单实例串行化；memory production owner set 让
EventRecord repository 与 tombstone repository 共享同一锁；file backend 以
`dataRoot/system/workspace-deletions/eventstream/<workspace>.json` 建立 path-global lock，并在锁内重读 tombstone，因而两个
独立 repository set 的 append-vs-seal 仍具有单一线性化点。普通 EventRecord `Delete` 也使用 active-fence guard，fence 后
返回 not-found，不能成为旁路；`EventRecordRepository` 不再提供 generic `PurgeWorkspace`，memory、raw file
和 workspace-scoped backend 都只能由携带完整 persisted fence 的 canonical Eventstream purger 清理物理记录。file backend
另以 path-global position lock 保护全局 StreamPosition floor。

file backend 的物理 compaction 是该 canonical purger 内部的 owner-private transaction，不是另一个公开状态机。它把完整
`fenced WorkspaceDeletion`、canonical root 与派生 transaction identity 写入当前格式的 durable swap journal；
`preparing/prepared/swapping` 只能回滚到 exact old generation，`installed/completed` 只能向前清理，全部 phase 都位于
`purge_requested → private dispatcher → PurgeWorkspaceEventRecords` 调用栈内。每次目录 rename 先持久化 intent，完成后持久化
progress，并同步 journal 文件、受影响目录及 parent；replacement 在 `prepared` 前递归同步。Append、Reserve、Observe、Get、
List、Delete 与 exact purge 在同一 repository mutex 内首先恢复或 fail-closed，因此重启后的首次普通读取也不会看到半套目录。
journal 使用唯一 deterministic pending 文件完成原子替换；canonical journal 是唯一 authority，损坏、scope/fence drift、路径
越界、受管 symlink、歧义 pending 或无 journal 的受管目录均拒绝。swap 只删除目标 Workspace 的 records；其它 Workspace 的
大 payload 与 delete marker、system/global records、unrelated owner files 及全局 StreamPosition 字节保持不变。该 current
格式不识别或升级历史 journal/temp 变体。

## 来源、payload 与实时输出边界

- 每个订阅同时校验 topic、具体 data 类型、必要 typed identity/`Validate()` 与精确 `event.Source()`；Eventstream 不把来路不明的同形 DTO 改写成正式 owner 记录。
- stable Result、Decision、Approval notification 以 owner 派生的 `NotificationID` 作为 EventRecord ID。相同 ID 仅在持久化记录与预期 topic/source/scope/payload 完全一致时视为 exact replay；内容漂移仍是 conflict。
- typed payload encoder 不保存 Memory 正文、Goal/DoD 原文、Decision resolution material 或 Approval context 原文；这些材料只保存 hash、bytes、count 等审计摘要。
- LLM usage payload 无损保存 owner 提供的 `logicalRequestId`/`physicalCallId`（Runtime usage 两者必须完整且
  `physicalCallId=requestId`）、bounded `physicalResponseMode=streaming|non_streaming` 与 exact
  `providerSubmitted`；generic non-Runtime usage 可以不携带双层身份，Eventstream 不补造。公共 payload 不保存并行
  `streaming` 布尔值。Eventstream 不按 request bytes、provider/model/status 推测身份、提交或模式，也不保存 provider raw body、
  prompt、credential、Authorization 或 endpoint。
- `SanitizeAuditPayload` 是追加前 guard：递归脱敏 secret-like key，并限制 payload 为 16 KiB、最多 32 个字段/数组项、
  单字符串 512 bytes、最大深度 4；无法安全容纳的内容退化为确定性 hash/size 摘要。owner read 只验证 persisted payload
  已满足同一合同；不再二次清洗或升级无效记录。
- file repository 的 current external payload 表示固定为 `record.Payload={}` 加 log-line 顶层 `payloadRef`，locator 指向
  非空对象文件。`Payload:null`、record 内嵌 `PayloadRef`、双重 inline/external material、数组、标量或空文件均 fail-closed。
- `EventRecordData.Validate()` 要求完整 immutable identity/source、正 `StreamPosition`、workspace-local 正 revision，以及已经清洗且有界的 payload；Application 消费者还校验发布 source 必须是 `blocks/eventstream`。

## 追加、回放与失败语义

普通 owner notification 通过 EventHub `PostEvent` 发布，因此它没有可向原发送方返回的同步错误通道。当前合同明确将这类审计记录定义为 best-effort：Eventstream 对瞬时 append 错误立即重试最多 3 次，最终失败记录结构化日志，不发布虚假的 `EventRecordAppendedData`；任何业务完成、Runtime 恢复或 owner 幂等都不得依赖该记录存在。

需要业务步骤 fail-closed 的持久化审计必须定义独立的具体同步 command/result 或 owner outbox，不能把普通 Post notification 描述为同步持久化。当前此类合同包括 Approval resolution checkpoint，以及 Workspace 驱动的 Eventstream deletion seal/purge。

完整 audit SSE 重连不依赖进程内 256 条 ring：server 先建立 live subscription，再按持久化 `StreamPosition` cursor 以每页 500 条读取直到追平，追赶期间的新追加进入 live queue。process-local ring 只用于非 EventRecord 的 projection frame 回放，不承担持久化 audit replay。

## 已覆盖回归

- topic/data/source/identity 校验、伪造 producer 拒绝与订阅释放；
- concrete result、默认/最大 limit、high-watermark 范围和 Post 强写拒绝；
- payload 脱敏、深度/长度/总量边界及无效 persisted payload 严格拒绝；
- LLM physical response mode 在 streaming/non-streaming、失败/output-limit 与 embedding/preflight 场景下 round-trip，
  公共 payload 不含 `streaming` boolean，且 raw provider request/response material 不落盘；
- current inline/external payload exactly-one、`Payload:null`、嵌套/冲突 reference、数组、标量和空 payload 文件拒绝；
- stable notification exact replay、conflicting/failed append 不发布虚假记录、瞬时失败重试；
- workspace revision 单调、跨 workspace clock skew、全局 `StreamPosition` cursor、重启与 purge 后位置 floor 单调；
- deletion command 的精确 source/typed payload、scope drift conflict、retain 禁止 purge、revision 1/2 与稳定 ack/proof；
- seal 后 append/Get/List/global list/high-watermark fail-closed，普通 Delete 不能绕过 fence，generic
  `EventRecordRepository.PurgeWorkspace` 生产 surface 静态清零；
- purge command adapter 零物理 sink 调用、`purge_requested` 唯一进入 private dispatcher、sink 失败零 CAS、direct
  `purge_completed`/retain/stale/scope drift 零 effect，以及 CAS 后 response-loss exact replay；
- memory shared guard 与两个独立 file repository set 的 seal-vs-append 线性化；
- retain 物理记录保留且 reopen 后继续隐藏；purge fixed point、永久 tombstone、reopen 隐藏、其他 workspace/global 记录保留；
- file purge journal 的 preparing partial-build、逐目录 rename/progress、installed/cleanup 全切点 reopen；首次任意 read/write
  recovery、完整 fence drift、corrupt/unknown/path escape、pending 歧义、symlink、受管 orphan 均 fail-closed；其它 Workspace
  external payload/tombstone 与 StreamPosition bytes/单调性 exact 保留；
- post-seal concrete `WorkspaceDeletedData` 仍可由 EventHub consumer 接收，但不会触发 Eventstream late append；
- memory/file/workspace-scoped repository 的 scope、cursor、exact 物理 purge、其他 owner 文件保留；
- HTTP audit replay 跨多页追平，以及 projection replay 与 audit replay 分离；
- authority 路径禁止依赖 Eventstream/EventRecord 的静态边界扫描。

当前组件合同没有待补的 code-closure 项；长时间运行的吞吐、磁盘压缩和断线重放规模只保留为非阻断 benchmark/soak 观测，不改变上述语义合同。
