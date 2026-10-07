import assert from 'node:assert/strict';
import test from 'node:test';
import { llmUsageValue } from '../src/llmUsagePresentation.ts';
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
