# Definition Admission Contract

Status: normative

Type: core-contract-part

Last Verified: 2026-08-26

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
  RuntimeProfileContracts[]    exact normalized typed Profile proof snapshot
  RequiredOwnerFactBindings[]  exact Profile requirement ref + owner/fact/operation
  OwnerFactConvergenceBindings[] exact missing trigger + one acquisition + prospective-satisfaction ref when required + reobserve contract
  RuntimeProfileContractsHash  hash of the exact normalized Profile contracts
  AdmissionCompilerVersion
  ResolvedProjectWorkspaceDirectory
  ProposedRelationInputs[]      admission-only typed inputs
  CandidateHash
```

- Definition admission 只提供 Workspace owner-defined concrete typed
  `DefinitionAdmissionIntentCommand/Result`。command 携带 immutable candidate、`ExpectedCandidateRevision`，且至多附带一个
  Decision owner 已返回的 immutable `WorkspaceDefinitionClarificationTicketFact` 或
  `WorkspaceDefinitionClarificationResolutionFact`；它不携带 target state、promotion/ack 意图或 caller 编译的
  admission outcome；
- public intent 只能映射为 Workspace-private event。Workspace private reducer 在 owner-local CAS 路径中完成
  candidate normalize/hash 校验与持久化、运行 pure static compiler，并独自选择 persistence、promotion、
  `pending_clarification_resolution` 或 `eligible`。Workorchd、setupdraft、HTTP/Web/TUI 或 LLM 不得在
  owner 外维护第二份可提升状态；
- result 必须返回 exact normalized candidate、绑定该 candidate 的完整 `DefinitionAdmissionResult`、
  `created|duplicate` candidate disposition，以及
  `clarification_required | clarification_ticket_required | clarification_resolution_required | eligible` factual disposition。
  只有 owner reducer 已完成 promotion 时 result 才可返回成对的 `WorkspaceStartupInputContract` 与
  `WorkspaceActivationAdmission`；Application 不得提交、覆盖或选择这些 disposition。CandidateItemRef 只在
  该 candidate scope 内引用 proposed relation，不能充当正式 ItemRef；
- `CandidateID` 标识一次连续 authoring/admission lineage，`CandidateRevision` 标识该 lineage 下的一次不可变尝试；对同一
  expected revision 提交 exact normalized duplicate 必须返回原 `CandidateID + CandidateRevision + CandidateHash` 以及确定性相同的
  owner admission result，不得新增记录。
  任何 materially changed input 必须以同一 lineage 的下一 revision CAS 提交，stale expected revision 拒绝且不得覆盖旧尝试；
- 一个 Workspace 的 admission history 只能存在一个 `CandidateID`；revision 必须从 1 开始严格连续为 `1..n`。不得以新的
  `CandidateID@revision=1` 重置 lineage，也不得插入 revision gap、跳过 expected-revision CAS 或以不同 base startup input
  续写既有 lineage；这些输入一律 fail-closed 且零 candidate/promotion 写入；
- `ProposedRelationInputs` 是唯一允许暂存 unsupported aggregation token 的 typed admission surface。它可以保留
  `one_of/any_of/...` 以产生精确 violation，但这些值不得进入 `WorkspaceDefinitionItem`、canonical relation、
  Template/Profile canonical contract、RuntimeBootstrap、provider-facing execution schema 或 startup-input contract；
- candidate revision 只表示一次不可变 authoring/admission 尝试，不是 semantic `DefinitionRevision`。invalid candidate
  不取得 `StartupInputHash`，不创建 canonical Goal/DoD item/relation、Run、Runtime aggregate、provider request 或 capability work；
- `BaseDefinitionRevision` 与 `BaseStartupInputHash` 必须同时为空或同时存在；存在时只声明该 candidate 要替换的 formal input
  lineage，不授予 candidate 任何 canonical/restart authority，且 promotion 必须验证二者仍属于同一 Workspace sealed input；
- `WorkspaceTemplateSnapshot` 必须传递性封存其选择的 exact Profile/policy snapshot；若 Profile 是独立资源，则使用
  `AdmissionPolicyBindings` 显式携带 ref/revision/hash。不得根据 candidate 提交后的 live Profile 重新解释同一 candidate；
- Template 与 Workspace 两侧的 `EvidencePolicies` 都必须在 candidate submit/promotion 前完成无损 typed 编译和递归校验；
  每个 owner slice 内的 policy ID 必须为非空字符串且唯一，typed policy 必须有效，normalize 不得丢项或覆盖重复项。任一
  policy snapshot 含有退役字段 `prospectiveTargetRef` 时直接拒绝，不能把它解释为默认 target 或延后到 Runtime 修正。
  两侧每一项 exact policy snapshot 都以独立 ref/revision/hash 进入 `AdmissionPolicyBindings`，并由 `CandidateHash` 覆盖；
  binding 只封存 admission policy authority，不携带 exact prospective target。任一侧 policy 内容漂移都必须形成新的
  candidate revision，不能修改已封存 lineage；final bootstrap gate 还必须把 activation 中的 typed policies 与 exact-current
  admitted Workspace policy projection 逐项比较，投影漂移不得创建 Run；
- `RuntimeProfileContracts` 是 admission compiler 的完整 typed 输入，不是仅供展示的副本。其规范化 hash 必须与
  `RuntimeProfileContractsHash` 精确一致；Workspace pure compiler 必须从该 snapshot 独立派生 owner-fact/convergence
  manifest，并逐项匹配 candidate 中的 bindings，Application 不能只提交一个 hash 或自算 manifest 取代 owner 校验；
- Profile 中每个 `required=true` 的 acceptance dimension/deliverable expectation 都会形成 required CWI，因此必须存在同一
  `scope + id` 的 exact evidence requirement，显式声明非空 `requiredEvidenceKinds`，且每种 evidence kind 至少有一个
  accepted operation 能返回对应 typed owner receipt proof。`dodItem:*`、`goalCoverage:*` 等 wildcard quality policy 不能
  替代 Profile-owned required CWI 的 exact proof declaration；相同 `scope + id` 也不能被多个 Profile 重复声明；
- 上述 Profile proof closure 缺失、evidence kind 不受支持或 operation 无 owner receipt proof 时返回
  `definition_profile_contract_invalid`；Profile snapshot 与派生 owner-fact/convergence manifest 不一致、closure 缺失、重复、
  悬空或成环时返回 `definition_profile_prerequisite_unresolved`。两者都必须在 promotion 前打开
  `select_or_fix_template_profile` blocking ticket，且零 Run/provider/capability work；candidate-owned relation/ref failure 仍使用
  `declared_prerequisite_unresolved + rewrite_definition_all_of`，不得混为同一修订入口；
- 每个 required owner fact 必须在 candidate 中恰好绑定一次。它要么声明为 activation 前必须已存在，要么封存唯一
  `owner_fact_missing` acquisition：接收 owner、operation kind/variant、target policy、capability operation selector、
  独立的 Definition target selector identity、reobserve owner/fact 与
  `MaxAcquisitionAttempts=1`。Profile-derived binding 的缺失、重复、多 provider、依赖悬空或依赖环统一返回
  `definition_profile_prerequisite_unresolved`；两套 selector 不得缺省、互相复制或作为 alias。compiler 不读取 live catalog，
  也不猜测 File/Shell 领域动作；
- acquisition target policy 为 `prospective_owner_target` 时，convergence binding 还必须携带一个非空、Profile-sealed
  `ProspectiveSatisfactionContractRef`；preexisting、`exact_target` 与 `existing_owner_scope` 禁止携带。Workspace pure compiler
  必须从 exact Profile snapshot 独立派生并逐项比较该 ref，且把它纳入 candidate/startup Profile contract hash。缺失、额外、
  与 Profile 漂移或用于非 prospective policy 都返回 `definition_profile_prerequisite_unresolved`，在 Run/provider/capability
  work 前打开 `select_or_fix_template_profile` blocking ticket；Application/LLM 不得补默认 ref。
- `RuntimeProfileContractsHash` 同时进入 candidate 与 promoted startup input。Start/Restart 编译出的 normalized
  Profile contract hash 必须与 Workspace owner 封存值精确一致；Profile 后续漂移不能改变已封存 candidate/Run；
- 同一 candidate 内容 permutation 后必须得到同一 `CandidateHash`；candidate identity/hash 的 schema version、compiler version、
  canonical bytes、base input identity、exact text、Template/Profile policy snapshot hash（完整 typed snapshot 必须可重算匹配）
  和 resolved directory 均进入 admission/ticket
  stale 与 duplicate 校验；
- Workspace owner 的 pure compiler 只能返回绑定 exact `CandidateID + CandidateRevision + CandidateHash` 的 typed outcome。
  Workspace private reducer 必须从 repository 精确定位 current immutable candidate，并以同一 compiler 的结果作为
  admission 事实；Application 不能把 `AdmissionResult`、result hash 或 canonical admitted hash 作为 transition
  authority 传入；
- reducer 对非 `admitted_all_of` 只持久化 candidate，并返回 `clarification_required`。对无既往 blocking
  clarification 的 `admitted_all_of` candidate，reducer 在同一 CAS 中自动 promotion 并返回 `eligible`。
  对修正既往 invalid candidate 的 `admitted_all_of` revision，未观察到 Decision ticket fact 时返回
  `clarification_ticket_required`；精确匹配 ticket fact 后才原子 promotion 并封存
  `pending_clarification_resolution`；精确匹配 resolution fact 后才由同一 reducer CAS 为 `eligible`；
- promotion 与 clarification acknowledgement 都不提供独立 public target-state command。重编译结果仍为
  `admitted_all_of` 时，Workspace private reducer 才能原子生成正式 semantic `DefinitionRevision`、
  `WorkspaceStartupInputContract`、完整 candidate-item mapping 与 activation admission。duplicate intent 返回同一正式
  identity；stale/mismatched candidate、ticket fact、resolution fact 或 owner 重编译拒绝均不得产生部分 Goal/DoD、
  relation、startup-input 或 eligibility 写入；
- Workorchd 只做跨 owner 用例组合：编译 immutable candidate、发送 single intent、根据 Workspace owner factual result
  打开/协调 Decision ticket 或 resolution，将 Decision owner result 映射为 immutable ticket/resolution fact 后重新发送
  同一 intent，最后读取 owner snapshot。它不能选择 persistence、promotion、pending 或 eligible transition，也不能
  自行把 CandidateItemRef 提升为 ItemRef。Workorchd 对 pure helper 的 read-side 调用只是防御校验或 ticket
  payload 编译，不产生 admission state；所有正式迁移仍只来自 Workspace owner-private event。ticket/resolution fact 必须携带
  exact Decision command/result pair；Workorchd 在映射前调用 result 的 `ValidateFor(exact command)`，Workspace owner 在 reducer
  内再次调用同一校验并重算 ticket/resolution identity 与 candidate/startup binding，任一不一致均拒绝；

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
  RuntimeProfileContractsHash
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
