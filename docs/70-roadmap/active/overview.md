# WorkOrch Active Roadmap

Status: active

Type: roadmap

Last Updated: 2026-07-15

## Purpose

仅索引 **当前仍未关闭** 的工作项。已完成 / code-closed / 被 Runtime V2 吸收的 V1 hardening 与 `goal-dod-nextaction-s*` 切片已迁入 [90-archive/completed](../../90-archive/completed/overview.md)，不再在此列出。

## Runtime Code Status

| 主题 | 状态 |
| --- | --- |
| Runtime V2 生产主链 | **代码已收口**（Start/Resume → `DriveRuntimeV2` only） |
| V1 StagePacket / NextAction / graph dispatch 主链 | **已删除** |
| 设计结论上提 | 见 [00-core/runtime-stage-contract.md](../../00-core/runtime-stage-contract.md)、[current-state.md](../../01-overview/current-state.md) |

## Active Items

### 运维切换（代码已关，生产 apply 未关）

| 文档 | 说明 |
| --- | --- |
| [deployed-workspace-progress-blocker-component-closure-plan.md](deployed-workspace-progress-blocker-component-closure-plan.md) | RV2-01~08 code/live complete；等待 clean build + 生产目录维护窗口正式 apply。稳定结论已上提 core/flow/module，本文件不再充当实现 SoT。 |

### 产品 / 能力

| 文档 | 说明 |
| --- | --- |
| [auto-governance-ticket-approval-plan.md](auto-governance-ticket-approval-plan.md) | 自动治理与 ticket approval 产品化 |
| [software-dev-dod-repair-closure-plan.md](software-dev-dod-repair-closure-plan.md) | software-dev profile DoD repair |
| [software-requirement-development-orchestration-plan.md](software-requirement-development-orchestration-plan.md) | 需求开发编排产品能力 |
| [unified-tui-console-migration-plan.md](unified-tui-console-migration-plan.md) | TUI / Web console 统一 |
| [file-patch-side-effect-owner-migration-plan.md](file-patch-side-effect-owner-migration-plan.md) | file-patch 副作用 owner 迁移 |
| [next-mvp-acceptance.md](next-mvp-acceptance.md) | 下一阶段 MVP 验收 |
| [run-closure-quality-improvement-plan.md](run-closure-quality-improvement-plan.md) | Run 收口质量 |
| [llm-context-budget-todo.md](llm-context-budget-todo.md) | LLM context budget 产品/实现跟进 |
| [ai-proxy-primary-llm-provider-cutover-design-2026-07-15.md](ai-proxy-primary-llm-provider-cutover-design-2026-07-15.md) | ai-proxy cutover：RR01–RR06 代码与门禁收口（make test/fmt/vet/build/boundary 绿）；仅待 live 后改 code-closed |
| [authority-live-verification-e2e-closure-design.md](authority-live-verification-e2e-closure-design.md) | authority live e2e 验证设计 |
| [module-block-contract-closure-validation-plan.md](module-block-contract-closure-validation-plan.md) | module/block 合同校验 |
| [module-block-initiator-inventory-audit-2026-07-09.md](module-block-initiator-inventory-audit-2026-07-09.md) | initiator 清单审计 |
| [operation-map-json-contract-audit-2026-07-14.md](operation-map-json-contract-audit-2026-07-14.md) | operation map / JSON 交互合同残留审计与收口 backlog |

## Archived From Active (this refresh)

本轮从 `active/` 迁出约 **146** 份文档，包括：

- `tui-workspace-runtime-refresh-history-closure-plan-2026-07-15.md`（FR01–FR07 + TC10 live completed, 2026-07-15）

- 全部 `goal-dod-nextaction-s*` 与 iterative delivery plan（completed）
- V1 contract-retry batch audit / evidence index
- 已被 Runtime V2 reducer 吸收的 convergence-authority / runtime-*-closure / nextaction hardening 计划
- `Status: completed|code-closed|closed|implemented` 的其余收口文

检索入口：[90-archive/completed/overview.md](../../90-archive/completed/overview.md)

## Rules

1. 新增 active 项必须对应真实未完成工作；已归档 plan 只有在代码/证据复核给出明确反证并同步作废旧 completion record 时才允许重新打开。
2. 实现约束以 `00-core` + 模块 readme + `workorchruntimecontract` 为准。
3. 某项 code-closed 且无运维/产品尾巴时，应 `git mv` 至 `90-archive/completed/` 并更新本 overview。
4. 清理失效内容：V1 主链术语、重复 audit、假活跃 Status 不得继续占用 active。
