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
| `blocks/memory` | `UpsertMemoryRecallTraceCommand/Result` | Run 在 committed creation 后交付 sealed activation memory selection；失败只影响可观测交付，不回滚 Run。 |
| Application/read consumers | concrete Get/List Run、Step、Checkpoint command/result | 只读 owner state；返回 detached copies。 |
| projection consumers | Run lifecycle data topics、`TopicRunContinuationRequested`、`TopicRunRuntimeCommandsReady` | 通知/恢复 handoff，不是另一个 lifecycle owner。 |

`TopicRunCommandRuntimeRecoveryInventory`、Runtime state readiness/read 和 retirement inventory/fact 在 setup 阶段提供给 Orchestrator；常规 query/mutation 在 owner start 后提供。每个 topic 使用 `pkg/events` 中的具体 result，不存在联合 envelope。

## 不变量与非职责

- SM-1 只能由 Run-private lifecycle event/reducer 转换；外部事实不得携带 caller-selected target state。
- 每个 Workspace 最多一个 current Run；generation head 单调且不因失败、清理或重启复用。
- Runtime aggregate 对 Run 是 opaque；Run 只执行 exact scope/revision CAS，不选择 next action、candidate、dispatch、evidence 或 terminal outcome。
- replacement 是 SM-1 subordinate transaction，不是第二个状态机 owner；successor 只能在 predecessor 的 exact termination/clarification binding 后物化。
- retirement fence 一旦进入 draining/retired，新的 creation、普通 SM-1 和 Step/Checkpoint mutation必须 fail closed；retired owner facts仍可供恢复校验。
- 不拥有 Workspace activation、Orchestrator SM-2/SM-3/SM-4、Decision/Approval 或 Result aggregate；不提供跨 owner repository/service。

## 能力验收

| 能力/维度 | 测试证据 | 结论 |
| --- | --- | --- |
| module lifecycle、重复 Start/Stop、in-flight Stop | `module_test.go::TestModuleIdentityConfigureStartStopContract`、`module_test.go::TestModuleStopWaitsForInFlightOwnerCommand` | 已覆盖 |
| canonical production dependency bundle | `module_test.go::TestModuleConfigureRequiresRunRepositoryAndEventHub`、`internal/pkg/repository/memory/workspace_retirement_two_phase_adversarial_test.go::TestMemoryRunOwnerBundleRejectsMixedRepositorySets`、`internal/pkg/workorchrepo/workspace_retirement_two_phase_adversarial_test.go::TestWorkspaceScopedFileRunOwnerBundleRejectsMixedStores` | 已覆盖 memory/file mixed binding rejection |
| public Run/Step/Checkpoint read scope | `module_test.go::TestRunCommandGetHonorsWorkspaceScope`、`module_test.go::TestRunStepReadCommandsHonorWorkspaceScope`、`module_test.go::TestRunCheckpointReadCommandsHonorWorkspaceScope` | 已覆盖 |
| SM-1 normal/duplicate/stale/terminal invariants | `biz/lifecycle_reducer_test.go::TestReduceActivateRunPendingToActive`、`biz/lifecycle_reducer_test.go::TestReduceActivateRunDuplicateIsNoop`、`biz/lifecycle_reducer_test.go::TestLifecycleHandlerStaleTerminalDoesNotChangeRun`、`biz/state_transitions_test.go::TestRunStateTransitionsRemainOwnedByRunBlock` | 已覆盖 |
| privileged source matrix 与零 mutation | `biz/privileged_source_test.go::TestRunSM1FactsRejectForgedSourcesWithoutStateTransition`、`biz/privileged_source_test.go::TestRunWorkspaceOwnedIngressRejectsForgedSourcesWithoutRetirementOrSuccessor`、`module_test.go::TestSM4OwnerPrivateReadCommandsRejectForgedSource` | 已覆盖；命令均为可实际转换的有效 fixture |
| restart/idempotent execution intent | `biz/workspace_execution_intent_test.go::TestWorkspaceExecutionRestartReplaysFromFilePersistenceAfterOwnerRestart`、`biz/workspace_execution_intent_test.go::TestWorkspaceExecutionRestartReplayActivatesDurablePendingCutWithoutNewRun`、`biz/workspace_execution_intent_test.go::TestWorkspaceExecutionRestartRequestIDBindsSealedPayload` | 已覆盖 |
| opaque Runtime CAS/recovery | `biz/lifecycle_reducer_test.go::TestRuntimeAggregatePersistenceRejectsAggregateRevisionRace`、`runtime_state_raw_roundtrip_test.go::TestRunStoreAndQueryPreserveOpaqueRuntimeStateRawShape`、`module_test.go::TestBuildRuntimeRecoveryInventoryContainsOnlyRunOwnedOpaqueFacts` | 已覆盖 |
| replacement duplicate/stale/restart | `biz/replacement_intent_test.go::TestMaterializeReplacementRequiresTerminalPredecessorAndIsIdempotent`、`biz/workspace_execution_intent_test.go::TestRuntimeClarificationRejectsForgedConflictBindingWithoutStateAdvance`、`biz/replacement_termination_delivery_test.go::TestReplacementRecoveryFreshOwnerRecoversRunningGeneration` | 已覆盖 |
| retirement negative/restart/read visibility | `biz/workspace_retirement_read_visibility_test.go::TestRunOwnerFileRetainRetirementHidesReadsAfterReopen`、`biz/privileged_source_test.go::TestRunWorkspaceOwnedIngressRejectsForgedSourcesWithoutRetirementOrSuccessor`、`internal/pkg/repository/memory/run_workspace_retirement_test.go::TestRunOwnerMemoryWorkspacePurgeUsesExactFenceAndPreservesHead` | 已覆盖 |
| replacement 每个 durable cutpoint 的真实进程 kill/reopen | 当前覆盖 fresh-owner remount 和 fault injection，未覆盖每个 replacement write cut 的子进程 kill matrix | 未覆盖 |
