# WorkOrch Domain Model Overview

Status: active

Type: domain-model

Last Updated: 2026-07-14

## Purpose

本目录用于定义 WorkOrch 的核心业务领域对象、关系、边界和不变量。

本层只回答：

- 系统有哪些核心对象
- 对象之间如何关联
- 哪些字段表达对象本体，哪些只是运行时投影或读模型投影
- 哪些边界必须稳定，才能支撑 runtime flow、module architecture 和 profile/capability 设计

生产 runtime 主链为 **Runtime V2 only**。领域模型保留业务对象边界；V2 权威过程面（Bootstrap / Facts / CoverageFocus / EvidenceLedger / SealedDispatch / Result 裁决边界）在 `run-step-result-artifact.md` 与 `../00-core/runtime-stage-contract.md` 对齐。Step 在领域叙述中可作为过程证据 / 非权威投影 / legacy 过程面存在，但不是 V2 调度权威。

## Domain Groups

- Workspace / Goal / DoD
- Run / Runtime V2 process authority / Result / Artifact（含 Step 过程投影边界）
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

- runtime stage 语义、Runtime V2 drive、repair contract、context gate、request audit 等动态规则属于
  `00-core/` 和 `20-runtime-flow/`
- 模块职责、代码落点和存储实现属于 `30-module-architecture/`
- software-dev、workspace template、MCP 能力等特化语义属于
  `40-profiles-capabilities/`
- 不得把 V1 `StagePacket` / `NextAction` / graph dispatch / `ExecutionContextPatch` / 自动 replan successor
  重新声明为领域生产对象
