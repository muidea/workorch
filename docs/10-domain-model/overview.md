# WorkOrch Domain Model Overview

Status: active

Type: domain-model-index

Last Verified: 2026-08-16

## Purpose

本目录定义正式业务对象、owner、关系与不变量。它不定义 HTTP DTO、repository encoding、UI view state 或第二套
Runtime 调度模型。

## Domain Groups

| Group | Formal owners | Document |
| --- | --- | --- |
| Definition | Workspace / Goal / DoD：`blocks/workspace` | [workspace-goal-dod.md](workspace-goal-dod.md) |
| Execution and outcome | Run：`blocks/run`; Runtime authority：Orchestrator; Result/Artifact：`blocks/result` | [run-step-result-artifact.md](run-step-result-artifact.md) |
| Governance | Decision：`blocks/decision`; Approval：`blocks/approval`; policy：Governance | [governance-decision-approval.md](governance-decision-approval.md) |
| Context and capability resources | Soul、Skill、MCP Registry、Memory 及 capability owners | [context-capability-memory.md](context-capability-memory.md) |
| Audit and read side | Eventstream owner 与 workorchd projection | [event-observability.md](event-observability.md) |

## Cross-Layer Rules

- 所有正式状态都由唯一 owner 的内部事件转换；其他组件只能发送 typed intent/fact。
- Run 物理持久化 opaque Runtime aggregate，但不解释 SM-2/3/4；Orchestrator 是其唯一语义 owner。
- Step、tool call、patch、checkpoint、timeline 和 progress 可以被投影，但不是 next-action authority。
- Eventstream/projection 可从 owner facts 重建；不得反向重建 authority。
- 领域特化属于 template/profile/Soul/capability，Runtime core 保持中立。
- Runtime 动态规则以 [core contract](../00-core/runtime-stage-contract.md) 为准。
