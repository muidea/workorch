# 卡片组件提取指南

## 概述

由于卡片渲染函数（如 `taskProgressCard`、`taskDecompositionCard` 等）与 main.tsx 中的大量辅助函数和类型紧密耦合，完全提取需要大量重构工作。

本指南提供了一个渐进式的迁移策略。

## 当前状态

main.tsx 中有 8 个卡片渲染函数：

1. `taskProgressCard` (105行) - 任务进度卡片
2. `taskDecompositionCard` (76行) - 任务分解卡片
3. `executionSectionsCard` (58行) - 执行分区卡片
4. `memoryRecallCard` (84行) - 记忆召回卡片
5. `webSourceCard` (84行) - Web来源卡片
6. `llmUsageCard` (110行) - LLM使用量卡片
7. `llmContextBudgetCard` (51行) - 上下文预算卡片
8. `contextBundleCard` (112行) - 上下文包卡片

**总计**: ~680 行代码

## 依赖关系

这些卡片函数依赖于：

### 辅助函数（已提取到 utils/）
- ✅ `stateLabel()` - 状态标签
- ✅ `getStateColor()` - 状态颜色
- ✅ `shortId()` - ID截断
- ✅ `errorMessage()` - 错误消息
- ✅ `compactJSON()` - JSON格式化
- ✅ `numberText()` - 数字格式化
- ✅ `costText()` - 费用格式化
- ✅ `mergeById()` - 数组合并
- ✅ `asStringArray()` - 字符串数组转换
- ✅ `taskProgressStepStatus()` - 步骤状态

### 组件（已提取到 components/common/）
- ✅ `StateTag` - 状态标签组件
- ✅ `MetricGrid` - 指标网格组件
- ✅ `CompactTags` - 紧凑标签组件
- ✅ `BoundedParagraph` - 有界段落组件

### 仍在 main.tsx 中的函数
- ❌ `boundedParagraph()` - 有界段落渲染（需要迁移到组件）
- ❌ `compactTags()` - 紧凑标签渲染（需要迁移到组件）
- ❌ `stateTag()` - 状态标签渲染（需要迁移到组件）
- ❌ `metricGrid()` - 指标网格渲染（需要迁移到组件）

## 迁移策略

### 方案 A：渐进式迁移（推荐）

**优点**：
- 风险低，可以逐步验证
- 不影响现有功能
- 可以随时回滚

**步骤**：

1. **阶段 1：统一辅助函数使用**
   - 在 main.tsx 中导入 utils 中的函数
   - 逐步替换本地实现
   - 验证功能正常

2. **阶段 2：创建包装组件**
   - 创建 `LegacyCardWrapper` 组件
   - 将卡片函数包装为组件
   - 保持原有逻辑不变

3. **阶段 3：逐个重构**
   - 从最简单的卡片开始（如 `llmContextBudgetCard`）
   - 重构为独立组件
   - 添加 Props 类型定义
   - 编写单元测试

4. **阶段 4：优化和清理**
   - 添加 memo 优化
   - 提取共用逻辑
   - 删除旧代码

### 方案 B：一次性重构

**优点**：
- 架构更清晰
- 一次性解决所有问题

**缺点**：
- 风险高，可能引入 bug
- 需要大量测试
- 短期内影响开发效率

## 示例：迁移 boundedParagraph

### 之前（main.tsx 中的函数）

```typescript
function boundedParagraph(value?: React.ReactNode, options?: { rows?: number; copyText?: string; className?: string }) {
  return (
    <Typography.Paragraph
      className={['workorch-bounded-text', options?.className].filter(Boolean).join(' ')}
      copyable={options?.copyText ? { text: options.copyText } : false}
      ellipsis={{ rows: options?.rows ?? 2, expandable: true, symbol: '展开' }}
    >
      {value || '-'}
    </Typography.Paragraph>
  );
}
```

### 之后（使用 BoundedParagraph 组件）

```typescript
// 在 main.tsx 中
import { BoundedParagraph } from './components/common';

// 替换所有 boundedParagraph() 调用
// 之前：
boundedParagraph(progress.summary, { rows: 2 })

// 之后：
<BoundedParagraph text={progress.summary} rows={2} />
```

## 快速开始：创建第一个卡片组件

### 1. 选择最简单的卡片

推荐从 `llmContextBudgetCard` 开始，因为它：
- 相对独立
- 依赖较少
- 逻辑清晰

### 2. 创建组件文件

```typescript
// components/cards/LLMContextBudgetCard.tsx
import { Empty, ProTable, Space } from 'antd';
import type { LLMContextBudgetTrace } from '../../types';
import { MetricGrid } from '../common';
import { compactJSON } from '../../utils';

interface LLMContextBudgetCardProps {
  traces?: LLMContextBudgetTrace[];
}

export function LLMContextBudgetCard({ traces }: LLMContextBudgetCardProps) {
  if (!traces || traces.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无上下文预算轨迹" />;
  }

  const truncated = traces.filter(t => t.truncated).length;
  const p0Overflow = traces.filter(t => t.p0Overflow).length;

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <MetricGrid items={[
        { key: 'total', label: '轨迹总数', value: traces.length, color: '#1668dc' },
        { key: 'truncated', label: '已截断', value: truncated, color: truncated ? '#cf1322' : undefined },
        { key: 'p0Overflow', label: 'P0溢出', value: p0Overflow, color: p0Overflow ? '#d4380d' : undefined }
      ]} />
      
      <ProTable<LLMContextBudgetTrace>
        rowKey="stepId"
        search={false}
        options={false}
        pagination={{ pageSize: 8 }}
        size="small"
        columns={[
          { title: '步骤', dataIndex: 'stepId', copyable: true, ellipsis: true },
          { title: '模型', dataIndex: 'model', ellipsis: true },
          { title: '最大字节', dataIndex: 'maxBytes', width: 120, render: (_, item) => item.maxBytes.toLocaleString() },
          { title: '实际字节', dataIndex: 'actualBytes', width: 120, render: (_, item) => item.actualBytes.toLocaleString() },
          { title: '截断', dataIndex: 'truncated', width: 80, render: (_, item) => item.truncated ? '是' : '否' }
        ]}
        expandable={{
          expandedRowRender: (item) => (
            <pre className="json-panel json-panel-large">{compactJSON(item)}</pre>
          )
        }}
        dataSource={traces}
      />
    </Space>
  );
}
```

### 3. 在 main.tsx 中使用

```typescript
// 导入
import { LLMContextBudgetCard } from './components/cards/LLMContextBudgetCard';

// 替换原来的函数调用
// 之前：
{llmContextBudgetCard(runObservability?.llmContextBudgets)}

// 之后：
<LLMContextBudgetCard traces={runObservability?.llmContextBudgets} />
```

## 下一步行动

### 立即可做（低风险）

1. ✅ 在 main.tsx 中导入并使用 utils 中的函数
2. ✅ 创建 `components/cards/index.ts` 导出文件
3. ✅ 提取 1-2 个简单的卡片组件作为示例

### 短期（1-2天）

4. 创建剩余的卡片组件
5. 逐步替换 main.tsx 中的函数调用
6. 验证功能完整性

### 中期（3-5天）

7. 优化卡片组件（添加 memo）
8. 提取共用逻辑
9. 添加单元测试

## 注意事项

1. **保持功能等价**：重构时确保行为完全一致
2. **类型安全**：为所有 Props 添加 TypeScript 类型
3. **样式一致**：保持与现有 styles.css 的兼容
4. **渐进式迁移**：不要一次性修改太多代码
5. **充分测试**：每次迁移后都要验证功能

## 总结

卡片组件的提取是一个复杂但重要的工作。建议采用渐进式迁移策略，从简单的卡片开始，逐步完成所有卡片的组件化。

当前已经完成的基础设施（utils、hooks、common components）为卡片组件的提取提供了良好的支持。
