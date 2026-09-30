import assert from 'node:assert/strict';
import test from 'node:test';
import { authoringCanConfirm, authoringCanReplayConfirmation, authoringNeedsPolling, parseAuthoringRecovery } from '../src/workspaceAuthoring.ts';
import type { DefinitionAuthoringView } from '../src/types.ts';

const ready = (): DefinitionAuthoringView => ({
  workspaceId: 'ws', workspaceRevision: 8, state: 'ready_to_confirm', templateCurrent: true,
  inputHash: 'input', derivationPermitId: 'permit',
  proposal: { goalText: '目标', items: [{ ref: 'item', text: '验收', sourceKind: 'DERIVED' }], proposalHash: 'proposal', specificationHash: 'spec', derivationPermitId: 'permit', reviews: ['dimension_coverage', 'negative_path', 'regression', 'resource_environment', 'traceability', 'verifier_feasibility'].map(kind => ({ kind, status: 'PASS', specificationHash: 'proposal' })) }
});

test('确认要求完整身份和六份 PASS，阻塞与过期模板不可确认', () => {
  assert.equal(authoringCanConfirm(ready()), true);
  for (const state of ['draft', 'review_blocked', 'outcome_unresolved', 'clarification_required']) assert.equal(authoringCanConfirm({ ...ready(), state }), false);
  assert.equal(authoringCanConfirm({ ...ready(), templateCurrent: false }), false);
  const missing = ready();
  delete missing.proposal!.specificationHash;
  assert.equal(authoringCanConfirm(missing), false);
  const duplicate = ready();
  duplicate.proposal!.reviews![1].kind = duplicate.proposal!.reviews![0].kind;
  assert.equal(authoringCanConfirm(duplicate), false);
  const failed = ready();
  failed.proposal!.reviews![0].status = 'UNKNOWN';
  assert.equal(authoringCanConfirm(failed), false);
});

test('确认拒绝非规范审查、过期提案、派生身份不一致和空验收', () => {
  const invented = ready();
  invented.proposal!.reviews![0].kind = 'invented';
  assert.equal(authoringCanConfirm(invented), false);
  const stale = ready();
  stale.proposal!.reviews![0].specificationHash = 'previous-proposal';
  assert.equal(authoringCanConfirm(stale), false);
  const mismatch = ready();
  mismatch.proposal!.derivationPermitId = 'previous-permit';
  assert.equal(authoringCanConfirm(mismatch), false);
  const empty = ready();
  empty.proposal!.items = [];
  assert.equal(authoringCanConfirm(empty), false);
});

test('刷新后仅允许相同工作空间和四份精确身份的原确认重放', () => {
  const view = ready();
  const recovery = { request: { requestId: 'stable', title: '名称', templateId: 'template', input: { goalText: '目标' } }, workspaceId: 'ws', pendingConfirmation: { inputHash: 'input', derivationPermitId: 'permit', proposalHash: 'proposal', specificationHash: 'spec', expectedWorkspaceRevision: 8 } };
  assert.equal(authoringCanReplayConfirmation(view, recovery), true);
  for (const state of ['confirming', 'admitting']) assert.equal(authoringCanReplayConfirmation({ ...view, state, workspaceRevision: 12 }, recovery), true);
  for (const state of ['draft', 'review_blocked', 'outcome_unresolved', 'clarification_required', 'admitted']) assert.equal(authoringCanReplayConfirmation({ ...view, state }, recovery), false);
  assert.equal(authoringCanReplayConfirmation({ ...view, inputHash: 'revision-2' }, recovery), false);
  assert.equal(authoringCanReplayConfirmation(view, { ...recovery, workspaceId: 'other' }), false);
  for (const field of ['inputHash', 'derivationPermitId', 'proposalHash', 'specificationHash'] as const) {
    assert.equal(authoringCanReplayConfirmation(view, { ...recovery, pendingConfirmation: { ...recovery.pendingConfirmation, [field]: 'stale' } }), false);
  }
  assert.equal(authoringCanReplayConfirmation(view, undefined), false);
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
  const pendingConfirmation = { inputHash: 'input', derivationPermitId: 'permit', proposalHash: 'proposal', specificationHash: 'spec', expectedWorkspaceRevision: 8 };
  assert.deepEqual(parseAuthoringRecovery(JSON.stringify({ ...saved, pendingConfirmation })), { ...saved, pendingConfirmation });
  assert.equal(parseAuthoringRecovery(JSON.stringify({ request: saved.request, pendingConfirmation })), undefined);
  assert.equal(parseAuthoringRecovery(JSON.stringify({ ...saved, pendingConfirmation: { ...pendingConfirmation, specificationHash: '' } })), undefined);
});
