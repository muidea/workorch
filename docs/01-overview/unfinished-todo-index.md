# WorkOrch Unfinished Work Index

Status: active

Type: index

Last Verified: 2026-08-16

## Current Work

| Priority | Work | Scope | Source |
| --- | --- | --- | --- |
| P0 | Runtime positive-convergence live closure | fresh-data real-provider persisted chain、existing-target replacement、later CWI、terminal delivery | [RUNTIME-LIVE](../70-roadmap/active/runtime-single-owner-model-capability-output-closure-plan.md) |
| P1 | Workspace reasoning effort deployed verification | Web/TUI、Resume/Restart、active config、two-workspace isolation | [WRE-LIVE](../70-roadmap/active/workspace-reasoning-effort-single-owner-closure-plan.md) |

只有上述两项占用 active 队列。细化状态见 [Current Unclosed Matrix](current-unclosed-matrix.md)。

## Required But Not Open

[Runtime Owner Convergence Verification Matrix](../60-verification/runtime-owner-convergence-matrix.md) 是每次 Runtime
变更必须持续通过的回归合同；它已实现，不是额外实施队列。

## Candidates

provider failover、tokenizer/native usage 精度和 context-too-large 差异化位于
[Demand Candidates](../70-roadmap/candidates/overview.md)，尚未晋级，不得作为当前缺陷或验收 blocker。

## Archive Rule

已完成代码切片、旧 schema/run 反例、dated live 日志、LOC/CC 快照和 superseded 设计只在 `90-archive/` 保留。
它们可以提供回归夹具，但不能覆盖 core contract、组件 README 或当前 active plan。
