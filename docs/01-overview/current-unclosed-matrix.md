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

## Residual Mandatory Work

当前没有必须关闭的 Runtime V2 代码项。

### A. Runtime V2 正向收敛（已关闭）

`SMO-R2-C12` 已关闭。当前 schema 为每个 CWI 封存 immutable `CoverageEvidenceBinding`；candidate 只能选择
sealed `bindingRef`，SM-2 仅从 sealed binding、sealed operation 与 matching owner receipt 派生 credit。缺 binding
在 activation 以 `coverage_binding_unavailable` 拒绝；raw CWI、assessment claim、`PassedChecks`、exit code、文件或
Git 存在均不能直接关闭 coverage。T22–T30、replay/CAS、22-unit boundary 与 fresh dual-workspace live 已通过。

`blocks/run` 的旧 aggregate 仍不做 compatibility/migration；发现缺 binding 的旧 schema 一律
`fresh_activation_required`。Decision/Approval 不得成为缺证据的绕过路径。完成证据见
[SMO-R2 C12 completed record](../90-archive/completed/runtime-v2-single-owner-state-machine-r2-c12-closure-record.md)。

### B. 运行单元架构 / 生命周期

ARCH-1 已完成并归档：22 组件的 pkg-purity、concrete Result、原始诊断、outcome 唯一化、deployed restart
以及 workspace delete → late projection event 验证均已通过，详见
[completed matrix](../90-archive/completed/component-contract-and-regression-closure-matrix.md)。

### C. 可选验证

| ID | 项 | 说明 | 历史过程稿 |
| --- | --- | --- | --- |
| OPT-1 | file-patch deployed live | 可选执行一次 `patch apply + validation + DoD closeout` deployed live；不阻塞 Runtime V2 收口 | [file-patch-side-effect-owner-migration-plan.md](../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

### D. 延期候选（非 active）

| ID | 项 | 说明 |
| --- | --- | --- |
| CAND-LLM-BUDGET | LLM context budget 高级项 | provider failover、真实 tokenizer/原生 usage 精度、context-too-large 差异化等 → [candidates](../70-roadmap/candidates/next-stage-demand-candidates.md) |

## Judgment

- **代码主链**：Runtime V2 only；V1 authority 不再是问题来源。complete candidate document 必须先持久化，
  single evaluator 只返回完整 typed diagnostic；是否 correction/final 由 SM-3 internal event 决定。owner
  feasibility 是正交 typed fact。C12 重新打开的不是 recovery/owner/projection，而是 SM-2 evidence/coverage
  authority：只有 sealed binding + owner receipt 才能关闭 CWI，LLM/raw check 不得越权。
- **正向收敛基础**：bootstrap、CAS、sealed authorization、三种 TargetPolicy、request session、receipt、
  binding-derived CWI credit 与 terminal delivery均为回归基线；latest persisted aggregate反例优先于过去的本地绿灯。
- **文档**：以 `00-core` + 模块 readme + `current-state` 为入口；不得再从已归档 plan 推导实现约束。
- **Active roadmap**：没有必需 Runtime V2 代码项；OPT-1 为非阻塞可选验证。ARCH-1、RUNTIME-SMO、SMO-R2 C12、
  旧 VPC、OPS-1、RCA-1、RPF 和 RPA-1 均归 archive，不能覆盖当前 Runtime 设计。
- **清理原则**：失效主链叙述、重复 audit、过程日记、live 产物一律归档或出库，不在 active 保留“假活跃”项。

## Navigation

- 事实入口：[current-state.md](current-state.md)
- Todo 索引：[unfinished-todo-index.md](unfinished-todo-index.md)
- Active overview：[../70-roadmap/active/overview.md](../70-roadmap/active/overview.md)
- Archive：[../90-archive/completed/overview.md](../90-archive/completed/overview.md)
