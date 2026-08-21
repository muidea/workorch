# WorkOrch Profiles And Capabilities

Status: normative

Type: profile-capability-index

Last Verified: 2026-08-18

## Purpose

本目录保存领域特化策略：模板和 Soul 如何解释任务、某种 capability 实际支持哪些 operation，以及这些声明如何
编译为中立 Runtime 输入。它不定义新的 owner 或状态机。

## Boundary

- template/profile 定义默认 Goal/DoD、evidence requirement、capability policy 和 semantic role binding。
- Soul 提供模型行为指导，不拥有 authorization、candidate seal 或 evidence acceptance。
- capability owner定义 operation schema、feasibility、side effect、revision/idempotency 与事实回执。
- Orchestrator只消费编译后的 typed manifest/binding/facts，不按 profile ID 或业务术语分支。
- workorchd负责从 owner snapshots组装 bootstrap，不解释 Runtime 下一步。

## Topics

- [Workspace authoring and built-in Template/Soul resources](workspace/overview.md)
- [Runtime capabilities](runtime-capabilities/overview.md)
- [MCP profile and catalog behavior](mcp.md)
- [Software-development profile](software-dev/overview.md)

## Invariants

- profile policy can narrow capabilities; it cannot broaden an owner catalog or bypass approval.
- semantic role names such as planning/execution/validation/artifact are model guidance, not SM-1..SM-4 states.
- operation must pass owner admissibility/feasibility and Orchestrator seal before execution.
- accepted evidence must bind current Run、focus、CWI、dispatch、operation 和 revision。
- historical graph/next-action vocabulary in compatibility data or projection is not a supported Runtime authority contract.
