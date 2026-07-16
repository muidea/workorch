# WorkOrch Current Unclosed Matrix

Status: active

Type: overview

Last Updated: 2026-07-16

## Purpose

汇总 Runtime V2 代码收口与 **2026-07-16 文档清理** 之后仍未关闭的事项。
已完成的过程计划、假活跃产品日记、dated live 记录与 verification artifacts **不再占用本矩阵**。

## Closed (recent)

| 类别 | 状态 |
| --- | --- |
| Runtime V2 生产主链 | 代码已收口 |
| V1 StagePacket / NextAction / DispatchNextStep / ExecutionContextPatch 主链 | 已删除 |
| 2026-07-14：约 146 份 completed/code-closed/V1-absorbed active plan | 已迁 `90-archive/completed/` |
| 2026-07-16：active 假活跃产品/审计/收口日记 + verification 过程记录 + artifacts | 已迁 archive 或移出仓库 |
| TUI workspace provider/model 选择与持久化 | `/model`、`show`、`clear`、workspace owner target、activation precedence、remote/embedded backend 已代码收口 |

## Residual Open

### A. 运维 / 生产切换

| ID | 项 | 说明 | 历史过程稿 |
| --- | --- | --- | --- |
| OPS-1 | 生产目录 V1 retirement 正式 apply | 代码/live complete；等待 clean build + 真实生产目录维护窗口执行 `runtime-v1-retirement --apply` | [deployed-workspace-progress-blocker…](../90-archive/completed/deployed-workspace-progress-blocker-component-closure-plan.md) |

### B. 可选验证

| ID | 项 | 说明 | 历史过程稿 |
| --- | --- | --- | --- |
| OPT-1 | file-patch deployed live | 可选：`patch apply + validation + DoD closeout` 一次 deployed live | [file-patch-side-effect-owner-migration-plan.md](../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

### C. 延期候选（非 active）

| ID | 项 | 说明 |
| --- | --- | --- |
| CAND-LLM-BUDGET | LLM context budget 高级项 | provider failover、真实 tokenizer/原生 usage 精度、context-too-large 差异化等 → [candidates](../70-roadmap/candidates/next-stage-demand-candidates.md) |

### D. 文档维护

| ID | 项 | 说明 |
| --- | --- | --- |
| D-1 | 权威层 V1 术语残留抽查 | `rg` 确认 StagePacket/NextAction 等不再被写成生产主链 |
| D-2 | 个别 block readme 措辞 | 可选轻触；实现以 orchestrator readme 为准 |
| D-3 | archive 索引 | completed/records overview 保持可检索 |

## Judgment

- **代码**：Runtime V2 only，可作为后续产品工作的稳定底座。
- **文档**：以 `00-core` + 模块 readme + `current-state` 为入口；不得再从已归档 plan 推导实现约束。
- **Active roadmap**：OPS-1（+ 可选 OPT-1）；其余历史切片在 archive / candidates 检索。
- **清理原则**：失效主链叙述、重复 audit、过程日记、live 产物一律归档或出库，不在 active 保留“假活跃”项。

## Navigation

- 事实入口：[current-state.md](current-state.md)
- Todo 索引：[unfinished-todo-index.md](unfinished-todo-index.md)
- Active overview：[../70-roadmap/active/overview.md](../70-roadmap/active/overview.md)
- Archive：[../90-archive/completed/overview.md](../90-archive/completed/overview.md)
