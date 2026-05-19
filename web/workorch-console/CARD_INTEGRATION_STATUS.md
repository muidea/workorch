# WorkOrch Console 卡片组件集成 - 当前状态

## 已完成工作

### 1. 基础设施（100%）
- ✅ 创建 8 个卡片组件文件
- ✅ 创建通用组件（StateTag、MetricGrid、CompactTags、BoundedParagraph、LoadingState）
- ✅ 创建通用 Hooks（usePagination、useSSE、useDebounce）
- ✅ 创建工具函数模块（constants、format、state）
- ✅ 创建状态管理 Store（workspaceStore）
- ✅ 添加组件导入到 main.tsx

### 2. 部分集成（40%）
- ✅ TaskProgressCard - 已替换 1 处调用（line 4607）
- ✅ TaskDecompositionCard - 已替换 1 处调用（line 4623）
- ✅ ExecutionSectionsCard - 已替换 1 处调用（line 4630）
- ✅ WebSourceCard - 已替换 1 处调用（line 4591）
- ❌ MemoryRecallCard - 未替换（1 处调用 line 4588）
- ❌ LLMUsageCard - 未替换（2 处调用 line 4597, 4860）
- ❌ LLMContextBudgetCard - 未替换（2 处调用 line 4594, 4855）
- ❌ ContextBundleCard - 未替换（1 处调用 line 4637）

## 当前构建错误

```
src/main.tsx(22,1): error TS6133: 'MemoryRecallCard' is declared but its value is never read.
src/main.tsx(24,1): error TS6133: 'LLMUsageCard' is declared but its value is never read.
src/main.tsx(25,1): error TS6133: 'LLMContextBudgetCard' is declared but its value is never read.
src/main.tsx(26,1): error TS6133: 'ContextBundleCard' is declared but its value is never read.
src/main.tsx(352,10): error TS6133: 'taskProgressCard' is declared but its value is never read.
src/main.tsx(459,10): error TS6133: 'taskDecompositionCard' is declared but its value is never read.
src/main.tsx(535,10): error TS6133: 'executionSectionsCard' is declared but its value is never read.
src/main.tsx(4607,15): error TS2322: Type '{ progress: TaskProgress; repairLoop: RepairLoop | undefined; }' is not assignable to type 'IntrinsicAttributes & TaskProgressCardProps'.
```

## 剩余工作清单

### 优先级 1：完成剩余组件调用替换

1. **MemoryRecallCard**（1 处）
   - Line 4588: `{memoryRecallCard(runSnapshot.memoryRecallTraces)}`
   - 替换为: `<MemoryRecallCard items={runSnapshot.memoryRecallTraces} />`

2. **LLMUsageCard**（2 处）
   - Line 4597: `{llmUsageCard(runLLMUsageSummary, runLLMUsageTraces)}`
   - Line 4860: `children: llmUsageCard(runLLMUsageSummary, runLLMUsageTraces)`
   - 替换为: `<LLMUsageCard summary={runLLMUsageSummary} traces={runLLMUsageTraces} />`

3. **LLMContextBudgetCard**（2 处）
   - Line 4594: `{llmContextBudgetCard(runLLMContextBudgets)}`
   - Line 4855: `children: llmContextBudgetCard(runLLMContextBudgets)`
   - 替换为: `<LLMContextBudgetCard traces={runLLMContextBudgets} />`

4. **ContextBundleCard**（1 处）
   - Line 4637: `{contextBundleCard(contextBundle)}`
   - 替换为: `<ContextBundleCard bundle={contextBundle} />`

5. **WebSourceCard**（1 处）
   - Line 4850: `children: webSourceCard(runWebSources)`
   - 替换为: `children: <WebSourceCard items={runWebSources} />`

### 优先级 2：删除旧函数定义

删除以下函数（约 800 行）：
- Line 352: `function taskProgressCard` (~100 行)
- Line 459: `function taskDecompositionCard` (~70 行)
- Line 535: `function executionSectionsCard` (~90 行)
- Line 637: `function memoryRecallCard` (~110 行)
- Line 760: `function webSourceCard` (~80 行)
- Line 843: `function llmUsageCard` (~110 行)
- Line 953: `function llmContextBudgetCard` (~50 行)
- Line 1004: `function contextBundleCard` (~100 行)

### 优先级 3：修复 TaskProgressCard props 类型

TaskProgressCard 组件期望的 props：
```typescript
interface TaskProgressCardProps {
  progress?: TaskProgress;
  repairLoop?: RepairLoop;
}
```

当前调用（line 4607）：
```tsx
<TaskProgressCard
  progress={runObservability?.taskProgress ?? runSnapshot?.taskProgress ?? snapshot.taskProgress}
  repairLoop={runObservability?.repairLoop ?? runSnapshot?.repairLoop}
/>
```

需要检查 TaskProgressCard.tsx 的接口定义是否正确。

## 预期成果

完成所有集成后：
- main.tsx 将减少约 **800-1000 行**
- 所有卡片组件完全模块化
- 构建无错误
- 代码可维护性大幅提升

## 建议

由于手动替换容易出错且工作量大，建议：
1. 使用脚本批量替换函数调用
2. 或者分批次提交，每次完成 1-2 个组件的集成
3. 每次替换后立即验证构建，确保无错误

## 文件统计

- 新增文件：24 个
- 新增代码：~1723 行
- main.tsx 当前：7240 行
- main.tsx 预期：~6400 行（完成集成后）
