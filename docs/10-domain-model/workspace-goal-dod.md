# Workspace / Goal / DoD Domain Model

Status: active

Type: domain-model

Last Updated: 2026-05-30

## Purpose

本文件定义 Workspace、Goal、DoD 及其目录边界在 WorkOrch 中的领域角色。

## Core Entities

### Workspace

Workspace 是 WorkOrch 的顶层业务容器。它承载：

- 用户交付物根目录
- WorkOrch 内部运行态目录
- Goal / DoD 定义
- 当前活跃 Run
- 默认和阶段性上下文绑定

建议稳定字段：

- `id`
- `title`
- `description`
- `deliverableDirectory`
- `internalDirectory`
- `state`
- `activeRunId`
- `defaultSoulId`

### Goal

Goal 表示当前 workspace 的目标定义。

它是 runtime 收敛和 replan 的正式输入之一，不应被自然语言摘要替代。

建议稳定字段：

- `id`
- `workspaceId`
- `text`
- `definitionRevision`
- `previousDefinitionHash`
- `previousGoalId`
- `lineageState`

### DoD

DoD 表示当前 workspace 的验收定义。

它是 accepted result 的正式前置条件，不应被模糊完成结论替代。

建议稳定字段：

- `id`
- `workspaceId`
- `items`
- `state`
- `definitionRevision`
- `previousDefinitionHash`
- `previousDoDId`
- `lineageState`

## Definition Revision And Lineage

Goal / DoD 的正式定义不应只有当前文本，还必须能稳定表达版本与变更 lineage。

当前对象层至少应稳定承接：

| 字段 | 当前方向 |
| --- | --- |
| `definitionRevision` | 当前有效定义版本；用于 activation / coverage baseline / successor run 继承边界 |
| `previousDefinitionHash` | 上一版本定义内容的稳定 hash；用于判断 definition change impact |
| `previousGoalId` / `previousDoDId` | 可选的上一版本对象引用；用于保持定义链 |
| `lineageState` | `active` / `superseded` / `historical` / 等价中立状态 |

约束：

- Goal 与 DoD 可以独立演进，但同一 activation 必须基于一组明确的 effective definition
- `definitionRevision` 变化后，runtime 不得把旧 revision 的 active coverage scope 直接当成当前定义
- `previousDefinitionHash` 只表达定义前后关系，不替代当前对象本体

## Definition Change Impact Boundary

当 Goal / DoD 发生变更时，对象层至少需要支持以下判断：

- 当前 active run 是否仍可继续沿用旧 definition
- 哪些 run / coverage ledger / result closeout 需要按新 revision 重建
- 哪些旧 definition 只能保留为 historical reference

这些判断的 flow 细节继续由 activation / verification / repair / replan 文档承接，
但版本与 lineage 的对象边界由本文件定义。

## Directory Boundary

Workspace 应使用双目录模型：

- `deliverableDirectory`
  - 用户项目根目录
  - shell/file/patch/MCP 面向用户交付物执行
- `internalDirectory`
  - WorkOrch 内部状态目录
  - 存放 runtime state、events、memory、context、llm-calls、artifacts metadata 等

推荐根布局：

```text
.workorch-workspaces/<workspace-id>/
.workorch-data/internal/workspaces/<workspace-id>/
```

其中：

- `deliverableDirectory` 是用户项目根目录
- `internalDirectory` 是 WorkOrch-managed runtime state root

禁止将以下内容写入 `deliverableDirectory`：

- run records
- step/tool-call records
- event streams
- memory recall traces
- runtime checkpoints
- LLM request/response logs
- governance tickets/resolutions

## Responsibility Boundary

Workspace 本体定义的是：

- 当前业务空间
- 当前正式目标
- 当前正式验收定义
- 交付物边界和内部状态边界

Workspace 不等于：

- 单次执行过程
- 单次决策工单
- 单次审批工单
- 单次 memory recall
- 单次事件流投影

## Invariants

- 新 workspace 创建时必须同时建立 `deliverableDirectory` 和 `internalDirectory`
- runtime 工具必须显式绑定 `deliverableDirectory`
- repository / read-model / payload refs 必须显式绑定 `internalDirectory`
- public API 不得暴露内部绝对路径
- Goal / DoD 原文必须保持为正式定义，不得仅保留压缩摘要
- deliverableDirectory 与 internalDirectory 的职责必须严格分离，不能由 LLM/tool 自行决定内部证据写入位置
- Goal / DoD 的当前有效定义必须可追溯到 definition revision 与上一版本 hash / lineage
- superseded / historical 的旧 definition 不得继续作为当前 active definition 投影给新 activation

## Projection Boundary

Workspace snapshot、workspace detail、task progress、context bundle 等都只是
Workspace 的投影，不得反向覆盖正式 Goal / DoD 定义。

## Source Documents

- `../30-module-architecture/storage-readmodel.md`
- `../00-core/runtime-stage-contract.md`
- `../01-overview/current-state.md`
