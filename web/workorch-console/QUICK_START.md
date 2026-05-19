# WorkOrch Web Console 新架构使用指南

## 快速开始

### 1. 使用工具函数

```typescript
// 导入工具函数
import { stateLabel, getStateColor, shortId, errorMessage } from './utils';

// 格式化状态
const label = stateLabel('running'); // "运行中 (running)"
const color = getStateColor('running'); // "blue"

// 截断 ID
const id = shortId('abc123def456ghi789', 8); // "abc123de..."

// 错误处理
try {
  await someApiCall();
} catch (error) {
  message.error(errorMessage(error));
}
```

### 2. 使用通用组件

```typescript
import { StateTag, MetricGrid, CompactTags, LoadingState } from './components/common';

// 状态标签
<StateTag state="running" />

// 指标网格
<MetricGrid items={[
  { key: 'total', label: '总数', value: 100, color: '#1668dc' },
  { key: 'completed', label: '已完成', value: 80, color: '#389e0d' }
]} />

// 紧凑标签
<CompactTags tags={['tag1', 'tag2', 'tag3']} maxVisible={2} />

// 加载状态
<LoadingState loading={loading} error={error}>
  <YourContent />
</LoadingState>
```

### 3. 使用 Hooks

```typescript
import { usePagination, useSSE, useDebounce } from './hooks';

// 分页
const { records, loading, hasMore, refresh, loadMore } = usePagination(
  async (cursor, limit) => {
    const page = await workorchApi.listRunSteps(runId, { cursorAfterId: cursor, limit });
    return {
      items: page.steps || [],
      nextCursor: page.nextCursorAfterId,
      hasMore: page.hasMore
    };
  },
  100
);

// SSE 事件流
useSSE((event) => {
  console.log('收到事件:', event.topic);
  if (event.topic === 'workspace.state_changed') {
    refresh();
  }
}, true);

// 防抖
const debouncedSearchTerm = useDebounce(searchTerm, 300);
```

### 4. 使用状态管理

```typescript
import { useWorkspaceStore } from './store/workspaceStore';

function MyComponent() {
  // 获取状态
  const workspaces = useWorkspaceStore((state) => state.workspaces);
  const selectedId = useWorkspaceStore((state) => state.selectedId);
  const loading = useWorkspaceStore((state) => state.loading);
  
  // 获取操作方法
  const loadWorkspaceList = useWorkspaceStore((state) => state.loadWorkspaceList);
  const selectWorkspace = useWorkspaceStore((state) => state.selectWorkspace);
  
  useEffect(() => {
    loadWorkspaceList();
  }, []);
  
  return (
    <Select
      value={selectedId}
      onChange={selectWorkspace}
      loading={loading}
    >
      {workspaces.map(ws => (
        <Select.Option key={ws.id} value={ws.id}>
          {ws.title}
        </Select.Option>
      ))}
    </Select>
  );
}
```

## 迁移示例

### 之前的代码

```typescript
// 重复的分页逻辑
const [runStepRecords, setRunStepRecords] = useState<StepView[]>([]);
const [runStepCursorAfterId, setRunStepCursorAfterId] = useState<string | undefined>();
const [runStepHasMore, setRunStepHasMore] = useState(false);
const [runStepLoading, setRunStepLoading] = useState(false);

async function loadRunStepPage(runId: string, cursorAfterId?: string) {
  setRunStepLoading(true);
  try {
    const page = await workorchApi.listRunSteps(runId, { cursorAfterId, limit: 100 });
    setRunStepRecords(prev => cursorAfterId ? mergeById(prev, page.steps || [], true) : page.steps || []);
    setRunStepCursorAfterId(page.nextCursorAfterId);
    setRunStepHasMore(page.hasMore);
  } finally {
    setRunStepLoading(false);
  }
}
```

### 现在的代码

```typescript
// 使用 usePagination Hook
const stepPagination = usePagination(
  async (cursor, limit) => {
    const page = await workorchApi.listRunSteps(runId, { cursorAfterId: cursor, limit });
    return {
      items: page.steps || [],
      nextCursor: page.nextCursorAfterId,
      hasMore: page.hasMore
    };
  },
  100
);

// 使用
<Button onClick={stepPagination.refresh}>刷新</Button>
<Button onClick={stepPagination.loadMore} disabled={!stepPagination.hasMore}>
  加载更多
</Button>
```

**代码减少**: 从 ~20 行减少到 ~10 行，减少 50%

## 最佳实践

### 1. 组件拆分

将大组件拆分为小组件，每个组件只负责一个功能：

```typescript
// ❌ 不好：一个组件做太多事情
function WorkspaceDetail() {
  // 500+ 行代码
}

// ✅ 好：拆分为多个小组件
function WorkspaceDetail() {
  return (
    <>
      <WorkspaceHeader />
      <WorkspaceTabs />
    </>
  );
}

function WorkspaceHeader() {
  // 50 行代码
}

function WorkspaceTabs() {
  // 100 行代码
}
```

### 2. 状态管理

使用 Zustand Store 管理全局状态，使用 useState 管理局部状态：

```typescript
// ✅ 全局状态：使用 Store
const workspaces = useWorkspaceStore((state) => state.workspaces);

// ✅ 局部状态：使用 useState
const [modalOpen, setModalOpen] = useState(false);
```

### 3. 数据获取

优先使用 usePagination Hook，避免重复代码：

```typescript
// ✅ 使用 Hook
const pagination = usePagination(fetcher, limit);

// ❌ 避免手动管理分页状态
const [records, setRecords] = useState([]);
const [cursor, setCursor] = useState();
// ...
```

### 4. 错误处理

使用统一的错误处理工具：

```typescript
import { errorMessage } from './utils';

try {
  await someOperation();
} catch (error) {
  message.error(errorMessage(error));
}
```

## 性能优化建议

### 1. 使用 memo

```typescript
import { memo } from 'react';

export const ExpensiveComponent = memo(({ data }) => {
  // 复杂的渲染逻辑
});
```

### 2. 使用 useMemo

```typescript
import { useMemo } from 'react';

const filteredData = useMemo(() => {
  return data.filter(item => item.state === 'active');
}, [data]);
```

### 3. 使用 useCallback

```typescript
import { useCallback } from 'react';

const handleClick = useCallback(() => {
  doSomething(id);
}, [id]);
```

## 常见问题

### Q: 如何在新架构中添加新功能？

A: 
1. 如果是通用功能，添加到 `utils/`、`hooks/` 或 `components/common/`
2. 如果是页面特定功能，添加到对应的 `pages/` 子目录
3. 如果需要全局状态，扩展或创建新的 Store

### Q: 原来的 main.tsx 还能用吗？

A: 可以。新架构与原代码可以共存，可以渐进式迁移。

### Q: 如何测试新组件？

A: 
```typescript
import { render, screen } from '@testing-library/react';
import { StateTag } from './StateTag';

test('renders state tag', () => {
  render(<StateTag state="running" />);
  expect(screen.getByText(/运行中/)).toBeInTheDocument();
});
```

## 下一步

1. 阅读 `REFACTOR_SUMMARY.md` 了解已完成的工作
2. 查看 `/home/rangh/.claude/plans/concurrent-snuggling-bumblebee.md` 了解完整计划
3. 开始提取卡片组件（任务 #6）
4. 逐步迁移页面组件

## 参考资源

- [Zustand 文档](https://github.com/pmndrs/zustand)
- [SWR 文档](https://swr.vercel.app/)
- [React Virtual 文档](https://tanstack.com/virtual/latest)
- [React 19 文档](https://react.dev/)
