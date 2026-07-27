# Workspace Block / Run / Result Architecture

Status: active

Type: module-architecture

Last Updated: 2026-07-27

## Purpose

本文件说明 Workspace、Run、Result、Eventstream 四个 Block owner 的正式业务状态职责。

生产主线以 **Runtime V2** 为准：Workspace 拥有 definition 聚合；Run 拥有 V2 aggregate 持久化与 CAS；Result 只持久化正式 outcome，不裁决 acceptance；Eventstream 只做审计/回放输入。

## Runtime V2 Boundary

| 组件 | Runtime V2 边界 |
| --- | --- |
| `blocks/workspace` | 拥有 Workspace / Goal / DoD / Template definition 聚合与 revision；不启动 Run、不判定 Result、不执行治理恢复 |
| `blocks/run` | 拥有 Run lifecycle 与 **Runtime V2 aggregate**（`RuntimeBootstrap`、`RuntimeStateRecord`、facts/authority/dispatch/candidate、governance binding）的 CAS 持久化；只由自身 reducer 迁移 lifecycle，不编排跨组件收敛 |
| `blocks/result` | 持久化 Result / Artifact 正式 outcome，并只由自身 reducer 迁移 Result lifecycle；**不判断** evidence quality、DoD coverage 或 accepted/rejected 策略 |
| `blocks/eventstream` | EventHub 订阅 → 规范化 EventRecord；投影/审计只读，不是业务 owner 修复入口 |

显式非目标（不得再写成生产主链）：

- `StagePacket` / `NextAction` / `DispatchNextStep` 作为调度权威
- Run owner 上的 Validate/Complete facade、execution-context patch resume
- Result block 自行裁决 acceptance

## Main Components

### `blocks/workspace` stateful block

职责：

- Workspace lifecycle
- Goal / DoD formal definition 与 definition revision
- workspace owner state and workspace-level projection inputs

Canonical owner states:

- `active`
- `blocked_on_approval`
- `superseded`

Decision blockers do not change workspace owner state. They are exposed through
run/ticket/projection fields owned by `blocks/run`, `blocks/decision` and
workorchd read model projection.

`blocks/run` 与 `kernel/orchestrator` 经 EventHub 读取 workspace definition；Workspace 不反向写 Run/Result。

### `blocks/run`

职责：

- Run / Step / ToolCall / Patch / ValidationResult / RuntimeCheckpoint persistence
- Run state machine primitives
- Runtime V2 typed state：`RuntimeBootstrap`、`RuntimeStateRecord`（facts、authority、sealed dispatch、candidate attempt、governance consumption/binding）与 expected revision CAS

Canonical `RunState` values:

- `pending`
- `active`
- `blocked_on_decision`
- `blocked_on_approval`
- `completed`
- `failed`
- `cancelled`
- `superseded`

Canonical `StepState` values:

- `pending`
- `executing`
- `succeeded`
- `failed`
- `skipped`
- `cancelled`

Step 与部分过程证据对象可继续作为 **非权威投影 / legacy 过程面** 存在（观测、分页、历史关联），但不是 V2 drive 调度权威。

Run supersede / terminal / blocker 的状态变更只由 Run owner 的内部 reducer 完成。Orchestrator 仅发布 immutable
governance/terminal fact，调用方不得指定 Run target state 或绕过 evidence gate 直接 Validate/Complete active Run。

对象拆分：

- `Run`：执行生命周期本体、active blocker refs、priority、failure reason、V2 aggregate 持有
- `Step`：过程单元输入输出与 lineage 关联（非 V2 调度权威）
- run evidence：`ToolCall` / `Patch` / `ValidationResult` / `RuntimeCheckpoint` — 只表达运行证据，不直接表达 accepted business outcome
- run-scoped coverage / focus：随 V2 facts / coverage focus 持久化；Run owner 不重排 focus
- run-scoped continuation：`ArtifactContinuation` 等 internal state 持有（若仍使用）

### `blocks/result` stateful block

职责：

- Result / Artifact 正式业务结果模型
- result/artifact persistence contract

对象拆分：

- `Result`：正式 DoD-backed accepted / rejected business outcome 的 **持久化**；Orchestrator 只密封 terminal payload，
  Result owner 消费 delivery fact 后在自身 reducer 中构造并写入本 block
- `Artifact`：deliverable artifact 与 process evidence artifact 的正式归属对象
- closeout 输入：只消费 Orchestrator 已裁决所需的 evidence refs、artifact refs、result policy 字段；本 block 不重新跑 gate

### `blocks/eventstream`

职责：

- formal EventRecord persistence
- EventHub subscriber for audit/replay

对象拆分：

- formal EventRecord append-only persistence
- workspace/run/step 过滤与 replay 输入面
- snapshot / observability 所需 persisted event 投影输入

约束：

- `blocks/eventstream` 只负责正式事件记录与回放输入
- 它不直接决定 workspace/run outcome 语义
- 无生产发布方的 V1 Step / 通用 progress 通知不作为生产订阅合同

## Boundary

这些组件负责正式业务状态或正式业务证据，不应负责：

- provider request 组装 / Runtime V2 candidate 解码（`blocks/llm`）
- shell/MCP/file/git/web/ssh 执行
- Orchestrator drive / reducer / sealed dispatch
- Web/TUI 展示策略

## Flow Participation

这些模块主要参与：

- activation -> workspace definition 读取 + Run 创建（可携带 `RuntimeBootstrap`）
- execution -> Orchestrator drive；Run owner CAS 持久化 V2 aggregate；过程证据投影
- acceptance -> Orchestrator 密封 terminal；Result/Workspace/Run 各自 owner-local delivery
- observability -> EventRecord persistence and replay（只读投影）

## Responsibility Split Summary

| 关注点 | 主要责任 |
| --- | --- |
| workspace definition / Goal / DoD | `blocks/workspace` |
| run lifecycle + Runtime V2 aggregate CAS | `blocks/run` |
| drive / evidence gate / acceptance 裁决 | `kernel/orchestrator`（非本文件 owner） |
| run process evidence / non-authoritative step 投影 | `blocks/run` |
| result / artifact persistence（不裁决） | `blocks/result` |
| formal event persistence / replay input | `blocks/eventstream` |

这些责任可以彼此引用，但不应在同一模块里混成“单一大状态拥有者”。

## Source Documents

- `architecture-overview.md`
- `kernel-orchestrator-governance.md`
- `../10-domain-model/run-step-result-artifact.md`
- `internal/modules/blocks/workspace/readme.md`
- `internal/modules/blocks/run/readme.md`
- `internal/modules/blocks/result/readme.md`
- `internal/modules/blocks/eventstream/readme.md`
