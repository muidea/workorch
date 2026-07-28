# WorkOrch Current Unclosed Matrix

Status: active

Type: overview

Last Updated: 2026-07-28

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
| RUNTIME-SMO | single-owner Runtime V2 convergence closure | 唯一活动 Runtime 队列。complete document 在 EventHub adapter 丢失并被误判 provider protocol 的问题已按 C7-B～C7-E 完成 handoff、SM-3 evaluator、canonical target 和全量门禁收口；仅保留 C7-F 重新部署后的新 workspace fresh live、restart 与并发验收。 | [单 owner统一收口方案](../70-roadmap/active/runtime-v2-single-owner-state-machine-closure-plan.md) |

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

- **代码主链**：Runtime V2 only；V1 authority 不再是问题来源。complete candidate document 必须先持久化，
  再由 SM-3 single evaluator 裁决；以 SMO C7-B～C7-F 为准。
- **正向收敛基础**：既有 bootstrap、CAS、sealed authorization、三种 TargetPolicy、request session、receipt与
  terminal delivery继续是回归基线；latest persisted aggregate反例优先于过去的本地绿灯和阶段完成声明。
- **文档**：以 `00-core` + 模块 readme + `current-state` 为入口；不得再从已归档 plan 推导实现约束。
- **Active roadmap**：RUNTIME-SMO 是唯一 Runtime V2 实施队列，另有 ARCH-1、OPS-1（+ 可选 OPT-1）；旧VPC、
  RCA-1、RPF和RPA-1均归archive，不能覆盖新的deployed反证。
- **清理原则**：失效主链叙述、重复 audit、过程日记、live 产物一律归档或出库，不在 active 保留“假活跃”项。

## Navigation

- 事实入口：[current-state.md](current-state.md)
- Todo 索引：[unfinished-todo-index.md](unfinished-todo-index.md)
- Active overview：[../70-roadmap/active/overview.md](../70-roadmap/active/overview.md)
- Archive：[../90-archive/completed/overview.md](../90-archive/completed/overview.md)
