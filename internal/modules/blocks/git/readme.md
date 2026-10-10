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
| Orchestrator | Orchestrator → Git | `AcquireRuntimeCapabilities`、`AcquireRuntimeOperationAdmissibility`、`ProbeRuntimeOperationFeasibility`、`ProbeRuntimeSemanticProofFeasibility`、`QueryRuntimeCWIReconciliation`、`AcquireCapabilityState`、`ExecuteOperation` Command/Result |
| Orchestrator | Git → Orchestrator | typed runtime-operation receipt query/commit；query 只授权 exact active claim，commit 只接受完整 receipt equality |
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
- 全部七个 Runtime ingress 都是 strong call：必须先确认同步 `Result` 与 exact `kernel/orchestrator` producer，再解析 concrete
  Command；Post/nil Result、错误 producer 或错误 concrete payload 均不得触发 File query、repository query/effect 或 receipt I/O。
- 本组件不拥有 Run、Result、CWI credit、retry、governance 或 terminal 状态；Git fact/evidence 不能直接判定 DoD。
- CWI reconciliation 只可陈述 repository fact；repository revision 不能替代 check、metric、artifact 或 semantic proof。

### 生命周期与排空合同

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
| invalid + scope | option/pathspec、remote/history rewrite、parent/external gitdir、detached HEAD、expected-head、File responder matrix、`TestGitRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls` | 已覆盖主要输入/repository scope，并穷举七 topic 的 wrong source、wrong concrete payload 与 Post/nil Result 零查询/零 effect |
| duplicate + idempotency | `TestRuntimeAdapterWriteInitIsIdempotentForExistingRepository`、重复 add/no-op、`TestGitOwnerStateChangingReceiptReplaysWithoutSecondOperationAfterOwnerRecreation`、`TestGitOwnerReadReceiptReplaysAfterOwnerRecreation` | 已覆盖 read/write owner receipt 与主要 write 幂等路径 |
| stale + freshness | `TestGitOwnerCapabilityBoundAdmissibilityRejectsMismatchedProbeBindingAsStale`、`TestGitOwnerRechecksFileWorkspaceTreeBeforeFeasibilityAndExecution` | 已覆盖 catalog/File revision stale fence |
| restart / owner 重建 | `TestGitOwnerStateChangingReceiptReplaysWithoutSecondOperationAfterOwnerRecreation`、`TestGitOwnerReadReceiptReplaysAfterOwnerRecreation`、capability receipt store durable tests | 已覆盖 read/write receipt 重放和 durable capability-state 加载 |
| lifecycle | `TestGitModuleIdentityConfigureStartStopContract`、`TestGitOwnerLifecycleFencesSubscriptionsAndCapabilityDelivery`、`TestGitOwnerLifecycleCoversEveryRuntimeTopic` | 覆盖全部七 topic 的重复 Start 单次投递和 Stop 后零投递 |
| effect + receipt | resource-change、repository revision、crash/restart、state-changing receipt tests、`TestGitOwnerReadReceiptReplaysAfterOwnerRecreation` | 覆盖 write effect 与 read `effect=none` receipt 的真实 EventHub query/commit/replay |
| boundary | `TestGitOwnerAcquiresCapabilitiesThroughTypedEventHubContract`、`TestGitRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls`、`TestGitOwnerSemanticProofEventHubReportsFeasibleThenStale`、seal/authority tests、File typed responder matrix | 覆盖七 topic 的 exact producer/strong-call/concrete payload，以及 Git owner catalog 上 state-change semantic feasibility 的真实 EventHub feasible/stale 路径 |
