import assert from 'node:assert/strict';
import test from 'node:test';

import { parseWorkspaceExecutionAdmission } from '../src/workspaceExecutionAdmission.ts';

const canonicalCases = [
  ['reconciliation_requested', 'current_run_continuation_accepted', true, 'accepted', 202],
  ['reconciliation_requested', 'pending_run_activated', true, 'accepted', 202],
  ['reconciliation_requested', 'new_run_activated', true, 'accepted', 202],
  ['replacement_pending', 'duplicate_restart', true, 'accepted', 202],
  ['replacement_pending', 'predecessor_supersede_requested', true, 'accepted', 202],
  ['restart_required', 'workspace_active_run_absent', false, 'rejected', 409],
  ['restart_required', 'selected_run_absent', false, 'rejected', 409],
  ['restart_required', 'selected_run_terminal', false, 'rejected', 409],
  ['restart_required', 'run_not_resumable', false, 'rejected', 409],
  ['fresh_activation_required', 'runtime_bootstrap_missing', false, 'rejected', 409],
  ['fresh_activation_required', 'definition_admission_missing', false, 'rejected', 409],
  ['fresh_activation_required', 'definition_admission_mismatch', false, 'rejected', 409],
  ['definition_changed_requires_restart', 'definition_changed', false, 'rejected', 409],
  ['governance_required', 'governance_pending', false, 'rejected', 409],
  ['governance_required', 'definition_clarification_pending', false, 'rejected', 409]
] as const;

function expectation(httpStatus: number) {
  return {
    httpStatus,
    httpStatusText: httpStatus === 202 ? 'Accepted' : 'Conflict',
    workspaceId: 'ws-test',
    requestId: 'request-test',
    intent: 'resume' as const
  };
}

function admission(outcome: string, reason: string, accepted: boolean, state: string) {
  return {
    service: 'workorch',
    action: 'execution-activation',
    workspaceId: 'ws-test',
    runId: 'run-test',
    requestId: 'request-test',
    intent: 'resume',
    outcome,
    reason,
    accepted,
    state
  };
}

test('accepts all seven canonical owner admission classifications', () => {
  for (const [outcome, reason, accepted, state, httpStatus] of canonicalCases) {
    const body = admission(outcome, reason, accepted, state);
    assert.equal(parseWorkspaceExecutionAdmission(body, expectation(httpStatus)), body);
  }
});

test('treats an ordinary JSON 409 error as a transport/contract error', () => {
  assert.throws(
    () => parseWorkspaceExecutionAdmission({ error: 'workspace projection conflict' }, expectation(409)),
    /workspace projection conflict/
  );
});

test('rejects mismatched response identity', () => {
  const body = admission('reconciliation_requested', 'current_run_continuation_accepted', true, 'accepted');
  assert.throws(
    () => parseWorkspaceExecutionAdmission({ ...body, workspaceId: 'ws-other' }, expectation(202)),
    /identity does not match/
  );
  assert.throws(
    () => parseWorkspaceExecutionAdmission({ ...body, requestId: 'request-other' }, expectation(202)),
    /identity does not match/
  );
  assert.throws(
    () => parseWorkspaceExecutionAdmission({ ...body, intent: 'restart' }, expectation(202)),
    /intent identity does not match/
  );
});

test('rejects non-canonical outcome/reason pairings', () => {
  const body = admission('governance_required', 'run_not_resumable', false, 'rejected');
  assert.throws(
    () => parseWorkspaceExecutionAdmission(body, expectation(409)),
    /outcome\/reason pairing is not canonical/
  );
  assert.throws(
    () => parseWorkspaceExecutionAdmission({ ...body, outcome: 'toString' }, expectation(409)),
    /outcome is not canonical/
  );
});

test('rejects classifier and HTTP status mismatches', () => {
  const rejected = admission('restart_required', 'run_not_resumable', false, 'rejected');
  assert.throws(
    () => parseWorkspaceExecutionAdmission({ ...rejected, accepted: true }, expectation(409)),
    /accepted\/state classification/
  );
  assert.throws(
    () => parseWorkspaceExecutionAdmission({ ...rejected, state: 'accepted' }, expectation(409)),
    /accepted\/state classification/
  );
  assert.throws(
    () => parseWorkspaceExecutionAdmission(rejected, expectation(202)),
    /HTTP status does not match/
  );
});
