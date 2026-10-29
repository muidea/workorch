import type { LLMUsageSummary, LLMUsageProviderModelBreakdown } from './types.ts';

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
