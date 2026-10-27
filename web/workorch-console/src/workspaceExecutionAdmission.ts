import type {
  RuntimeExecutionIntentOutcome,
  RuntimeExecutionIntentReason,
  WorkspaceExecutionAdmission,
  WorkspaceExecutionIntent
} from './types';

type WorkspaceExecutionAdmissionExpectation = {
  httpStatus: number;
  httpStatusText: string;
  workspaceId: string;
  requestId: string;
  intent: WorkspaceExecutionIntent;
};

const canonicalReasons: Record<RuntimeExecutionIntentOutcome, readonly RuntimeExecutionIntentReason[]> = {
  reconciliation_requested: [
    'current_run_continuation_accepted',
    'pending_run_activated',
    'new_run_activated'
  ],
  replacement_pending: [
    'duplicate_restart',
    'predecessor_supersede_requested',
    'terminal_predecessor_successor_requested'
  ],
  fresh_intent_unresolved: ['fresh_intent_result_unknown'],
  restart_required: [
    'workspace_active_run_absent',
    'selected_run_absent',
    'selected_run_terminal',
    'run_not_resumable'
  ],
  fresh_activation_required: [
    'runtime_bootstrap_missing',
    'definition_admission_missing',
    'definition_admission_mismatch',
    'preparation_creation_fence_missing'
  ],
  definition_changed_requires_restart: ['definition_changed'],
  governance_required: [
    'governance_pending',
    'definition_clarification_pending'
  ]
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isOptionalString(body: Record<string, unknown>, key: string) {
  return body[key] === undefined || typeof body[key] === 'string';
}

function responseError(body: unknown, expectation: WorkspaceExecutionAdmissionExpectation, violation: string) {
  if (isRecord(body) && typeof body.error === 'string' && body.error.trim()) {
    return new Error(body.error.trim());
  }
  const status = `${expectation.httpStatus} ${expectation.httpStatusText}`.trim();
  return new Error(`workspace execution admission contract violation (${status}): ${violation}`);
}

function canonicalClassification(outcome: RuntimeExecutionIntentOutcome) {
  const accepted = outcome === 'reconciliation_requested' || outcome === 'replacement_pending';
  return {
    accepted,
    state: accepted ? 'accepted' : outcome === 'fresh_intent_unresolved' ? 'pending' : 'rejected',
    httpStatus: accepted ? 202 : 409
  } as const;
}

export function parseWorkspaceExecutionAdmission(
  body: unknown,
  expectation: WorkspaceExecutionAdmissionExpectation
): WorkspaceExecutionAdmission {
  if (!isRecord(body)) {
    throw responseError(body, expectation, 'response body must be a JSON object');
  }
  if (typeof body.service !== 'string' || !body.service.trim()) {
    throw responseError(body, expectation, 'service identity is missing');
  }
  if (typeof body.workspaceId !== 'string' || !body.workspaceId.trim() ||
      typeof body.requestId !== 'string' || !body.requestId.trim()) {
    throw responseError(body, expectation, 'workspace/request identity is missing');
  }
  if (body.workspaceId !== expectation.workspaceId || body.requestId !== expectation.requestId) {
    throw responseError(body, expectation, 'workspace/request identity does not match the submitted command');
  }
  if ((body.intent !== 'resume' && body.intent !== 'restart') || body.intent !== expectation.intent) {
    throw responseError(body, expectation, 'intent identity does not match the submitted command');
  }
  if (typeof body.outcome !== 'string' || !Object.hasOwn(canonicalReasons, body.outcome)) {
    throw responseError(body, expectation, 'outcome is not canonical');
  }
  const outcome = body.outcome as RuntimeExecutionIntentOutcome;
  if (typeof body.reason !== 'string' || !canonicalReasons[outcome].includes(body.reason as RuntimeExecutionIntentReason)) {
    throw responseError(body, expectation, 'outcome/reason pairing is not canonical');
  }
  const classification = canonicalClassification(outcome);
  if (outcome === 'fresh_intent_unresolved') {
    if (body.intent !== 'restart' || body.runId !== undefined && body.runId !== '' ||
        typeof body.pendingRequestId !== 'string' || !body.pendingRequestId.trim() ||
        body.pendingRequestId !== body.pendingRequestId.trim() || new TextEncoder().encode(body.pendingRequestId).length > 256 ||
        /[\u0000\r\n]/.test(body.pendingRequestId) ||
        typeof body.pendingStartupInputHash !== 'string' || !body.pendingStartupInputHash.trim() ||
        body.pendingStartupInputHash !== body.pendingStartupInputHash.trim() || new TextEncoder().encode(body.pendingStartupInputHash).length > 256 ||
        typeof body.pendingPayloadHash !== 'string' || !/^[a-f0-9]{64}$/.test(body.pendingPayloadHash)) {
      throw responseError(body, expectation, 'unresolved original identity is invalid');
    }
  } else if (body.pendingRequestId !== undefined && body.pendingRequestId !== '' ||
             body.pendingStartupInputHash !== undefined && body.pendingStartupInputHash !== '' ||
             body.pendingPayloadHash !== undefined && body.pendingPayloadHash !== '') {
    throw responseError(body, expectation, 'resolved outcome carries an unresolved identity');
  }
  if (body.accepted !== classification.accepted || body.state !== classification.state) {
    throw responseError(body, expectation, 'accepted/state classification does not match the owner outcome');
  }
  if (expectation.httpStatus !== classification.httpStatus) {
    throw responseError(body, expectation, 'HTTP status does not match the owner outcome');
  }
  for (const key of ['backend', 'dataRoot', 'action', 'runId', 'requestReason', 'source', 'diagnostic', 'error']) {
    if (!isOptionalString(body, key)) {
      throw responseError(body, expectation, `${key} must be a string when present`);
    }
  }
  return body as WorkspaceExecutionAdmission;
}
