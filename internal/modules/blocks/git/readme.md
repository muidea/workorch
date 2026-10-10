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

## 本地生命周期资源合同

- `Start` 只在 SimpleObserver/Hub 实际完成订阅后逐 topic 记账；第 N 个订阅失败返回原 checked error，已成功资源和 owner 引用保留，可检查式清理或从未完成 topic 重试。
- `Biz.Teardown(ctx)` 返回 error，按账本逆序退订，每次成功才移除该项；失败或 context 取消保留剩余订阅、adapter 和 capability receipt store。`Module.Stop` 只有完整清理成功才清空同一 owner 引用，plugin 的 void `Teardown(ctx)` 将错误交给 framework guard。
- 生命周期调用由调用方串行化；账本不持有跨 Hub control action 的 owner 锁，也不替代输入停止与在途 handler 排空。最终释放 adapter/store 的前提是调用方已完成真实排空，退订完成不等于 quiescence。本组件尚未提供全局停机屏障或阶段准入 guard。
- `TestGitOwnerResourceLedgerEveryStartFailureCanTeardown`、`TestGitOwnerResourceLedgerEveryTeardownFailureRetainsDependenciesAndRetries`、`TestGitOwnerResourceLedgerCancellationRetainsUnreleasedResources` 及 Module resource-ledger tests 覆盖逐 N 故障、取消、retry 和同 owner 引用保留；这些测试不是全局生命周期完成证明。

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
