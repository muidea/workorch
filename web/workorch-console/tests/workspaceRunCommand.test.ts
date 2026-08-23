import assert from 'node:assert/strict';
import test from 'node:test';

import type { RunCommandStatus } from '../src/types.ts';
import { presentWorkspaceRunCommand } from '../src/workspaceRunCommand.ts';

test('preserves accepted owner admission despite rejection-like request reason', () => {
  const command: RunCommandStatus = {
    workspaceId: 'ws-command',
    runId: 'run-command',
    state: 'accepted',
    intent: 'resume',
    requestId: 'request-command',
    requestReason: 'please_reject',
    source: 'web',
    outcome: 'reconciliation_requested',
    admissionReason: 'current_run_continuation_accepted',
    diagnostic: 'current Run continuation accepted',
    acceptedAt: '2026-08-23T00:00:00Z',
    revision: 7
  };

  assert.deepEqual(presentWorkspaceRunCommand(command), command);
});

test('preserves rejected owner outcome reason diagnostic and separate request reason', () => {
  const command: RunCommandStatus = {
    workspaceId: 'ws-command',
    state: 'rejected',
    intent: 'resume',
    requestId: 'request-command',
    requestReason: 'force_accept',
    source: 'web',
    outcome: 'restart_required',
    admissionReason: 'run_not_resumable',
    diagnostic: 'Run is not resumable',
    rejectedAt: '2026-08-23T00:00:00Z',
    revision: 8
  };

  const presented = presentWorkspaceRunCommand(command);
  assert.equal(presented?.state, 'rejected');
  assert.equal(presented?.outcome, 'restart_required');
  assert.equal(presented?.admissionReason, 'run_not_resumable');
  assert.equal(presented?.diagnostic, 'Run is not resumable');
  assert.equal(presented?.requestReason, 'force_accept');
});

test('does not invent a state for an incomplete projection', () => {
  assert.equal(presentWorkspaceRunCommand(undefined), undefined);
  assert.equal(presentWorkspaceRunCommand({ state: 'pending' } as unknown as RunCommandStatus), undefined);
});
