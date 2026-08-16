# Workspace Block / Run / Result Architecture

Status: active

Type: module-architecture

Last Verified: 2026-08-16

## Purpose

本文件说明 Workspace、Run、Result、Eventstream 四个 Block owner 的正式业务状态职责。

生产主线以 **Runtime** 为准：Workspace 拥有 definition 聚合；Orchestrator 拥有 SM-2/3/4 aggregate 语义，
Run 只提供 opaque 物理持久化/CAS；Result 只持久化 sealed outcome，不裁决 acceptance；Eventstream 只做
审计、查询和 projection rebuild 输入，不参与 Runtime recovery/replay。

## Runtime Boundary

| 组件 | Runtime 边界 |
| --- | --- |
| `blocks/workspace` | 拥有 Workspace / Goal / DoD / Template definition、canonical selection、execution-intent admission 与 active-run release；只向 Run owner转交 typed intent，不选择 Run lifecycle target |
| `blocks/run` | 拥有 SM-1 Run lifecycle/lineage/ReplacementIntent；为 Orchestrator-owned `RuntimeStateRecord` 提供 opaque record query、expected-revision CAS 和 inventory，不解释 facts/authority/dispatch/candidate/governance/delivery ledger |
| `blocks/result` | 消费 sealed `TerminalOutcome`，持久化 Result / Artifact 正式 outcome，并只由自身 reducer 迁移 Result lifecycle；**不判断** evidence quality、DoD coverage 或 accepted/rejected 策略 |
| `blocks/eventstream` | EventHub 订阅 → 有界 EventRecord；投影/审计只读，不是业务 owner 修复、Runtime replay 或状态重建入口 |

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

workorchd、`blocks/run` 与 `kernel/orchestrator` 经 EventHub 读取或引用 workspace definition；Workspace 只发布/转交
immutable execution/release facts，不直接写 Run/Result repository 或选择其状态。

### `blocks/run`

职责：

- Run / Step / ToolCall / Patch / ValidationResult / RuntimeCheckpoint persistence
- SM-1 Run state machine、lineage、terminal-delivery acknowledgement 与 restart `ReplacementIntent`
- typed `RuntimeBootstrap` 是 Run 创建时的 immutable Definition seed；`RuntimeStateRecord` 作为 opaque payload 只做
  presence/revision/scope query 与 expected-revision CAS，内部 invariant 归 Orchestrator

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

Step 与部分过程证据对象可继续作为 **非权威投影 / legacy 过程面** 存在（观测、分页、历史关联），但不是 Runtime drive 调度权威。

Run activation/continuation/cancel/supersede/terminal 的状态变更只由 Run owner 的内部 reducer 完成。Orchestrator
只发布 immutable governance continuation/sealed terminal outcome，调用方不得指定 Run target state 或绕过 evidence
gate 直接 Block/Fail/Validate/Complete active Run。

对象拆分：

- `Run`：执行生命周期本体、active blocker refs、priority、failure reason、RuntimeBootstrap binding 与 opaque
  RuntimeStateRecord persistence slot
- `Step`：过程单元输入输出与 lineage 关联（非 Runtime 调度权威）
- run evidence：`ToolCall` / `Patch` / `ValidationResult` / `RuntimeCheckpoint` — 只表达运行证据，不直接表达 accepted business outcome
- run-scoped coverage / focus：随 opaque RuntimeStateRecord 持久化；Run owner 不读取、重排或校验其语义
- run-scoped continuation：`ArtifactContinuation` 等 internal state 持有（若仍使用）

Current-baseline Restart 先把 immutable `ReplacementIntent` CAS 持久化在 predecessor；只有 sealed superseded terminal
依次被 Result、Workspace、Run owner确认后，Run-private reducer 才创建并激活一个 successor。Application、Workspace、
Orchestrator 和 projection 都不能提前创建或链接它。非当前 baseline 返回 `data_reset_required`。

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

- formal bounded EventRecord persistence
- EventHub subscriber for audit/query/projection rebuild

对象拆分：

- formal EventRecord append-only persistence
- workspace/run/owner 过滤与 projection rebuild 输入面
- snapshot / observability 所需 persisted event 投影输入

约束：

- `blocks/eventstream` 只负责正式事件记录、查询与派生 projection rebuild 输入
- 它不直接决定 workspace/run outcome 语义
- 它不作为 SM-4 Runtime recovery/replay 输入，不从 event history 重建 authority 或 aggregate
- 无生产发布方的 禁止的 Step / 通用 progress 通知不作为生产订阅合同

## Boundary

这些组件负责正式业务状态或正式业务证据，不应负责：

- provider request 组装 / Runtime candidate 解码（`blocks/llm`）
- shell/MCP/file/git/web/ssh 执行
- Orchestrator drive / reducer / sealed dispatch
- Web/TUI 展示策略

## Flow Participation

这些模块主要参与：

- activation -> workorchd 编译 bootstrap，Workspace owner准入 intent，Run owner创建/恢复/记录 replacement
- execution -> Orchestrator drive；Run owner CAS 持久化 Runtime aggregate；过程证据投影
- acceptance -> Orchestrator 密封 terminal；Result/Workspace/Run 各自 owner-local delivery
- observability -> EventRecord persistence and replay（只读投影）

## Responsibility Split Summary

| 关注点 | 主要责任 |
| --- | --- |
| workspace definition / Goal / DoD | `blocks/workspace` |
| run lifecycle + RuntimeStateRecord opaque persistence/CAS | `blocks/run` |
| SM-2/3/4 aggregate semantics、drive / evidence gate / acceptance 裁决 | `kernel/orchestrator`（非本文件 owner） |
| run process evidence / non-authoritative step 投影 | `blocks/run` |
| result / artifact persistence（不裁决） | `blocks/result` |
| formal bounded event persistence / query / projection rebuild input | `blocks/eventstream` |

这些责任可以彼此引用，但不应在同一模块里混成“单一大状态拥有者”。

## Source Documents

- `architecture-overview.md`
- `kernel-orchestrator-governance.md`
- `../10-domain-model/run-step-result-artifact.md`
- `internal/modules/blocks/workspace/readme.md`
- `internal/modules/blocks/run/readme.md`
- `internal/modules/blocks/result/readme.md`
- `internal/modules/blocks/eventstream/readme.md`
