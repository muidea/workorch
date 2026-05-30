# WorkOrch Web Console Bundle And UI Optimization Record

Status: active

Type: product-interface

Last Updated: 2026-05-29

## Purpose

本文件承接 Web Console 打包与 UI 容器层级优化记录，保留后续继续维护 Console 时应复用的产品层经验与约束。

## Problem Snapshot

本轮优化前，Console 同时存在两类问题：

- 打包输出过大，`vendor-antd-*` 与主入口承载过多不必要代码。
- 页面把 `ProCard` 同时当作内容卡片和布局容器，导致可见层级混乱。

这些问题会直接影响首屏加载体积、依赖树复杂度和页面视觉一致性。

## Product Decisions

### 1. 依赖与兼容层

- 移除 `@ant-design/pro-components` 聚合依赖，避免为少量布局/表格能力引入过大的依赖树。
- 在项目内提供轻量兼容层，只保留当前页面确实使用的 `ProLayout`、`PageContainer`、`ProCard`、`ProTable`、`CardGrid` 能力。
- 兼容层只作为现有页面迁移的承接层，不复制 Pro Components 全量 API。

### 2. 入口与分包

- 入口仅保留应用壳和路由状态，页面组件按页面粒度 lazy load。
- manual chunk 只按真实缓存边界拆分，不再用单个粗粒度 `vendor-antd` 掩盖依赖结构问题。
- 不再把大型 UI 聚合入口和 UI runtime 强行塞进同一个固定 chunk。

### 3. UI 容器语义

- `PageContainer` 只作为页面壳。
- `CardGrid` 只作为布局容器。
- `ProCard` 只用于有明确标题、边界或内容分组的可见卡片。
- 展开行详情和普通页面区块都应避免无意义的 card-in-card。

## Measured Outcome

- transform 模块数约从 `9879` 降到 `3120`
- 主入口约从 `280 KB / gzip 85 KB` 降到 `7 KB / gzip 3 KB`
- `vendor-antd` 级别大包收敛为更合理的 `vendor-antd-runtime`
- `@ant-design/pro-components` 与 `.ant-pro-*` 样式依赖已清空
- 纯布局型 `ProCard gutter/wrap/split` 用法已清空

## Verification

每次同类优化后至少执行：

```bash
npm --prefix web/workorch-console run build
git diff --check
rg -n "@ant-design/pro-components|ant-pro" web/workorch-console internal/modules/application/workorchd/service/server/consoleassets/dist
rg -n "<ProCard[^>]*gutter=|<ProCard[^>]*split=|<ProCard[^>]*wrap" web/workorch-console/src
```

验收重点：

- build 必须通过
- 不残留 Pro Components 依赖和 `.ant-pro-*` 样式
- 不再用 `ProCard gutter/wrap/split` 表达布局
- 构建产物中不重新出现异常粗大的聚合 UI chunk

## Reuse Rules

- 控制台类页面优先使用基础组件组合，不默认引入 Pro/Enterprise 聚合组件。
- 页面壳、布局容器、内容卡片必须保持职责分离。
- 兼容层应保持小而明确；新增字段必须以当前页面真实使用为依据。
- 若未来还需继续压缩 `vendor-antd-runtime`，优先从页面实际使用、低频页面动态导入和 helper 拆分入手，而不是恢复 Pro Components 聚合依赖。

## Source Documents

- `../../90-archive/records/overview.md`
