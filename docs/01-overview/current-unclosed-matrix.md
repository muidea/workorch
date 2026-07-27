# WorkOrch Current Unclosed Matrix

Status: active

Type: overview

Last Updated: 2026-07-27

## Purpose

汇总 Runtime V2 代码收口与 **2026-07-16 文档清理** 之后仍未关闭的事项。
已完成的过程计划、假活跃产品日记、dated live 记录与 verification artifacts **不再占用本矩阵**。

## Closed / Archived

Runtime V2 production path is V2-only and the V1 mainline is removed. All dated closure records, including
freshness and candidate-admission records, are archive-only regression baselines. They do not appear in this
matrix as separately actionable work and cannot override a newer persisted aggregate counterexample.

## Residual Open

### A. Runtime V2 正向收敛

| ID | 项 | 说明 | 当前方案 |
| --- | --- | --- | --- |
| RUNTIME-VPC | contract normalization and live convergence | R5 将 owner facts 先编译为唯一 sealed manifest，renderer/decoder/evaluator/owner feasibility 只消费该授权；complete rejection 必须是可修正一次或明确终态。N1～N8 代码/本地门禁已完成；剩余 real-provider 双 activation 与三态 deployed restart 依赖外部 fixture，不阻塞代码收口。旧 R4/P0～P6/A-4 仅是 archive 回归基线，不得定义当前顺序。 | [VPC candidate/receipt 方案](../70-roadmap/active/runtime-v2-validation-progress-closure-plan.md) |
| RUNTIME-SMO | single-owner state machine and lifecycle closure | owner reducer、事实 topic 与旧 Orchestrator facade 已完成第一阶段迁移，但 2026-07-27 实现复核确认 generic lifecycle 写入口、governance sealed identity、administrative terminal、delivery fence/CAS、startup recovery 与门禁仍未闭合；不得以 candidate 合同或 TUI/Web 分支绕过。 | [单 owner 收口方案（第 12 节为唯一执行队列）](../70-roadmap/active/runtime-v2-single-owner-state-machine-closure-plan.md) |

### B. 运行单元架构 / 生命周期

| ID | 项 | 说明 | 当前方案 |
| --- | --- | --- | --- |
| ARCH-1 | 22 组件 framework boundary 验证/归档 | Base Biz、Module-root EventHub 拓扑及本地 structural/race/file-backed 门禁已完成；组件职责/typed 合同/独立回归按矩阵逐项收口，待外部重启目标与三类 fixture manifest 执行 deployed SM-4 matrix | [组件矩阵](../70-roadmap/active/component-contract-and-regression-closure-matrix.md) |

### C. 运维 / 生产切换

| ID | 项 | 说明 | 历史过程稿 |
| --- | --- | --- | --- |
| OPS-1 | 生产目录 V1 retirement 正式 apply | 代码/live complete；等待 clean build + 真实生产目录维护窗口执行 `runtime-v1-retirement --apply` | [deployed-workspace-progress-blocker…](../90-archive/completed/deployed-workspace-progress-blocker-component-closure-plan.md) |

### D. 可选验证

| ID | 项 | 说明 | 历史过程稿 |
| --- | --- | --- | --- |
| OPT-1 | file-patch deployed live | 可选：`patch apply + validation + DoD closeout` 一次 deployed live | [file-patch-side-effect-owner-migration-plan.md](../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

### E. 延期候选（非 active）

| ID | 项 | 说明 |
| --- | --- | --- |
| CAND-LLM-BUDGET | LLM context budget 高级项 | provider failover、真实 tokenizer/原生 usage 精度、context-too-large 差异化等 → [candidates](../70-roadmap/candidates/next-stage-demand-candidates.md) |

### F. 文档维护

| ID | 项 | 说明 |
| --- | --- | --- |
| D-1 | 权威层 V1 术语残留抽查 | `rg` 确认 StagePacket/NextAction 等不再被写成生产主链 |
| D-2 | 个别 block readme 措辞 | 可选轻触；实现以 orchestrator readme 为准 |
| D-3 | archive 索引 | completed/records overview 保持可检索 |

## Judgment

- **代码主链**：Runtime V2 only；V1 authority 不再是问题来源。但 `RUNTIME-SMO` 所列 lifecycle owner 改造仍是
  未关闭代码项：已有 owner reducer 不代表单 owner 语义已闭合；以 SMO 第 12 节的 R1～R8 为准。
- **正向收敛基础**：既有 bootstrap、CAS、sealed dispatch、receipt/evidence 与 terminal delivery 继续是回归基线；N1～N8 代码收口已完成：sealed authorization、三种 TargetPolicy、admissibility 编译、独立预算、manifest-only renderer/decoder、correctable_once/rejected_final、统一 owner feasibility、五类结果 persistence/recovery、legacy 清零，以及 selector-only MCP inventory 不强制 targetRefs。本地 L1～L5、`make release-check`、`verify-runtime-v2-positive-convergence-file`、managed `verify-ci-mvp-file`、`verify-runtime-v2-p6-scripted` 已通过。剩余 real-provider 双 activation 与三态 deployed restart 需外部部署 fixture（与 ARCH-1 同类 observation residual）。
- **文档**：以 `00-core` + 模块 readme + `current-state` 为入口；不得再从已归档 plan 推导实现约束。
- **Active roadmap**：RUNTIME-VPC（candidate/receipt）与 RUNTIME-SMO（state owner/lifecycle）是唯一两个不重叠的
  Runtime V2 实施队列，另有 ARCH-1、OPS-1（+ 可选 OPT-1）；RCA-1、RPF 和 RPA-1 的历史 completion record
  均归 archive，但不得覆盖新的 deployed 反证。
- **清理原则**：失效主链叙述、重复 audit、过程日记、live 产物一律归档或出库，不在 active 保留“假活跃”项。

## Navigation

- 事实入口：[current-state.md](current-state.md)
- Todo 索引：[unfinished-todo-index.md](unfinished-todo-index.md)
- Active overview：[../70-roadmap/active/overview.md](../70-roadmap/active/overview.md)
- Archive：[../90-archive/completed/overview.md](../90-archive/completed/overview.md)
