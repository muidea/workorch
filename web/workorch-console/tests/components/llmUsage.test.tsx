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
  it('没有原生缓存用量时显示未知，明确零才显示零比例', () => {
    const summary = { ...zero, entryTotal: 1, inputTokens: 100, availability: 'complete' as const, scope: 'workspace' as const, recordLimit: 200, observedRecords: 1 };
    const view = render(<LLMUsageCard summary={summary} />);
    expect(screen.getAllByText('未知').length).toBeGreaterThanOrEqual(3);
    expect(screen.queryByText('0.0%')).toBeNull();
    view.rerender(<LLMUsageCard summary={{ ...summary, cacheReadKnownEntries: 1, cacheReadTokens: 0, cacheWriteKnownEntries: 1, cacheWriteTokens: 80 }} />);
    expect(screen.getByText('0.0%')).toBeTruthy();
    expect(screen.getByText('80')).toBeTruthy();
    expect(screen.queryByText('未知')).toBeNull();
  });

  it('详情 helper 保留部分已知缓存计数，比例仍未知', () => {
    render(llmUsageCard({ ...zero, entryTotal: 2, inputTokens: 200, availability: 'complete', scope: 'workspace', recordLimit: 200, observedRecords: 2, cacheReadKnownEntries: 1, cacheReadTokens: 100 }));
    expect(screen.getByText('100（部分已知）')).toBeTruthy();
    expect(screen.getAllByText('未知').length).toBeGreaterThanOrEqual(2);
  });

  for (const surface of ['card', 'detail'] as const) {
    it(`${surface} 分开展示已测调用命中率、token 比例及未知调用`, () => {
      const summary = { ...zero, entryTotal: 5, availability: 'partial' as const, scope: 'run' as const, recordLimit: 200, observedRecords: 5, nativeCache: {
        physicalCalls: 4, measuredCalls: 2, unknownCalls: 2, hitCalls: 1, measuredInputTokens: 300, cacheReadTokens: 80, uncachedInputTokens: 220, hitRequestRatio: 0.5, cachedInputTokenRatio: 80 / 300
      } };
      const view = render(surface === 'card' ? <LLMUsageCard summary={summary} /> : llmUsageCard(summary));
      expect(screen.getByText('4 次提交 / 2 次已测 / 2 次未知')).toBeTruthy();
      expect(screen.getByText('50.0% / 26.7%')).toBeTruthy();
      expect(screen.getByText('220')).toBeTruthy();
      view.rerender(surface === 'card' ? <LLMUsageCard summary={{ ...summary, nativeCache: { ...summary.nativeCache, measuredCalls: 0 } }} /> : llmUsageCard({ ...summary, nativeCache: { ...summary.nativeCache, measuredCalls: 0 } }));
      expect(screen.getByText('未取得')).toBeTruthy();
    });
  }

});
