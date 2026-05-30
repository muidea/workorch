# WorkOrch Current State

Status: active

Type: overview

Last Updated: 2026-05-30

## Purpose

本文件用于表达当前文档体系和仓库状态的主入口信息。

它只保留：

- 当前唯一核心文档入口
- 当前迁移计划入口
- 当前本轮活跃文档治理工作

它不再承担以下职责：

- 完整服务验收基线
- 全量 API 清单
- 历史 MVP / P1 / closure 记录
- 已完成阶段计划索引

这些内容后续将分别迁移到 `50-product-interface/`、`60-verification/`
和 `90-archive/`。

## Current Documentation Authority

- 唯一核心 runtime 合约：
  [runtime-stage-contract.md](../00-core/runtime-stage-contract.md)
- 核心 request/cache 从属说明：
  [runtime-request-cache-note.md](../00-core/runtime-request-cache-note.md)
- 当前文档体系重组计划：
  [docs-domain-module-flow-reorganization-plan.md](../docs-domain-module-flow-reorganization-plan.md)
- 当前“内容不完整 / 无法完整衔接”统一追踪：
  [incomplete-and-connection-gap-tracker.md](incomplete-and-connection-gap-tracker.md)
- 当前“信息模型衔接一致性”统一审计：
  [information-model-alignment-audit.md](information-model-alignment-audit.md)

## Active Documentation Work

- 当前按“领域模型 / 功能模块 / 数据处理流”重组 `docs/`
- 第一轮目录骨架已建立完成
- 当前输出重点：
  - 已建立 `00-core` 到 `90-archive` 的目标目录骨架
  - 已建立跨批次的“内容不完整 / 无法完整衔接”统一追踪
  - 已建立“信息模型衔接一致性”统一审计
  - 当前重点转为对象边界、流转语义、读模型投影和旧引用收尾的一致性收口

当前阶段判断：

- active 设计正文的大规模拆分迁移已基本完成
- active 正文中的大部分旧 root 引用已清理完成
- 当前剩余工作主要收缩为：
  - 少量字段/规则补细
  - 少量过渡说明与历史追溯清理
  - archive / overview 层的最终闭合

## Active Runtime / Verification Status

- 2026-05-23 closure:
  - `POST /api/v1/workorch/workspaces/{workspaceId}/execution-activations` 已改为返回 lightweight snapshot projection
  - deployed verification 已通过：
    `verify-software-dev-goal-dod-e2e.sh`,
    `verify-software-requirement-development-orchestration.sh`,
    `verify-workspace-web-search-e2e.sh`,
    `verify-weather-ticket-approval-query.sh`,
    `verify-tui-long-output-e2e.sh`,
    `make verify-e2e-full`
  - e2e 改写全局 LLM 配置时已使用共享锁 `../../../../../../tmp/workorch-e2e-llm-config.lock`

- 2026-05-12 design-vs-implementation audit:
  - DMI-16 ~ DMI-24 与实现一致
  - runtime 核心流程契约大体一致，但仍存在少量文档滞后与轻微实现偏差
  - 架构文档相对代码树有明显滞后，后续以新 `30-module-architecture/` 为主持续收口
  - 恢复治理设计与实现大体一致，但 duplicate failure、预算数值、自动恢复路径等说明仍需继续细化

## Active Follow-up TODO

- Activation response:
  - 补 `execution-activations` 的 latency/size regression coverage
  - 增加 activation critical path observability
- Verification isolation:
  - 将共享 LLM 配置锁扩展到其他会改写全局 provider 配置的脚本
  - 评估引入 per-test provider namespace / config overlay
- Read-model cost control:
  - 降低脚本和 UI 对 full workspace snapshot 的默认依赖
  - 评估 workspace board/detail 是否统一把 activation response 视为 lightweight acknowledgement

## Current Repository Notes

- 当前仓库仍是单 `go.mod` 代码库。
- `application/workorch` 是主执行入口。
- `internal/modules/` 是主要模块落点。
- 原始 `docs/` 仍在迁移中，旧路径暂时保留以便过渡。

## Stable Repository Facts

当前可稳定视为仓库事实的内容包括：

- 单仓 Go 服务工程，主执行面仍围绕 `application/workorch`
- 文档、运行时、appservice、kernel、blocks、read-model 仍在同一仓内协同演进
- 服务对外正式产品面稳定收敛到：
  - HTTP / Web Console：`workorchd`
  - local-first CLI/TUI：`workorch`
- 当前设计权威已经从旧根目录综合文档分离到：
  - `00-core/`
  - `10-domain-model/`
  - `20-runtime-flow/`
  - `30-module-architecture/`
  - `50-product-interface/`
  - `60-verification/`

## Main Module Set Snapshot

当前仓库主模块集合可稳定概括为：

- `application/workorch`
  - 统一执行入口；承接本地 CLI/TUI 与 server 启动
- `internal/modules/application/workorchd`
  - HTTP / Web / appservice / bootstrap / observed read-model 产品面
- `internal/modules/kernel`
  - Workspace / Run / Goal / DoD / Result / Artifact / Governance / Context 主体模型与 orchestration 核心
- `internal/modules/blocks`
  - LLM / shell / patch / MCP / approval 等运行适配块
- `internal/pkg/repository`
  - memory / file backend 持久化契约与实现
- `internal/pkg/events`
  - runtime / governance / projection 所需事件总线与 envelope 契约

当前文档与这些模块的稳定映射已经形成：

- 对象边界
  - `10-domain-model/`
- 运行链路
  - `20-runtime-flow/`
- 代码职责落点
  - `30-module-architecture/`
- 产品面与验收面
  - `50-product-interface/`、`60-verification/`

## Migration Progress Snapshot

当前文档迁移进度可稳定判断为：

- 已完成：
  - 目录骨架与阅读入口重组
  - core contract 唯一入口统一
  - active API / verification / profiles / product / archive 主体迁移
  - active 正文中的绝大多数旧 root 引用清理
- 正在收尾：
  - 少量对象边界、规则和示例补细
  - overview / archive / history 文档的最终压缩
  - 少量历史兼容入口与追溯说明收口
- 未完成但已收缩到尾项：
  - 少量历史 audit 语境与旧引用兼容说明
  - 个别文档中的示意/产物路径样例进一步规范化

## Current Stable Structure Snapshot

- `00-core/`：唯一 runtime contract 与 request/cache 从属说明
- `10-domain-model/`：Goal / DoD、Run / Step / Result / Artifact、Context / Capability / Memory、Event / Observability 对象边界
- `20-runtime-flow/`：activation、planning/execution、verification/acceptance、repair/replan、context assembly 链路
- `30-module-architecture/`：application、kernel、blocks、storage/read-model、provider assembly 等模块边界
- `40-profiles-capabilities/`：workspace / software-dev / mcp / runtime capability 特化
- `50-product-interface/`：service API、TUI、Web Console 产品面
- `60-verification/`：service acceptance、verifier、TUI acceptance、audits
- `70-roadmap/`：活跃计划与候选项
- `90-archive/`：completed / superseded / records

## Migration Note

历史综合状态入口已完成拆分：

- 当前事实与迁移状态：
  - `current-state.md`
- 服务 API：
  - `../50-product-interface/service-api.md`
- 服务验收：
  - `../60-verification/service-acceptance.md`
- 历史交付记录：
  - `../90-archive/records/delivery-history.md`

其余兼容入口与退役时机统一由：

- `status-reference-cleanup-audit.md`
