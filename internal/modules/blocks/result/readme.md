# Result Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-26

## 功能范围与边界

`blocks/result` 是 Result formal outcome 聚合的唯一 owner。聚合根为 Result，Artifact 是 owner 物化并与
Result 同 scope 的正式子实体。该 block 负责 identity、持久化状态、幂等 fence、artifact 元数据、查询与 Workspace-scoped 清理；
不负责 evidence quality、DoD coverage 或 accepted 判定策略，也不读取 Runtime authority 重新裁决结果。

## 写入与状态合同

- Runtime formal Result 的唯一生产写入口是 typed `RuntimeTerminalDeliveryFact`，且 source 必须为
  `kernel/orchestrator`。调用方只提交 sealed `ResultMaterial`，不能提交 Result ID、state、revision、时间戳或完整
  Result/Artifact entity。
- Result owner 按 `<workspace-id>/<run-id>/result/dod-assessment` 派生 canonical Result ID，并对完整规范化 material
  计算 `result-material-sha256:*`。相同 terminal identity、相同 material 是幂等重放；相同 Result ID 下 identity 或
  material 任一漂移均为 conflict，不能创建第二条 Result。
- Result 状态转换矩阵为：同状态幂等；`draft -> accepted | rejected`；`accepted | rejected -> superseded`。其中
  `draft` 只用于需要先物化 Artifact 子实体的 owner-private pending 状态；无 Artifact 的 terminal delivery 直接单次
  持久化最终状态。Runtime 的 failed/cancelled/superseded terminal outcome 均形成 rejected Result，不开放外围状态
  更新命令。
- terminal material 的顶层 `ArtifactRefs` 是允许引用集合；summary、coverage 与 closeout 中的 artifact ref 必须属于
  该集合。owner 将每个 raw ref 物化为 canonical child Artifact ID，并只在 Result 顶层、summary 与 coverage 投影中
  改写为 child ID；formal closeout 保留其 hash 已封存的 raw ref，child 的 `URI`/`SourceRef` 提供二者映射。正文不进入
  EventHub，Artifact scope 必须与父 Result 的 workspace/run/result 完全一致。
- current-schema formal closeout 绑定 `CloseoutID + StartupInputHash + DefinitionRevision + RuntimeStateRevision + TerminalOutcome`
  并逐 requirement/CWI 保存 coverage、assessment、evidence 三个独立状态。accepted evidence 必须给出 owner revision-set
  hash；顶层 accepted refs 必须等于逐项 accepted union，完整 closeout 以 canonical hash 防漂移。`completed` 不接受
  uncovered、unsatisfied、indeterminate 或无 accepted evidence 的 requirement；Result owner只校验封存完整性，不重新授予 credit。
- 不提供 Result/Artifact 单项 delete command。删除只进入 Workspace owner 的 durable deletion transaction；Result owner
  自己执行 fixed-point Result/Artifact purge，外部 caller 不能绕过 tombstone 或留下孤立子实体。

## 查询合同

- 每个 topic 都有独立 concrete result type；不存在同时承载 Result、Artifact、列表和删除计数的通用 result
  envelope，也不存在 generic entity create/update command。
- Workspace 删除只接受 `blocks/workspace` 的 concrete seal/purge command。Result owner 持久化自身 tombstone，并以 owner-private internal event 推进 `fenced → purged`；command adapter 只能提交 `purge_requested`，物理 Result/Artifact fixed-point sink 只能由同一 private dispatcher 调用，完成后再提交 `purge_completed` CAS。retain 隐藏公共 Result/Artifact 查询且不物理删除。所有 terminal delivery、recovery 及实体 mutation 都先核对同一 owner fence；file backend 与 tombstone 共用 path-global workspace 锁，memory production repository set 共用 guard。seal/purge ack 使用确定性的 `FenceID`、`PurgeID`、`Revision`、`AckHash`、`PurgeProofHash`，重放不以删除计数作为身份。
- repository helper 与 Result/Artifact mutation method 均为 Biz 包内私有实现；Biz 不导出或嵌入可由其他组件取得的
  `Service` facade，跨组件写入只能进入 owner typed EventHub handler。
- 所有 get/list command 都显式携带 `WorkspaceID`。按 Run/Result 查询时也必须绑定相同 workspace context；
  memory 与 file backend 对重复局部 ID 采用相同的 scoped/ambiguous 语义。
- 所有 list command 使用 ID keyset `Cursor` 和 `Limit`，默认 100、单页上限 200。Application 与 Orchestrator 的
  production collector 同时具备 cursor visited-set 与 1,000 页 hard limit。
- entity 离开 owner 前通过 `CloneResult`/`CloneArtifact` 隔离可变 slice 与嵌套值。terminal material normalization
  自身返回 detached value，owner 映射 canonical Artifact ID 时不再额外执行完整 `CloneResult`。allocation benchmark
  是性能回归观测，不把固定拷贝次数或 alloc 数写成语义合同。

## 通知与恢复合同

- Result 最终状态持久化后由 owner 发布 accepted/rejected `ResultChangedData`；通知含 owner-derived
  `NotificationID` 与 `TerminalMaterialHash`，不携带 Artifact 正文。
- terminal Result 行本身是通知的 durable replay source。相同 SM-4 delivery 重放会再次发布同一通知；owner 启动时
  及运行期间每分钟还会通过 owner-private、单页 200/最多 1,000 页的 recovery inventory 有界重放正式结果。
- file repository 只接受 current object-shaped `SummaryDetail.Diagnostics`；`null`、数组、标量、未知/退役字段与空文件
  均在 read/recovery 边界 fail-closed，不补值、不改写。
- Eventstream 将 `NotificationID` 用作 EventRecord ID；重复 append 只对该 stable identity 视为幂等，其他 ID conflict
  仍 fail-visible。因此 Result 持久化与 SM-4 acknowledgement 不依赖 projection，可同时关闭“持久化后崩溃/Hub
  投递中断导致永久缺通知”的窗口。

## 组件交互及合同

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | Orchestrator → Result | sealed `RuntimeTerminalDeliveryFact` / `ResultLifecycleCommandResult` | 完整；Result owner 派生 identity/state，SM-4 只接收 owner ack |
| `application/workorchd` | Application → Result | concrete scoped get/list command/result | 完整；Application 只读，Workspace owner 独占删除协调 ingress |
| `blocks/workspace` | Workspace → Result | workspace-scoped seal/purge command/result | 完整；Result owner 私有 fence 与 fixed-point sink |
| Eventstream / projection | Result → consumer | accepted/rejected `ResultChangedData` + stable `NotificationID` | 完整；Result durable replay、Eventstream 幂等去重，projection 不参与 Runtime authority |
| Run / Workspace | 无 repository 直连 | 同一 sealed terminal outcome 的 Result → Workspace → Run delivery 顺序 | 完整；Result 不直接迁移 SM-1 或 Workspace lifecycle |

## 相关约束

- 本 block 只访问 Result 与 Artifact repository，不得访问 Run、Workspace、Decision、EvidenceRecord repository 或
  其他 owner service。
- Result owner 只按 sealed outcome identity 执行 owner-local transition，不读取 `ConvergenceState`、focus、repair 或
  error message 重新裁决；delivery feedback 保留 stable category 与安全 original diagnostic。
- 如需新增 Artifact 写入，必须扩展只携带 immutable artifact material 的 typed terminal/owner contract，由 Result
  owner 派生 identity、state 和时间戳；不得恢复允许调用方提交完整 Artifact entity 的 generic write surface。

## 已覆盖回归

- canonical Result/Artifact ID、规范化 material hash、相同 delivery 重放、material/identity drift conflict。
- Result pending、Artifact 部分写入后的重启收敛，以及持久化 terminal Result 的启动通知重放。
- Result/Artifact scope、父引用、Workspace retain/purge fixed point 与 file backend 跨 workspace ResultID 隔离。
- concrete command/result 类型、默认/最大分页、cursor continuation/cycle guard 和 query clone 隔离。
- stable Eventstream dedupe、nested artifact reference closure、terminal material isolation 与 allocation benchmark。
- formal closeout identity/hash、三轴 requirement outcome、accepted-evidence union、owner revision set 与 indeterminate-success 拒绝。
- 1,024 组 unique evidence/artifact refs 的 normalization/hash、canonical child mapping、重排稳定性、输入隔离与
  toolchain-relative allocation growth guard；同时保留 hash 与 Result mapping 的独立 allocation benchmark。
- `SummaryDetail.Diagnostics:null`、非对象 diagnostics、未知/退役字段与空文件严格拒绝。

## 当前验证结论

- 当前没有已知的本地可执行合同或测试缺口；性能守卫只约束引用规模增长不得出现异常的超线性 allocation，
  不把特定机器 wall-clock 或绝对 alloc 数写成业务合同。
