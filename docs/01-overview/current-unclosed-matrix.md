# WorkOrch Current Unclosed Matrix

Status: active

Type: overview

Last Updated: 2026-07-14

## Purpose

汇总 Runtime V2 代码收口之后仍未关闭的事项。已完成的 V1 hardening / goal-dod-nextaction 切片已迁入 `90-archive/completed/`，不再占用本矩阵。失效的 V1 主链叙述与重复 audit 已从 active 清理。

## Closed (this refresh)

| 类别 | 状态 |
| --- | --- |
| Runtime V2 生产主链（drive / reducer / sealed dispatch / governance wait / terminal projection） | 代码已收口 |
| V1 StagePacket / NextAction / DispatchNextStep / ExecutionContextPatch 主链 | 已删除 |
| 约 146 份 completed/code-closed/V1-absorbed active plan | 已迁 `90-archive/completed/` |
| 权威层文档与 V2 对齐（本轮 docs refresh） | 以 `00-core` + flow + module + current-state 为准 |

## Residual Open

### A. 运维 / 生产切换

| ID | 项 | 说明 | 文档 |
| --- | --- | --- | --- |
| OPS-1 | deployed workspace progress blocker 生产 apply | RV2-01~08 code/live complete；等待 clean build + 真实生产目录维护窗口 | [deployed-workspace-progress-blocker-component-closure-plan.md](../70-roadmap/active/deployed-workspace-progress-blocker-component-closure-plan.md) |

### B. 产品 / 能力未关

| ID | 项 | 文档 |
| --- | --- | --- |
| P-1 | auto-governance ticket approval | [auto-governance-ticket-approval-plan.md](../70-roadmap/active/auto-governance-ticket-approval-plan.md) |
| P-2 | software-dev DoD repair | [software-dev-dod-repair-closure-plan.md](../70-roadmap/active/software-dev-dod-repair-closure-plan.md) |
| P-3 | software requirement development orchestration | [software-requirement-development-orchestration-plan.md](../70-roadmap/active/software-requirement-development-orchestration-plan.md) |
| P-4 | unified TUI / console migration | [unified-tui-console-migration-plan.md](../70-roadmap/active/unified-tui-console-migration-plan.md) |
| P-5 | file-patch side-effect owner migration | [file-patch-side-effect-owner-migration-plan.md](../70-roadmap/active/file-patch-side-effect-owner-migration-plan.md) |
| P-6 | next MVP acceptance | [next-mvp-acceptance.md](../70-roadmap/active/next-mvp-acceptance.md) |
| P-7 | run closure quality | [run-closure-quality-improvement-plan.md](../70-roadmap/active/run-closure-quality-improvement-plan.md) |
| P-8 | llm context budget | [llm-context-budget-todo.md](../70-roadmap/active/llm-context-budget-todo.md) |
| P-9 | authority live e2e verification design | [authority-live-verification-e2e-closure-design.md](../70-roadmap/active/authority-live-verification-e2e-closure-design.md) |
| P-10 | module-block contract validation | [module-block-contract-closure-validation-plan.md](../70-roadmap/active/module-block-contract-closure-validation-plan.md) |
| P-11 | module-block initiator inventory | [module-block-initiator-inventory-audit-2026-07-09.md](../70-roadmap/active/module-block-initiator-inventory-audit-2026-07-09.md) |

### C. 文档 / 编辑性

| ID | 项 | 说明 |
| --- | --- | --- |
| D-1 | 权威层 V1 术语残留 | 刷新后应用 `rg` 抽查 StagePacket/NextAction 等是否仍被写成生产主链 |
| D-2 | 个别 block readme stage 措辞 | 可选轻触；实现以 orchestrator readme 为准 |
| D-3 | archive 索引维护 | completed overview 保持可检索，不重写历史正文 |

## Judgment

- **代码**：Runtime V2 only，可作为后续产品工作的稳定底座。
- **文档**：以 `00-core` + 模块 readme + `current-state` 为入口；不得再从已归档 plan 推导实现约束。
- **Active roadmap**：仅保留上表真实未关项；其余历史切片在 archive 检索。
- **清理原则**：失效主链叙述、重复 V1 audit、已被 V2 reducer 吸收的 hardening plan 一律归档或删除入口引用，不在 active 保留“假活跃”项。
