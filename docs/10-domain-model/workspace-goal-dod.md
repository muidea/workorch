# Workspace / Goal / DoD Domain Model

Status: active

Type: domain-model

Last Verified: 2026-08-16

## Ownership

`blocks/workspace` 是 Workspace、Goal、DoD 和 WorkspaceTemplate 的唯一 owner。application、Run、Orchestrator 和
projection 都只能消费 typed snapshot 或提交命令，不能直接更新这些对象。

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

## Goal

Goal 表达一个 Workspace 的正式目标。owner model 包含 `ID`、`WorkspaceID`、`Text`、
`DefinitionRevision`、previous definition/hash lineage 与 `LineageState`。

## Definition Of Done

DoD 表达可验证的完成条件。owner model 包含 `ID`、`WorkspaceID`、`Items`、`State`、
`DefinitionRevision`、previous definition/hash lineage 与 `LineageState`。

DoD state 为 `draft`、`ready` 或 `satisfied`。`satisfied` 的业务判定来自已密封的 Runtime closeout，不能由 UI、
application 或 Result owner自行推断。

## Definition Revision

- Goal 与 DoD 通过同一 effective definition revision 进入 `RuntimeBootstrap.Definition`。
- Run 创建后保留该不可变引用；运行中的 Workspace 编辑不能就地改写 Run authority。
- 新定义要参与执行时，用户显式提交 workspace `restart` intent。
- restart 由 Run owner 管理 predecessor `ReplacementIntent`、终态投递和 successor 创建，不属于 Workspace
  metadata update。

## Directory Invariants

- `DeliverableDirectory` 是 capability owner 可访问的用户交付根；具体 path scope 仍由 operation contract 校验。
- `InternalDirectory` 只保存 WorkOrch 内部数据，不得被当作用户产物目录。
- file-backed 默认数据根和 workspace 交付根相互分离。
- projection/API 对 artifact 使用逻辑引用，不泄漏内部绝对路径作为跨组件 authority。

## Activation Invariants

- 没有有效 Goal/DoD 时不能形成完整 bootstrap。
- bootstrap 只封存 definition、activation、profile/capability/governance/resource refs 等 typed 值。
- per-CWI proof 先由 bounded semantic plan 提议、由 capability owner确认 feasibility，再由 Orchestrator 内部事件密封。
- 调用方不能上传 target/recipe/check/metric/artifact authority graph。
