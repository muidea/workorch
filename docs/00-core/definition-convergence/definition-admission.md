# Definition Admission Contract

Status: normative

Type: core-contract-part

Last Verified: 2026-08-18

## 仅允许 all-of terminal 语义

- canonical Definition aggregation、Template/Profile terminal declaration、adopted assisted relation、RuntimeBootstrap dependency、
  scheduling contract 和 provider-facing Definition/relation schema 只允许 `all_of` 合取，不提供 `one_of`、`any_of`、
  `any_settled`、quorum、threshold 或 first-success terminal mode；
- 每个 required CWI 都必须进入唯一 derived terminal all-of root；只有全部 member 具有 current binding/evidence 且 settled，
  SM-2 才能形成 completed terminal；
- 多个 item 提供同一 required ref 不是“可任选的替代项”，而是 static ambiguity；Definition 必须改写为互不歧义的 required
  refs、明确唯一 provider，或把确实都必需的成员声明为 all-of aggregation；
- capability owner 在一个已固定 CWI 下提供多个 admissible operation kind/variant，只是 SM-3 candidate 可行性选择，不会减少
  required CWI 集合，也不构成 Definition one-of；
- 一个 concrete operation 的 tagged union、互斥 parameter 或 `RequireOneOfParameters` 可以作为 capability-owner input schema
  约束保留；它只要求 candidate 为同一个 sealed operation 提供唯一合法输入，不得被提升为 CWI/Goal/DoD 的可替代终态；
- 对 pre-seal candidate 中显式 one-of，或已声明 relation/ref/mapping 无法确定性编译为完整 all-of 的候选，
  系统不得自动选分支、静默改写、调用 LLM 猜测、提升为 canonical Definition 或先创建 Run；必须按
  [Workspace clarification](clarification.md) 生成阻塞票据。

上述最后一条只约束**可由 pre-seal candidate 确定**的显式 one-of/ambiguity。任意自然语言中未结构化的隐含 one-of 不允许
Workorchd 通过关键词或领域猜测裁决；它可以在 startup input 封存后的 Runtime semantic processing 中由 LLM 作为 bounded
diagnostic 暴露，再由 SM-2 按 [Runtime clarification](runtime-clarification.md)
fail-closed 建票。

## Pre-seal admission, startup input immutability and execution-time LLM

正式 startup input 之前只有 Workspace owner 可持久化的 pre-seal admission 对象：

```text
DefinitionAdmissionCandidate
  SchemaVersion
  WorkspaceID
  CandidateID
  CandidateRevision
  BaseDefinitionRevision        optional; paired with BaseStartupInputHash
  BaseStartupInputHash          optional; only when revising an existing sealed input
  GoalInput                     CandidateItemRef + exact proposed text + source identity
  DoDInputs[]                   CandidateItemRef + exact proposed item + source identity
  WorkspaceTemplateSnapshot    ref + revision + exact snapshot hash
  AdmissionPolicyBindings[]    exact Profile/policy ref + revision + snapshot hash
  AdmissionCompilerVersion
  ResolvedProjectWorkspaceDirectory
  ProposedRelationInputs[]      admission-only typed inputs
  CandidateHash
```

- candidate 由 Workspace owner command normalize、hash 和 CAS 持久化；Workorchd、setupdraft、HTTP/Web/TUI 或 LLM
  不得在 owner 外维护第二份可提升状态；
- candidate submit 使用 Workspace owner-defined concrete typed
  `SubmitDefinitionAdmissionCandidateCommand/Result`；result 必须返回 exact normalized candidate identity/hash 与
  `created|duplicate` disposition。CandidateItemRef 只在该 candidate scope 内引用 proposed relation，不能充当正式 ItemRef；
- `CandidateID` 标识一次连续 authoring/admission lineage，`CandidateRevision` 标识该 lineage 下的一次不可变尝试；对同一
  expected revision 提交 exact normalized duplicate 必须返回原 `CandidateID + CandidateRevision + CandidateHash`，不得新增记录。
  任何 materially changed input 必须以同一 lineage 的下一 revision CAS 提交，stale expected revision 拒绝且不得覆盖旧尝试；
- `ProposedRelationInputs` 是唯一允许暂存 unsupported aggregation token 的 typed admission surface。它可以保留
  `one_of/any_of/...` 以产生精确 violation，但这些值不得进入 `WorkspaceDefinitionItem`、canonical relation、
  Template/Profile canonical contract、RuntimeBootstrap、provider-facing execution schema 或 startup-input contract；
- candidate revision 只表示一次不可变 authoring/admission 尝试，不是 semantic `DefinitionRevision`。invalid candidate
  不取得 `StartupInputHash`，不创建 canonical Goal/DoD item/relation、Run、Runtime aggregate、provider request 或 capability work；
- `BaseDefinitionRevision` 与 `BaseStartupInputHash` 必须同时为空或同时存在；存在时只声明该 candidate 要替换的 formal input
  lineage，不授予 candidate 任何 canonical/restart authority，且 promotion 必须验证二者仍属于同一 Workspace sealed input；
- `WorkspaceTemplateSnapshot` 必须传递性封存其选择的 exact Profile/policy snapshot；若 Profile 是独立资源，则使用
  `AdmissionPolicyBindings` 显式携带 ref/revision/hash。不得根据 candidate 提交后的 live Profile 重新解释同一 candidate；
- 同一 candidate 内容 permutation 后必须得到同一 `CandidateHash`；candidate identity/hash 的 schema version、compiler version、
  canonical bytes、base input identity、exact text、Template/Profile policy snapshot 和 resolved directory 均进入 admission/ticket
  stale 与 duplicate 校验；
- Workorchd pure admission compiler 只能返回绑定 exact `CandidateID + CandidateRevision + CandidateHash` 的 typed outcome。
  只有 `admitted_all_of` outcome 可由 Workorchd application use case 通过同步 EventHub `Send` 提交 Workspace owner
  `PromoteDefinitionAdmissionCandidateCommand/Result`；
- Workspace owner 只在 candidate identity、admission-result hash 和 current candidate CAS 全部匹配时原子 promotion，生成正式
  semantic `DefinitionRevision` 与 `WorkspaceStartupInputContract`。duplicate promotion 返回同一正式 identity，stale/mismatched
  outcome 拒绝且不产生部分 Goal/DoD、relation 或 startup-input 写入；
- `PromoteDefinitionAdmissionCandidateCommand/Result` 必须是 Workspace owner `pkg/events` 的 concrete typed contract。command
  只携带 candidate identity/hash、`admitted_all_of` result hash 与 canonical admitted-input hash；result 返回正式
  `DefinitionRevision + StartupInputHash + CandidateItemToItemRefMappingHash`。Workspace owner 持久化完整 mapping，Workorchd 只能
  使用该 owner result 编译 Runtime mapping，不能自行把 CandidateItemRef 提升为 ItemRef。

Workspace owner 还必须为每次 promotion 持久化不进入 `StartupInputHash` 的 activation admission fact：

```text
WorkspaceActivationAdmission
  WorkspaceID
  CandidateID + CandidateRevision + CandidateHash
  DefinitionRevision + StartupInputHash
  State = eligible | pending_clarification_resolution
  BlockingTicketID                 optional; required while pending
  ClarificationResolutionID       optional; required when pending becomes eligible
  ClarificationResolutionBindingHash optional
```

- 没有关联 static ticket 的 admitted candidate 在同一 promotion CAS 中进入 `eligible`；
- 用于纠正 static ticket 的 candidate promotion 后先进入 `pending_clarification_resolution`。只有 Workspace owner 收到并验证
  concrete typed Decision resolution acknowledgement，才可用 owner-local CAS 转为 `eligible`；
- 所有 Workspace Start/Restart execution-intent ingress 都必须读取该 owner fact。pending、missing 或 binding mismatch 必须返回
  typed `definition_clarification_pending`，不得仅因 formal startup input 已存在而创建 Run；
- activation admission 是启动资格，不是第五状态机，也不改变四项 startup input 或 `StartupInputHash`。

promotion 成功后，Workspace owner 才在任何执行期 LLM request 前封存唯一 startup-input contract：

```text
WorkspaceStartupInputContract
  WorkspaceID
  DefinitionRevision
  GoalDefinitionRef + exact text hash
  DoDDefinitionRefs[] + exact item-set hash
  WorkspaceTemplateRef + TemplateRevision + TemplateSnapshotHash
  ResolvedProjectWorkspaceDirectory
  StartupInputHash
```

- 用户输入的 directory 可以为空，但 owner/application 必须先将其解析为实际项目工作目录；默认值仍为
  `.workorch-workspaces/<workspace-id>`。Run/LLM/capability contract 使用已解析值，不把空字符串当作 root；
- `CandidateHash` 只用于 admission lineage；正式 Run、Runtime、LLM、ticket recovery 和 terminal authority 使用 promotion 产生的
  `DefinitionRevision + StartupInputHash`，不得把 invalid/pre-seal candidate hash 冒充为 startup-input identity；
- 每个 Run 的 bootstrap、Definition partition、scheduling contract、LLM request/session、candidate、dispatch、evidence、
  clarification ticket 和 terminal 都必须绑定同一 `StartupInputHash`；
- startup input 一旦被当前 Run 封存，执行期 LLM 可以读取其授权的 exact partition/context，提出 semantic proof、gap
  diagnostic 或 operation candidate，但不能修改 Goal/DoD、切换 Template snapshot、改变项目 root、生成新的 Definition
  revision、扩张 required CWI、选择 Runtime action、授予 credit 或裁决 terminal；
- LLM 输出中的改写文本、替代 directory/template、未声明 requirement 或 global order 只可作为 rejected diagnostic；不能
  回写 Workspace owner、RuntimeBootstrap、C0～C2 或 scheduling contract；
- 用户采纳任何 Goal/DoD/关系澄清都必须先通过 Workspace owner command 创建新 candidate；只有 admitted promotion 才创建
  新 semantic Definition revision/startup input。当前 Run 保持原 startup input；Resume 不能吸收新 revision，只有显式
  Restart 可在 predecessor delivery 后取得新的 startup input；
- “手工 authoring 可不调用 setup-draft LLM”只是产品输入能力，不是 Runtime 零 LLM 不变量。startup input 封存后，SM-3
  可以按现有 bounded request/session、cache、budget 和 recovery 合同调用 LLM。

## Related Contracts

- canonical items、mapping、relations、all-of gates 与 completeness validation：
  [Canonical Definition Model](canonical-definition.md)
- static candidate clarification：[Workspace Definition Clarification](clarification.md)
- execution-time semantic gap：[Runtime Definition Clarification](runtime-clarification.md)
