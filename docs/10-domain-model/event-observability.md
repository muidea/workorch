# Event / Observability Domain Model

Status: active

Type: domain-model

Last Updated: 2026-05-30

## Purpose

本文件定义 WorkOrch 中 Event、Observability、Outcome、TaskProgress 等投影对象的领域边界。

## Core Entities

### EventRecord

EventRecord 是持久化的正式事件对象。

建议稳定字段：

- `id`
- `version`
- `topic`
- `occurredAt`
- `sourceModule`
- `subjectType`
- `subjectId`
- `workspaceId`
- `runId`
- `stepId`
- `payload`
- `payloadRef`
- `payloadSummary`

稳定边界补充：

- 大 payload 不应默认内嵌在主要事件投影中
- 正式事件应优先持有 `payloadRef`、payload summary 或等价引用化表达
- 事件元数据必须足以支持按 workspace/run/step 过滤、排序、分页和回放

#### Event Payload Summary Contract

`payload` / `payloadSummary` / `payloadRef` 当前应保持如下对象层边界：

| 字段 | 当前语义 |
| --- | --- |
| `payload` | 仅用于小型、稳定、适合默认投影的结构化事件摘要 |
| `payloadSummary` | 大 payload 的 compact 摘要；优先服务默认读模型与事件列表 |
| `payloadRef` | 显式打开 raw/detail 的受控引用；不替代默认列表投影 |

约束：

- `payloadSummary` 与 `payloadRef` 可同时存在
- 大 payload 不得因为存在 `payloadRef` 就完全丢失默认摘要
- event list / snapshot / observability 默认优先消费 `payloadSummary` 或小型 `payload`
- raw payload 打开链路属于产品/API/detail 层，不改变 EventRecord 本体身份

### Run Observability

Run observability 是围绕单次 Run 的事件与证据投影。

它可包含：

- timeline
- step summary
- tool call summary
- runtime checkpoint summary
- memory recall trace
- persisted event timeline

Run observability 应优先消费事件元数据、summary 和 refs，而不是把 raw payload 作为默认主视图数据源。

### Outcome / TaskProgress

它们属于 Workspace / Run 的读模型投影。

它们应优先表达：

- 当前最新 accepted Result 的状态
- 若无 accepted Result，则表达当前最新 terminal Run 的状态和缺口
- 当前 coverage / progress 摘要
- 当前剩余缺口

而不是成为正式状态拥有者。

#### Latest Result / Run Selection Rule

`Outcome` / `TaskProgress` 的投影优先级应稳定为：

1. 最新 non-superseded accepted Result
2. 若无 accepted Result，则最新 terminal Run
3. 若仍无 terminal Run，则当前 active Run 的 progress/blocker 摘要

约束：

- 历史 accepted / rejected Result 必须继续保留在列表中，但不得覆盖更新的 terminal run 投影
- 已被后续成功证据覆盖的旧失败，只能保留为 lineage / diagnostics
- workspace blocker / taskProgress 必须优先跟随 active run，而不是历史 terminal run

#### Outcome / TaskProgress Projection Surface

对象层至少应能稳定承接：

| 投影对象 | 关键方向 |
| --- | --- |
| `WorkspaceOutcome` | 当前 workspace 级 closure、active blocker、latest accepted result 或 latest terminal run |
| `RunOutcome` | 当前 run 级 closure、accepted/rejected result、latest closure evidence |
| `TaskProgress` | milestones / blockers / recovery / repairLoop / current gap |

这些投影只表达“当前视图”，不替代 Run / Result / Goal / DoD 本体。

## Boundary

EventRecord 与 Observability 的边界是：

- Event stream 用于审计、回放、观察
- 它不是正式业务状态拥有者
- read model 可以消费 event stream，但不能把 event stream 变成唯一 truth source

## Invariants

- 事件持久化应保留业务 topic 语义
- 大 payload 应通过 ref 暴露，不应默认内嵌到主要投影
- outcome / taskProgress / snapshot 必须服从最新终态证据优先
- 读模型只能投影，不得反向覆盖正式 Goal / DoD 或 runtime ledger
- append-only event stream 与 raw payload ref 可服务审计和回放，但都不应成为唯一 truth source
- event payload summary 与 raw payload ref 应可同时存在：summary 用于默认读模型，ref 用于显式打开详情

## Projection Boundary

以下都应视为投影而非本体：

- Workspace snapshot
- Run snapshot
- Run observability
- task progress
- context bundle
- event list page

## Product / Read-Model Mapping

本层与产品/读模型的稳定衔接方向至少包括：

- `run snapshot`
  - `outcome`
  - `taskProgress`
  - `eventRecords`
- `workspace snapshot`
  - `outcome`
  - `taskProgress`
  - workspace 级 event / run 聚合
- `run observability`
  - timeline / evidence / latest checkpoint / closure trace
- `events`
  - 分页、过滤、回放输入面

这些对象的字段级 API 语义继续由 `service-api.md` 承接，
但本体边界由本文件定义。

## Source Documents

- `../90-archive/completed/result-event-development-plan.md`
- `../90-archive/superseded/workspace-data-layout-plan.md`
- `../50-product-interface/service-api.md`
- `../90-archive/records/delivery-history.md`
- `../00-core/runtime-stage-contract.md`

## Open Migration Notes

- Event payload summary / raw detail 的模块实现与打开链路继续由模块层和产品层补细。
- Workspace / Run outcome 与 taskProgress 的字段级 API 表继续由 product-interface 层维护。
