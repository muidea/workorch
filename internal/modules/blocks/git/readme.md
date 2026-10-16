# Git 能力组件

Status: active

Type: capability-block-contract

Last Verified: 2026-08-30

## 功能范围与边界

`blocks/git` 是 `git/read` 与 `git/write` 的 capability owner。它在 workspace deliverable repository 内执行固定 Git
operation catalog，观察 repository/File revision，并把 owner-private 命令结果规范化为 typed fact 或
`RuntimeOperationReceipt`。

Git stdout、stderr、diff 和完整私有 DTO 不跨 owner。跨 owner 的执行结果只有有界 evidence、resource change 与 receipt。

## 组件交互及合同

| 交互方 | 方向 | concrete typed 合同 |
| --- | --- | --- |
| Orchestrator | Orchestrator → Git | `AcquireRuntimeCapabilities`、`AcquireRuntimeOperationAdmissibility`、`ProbeRuntimeOperationFeasibility`、`ProbeRuntimeSemanticProofFeasibility`、`QueryRuntimeCWIReconciliation`、`AcquireCapabilityState`、`ExecuteOperation`、`QueryRuntimeCommitSnapshot` Command/Result |
| Orchestrator | Git → Orchestrator | typed runtime-operation receipt query/commit；receipt miss 只授权 exact active claim，快照读取用 `ReadSource` 反查原 persisted dispatch/execution；commit 只接受完整 receipt equality |
| Workspace | Git → Workspace | 快照只读查询经 `GetWorkspace` 取得 actual internal/deliverable directory；caller directory 不授读取权限 |
| File owner | Git → File | `AcquireWorkspaceManifestCommand/Result`，用于 workspace-tree binding 和 effect 前 freshness 重验 |
| Git repository | Git-private | 固定 `status/diff/log/show/branch/init/add/commit` argv、输出解析、revision/effect observation |

Owner topic、Command/Result 与 seal 类型的定义源是 `blocks/git/pkg/events`。Git 不读取 File service/repository，也不暴露
跨 owner service facade。

## 不变量与非职责

- repository root 必须等于解析后的 deliverable root；父仓库、bare repository、外置或 symlink `.git` 均不取得 authority。
- read ref/baseRef 拒绝 option-shaped、控制字符和超长输入；不提供任意 Git CLI、remote 或 history rewrite 入口。
- write 只能使用 sealed operation 授权的 target/pathspec；`add` 根据完整 index delta 判断 changed/no-op。
- capability-state receipt 按 workspace 分区、不可变并 durable 发布；同 acquisition key 的不同 snapshot 必须冲突。
- effect 前必须重验 File-owned workspace-tree revision，repository revision 与 File revision 不得相互替代。
- 全部八个 Runtime ingress 都是 strong call：必须先确认同步 `Result` 与 exact `kernel/orchestrator` producer，再解析 concrete
  Command；Post/nil Result、错误 producer 或错误 concrete payload 均不得触发 File query、repository query/effect 或 receipt I/O。
- 本组件不拥有 Run、Result、CWI credit、retry、governance 或 terminal 状态；Git fact/evidence 不能直接判定 DoD。
- CWI reconciliation 只可陈述 repository fact；repository revision 不能替代 check、metric、artifact 或 semantic proof。

### 本地 commit 的实际对象依赖

- production Execute 在 effect 前要求 exact Workspace/Definition 身份。非 no-op commit 成功后，由同一 Git owner
  完整读取观察到的 immutable commit 和全部可达递归 tree/blob，按 Git object header/body 反查 SHA-1 或 SHA-256 对象身份，
  并以完整原 bytes 的 SHA256 与字段/对象 selector 编译 receipt `Evidence.OwnerRevisionSet`。
- 对象读取禁用 replace refs；原 binary tree grammar 只识别 tree、普通/可执行 blob 和 symlink blob，不将文件名解析为 argv。
  gitlink/submodule 明确拒绝，不能声称保存另一 repository 的 bytes。共享对象只读取一次，但每条可达路径仍验证深度。
- 同一次有界 capture 最多 1024 个不同对象、总原 bytes 4MiB、深度 64，单对象上限为 64KiB。
  缺失、损坏、类型不符、取消或超限均返回错误，不把部分对象标为完整；graph capture 和 HEAD 前后观察共用 owner timeout。
  effect 可能已经发生的读取失败不发布 fabricated completed/rejected/no-op receipt；原 SM-3 unknown 身份、预算与对账路径保留。
- 该集合只证明 owner 实际读取的 commit/tree/blob 对象，不代表当前工作树、子模块、测试或终验结果。
  不读取整个 repository hash，不从 authority/evidence revision 合成来源；无关工作树变化不改变 immutable 对象依赖。
- 当 sealed execution 有 Workspace internal directory 时，Git owner 以 `git/commit-object-snapshot/v1` 保存完整原值：
  exact Workspace/Run/Dispatch/Operation/Definition、execution key/fence、operation/authority hash、实际 HEAD/root tree 和
  全部 object bytes。binary bodies 使用可逆 byte-array 编码，读取独立反查每个 Git object ID 及完整图，拒绝缺项、重复和额外对象。
  受限 `git/commit-object-snapshots` 目录为 0700、文件为 0600；拒绝 symlink 和不安全权限，使用 rooted I/O、file/directory fsync
  与 no-overwrite publication。文件以 scope hash 定位，同 execution/fence 的另一个有效正文也冲突；receipt 的既有
  `ContentAddress/ContentHash/EvidenceRefs` 绑定整个原 record SHA256。没有 internal directory 的 adapter-only caller 不声明原文归档。
- capture 前后真实 HEAD 必须等于实际 commit output；这是提交时观察，不是终态 fresh HEAD、工作树一致或已评审输入证明。
  原文归档不新增 Runtime 状态机、credit 或 owner 查询权限，也不替代独立的最终快照消费者。
- 已持久 receipt 的 shutdown drain/owner 重建重放保持原对象集合，不重新 commit 或用当前 HEAD 替换归档。
  `git_commit_read_dependencies_test.go` 覆盖完整原 bytes、raw/重载/返回隔离、非法 scope、缺失/超限对象、
  真实 EventHub commit、receipt persistence 后响应丢失与 exact receipt 重放。
  `git_commit_snapshot_test.go` 独立对照 Git 的完整递归对象集合及 binary bytes，并验证 SHA256 repo、并发重复、same-scope conflict、
  篡改、publication 前后取消恢复、对象数/总 bytes 边界和共享子树深度门禁。
  Orchestrator `TestDriveRuntimeFileBackedFreshFileShellGitAcceptance` 独立核对实际快照、原 sealed execution/key/fence、
  receipt → accepted EvidenceFact → immutable material → 正式 Result 同一引用及三个 ACK；此 private-startup/scripted-provider
  测试不替代公开 authoring/activation、终态 owner 当前性、已评审输入联结或 live 验收。
  Orchestrator receipt/evidence 现有完整 hash/raw/source 门禁继续适用。
- 本片不改变 SM-1～SM-4 transition、不授 CWI credit、不开放 implementation-work availability；
  terminal 全部 actual dependency union 与材料消费仍由各自合同单独负责。

### 原 commit 快照的有界当前观察

`QueryRuntimeCommitSnapshot` 使用 closed `git/commit-object-snapshot-query/v1` concrete Command/Result，
是 SM-4 的 read-only participant，不是 Git 的 Runtime transition 或新的执行入口。
command 必须绑定原 transaction、sealed durable commit、execution key/fence/authority、receipt hash、
archive address、完整原 object dependencies，以及 request/state revision；不接受 caller directory。
Git 经 Orchestrator 的 `ReadSource` 独立反查原 persisted receipt，miss/错源/错scope/错回执均不得读取归档或 repository。
Workspace owner 再提供 actual directories；retired/unavailable/wrong typed response 明确失败，无目录 fallback。

Git 先完整验证受限归档的 scope、address、原 bytes 与对象图，并独立重算原 dependencies；随后重验 repository root
并在同一 owner timeout 内观察 HEAD。HEAD 与归档相同，必须重新完整读取全部 commit/tree/blob、校验实际 bytes，
重算 dependency set 且 HEAD 前后相同才返回 `CurrentDependencies`；HEAD 不同则返回实际 `ObservedHead`、原快照身份
和 nil current set，不冒原快照仍当前。观察期间 HEAD 漂移、缺失/损坏对象、超限或取消均返回错误，不能把失败当 absence。
raw object bodies 不跨 owner；bounded reply 只包含身份、计数与 verified dependencies，返回集合 detached。

v2 `git/commit-object-snapshot-query/v2` 在同一 read-only topic 显式增加有界 input-byte membership。
command 必须携带完整 canonical input binding/subject（最多 16 files、64KiB input bytes、512 native mappings）；
v1 禁止此扩展，v2 缺失完整 subject 拒绝。Git 不认定 caller subject 已通过 LLM 评审、不授 review/credit authority：
Orchestrator 必须先从完整合法 aggregate 的原 evidence-committed validation、exact current binding 与唯一 accepted
semantic batch 反查真实输入；不能扫描任意 consistent 历史报告或自动升级无评审归档。

Git 在完整验证原归档之后，按 raw tree grammar 的 exact path 逐层匹配原 filename bytes，最终只接受普通／可执行
blob，比较完整原 bytes 和 SHA256。缺文件、仅别处有相同 blob、内容变化／前缀、directory、symlink traversal／leaf、
路径规范化别名或超限均明确拒绝，不能返回部分成功。原 tree 的非法 UTF-8／特殊 sibling filename 不改写；
有界 reply 回显 subject hash、全部原 path/content hash 与 blob ID，不返回归档 raw bodies。
每个 blob/hash 还必须属于原完整 dependencies；HEAD 漂移时可保留原 membership，但 `CurrentDependencies` 仍为 nil，
不得将原 membership 冒为当前工作树或新的 HEAD proof。返回 detached，owner 重建读取同一原归档。

该观察不证明工作树/index clean、评审语义、全 owner freshness 或持续原子快照；不会改写原 receipt、
封存终态材料、授 credit 或开放 availability。BeginShutdown 后拒绝新 query，已接纳 query 的 nested source/Workspace
读取及 Git process 必须真实退出才释放 lifecycle ticket；timeout 保留依赖供独立预算重试。
`TestGitCommitSnapshotQueryReadsOriginalArchiveAndFreshObjects` 固定 binary archive、wrong source/scope/address/dependencies、
完整 response fence、HEAD 变化、归档篡改、duplicate、owner 重建及真实 nested-source-read quiescence。
Orchestrator `TestDriveRuntimeFileBackedFreshFileShellGitAcceptance` 通过 production query sender → Git → 原 receipt handler →
file-backed Run aggregate 反查原 execution 与完整图，校准 v2 技术 byte-membership sender（不是评审来源），
无评审的原 validation 不得升级；后续真实 HEAD 变化不得被三 ACK 或旧 evidence 隐藏；query 零 aggregate 写入。
`TestRuntimeReviewedInputSnapshotSourcesOwnerMainlineAndRestart` 另以真实 File/Shell/Run 的 accepted review、实际 validation
和 file-backed reload 验证 reviewed-source compiler。上述配对不等于终态材料消费者或公开 fullflow 已闭合。

### 生命周期与排空合同

v3 `git/commit-object-snapshot-query/v3` 复用原 read topic，要求原 receipt 的 exact commit/root identity 和完整原 validation workspace-byte read set，并将其逐项
联结到原完整 tree/blob 图；不支持的 resource kind 明确拒绝。它另外前后读取完整 bounded porcelain status（tracked、
index、untracked、submodule），拒绝截断或观察中变动，返回 clean fact。原 accepted commit receipt 自身封存
owner-neutral snapshot/root identity；query 在 Workspace/文件系统读取前反查原 receipt，并在归档读取及 response 边界精确比较，
不能把同 graph 的 blob/tree 换作 HEAD/root。SM-4 V2 consumer 在首次不可变材料 CAS 独立反证并消费这些事实；V1/V2 拒绝该扩展，仍不宣称 clean。
sealed 后交付重试不依赖新的 Git query。该时点证明不代表 subprocess 全部读取或所有 owner 持续当前性。

- `Start` 通过 Base checked SimpleObserver API 逐成功 topic 记账；第 N 个失败保留同一 owner 与成功订阅。Start 关闭后不可重新打开代次。
- `BeginShutdown` 关闭 fresh root 准入；每个实际 EventHub handler 从准入到真实返回都持有 owner Lifecycle 票。cancel/调用者等待超时不释放票，因此子进程等待和 nested receipt commit 都留在实际执行 handler 的排空范围。
- Execute 的显式 drain 只做 sealed command 校验与 exact persisted receipt 查询/重放；未命中必须另取 fresh 票，不能借 drain 创建新 effect、governance receipt 或重新执行。其他 capability/admissibility/feasibility/reconciliation 查询在关门后拒绝 fresh root。
- `Quiesce(ctx)` 等待本 owner 真实在途归零；超时保留 adapter、订阅和同一 owner 引用，可使用独立预算重试。局部归零不封闭合法 drain，最终 Teardown 仍必须由应用共享 Hub/任务全局屏障后的调用方授权，不能把退订当作排空。
- `Biz.Teardown(ctx)` 返回 error，每项退订真实成功才 pop，失败或取消保留未释放依赖。`Module.Stop` 成功才清空 owner；public void `Teardown` 的失败交给 framework guard。生命周期配置/最终释放调用由调用方串行化，不持有跨 Hub action 的 owner 锁。
- `OwnerQuiescenceWaitsForRealHandlerAndRetainsRetryDependencies`、`OwnerQuiescenceExplicitReceiptDrainStillWaitsForCompletion` 与 ResourceLedger/Module tests 验证本地阻塞、取消、关门、retry 和 partial Start；这些测试不证明全进程所有 owner 已适配，也不改变 SM-1～SM-4 业务合同。

## 能力验收

| 验收维度 | 自动化证据 | 状态 |
| --- | --- | --- |
| normal | `TestRuntimeAdapterReadBranchLogAndShowFromCommittedRepository`、read/status/diff 与 init/add/commit adapter tests | 已覆盖固定 operation catalog 的主要正常路径，包括非空 branch/log/show 解码 |
| invalid + scope | option/pathspec、remote/history rewrite、parent/external gitdir、detached HEAD、expected-head、File responder matrix、`TestGitRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls` | 已覆盖主要输入/repository scope，并穷举八 topic 的 wrong source、wrong concrete payload 与 Post/nil Result 零查询/零 effect |
| duplicate + idempotency | `TestRuntimeAdapterWriteInitIsIdempotentForExistingRepository`、重复 add/no-op、`TestGitOwnerStateChangingReceiptReplaysWithoutSecondOperationAfterOwnerRecreation`、`TestGitOwnerReadReceiptReplaysAfterOwnerRecreation` | 已覆盖 read/write owner receipt 与主要 write 幂等路径 |
| stale + freshness | `TestGitOwnerCapabilityBoundAdmissibilityRejectsMismatchedProbeBindingAsStale`、`TestGitOwnerRechecksFileWorkspaceTreeBeforeFeasibilityAndExecution` | 已覆盖 catalog/File revision stale fence |
| restart / owner 重建 | `TestGitOwnerStateChangingReceiptReplaysWithoutSecondOperationAfterOwnerRecreation`、`TestGitOwnerReadReceiptReplaysAfterOwnerRecreation`、capability receipt store durable tests | 已覆盖 read/write receipt 重放和 durable capability-state 加载 |
| lifecycle | `TestGitModuleIdentityConfigureStartStopContract`、`TestGitOwnerLifecycleFencesSubscriptionsAndCapabilityDelivery`、`TestGitOwnerLifecycleCoversEveryRuntimeTopic` | 覆盖全部八 topic 的重复 Start 单次投递和 Stop 后零投递 |
| effect + receipt | resource-change、repository revision、crash/restart、state-changing receipt tests、`TestGitOwnerReadReceiptReplaysAfterOwnerRecreation` | 覆盖 write effect 与 read `effect=none` receipt 的真实 EventHub query/commit/replay |
| boundary | `TestGitOwnerAcquiresCapabilitiesThroughTypedEventHubContract`、`TestGitRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls`、`TestGitOwnerSemanticProofEventHubReportsFeasibleThenStale`、seal/authority tests、File typed responder matrix | 覆盖八 topic 的 exact producer/strong-call/concrete payload，以及 Git owner catalog 上 state-change semantic feasibility 的真实 EventHub feasible/stale 路径 |
