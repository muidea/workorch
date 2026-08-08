# WorkOrch Documentation Map

Status: active

Last Updated: 2026-08-08

## Purpose

本文件是当前 `docs/` 的正式阅读导航入口，按“核心合约 → 当前事实 → 产品设计 → 对象 → 运行链路 → 模块实现 → 特化能力 → 产品接口 → 验证 → roadmap/archive”顺序阅读。

**实现细节优先以模块 `readme.md` + `internal/pkg/workorchruntimecontract` 为准；中立规则以 `00-core` 为准。**

## Recommended Reading Order

1. [00-core/runtime-stage-contract.md](00-core/runtime-stage-contract.md)（中立 stage 语义 + Runtime 映射）
2. [01-overview/current-state.md](01-overview/current-state.md)（生产主链事实：V2 only）
3. `internal/modules/kernel/orchestrator/readme.md`（实现 SoT）
4. [20-runtime-flow/overview.md](20-runtime-flow/overview.md)
5. [30-module-architecture/overview.md](30-module-architecture/overview.md)
6. [05-product/overview.md](05-product/overview.md)
7. [10-domain-model/overview.md](10-domain-model/overview.md)
8. [40-profiles-capabilities/overview.md](40-profiles-capabilities/overview.md)
9. [50-product-interface/overview.md](50-product-interface/overview.md)
10. [60-verification/overview.md](60-verification/overview.md)
11. [70-roadmap/active/overview.md](70-roadmap/active/overview.md)（仅未关项）
12. [90-archive/completed/overview.md](90-archive/completed/overview.md)（历史）

## Read By Task

- Runtime 核心规则：
  [runtime-stage-contract.md](00-core/runtime-stage-contract.md)
- 当前 Runtime CWI reconciliation、`/run resume`/`/run restart` scope 与 single-owner lifecycle 收口：
  [runtime-cwi-reconciliation-single-owner-closure-plan.md](70-roadmap/active/runtime-cwi-reconciliation-single-owner-closure-plan.md)
- Workspace reasoning effort、ai-proxy capability、Run seal、Web Start/Restart 与 TUI `/model` 收口：
  [workspace-reasoning-effort-single-owner-closure-plan.md](70-roadmap/active/workspace-reasoning-effort-single-owner-closure-plan.md)
- 当前仓库事实 / 未关项：
  [current-state.md](01-overview/current-state.md),
  [current-unclosed-matrix.md](01-overview/current-unclosed-matrix.md),
  [unfinished-todo-index.md](01-overview/unfinished-todo-index.md)
- 上下文组织 / request cache：
  [runtime-request-cache-note.md](00-core/runtime-request-cache-note.md),
  [context-assembly-flow.md](20-runtime-flow/context-assembly-flow.md),
  [llm-provider-assembly.md](30-module-architecture/llm-provider-assembly.md)
- Goal / DoD / Result / Artifact：
  [workspace-goal-dod.md](10-domain-model/workspace-goal-dod.md),
  [run-step-result-artifact.md](10-domain-model/run-step-result-artifact.md),
  [verification-acceptance-flow.md](20-runtime-flow/verification-acceptance-flow.md)
- 修复 / 重规划 / 治理：
  [repair-replan-flow.md](20-runtime-flow/repair-replan-flow.md),
  [governance-decision-approval.md](10-domain-model/governance-decision-approval.md)
- 模块职责与代码落点：
  [architecture-overview.md](30-module-architecture/architecture-overview.md),
  [kernel-orchestrator-governance.md](30-module-architecture/kernel-orchestrator-governance.md),
  [repository-structure.md](30-module-architecture/repository-structure.md)
- software-dev / workspace / MCP 特化：
  [40-profiles-capabilities/overview.md](40-profiles-capabilities/overview.md)
- HTTP / 部署 / TUI：
  [50-product-interface/overview.md](50-product-interface/overview.md),
  [service-api.md](50-product-interface/service-api.md),
  [deployment.md](50-product-interface/deployment.md),
  [tui/requirements.md](50-product-interface/tui/requirements.md)
- 验收：
  [60-verification/overview.md](60-verification/overview.md)

## Read By Role

- 架构 / 框架：`00-core`、`current-state`、`20-runtime-flow`、`30-module-architecture`、orchestrator readme
- 业务建模：`05-product`、`10-domain-model`、`40-profiles-capabilities`
- 产品 / API：`50-product-interface`
- 验收 / 运维：`60-verification`、`deployment`
- 规划 / 历史：`70-roadmap/active`（未关）、`90-archive`（历史）

## Directory Guide

| 目录 | 内容 |
| --- | --- |
| `00-core/` | 唯一核心 runtime 合约 + request/cache note |
| `01-overview/` | 当前事实、未关矩阵、todo 索引 |
| `05-product/` | PRD、产品架构、业务流程合同 |
| `10-domain-model/` | 业务对象与不变量 |
| `20-runtime-flow/` | activation / drive / evidence / repair 主链 |
| `30-module-architecture/` | application / kernel / blocks / storage 边界 |
| `40-profiles-capabilities/` | profile / capability 特化 |
| `50-product-interface/` | API / 部署 / TUI / Web |
| `60-verification/` | acceptance / audit / verifier |
| `70-roadmap/active/` | **仅**真实未关计划 |
| `90-archive/` | completed / superseded / records |

## Core Entry

- 唯一核心设计合约：[runtime-stage-contract.md](00-core/runtime-stage-contract.md)
- Request/cache 从属：[runtime-request-cache-note.md](00-core/runtime-request-cache-note.md)
- 实现 SoT：`internal/modules/kernel/orchestrator/readme.md`

若旧文档与核心合约冲突，以 `00-core/runtime-stage-contract.md` 为准。若设计文档与模块 readme/代码冲突，以代码与模块 readme 为准并回写设计文档。

## Current Documentation Status（2026-07-29）

- **生产代码路径 = Runtime only**（Start/Resume → `DriveRuntime`）。
- 权威层已按 V2 刷新：`00-core` 双层映射、`01-overview` 事实入口、`20-runtime-flow` / `30-module-architecture` 主链。
- 2026-07-14：约 146 份 completed / V1-absorbed plan 已迁 `90-archive/completed/`。
- **2026-07-16 文档清理**：`active/` 假活跃产品/审计/收口日记全部归档；dated live verification 迁 `records/`；`60-verification/artifacts/**` 出库。
- **RUNTIME-SMO-R2 已完成验证、待随本次代码提交归档**：它关闭了 current-schema recovery 的 Run inventory 越权
  分类、semantic conflict→CAS retry、retry lease、projection 伪运行状态与 assessment observation 越权选择 repair
  action。历史 SMO/C7/C8 与 VPC 仅作回归反例来源；不得新增第二套状态迁移或以 TUI/Web 兼容分支绕过 `00-core`
  与 Orchestrator implementation SoT。ARCH-1 的 workspace delete → late projection event deployed 验证仍已完成并归档；
  OPS-1 所设想的 V1 maintenance apply 不存在且已被 V2-only production boundary 取代。
- **已废止作为生产主链叙述**：StagePacket、NextAction、graph dispatch checkpoint、ExecutionContextPatch、自动 replan successor。
- 根目录正式导航：本文。历史目录重组计划见 `90-archive/completed/docs-domain-module-flow-reorganization-plan.md`。

详见：[01-overview/current-state.md](01-overview/current-state.md)、[70-roadmap/active/overview.md](70-roadmap/active/overview.md)。
