# WorkOrch Domain Model Overview

Status: active

Type: domain-model

Last Updated: 2026-05-29

## Purpose

本目录用于定义 WorkOrch 的核心业务领域对象、关系、边界和不变量。

本层只回答：

- 系统有哪些核心对象
- 对象之间如何关联
- 哪些字段表达对象本体，哪些只是运行时投影或读模型投影
- 哪些边界必须稳定，才能支撑 runtime flow、module architecture 和 profile/capability 设计

## Domain Groups

- Workspace / Goal / DoD
- Run / Step / Result / Artifact
- Governance: Decision / Approval / Resolution
- Context: Soul / Skill / MCP / Capability / Memory
- Event / Observability / Outcome

## Reading Order

1. [workspace-goal-dod.md](workspace-goal-dod.md)
2. [run-step-result-artifact.md](run-step-result-artifact.md)
3. [governance-decision-approval.md](governance-decision-approval.md)
4. [context-capability-memory.md](context-capability-memory.md)
5. [event-observability.md](event-observability.md)

## Cross-Layer Boundary

- runtime stage、repair contract、context gate、request audit 等动态规则属于
  `00-core/` 和 `20-runtime-flow/`
- 模块职责、代码落点和存储实现属于 `30-module-architecture/`
- software-dev、workspace template、MCP 能力等特化语义属于
  `40-profiles-capabilities/`
