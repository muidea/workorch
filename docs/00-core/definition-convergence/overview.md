# Definition Convergence Contract

Status: normative

Type: core-contract-index

Last Verified: 2026-08-25

## Purpose

本合同定义 Workspace Goal/DoD 如何从 pre-seal input 收敛为 immutable startup input、完整 all-of semantic graph、bounded
Runtime processing 和 terminal completion。它不维护实施阶段或迁移进度。

## Normative Topics

1. [Definition Admission](definition-admission.md)：pre-seal candidate、all-of-only promotion、activation admission 与 startup-input seal。
2. [Canonical Definition Model](canonical-definition.md)：item identity、Definition→CWI mapping、relation、all-of gate 与 completeness。
3. [Semantic Scheduling](semantic-scheduling.md)：dependency-first、ID-free rank、ready projection、stability barrier。
4. [Workspace Clarification](clarification.md)：显式 unsupported relation 与 declared static gap 的 pre-Run blocking ticket。
5. [Runtime Clarification](runtime-clarification.md)：sealed Run 中 implicit semantic gap 的专用 dispatch、ticket 与 replacement resolution。
6. [Context Strategy](context-strategy.md)：完整 authority、immutable C1 partition 与 bounded C5 expansion。
7. [Runtime Integration](runtime-integration.md)：四状态机接入、三条 prerequisite lane 与组件落点。
8. [Activation Flow](activation-flow.md)：candidate→admission→promotion→bootstrap→Run handoff。
9. [Verification](verification.md)：规范性场景与零副作用断言。

## Global Rules

- canonical Definition aggregation、dependency、gate、terminal 和 provider-facing execution schema 只允许 `all_of`。
- unsupported token 只可存在于 admission-only candidate，用于 deterministic rejection；它不是 canonical relation。
- Workspace owner 独占 candidate 与 formal Definition；Run owner 独占 activation/replacement；Orchestrator 独占 Runtime authority。
- Workorchd 做 pure static compile 和 application orchestration，不成为 Definition、ticket 或 Runtime owner。
- static declared gap、dynamic owner readiness 和 runtime-discovered semantic gap 是三条独立、不可互相伪装的 lane。
- pre-seal Profile 声明必须为每个 `prospective_owner_target` owner-fact acquisition 封存 exact
  `ProspectiveSatisfactionContractRef`；Runtime 只能用该 ref 触发 owner-defined pure composite feasibility，不能由
  Orchestrator/Application/LLM 推断 acquisition 后的 owner fact。
