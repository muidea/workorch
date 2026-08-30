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

## 不变量与非职责

- Ticket、Resolution、clarification/proposal/checkpoint 的转换只由 Decision-private lifecycle event/reducer触发；外部 command 不得携带 target state或 owner timestamp。
- 相同 sealed identity/fingerprint 重放幂等；payload、scope、candidate revision、replacement intent 或 binding hash 漂移必须 conflict/stale。
- Definition clarification 与 ordinary resolution 是两条显式合同；background reconciliation 不得把 clarification 降级为普通 resolved ticket。
- Runtime governance proposal 只有 Orchestrator 返回 committed/duplicate 后才能 finalize；失败保留可恢复 pending，不能假装跨 owner transaction 已完成。
- Workspace deletion fence 后普通读写/late delivery fail closed；physical purge 只由 Decision-private dispatcher触发。
- 不拥有 Run SM-1、Runtime SM-2～SM-4、Workspace admission、Approval 或 Result 状态机；不选择下一动作，不暴露 repository/service。

## 能力验收

| 能力/维度 | 测试证据 | 结论 |
| --- | --- | --- |
| module lifecycle与核心依赖校验 | `module_test.go::TestModuleIdentityConfigureStartStopContract`、`module_test.go::TestModuleConfigureRequiresCoreDependencies` | 已覆盖 |
| ordinary 8-topic semantic family、bounded scope/cursor | `biz/command_router_test.go::TestDecisionCommandTopicsHaveOneSemanticFamily`、`module_test.go::TestDecisionTicketListsAreExplicitBoundedAndCompositeCursorPaged` | 已覆盖；该测试不证明完整16-topic inventory |
| ordinary ticket/resolution normal/duplicate/conflict/recovery | `biz/decision_service_test.go::TestService_ResolveTicket_OpenToResolvedAndPersistsResolution`、`biz/decision_service_test.go::TestService_EnsureResolvedTicket_IsIdempotentAndRejectsResolutionMismatch`、`biz/decision_service_test.go::TestBiz_ReconciliationClosesOrdinaryResolutionWriteCut` | 已覆盖 |
| ordinary EnsureResolved exact source与零 mutation | `biz/owner_contract_test.go::TestDecisionOrdinaryResolutionAcceptsOnlyApplicationOrGovernanceSources` | 已覆盖；仅 `application/workorchd/appservice`、`kernel/governance` |
| capability 与 clarification source matrix/zero mutation | `biz/owner_contract_test.go::TestDecisionRuntimeCapabilityAcquisitionIsOrchestratorOnly`、`biz/owner_contract_test.go::TestDecisionPrivilegedIngressRejectsForgedSourcesWithoutMutation`、`module_test.go::TestRuntimeGovernanceOpenRequiresOrchestratorSourceAndMatchingWorkspaceContext` | 已覆盖；fixture 可进入真实 owner mutation path |
| Workspace clarification duplicate/provenance/fact query | `biz/definition_clarification_test.go::TestOpenWorkspaceDefinitionClarificationReplayMatchesPersistedFieldsNotOnlyHash`、`biz/definition_clarification_test.go::TestQueryWorkspaceDefinitionClarificationFactReadsExactPersistedTicketAndResolution` | 已覆盖；persisted owner stale reconcile未覆盖，当前revision-pair证据仅为DTO validation |
| Runtime governance commit failure/replay | `module_test.go::TestRuntimeGovernanceInvalidCommitResultRemainsRecoverable`、`module_test.go::TestRuntimeGovernanceResolutionDoesNotResolveTicketBeforeAggregateCommit` | 已覆盖 |
| Runtime clarification owner replay/restart | `pkg/events/definition_clarification_test.go::TestRuntimeReplacementResolutionResultValidatesExactDispositionAndBinding`、`biz/owner_contract_test.go`仅覆盖DTO与单次正常/source路径 | duplicate、stale、restart与file cutpoint未覆盖 |
| deletion duplicate/lifecycle/restart与purge source | `module_test.go::TestDecisionWorkspaceDeletionFenceRetainAndPurgeAreStable`、`biz/workspace_deletion_delivery_test.go::TestDecisionWorkspaceDeletionRejectsForgedPurgeAndUsesPrivatePath`、`biz/workspace_deletion_delivery_test.go::TestDecisionRestartSkipsLateResolvedDeliveryAfterDeletionSeal` | 已覆盖；forged seal source未覆盖 |
| Runtime-governance proposal/delivery file cutpoints | `biz/file_cutpoint_recovery_test.go::TestFileBackedDecisionRecoversEveryOwnerWriteCutAfterProcessKill`、`module_test.go::TestDecisionFileDeletionFenceLinearizesAcrossRepositorySets` | 仅覆盖proposal pending、resolution created、delivery finalized、evidence recorded四个governance cut；不代表全部Decision write cut |
| real scheduler repeated-tick recovery on reopened file backend | file cutpoints与手动 reconciliation 已覆盖；尚无真实 `BackgroundRoutine` 多 tick + reopen 组合测试 | 未覆盖 |
