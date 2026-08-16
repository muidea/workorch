# WorkOrch Completed Archive

Status: active

Type: archive

Last Verified: 2026-08-16

## Purpose

承接已完成的阶段计划、专项收口和历史实现记录。**不再作为当前 active 设计入口。**

## Archive Handling Rule

- 正文保留稳定历史价值 → 留在 `completed/`
- 仅过渡或被新结构完全替代 → 可再迁 `superseded/`
- 当前事实 / API / 验收 **不得** 回写本目录
- 实现约束以 `00-core` + 模块 readme + `workorchruntimecontract` 为准

## 2026-08-16 Single-Owner Documentation Refresh

| 文件 | 一行结论 |
| --- | --- |
| [runtime-single-owner-code-closure-runbook.md](runtime-single-owner-code-closure-runbook.md) | Orchestrator/Workorchd 单 Owner 代码收口的历史执行记录 |
| [runtime-cwi-reconciliation-single-owner-closure-plan.md](runtime-cwi-reconciliation-single-owner-closure-plan.md) | CWI reconciliation 单 Owner 收口的完成计划 |
| [runtime-contract-drift-closure-plan.md](runtime-contract-drift-closure-plan.md) | active defaults/verifiers/API/container 与当前 Runtime-only 合同完成对齐并加防回流门禁 |

## 2026-08-04 Component Event Contract Closure

| 文件 | 一行结论 |
| --- | --- |
| [component-event-contract-closure-plan.md](component-event-contract-closure-plan.md) | 22/22 组件 EventHub Topic/Data/Result owner-local concrete 合同完成；全仓 test/race、console、managed deployed 与 fresh 双 workspace/C7 验证通过 |

## 2026-07-16–18 Docs Cleanup Ingest

本轮从 `70-roadmap/active/`、`docs/` 根与 `60-verification/` 迁入过程稿（**不约束新实现**）：

| 文件 | 一行结论 |
| --- | --- |
| `ai-proxy-primary-llm-provider-cutover-design-2026-07-15.md` | code-closed + live 通过 |
| `operation-map-json-contract-audit-2026-07-14.md` | map/JSON 交互合同收口 completed |
| `module-block-initiator-inventory-audit-2026-07-09.md` | 22 组件职责审计 completed |
| `runtime-typed-event-contract-closure-summary-2026-07-14.md` | V2 强类型事件收口总结 |
| `auto-governance-ticket-approval-plan.md` | AG 全 verified |
| `software-dev-dod-repair-closure-plan.md` | SDR 全 completed |
| `software-requirement-development-orchestration-plan.md` | SRD 全 verified/implemented |
| `next-mvp-acceptance.md` | N1–N4 completed |
| `unified-tui-console-migration-plan.md` | 入口收敛完成；仅勿回退约束 |
| `run-closure-quality-improvement-plan.md` | 过程日记（稳定定义已迁正式结构） |
| `authority-live-verification-e2e-closure-design.md` | 历史 live design；P0–P2 已 code-closed |
| `module-block-contract-closure-validation-plan.md` | 收口方法论，非开放 backlog |
| `file-patch-side-effect-owner-migration-plan.md` | 代码迁移完成；可选 OPT-1 live |
| `llm-context-budget-todo.md` | 基础已收口；高级项进 candidates |
| `docs-domain-module-flow-reorganization-plan.md` | 目录重组历史计划 |
| `tui-data-stability-command-optimization-plan.md` | TUI 数据稳定化过程收口 |
| `tui-workspace-model-selection-closure-plan.md` | `/model` 完成 providerId/modelId 选择、workspace 持久化、Runtime 新 Run 优先级与 deployed e2e 收口 |
| `component-contract-and-regression-closure-matrix.md` | ARCH-1 completed：22 单元 boundary/lifecycle 合同、workspace delete → late projection event managed deployed 验证与 positive-convergence regression 通过 |

## 2026-07-14 Runtime Docs Refresh Ingest

本轮从 `70-roadmap/active/` 迁入约 **146** 份文档，主要类别：

| 类别 | 说明 |
| --- | --- |
| goal-dod-nextaction | `goal-dod-nextaction-s0`～`s11` 全系列 + iterative delivery plan |
| V1 contract-retry | batch1–7 audit、evidence index、full-chain / convergence 计划 |
| convergence-authority | 旧 authority/single-source/focus-drift 等收口（已被 V2 reducer 吸收） |
| runtime-*-closure | 各类 V1 nextaction / ledger / validation / statechange hardening |
| 其他 completed/code-closed | Status 已关闭的收口与 audit |

完整文件列表以本目录 `ls` 为准。检索时按文件名主题过滤即可。

## Early Completed Themes（既有）

- MVP closure / P1 delivery
- result / artifact / event stream
- runtime expansion / advanced scheduling
- llm-driven workspace execution
- framework alignment / recovery governance

## Representative Early Documents

- [mvp-closure-record.md](mvp-closure-record.md)
- [p1-development-record.md](p1-development-record.md)
- [runtime-expansion-plan.md](runtime-expansion-plan.md)
- [advanced-runtime-scheduling-plan.md](advanced-runtime-scheduling-plan.md)
- [result-event-development-plan.md](result-event-development-plan.md)
- [software-dev-code-agent-closure-plan.md](software-dev-code-agent-closure-plan.md)
- [tui-improvements-absorption-plan.md](tui-improvements-absorption-plan.md)
- [runtime-recovery-governance-closure-design.md](runtime-recovery-governance-closure-design.md)
- [llm-driven-development-plan.md](llm-driven-development-plan.md)
- [core-runtime-closure-history.md](core-runtime-closure-history.md)

## Cross-Archive Navigation

- 当前事实：[../../01-overview/current-state.md](../../01-overview/current-state.md)
- 当前 roadmap：[../../70-roadmap/active/overview.md](../../70-roadmap/active/overview.md)
- 历史 records：[../records/overview.md](../records/overview.md)
- superseded：[../superseded/overview.md](../superseded/overview.md)
