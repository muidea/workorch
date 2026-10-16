# Result Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-09-10

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

后台 Timer 注册失败由 `Start` 原样返回；已经取得的订阅、service 与 cancel 句柄必须保留用于清理，不能把任务提交失败当作启动成功。失败后先完成 owner 清理再重新启动。该任务错误合同不单独证明完整的进程输入／排空／释放屏障。

### 生命周期合同

- 10 项订阅逐成功登记；部分订阅或 Timer 注册失败保留 service、cancel 与账本，未清理前再次 Start 返回冲突，不把部分启动当作完整成功。退订成功才移除对应项。
- BeginShutdown 非等待式关闭新删除和新 recovery 调度并取消 Timer context，保留只读查询及 exact Orchestrator terminal delivery；原 scope/source/sealed material/fence 校验不变。
- Start 与已进入 recovery callback、真实 handler 持票直到 repository、锁等待及通知发布返回；调用方取消、Timer 取消和队列提交成功都不等于真实退出。关闭后迟到 queued callback 不再访问 repository；共享 Timer goroutine/队列最终由 BackgroundRoutine.Shutdown 排空，本地 Quiesce 不能替代该屏障。
- Stop 超时保留 owner、cancel 与依赖；最终封全部入站并再次排空，逆序 checked 退订全部成功才清 Module 引用。Configure 不能覆盖未释放 owner，plugin guard 保留 cause，未知 panic 不吞且释放清理 claim。
- 调用方串行 Configure/Start/Stop；不更改 SM-1～SM-4、终态材料或 credit。`biz/owner_lifecycle_test.go` 与 `module_lifecycle_test.go` 覆盖订阅切点、超时重试、callback 实际退出/迟到拒绝及 framework guard。

## 不变量与非职责

- Result/Artifact scope 必须与 terminal material 的 workspace/run/result 完全一致；返回值和 notification 必须深拷贝。
- formal closeout、material hash、artifact ref 映射和 terminal delivery identity 一经封存不可漂移。
  terminal ingress 在任何规范化前校验 closeout 原值；规范化可消除的空白、重复集合或 hash 漂移不得被当成合法材料。
  Result acknowledgement 携带 owner 实际保存的 detached `ResultCloseout`，noop 重放返回同一材料；该回执不是当前 Runtime
  authority 的替代品。回执和 opaque record 的原值克隆保留 bytes 与 nil/empty 身份，不重新派生历史材料。
- 原始 owner diagnostic、逐项 DoD 与实现/验证/限制/风险反馈是 Orchestrator 已裁决的 sealed material；Result 不根据
  diagnostic 选择 retry 或重判验收。Result renderer 版本只由 Orchestrator 在 Run 初始化时封存；同版本 pending delivery
  必须复用原 material，不能由 Result owner 在重放时补写新字段或重算另一份摘要。
- 新 Run 的 Result feedback v5 保留 Orchestrator 已封存的 blocking input-review batch 逐 check/obligation 事实，
  包括输入 refs、subject/report hash、原 CWI/requirement、结论及原因；原报告按原值保全，由受限读取边界保护。
  terminal 逐 evidence 另保全原 owner 读取快照（包括 rejected/stale 项），正文进入 closeout/material hash。
  原 snapshot 不等于当前 owner 观察；Result 不合并版本、不修复冲突、不增加 credit，返回值与 ACK 均 detached。
  Result 只存储和返回这份 typed material，不读原输入、不再次调用 LLM、不产生重试、审批或验收 credit。
  feedback v0/v1/v2/v3 aggregate 在 Runtime 入口拒绝，不自动升级、回填或重驱旧归档。
- Result 状态同状态重放幂等；`draft → accepted|rejected`，已终态只允许 owner-internal `→ superseded`，不存在外围 generic update/delete。
- Workspace tombstone 后普通读写永久 fail closed；purge 物理 sink 只能由 Result-private deletion dispatcher 调用。
- 不拥有 evidence quality、DoD coverage credit、terminal outcome 选择、Run SM-1 或 Runtime SM-2～SM-4。
- 不从 Runtime authority/repository反向重建裁决，也不向其他组件暴露 Result repository/service。

## 能力验收

| 能力/维度 | 测试证据 | 结论 |
| --- | --- | --- |
| module lifecycle、依赖校验、重复 Run | `module_test.go::TestModuleIdentityConfigureStartStopContract`、`module_test.go::TestModuleSetupRequiresRepositories`、`module_test.go::TestModuleRunIsIdempotent` | 已覆盖 |
| Timer 注册错误与部分启动资源保留 | `biz/lifecycle_handler_test.go::TestResultStartReturnsTimerFailureAndRetainsPartialStartCleanup` | 返回原始提交错误；保留并清理已取得订阅和 cancel |
| owner-private terminal materialization；public Result/Artifact query、scope、clone、list bound | `module_test.go::TestModuleManagesResultsAndArtifacts`、`module_test.go::TestResultOwnerListContractsAreBoundedAndCursorBased`、`biz/result_service_test.go::TestServiceEnforcesArtifactParentScope`、`pkg/events/messages_test.go::TestCloneResultAndArtifactIsolateEventHubBoundary` | 已覆盖；测试中的 seed 不是 public CRUD |
| terminal normal/duplicate/material drift/partial recovery | `biz/lifecycle_handler_test.go::TestTerminalDeliveryDerivesCanonicalResultAndRejectsMaterialDrift`、`biz/lifecycle_handler_test.go::TestTerminalDeliveryRecoversDraftAndPartialArtifactMaterialization`、`biz/lifecycle_handler_test.go::TestTerminalDeliveryReplayReemitsStableNotification` | 已覆盖 |
| privileged terminal/seal/purge sources与零 mutation | `biz/lifecycle_handler_test.go::TestResultPrivilegedIngressRejectsForgedSourcesWithoutOwnerMutation` | 已覆盖 |
| transition negative/material drift | `biz/result_service_test.go::TestResultTransitionMatrixRejectsTerminalRegression`、`biz/result_service_test.go::TestServiceRejectsTerminalMaterialMutation` | 已覆盖；public contract无 revision-based stale mutation，stale维度不适用 |
| deletion retain/purge/idempotency/private dispatcher | `module_test.go::TestResultWorkspaceDeletionFenceHidesRetainedDataAndPurgeIsStable`、`biz/workspace_deletion_private_dispatch_test.go::TestWorkspaceDeletionPhysicalPurgeUsesOwnerPrivateDispatcher`、`biz/workspace_deletion_private_dispatch_test.go::TestWorkspaceDeletionPrivateDispatcherRejectsForgedPurgeCompletion` | 已覆盖 |
| durable terminal notification 的 fresh-owner Start replay | `biz/lifecycle_handler_test.go::TestResultFileOwnerReplaysTerminalAndResumesDeletionAfterReopen`、`biz/lifecycle_handler_test.go::TestResultOwnerStartReplaysDurableTerminalNotification` | 已覆盖；前者第二个 file owner 显式 Start |
| content boundary/limit/hash | `biz/result_service_test.go::TestServiceReadsBoundedArtifactContentFromOwnerState`、`pkg/events/messages_test.go::TestReadArtifactContentResultIsBoundedHashedAndCloned` | 已覆盖 |
| deletion restart via fresh Module.Start + EventHub ingress | `module_test.go::TestResultWorkspaceDeletionResumesThroughFreshModuleAndEventHubAfterFileReopen` | 已覆盖重开file backend、fresh Module.Start与typed EventHub purge |
| Artifact 多子项 durable write cut 的真实进程 kill/reopen | `biz/lifecycle_process_kill_test.go::TestResultArtifactDurableWriteCutsRecoverAfterProcessKill` | 已覆盖 draft Result、每个 child Artifact create、terminal Result CAS；每项均 SIGKILL 子进程并由 fresh file owner 重放 exact terminal delivery，验证无重复、无材料漂移与单次 draft→terminal |
