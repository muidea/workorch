# WorkOrch Current Unclosed Matrix

Status: active

Type: overview

Last Updated: 2026-07-30

## Purpose

汇总 Runtime V2 代码收口与 **2026-07-16 文档清理** 之后仍未关闭的事项。
已完成的过程计划、假活跃产品日记、dated live 记录与 verification artifacts **不再占用本矩阵**。

## Closed / Archived

Runtime V2 production path is V2-only and the V1 mainline is removed. All dated closure records, including
freshness and candidate-admission records, are archive-only regression baselines. They do not appear in this
matrix as separately actionable work and cannot override a newer persisted aggregate counterexample.

2026-07-30 audit also removed OPS-1: it referred to a Runtime V1 maintenance command that has no code, typed
contract, or CLI entry point. It is superseded by the V2-only production boundary and remains archive history,
not an operational task.

## Residual Open

### A. Runtime V2 正向收敛

`RUNTIME-SMO-R2` 为当前唯一 Runtime V2 收口项。它由 current-schema recovery 的明确反例重新打开：Run inventory
越权读取 SM-2/3/4、泛化 `ErrConflict` 误入 CAS retry、retry timer 未按 persisted owner lease 合并、不可读 schema
Run 仍被 resume、startup 在 Run 常规 command ready 前使用 Get/启动 drive，以及 projection 将历史 attempt 伪装为
running。实施顺序、组件与验收见
[活动收口计划](../70-roadmap/active/runtime-v2-single-owner-state-machine-closure-plan.md)。

历史 [RUNTIME-SMO completed record](../90-archive/completed/runtime-v2-single-owner-state-machine-closure-plan.md)
仍只提供回归基线，不是当前实施规范。

### B. 运行单元架构 / 生命周期

ARCH-1 已完成并归档：22 组件的 pkg-purity、concrete Result、原始诊断、outcome 唯一化、deployed restart
以及 workspace delete → late projection event 验证均已通过，详见
[completed matrix](../90-archive/completed/component-contract-and-regression-closure-matrix.md)。

### C. 可选验证

| ID | 项 | 说明 | 历史过程稿 |
| --- | --- | --- | --- |
| OPT-1 | file-patch deployed live | 可选：`patch apply + validation + DoD closeout` 一次 deployed live | [file-patch-side-effect-owner-migration-plan.md](../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

### D. 延期候选（非 active）

| ID | 项 | 说明 |
| --- | --- | --- |
| CAND-LLM-BUDGET | LLM context budget 高级项 | provider failover、真实 tokenizer/原生 usage 精度、context-too-large 差异化等 → [candidates](../70-roadmap/candidates/next-stage-demand-candidates.md) |

## Judgment

- **代码主链**：Runtime V2 only；V1 authority 不再是问题来源。complete candidate document 必须先持久化，
  single evaluator 只返回完整 typed diagnostic；是否 correction/final 由 SM-3 internal event 决定。owner
  feasibility 是正交 typed fact。`RUNTIME-SMO-R2` 只收口新的 recovery/owner/projection 反例；旧 SMO/C7/C8
  继续只作为回归反例来源。
- **正向收敛基础**：既有 bootstrap、CAS、sealed authorization、三种 TargetPolicy、request session、receipt与
  terminal delivery继续是回归基线；latest persisted aggregate反例优先于过去的本地绿灯和阶段完成声明。
- **文档**：以 `00-core` + 模块 readme + `current-state` 为入口；不得再从已归档 plan 推导实现约束。
- **Active roadmap**：`RUNTIME-SMO-R2` 与可选 OPT-1；ARCH-1、历史 RUNTIME-SMO、旧 VPC、OPS-1、RCA-1、
  RPF 和 RPA-1 均归 archive，不能覆盖新的 deployed/current-schema 反证。
- **清理原则**：失效主链叙述、重复 audit、过程日记、live 产物一律归档或出库，不在 active 保留“假活跃”项。

## Navigation

- 事实入口：[current-state.md](current-state.md)
- Todo 索引：[unfinished-todo-index.md](unfinished-todo-index.md)
- Active overview：[../70-roadmap/active/overview.md](../70-roadmap/active/overview.md)
- Archive：[../90-archive/completed/overview.md](../90-archive/completed/overview.md)
