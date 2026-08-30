# Workspace Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-30

## 功能范围与边界

`blocks/workspace` 是 Workspace、Goal、DoD、WorkspaceTemplate、pre-seal `DefinitionAdmissionCandidate`、sealed startup input、activation admission、active-run 投影和 Workspace 删除 journal 的唯一 owner。它负责定义聚合的版本/CAS、完整 `all_of` admission、启动资格、执行意图准入和 Workspace 资源退役协调。

默认项目目录由 `application/workorchd` 在 setup 输入阶段解析；Workspace 只封存并校验解析后的目录，不实现 `.workorch-workspaces/<workspace-id>` 默认解析器。one-of/歧义定义只保留 pre-seal candidate，并通过 Decision 持久 clarification；不得创建 eligible admission 或 Run。

## 组件交互及合同

| 交互方 | typed command/data/result | 边界 |
| --- | --- | --- |
| `application/workorchd` | Workspace/Goal/DoD/Template CRUD；`DefinitionAdmissionIntentCommand/Result`；`WorkspaceExecutionIntentCommand/Result`；`DeleteWorkspaceCommand/Result` | Application 提交输入和意图；Workspace 决定自身聚合转换。definition admission、execution intent 只接受 Application source。 |
| `kernel/system` | `EnsureWorkspaceTemplateDefaultsCommand/Result` | builtin defaults 唯一特权 producer；只接受 `kernel/system`。 |
| `blocks/decision` | `QueryWorkspaceDefinitionClarificationFactCommand/Result` | Workspace 在 promotion 前读取 Decision 已持久化的 ticket/resolution 事实，不接受 caller 自证 binding。 |
| `blocks/run` | Workspace→Run execution/retirement/purge；Run→Workspace lifecycle fact/inventory | Workspace 管 active slot 投影；Run 管 SM-1 与 retirement 事实。 |
| `kernel/orchestrator` | `RuntimeTerminalDeliveryFact → WorkspaceLifecycleCommandResult`；Runtime quiescence command/result | terminal delivery 只接受 Orchestrator；Workspace 只映射已验证事实到自身 journal。 |
| Result/Memory/Decision/Approval/Eventstream owners | concrete seal/purge command/result | Workspace 排序协调删除；各资源 owner 独自执行自身 fence、purge 与证明。 |
| projection consumers | `WorkspaceChangedData`、`WorkspaceDeletedData`、`GoalDefinedData`、`DoDDefinedData` | 提交后通知，不是 authority。 |

公开 EventHub 合同均位于 `pkg/events`，每个 topic 使用具体 command/result；跨 owner 不暴露 repository/service。当前 `biz.New` 分别接收并校验 Workspace、Goal、DoD、Template repository、deletion cleaner 与 EventHub 非空。

## 不变量与非职责

- startup input 一经 promotion 不可被 Runtime LLM 修改；candidate/admission lineage、definition revision 与 `StartupInputHash` 必须 exact 绑定。
- Definition 仅支持完整 `all_of`；unsupported alternative 必须保留阻塞 clarification，不得降级或补默认语义。
- Workspace 的 active-run slot 只消费 Run owner fact 和 Orchestrator sealed terminal delivery；普通 Run 通知不能释放 slot。
- 删除 journal 的转换只由 Workspace-private event/reducer 触发；下游 owner result 只能作为已验证事实输入。
- 不拥有 Run SM-1、Orchestrator SM-2/SM-3/SM-4、Result 裁决、Decision/Approval 生命周期或 capability 执行。
- 不解析默认项目目录，不读取 capability catalog，不选择 Runtime operation/target，也不跨 owner 直接清理资源。

## 能力验收

| 能力/维度 | 测试证据 | 结论 |
| --- | --- | --- |
| module lifecycle与依赖非空校验 | `module_test.go::TestModuleIdentityConfigureStartStopContract`、`module_test.go::TestModuleConfigureRequiresAllRepositories` | 已覆盖；不证明依赖同 backend |
| Workspace create/get；Goal/DoD scope与definition revision；Template mapping clone | `biz/goal_dod_manager_test.go::TestGoalDoDManager_CreateWorkspace`、`biz/goal_dod_manager_test.go::TestGoalDoDManager_DefinitionRevisionIsMonotonicAcrossContentReversion`、`biz/command_router_test.go::TestWorkspaceDefinitionCommandsEnforceScopeAcrossBackends`、`biz/template_commands_test.go::TestWorkspaceTemplateOwnerMappingsDeepCloneCommandAndRepositoryValues` | 已覆盖这些明确路径 |
| Workspace update/successful delete/list、Goal/DoD successful delete/list、Template CRUD | 当前引用测试没有覆盖完整公开 CRUD 正向路径 | 未覆盖 |
| Definition normal/unsupported/duplicate/stale/provenance | `biz/definition_admission_test.go::TestDefinitionAdmissionIntentAutomaticallyPromotesEligibleCandidate`、`biz/definition_admission_test.go::TestDefinitionAdmissionIntentKeepsUnsupportedAlternativePreseal`、`biz/definition_admission_test.go::TestDefinitionAdmissionRejectsSelfConsistentUnpersistedDecisionTicketWithoutPromotion`、`biz/definition_admission_test.go::TestDefinitionAdmissionRejectsStaleLineageCAS` | 已覆盖 |
| privileged source matrix 与零 mutation | `biz/privileged_source_test.go::TestWorkspacePrivilegedIngressRejectsForgedSourcesWithoutOwnerMutation`、`biz/privileged_source_test.go::TestWorkspaceTemplateDefaultsIngressAcceptsOnlySystemOwner` | 已覆盖 |
| execution normal/duplicate/stale/restart | `biz/execution_intent_test.go::TestWorkspaceExecutionIntentDuplicateReplayUsesActiveRunAndRejectsResultDrift`、`biz/execution_intent_test.go::TestWorkspaceExecutionInternalEventRejectsStaleStartupWithoutMutation`、`biz/execution_intent_test.go::TestWorkspaceRestartMaterializesDefaultSelectionByPrivateEventBeforeRunOwner` | 已覆盖 |
| Run projection lost/duplicate/stale/restart | `biz/run_projection_reconciliation_test.go::TestWorkspaceRunProjectionFreshOwnerRecoversWithoutHistoricalNotification`、`biz/run_projection_reconciliation_test.go::TestWorkspaceRunProjectionRevisionFenceRejectsStaleAndConflictingFacts`、`biz/lifecycle_reducer_test.go::TestReduceRunProjectionFactDuplicateIsNoop` | 已覆盖 |
| deletion normal/negative/idempotency/restart | `biz/deletion_coordinator_test.go::TestWorkspaceDeletionCoordinatorUsesExactRetainAndPurgeOrder`、`biz/deletion_internal_event_test.go::TestWorkspaceDeletionRejectsOutOfOrderAndMismatchedOwnerAcknowledgements`、`biz/deletion_coordinator_test.go::TestWorkspaceDeletionResponseLossResumesFromPersistedPhaseOnExactDeleteRetry`、`biz/deletion_coordinator_test.go::TestWorkspaceDeletionFreshOwnerStartRecoversPersistedFileFinalization` | 已覆盖 |
| repository backend construction binding | 当前构造与本组件测试均未证明五个 repository/cleaner 来自同一 backend | 未覆盖 |
| file-backed admission/clarification cutpoints | 尚无覆盖 candidate 写入、Decision fact 确认和 promotion 之间每个 file cutpoint 的进程级恢复测试 | 未覆盖 |
