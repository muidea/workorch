# 性能优化计划

## 概述

针对 WorkOrch Console 的性能瓶颈进行优化，主要关注：
1. React.memo - 避免不必要的组件重渲染
2. useMemo - 缓存复杂计算结果
3. useCallback - 稳定函数引用
4. 虚拟滚动 - 优化长列表渲染

## 已完成优化

### ✅ SSE 事件处理优化
- 使用 useRef 存储回调，避免因回调变化导致 EventSource 重连
- 防抖处理（150ms），避免频繁触发更新
- 自动错误处理和清理
- **影响**: 减少不必要的网络连接和状态更新

## 待优化组件

### 高优先级（大型列表组件）

#### 1. WorkspaceDetailPage - 运行时数据表格
- **问题**: 包含多个 ProTable，每次状态更新都会重新渲染
- **优化方案**:
  - 使用 useMemo 缓存表格列定义
  - 使用 useMemo 缓存数据源
  - 使用 useCallback 稳定事件处理函数
- **预期收益**: 减少 50-70% 的重渲染

#### 2. DecisionQueuePage - 决策队列表格
- **问题**: ProTable 在每次状态更新时重新渲染
- **优化方案**:
  - useMemo 缓存 columns 定义
  - useCallback 稳定 loadPage 函数
- **预期收益**: 减少 40-60% 的重渲染

#### 3. ApprovalQueuePage - 审批队列表格
- **问题**: 同 DecisionQueuePage
- **优化方案**: 同 DecisionQueuePage
- **预期收益**: 减少 40-60% 的重渲染

#### 4. WorkspaceBoardPage - 工作空间看板
- **问题**: 卡片列表在每次状态更新时重新渲染
- **优化方案**:
  - React.memo 包装卡片组件
  - useMemo 缓存卡片数据
- **预期收益**: 减少 30-50% 的重渲染

### 中优先级（复杂计算）

#### 5. ConfigurationPage - 配置页面
- **问题**: 包含大量表单和配置项，计算复杂
- **优化方案**:
  - useMemo 缓存模板选项
  - useMemo 缓存提供商列表
  - useCallback 稳定表单处理函数
- **预期收益**: 减少 20-40% 的重渲染

#### 6. RuntimeControlPage - 运行时控制
- **问题**: 包含多个表格和复杂的能力配置
- **优化方案**:
  - useMemo 缓存表格数据
  - useCallback 稳定事件处理
- **预期收益**: 减少 20-40% 的重渲染

### 低优先级（小型组件）

#### 7. 卡片组件优化
- TaskProgressCard
- TaskDecompositionCard
- ExecutionSectionsCard
- MemoryRecallCard
- WebSourceCard
- LLMUsageCard
- LLMContextBudgetCard
- ContextBundleCard

**优化方案**: 使用 React.memo 包装，避免父组件更新时重渲染

## 优化策略

### React.memo 使用原则
1. 用于纯展示组件（props 相同时渲染结果相同）
2. 用于大型列表项组件
3. 用于频繁重渲染但 props 很少变化的组件
4. **不要过度使用** - 小型组件的 memo 开销可能大于收益

### useMemo 使用原则
1. 用于复杂计算（如数组过滤、排序、映射）
2. 用于对象/数组字面量（避免引用变化）
3. 用于表格列定义（columns）
4. **不要过度使用** - 简单计算不需要 memo

### useCallback 使用原则
1. 用于传递给子组件的回调函数
2. 用于传递给 memo 组件的函数 props
3. 用于 useEffect 依赖的函数
4. **不要过度使用** - 不传递给子组件的函数不需要 callback

## 实施步骤

### 第一阶段：表格组件优化
1. ✅ 优化 SSE 事件处理
2. 优化 DecisionQueuePage 表格
3. 优化 ApprovalQueuePage 表格
4. 优化 WorkspaceDetailPage 表格

### 第二阶段：列表组件优化
1. 优化 WorkspaceBoardPage 卡片列表
2. 优化 RuntimeControlPage 表格
3. 优化 ConfigurationPage 表单

### 第三阶段：卡片组件优化
1. 为所有卡片组件添加 React.memo
2. 优化卡片内部的复杂计算

### 第四阶段：虚拟滚动（任务 #31）
1. 为长列表实现虚拟滚动
2. 使用 react-window 或 react-virtual

## 性能监控

### 关键指标
- 首次渲染时间（FCP）
- 交互响应时间（TTI）
- 列表滚动帧率（FPS）
- 内存使用量

### 监控工具
- React DevTools Profiler
- Chrome Performance 面板
- Lighthouse

## 预期成果

- **重渲染次数**: 减少 40-60%
- **内存使用**: 减少 20-30%
- **交互响应**: 提升 30-50%
- **用户体验**: 显著提升，特别是在大数据量场景

---
创建时间: 2026-05-XX
状态: 进行中
