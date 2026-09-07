# Result Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-30

## 功能范围与边界

`blocks/result` 是 Result 聚合及其 Artifact 子实体、terminal material binding、Workspace deletion tombstone 的唯一 owner。它从 Orchestrator 已裁决并封存的 terminal delivery 物化 canonical Result/Artifact，维护状态/identity/幂等 fence，提供 bounded query/content read，并执行自身 Workspace-scoped retain/purge。

持久模型允许 `draft`、`accepted`、`rejected`、`superseded`；Runtime terminal ingress 仅将 completed 映射为 accepted，将 failed/cancelled/superseded outcome 映射为 rejected。`draft` 仅是 Artifact 物化期间的 owner-private cutpoint。四个 data topic `created/accepted/rejected/superseded` 描述 Result 模型生命周期/恢复重放，不能等同四种 Runtime terminal outcome。

## 组件交互及合同

| 交互方 | typed command/data/result | 边界 |
| --- | --- | --- |
| `kernel/orchestrator` | `RuntimeTerminalDeliveryFact → ResultLifecycleCommandResult` | 唯一 terminal producer；source 必须为 Orchestrator。Result 校验 sealed material，但不重判 evidence/DoD。 |
| `blocks/workspace` | `SealWorkspaceResultDeletionCommand/Result`、`PurgeWorkspaceResultsCommand/Result` | 唯一 deletion producer；Result 独自 fence、fixed-point purge、证明和 replay。 |
| Application/read consumers | Get/List Result；List Artifact by workspace/run/result；`ReadArtifactContentCommand/Result` | 返回 detached typed value；content read 有 scope、大小和 hash 校验，不暴露 path/repository。 |
| projection consumers | `ResultChangedData` on `created/accepted/rejected/superseded` | 提交后或启动恢复通知，不授予 authority。 |

canonical Result ID 由 workspace/run 派生；Artifact ID 由 Result ID 与 sealed source ref 派生。相同 terminal identity/material 是幂等重放，任一 identity 或 material 漂移均 conflict。每个 EventHub topic 使用 `pkg/events` 中的具体 result。
生产 `Configure` 只接受通过 `Validate()` 的 canonical `ResultOwnerRepositories` 与 EventHub；公开面没有 generic create/update/delete。

## 不变量与非职责

- Result/Artifact scope 必须与 terminal material 的 workspace/run/result 完全一致；返回值和 notification 必须深拷贝。
- formal closeout、material hash、artifact ref 映射和 terminal delivery identity 一经封存不可漂移。
- 原始 owner diagnostic、逐项 DoD 与实现/验证/限制/风险反馈是 Orchestrator 已裁决的 sealed material；Result 不根据
  diagnostic 选择 retry 或重判验收。Result renderer 版本只由 Orchestrator 在 Run 初始化时封存；旧 Run pending delivery
  必须复用旧 material，不能由 Result owner 在升级或重放时补写新字段或重算另一份摘要。
- Result 状态同状态重放幂等；`draft → accepted|rejected`，已终态只允许 owner-internal `→ superseded`，不存在外围 generic update/delete。
- Workspace tombstone 后普通读写永久 fail closed；purge 物理 sink 只能由 Result-private deletion dispatcher 调用。
- 不拥有 evidence quality、DoD coverage credit、terminal outcome 选择、Run SM-1 或 Runtime SM-2～SM-4。
- 不从 Runtime authority/repository反向重建裁决，也不向其他组件暴露 Result repository/service。

## 能力验收

| 能力/维度 | 测试证据 | 结论 |
| --- | --- | --- |
| module lifecycle、依赖校验、重复 Run | `module_test.go::TestModuleIdentityConfigureStartStopContract`、`module_test.go::TestModuleSetupRequiresRepositories`、`module_test.go::TestModuleRunIsIdempotent` | 已覆盖 |
| owner-private terminal materialization；public Result/Artifact query、scope、clone、list bound | `module_test.go::TestModuleManagesResultsAndArtifacts`、`module_test.go::TestResultOwnerListContractsAreBoundedAndCursorBased`、`biz/result_service_test.go::TestServiceEnforcesArtifactParentScope`、`pkg/events/messages_test.go::TestCloneResultAndArtifactIsolateEventHubBoundary` | 已覆盖；测试中的 seed 不是 public CRUD |
| terminal normal/duplicate/material drift/partial recovery | `biz/lifecycle_handler_test.go::TestTerminalDeliveryDerivesCanonicalResultAndRejectsMaterialDrift`、`biz/lifecycle_handler_test.go::TestTerminalDeliveryRecoversDraftAndPartialArtifactMaterialization`、`biz/lifecycle_handler_test.go::TestTerminalDeliveryReplayReemitsStableNotification` | 已覆盖 |
| privileged terminal/seal/purge sources与零 mutation | `biz/lifecycle_handler_test.go::TestResultPrivilegedIngressRejectsForgedSourcesWithoutOwnerMutation` | 已覆盖 |
| transition negative/material drift | `biz/result_service_test.go::TestResultTransitionMatrixRejectsTerminalRegression`、`biz/result_service_test.go::TestServiceRejectsTerminalMaterialMutation` | 已覆盖；public contract无 revision-based stale mutation，stale维度不适用 |
| deletion retain/purge/idempotency/private dispatcher | `module_test.go::TestResultWorkspaceDeletionFenceHidesRetainedDataAndPurgeIsStable`、`biz/workspace_deletion_private_dispatch_test.go::TestWorkspaceDeletionPhysicalPurgeUsesOwnerPrivateDispatcher`、`biz/workspace_deletion_private_dispatch_test.go::TestWorkspaceDeletionPrivateDispatcherRejectsForgedPurgeCompletion` | 已覆盖 |
| durable terminal notification 的 fresh-owner Start replay | `biz/lifecycle_handler_test.go::TestResultFileOwnerReplaysTerminalAndResumesDeletionAfterReopen`、`biz/lifecycle_handler_test.go::TestResultOwnerStartReplaysDurableTerminalNotification` | 已覆盖；前者第二个 file owner 显式 Start |
| content boundary/limit/hash | `biz/result_service_test.go::TestServiceReadsBoundedArtifactContentFromOwnerState`、`pkg/events/messages_test.go::TestReadArtifactContentResultIsBoundedHashedAndCloned` | 已覆盖 |
| deletion restart via fresh Module.Start + EventHub ingress | `module_test.go::TestResultWorkspaceDeletionResumesThroughFreshModuleAndEventHubAfterFileReopen` | 已覆盖重开file backend、fresh Module.Start与typed EventHub purge |
| Artifact 多子项 durable write cut 的真实进程 kill/reopen | `biz/lifecycle_process_kill_test.go::TestResultArtifactDurableWriteCutsRecoverAfterProcessKill` | 已覆盖 draft Result、每个 child Artifact create、terminal Result CAS；每项均 SIGKILL 子进程并由 fresh file owner 重放 exact terminal delivery，验证无重复、无材料漂移与单次 draft→terminal |
