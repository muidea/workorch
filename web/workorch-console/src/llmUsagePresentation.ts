import type { LLMUsageSummary } from './types.ts';

export function llmUsageValue(summary: LLMUsageSummary | undefined, key: 'entryTotal' | 'inputTokens' | 'outputTokens' | 'totalTokens' | 'cacheReadTokens' | 'cacheWriteTokens'): string {
  if (!summary || summary.availability === 'not_loaded') return '未加载';
  const value = summary[key];
  if (value === undefined) return '-';
  return `${value}${summary.availability === 'complete' ? '（查询窗口）' : '（不完整）'}`;
}
