import assert from 'node:assert/strict';
import test from 'node:test';
import { creationRecoveryFromResponse, WorkorchRequestError } from '../src/creationRecovery.ts';

test('创建恢复提示区分已持久与未知，并要求原请求重放', () => {
  for (const persistence of ['unknown', 'persisted']) {
    const recovery = { workspaceId: 'ws', requestId: 'request', persistence, phase: 'directory_preparation', retrySameRequest: true };
    assert.deepEqual(creationRecoveryFromResponse({ code: 'workspace_creation_incomplete', creationRecovery: recovery }), recovery);
    const error = new WorkorchRequestError('待恢复', creationRecoveryFromResponse({ code: 'workspace_creation_incomplete', creationRecovery: recovery }));
    assert.equal(error.message, '待恢复');
    assert.equal(error.creationRecovery?.requestId, 'request');
  }
});

test('不完整或未知状态的创建恢复提示不能冒充合法回执', () => {
  const recovery = { workspaceId: 'ws', requestId: 'request', persistence: 'unknown', phase: 'owner_persistence', retrySameRequest: true };
  for (const patch of [{ workspaceId: '' }, { requestId: 3 }, { persistence: 'complete' }, { phase: 'executed' }, { retrySameRequest: false }]) {
    assert.equal(creationRecoveryFromResponse({ code: 'workspace_creation_incomplete', creationRecovery: { ...recovery, ...patch } }), undefined);
  }
  assert.equal(creationRecoveryFromResponse({ code: 'conflict', creationRecovery: recovery }), undefined);
  assert.equal(creationRecoveryFromResponse(null), undefined);
});
