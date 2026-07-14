# WorkOrch Docs 未完成待办项索引

Status: active

Type: index

Last Updated: 2026-07-14

## Purpose

Runtime V2 代码收口与权威层文档刷新之后的 **真实未关项** 单点索引。  
已完成切片、V1 hardening、重复 audit **不再罗列**（见 `90-archive/completed/`）。

## 阅读导航

1. [A. 运维切换](#a-运维切换)
2. [B. 产品 / 能力未关](#b-产品--能力未关)
3. [C. 文档维护](#c-文档维护)
4. [D. 已清理 / 不再跟踪](#d-已清理--不再跟踪)

## A. 运维切换

| # | 主题 | 待办 | 文档 |
| --- | --- | --- | --- |
| A1 | 生产 apply | clean build + 生产目录维护窗口正式 apply RV2 切换结论 | `70-roadmap/active/deployed-workspace-progress-blocker-component-closure-plan.md` |

## B. 产品 / 能力未关

| # | 主题 | 文档 |
| --- | --- | --- |
| B1 | auto-governance ticket approval | `70-roadmap/active/auto-governance-ticket-approval-plan.md` |
| B2 | software-dev DoD repair | `70-roadmap/active/software-dev-dod-repair-closure-plan.md` |
| B3 | software requirement orchestration | `70-roadmap/active/software-requirement-development-orchestration-plan.md` |
| B4 | unified TUI / console | `70-roadmap/active/unified-tui-console-migration-plan.md` |
| B5 | file-patch owner migration | `70-roadmap/active/file-patch-side-effect-owner-migration-plan.md` |
| B6 | next MVP acceptance | `70-roadmap/active/next-mvp-acceptance.md` |
| B7 | run closure quality | `70-roadmap/active/run-closure-quality-improvement-plan.md` |
| B8 | llm context budget | `70-roadmap/active/llm-context-budget-todo.md` |
| B9 | authority live e2e design | `70-roadmap/active/authority-live-verification-e2e-closure-design.md` |
| B10 | module-block contract validation | `70-roadmap/active/module-block-contract-closure-validation-plan.md` |
| B11 | module-block initiator inventory | `70-roadmap/active/module-block-initiator-inventory-audit-2026-07-09.md` |
| B12 | operation map / JSON 交互合同残留收口 | `70-roadmap/active/operation-map-json-contract-audit-2026-07-14.md` |

## C. 文档维护

| # | 主题 | 说明 |
| --- | --- | --- |
| C1 | 权威层术语抽查 | 确认 StagePacket/NextAction 等不再被写成生产主链 |
| C2 | block readme 措辞 | 可选；实现 SoT 仍是 `kernel/orchestrator/readme.md` |
| C3 | archive 索引 | 保持 `90-archive/completed/overview.md` 可检索 |

## D. 已清理 / 不再跟踪

以下类别 **已从 active 移除**，不在本索引展开：

- 全部 `goal-dod-nextaction-s0`～`s11` 与 iterative plan（completed → archive）
- V1 `runtime-contract-retry-*` audit / batch / evidence index
- 被 Runtime V2 reducer 吸收的 `convergence-authority-*`、`runtime-*-closure-plan`、`*-nextaction-*` hardening
- 目录迁移时代的「正文细化 / 旧 root 引用清理」大批量编辑项（结构迁移已实质完成）

事实入口：[current-state.md](current-state.md)  
未关矩阵：[current-unclosed-matrix.md](current-unclosed-matrix.md)  
Active 索引：[../70-roadmap/active/overview.md](../70-roadmap/active/overview.md)
