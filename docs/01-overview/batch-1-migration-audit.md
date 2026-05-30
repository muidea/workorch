# Batch 1 Migration Audit

Status: active

Type: audit

Last Updated: 2026-05-30

## Scope

本审计文档记录 `Batch 1` 迁移过程中识别出的三类问题：

- 冲突和不一致
- 内容不完整、需要补充
- 无法完整衔接的内容断点

说明：

- 本文中的“旧综合状态文档”指当时仍位于根目录、后续降级为
  `../workorch-status.md` 的历史汇总文档
- 除首次点名外，后文优先使用“旧综合状态文档”称呼，以强调这里记录的是
  Batch 1 当时的历史语境，而不是当前事实依赖

## 1. 冲突和不一致

### 1.1 Core 入口路径曾经不一致，现已修复主入口

- `../../AGENTS.md` 和 `../README.md` 已切换到 `../00-core/runtime-stage-contract.md`
- 旧路径 `../workorch-runtime-stage-contract-neutrality-design.md` 当前仅保留过渡说明

处理结论：

- 主入口已修复
- 后续仍需逐步清理其他旧文档中的历史引用

### 1.2 Request Note 入口路径曾经不一致，现已修复主入口

- `../../AGENTS.md` 已切换到 `../00-core/runtime-request-cache-note.md`
- 旧路径 `../workorch-llm-request-cache-aware-context-design.md` 当前仅保留过渡说明

处理结论：

- 主入口已修复
- 后续仍需清理旧文档中的历史引用

### 1.3 当前状态入口与旧综合状态文档角色冲突

- 旧综合状态文档在 Batch 1 当时仍承担当前事实、接口清单、验收、已完成计划、历史记录多种角色
- `current-state.md` 已建立为新的当前入口

处理结论：

- 该旧综合状态文档已在后续批次继续拆分，当前已降级为过渡索引

## 2. 内容不完整，需要补充

### 2.1 新的 docs 总入口仍缺阅读导航

当前 `../README.md` 已建立，但仍需后续批次补充：

- 领域模型入口
- runtime flow 入口
- module architecture 入口
- profile/capability 入口
- verification 入口

### 2.2 当前事实入口仍缺稳定仓库事实摘要

`current-state.md` 目前只固化了文档治理入口和迁移状态，
后续还需要补充：

- 仓库主结构摘要
- 当前主模块集合摘要
- 当前文档迁移进度摘要

### 2.3 缺少服务接口与验收的承接文档

Batch 1 当时，旧综合状态文档中的以下内容还没有迁出：

- 服务 API 入口清单
- service acceptance baseline
- 历史交付记录

后续批次已按新结构补齐：

- `../50-product-interface/service-api.md`
- `../60-verification/service-acceptance.md`
- `../90-archive/records/delivery-history.md`

## 3. 无法完整衔接的内容断点

### 3.1 旧文档间交叉引用仍指向旧路径

当前仍有多份文档继续引用：

- `../workorch-runtime-stage-contract-neutrality-design.md`
- `../workorch-llm-request-cache-aware-context-design.md`
- 旧综合状态文档

这意味着在后续批次完成前，文档体系仍处于“新旧入口并存”的过渡态。

处理结论：

- 允许过渡，但必须逐批清理交叉引用

### 3.2 Core 文档中的从属说明尚未剥离

`../00-core/runtime-stage-contract.md` 目前仍是从旧文档直接复制而来。
它内部仍引用旧 request note 路径，并保留大量后续需要迁出的非 core 内容。

处理结论：

- Batch 1 仅建立入口，不做大幅重构
- 后续批次必须继续从 core 文档中迁出非 core 内容

### 3.3 旧综合状态文档仍是信息汇总源

Batch 1 当时，很多仓库信息、API 入口、验收基线仍只能从
旧综合状态文档获得。在 `../50-product-interface/service-api.md`、`../60-verification/service-acceptance.md`
和 `../90-archive/records/delivery-history.md` 创建前，新结构无法完全替代旧状态文档。

处理结论：

- 这是当时的已知过渡断点
- 当前对应承接文档已建立，该旧综合状态文档已降级为过渡索引
