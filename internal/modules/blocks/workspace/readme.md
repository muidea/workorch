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
| `application/workorchd` | Workspace/Goal/DoD/Template CRUD；`ConfirmDefinitionAuthoringCommand/Result`；`DefinitionAdmissionIntentCommand/Result`；`WorkspaceExecutionIntentCommand/Result`；`DeleteWorkspaceCommand/Result` | Application 提交输入和意图；Workspace 决定自身聚合转换。authoring confirmation、definition admission、execution intent 只接受 Application source。 |
| `kernel/system` | `EnsureWorkspaceTemplateDefaultsCommand/Result` | startup ensure 唯一 producer；owner 创建缺失资源并自动更新 exact reviewed 未定制 Template；只接受 `kernel/system`。 |
| `application/workorchd` | `QueryBuiltinWorkspaceTemplateDefaultsCommand/Result`、`UpgradeBuiltinWorkspaceTemplateDefaultCommand/Result` | exact `application/workorchd/appservice`；只操作 Template catalog，不改变已 admission Workspace 或 Run。 |
| `blocks/decision` | `QueryWorkspaceDefinitionClarificationFactCommand/Result` | Workspace 在 promotion 前读取 Decision 已持久化的 ticket/resolution 事实，不接受 caller 自证 binding。 |
| `blocks/run` | Workspace→Run execution/retirement/purge；Run→Workspace lifecycle fact/inventory | Workspace 管 active slot 投影；Run 管 SM-1 与 retirement 事实。 |
| `kernel/orchestrator` | `RuntimeTerminalDeliveryFact → WorkspaceLifecycleCommandResult`；Runtime quiescence command/result | terminal delivery 只接受 Orchestrator；Workspace 只映射已验证事实到自身 journal。 |
| Result/Memory/Decision/Approval/Eventstream owners | concrete seal/purge command/result | Workspace 排序协调删除；各资源 owner 独自执行自身 fence、purge 与证明。 |
| projection consumers | `WorkspaceChangedData`、`WorkspaceDeletedData`、`GoalDefinedData`、`DoDDefinedData` | 提交后通知，不是 authority。 |

公开 EventHub 合同均位于 `pkg/events`，每个 topic 使用具体 command/result；跨 owner 不暴露 repository/service。`biz.New` 只接收 backend factory 生成的 `WorkspaceOwnerRepositories` aggregate；Workspace、Goal、DoD、Template repository 与 deletion cleaner 必须携带同一个 opaque owner binding，独立构造、跨 backend 或跨 factory 混装在 owner 启动前拒绝。

## 不变量与非职责

- 正式创作创建入口接受稳定 RequestID，Workspace ID 由该身份确定。初始草稿与不可变创建回执在同一次 owner repository Create 中保存；同身份不同内容拒绝，响应丢失／file reopen 后重放不增写，后续修订也不会被初始创建重放覆盖。目录初始化由 Application 在身份持久后执行，失败可用同一请求重试；创建不物化正式 Goal／DoD、不准入、不创建 Run。模板与目录修改通过创作保存事件随新 input revision 一次 CAS 更新，不以普通配置修改替代创作输入。
- startup input 一经 promotion 不可被 Runtime LLM 修改；candidate/admission lineage、definition revision 与 `StartupInputHash` 必须 exact 绑定。
- 新建期 `DefinitionAuthoringRecord` 仅保存 Goal-only 草稿、Template 快照、派生与审查请求；正式 Goal／DoD 与 startup/eligible admission 必须在用户精确确认后另行物化。资源引用最多 8 个；Application 向 File owner 取得有界文件事实，派生 reserve 若含资源，Workspace 在任何自身 mutation lock 外向 File owner 核对 exact 内容/hash 和当前草稿 revision，再由纯 reducer 对 Workspace revision 及模板做 CAS 栅栏。创作规格结构校验把持久派生快照映射为不可由 provider 改写的必需资源身份，六审查依然只是候选事实。持久资源快照和 permit 仅证明该时点的输入身份，不保证多文件原子读取或后续内容不变。确认 journal 和最终准入各自在 mutation/deletion 锁外重新向 File owner 验证原派生资源及当前 Workspace revision，随后在锁内核对 input、permit、snapshot hash、Template/configuration 与 revision 才可 CAS；Prepared journal 恢复及阻塞候选的重复准入也必须重新验证。File 与 Workspace 没有共同事务或跨文件原子快照，文件在最终验证后至 Workspace CAS 之间发生的外部变化不能被宣称已原子阻止。
- `DefinitionAuthoringConfirmation` 是同 owner 的持久 prepare／materialized journal：只从已持久派生提案和六份唯一 PASS 审查重组 exact hash，封存原派生资源快照 hash 与确定性 Goal／DoD after-image，再逐仓幂等物化；任一 file-backed cutpoint 可重开续写，重复请求不增写，冲突行不得覆盖。prepared 阶段的部分 Goal／DoD 写入对普通正式定义读取不可见，旧规格编译入口也不能消费创作期正式行。已物化确认的纯候选构建保留原始六审查的候选 item ref 和 `SpecificationHash`，并将正式 Goal／DoD 的精确 ID／revision 绑定到候选来源身份；后续 startup 映射负责生成正式 item ref，不能假称审查者评审过重写 hash 的规格，也不能把 `UNVERIFIED` 变成 Profile 证据声明。Application 的确认后继已接正式候选与准入，阻塞候选由 Decision owner 打开澄清 ticket，不创建 Run；新建期旧准入入口仍关闭。journal 在场时普通 Workspace 配置更新、正式 Goal／DoD CRUD 与删除入口均不允许改写该事务。未封存 startup 的阻塞候选允许提交新的创作 revision：归档原记录及 materialized journal，保留请求预算和正式定义 lineage，重新派生并完成六审查后沿同一 `CandidateID` 追加连续 candidate revision；prepared 后继仍不能暴露部分正式行。仍阻塞的后继由 Application 协调 Decision 替换旧票据；通过纯准入的后继须先验证持久旧票据，封存 `pending_clarification_resolution`，再验证 Decision 持久 resolution 后才能 eligible。确认重放和查询保留原 blocker 身份，不自动创建 Run。此 owner/Application 路径已验证，不代表 HTTP/Web 创建向导已接通。
- 新 Start/Restart 自动加载当前 Template/Soul。Application 提交带 `CurrentTemplate`、expected Workspace revision/configuration
  hash 的新 candidate；Workspace-private admission event 持有 Template mutation lock，复核当前 Template hash，物化变化的
  template-bound 字段并 CAS 追加新 candidate/promotion。Goal/DoD、目录、provider selection、生命周期与旧 Run 不变。
  `TemplateBindingHash` 是 live authoring 版本标记，不是 Run seal；同 Template 保留 Workspace 配置，变化的 Template 整体
  替换旧绑定策略，包括移除字段。无需手动 refresh；详见 [automatic configuration loading](../../../../docs/40-profiles-capabilities/workspace/builtin-default-resources.md#automatic-configuration-loading)。
- 已准入历史只检查 scope/identity/lineage，不用当前 schema/compiler 重新准入；candidate clone 必须原样 detached，不做
  normalization、补字段或 rehash。新输入严格使用当前 compiler，历史身份损坏与未解决 clarification 不能被自动更新绕过。
  Resume/recovery 不进入此 authoring 事务，SM-1～SM-4 仍消费原 Run seal。
- Definition 仅支持完整 `all_of`；unsupported alternative 必须保留阻塞 clarification，不得降级或补默认语义。
- Profile 声明必须无损保留全部成员直至 owner admission；空/重复 identity、缺失 owner/state/variant 不能被 normalization
  清除。Workspace 只通过既有 private admission event 拒绝并保存 invalid candidate，不创建 startup/eligible admission。
  recipe/check/metric/artifact/owner-fact proof identity/capability-state 全部进入 Profile seal；同版本重复、重启和内容漂移遵守相同 hash/CAS fence。
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

### 规格候选与模板组合

- Template 的 `ParentTemplateIDs` 由 Workspace owner 解析为有界 DAG；父声明顺序决定稳定拓扑顺序，共享祖先只合并一次。循环、缺失、重复父、深度/规模超限或无法安全合并的冲突拒绝。DoD 合取，不允许子模板覆盖父验收要求；同路径 context、同 ID evidence policy 的不一致当前直接拒绝，不猜测哪个更严格。
- 模板读取返回 effective 内容及只读 `EffectiveSources`。父内容及拓扑参与派生快照身份；create/update 只能提交原始声明，不能自报 effective 来源。新 Start/Restart 在 owner mutation lock 内核对整个有效快照，既有 Run 不热替换；无父模板保留既有 snapshot hash 算法。
- `DefinitionAdmissionCandidate.Specification` 是可选、版本化的规格提案。资源、维度、需求来源、义务、exact Profile 验证声明和六类 completeness review 全部进入候选身份；审查绑定不含审查自身的 content hash，startup 的 `SpecificationHash` 绑定包含审查的 admission hash。
- 六类审查分别覆盖维度、负路径、回归、资源环境、显式约束追踪和可验证性。FAIL/UNKNOWN、缺必需资源、缺必需维度/义务、缺 DoD 映射或不匹配的 Profile 声明不能晋升。PASS 只是有来源的评审事实，不直接授予 Runtime evidence credit，也不证明任意自然语言目标的完整性。
- nil Specification 保留既有候选与 startup hash。此合同约束提交的规格候选，不表示所有 Web/TUI 输入已自动经过完整 Goal Compiler；正式执行仍须通过 exact CWI、owner feasibility、sealed dispatch 和 SM-2 闭包。

## 能力验收

| 能力/维度 | 测试证据 | 结论 |
| --- | --- | --- |
| 创作阻塞后继与确认恢复 | `biz/definition_authoring_successor_test.go` | 同 CandidateID 连续 revision、旧票据 resolved／新票据 open、候选追加与 pending startup 重开、安全 resolution、prepared／Goal／DoD／materialized cutpoint 及零自动 Run；不是 HTTP/Web 闭环验收 |
| 多父模板 DAG 与解析结果隔离 | `pkg/models/template_inheritance_dag_test.go`、`biz/template_inheritance_test.go` | 覆盖 diamond 去重、父顺序身份、共享祖先最长路径上限、父引用深拷贝及 owner 模板操作 |
| 规格候选结构与防降级 | `pkg/models/goal_specification_test.go`、`pkg/definitionadmission/goal_specification_test.go`、`biz/goal_specification_persistence_test.go` | 覆盖六类审查绑定、引用唯一性、占位验证声明拒绝、startup 哈希及 owner 后继候选禁止移除规格；不等同于自动规格生产入口验收 |
| module lifecycle与 owner repository binding | `module_test.go::TestModuleIdentityConfigureStartStopContract`、`module_test.go::TestModuleConfigureRequiresBoundOwnerRepositoriesAndEventHub`、`internal/pkg/repository/resource_owner_repositories_test.go::TestResourceOwnerRepositoryBundlesRejectMixedMemorySets`、`TestResourceOwnerRepositoryBundlesValidateForMemoryAndReopenedFileBackends` | 已覆盖必需 aggregate/EventHub、memory/file factory 正向构造、zero/unbound/mixed set 启动前拒绝与 file reopen binding 重建 |
| Workspace create/get；Goal/DoD scope与definition revision；Template mapping clone | `biz/goal_dod_manager_test.go::TestGoalDoDManager_CreateWorkspace`、`biz/goal_dod_manager_test.go::TestGoalDoDManager_DefinitionRevisionIsMonotonicAcrossContentReversion`、`biz/command_router_test.go::TestWorkspaceDefinitionCommandsEnforceScopeAcrossBackends`、`biz/template_commands_test.go::TestWorkspaceTemplateOwnerMappingsDeepCloneCommandAndRepositoryValues` | 已覆盖这些明确路径 |
| Workspace update/list、Goal/DoD update/delete/list、Template CRUD | `biz/command_router_test.go::TestWorkspacePublicManagementCRUDPositivePaths` | 已覆盖；Workspace coordinated delete由下方deletion矩阵验收 |
| Template explicit upgrade | [`biz/builtin_defaults_test.go`](biz/builtin_defaults_test.go) | memory/file、seed 不覆盖旧版本、错误 source/Post、未知/定制/stale/changed-target 拒绝、写后响应丢失与 fresh-handle replay、并发普通编辑保护 |
| automatic configuration / archive isolation | [`biz/current_configuration_admission_test.go`](biz/current_configuration_admission_test.go)、[`biz/defaults_test.go`](biz/defaults_test.go)、[`pkg/models/template_test.go`](pkg/models/template_test.go) | file reopen、旧 schema 原样保存、当前配置准入与幂等、无效配置阻塞、并发 fence、Soul fresh read、历史 Run 零改写、未定制内置 Template 自动更新 |
| Definition normal/unsupported/duplicate/stale/provenance | `biz/definition_admission_test.go::TestDefinitionAdmissionIntentAutomaticallyPromotesEligibleCandidate`、`biz/definition_admission_test.go::TestDefinitionAdmissionIntentKeepsUnsupportedAlternativePreseal`、`biz/definition_admission_test.go::TestDefinitionAdmissionRejectsSelfConsistentUnpersistedDecisionTicketWithoutPromotion`、`biz/definition_admission_test.go::TestDefinitionAdmissionRejectsStaleLineageCAS` | 已覆盖 |
| lossless Profile identity / current-schema restart | `biz/definition_profile_identity_test.go::TestDefinitionProfileIdentityRejectionSurvivesOwnerRestart`、`workorchruntimecontract/runtime_profile_identity_test.go` | required→optional 重复声明不能消失；file-backed candidate 保留两项、fresh owner 重复零写、零 startup/activation、同 hash 篡改拒绝；无 Run/provider/capability 依赖 |
| privileged source matrix 与零 mutation | `biz/privileged_source_test.go::TestWorkspacePrivilegedIngressRejectsForgedSourcesWithoutOwnerMutation`、`biz/privileged_source_test.go::TestWorkspaceTemplateDefaultsIngressAcceptsOnlySystemOwner` | 已覆盖 |
| execution normal/duplicate/stale/restart | `biz/execution_intent_test.go::TestWorkspaceExecutionIntentDuplicateReplayUsesActiveRunAndRejectsResultDrift`、`biz/execution_intent_test.go::TestWorkspaceExecutionInternalEventRejectsStaleStartupWithoutMutation`、`biz/execution_intent_test.go::TestWorkspaceRestartMaterializesDefaultSelectionByPrivateEventBeforeRunOwner` | 已覆盖 |
| Run projection lost/duplicate/stale/restart/public notification | `biz/run_projection_reconciliation_test.go::TestWorkspaceRunProjectionFreshOwnerRecoversWithoutHistoricalNotification`、`biz/run_projection_reconciliation_test.go::TestWorkspaceRunProjectionRevisionFenceRejectsStaleAndConflictingFacts`、`biz/lifecycle_reducer_test.go::TestReduceRunProjectionFactDuplicateIsNoop`、`biz/run_projection_notification_test.go` | 已覆盖 cursor 持久化、真实公开字段变化通知与 cursor-only 零通知 |
| deletion normal/negative/idempotency/restart | `biz/deletion_coordinator_test.go::TestWorkspaceDeletionCoordinatorUsesExactRetainAndPurgeOrder`、`biz/deletion_internal_event_test.go::TestWorkspaceDeletionRejectsOutOfOrderAndMismatchedOwnerAcknowledgements`、`biz/deletion_coordinator_test.go::TestWorkspaceDeletionResponseLossResumesFromPersistedPhaseOnExactDeleteRetry`、`biz/deletion_coordinator_test.go::TestWorkspaceDeletionFreshOwnerStartRecoversPersistedFileFinalization` | 已覆盖 coordinated delete 正向路径与恢复 |
| file-backed admission/clarification cutpoints | `biz/definition_admission_file_recovery_test.go::TestDefinitionAdmissionFileBackedCutpointsRecoverThroughClarificationPromotion`、`internal/pkg/workorchrepo/workspace_scoped_repositories_test.go::TestWorkspaceScopedPreSealDecisionPathSupportsWorkspaceCRUDAndReopen` | 已覆盖 unsupported candidate、corrected candidate、无 RunID 的 Decision ticket、pending promotion、Decision resolution、eligible CAS 各 durable cut 后 fresh-owner reopen与 duplicate/response-loss replay；同时覆盖该 Workspace-scoped Decision file path 的 list/update/delete/reopen及 Decision-owner workspace purge。单次 repository atomic write 内部的 OS kill/fsync/rename crash atomicity 属于 host/filesystem deployment 边界，由 [`Service Acceptance / Persistence And Restart`](../../../../docs/50-product-interface/service-acceptance.md#persistence-and-restart) 验收，不由 Workspace reducer 推导 |
