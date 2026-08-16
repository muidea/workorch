# WorkOrch Profiles And Capabilities

Status: active

Type: profile-capability-index

Last Verified: 2026-08-16

## Purpose

本目录保存领域特化策略：模板和 Soul 如何解释任务、某种 capability 实际支持哪些 operation，以及这些声明如何
编译为中立 Runtime 输入。它不定义新的 owner 或状态机。

## Boundary

- template/profile 定义默认 Goal/DoD、evidence requirement、capability policy 和 semantic role binding。
- Soul 提供模型行为指导，不拥有 authorization、candidate seal 或 evidence acceptance。
- capability owner定义 operation schema、feasibility、side effect、revision/idempotency 与事实回执。
- Orchestrator只消费编译后的 typed manifest/binding/facts，不按 profile ID 或业务术语分支。
- workorchd负责从 owner snapshots组装 bootstrap，不解释 Runtime 下一步。

## Contents

- [Built-in templates and Souls](builtin-templates-and-souls.md)
- Workspace：[setup draft](workspace/setup-draft-session.md)、[Soul routing](workspace/soul-routing.md)、
  [default Soul resources](workspace/soul-md-default-resource.md)
- Runtime capabilities：[file](runtime-capabilities/file-diff-patch.md)、
  [git](runtime-capabilities/builtin-git-tool.md)、[web](runtime-capabilities/web-search.md)
- MCP：[capability management](mcp/capability-management.md)
- Software development：[code agent](software-dev/code-agent.md)、
  [Goal/DoD evidence](software-dev/goal-coverage-and-dod.md)、
  [LSP bridge](software-dev/lsp-bridge-integration.md)

## Invariants

- profile policy can narrow capabilities; it cannot broaden an owner catalog or bypass approval.
- semantic role names such as planning/execution/validation/artifact are model guidance, not SM-1..SM-4 states.
- operation must pass owner admissibility/feasibility and Orchestrator seal before execution.
- accepted evidence must bind current Run、focus、CWI、dispatch、operation 和 revision。
- historical graph/next-action vocabulary in compatibility data or projection is not a supported Runtime authority contract.
