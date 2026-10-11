# Decision Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-30

## 功能范围与边界

`blocks/decision` 是 DecisionTicket、DecisionResolution、Workspace/Runtime Definition clarification binding、Runtime governance resolution proposal、交付 checkpoint 和 Workspace deletion tombstone 的唯一 owner。它负责 ticket 的 `open → resolved`、resolution identity/fingerprint、clarification 与 Runtime governance 两阶段提交后的 owner-local恢复。

Decision 的公开 surface 包含 8 个 ordinary command topics、1 个 Runtime governance open、1 个 capability acquisition 及 6 个 Definition clarification topics。每个 topic 使用独立 concrete command/result；不存在 generic ticket mutation envelope。

## 组件交互及合同

| 交互方 | typed command/data/result | 边界 |
| --- | --- | --- |
| `application/workorchd` | Get/List；Workspace clarification open/reconcile/resolve | Workspace clarification mutation 只接受 exact `application/workorchd` source。 |
| `application/workorchd/appservice` | ordinary `EnsureResolvedCommand/Result` | human/application resolution ingress 只接受 exact appservice source。 |
| `blocks/workspace` | `QueryWorkspaceDefinitionClarificationFactCommand/Result`；seal/purge Decision deletion | fact query 与 deletion 只接受 Workspace source；返回持久 ticket/resolution 的 exact binding。 |
| `kernel/orchestrator` | capability acquisition、Runtime governance open、Runtime clarification open；Decision→Orchestrator governance commit/evidence | privileged ingress 只接受 Orchestrator；Decision 不自行提交 Runtime authority。 |
| `blocks/run` | `ResolveRuntimeDefinitionClarificationByReplacementCommand/Result` | 只接受 Run 已持久化的 exact replacement binding。 |
| `kernel/governance` | scoped decision read；ordinary `EnsureResolvedCommand/Result` | Governance 可提交已授权的 ordinary resolution intent，不取得 Decision repository。 |
| Eventstream/projection consumers | `TicketChangedData`、`TicketResolvedData` | audit/display 消费；不是 Decision lifecycle owner。 |

Workspace clarification来源矩阵为 Application open/reconcile/resolve、Workspace persisted-fact query；Runtime clarification来源矩阵为 Orchestrator open、Run replacement resolve。Runtime capability catalog 只发布 `decision/request` governance kind。
生产 `Configure` 只接受通过 `Validate()` 的 canonical `DecisionOwnerRepositories` 与 EventHub。

验证输入一致性无法判断复用 Runtime governance open 和既有两阶段 resolution，标记 failure class
`validation_input_review`，不是 `revise_definition_and_restart` clarification。Orchestrator 封存准确 report/request/
input subject 和具体问题；Decision 只管理 ticket/proposal lifecycle。`provide_missing_info` 必须携带非空、完整 UTF-8、
最多 2048 bytes 的规范化 `summary`，不可用 ticket 原问题回填缺失答案；`reject_current_path` 仍表示终止当前路径。
信息没有 approval/credit 含义，Orchestrator 接纳后重新评审，Decision 不修改原 Definition 或开启替代 Run。

## 不变量与非职责

- Ticket、Resolution、clarification/proposal/checkpoint 的转换只由 Decision-private lifecycle event/reducer触发；外部 command 不得携带 target state或 owner timestamp。
- 相同 sealed identity/fingerprint 重放幂等；payload、scope、candidate revision、replacement intent 或 binding hash 漂移必须 conflict/stale。
- memory/file Ticket repository 的 Create/Update/Get/List 统一复用 Decision 模型 `CloneTicket`，隔离 `AllowedRecoveryActions`、`DefinitionClarification` 及其 `OffendingRefs`、Runtime proposal 和 payload。调用方修改写入或读取副本不得绕过 owner dispatcher 改变原票据；当前格式 file reopen 保留原正文和来源。本合同不新增决议类型、D2 批准、启动权限或 SM-1～SM-4 转换。
- Definition clarification 与 ordinary resolution 是两条显式合同；background reconciliation 不得把 clarification 降级为普通 resolved ticket。
- Runtime governance proposal 只有 Orchestrator 返回 committed/duplicate 后才能 finalize；失败保留可恢复 pending，不能假装跨 owner transaction 已完成。
- Workspace deletion fence 后普通读写/late delivery fail closed；physical purge 只由 Decision-private dispatcher触发。
- 不拥有 Run SM-1、Runtime SM-2～SM-4、Workspace admission、Approval 或 Result 状态机；不选择下一动作，不暴露 repository/service。

### 进程生命周期合同

- Biz 首次 Start 使用 `basebiz.Lifecycle` 开一代；重复 Start 不 Reset，部分订阅/任务提交失败保留逐成功 ledger，Module 持有原 owner 供检查式清理，不能直接重开失败代次。
- `BeginShutdown` 关 fresh open/reconcile/capability/deletion 并取消 recovery loop，不等待、不退订。ordinary resolution、clarification resolution 与 query 是显式 drain；原 concrete DTO、source/scope、持久 binding 校验不变，不按 Source 扩大新的 root 授权。
- `Quiesce(ctx)` 按调用预算等待真实 handler 与 accepted task 退出，timeout/cancel 不清 done/cancel/依赖、不虚构完成，可重试。task 在提交前记在途，实际任务 defer 释放。
- final `Teardown(ctx) error` 关 drain 并复核在途，逆序 checked 取消且逐成功移除 ledger；失败保留未完成项和 Module owner。未知 panic 原样上抛；锁只护本地状态，不跨 Hub I/O。
- Module BeginShutdown/Quiesce 与 plugin error guard 贯通，void Teardown 通过 `cd.WrapError` panic 传递 Stop cause，成功后才清 owner。依赖应用共享 Hub/task 排空屏障；这不是单独的全局生命周期完成声明，SM-1～SM-4、budget 与 formal ticket/proposal transition 不变。

## 能力验收

| 能力/维度 | 测试证据 | 结论 |
| --- | --- | --- |
| module lifecycle与核心依赖校验 | `module_test.go::TestModuleIdentityConfigureStartStopContract`、`module_test.go::TestModuleConfigureRequiresCoreDependencies` | 已覆盖 |
| 16-topic inventory、ordinary semantic family、bounded scope/cursor | `biz/command_router_test.go::TestDecisionOwnerCommandInventoryCoversAllSixteenTypedTopics`、`biz/command_router_test.go::TestDecisionCommandTopicsHaveOneSemanticFamily`、`module_test.go::TestDecisionTicketListsAreExplicitBoundedAndCompositeCursorPaged` | 已覆盖完整公开command inventory与ordinary query边界 |
| ordinary ticket/resolution normal/duplicate/conflict/recovery | `biz/decision_service_test.go::TestService_ResolveTicket_OpenToResolvedAndPersistsResolution`、`biz/decision_service_test.go::TestService_EnsureResolvedTicket_IsIdempotentAndRejectsResolutionMismatch`、`biz/decision_service_test.go::TestBiz_ReconciliationClosesOrdinaryResolutionWriteCut` | 已覆盖 |
| ordinary EnsureResolved exact source与零 mutation | `biz/owner_contract_test.go::TestDecisionOrdinaryResolutionAcceptsOnlyApplicationOrGovernanceSources` | 已覆盖；仅 `application/workorchd/appservice`、`kernel/governance` |
| capability 与 clarification source matrix/zero mutation | `biz/owner_contract_test.go::TestDecisionRuntimeCapabilityAcquisitionIsOrchestratorOnly`、`biz/owner_contract_test.go::TestDecisionPrivilegedIngressRejectsForgedSourcesWithoutMutation`、`module_test.go::TestRuntimeGovernanceOpenRequiresOrchestratorSourceAndMatchingWorkspaceContext` | 已覆盖；fixture 可进入真实 owner mutation path |
| Workspace clarification duplicate/provenance/stale reconcile/fact query | `biz/definition_clarification_test.go::TestOpenWorkspaceDefinitionClarificationReplayMatchesPersistedFieldsNotOnlyHash`、`biz/definition_clarification_test.go::TestWorkspaceDefinitionClarificationMutationIngressRejectsOuterTicketDriftBeforeWrite`、`biz/definition_clarification_test.go::TestWorkspaceDefinitionClarificationReconcileFencesResolutionOnlySuccessorBeforeOpeningAnotherTicket`、`biz/definition_clarification_test.go::TestWorkspaceDefinitionClarificationReconcileRejectsCorruptBoundStateBeforeMutation`、`biz/definition_clarification_test.go::TestWorkspaceDefinitionClarificationReconcileRejectsCorruptSuccessorBeforeSelectingResolution`、`biz/definition_clarification_test.go::TestWorkspaceDefinitionClarificationReconcileRejectsResolvedSuccessorWithoutResolutionBeforeMutation`、`biz/definition_clarification_test.go::TestDefinitionClarificationResolvedOpenRejectsMissingResolutionRecordBeforeNotification`、`biz/definition_clarification_test.go::TestDefinitionClarificationResolvedReplayRejectsResolutionTimestampDriftBeforeDelivery`、`biz/owner_contract_test.go::TestDecisionPrivilegedIngressRejectsForgedSourcesWithoutMutation`、`biz/definition_clarification_test.go::TestQueryWorkspaceDefinitionClarificationFactReadsExactPersistedTicketAndResolution` | 已覆盖；open/reconcile/resolve 均逐字段验证 outer Ticket、opening/lifecycle binding 及 resolved Ticket↔Resolution ID/type/payload/time；old-ticket-owned resolution 先选择唯一 successor，漂移、缺 row 或 corrupt successor 在任何新 Resolution/ticket/notification 前拒绝；已绑定 old ticket 缺 Resolution/successor 时 fail closed 且不重建 owner authority |
| Runtime governance commit failure/replay | `module_test.go::TestRuntimeGovernanceInvalidCommitResultRemainsRecoverable`、`module_test.go::TestRuntimeGovernanceResolutionDoesNotResolveTicketBeforeAggregateCommit` | 已覆盖 |
| validation-input factual resolution | `biz/runtime_input_review_test.go`、Orchestrator `biz/runtime_validation_input_review_mainline_test.go` | 真实 Decision owner open/resolve、空/超限信息零 mutation、pending recovery 禁止问题回填答案、exact duplicate、信息后重新评审、reject；不授予 validation/CWI credit |
| Runtime clarification owner replay/stale/restart | `module_test.go::TestDecisionRuntimeClarificationReplayStaleAndResolutionSurviveFileOwnerReopen`、`biz/definition_clarification_test.go::TestOpenRuntimeDefinitionClarificationRejectsPersistedSemanticDriftBeforeNotification`、`biz/definition_clarification_test.go::TestResolveRuntimeDefinitionClarificationRejectsPersistedSemanticDriftBeforeWrite`、`biz/definition_clarification_test.go::TestDefinitionClarificationResolvedOpenRejectsMissingResolutionRecordBeforeNotification`、`biz/definition_clarification_test.go::TestDefinitionClarificationResolvedReplayRejectsResolutionTimestampDriftBeforeDelivery`、`biz/owner_contract_test.go::TestDecisionPrivilegedIngressRejectsForgedSourcesWithoutMutation`、`pkg/events/definition_clarification_test.go::TestRuntimeReplacementResolutionResultValidatesExactDispositionAndBinding` | 已覆盖 open/resolve exact replay、scope stale、outer/binding/state-time 及 resolved Ticket↔Resolution row/time drift，均零 notification/零 Resolution/零 ticket mutation，以及 fresh file owner reopen |
| deletion duplicate/lifecycle/restart与seal/purge source | `module_test.go::TestDecisionWorkspaceDeletionFenceRetainAndPurgeAreStable`、`biz/workspace_deletion_delivery_test.go::TestDecisionWorkspaceDeletionRejectsForgedPurgeAndUsesPrivatePath`、`biz/workspace_deletion_delivery_test.go::TestDecisionRestartSkipsLateResolvedDeliveryAfterDeletionSeal` | 已覆盖；forged seal/purge均在owner mutation前拒绝 |
| Runtime-governance proposal/delivery file cutpoints | `biz/file_cutpoint_recovery_test.go::TestFileBackedDecisionRecoversRuntimeGovernanceWriteCutsAfterProcessKill`、`module_test.go::TestDecisionFileDeletionFenceLinearizesAcrossRepositorySets` | 已覆盖 proposal pending、resolution created、delivery finalized、evidence recorded 四个 governance durable cut 与 deletion fence 线性化 |
| Workspace Definition clarification durable cutpoints | `biz/file_cutpoint_recovery_test.go::TestFileBackedDecisionRecoversWorkspaceClarificationWriteCutsAfterProcessKill`、`biz/definition_clarification_test.go::TestWorkspaceDefinitionClarificationReconcileFencesResolutionOnlySuccessorBeforeOpeningAnotherTicket` | 已覆盖 ticket create→created marker、admission resolution create→ticket CAS→resolved marker，以及 old resolution create→successor ticket create/marker→old ticket CAS→resolved marker 的真实进程 kill/file reopen/exact replay；三个未绑定 cut 的漂移 replay 均验证零新 ticket、零 old-ticket/resolution mutation、零 notification |
| real scheduler repeated-tick recovery on reopened file backend | `module_test.go::TestDecisionReopenedFileOwnerRecoversOnRepeatedBackgroundTick` | 已覆盖fresh file owner首次失败后由真实 `BackgroundRoutine` 后续tick完成proposal/evidence交付 |
