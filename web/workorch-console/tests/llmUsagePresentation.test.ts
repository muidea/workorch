import assert from 'node:assert/strict';
import test from 'node:test';
import { llmUsageValue, llmCacheTokenValue, llmCacheReadRatio } from '../src/llmUsagePresentation.ts';
import type { LLMUsageSummary } from '../src/types.ts';

const zero: LLMUsageSummary = {
  availability: 'complete', scope: 'workspace', recordLimit: 200, observedRecords: 0,
  entryTotal: 0, inputTokens: 0, outputTokens: 0, totalTokens: 0,
  estimatedEntries: 0, providerNativeEntries: 0, failedEntries: 0,
};

test('未加载用量不显示零调用或零 Token', () => {
  assert.equal(llmUsageValue(undefined, 'entryTotal'), '未加载');
  assert.equal(llmUsageValue({ ...zero, availability: 'not_loaded' }, 'inputTokens'), '未加载');
});

test('已加载数值明确局限于查询窗口或不完整结果', () => {
  assert.equal(llmUsageValue(zero, 'entryTotal'), '0（查询窗口）');
  assert.equal(llmUsageValue({ ...zero, availability: 'partial', inputTokens: 8 }, 'inputTokens'), '8（不完整）');
});

test('缓存缺失、明确零、部分已知与读写分开展示', () => {
  const unknown = { ...zero, entryTotal: 6, observedRecords: 6 };
  assert.equal(llmCacheTokenValue(unknown, 'read'), '未知');
  assert.equal(llmCacheReadRatio(unknown), '未知');
  const knownZero = { ...unknown, cacheReadKnownEntries: 6, cacheReadTokens: 0 };
  assert.equal(llmCacheTokenValue(knownZero, 'read'), '0');
  assert.equal(llmCacheReadRatio(knownZero), '0.0%');
  assert.equal(llmCacheTokenValue(knownZero, 'write'), '未知');
  const partial = { ...unknown, cacheReadKnownEntries: 2, cacheReadTokens: 500, cacheReadInputRatio: 0.5 };
  assert.equal(llmCacheTokenValue(partial, 'read'), '500（部分已知）');
  assert.equal(llmCacheReadRatio(partial), '未知');
  const written = { ...knownZero, cacheWriteKnownEntries: 6, cacheWriteTokens: 1000 };
  assert.equal(llmCacheTokenValue(written, 'write'), '1,000');
});
