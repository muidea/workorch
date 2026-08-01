# WorkOrch Current Unclosed Matrix

Status: active

Type: overview

Last Updated: 2026-07-31

## Purpose

汇总 Runtime 代码收口与 **2026-07-16 文档清理** 之后仍未关闭的事项。
已完成的过程计划、假活跃产品日记、dated live 记录与 verification artifacts **不再占用本矩阵**。

## Closed / Archived

Runtime production path is Runtime-only. Dated closure records do not appear in this matrix as actionable work
and cannot override a current persisted aggregate counterexample.

2026-07-30 audit also removed OPS-1: it referred to a Runtime maintenance command that has no code, typed
contract, or CLI entry point. It is superseded by the Runtime-only production boundary and remains archive history,
not an operational task.

## Residual Mandatory Work

### A. Runtime 单 owner lifecycle、统一 ingress 与 CWI reconciliation（R3，active）

最新 deployed LSP Bridge live 的 current-schema 反例是 activation compiler 矛盾：presentation policy 将四个
DoD CWI 合入同一 focus，而 validation binding 合同禁止复用一个 recipe/check 关闭多项 CWI，因此 `/run restart`
在创建 Run 前以 `coverage_binding_unavailable` 拒绝。此前 direct Run lifecycle、Run activate/continue source guard、
owner intent outcome forwarding 与 SM-4 Drive/termination producer trust 已完成代码收口，现仅作为回归基线。

`SMO-R2-C12` 仍是 raw-credit 防线：candidate raw CWI、assessment claim、`PassedChecks`、exit code、文件/Git
存在不得直接关闭 coverage。但它不替代 profile semantic proof，也不定义 resume/restart 的 reconciliation。R3 要求：

- `/run resume` 对 current non-terminal Run 的 sealed Definition/CWI/binding 全量 reconciliation；
- `/run restart` 以 workspace 当前 Goal/DoD/profile 创建 new Run，并在首轮 SM-2 做 full-scope reconciliation；
- validation/behavior CWI 必须编译为单 CWI focus 与独立 binding/proof；slice 只能表达展示和排序，不能复用 semantic receipt；
- profile/template 为每个可关闭 CWI 声明 owner-grounded target/selector、operation/effect、recipe/check/metric/artifact；
- activation 必须一次性报告全部缺 proof CWI，零 Run/LLM/operation/ticket/effect；
- application/backend/TUI/Web 必须呈现真实 activation outcome，并在无 active Run 的 preflight failure 中禁止推荐 resume；
- capability owner 只给 observation/receipt，只有 SM-2 能 retain/reopen coverage（File/Shell/Git/MCP 已增加不足证据回归）；
- 非 current schema 只产生 `fresh_activation_required`，不参与执行。

唯一实施队列见
[runtime-cwi-reconciliation-single-owner-closure-plan.md](../70-roadmap/active/runtime-cwi-reconciliation-single-owner-closure-plan.md)。

### B. 运行单元架构 / 生命周期

ARCH-1 已完成并归档：22 组件的 pkg-purity、concrete Result、原始诊断、outcome 唯一化、deployed restart
以及 workspace delete → late projection event 验证均已通过，详见
[completed matrix](../90-archive/completed/component-contract-and-regression-closure-matrix.md)。

### D. 可选验证

| ID | 项 | 说明 | 历史过程稿 |
| --- | --- | --- | --- |
| OPT-1 | file-patch deployed live | 可选执行一次 `patch apply + validation + DoD closeout` deployed live；不阻塞 Runtime 收口 | [file-patch-side-effect-owner-migration-plan.md](../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

### E. 延期候选（非 active）

| ID | 项 | 说明 |
| --- | --- | --- |
| CAND-LLM-BUDGET | LLM context budget 高级项 | provider failover、真实 tokenizer/原生 usage 精度、context-too-large 差异化等 → [candidates](../70-roadmap/candidates/next-stage-demand-candidates.md) |

## Judgment

- **代码主链**：Runtime only；禁止的 authority 不再是问题来源。R3 不增加第二主链或第五状态机，而是
  SM-2 的 owner-local reconciliation 前置 input。complete candidate document 必须先持久化，
  single evaluator 只返回完整 typed diagnostic；是否 correction/final 由 SM-3 internal event 决定。owner
  feasibility 是正交 typed fact。C12 重新打开的不是 recovery/owner/projection，而是 SM-2 evidence/coverage
  authority：只有 sealed binding + owner receipt 才能关闭 CWI，LLM/raw check 不得越权。
- **正向收敛基础**：bootstrap、CAS、sealed authorization、三种 TargetPolicy、request session、receipt、
  binding-derived CWI credit、single-owner lifecycle/outcome/SM-4 producer trust 与 terminal delivery均为 R3 回归基线；
  当前 active 缺口是可执行 CWI proof 的编译与 deployed semantic matrix。latest persisted aggregate/live 反例优先于过去的本地绿灯。
- **文档**：以 `00-core` + 模块 readme + `current-state` 为入口；当前实施顺序仅由 R3 active plan 定义，archive
  不得推导新实现约束。
- **Active roadmap**：R3 是唯一必需 Runtime 项；OPT-1 为非阻塞可选验证。ARCH-1、RUNTIME-SMO、SMO-R2 C12、
  旧 VPC、OPS-1、RCA-1、RPF 和 RPA-1 均归 archive，不能覆盖当前 Runtime 设计。
- **清理原则**：失效主链叙述、重复 audit、过程日记、live 产物一律归档或出库，不在 active 保留“假活跃”项。

## Navigation

- 事实入口：[current-state.md](current-state.md)
- Todo 索引：[unfinished-todo-index.md](unfinished-todo-index.md)
- Active overview：[../70-roadmap/active/overview.md](../70-roadmap/active/overview.md)
- Archive：[../90-archive/completed/overview.md](../90-archive/completed/overview.md)
