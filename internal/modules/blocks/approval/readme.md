# Approval Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-30

## 功能范围与边界

`blocks/approval` 是 Approval 聚合、immutable authorization context、`pending → approved|rejected` lifecycle、Runtime resolution proposal、evidence/audit/notification checkpoint 和 Workspace deletion tombstone 的唯一 owner。它持久化批准事实并恢复未完成交付，但不执行被授权副作用。

Web capability 与 Approval owner 没有直接 EventHub query 合同。`blocks/web` 只验证 Runtime sealed dispatch 中携带的 approved grant/target hash；它不调用 `GetApprovalCommand`，也不取得 Approval repository。

## 组件交互及合同

| 交互方 | typed command/data/result | 边界 |
| --- | --- | --- |
| `kernel/orchestrator` | capability acquisition、`OpenRuntimeApprovalRequestCommand/Result`；Approval→Orchestrator resolution commit/evidence | Runtime request与capability只接受 Orchestrator；终态前先 exact commit，终态后记录 evidence。 |
| `application/workorchd/appservice` | `GetApprovalCommand → GetApprovalResult`、typed lists、EnsureApproved/EnsureRejected | human resolution source必须与 `human_operator` principal exact匹配；Application不直接修改 state。 |
| `kernel/governance` | scoped query、EnsureApproved/EnsureRejected | auto-governance source必须与 `auto_governance` principal exact匹配。 |
| `blocks/workspace` | `SealWorkspaceApprovalDeletionCommand/Result`、`PurgeWorkspaceApprovalsCommand/Result` | 唯一 deletion producer；Approval独自执行 fence/purge。 |
| `blocks/eventstream` | `RecordApprovalResolutionCommand/Result` | terminal audit ack；失败保留 checkpoint待重放。 |
| projection consumers | `ApprovalChangedData` on `TopicApprovalResolved` | display notification，不是授权或 lifecycle authority。 |

所有 query/mutation 使用 `pkg/events` concrete result；列表显式 scope、bounded cursor。`Approval.Context` 在 open 时封存；resolution只能写 `ResolutionContext`，不得替换 action/provider/URL/query 等授权目标。
生产 `Configure` 只接受通过 `Validate()` 的 canonical `ApprovalOwnerRepositories` 与 EventHub。

## 不变量与非职责

- Approval state、proposal state、terminal checkpoint 只由 Approval-private lifecycle event/reducer转换；caller不得提交 target state/timestamp。
- Runtime Approval 仅在 Orchestrator commit 返回 committed/duplicate 后进入终态；invalid/stale/failure保留可恢复 proposal或明确拒绝。
- 完全相同 identity/sealed payload重放幂等；scope、authorization hash、principal或resolution漂移必须 conflict。
- terminal后 evidence→Eventstream audit→notification 按持久 checkpoint恢复；recovery不得重新决定 approved/rejected。
- Workspace fence后普通 mutation和late delivery fail closed；purge只由 Approval-private dispatcher调用。
- 不拥有 SM-1～SM-4，不执行 Web/SSH/Shell/File副作用，不直接服务Web，也不暴露 repository/service。

## 进程生命周期合同

- Biz 持有 owner-neutral `basebiz.Lifecycle`，首次 Start 开一代；重复 Start 不 Reset，部分 Start 失败保留已获取订阅与任务句柄，Module 先保存 Biz，必须检查式清理后重新 Configure。
- `BeginShutdown` 立即关闭 fresh Runtime open/capability/deletion 入口并取消 reconciliation，不等待、不退订。既有 resolution 与只读 query 使用显式 drain 入口，仍执行原 source、principal、scope 和持久身份校验；不能因调用 Source 可信而放行新的 root。
- `Quiesce(ctx)` 等待真实 handler/任务结束；timeout/cancel 保留 done/cancel、订阅 ledger 和依赖，允许新停机预算重试。任务入队前记在途，不能在等待方超时后提前 leave。
- final `Teardown(ctx) error` 封闭 drain 后再次复核真实排空，按逆序逐项 checked 取消；只在取消成功后移除该 ledger 项。失败/未知 panic 保留 owner 与未完成依赖，未知 panic 原样上抛，不持锁跨 Hub。
- Module 暴露 BeginShutdown/Quiesce；void plugin Teardown 把 Stop 错误经 `cd.WrapError` panic 交给 framework guard，成功后才清空 Biz。final release 仍依赖应用先完成共享 Hub/任务屏障；本 owner 接线不单独宣告整进程 S1–S5 完成，不改业务 SM 或预算。

## 能力验收

| 能力/维度 | 测试证据 | 结论 |
| --- | --- | --- |
| module lifecycle与核心依赖校验 | `module_test.go::TestModuleIdentityConfigureStartStopContract`、`module_test.go::TestModuleConfigureRequiresCoreDependencies` | 已覆盖 |
| typed command inventory、scope/cursor/clone | `biz/command_router_test.go::TestApprovalCommandTopicsHaveOneSemanticFamily`、`module_test.go::TestApprovalListCommandsAreTypedBoundedAndCursorValidated`、`pkg/events/messages_test.go::TestGetApprovalResultCloneBoundaryUsesDetachedOwnerState` | 已覆盖 |
| capability/open source与duplicate conflict | `module_test.go::TestApprovalPublishesRuntimeGovernanceCapabilityOnlyToOrchestrator`、`biz/runtime_governance_adapter_test.go::TestRuntimeApprovalRequestRequiresOrchestratorAndExactReplay` | 已覆盖 |
| source-principal admission与approved/rejected矩阵 | `biz/runtime_governance_adapter_test.go::TestApprovalResolutionIngressRequiresTrustedSourceAndPrincipal`、`biz/runtime_governance_adapter_test.go::TestApprovalResolutionIngressCoversPrincipalAndTerminalMatrix` | 已覆盖Application/human、Governance/auto、approved/rejected及两类伪造principal零mutation |
| lifecycle normal/duplicate/opposite terminal | `biz/approval_service_test.go::TestService_ResolveApproval_PendingTransitions`、`biz/approval_service_test.go::TestService_EnsureApprovalResolution_IsIdempotentAndRejectsOppositeTerminalState`、`biz/approval_service_test.go::TestApprovalLifecycleDuplicateObservedFactDoesNotMutateOwnerState` | 已覆盖 |
| proposal/commit/evidence/audit recovery | `biz/runtime_governance_adapter_test.go::TestRuntimeApprovalProposalRecoversAfterInvalidCommitResult`、`biz/runtime_governance_adapter_test.go::TestApprovalTerminalEvidenceDeliveryRecoversWithoutReResolving`、`biz/runtime_governance_adapter_test.go::TestApprovalResolutionAuditAckFailureRecoversFromDurableTerminal` | 已覆盖 |
| deletion private path/restart与seal/purge source | `biz/workspace_deletion_delivery_test.go::TestApprovalWorkspaceDeletionRejectsForgedPurgeAndUsesPrivatePath`、`module_test.go::TestApprovalFileDeletionFenceLinearizesAcrossRepositorySets`、`biz/workspace_deletion_delivery_test.go::TestApprovalRestartSkipsLateResolvedDeliveryAfterDeletionSeal` | 已覆盖；forged seal/purge均在owner mutation前拒绝 |
| file-backed proposal/terminal delivery checkpoints | `biz/runtime_governance_adapter_test.go::TestApprovalFileOwnerRecoversDurableRuntimeProposalAfterReopen`、`biz/runtime_governance_adapter_test.go::TestApprovalFileOwnerRecoversTerminalDeliveryCheckpointsAfterReopen` | 已覆盖pending proposal、evidence pending、audit pending在fresh file owner上的恢复 |
| Web直连Approval | 无此生产合同，按设计不适用 | 不适用 |
