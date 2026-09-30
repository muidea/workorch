import assert from 'node:assert/strict';
import test from 'node:test';
import { authoringCanConfirm, authoringNeedsPolling, parseAuthoringRecovery } from '../src/workspaceAuthoring.ts';
import type { DefinitionAuthoringView } from '../src/types.ts';

const ready = (): DefinitionAuthoringView => ({
  workspaceId: 'ws', workspaceRevision: 8, state: 'ready_to_confirm', templateCurrent: true,
  inputHash: 'input', derivationPermitId: 'permit',
  proposal: { goalText: '目标', items: [], proposalHash: 'proposal', specificationHash: 'spec', derivationPermitId: 'permit', reviews: Array.from({ length: 6 }, (_, index) => ({ kind: String(index), status: 'PASS' })) }
});

test('确认要求完整身份和六份 PASS，阻塞与过期模板不可确认', () => {
  assert.equal(authoringCanConfirm(ready()), true);
  for (const state of ['draft', 'review_blocked', 'outcome_unresolved', 'clarification_required']) assert.equal(authoringCanConfirm({ ...ready(), state }), false);
  assert.equal(authoringCanConfirm({ ...ready(), templateCurrent: false }), false);
  const missing = ready();
  delete missing.proposal!.specificationHash;
  assert.equal(authoringCanConfirm(missing), false);
  const failed = ready();
  failed.proposal!.reviews![0].status = 'UNKNOWN';
  assert.equal(authoringCanConfirm(failed), false);
});

test('已接受任务及持久中间阶段继续查询，不把未决当成功', () => {
  assert.equal(authoringNeedsPolling({ ...ready(), state: 'draft', compilationAccepted: true }), true);
  for (const state of ['deriving', 'derived', 'reviewing', 'outcome_unresolved']) assert.equal(authoringNeedsPolling({ ...ready(), state }), true);
  assert.equal(authoringNeedsPolling(ready()), false);
});

test('恢复保留发送前创建身份与原 payload，拒绝损坏记录', () => {
  const saved = { request: { requestId: 'stable', title: '名称', templateId: 'template', input: { goalText: '原目标' } }, workspaceId: 'ws' };
  assert.deepEqual(parseAuthoringRecovery(JSON.stringify(saved)), saved);
  assert.equal(parseAuthoringRecovery('{'), undefined);
  assert.equal(parseAuthoringRecovery('{}'), undefined);
  assert.equal(parseAuthoringRecovery(null), undefined);
});
