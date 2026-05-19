# WorkOrch Web Console 渐进式迁移指南

## 当前状态

✅ **第一阶段完成**：基础设施已就绪
- 工具函数模块（utils/）
- 通用 Hooks（hooks/）
- 通用组件（components/common/）
- 状态管理（store/）
- 完整的目录结构

⏳ **第二阶段进行中**：渐进式迁移 main.tsx

## 为什么采用渐进式迁移？

由于 main.tsx 有 7224 行代码，一次性重构风险太大。渐进式迁移可以：
- ✅ 降低风险，每次改动可验证
- ✅ 保持功能正常运行
- ✅ 随时可以回滚
- ✅ 团队成员可以并行工作

## 迁移策略

### 阶段 2.1：准备工作（已完成）

- [x] 创建工具函数模块
- [x] 创建通用 Hooks
- [x] 创建通用组件
- [x] 创建状态管理 Store
- [x] 在 main.tsx 中添加导入注释（待启用）

### 阶段 2.2：替换工具函数（推荐下一步）

**目标**：将 main.tsx 中的工具函数调用替换为 utils 中的版本

**步骤**：

1. **启用一个工具函数导入**
   ```typescript
   // 在 main.tsx 顶部，取消注释一行
   import { errorMessage } from './utils';
   ```

2. **全局搜索替换**
   ```bash
   # 在 main.tsx 中搜索所有 errorMessage 的使用
   # 确认它们都使用了本地定义的 errorMessage 函数
   # 然后删除本地定义，使用导入的版本
   ```

3. **验证构建**
   ```bash
   npm run build
   ```

4. **测试功能**
   - 启动开发服务器
   - 测试相关功能
   - 确认无错误

5. **重复步骤 1-4**，逐个替换其他工具函数

**优先级排序**（从简单到复杂）：

1. ✅ `errorMessage()` - 最简单，无依赖
2. ✅ `isNotFoundError()` - 简单，无依赖
3. ✅ `shortId()` - 简单，无依赖
4. ✅ `compactJSON()` - 简单，无依赖
5. ✅ `numberText()` - 简单，无依赖
6. ✅ `costText()` - 简单，无依赖
7. ✅ `mergeById()` - 中等，用于分页
8. ✅ `asStringArray()` - 简单，类型转换
9. ✅ `stateLabel()` - 中等，使用常量
10. ✅ `getStateColor()` - 中等，使用常量

### 阶段 2.3：替换组件（中等难度）

**目标**：将渲染函数替换为组件

**示例：替换 stateTag()**

**之前**：
```typescript
// main.tsx 中的函数
function stateTag(state?: string) {
  const label = stateLabel(state);
  const colorMap: Record<string, string> = {
    running: 'blue',
    completed: 'green',
    // ...
  };
  return <Tag color={colorMap[state || ''] || 'default'}>{label}</Tag>;
}

// 使用
{stateTag(item.state)}
```

**之后**：
```typescript
// 导入组件
import { StateTag } from './components/common';

// 使用
<StateTag state={item.state} />
```

**步骤**：

1. 启用组件导入
2. 全局搜索 `stateTag(`
3. 替换为 `<StateTag state={...} />`
4. 删除本地 `stateTag` 函数定义
5. 验证和测试

**组件替换优先级**：

1. ✅ `stateTag()` → `<StateTag>` - 最常用
2. ✅ `compactTags()` → `<CompactTags>` - 常用
3. ✅ `boundedParagraph()` → `<BoundedParagraph>` - 常用
4. ✅ `metricGrid()` → `<MetricGrid>` - 中等使用

### 阶段 2.4：提取卡片组件（较复杂）

**目标**：将卡片渲染函数转换为独立组件

**从最简单的开始**：

1. **LLMContextBudgetCard**（51行，最简单）
   - 创建 `components/cards/LLMContextBudgetCard.tsx`
   - 复制函数逻辑
   - 转换为组件
   - 添加 Props 类型
   - 在 main.tsx 中使用

2. **ExecutionSectionsCard**（58行）
3. **TaskDecompositionCard**（76行）
4. **MemoryRecallCard**（84行）
5. **WebSourceCard**（84行）
6. **TaskProgressCard**（105行）
7. **LLMUsageCard**（110行）
8. **ContextBundleCard**（112行，最复杂）

### 阶段 2.5：拆分页面组件（最复杂）

**目标**：将页面组件从 main.tsx 提取到独立文件

**优先级**：

1. **ServicePage**（23行，最简单）
2. **WorkspaceBoardPage**（125行）
3. **GovernanceQueuePage**（24行）
4. **DecisionQueuePage**（248行）
5. **ApprovalQueuePage**（193行）
6. **RuntimeControlPage**（571行）
7. **ConfigurationPage**（774行）
8. **WorkspaceDetailPage**（2559行，最复杂）

## 实施建议

### 每次迁移的标准流程

1. **选择目标**：选择一个函数/组件/页面
2. **创建分支**：`git checkout -b refactor/migrate-xxx`
3. **实施迁移**：
   - 启用导入
   - 替换使用
   - 删除旧代码
4. **验证**：
   - `npm run build` 通过
   - 启动开发服务器测试
   - 检查控制台无错误
5. **提交**：`git commit -m "refactor: migrate xxx to new architecture"`
6. **合并**：合并到主分支

### 时间估算

| 阶段 | 工作量 | 预计时间 |
|------|--------|----------|
| 2.2 工具函数替换 | 10个函数 | 2-3小时 |
| 2.3 组件替换 | 4个组件 | 3-4小时 |
| 2.4 卡片组件 | 8个卡片 | 1-2天 |
| 2.5 页面组件 | 8个页面 | 3-5天 |
| **总计** | | **5-7天** |

### 风险控制

1. **每次只改一个**：不要同时迁移多个函数/组件
2. **充分测试**：每次迁移后都要测试相关功能
3. **保留备份**：可以创建 `main.tsx.backup`
4. **使用 Git**：每次迁移都提交，方便回滚
5. **团队协作**：多人并行时注意避免冲突

## 快速开始

### 立即可做：替换第一个工具函数

```bash
# 1. 编辑 main.tsx
# 取消注释这一行：
# import { errorMessage } from './utils';

# 2. 删除 main.tsx 中的 errorMessage 函数定义（约第220行）
# function errorMessage(error: unknown): string {
#   return error instanceof Error ? error.message : String(error);
# }

# 3. 验证构建
npm run build

# 4. 测试
npm run dev
```

### 下一步

完成第一个函数替换后，继续替换其他工具函数，按照优先级列表逐个进行。

## 参考资源

- `QUICK_START.md` - 新架构使用指南
- `CARD_MIGRATION_GUIDE.md` - 卡片组件迁移详细指南
- `PROJECT_STATUS.md` - 项目当前状态

## 需要帮助？

如果在迁移过程中遇到问题：
1. 查看相关文档
2. 检查 TypeScript 错误提示
3. 参考已完成的工具函数和组件
4. 保持小步快跑，每次只改一点

---

**当前状态**: ⏳ 准备就绪，可以开始阶段 2.2  
**建议**: 从替换 `errorMessage()` 开始
