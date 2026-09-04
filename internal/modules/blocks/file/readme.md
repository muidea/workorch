# File Capability Block

Status: active

Type: capability-block-contract

Last Verified: 2026-09-04

## 功能范围与边界

`blocks/file` 拥有 workspace deliverable 边界内的 list/read/write/edit/patch-preview/patch-apply effect，以及 File
capability catalog、operation admissibility、candidate conformance、feasibility、target selection/snapshot、workspace
manifest/revision、repair-surface confirmation 和 post-effect reconciliation 等 factual observation。它签发 File operation
evidence 与 receipt 内容。

本组件不拥有 Runtime CWI、authority/focus、dispatch lifecycle、receipt persistence、retry/replan、governance decision 或
validation verdict。它不解释 Goal/DoD，不调用其它 capability owner，也不通过 File fact 触发 SM-2/SM-3 transition。

## 组件交互及合同

| 交互方 | 方向 | typed EventHub 合同 | 调用/授权语义 |
| --- | --- | --- | --- |
| `kernel/orchestrator` | Orchestrator → File | 10 个 Runtime read/conformance/feasibility command/result；6 个 sealed execute command，统一 `ExecuteOperationResult` | 接受 exact `kernel/orchestrator` source |
| `blocks/git` | Git → File | `AcquireWorkspaceManifestCommand/Result` | Git 在 capability-state/feasibility/effect freshness 中读取 File-owned tree revision；该 topic 额外接受 exact `blocks/git` source |
| `kernel/orchestrator` | File → Orchestrator | `QueryRuntimeOperationReceiptCommand/Result`、`CommitRuntimeOperationReceiptCommand/Result` | replay fence 与 receipt persistence；File 不直接写 Orchestrator state |
| workspace filesystem | File ↔ filesystem | owner-private `RuntimeAdapter` | effect 只能落在 resolved deliverable root 内 |

File owner 自定义的 16 组入站 command/result（10 read + 6 execute）位于 [`pkg/events`](pkg/events)；File 出站使用的 2 组
receipt query/commit 由 Orchestrator 定义，不计入这 16 组。candidate conformance 是 SM-3 pre-seal owner observation，其余
read/feasibility 合同属于 SM-2 owner observation，execute 合同属于 SM-3 sealed operation participation；它们都不是 reducer
API。`SealedRuntimeOperation` 绑定 canonical payload hash、operation identity 与 execution scope，handler 不能从自由文本重建
operation。

## 不变量与非职责

- `Configure` 缺 EventHub 立即失败；`Start` 重复调用不重复订阅，`Stop` 释放订阅和 adapter。
- 所有 Runtime ingress 在 owner query、effect 或 receipt I/O 前校验 producer；除 workspace manifest 显式允许 Git owner读取外，
  其余只接受 exact Orchestrator source。execute 还校验 workspace/run/dispatch、authority transaction、operation ID/kind、sealed payload
  hash、target 与 execution fence。
- path 必须是 canonical workspace-relative regular-file target；absolute path、`..` escape、symlink escape、binary edit、
  ambiguous replace、stale base hash 和越界 inventory 均 fail-closed、零 effect。
- preview、catalog、selection、manifest、feasibility、repair confirmation 与 reconciliation 都是只读事实，不得创建目录/文件。
- `file/write`、`file/edit`、`patch/apply` descriptor 共同声明 File-owned regular-target confirmation protocol；File 只按 concrete
  typed command 对 observation candidates 做 current manifest/revision 检查并返回事实，不选择 Profile route、repair action、
  retry 或 SM-2 transition。command 的 `RequestID + IssuedAtStateRevision` 是 Orchestrator-issued opaque request fence，File 必须
  校验其存在并在 result 中 exact echo，不能解释、保存或改写；confirmation 的 `SubjectRef` 也不是 owner 自选字段，只能返回该
  protocol 固定的 `workspace_deliverable`。缺失/错误 echo 或 subject 必须在 adapter 边界 fail closed。
  `RecordedAtStateRevision` 只属于 Orchestrator 持久化 record，并由其 CAS 固定为 `IssuedAtStateRevision + 1`；File 不生成或保存该
  revision。未声明该协议的 owner operation 不能被 pre-seal 为 validation-repair route。
- candidate constraint 分两阶段。conformance 必须复用 effect owner 的 exact parser/matcher，在零 workspace I/O 下验证 unified
  diff 的 single-file/target identity，以及 add-file (`/dev/null`) 或 existing-update 形态是否匹配 sealed target policy，只返回
  manifest-bound 的 `conformant | nonconformant | owner_contract_defect` 静态事实。feasibility 对合法但无法应用到 current
  owner-observed target 的 diff 返回 manifest-bound `candidate_constraint_violated`，携带 exact target/revision 与封闭 shape。
  File 在两阶段都不选择 correction、retry、terminal 或 Runtime state transition。
- feasibility 必须按 sealed target policy 对资源事实进行完备分类：prospective target 已存在，以及 exact/existing target
  已缺失，返回 `policy_disallowed + sealed_target_authorization_contradicted` 和 exact owner policy/revision；candidate-local
  patch conflict 不得冒充 binding contradiction。
- `workspace-relative-new-file-target` 是 create-only selector：prospective path 缺失时返回 `create`；File effect 后相同 path
  已存在时必须返回 `unsupported`，不能把它重解释为普通 existing-target `selected`。
- `workspace-relative-existing-file-target` 是 `file/edit` 的 existing/exact owner authorization；它只授权 manifest 中已列出的
  current regular file，不能创建路径。该 selector 只提供 capability fact，repair target 仍须来自 validation observation 与
  File confirmation，File 不选择 repair transition。
- workspace manifest/revision 对 deliverable topology/content 变化敏感；admissibility/target snapshot 漂移返回 typed stale，
  不自行刷新 authority。
- state-changing effect 返回 exact before/after revision 和 bounded evidence。execution key 已有 committed receipt 时返回 replay
  fact，不执行第二次写；effect 成功但 receipt commit 丢失时只允许 sealed effect-aware recovery。
- File 不保存 Runtime continuation、FailureTarget、CWI 或 decision；Orchestrator receipt commit 成功后才向 caller 返回 committed receipt。

## 能力验收

| 维度 | 可执行证据 | 覆盖结论 |
| --- | --- | --- |
| normal list/read/write/edit/preview/apply | [`module_test.go`](module_test.go)、[`biz/file_runtime_adapter_test.go`](biz/file_runtime_adapter_test.go) | 覆盖 create-only 与 existing-target selector、读写编辑及 patch 主路径 |
| typed contract / sealed identity | [`pkg/events/operation_test.go`](pkg/events/operation_test.go)、`kernel/orchestrator/biz/prerequisite_event_adapter_test.go::TestAcquireRuntimeTargetSelectionPrerequisiteUsesTypedEventHubContract` | 16 topic inventory、payload/scope binding、create-only selector effect 前 `create`/effect 后 `unsupported` 覆盖 |
| candidate constraints | [`biz/file_runtime_candidate_conformance_test.go`](biz/file_runtime_candidate_conformance_test.go)、`biz/file_runtime_adapter_test.go::TestRuntimeAdapterRuntimeFeasibilityRejectsStaleUnifiedDiff`、`module_test.go::TestFileModuleReportsUnifiedDiffTargetApplicabilityAsTypedCandidateConstraint`、[`pkg/events/operation_test.go`](pkg/events/operation_test.go) | static valid/invalid unified diff、target/target-policy mode、零 workspace I/O conformance，以及真实 owner EventHub 路径中 current target conflict 的 manifest/revision-bound typed applicability violation 与零 effect 覆盖 |
| exact source | `TestFileRuntimeOwnerCommandsRejectUntrustedSources`、`TestFileWorkspaceManifestAllowsGitOwnerSource`、`TestFileModuleRejectsDriftedSealedOperationBeforeEffect` | 10 个 read topic 的 producer matrix 与 execute wrong-source 覆盖 |
| path/binary/symlink/ambiguous negative | `TestRuntimeAdapterRejectsPathEscape`、`TestRuntimeAdapterRejectsSymlinkEscape`、`TestRuntimeAdapterEditRejectsBinaryFile`、`TestRuntimeAdapterEditRejectsAmbiguousMatch` | 覆盖拒绝且零 effect |
| stale/freshness / manifest revision | `TestRuntimeAdapterProbeRejectsStaleAdmissibilitySnapshotWithoutMutation`、`TestRuntimeAdapterProbeReportsTypedStaleAfterExternalWorkspaceMutation`、`TestRuntimeAdapterEditRejectsBaseHashMismatch`、`TestAcquireWorkspaceManifestRevisionTracksContent`、`TestOwnerFeasibilityCategoriesAreTotal` | 覆盖 stale、resource state 与 sealed target policy contradiction 的完备分类 |
| duplicate/idempotency/receipt loss | `TestFileOwnerStateChangingReceiptReplaysWithoutSecondWrite`、`TestRuntimeAdapterPatchApplyFileWriteConfirmsSealedReplayAfterReceiptLoss`、`TestFileOwnerPatchApplyReplaysSealedWriteAfterReceiptCommitFailure` | 覆盖 |
| reconciliation / restart | [`biz/owner_reconciliation_test.go`](biz/owner_reconciliation_test.go) | fresh owner restart、changed/deleted target 覆盖 |
| repair confirmation | [`biz/repair_surface_confirmation_test.go`](biz/repair_surface_confirmation_test.go) 中的 `TestFileOwnerConfirmsOnlyCurrentRegularRepairSurface`、`kernel/orchestrator/biz/prerequisite_event_adapter_test.go::TestConfirmFileRuntimeRepairSurfaceRequiresExactRequestFence`、`kernel/orchestrator/biz/runtime/shell_owner_receipt_integration_test.go::TestShellOwnerObservedFailureTargetBindsThroughEventHubAndFileSnapshot`、`kernel/orchestrator/biz/runtime/repair_surface_confirmation_plan_test.go::TestRuntimeRepairSurfaceConfirmationRequestIDBindsRevisionAndRefreshLineage`、`workorchruntimecontract::TestRuntimeRepairSurfaceConfirmationRecordProvidesTypedReplayFence` | expected workspace-tree revision 与 current 相同时的 regular target confirmed/rejected、mutation 后 stale+零 target、request/issued/expected revision exact echo、stale refresh 新 fence与旧 response 越窗拒绝、protocol-bound subject、recorded revision 及 cross-owner/unsupported 拒绝覆盖 |
| lifecycle / dependency fail-fast | [`module_test.go`](module_test.go)、`TestFileOwnerStartIsIdempotentAndTeardownClearsLifecycle`、`TestFileOwnerWrongDTOAndLifecycleMatrixCoversAllSixteenTopics` | 覆盖 16 topic 的重复 Start 单次投递与 Stop 后零 payload/effect |
| wrong DTO matrix | `TestFileOwnerWrongDTOAndLifecycleMatrixCoversAllSixteenTopics` | 覆盖全部 16 topic 的 wrong concrete command，并校验每次恰有一个 owner subscription |
| provider / external network | 不适用 | 不适用 |
