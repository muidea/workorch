# WorkOrch Active Roadmap

Status: active

Type: roadmap-index

Last Verified: 2026-08-17

## Purpose

本目录只保留真实未关工作。代码已完成、仅作为回归依据或包含 dated run/schema/benchmark 的文档必须进入
`90-archive/` 或 `60-verification/`，不能继续与当前实施计划并列。

## Active Work

| ID | 状态 | 唯一目标 | Plan |
| --- | --- | --- | --- |
| WGSC | open / code closure | 将大量 Goal/DoD 编译为唯一语义依赖图与稳定 Definition partition，由 SM-2 在不使用序号/ID 业务排序、C1 不随同 partition focus 漂移的前提下持续推进 CWI | [Workspace Goal/DoD Global Semantic Convergence](workspace-goal-dod-global-semantic-convergence-closure-plan.md) |
| RUNTIME-LIVE | deployed verification pending | 在 fresh development data 与真实 provider 上证明 semantic seal、candidate/dispatch、owner receipt/evidence、existing-target revision-bound replacement、后续 CWI 与 terminal delivery 持续收敛 | [Runtime Positive-Convergence Live Closure](runtime-single-owner-model-capability-output-closure-plan.md) |
| WRE-LIVE | deployed verification pending | 证明 Workspace model/reasoning selection 在 Web/TUI、Resume/Restart、active configuration 和双 Workspace 间保持单 Owner | [Workspace Reasoning Effort Deployed Verification](workspace-reasoning-effort-single-owner-closure-plan.md) |

## Stable Regression Contract

[Runtime Owner Convergence Verification Matrix](../../60-verification/runtime-owner-convergence-matrix.md) 固化 CWI、
candidate、single-owner、restart、budget、delivery 等已实现门禁。它是验证合同，不是新的 active 实施队列。

## Recently Archived

- [CWI single-owner closure](../../90-archive/completed/runtime-cwi-reconciliation-single-owner-closure-plan.md)
- [Orchestrator/Workorchd single-owner code runbook](../../90-archive/completed/runtime-single-owner-code-closure-runbook.md)
- [22-component complexity snapshot](../../90-archive/records/runtime-22-component-function-complexity-inventory-2026-08-16.md)
- [SO-MC code closure record](../../90-archive/records/runtime-single-owner-model-capability-output-code-closure-record-2026-08-16.md)
- [WRE code closure record](../../90-archive/records/workspace-reasoning-effort-code-closure-record-2026-08-16.md)
- [Runtime contract drift closure](../../90-archive/completed/runtime-contract-drift-closure-plan.md)
- [Runtime contract drift code closure record](../../90-archive/records/runtime-contract-drift-code-closure-record-2026-08-16.md)

## Rules

1. `00-core`、Orchestrator README 和 typed contracts 定义 Runtime authority；active plan 只描述未关验收。
2. live 失败以最早 persisted counterexample 为准，但不能据此增加第二 Owner、第五状态机或字段级旁路。
3. managed mock、局部制品、revision 增长或正确 fail-closed 不能替代完整正向 terminal chain。
4. 完成的计划立即归档；历史 schema/version/run ID 不进入当前事实或组件 README。

## Navigation

- [Current State](../../01-overview/current-state.md)
- [Current Unclosed Matrix](../../01-overview/current-unclosed-matrix.md)
- [Documentation Map](../../README.md)
- [Completed Archive](../../90-archive/completed/overview.md)
- [Records Archive](../../90-archive/records/overview.md)
