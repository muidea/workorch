# Memory Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-30

## 功能范围与边界

`blocks/memory` 是 MemoryItem、immutable MemoryRecallTrace 和 Workspace deletion tombstone 的唯一 owner。它负责 scoped item CRUD/search、recall trace create-once记录/查询、提交后通知，以及自身 Workspace retain/purge。

Memory purge 不是分页算法。owner-private sink 每一 pass 直接读取该 Workspace 的完整 item/trace inventory，删除后重新读取，最多执行 1000 次 fixed-point pass；公共 list/search 的 cursor/limit 不进入删除语义。

## 组件交互及合同

| 交互方 | typed command/data/result | 边界 |
| --- | --- | --- |
| `application/workorchd` | item Get/List/Search/Create/Update/Delete；recall trace List | Application选择并封存 activation memory context；Memory只管理资源事实。 |
| `blocks/run` | `UpsertMemoryRecallTraceCommand/Result` | 只接受 exact Run owner source；Run在 committed creation后交付 sealed recall evidence；exact duplicate零写入，漂移 conflict。 |
| `kernel/governance` | workspace-scoped bounded item reads | Governance不取得mutation facade。 |
| `blocks/workspace` | `SealWorkspaceMemoryDeletionCommand/Result`、`PurgeWorkspaceMemoryCommand/Result` | 唯一 deletion producer；source必须为 Workspace。 |
| Eventstream/projection consumers | `MemoryChangedData`、`MemoryRecallRecordedData` | 提交后audit/display通知；repository记录仍是authority。 |

10 个 command topics分别使用具体 result：item Create/Get/List/Search/Update/Delete、trace Upsert/List、Workspace Seal/Purge。生产构造只接受同一 backend binding 的 canonical `MemoryOwnerRepositories` 与 EventHub，不提供独立 repository fallback。

## 不变量与非职责

- 每个 item/trace command 必须显式 workspace scope；EventHub context与command scope冲突时 fail closed。
- RecallTrace是 create-once：相同完整内容重放 duplicate且不再通知，任何字段漂移 conflict。
- RecallTrace Upsert 的唯一 producer 是 `blocks/run`；其他 source 即使命令完整有效也必须零 mutation fail closed。
- 公共 list默认100/最大200，search默认20/最大100；这些分页限制不截断owner-private deletion inventory。
- Workspace fence后普通读写/late trace永久fail closed；retain物理保留但隐藏，purge只由Memory-private dispatcher调用。
- 返回 model、query collections和notification payload必须深拷贝；repository失败不得发布成功通知。
- 不拥有Runtime SM-1～SM-4，不选择recall候选/next action，不授予coverage credit，也不向其他组件暴露repository/service。

## 能力验收

| 能力/维度 | 测试证据 | 结论 |
| --- | --- | --- |
| lifecycle与canonical bundle | `module_test.go::TestModuleIdentityConfigureStartStopContract`、`module_test.go::TestModuleConfigureRequiresOwnerRepositoryBundle`、`internal/pkg/repository/resource_owner_repositories_test.go::TestResourceOwnerRepositoryBundlesRejectMixedMemorySets`、`internal/pkg/repository/resource_owner_repositories_test.go::TestResourceOwnerRepositoryBundlesValidateForMemoryAndReopenedFileBackends` | 已覆盖 memory/file binding validation |
| item CRUD/search scope/negative/bounds | `module_test.go::TestMemoryItemOwnerEnforcesWorkspaceScope`、`biz/memory_service_test.go::TestServiceCreateSearchAndValidateMemory`、`biz/memory_service_test.go::TestServiceBoundsMemoryAndRecallQueriesWithCursors`、`biz/memory_service_test.go::TestServiceRejectsInvalidMemoryVectorsAndScope` | 已覆盖 |
| trace normal/duplicate/conflict/clone | `module_test.go::TestMemoryRecallTraceCommandsAcceptContractPayloads`、`module_test.go::TestMemoryRecallTraceOwnerEnforcesScopeAndImmutableReplay`、`pkg/events/messages_test.go::TestMemoryRecallTraceResultRoundTripDoesNotShareMutableFields` | 已覆盖 |
| trace exact Run source与 forged zero mutation | `module_test.go::TestMemoryRecallTraceUpsertAcceptsOnlyRunSourceWithoutForgedMutation` | 已覆盖；正向和伪造命令均先通过 payload Validate |
| notification commit boundary | `module_test.go::TestMemoryOwnerCommandsPublishLifecycleNotifications`、`module_test.go::TestMemoryOwnerDoesNotPublishWhenRepositoryCommitFails` | 已覆盖 |
| deletion source/private path/idempotency | `module_test.go::TestMemoryWorkspaceDeletionCommandsAreWorkspaceOnly`、`module_test.go::TestMemoryWorkspaceDeletionFenceHidesRetainedDataAndPurgeIsStable`、`biz/workspace_deletion_private_dispatch_test.go::TestWorkspaceDeletionPrivateDispatcherRejectsForgedPurgeCompletion` | 已覆盖 |
| larger-than-public-page purge与file reopen | `module_test.go::TestMemoryFileOwnerPurgesInventoryLargerThanPublicPageAcrossReopen` | 已覆盖 |
| file backend workspace-local ID isolation | `module_test.go::TestMemoryRecallTraceOwnerPreservesScopeWithFileBackend` | 已覆盖 |
| trace exact replay/conflict after file owner reopen | `module_test.go::TestMemoryRecallTraceExactReplayAndConflictSurviveFileOwnerReopen` | 已覆盖stop/reopen后的duplicate、drift conflict与持久事实不变 |
| durable notification outbox | 当前通知仅是post-commit fact，outbox不属于本owner合同 | 不适用 |
