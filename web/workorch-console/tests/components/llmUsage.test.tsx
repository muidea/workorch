import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { LLMUsageCard } from '../../src/components/cards/LLMUsageCard';
import { llmUsageCard } from '../../src/helpers';

const zero = {
  entryTotal: 0, inputTokens: 0, outputTokens: 0, totalTokens: 0,
  estimatedEntries: 0, providerNativeEntries: 0, failedEntries: 0,
};

describe('LLMUsageCard 查询范围', () => {
  it('缺少查询结果时不冒充零调用', () => {
    render(<LLMUsageCard />);
    expect(screen.getByText(/用量未加载/)).toBeTruthy();
    expect(screen.queryByText(/未记录/)).toBeNull();
  });

  it('已查询的空窗口明确说明范围而非当前 Run 无调用', () => {
    render(<LLMUsageCard summary={{ ...zero, availability: 'complete', scope: 'workspace', recordLimit: 200, observedRecords: 0 }} />);
    expect(screen.getByText(/已查询的 Workspace 用量窗口内没有记录/)).toBeTruthy();
    expect(screen.queryByText(/当前Run未记录/)).toBeNull();
  });

  it('详情页实际使用的 helper 同样不冒充零调用', () => {
    render(llmUsageCard({ ...zero, availability: 'not_loaded', scope: 'workspace', recordLimit: 200, observedRecords: 0 }));
    expect(screen.getByText(/用量未加载/)).toBeTruthy();
    expect(screen.queryByText(/未记录/)).toBeNull();
  });

  it('不可解析的窗口保留不完整提示，不把已加载事件当成功调用', () => {
    render(<LLMUsageCard summary={{ ...zero, availability: 'partial', scope: 'run', recordLimit: 200, observedRecords: 2 }} />);
    expect(screen.getByText(/用量数据不完整/)).toBeTruthy();
    expect(screen.queryByText(/未记录/)).toBeNull();
  });
});
