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
    'predecessor_supersede_requested'
  ],
  restart_required: [
    'workspace_active_run_absent',
    'selected_run_absent',
    'selected_run_terminal',
    'run_not_resumable'
  ],
  fresh_activation_required: [
    'runtime_bootstrap_missing',
    'definition_admission_missing',
    'definition_admission_mismatch'
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
    state: accepted ? 'accepted' : 'rejected',
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
