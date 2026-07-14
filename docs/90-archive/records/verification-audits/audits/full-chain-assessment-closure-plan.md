# WorkOrch Full-Chain Assessment Closure Audit

Status: superseded-by-runtime-contract-retry-audit

Type: audit

Last Updated: 2026-06-22

## Purpose

本文承接 WorkOrch 全链路评估中的审计性内容，聚焦跨层缺口、收口方向和已确认风险，不再把它作为主计划文档使用。

Current tracking note: this historical audit is retained as a source record.
Active runtime-contract convergence closure is tracked in
`../../70-roadmap/active/runtime-contract-retry-full-chain-audit-plan.md` and
`../../70-roadmap/active/runtime-contract-retry-convergence-closure-plan.md`.
Do not use this document alone as an active code-closure backlog.

## Cross-Layer Audit Scope

- L0 合同模型：Goal / DoD / Run / Step / Decision / Approval / Result / Artifact / Memory
- L1 Orchestrator：executionGraph、follow-up、completion gate、repair / replan
- L2 LLM Context：prompt layering、context budget、redaction、usage
- L3 Template / Soul：`software-dev` 六阶段 Soul 与特化策略
- L4 Read Model / UI：Web/TUI/CLI 展示与状态刷新
- L5 E2E：deployed e2e、mock provider、复杂链路回归

## Confirmed Full-Chain Risks

- `completionState=continue` 且无 actionable continuation 时，容易在 validation / repair / replan 之间失去统一收口
- 全局 prompt 容易混入 template-specific 或 software-dev-specific 语义
- 出站 LLM context 存在历史治理 secret 泄漏风险
- 复杂链路 e2e 容易出现默认 skip 或场景覆盖盲区
- Web/TUI/CLI 展示若不复用同一 read model，容易出现字段漂移

## Closure Directions

- graph exhausted 必须进入明确的 validation / repair / replan 分流
- core prompt 只保留中立 contract；capability 规则和 template/Soul guidance 必须分层注入
- provider request 出口必须统一 redaction
- 关键 e2e 不允许默认 skip
- read model / UI 必须复用同一业务投影

## Current Residual Themes

- completion follow-up expansion limit 与 provider retry 轻量化仍需持续守护
- optional file read / patch preview repair / evidence quality gate 仍需持续回归
- full-chain audit 的残余问题应继续分发到 runtime-flow、verification 和 profile 文档，而不是重新聚回单一大计划文档

## Source Documents

- `../../20-runtime-flow/overview.md`
- `../core-runtime-verifier.md`
