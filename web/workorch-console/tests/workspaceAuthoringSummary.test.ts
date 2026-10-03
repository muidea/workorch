import assert from 'node:assert/strict';
import test from 'node:test';
import { authoringSummaryPresentation } from '../src/workspaceAuthoringSummary.ts';

test('创作摘要呈现已知失败阶段和审查数量', () => {
  assert.deepEqual(authoringSummaryPresentation({
    state: 'failed', failurePhase: 'proposal', failureCode: 'invalid_proposal', reviewCount: 2, inputRevision: 5,
  }), {
    label: '创作失败', color: 'red', type: 'error', failurePhase: '方案生成', failureCode: '方案无效', reviewCount: 2,
  });
});

test('未决只表示没有持久完成结果，不推断任务停止', () => {
  assert.equal(authoringSummaryPresentation({ state: 'outcome_unresolved', reviewCount: 0, inputRevision: 1 })?.label, '请求结果未决');
});

test('已完成审查不宣称当前确认资格', () => {
  assert.equal(authoringSummaryPresentation({ state: 'reviewed', reviewCount: 6, inputRevision: 1 })?.label, '审查已完成，确认资格待核对');
});

test('空状态不渲染，不将缺失摘要改写为草稿', () => {
  assert.equal(authoringSummaryPresentation(undefined), undefined);
  assert.equal(authoringSummaryPresentation({ state: '  ', reviewCount: 0, inputRevision: 1 }), undefined);
});

test('未知状态保持未知且不推断确认能力', () => {
  assert.deepEqual(authoringSummaryPresentation({ state: 'future_state', reviewCount: -1, inputRevision: 1 }), {
    label: '未知状态（future_state）', color: 'default', type: 'warning',
  });
});

test('仅有限的有效审查数量被展示', () => {
  assert.equal(authoringSummaryPresentation({ state: 'draft', reviewCount: Number.MAX_SAFE_INTEGER, inputRevision: 1 })?.reviewCount, Number.MAX_SAFE_INTEGER);
  assert.equal(authoringSummaryPresentation({ state: 'draft', reviewCount: 1.5, inputRevision: 1 })?.reviewCount, undefined);
});
