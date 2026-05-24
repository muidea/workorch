# WorkOrch Console Web 打包与 UI 容器层级优化经验总结

本文记录本轮 `web/workorch-console` 优化中的可复用经验，适用于后续继续维护 WorkOrch Console 或其他类似内置 Web 控制台。

## 背景问题

本轮优化前，`npm --prefix web/workorch-console run build` 输出中存在明显异常：

- `vendor-antd-*.js` 约 `2028 KB`，gzip 后约 `634 KB`。
- Vite transform 模块数约 `9879`。
- 主入口 `workorch-console.js` 约 `280 KB`。
- 页面中大量使用 `ProCard gutter/wrap/split` 作为布局容器，替换 Pro Components 后容易产生多层可见 Card，导致视觉层级不一致。

这些问题的共同表现是：打包输出过大、首屏入口承载过多内容、依赖树复杂、页面容器层级和视觉层级不一致。

## 根因判断

### 1. 手动 vendor chunk 聚合过粗

原 Vite 配置将以下依赖强制聚合到一个 `vendor-antd` chunk：

- `antd`
- `@ant-design/pro-components`
- `@ant-design/icons`

这会让最终产物看起来像“全量 AntD 输出”。实际更准确地说，是 AntD、Pro Components、icons 及其依赖被人为聚合成一个大 chunk，失去了按页面和按实际使用模块拆分的空间。

### 2. `@ant-design/pro-components` 聚合入口成本高

Console 实际只用到了类似：

- `ProLayout`
- `PageContainer`
- `ProCard`
- `ProTable`

但从 `@ant-design/pro-components` 聚合入口导入，会引入 Pro Components 生态的较大依赖树。即使业务页面只用了少量基础展示能力，打包器也难以将这些复杂依赖裁剪到很小。

### 3. 页面静态导入过多

入口文件静态导入多个页面，会让首屏入口包含不必要的页面代码和组件依赖。对于控制台类应用，用户通常一次只看一个页面，页面级 lazy loading 更符合使用方式。

### 4. UI 布局语义混乱

`ProCard` 同时被用作：

- 真正的内容 Card。
- 纯布局容器。
- 分栏容器。
- 展开行详情容器。

在 Pro Components 中这些模式可能视觉上还能接受，但替换为轻量兼容层后，如果不区分“布局容器”和“内容容器”，就会出现多层边框、多层阴影、间距不一致的问题。

## 最终处理方案

### 1. 移除 Pro Components 聚合依赖

移除依赖：

```json
"@ant-design/pro-components": "..."
```

保留实际需要的基础依赖：

- `antd`
- `@ant-design/icons`
- `react`
- `react-dom`
- `zustand`

本轮移除后，依赖树减少约 `62 packages`。

### 2. 引入本地轻量兼容层

新增本地组件：

- `ProLayout`
- `PageContainer`
- `ProCard`
- `ProTable`
- `CardGrid`

落点：

```text
web/workorch-console/src/components/proCompat.tsx
```

设计原则：

- 兼容现有页面调用形态，降低迁移成本。
- 底层只使用 `antd` 的 `Layout`、`Menu`、`Card`、`Table`、`Typography`、`Spin`。
- 忽略 `search/options` 等 ProTable 展示开关，只保留当前页面实际需要的表格能力。
- `ProCard` 只在存在 `title/subTitle/extra/actions` 等内容边界时渲染为可见 Card。
- 无标题、仅 `gutter/wrap/split/bordered=false` 的布局型 `ProCard` 不再产生可见 Card。
- 新增 `CardGrid` 专门表达布局，不再让 `ProCard` 同时承担布局职责。

### 3. 页面级 lazy loading

入口只保留应用外壳和路由状态，页面组件按需加载：

- `WorkspaceBoardPage`
- `WorkspaceDetailPage`
- `GovernanceQueuePage`
- `RuntimeControlPage`
- `ConfigurationPage`
- `ServicePage`

优化后，主入口从约 `280 KB / gzip 85 KB` 降至约 `7 KB / gzip 3 KB`。

### 4. Vite chunk 策略改为按实际依赖拆分

保留明确、低风险的 chunk 分组：

- `vendor-react`
- `vendor-antd-runtime`
- `vendor-state`

避免继续使用 `vendor-antd: ['antd', '@ant-design/pro-components', '@ant-design/icons']` 这种粗粒度聚合。

经验规则：

- 不要把大型 UI 聚合入口和 UI runtime 强制塞进一个固定 chunk。
- manual chunk 应服务于缓存和加载边界，而不是用来掩盖依赖结构问题。
- 若出现 circular chunk warning，优先减少人为拆分，而不是继续补更多匹配规则。

### 5. 清理无效容器层级

将页面中的纯布局型容器从：

```tsx
<ProCard gutter={16} wrap>
  <ProCard title="A">...</ProCard>
  <ProCard title="B">...</ProCard>
</ProCard>
```

改为：

```tsx
<CardGrid>
  <ProCard title="A">...</ProCard>
  <ProCard title="B">...</ProCard>
</CardGrid>
```

收口准则：

- `PageContainer` 只作为页面壳。
- `CardGrid` 只作为布局。
- `ProCard` 只作为有明确标题、边界或内容分组的 Card。
- 展开行详情中也避免使用“空 Card 包多 Card”。
- 不使用 card-in-card 表达普通页面区块。

## 优化结果

本轮优化后的典型构建结果：

```text
transformed modules: 3120
workorch-console.js: 7.04 KB / gzip 2.96 KB
vendor-antd-runtime.js: 967.15 KB / gzip 300.75 KB
vendor-react.js: 193.00 KB / gzip 60.58 KB
```

与优化前相比：

- transform 模块数从约 `9879` 降到 `3120`。
- 原 `vendor-antd` 从约 `2028 KB / gzip 634 KB` 降到 `vendor-antd-runtime` 约 `967 KB / gzip 301 KB`。
- 入口文件从约 `280 KB / gzip 85 KB` 降到约 `7 KB / gzip 3 KB`。
- 页面代码按页面拆分，首屏只加载应用壳和当前页面所需内容。
- `@ant-design/pro-components` 与 `.ant-pro-*` 样式引用已清空。
- `ProCard gutter/wrap/split` 作为布局容器的用法已清空。

## 验证方式

每次类似优化后至少执行：

```bash
npm --prefix web/workorch-console run build
git diff --check
rg -n "@ant-design/pro-components|ant-pro" web/workorch-console internal/modules/application/workorchd/service/server/consoleassets/dist
rg -n "<ProCard[^>]*gutter=|<ProCard[^>]*split=|<ProCard[^>]*wrap" web/workorch-console/src
```

关注点：

- build 必须通过。
- 不应残留 `@ant-design/pro-components`。
- 不应残留 `.ant-pro-*` 样式依赖。
- 不应继续使用 `ProCard gutter/wrap/split` 表达布局。
- 构建产物中不应重新出现超大的 Pro Components 聚合包。

## 后续复用准则

### 依赖准则

- 引入 UI 库前先确认是否需要完整组件生态。
- 控制台类页面优先使用基础组件组合，不默认引入 Pro/Enterprise 聚合组件。
- 若只需要 Card/Table/Layout 的基础能力，优先在项目内封装轻量 wrapper。

### 打包准则

- 页面级功能使用 lazy loading。
- vendor chunk 只按真实缓存边界拆分。
- 不把“库名”直接等同于“chunk 边界”；尤其不要把聚合包固定塞入单一 vendor。
- 构建体积变化必须记录 gzip 前后数据。

### UI 层级准则

- 页面壳、布局、内容 Card 三者职责分离。
- 纯布局必须使用 `CardGrid` 或普通 CSS 容器。
- Card 只用于有标题、边界、操作区或明确内容分组的区域。
- 避免 card-in-card，除非内层是表格展开详情、重复项或确实需要独立边界。
- 展开行详情默认使用轻量布局，不要再套视觉容器。

### 代码维护准则

- 兼容层应保持小而明确，不复制 Pro Components 的完整 API。
- 新增兼容字段必须以当前页面真实使用为依据。
- 如果某个页面需要复杂 Pro 能力，应先评估是否值得局部引入，而不是恢复全局聚合依赖。

## 风险与边界

本轮优化保留了 `antd` 本体，因此 `vendor-antd-runtime` 仍会相对较大。继续压缩该 chunk 的方向不是恢复 Pro Components，而是：

- 减少页面实际使用的重量级 antd 组件。
- 对低频页面做更细的动态导入。
- 评估是否将部分只读展示表格替换为轻量 HTML table。
- 避免在公共 helper 中导入过多 antd 组件，必要时按页面拆分 helper。

当前不建议为了追求更小体积而直接手写所有 antd 基础组件替代品；这会显著增加 UI 一致性和维护成本。
