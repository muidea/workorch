# Workspace / Goal / DoD Domain Model

Status: normative

Type: domain-model

Last Verified: 2026-08-22

## Ownership

`blocks/workspace` 是 Workspace、Goal、DoD 和 WorkspaceTemplate 的唯一 owner。application、Run、Orchestrator 和
projection 都只能消费 typed snapshot 或提交命令，不能直接更新这些对象。

## Definition Admission Candidate

Workspace owner 在正式 Definition 封存前持久化不可变的 `DefinitionAdmissionCandidate`。它绑定候选 Goal/DoD、
Template/Profile policy snapshot、resolved project directory、compiler version 与 candidate hash；候选 revision 只表示一次
authoring/admission 尝试，不是正式 `DefinitionRevision`。

纯 admission compiler 只接受可完整编译为 `all_of` 的 Definition。显式 `one_of`、`any_of`、threshold 或无法确定唯一
required relation 的候选不能被自动择支、LLM 改写或提升，必须创建阻塞 clarification ticket。完整字段、promotion CAS 与
重复/陈旧规则见 [Definition Admission Contract](../00-core/definition-convergence/definition-admission.md)；formal item、
relation、Definition→CWI mapping 与 all-of gate 见
[Canonical Definition Model](../00-core/definition-convergence/canonical-definition.md)。

## Workspace

Workspace 是长期业务容器。当前 owner model 包含：

| Field group | Meaning |
| --- | --- |
| `Revision`, `ID`, `Title`, `Description`, `State` | owner-local CAS identity 与基本信息 |
| `DeliverableDirectory`, `InternalDirectory` | 用户交付边界与 WorkOrch 内部状态边界 |
| `ActiveRunID`, `TerminalDelivery` | 当前 Run 绑定和终态 slot acknowledgement fence |
| `TemplateID`, `ProfileID` | 激活时使用的模板/profile 引用 |
| `DefaultLLMSelection` | Workspace 默认 provider/model/reasoning selection |
| `DefaultSoulID`, `SoulBindings` | 默认 Soul 与按语义位置的 Soul 绑定 |
| `ContextHints`, `EvidencePolicies` | 模板/管理接口维护的可选声明式策略 |

正常 TUI/Web 创建流程不要求用户手写 `ContextHints` 或 `EvidencePolicies`；它们通常来自模板或管理接口。
terminal release只接受`ActiveRunID == outcome.RunID`；空slot不能补写首次acknowledgement fence，只有已持久化同identity
delivery的duplicate才能幂等返回。

## Goal

Goal 表达一个 Workspace 的正式目标。owner model 包含 `ID`、`WorkspaceID`、`Text`、
`DefinitionRevision`、previous definition/hash lineage 与 `LineageState`。

## Definition Of Done

DoD 表达可验证的完成条件。owner model 包含 `ID`、`WorkspaceID`、`Items`、`State`、
`DefinitionRevision`、previous definition/hash lineage 与 `LineageState`。

正式 DoD definition resource 只有 `ready`：pre-seal authoring 状态属于 immutable
`DefinitionAdmissionCandidate`，不能作为可启动 DoD 保存。`satisfied | unsatisfied | indeterminate` 是每个 Run、每个
CWI 的 Orchestrator assessment/terminal closeout 结论；Result 只保存已密封 outcome，任何 owner 都不得把该结论回写到
可复用的 Workspace DoD definition 形成第二套状态机。

## Definition Revision

- Goal 与 DoD 通过同一 effective definition revision 进入 `RuntimeBootstrap.Definition`。
- Goal/DoD resource revision 采用 `<kind>:<20 位递增代次>:<content-hash>`。代次在同一 Workspace、同一 kind
  内单调增加，内容回退不会复用历史 revision；相同规范化内容的重复 update 不推进代次。
- 新 Goal/DoD identity 连接 owner 当前唯一 canonical leaf，identity 和 repository/list 顺序不表达先后。历史节点
  不接受直接更新，current leaf 不接受物理删除；谱系歧义必须 fail-closed。
- Run 创建后保留该不可变引用；运行中的 Workspace 编辑不能就地改写 Run authority。
- 新定义要参与执行时，用户显式提交 workspace `restart` intent。
- restart 由 Run owner 管理 predecessor `ReplacementIntent`、终态投递和 successor 创建，不属于 Workspace
  metadata update。

## Startup Input And Activation Admission

一次正式启动输入只由四项组成：Goal/DoD Definition、Workspace Template snapshot、resolved project workspace directory，
以及它们共同产生的 `StartupInputHash`。封存后，执行期可以在不修改这些输入的前提下调用 LLM 做语义规划、诊断与候选提议。

Workspace owner 为每次 promotion 另存 `WorkspaceActivationAdmission`。普通 admitted candidate 在同一 CAS 中进入 `eligible`；
纠正 clarification ticket 的 candidate 先进入 `pending_clarification_resolution`，只有 Decision resolution 被 Workspace owner
通过 typed acknowledgement 验证后才转为 `eligible`。该事实不进入 `StartupInputHash`，也不是新的 Runtime 状态机。

## Directory Invariants

- `DeliverableDirectory` 是 capability owner 可访问的用户交付根；具体 path scope 仍由 operation contract 校验。
- `InternalDirectory` 只保存 WorkOrch 内部数据，不得被当作用户产物目录。
- file-backed 默认数据根和 workspace 交付根相互分离。
- projection/API 对 artifact 使用逻辑引用，不泄漏内部绝对路径作为跨组件 authority。

## Activation Invariants

- 没有有效 Goal/DoD 时不能形成完整 bootstrap。
- canonical Definition、Template/Profile terminal declaration 与 Runtime dependency 只允许 `all_of`；不提供 `one_of` 终态。
- 所有 Start/Restart ingress 都必须验证 `WorkspaceActivationAdmission=eligible`；pending、missing 或 binding mismatch
  返回 typed `definition_clarification_pending`，不得创建 Run。
- bootstrap 只封存 definition、activation、profile/capability/governance/resource refs 等 typed 值。
- per-CWI proof 先由 bounded semantic plan 提议、由 capability owner确认 feasibility，再由 Orchestrator 内部事件密封。
- 调用方不能上传 target/recipe/check/metric/artifact authority graph。
- Goal/DoD 的 read/update/delete command 必须带 WorkspaceID；memory/file backend 均按该 scope 解析局部 identity，
  不允许通过另一个 Workspace 的路径或 context 读取、修改或删除对象。
