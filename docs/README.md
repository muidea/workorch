# WorkOrch Documentation Map

## Purpose

本文件是当前 `docs/` 的正式阅读导航入口，用于帮助按“核心合约 -> 对象 -> 运行链路 -> 模块实现 -> 特化能力 -> 产品接口 -> 验证 -> roadmap/archive”顺序阅读。

## Recommended Reading Order

1. [00-core/runtime-stage-contract.md](00-core/runtime-stage-contract.md)
2. [01-overview/current-state.md](01-overview/current-state.md)
3. [10-domain-model/overview.md](10-domain-model/overview.md)
4. [20-runtime-flow/overview.md](20-runtime-flow/overview.md)
5. [30-module-architecture/overview.md](30-module-architecture/overview.md)
6. [40-profiles-capabilities/overview.md](40-profiles-capabilities/overview.md)
7. [50-product-interface/overview.md](50-product-interface/overview.md)
8. [60-verification/overview.md](60-verification/overview.md)
9. [70-roadmap/active/overview.md](70-roadmap/active/overview.md)
10. [90-archive/completed/overview.md](90-archive/completed/overview.md)

## Read By Task

按常见问题可优先阅读：

- Runtime 核心规则：
  [runtime-stage-contract.md](00-core/runtime-stage-contract.md)
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
  [repository-structure.md](30-module-architecture/repository-structure.md)
- software-dev / workspace / MCP 特化：
  [40-profiles-capabilities/overview.md](40-profiles-capabilities/overview.md)
- HTTP / 部署 / TUI：
  [50-product-interface/overview.md](50-product-interface/overview.md),
  [service-api.md](50-product-interface/service-api.md),
  [50-product-interface/deployment.md](50-product-interface/50-product-interface/deployment.md),
  [50-product-interface/tui/requirements.md](50-product-interface/tui/requirements.md)
- 验收 / 审计 / 历史记录：
  [60-verification/overview.md](60-verification/overview.md),
  [90-archive/records/overview.md](90-archive/records/overview.md)

## Read By Role

按典型使用角色可优先阅读：

- 架构/框架设计
  - [00-core/runtime-stage-contract.md](00-core/runtime-stage-contract.md)
  - [20-runtime-flow/overview.md](20-runtime-flow/overview.md)
  - [30-module-architecture/overview.md](30-module-architecture/overview.md)
- 业务功能/对象建模
  - [10-domain-model/overview.md](10-domain-model/overview.md)
  - [40-profiles-capabilities/overview.md](40-profiles-capabilities/overview.md)
- 产品/API/交互设计
  - [50-product-interface/overview.md](50-product-interface/overview.md)
  - [50-product-interface/service-api.md](50-product-interface/service-api.md)
- 验收/验证/运维
  - [60-verification/overview.md](60-verification/overview.md)
  - [50-product-interface/50-product-interface/deployment.md](50-product-interface/50-product-interface/deployment.md)
- 规划/历史追溯
  - [70-roadmap/active/overview.md](70-roadmap/active/overview.md)
  - [90-archive/records/overview.md](90-archive/records/overview.md)
  - [90-archive/completed/overview.md](90-archive/completed/overview.md)

## Quick Jumps

- 当前事实入口
  - [01-overview/current-state.md](01-overview/current-state.md)
- 当前剩余尾项
  - [01-overview/current-unclosed-matrix.md](01-overview/current-unclosed-matrix.md)
- 内容不完整 / 衔接缺口
  - [01-overview/incomplete-and-connection-gap-tracker.md](01-overview/incomplete-and-connection-gap-tracker.md)
- 信息模型一致性审计
  - [01-overview/information-model-alignment-audit.md](01-overview/information-model-alignment-audit.md)
- 当前活跃路线图
  - [70-roadmap/active/overview.md](70-roadmap/active/overview.md)
- 历史记录与完成项
  - [90-archive/records/overview.md](90-archive/records/overview.md)
  - [90-archive/completed/overview.md](90-archive/completed/overview.md)

## Directory Guide

- `00-core/`
  - 唯一核心 runtime 合约与其从属 request/cache note
- `01-overview/`
  - 当前仓库事实、迁移状态、缺口追踪
- `10-domain-model/`
  - 业务对象、关系、边界和不变量
- `20-runtime-flow/`
  - activation / execution / verification / repair / replan / context assembly 主链
- `30-module-architecture/`
  - application / kernel / blocks / storage / provider assembly 的实现边界
- `40-profiles-capabilities/`
  - workspace / software-dev / mcp / runtime capability 等特化设计
- `50-product-interface/`
  - deployment、service API、TUI、Web Console 产品面
- `60-verification/`
  - acceptance、audit、verifier、loadtest
- `70-roadmap/`
  - 当前活跃计划与候选项
- `90-archive/`
  - completed / superseded / records

## Core Entry

WorkOrch runtime 的唯一核心设计合约是：

- [runtime-stage-contract.md](00-core/runtime-stage-contract.md)

涉及 LLM request 组织、cache-aware rendering、request audit 的从属说明是：

- [runtime-request-cache-note.md](00-core/runtime-request-cache-note.md)

若旧文档与核心合约冲突，以 `00-core/runtime-stage-contract.md` 为准。

## Current Documentation Status

当前文档迁移按 [docs-domain-module-flow-reorganization-plan.md](docs-domain-module-flow-reorganization-plan.md:1) 执行。

当前已完成：

- 固化唯一 core 文档入口
- 从状态文档中抽出当前事实入口
- 记录迁移过程中的冲突、不完整项和衔接断点
- 第一轮目录骨架与主要阅读入口已经建立完成
- active 设计正文的大部分旧 root 引用已经清理

当前阶段：

- 若关注当前权威设计，优先阅读 `00-core` 到 `60-verification`
- 若关注历史实现或已完成计划，优先阅读 `90-archive`
- 若关注仍未完全收口的少量尾项，查看：
  - [current-unclosed-matrix.md](01-overview/current-unclosed-matrix.md)
  - [information-model-alignment-audit.md](01-overview/information-model-alignment-audit.md)

当前根目录保留 2 份正式文档：

- 本文档
- `docs-domain-module-flow-reorganization-plan.md`

当前仍建议结合以下文档使用：

- [incomplete-and-connection-gap-tracker.md](01-overview/incomplete-and-connection-gap-tracker.md)
- [information-model-alignment-audit.md](01-overview/information-model-alignment-audit.md)
- [current-unclosed-matrix.md](01-overview/current-unclosed-matrix.md)
- [docs-domain-module-flow-reorganization-plan.md](docs-domain-module-flow-reorganization-plan.md)
