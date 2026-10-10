# Run Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-30

## 功能范围与边界

`blocks/run` 是 Run lifecycle（SM-1）、Workspace run generation/lineage、replacement intent、Step、RuntimeCheckpoint、Workspace retirement fence 的唯一 owner；同时为 Orchestrator-owned Runtime aggregate 提供 opaque `RuntimeStateRecord` 的读与 expected-revision CAS。Run 只持久化 opaque Runtime bytes/ref/revision，不解释 SM-2～SM-4 语义。

生产 `Configure` 只接受完整、同一 construction binding 的 canonical `RunOwnerRepositories`（Runs、Steps、Checkpoints、Workspace purger）和 EventHub。不存在从独立 repositories 拼装 bundle 的生产 fallback；需要聚焦 repository 的测试由 `_test.go` fixture 构造 test-only bundle。

## 组件交互及合同

| 交互方 | typed command/data/result | 边界 |
| --- | --- | --- |
| `blocks/workspace` | `WorkspaceExecutionIntentCommand/Result`；prepare/finalize/purge retirement；Run lifecycle fact/inventory | Workspace 选择已获准执行意图；Run 创建/继续 successor 并独自转换 SM-1。retirement mutation 只接受 Workspace。 |
| `kernel/orchestrator` | governance wait、continuation eligible、terminal delivery facts；Runtime state read/store；recovery/retirement inventory/fact | SM-1 facts和 opaque CAS 只接受 Orchestrator；Run 不检查或修改 Runtime authority。 |
| `blocks/decision` | `ResolveRuntimeDefinitionClarificationByReplacementCommand/Result` | Run 只提交已持久化的 exact replacement binding；Decision 独占 ticket/resolution，返回 fact 经完整校验后才进入 Run-private replacement reducer，不能直接选择 SM-1 target。 |
| `blocks/memory` | `UpsertMemoryRecallTraceCommand/Result` | Run 在 committed creation 后交付 sealed activation memory selection；失败只影响可观测交付，不回滚 Run。 |
| Application/read consumers | concrete Get/List Run、Step、Checkpoint command/result | 只读 owner state；返回 detached copies。 |
| projection consumers | Run lifecycle data topics、`TopicRunContinuationRequested`、`TopicRunRuntimeCommandsReady` | 通知/恢复 handoff，不是另一个 lifecycle owner。 |

`TopicRunCommandRuntimeRecoveryInventory`、Runtime state readiness/read 和 retirement inventory/fact 在 setup 阶段提供给 Orchestrator；常规 query/mutation 在 owner start 后提供。每个 topic 使用 `pkg/events` 中的具体 result，不存在联合 envelope。

## 不变量与非职责

- SM-1 只能由 Run-private lifecycle event/reducer 转换；外部事实不得携带 caller-selected target state。
- 每个 Workspace 最多一个 current Run；generation head 单调且不因失败、清理或重启复用。
- Runtime aggregate 对 Run 是 opaque；Run 只执行 exact scope/revision CAS，不选择 next action、candidate、dispatch、evidence 或 terminal outcome。
  物理 CAS 校验仍逐项比较全部非 opaque Run 字段；新 RuntimeState 完整替换旧账本时，不深拷贝随后丢弃的旧
  RuntimeState。新账本的 detached clone、lifecycle fence 规范化和完整比较不变；不缓存成功校验，也不改变任何
  SM-1～SM-4 transition、预算、generation、fresh-load 或 Orchestrator 完整 aggregate gate。
- replacement 是 SM-1 subordinate transaction，不是第二个状态机 owner；successor 只能在 predecessor 的 exact termination/clarification binding 后物化。
- retirement fence 一旦进入 draining/retired，新的 creation、普通 SM-1 和 Step/Checkpoint mutation必须 fail closed；retired owner facts仍可供恢复校验。
- 不拥有 Workspace activation、Orchestrator SM-2/SM-3/SM-4、Decision/Approval 或 Result aggregate；不提供跨 owner repository/service。

## 进程生命周期（不改变 Runtime 状态机）

Run 使用共享 `basebiz.Lifecycle` 跟踪真实 handler、恢复扫描、提交过程和已接受后台 callback，而不是以 cancel 或 Send 等待结束代替完成。恢复 callback 在入队前登记，真实返回（包括取消 timer、排队后执行或 panic 的 defer）才退账；拒绝提交立即退账，没有 `WaitGroup.Add/Wait` 竞争。

- `Configure` 在任何 owner 初始化/订阅前由 Module 保留 Biz；Setup、Start 每项订阅成功立即记账，部分失败保留已取得资源，不能用再次 Configure 覆盖未释放 owner。
- `BeginShutdown` 非阻塞关闭新的 Start、Workspace 执行意图、continuation 及 replacement 调度，取消进程内 replacement context，但不退订、不清 repos、cancel/context/ledger，也不改变 persisted SM-1 recovery lease。
- drain 按具体操作分类：只读 inventory/readiness/Run/Step/Checkpoint、opaque Runtime CAS、已有 governance wait 和 exact terminal delivery 保持原 typed source/scope/revision/private reducer 验证。Orchestrator 的 Source 不是广泛新执行授权；Workspace 新执行及 retirement mutation 不走 drain。ready 仍描述已有 Run command 订阅，不能因 BeginShutdown 提前置 false。
- `Quiesce(ctx)` 等待本 owner 的真实在途归零；超时返回原始原因并保留依赖和句柄，可用独立预算重试。归零后仍可接收已验证的跨 owner 收尾，不能把局部 Quiesce 当全局屏障。
- final `Stop(ctx)` 只在调用方完成所有 owner 与共享 Hub/task 排空之后释放；standalone Stop 仍执行 owner-local BeginShutdown/Quiesce。最终关闭本 owner drain，逆序取消订阅，只有 Hub 成功回执后才 pop；失败/未知 panic 保留剩余 ledger 与 owner/repos。`commandMu` 不跨 Hub 操作或 handler body 持有。
- plugin 暴露 Run 专属阶段方法；void Teardown 把 Stop 错误以 `cd.WrapError` panic 交真实 manager guard，并保留 errors.Is 因果链。未知 panic 不在 Biz 改写。本批不切换进程全局 Quiescer，也不修改其他 owner 或共享 Base/task API。

影响边界是 SM-1 的进程内 admission/完成跟踪和 SM-4 依赖的 Run read/CAS/receipt 保活；不新增或改变 SM-1～SM-4 的业务 transition、authority、private transaction、预算或重启依据。

## 能力验收

| 能力/维度 | 测试证据 | 结论 |
| --- | --- | --- |
| module lifecycle、重复 Start/Stop、in-flight Stop | `module_test.go::TestModuleIdentityConfigureStartStopContract`、`module_test.go::TestModuleStopWaitsForInFlightOwnerCommand` | 已覆盖 |
| staged lifecycle、部分获取、checked retry | `plugin_lifecycle_test.go::TestRunLifecyclePartialSubscriptionsRemainReachableAndReleaseInReverse`、`TestRunLifecycleFailedUnsubscribeKeepsOwnerAndResumesLedger`、`TestRunLifecycleUnknownSubscriptionPanicPreservedWithoutLockedCleanup`、`TestRunLifecycleBeginShutdownKeepsReadinessAndAcceptedCASUntilRealHandlerExit` | Run owner 局部阶段已覆盖；不代表全进程屏障已切换或其他 owner 合规 |
| recovery 真实完成及 typed drain | `biz/owner_lifecycle_test.go::TestRunLifecycleCanceledQueuedRecoveryIsNotCompletedUntilCallbackReturns`、`TestRunLifecycleRejectedRecoverySubmissionDoesNotLeakInflight`、`TestRunLifecycleShutdownAdmitsOnlyValidatedCompletionNotNewExecution`、`TestRunLifecycleNestedSendRetainsParentContextThroughShutdown` | 排队取消不冒充完成，超时保留/重试、零 fresh recovery、terminal duplicate/source 拒绝及同 Hub nested Send 已覆盖 |
| canonical production dependency bundle | `module_test.go::TestModuleConfigureRequiresRunRepositoryAndEventHub`、`internal/pkg/repository/memory/workspace_retirement_two_phase_adversarial_test.go::TestMemoryRunOwnerBundleRejectsMixedRepositorySets`、`internal/pkg/workorchrepo/workspace_retirement_two_phase_adversarial_test.go::TestWorkspaceScopedFileRunOwnerBundleRejectsMixedStores` | 已覆盖 memory/file mixed binding rejection |
| public Run/Step/Checkpoint read scope | `module_test.go::TestRunCommandGetHonorsWorkspaceScope`、`module_test.go::TestRunStepReadCommandsHonorWorkspaceScope`、`module_test.go::TestRunCheckpointReadCommandsHonorWorkspaceScope` | 已覆盖 |
| SM-1 normal/duplicate/stale/terminal invariants | `biz/lifecycle_reducer_test.go::TestReduceActivateRunPendingToActive`、`biz/lifecycle_reducer_test.go::TestReduceActivateRunDuplicateIsNoop`、`biz/lifecycle_reducer_test.go::TestLifecycleHandlerStaleTerminalDoesNotChangeRun`、`biz/state_transitions_test.go::TestRunStateTransitionsRemainOwnedByRunBlock` | 已覆盖 |
| privileged source matrix 与零 mutation | `biz/privileged_source_test.go::TestRunSM1FactsRejectForgedSourcesWithoutStateTransition`、`biz/privileged_source_test.go::TestRunWorkspaceOwnedIngressRejectsForgedSourcesWithoutRetirementOrSuccessor`、`module_test.go::TestSM4OwnerPrivateReadCommandsRejectForgedSource` | 已覆盖；命令均为可实际转换的有效 fixture |
| restart/idempotent execution intent | `biz/workspace_execution_intent_test.go::TestWorkspaceExecutionRestartReplaysFromFilePersistenceAfterOwnerRestart`、`biz/workspace_execution_intent_test.go::TestWorkspaceExecutionRestartReplayActivatesDurablePendingCutWithoutNewRun`、`biz/workspace_execution_intent_test.go::TestWorkspaceExecutionRestartRequestIDBindsSealedPayload` | 已覆盖 |
| opaque Runtime CAS/recovery | `biz/lifecycle_reducer_test.go::TestRuntimeAggregatePersistenceRejectsAggregateRevisionRace`、`runtime_state_raw_roundtrip_test.go::TestRunStoreAndQueryPreserveOpaqueRuntimeStateRawShape`、`module_test.go::TestBuildRuntimeRecoveryInventoryContainsOnlyRunOwnedOpaqueFacts`、`pkg/models/runtime_state_cas_clone_test.go::TestOpaqueRuntimeStateCASCloneElisionPreservesEveryRunFieldBoundary`、`pkg/models/runtime_state_cas_clone_test.go::TestOpaqueRuntimeStateCASCloneElisionMatchesReferenceAndPreservesInputs` | 已覆盖；无效旧账本不因 clone 被修复，非 opaque 字段不得随 Runtime 写入变化 |
| replacement duplicate/stale/restart | `biz/replacement_intent_test.go::TestMaterializeReplacementRequiresTerminalPredecessorAndIsIdempotent`、`biz/workspace_execution_intent_test.go::TestRuntimeClarificationRejectsForgedConflictBindingWithoutStateAdvance`、`biz/replacement_termination_delivery_test.go::TestReplacementRecoveryFreshOwnerRecoversRunningGeneration` | 已覆盖 |
| replacement startup scope/order/canonical coordinates | `biz/workspace_execution_intent_test.go::TestRunConfigureScopesPendingReplacementPredecessorByWorkspace`、`biz/workspace_execution_intent_test.go::TestRunConfigureRejectsNonCanonicalPendingReplacementCoordinatesBeforeAnyCAS`、`biz/workspace_execution_intent_test.go::TestRunConfigureRejectsNonCanonicalRootActivationBeforeAnyCAS`、`biz/workspace_execution_intent_test.go::TestRunConfigureRejectsRootPendingActivationWithoutRequestBeforeAnyCASOrSM4`、`biz/workspace_execution_intent_test.go::TestRunConfigureRejectsIncompletePendingReplacementBeforeAnyCAS`、`biz/workspace_execution_intent_test.go::TestRunConfigureRetainsExhaustedPendingReplacementForOwnerRecovery`、`biz/workspace_execution_intent_test.go::TestWorkspaceExecutionRestartFindsPendingReplacementAcrossRepositoryOrders` | 已覆盖；相同 `run-001` 按 Workspace 隔离，root/linked generation 与 RunID 损坏、缺失 activation/predecessor/result lineage 均在任何 CAS 前拒绝且 pending 不进入 SM-4；合法 exhausted lease 保留 linked child 但不激活、不再调度，exact replay 仍返回 replacement-pending；repository 顺序不影响定位 |
| retirement negative/restart/read visibility | `biz/workspace_retirement_read_visibility_test.go::TestRunOwnerFileRetainRetirementHidesReadsAfterReopen`、`biz/privileged_source_test.go::TestRunWorkspaceOwnedIngressRejectsForgedSourcesWithoutRetirementOrSuccessor`、`internal/pkg/repository/memory/run_workspace_retirement_test.go::TestRunOwnerMemoryWorkspacePurgeUsesExactFenceAndPreservesHead` | 已覆盖 |
| replacement durable cutpoint 的真实进程 kill/reopen | `biz/replacement_process_kill_test.go::TestRunReplacementDurableWriteCutsRecoverAfterProcessKill`、`biz/workspace_execution_intent_test.go::TestWorkspaceExecutionRestartFindsPendingReplacementAcrossRepositoryOrders` | 已覆盖 active predecessor 主线的 intent seal、administrative recovery claim、Run-private terminal seal、termination acknowledgement、successor recovery claim/create、predecessor lineage seal、successor activation、replacement completion 九个 owner write cut；每项均 SIGKILL 子进程并由 fresh file owner 重驱，linked pending successor 在 lineage seal 前不激活、不进入 SM-4 inventory，exact restart 对 repository 顺序无关 |
