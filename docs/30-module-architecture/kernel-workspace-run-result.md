# Kernel Workspace / Run / Result Architecture

Status: active

Type: module-architecture

Last Updated: 2026-05-30

## Purpose

本文件说明 kernel 中负责正式业务状态的模块职责。

## Main Modules

### `kernel/workspace`

职责：

- Workspace lifecycle
- Goal / DoD formal definition
- workspace-level state projection inputs

### `kernel/run`

职责：

- Run / Step / ToolCall / Patch / ValidationResult / RuntimeCheckpoint persistence
- Run state machine primitives

更细拆分方向：

- `Run`
  - 承担执行生命周期本体、active blocker refs、priority、failure reason
- `Step`
  - 承担单次执行单元输入输出与 graph/lineage 关联
- run evidence
  - `ToolCall` / `Patch` / `ValidationResult` / `RuntimeCheckpoint`
  - 只表达运行证据，不直接表达 accepted business outcome
- run-scoped coverage ledger
  - `CoverageWorkItem` / `coverageWorkItemContext` 的正式持有和更新输入
- run-scoped continuation
  - `ArtifactContinuation` 的正式 internal state 持有

### `kernel/result`

职责：

- Result / Artifact 正式业务结果模型
- result/artifact persistence contract

更细拆分方向：

- `Result`
  - 正式 DoD-backed accepted / rejected business outcome
- `Artifact`
  - deliverable artifact 与 process evidence artifact 的正式归属对象
- closeout gate input
  - 只消费 DoD assessment、accepted evidence refs、artifact refs、result policy 所需的正式输入

### `kernel/eventstream`

职责：

- formal EventRecord persistence
- EventHub subscriber for audit/replay

更细拆分方向：

- formal EventRecord append-only persistence
- workspace/run/step 过滤与 replay 输入面
- snapshot / observability 所需 persisted event 投影输入

约束：

- `kernel/eventstream` 只负责正式事件记录与回放输入
- 它不直接决定 workspace/run outcome 语义

## Boundary

这些模块负责正式业务状态或正式业务证据，不应负责：

- provider request 组装
- shell/MCP/file/git/web/ssh 执行
- Web/TUI 展示策略

## Flow Participation

这些模块主要参与：

- activation -> Run creation
- execution -> Step/ToolCall/Patch/ValidationResult/Checkpoint evidence
- acceptance -> Result/Artifact creation
- observability -> EventRecord persistence and replay

## Responsibility Split Summary

| 关注点 | 主要 kernel 责任 |
| --- | --- |
| run lifecycle | `kernel/run` |
| run evidence | `kernel/run` 及其 evidence objects |
| run-scoped coverage ledger / continuation | `kernel/run` |
| result / artifact closeout | `kernel/result` |
| formal event persistence / replay input | `kernel/eventstream` |

这些责任可以彼此引用，但不应在同一模块里混成“单一大状态拥有者”。

## Source Documents

- `architecture-overview.md`
- `../90-archive/completed/result-event-development-plan.md`
- `../90-archive/records/delivery-history.md`
