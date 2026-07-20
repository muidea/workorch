# WorkOrch Active Roadmap

Status: active

Type: roadmap

Last Updated: 2026-07-20

## Purpose

仅索引 **当前仍未关闭** 的工作项。已完成 / code-closed / 过程日记 / 被 Runtime V2 吸收的计划一律在 [90-archive/completed](../../90-archive/completed/overview.md) 检索，**不得**再从 archive 推导新实现约束。

## Runtime Code Status

| 主题 | 状态 |
| --- | --- |
| Runtime V2 生产主链 | **代码已收口**（Start/Resume → `DriveRuntimeV2` only） |
| Runtime V2 正向收敛基础闭环 | **completed**；RVC-R17～R26 已 code-landed，RVC-R16 bounded terminal 与 canonical canary liveness 已通过；完成记录迁入 archive |
| Runtime V2 production admissibility + positive acceptance | **completed**；RPA-1 首次 gate 与 RPF-1～5 durable-effect freshness 均已经 fresh deployed bounded acceptance 复核 |
| Runtime V2 prerequisite freshness | **completed**；completion record 已归档，覆盖 observation/change/stale、refresh budget、restart/crash 与跨组件回归 |
| Runtime V2 candidate admission transaction | **remediation active**；2026-07-20 live `run-009` 证明 partial replay rejection 可绕过 attempt failure 并形成 terminal+pending aggregate，按 typed admission + atomic closure 收口 |
| 22-component framework boundary | **remediation active**；共享 Base Biz、全量本地 structural/race/file-backed 门禁已完成，待外部 deployed restart-fixture matrix |
| V1 StagePacket / NextAction / graph dispatch 主链 | **已删除** |
| 设计结论上提 | 见 [00-core/runtime-stage-contract.md](../../00-core/runtime-stage-contract.md)、[current-state.md](../../01-overview/current-state.md) |

## Residual Open（仅此）

| ID | 项 | 说明 | 方案 / 历史过程稿 |
| --- | --- | --- | --- |
| RCA-1 | Runtime V2 candidate admission transaction closure | 将 active rejected shape / replay admission authority 从 contract package 收回 Orchestrator，删除错误文本分类，原子闭合 attempt 0/1，并禁止 terminal aggregate 残留 pending attempt | [细化收口方案](runtime-v2-candidate-admission-transaction-closure.md) |
| ARCH-1 | 22 组件 framework boundary / lifecycle 验证 | Base Biz / Module-root EventHub 拓扑及本地验证已收口；按 22 组件合同矩阵逐项保持独立回归，待有重启后的外部目标与三类 fixture manifest 后执行 deployed SM-4 matrix | [主方案](go-multi-module-22-component-boundary-remediation.md) / [组件矩阵](component-contract-and-regression-closure-matrix.md) |
| OPS-1 | 生产目录 V1 retirement 正式 apply | 代码/live 已 complete；等待 clean build + 真实生产目录维护窗口执行 `runtime-v1-retirement --apply` | [deployed-workspace-progress-blocker-component-closure-plan.md](../../90-archive/completed/deployed-workspace-progress-blocker-component-closure-plan.md) |
| OPT-1 | file-patch deployed live（可选） | 代码迁移已完成；可选补一次 `patch apply + validation + DoD closeout` 的 deployed live | [file-patch-side-effect-owner-migration-plan.md](../../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

延期产品候选（**不占 active**）：LLM context budget 高级项（failover / tokenizer / 原生 usage 精度 / context-too-large 差异化）→ [candidates/next-stage-demand-candidates.md](../candidates/next-stage-demand-candidates.md)。

统一清单：[current-unclosed-matrix.md](../../01-overview/current-unclosed-matrix.md)、[unfinished-todo-index.md](../../01-overview/unfinished-todo-index.md)。

## 2026-07-16–18 Docs Cleanup Ingest

本轮从 `active/` 与验证过程目录迁出 / 移除：

### → `90-archive/completed/`

- `runtime-v2-positive-convergence-closure-plan.md`（历史 RVC-01～18 completion record；其 deployed 结论已由当前 remediation 复核作废）
- `runtime-v2-production-admissibility-closure-2026-07-18.md`（RPA-1 当时切片完成；首次 inventory/selector/feasibility 与 bounded/positive acceptance 证据保留，durable-effect freshness 已由 RPF 重新打开）
- `ai-proxy-primary-llm-provider-cutover-design-2026-07-15.md`（code-closed + live 通过）
- `operation-map-json-contract-audit-2026-07-14.md`
- `module-block-initiator-inventory-audit-2026-07-09.md`
- `runtime-v2-typed-event-contract-closure-summary-2026-07-14.md`
- `auto-governance-ticket-approval-plan.md`（AG 全 verified）
- `software-dev-dod-repair-closure-plan.md`（SDR 全 completed）
- `software-requirement-development-orchestration-plan.md`（SRD 全 verified/implemented）
- `next-mvp-acceptance.md`（N1–N4 completed）
- `unified-tui-console-migration-plan.md`（仅勿回退约束）
- `run-closure-quality-improvement-plan.md`（过程日记）
- `authority-live-verification-e2e-closure-design.md`（V1-era，P0–P2 已 code-closed）
- `module-block-contract-closure-validation-plan.md`（收口方法论）
- `deployed-workspace-progress-blocker-component-closure-plan.md`（长日记；residual 仅 OPS-1）
- `file-patch-side-effect-owner-migration-plan.md`（长日记；residual 仅 OPT-1）
- `llm-context-budget-todo.md`（基础已收口；高级项进 candidates）
- `docs-domain-module-flow-reorganization-plan.md`（目录重组历史计划）
- `tui-data-stability-command-optimization-plan.md`（TUI 过程收口）

### → `90-archive/records/`

- `ai-proxy-primary-llm-provider-cutover-live-verification-2026-07-15.md`
- `tui-workspace-runtime-refresh-history-closure-verification-2026-07-15.md`
- `authority-live-e2e-closure-verification.md`
- `tui-display-closure-tracking.md`

### 从仓库移除

- `docs/60-verification/artifacts/**`（live 运行产物；结论保留在 records markdown；git 历史可找回）

## Rules

1. 新增 active 项必须对应真实未完成工作；已归档 plan 只有在代码/证据复核给出明确反证并同步作废旧 completion record 时才允许重新打开。
2. 实现约束以 `00-core` + 模块 readme + `workorchruntimecontract` 为准。
3. 某项 code-closed 且无运维/产品尾巴时，应 `git mv` 至 `90-archive/completed/` 并更新本 overview。
4. active **不**存放 100KB+ 过程日记；过程细节只在 archive 检索。
5. 清理失效内容：V1 主链术语、重复 audit、假活跃 Status 不得继续占用 active。
