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

## 能力验收

| 能力/维度 | 测试证据 | 结论 |
| --- | --- | --- |
| module lifecycle与核心依赖校验 | `module_test.go::TestModuleIdentityConfigureStartStopContract`、`module_test.go::TestModuleConfigureRequiresCoreDependencies` | 已覆盖 |
| typed command inventory、scope/cursor/clone | `biz/command_router_test.go::TestApprovalCommandTopicsHaveOneSemanticFamily`、`module_test.go::TestApprovalListCommandsAreTypedBoundedAndCursorValidated`、`pkg/events/messages_test.go::TestGetApprovalResultCloneBoundaryUsesDetachedOwnerState` | 已覆盖 |
| capability/open source与duplicate conflict | `module_test.go::TestApprovalPublishesRuntimeGovernanceCapabilityOnlyToOrchestrator`、`biz/runtime_governance_adapter_test.go::TestRuntimeApprovalRequestRequiresOrchestratorAndExactReplay` | 已覆盖 |
| human source-principal admission | `biz/runtime_governance_adapter_test.go::TestApprovalResolutionIngressRequiresTrustedSourceAndPrincipal` | 覆盖Application/human approved代表路径；Governance/auto正向、伪造principal及rejected topic矩阵未覆盖 |
| lifecycle normal/duplicate/opposite terminal | `biz/approval_service_test.go::TestService_ResolveApproval_PendingTransitions`、`biz/approval_service_test.go::TestService_EnsureApprovalResolution_IsIdempotentAndRejectsOppositeTerminalState`、`biz/approval_service_test.go::TestApprovalLifecycleDuplicateObservedFactDoesNotMutateOwnerState` | 已覆盖 |
| proposal/commit/evidence/audit recovery | `biz/runtime_governance_adapter_test.go::TestRuntimeApprovalProposalRecoversAfterInvalidCommitResult`、`biz/runtime_governance_adapter_test.go::TestApprovalTerminalEvidenceDeliveryRecoversWithoutReResolving`、`biz/runtime_governance_adapter_test.go::TestApprovalResolutionAuditAckFailureRecoversFromDurableTerminal` | 已覆盖 |
| deletion private path/restart与purge source | `biz/workspace_deletion_delivery_test.go::TestApprovalWorkspaceDeletionRejectsForgedPurgeAndUsesPrivatePath`、`module_test.go::TestApprovalFileDeletionFenceLinearizesAcrossRepositorySets`、`biz/workspace_deletion_delivery_test.go::TestApprovalRestartSkipsLateResolvedDeliveryAfterDeletionSeal` | 已覆盖；forged seal source未覆盖 |
| file-backed proposal/terminal delivery cutpoints | 当前 proposal/evidence/audit cutpoint使用 memory repositories；尚无 reopened file owner逐 checkpoint恢复测试 | 未覆盖 |
| Web直连Approval | 无此生产合同，按设计不适用 | 不适用 |
