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
| `kernel/system` | `EnsureWorkspaceTemplateDefaultsCommand/Result` | startup ensure 唯一 producer；owner 创建缺失资源并自动更新 exact reviewed 未定制 Template；只接受 `kernel/system`。 |
| `application/workorchd` | `QueryBuiltinWorkspaceTemplateDefaultsCommand/Result`、`UpgradeBuiltinWorkspaceTemplateDefaultCommand/Result` | exact `application/workorchd/appservice`；只操作 Template catalog，不改变已 admission Workspace 或 Run。 |
| `blocks/decision` | `QueryWorkspaceDefinitionClarificationFactCommand/Result` | Workspace 在 promotion 前读取 Decision 已持久化的 ticket/resolution 事实，不接受 caller 自证 binding。 |
| `blocks/run` | Workspace→Run execution/retirement/purge；Run→Workspace lifecycle fact/inventory | Workspace 管 active slot 投影；Run 管 SM-1 与 retirement 事实。 |
| `kernel/orchestrator` | `RuntimeTerminalDeliveryFact → WorkspaceLifecycleCommandResult`；Runtime quiescence command/result | terminal delivery 只接受 Orchestrator；Workspace 只映射已验证事实到自身 journal。 |
| Result/Memory/Decision/Approval/Eventstream owners | concrete seal/purge command/result | Workspace 排序协调删除；各资源 owner 独自执行自身 fence、purge 与证明。 |
| projection consumers | `WorkspaceChangedData`、`WorkspaceDeletedData`、`GoalDefinedData`、`DoDDefinedData` | 提交后通知，不是 authority。 |

公开 EventHub 合同均位于 `pkg/events`，每个 topic 使用具体 command/result；跨 owner 不暴露 repository/service。`biz.New` 只接收 backend factory 生成的 `WorkspaceOwnerRepositories` aggregate；Workspace、Goal、DoD、Template repository 与 deletion cleaner 必须携带同一个 opaque owner binding，独立构造、跨 backend 或跨 factory 混装在 owner 启动前拒绝。

## 不变量与非职责

- startup input 一经 promotion 不可被 Runtime LLM 修改；candidate/admission lineage、definition revision 与 `StartupInputHash` 必须 exact 绑定。
- 新 Start/Restart 自动加载当前 Template/Soul。Application 提交带 `CurrentTemplate`、expected Workspace revision/configuration
  hash 的新 candidate；Workspace-private admission event 持有 Template mutation lock，复核当前 Template hash，物化变化的
  template-bound 字段并 CAS 追加新 candidate/promotion。Goal/DoD、目录、provider selection、生命周期与旧 Run 不变。
  `TemplateBindingHash` 是 live authoring 版本标记，不是 Run seal；同 Template 保留 Workspace 配置，变化的 Template 整体
  替换旧绑定策略，包括移除字段。无需手动 refresh；详见 [automatic configuration loading](../../../../docs/40-profiles-capabilities/workspace/builtin-default-resources.md#automatic-configuration-loading)。
- 已准入历史只检查 scope/identity/lineage，不用当前 schema/compiler 重新准入；candidate clone 必须原样 detached，不做
  normalization、补字段或 rehash。新输入严格使用当前 compiler，历史身份损坏与未解决 clarification 不能被自动更新绕过。
  Resume/recovery 不进入此 authoring 事务，SM-1～SM-4 仍消费原 Run seal。
- Definition 仅支持完整 `all_of`；unsupported alternative 必须保留阻塞 clarification，不得降级或补默认语义。
- Workspace 的 active-run slot 只消费 Run owner fact 和 Orchestrator sealed terminal delivery；普通 Run 通知不能释放 slot。
- `RunProjectionCursor` 是 Workspace owner-private delivery fence：每个更高 Run/Runtime revision 都必须持久化，但只有
  `WorkspaceChangedData` 的公开字段实际变化时才发布 `WorkspaceStateChanged`。cursor-only CAS 不得伪装成公开 Workspace
  状态变化；workorchd 继续直接消费 typed Run notification 更新运行视图。
- 删除 journal 的转换只由 Workspace-private event/reducer 触发；下游 owner result 只能作为已验证事实输入。
- Template builtin upgrade 只进入 `workspaceTemplateUpgradeEvent → submitBuiltinTemplateUpgrade`；比较与替换在同一
  owner-local lock 内，并与普通 Template CRUD/ensure 串行。current full-content hash 必须精确命中 reviewed historical
  fingerprint 与 caller expected hash，target hash 必须等于当前内嵌值。未知/定制/stale 内容拒绝；已达目标的 exact replay
  不写入。此私有 catalog 事务不修改 definition admission、active-run slot 或 SM-1～SM-4。
  内置 adapter 升版必须登记此前发布内容的精确指纹，并以完整 frozen fixture 验证自动更新及定制保护；
  `TestEntrypointTemplateUpgradeReopensReviewedPreviousDefault` 覆盖 memory/file reopen，不凭内置 ID 扩大升级范围。
- 不拥有 Run SM-1、Orchestrator SM-2/SM-3/SM-4、Result 裁决、Decision/Approval 生命周期或 capability 执行。
- 不解析默认项目目录，不读取 capability catalog，不选择 Runtime operation/target，也不跨 owner 直接清理资源。

## 能力验收

| 能力/维度 | 测试证据 | 结论 |
| --- | --- | --- |
| module lifecycle与 owner repository binding | `module_test.go::TestModuleIdentityConfigureStartStopContract`、`module_test.go::TestModuleConfigureRequiresBoundOwnerRepositoriesAndEventHub`、`internal/pkg/repository/resource_owner_repositories_test.go::TestResourceOwnerRepositoryBundlesRejectMixedMemorySets`、`TestResourceOwnerRepositoryBundlesValidateForMemoryAndReopenedFileBackends` | 已覆盖必需 aggregate/EventHub、memory/file factory 正向构造、zero/unbound/mixed set 启动前拒绝与 file reopen binding 重建 |
| Workspace create/get；Goal/DoD scope与definition revision；Template mapping clone | `biz/goal_dod_manager_test.go::TestGoalDoDManager_CreateWorkspace`、`biz/goal_dod_manager_test.go::TestGoalDoDManager_DefinitionRevisionIsMonotonicAcrossContentReversion`、`biz/command_router_test.go::TestWorkspaceDefinitionCommandsEnforceScopeAcrossBackends`、`biz/template_commands_test.go::TestWorkspaceTemplateOwnerMappingsDeepCloneCommandAndRepositoryValues` | 已覆盖这些明确路径 |
| Workspace update/list、Goal/DoD update/delete/list、Template CRUD | `biz/command_router_test.go::TestWorkspacePublicManagementCRUDPositivePaths` | 已覆盖；Workspace coordinated delete由下方deletion矩阵验收 |
| Template explicit upgrade | [`biz/builtin_defaults_test.go`](biz/builtin_defaults_test.go) | memory/file、seed 不覆盖旧版本、错误 source/Post、未知/定制/stale/changed-target 拒绝、写后响应丢失与 fresh-handle replay、并发普通编辑保护 |
| automatic configuration / archive isolation | [`biz/current_configuration_admission_test.go`](biz/current_configuration_admission_test.go)、[`biz/defaults_test.go`](biz/defaults_test.go)、[`pkg/models/template_test.go`](pkg/models/template_test.go) | file reopen、旧 schema 原样保存、当前配置准入与幂等、无效配置阻塞、并发 fence、Soul fresh read、历史 Run 零改写、未定制内置 Template 自动更新 |
| Definition normal/unsupported/duplicate/stale/provenance | `biz/definition_admission_test.go::TestDefinitionAdmissionIntentAutomaticallyPromotesEligibleCandidate`、`biz/definition_admission_test.go::TestDefinitionAdmissionIntentKeepsUnsupportedAlternativePreseal`、`biz/definition_admission_test.go::TestDefinitionAdmissionRejectsSelfConsistentUnpersistedDecisionTicketWithoutPromotion`、`biz/definition_admission_test.go::TestDefinitionAdmissionRejectsStaleLineageCAS` | 已覆盖 |
| privileged source matrix 与零 mutation | `biz/privileged_source_test.go::TestWorkspacePrivilegedIngressRejectsForgedSourcesWithoutOwnerMutation`、`biz/privileged_source_test.go::TestWorkspaceTemplateDefaultsIngressAcceptsOnlySystemOwner` | 已覆盖 |
| execution normal/duplicate/stale/restart | `biz/execution_intent_test.go::TestWorkspaceExecutionIntentDuplicateReplayUsesActiveRunAndRejectsResultDrift`、`biz/execution_intent_test.go::TestWorkspaceExecutionInternalEventRejectsStaleStartupWithoutMutation`、`biz/execution_intent_test.go::TestWorkspaceRestartMaterializesDefaultSelectionByPrivateEventBeforeRunOwner` | 已覆盖 |
| Run projection lost/duplicate/stale/restart/public notification | `biz/run_projection_reconciliation_test.go::TestWorkspaceRunProjectionFreshOwnerRecoversWithoutHistoricalNotification`、`biz/run_projection_reconciliation_test.go::TestWorkspaceRunProjectionRevisionFenceRejectsStaleAndConflictingFacts`、`biz/lifecycle_reducer_test.go::TestReduceRunProjectionFactDuplicateIsNoop`、`biz/run_projection_notification_test.go` | 已覆盖 cursor 持久化、真实公开字段变化通知与 cursor-only 零通知 |
| deletion normal/negative/idempotency/restart | `biz/deletion_coordinator_test.go::TestWorkspaceDeletionCoordinatorUsesExactRetainAndPurgeOrder`、`biz/deletion_internal_event_test.go::TestWorkspaceDeletionRejectsOutOfOrderAndMismatchedOwnerAcknowledgements`、`biz/deletion_coordinator_test.go::TestWorkspaceDeletionResponseLossResumesFromPersistedPhaseOnExactDeleteRetry`、`biz/deletion_coordinator_test.go::TestWorkspaceDeletionFreshOwnerStartRecoversPersistedFileFinalization` | 已覆盖 coordinated delete 正向路径与恢复 |
| file-backed admission/clarification cutpoints | `biz/definition_admission_file_recovery_test.go::TestDefinitionAdmissionFileBackedCutpointsRecoverThroughClarificationPromotion`、`internal/pkg/workorchrepo/workspace_scoped_repositories_test.go::TestWorkspaceScopedPreSealDecisionPathSupportsWorkspaceCRUDAndReopen` | 已覆盖 unsupported candidate、corrected candidate、无 RunID 的 Decision ticket、pending promotion、Decision resolution、eligible CAS 各 durable cut 后 fresh-owner reopen与 duplicate/response-loss replay；同时覆盖该 Workspace-scoped Decision file path 的 list/update/delete/reopen及 Decision-owner workspace purge。单次 repository atomic write 内部的 OS kill/fsync/rename crash atomicity 属于 host/filesystem deployment 边界，由 [`Service Acceptance / Persistence And Restart`](../../../../docs/50-product-interface/service-acceptance.md#persistence-and-restart) 验收，不由 Workspace reducer 推导 |
