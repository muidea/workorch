import type { LLMUsageSummary, LLMUsageProviderModelBreakdown, NativeCacheSummary } from './types.ts';

export function llmUsageValue(summary: LLMUsageSummary | undefined, key: 'entryTotal' | 'inputTokens' | 'outputTokens' | 'totalTokens' | 'cacheReadTokens' | 'cacheWriteTokens'): string {
  if (!summary || summary.availability === 'not_loaded') return '未加载';
  if (key === 'cacheReadTokens' || key === 'cacheWriteTokens') {
    return `${llmCacheTokenValue(summary, key === 'cacheReadTokens' ? 'read' : 'write')}${summary.availability === 'complete' ? '（查询窗口）' : '（不完整）'}`;
  }
  const value = summary[key];
  if (value === undefined) return '-';
  return `${value}${summary.availability === 'complete' ? '（查询窗口）' : '（不完整）'}`;
}

type CacheSummary = LLMUsageSummary | LLMUsageProviderModelBreakdown;

export function llmCacheTokenValue(summary: CacheSummary, kind: 'read' | 'write'): string {
  const known = kind === 'read' ? summary.cacheReadKnownEntries : summary.cacheWriteKnownEntries;
  const value = (kind === 'read' ? summary.cacheReadTokens : summary.cacheWriteTokens) ?? 0;
  if (!known) return '未知';
  return `${value.toLocaleString()}${known < summary.entryTotal ? '（部分已知）' : ''}`;
}

export function llmCacheReadRatio(summary: CacheSummary): string {
  if (!summary.entryTotal || summary.cacheReadKnownEntries !== summary.entryTotal) return '未知';
  return `${((summary.cacheReadInputRatio ?? 0) * 100).toFixed(1)}%`;
}

export function nativeCacheSummaryItems(value?: NativeCacheSummary) {
  if (!value) return [];
  return [
    { key: 'physicalCacheCalls', label: '真实调用缓存观测', children: `${value.physicalCalls} 次提交 / ${value.measuredCalls} 次已测 / ${value.unknownCalls} 次未知` },
    { key: 'measuredCacheRatios', label: '已测调用命中 / 输入缓存占比', children: value.measuredCalls > 0 ? `${(value.hitRequestRatio * 100).toFixed(1)}% / ${(value.cachedInputTokenRatio * 100).toFixed(1)}%` : '未取得' },
    { key: 'uncachedInput', label: '已测未缓存输入', children: value.uncachedInputTokens.toLocaleString() }
  ];
}
