import assert from 'node:assert/strict';
import test from 'node:test';

import {
  canonicalDefinitionClarification,
  canonicalActiveRunIsGovernanceBlocked,
  governanceContinuationGuidance,
  governanceResourceKey,
  isDedicatedDefinitionDecision,
  isCanonicalApprovalMutationTarget,
  isCanonicalDecisionMutationTarget,
  mergeGovernanceResources,
  requireCanonicalDecisionMutationTargets,
  workspaceBoardExecutionIntentAllowed,
  workspaceDefinitionClarification,
  workspaceExecutionIntentAllowed
} from '../src/governanceMutationTarget.ts';
import type { Approval, Decision, WorkspaceSnapshot } from '../src/types.ts';

const decision: Decision = {
  id: 'ticket-canonical', workspaceId: 'ws-governance', runId: 'run-active', reason: 'input required', state: 'open'
};
const approval: Approval = {
  id: 'approval-canonical', workspaceId: 'ws-governance', runId: 'run-active', reason: 'approval required', state: 'pending'
};

function snapshot(blocker: 'decision' | 'approval'): WorkspaceSnapshot {
  return {
    workspace: { revision: 1, id: 'ws-governance', title: 'governance', deliverableDirectory: 'workspaces/governance', state: 'active', activeRunId: 'run-active' },
    taskProgress: {} as WorkspaceSnapshot['taskProgress'],
    acceptance: {} as WorkspaceSnapshot['acceptance'],
    llmContextBudgetSummary: {} as WorkspaceSnapshot['llmContextBudgetSummary'],
    llmUsageSummary: {} as WorkspaceSnapshot['llmUsageSummary'],
    runs: [{
      id: 'run-active', workspaceId: 'ws-governance',
      state: blocker === 'decision' ? 'blocked_on_decision' : 'blocked_on_approval',
      blockingTicketId: blocker === 'decision' ? decision.id : undefined,
      blockingApprovalId: blocker === 'approval' ? approval.id : undefined
    }],
    decisions: [decision],
    approvals: [approval],
    governanceDisplay: {
      promptMode: 'none', decisionId: 'ticket-forged-display', approvalId: 'approval-forged-display'
    }
  };
}

test('admits only the exact active Run blocker and ignores forged display hints', () => {
  assert.equal(isCanonicalDecisionMutationTarget(snapshot('decision'), decision), true);
  assert.equal(isCanonicalApprovalMutationTarget(snapshot('approval'), approval), true);
  assert.equal(isCanonicalDecisionMutationTarget(snapshot('decision'), { ...decision, id: 'ticket-extra' }), false);
  assert.equal(isCanonicalApprovalMutationTarget(snapshot('approval'), { ...approval, id: 'approval-extra' }), false);
});

test('rejects stale scope, absent concrete resources, and typed definition decisions', () => {
  const current = snapshot('decision');
  assert.equal(isCanonicalDecisionMutationTarget(current, { ...decision, runId: 'run-stale' }), false);
  assert.equal(isCanonicalDecisionMutationTarget({ ...current, decisions: [] }, decision), false);
  const definition = {
    ...decision,
    definitionClarification: {
      class: 'runtime_definition', bindingState: 'open', requiredAction: 'revise_definition_and_restart',
      violationOrGapCode: 'semantic_ambiguity', offendingRefs: ['goal:goal-1']
    }
  } satisfies Decision;
  assert.equal(isCanonicalDecisionMutationTarget({ ...current, decisions: [definition] }, definition), false);
  assert.equal(isDedicatedDefinitionDecision({ ...decision, decisionKind: 'runtime_definition_clarification' }), false);
  assert.equal(isDedicatedDefinitionDecision({ ...decision, failureClass: 'definition_admission' }), false);
});

test('selects a typed definition blocker only from the exact canonical owner intersection', () => {
  const current = snapshot('decision');
  const definition: Decision = {
    ...decision,
    decisionKind: 'runtime_definition_clarification',
    failureClass: 'runtime_definition_gap',
    definitionClarification: {
      class: 'runtime_definition', bindingState: 'open', requiredAction: 'revise_definition_and_restart',
      violationOrGapCode: 'implicit_one_of', offendingRefs: ['dod:item-1'], definitionRevision: 'definition-old',
      authoritySnapshotId: 'authority-1', clarificationDispatchId: 'dispatch-1', focusRef: 'focus-1'
    }
  };
  assert.equal(canonicalDefinitionClarification({ ...current, decisions: [definition] })?.id, definition.id);
  assert.equal(canonicalDefinitionClarification({
    ...current,
    decisions: [{ ...definition, id: 'ticket-extra' }],
    governanceDisplay: { promptMode: 'decision', decisionId: definition.id }
  }), undefined);
  assert.equal(canonicalDefinitionClarification({
    ...current,
    workspace: { ...current.workspace, activeRunId: 'run-other' },
    decisions: [definition]
  }), undefined);
  assert.equal(canonicalDefinitionClarification({
    ...current,
    decisions: [{ ...definition, definitionClarification: undefined }]
  }), undefined, 'legacy text classification cannot reconstruct typed clarification authority');
});

test('blocks client Resume while explicit definition Restart remains owner-adjudicated', () => {
  const decisionBlocked = snapshot('decision');
  const approvalBlocked = snapshot('approval');
  assert.equal(canonicalActiveRunIsGovernanceBlocked(decisionBlocked), true);
  assert.equal(workspaceExecutionIntentAllowed(decisionBlocked, 'resume'), false);
  assert.equal(workspaceExecutionIntentAllowed(approvalBlocked, 'resume'), false);
  assert.equal(workspaceBoardExecutionIntentAllowed(decisionBlocked, 'resume'), false);
  assert.equal(workspaceBoardExecutionIntentAllowed(approvalBlocked, 'resume'), false);
  assert.equal(workspaceExecutionIntentAllowed(decisionBlocked, 'restart'), true, 'ordinary governance keeps explicit replacement intent');

  const definition: Decision = {
    ...decision,
    definitionClarification: {
      class: 'runtime_definition', bindingState: 'open', requiredAction: 'revise_definition_and_restart',
      violationOrGapCode: 'semantic_ambiguity', offendingRefs: ['goal:goal-1'], definitionRevision: 'definition-old',
      authoritySnapshotId: 'authority-1', clarificationDispatchId: 'dispatch-1', focusRef: 'focus-1'
    }
  };
  const definitionBlocked = { ...decisionBlocked, decisions: [definition] };
  assert.equal(workspaceExecutionIntentAllowed(definitionBlocked, 'resume'), false);
  assert.equal(workspaceExecutionIntentAllowed(definitionBlocked, 'restart'), true);
  assert.equal(workspaceExecutionIntentAllowed({
    ...definitionBlocked,
    workspace: { ...definitionBlocked.workspace, revision: 999 },
    governanceDisplay: { promptMode: 'none', decisionId: 'display-changed' }
  }, 'restart'), true,
    'display changes do not enable or disable explicit Restart');
  assert.equal(workspaceBoardExecutionIntentAllowed(definitionBlocked, 'resume'), false);
  assert.equal(workspaceBoardExecutionIntentAllowed(definitionBlocked, 'restart'), false, 'Board always navigates for Definition clarification');
  assert.equal(workspaceExecutionIntentAllowed({
    ...definitionBlocked,
    decisions: [],
    governanceDisplay: { promptMode: 'decision', decisionId: definition.id }
  }, 'restart'), true, 'display-only clarification cannot suppress an explicit replacement intent');
});

test('pre-seal definition clarification offers only definition editing and no execution mutation', () => {
  const current = snapshot('decision');
  const preSeal: Decision = {
    ...decision,
    runId: '',
    definitionClarification: {
      class: 'workspace_definition_pre_seal', bindingState: 'open', requiredAction: 'revise_definition',
      violationOrGapCode: 'implicit_one_of', offendingRefs: ['dod:item-1']
    }
  };
  const preSealSnapshot = {
    ...current,
    workspace: { ...current.workspace, activeRunId: undefined },
    runs: [], decisions: [preSeal]
  };
  assert.equal(workspaceDefinitionClarification(preSealSnapshot)?.id, preSeal.id);
  assert.equal(workspaceExecutionIntentAllowed(preSealSnapshot, 'resume'), false);
  assert.equal(workspaceExecutionIntentAllowed(preSealSnapshot, 'restart'), false);
});

test('governance success guidance waits for owner continuation instead of scheduling Resume', () => {
  assert.match(governanceContinuationGuidance, /自动收敛/);
  assert.match(governanceContinuationGuidance, /delivery diagnostic/);
  assert.doesNotMatch(governanceContinuationGuidance, /恢复运行|\/run resume/i);
});

test('preflights an entire batch before returning admission', async () => {
  let reads = 0;
  await assert.rejects(
    requireCanonicalDecisionMutationTargets([decision, { ...decision, id: 'ticket-extra' }], async () => {
      reads += 1;
      return snapshot('decision');
    }),
    /未提交任何变更/
  );
  assert.equal(reads, 1);
});

test('keeps duplicate IDs from different workspaces distinct in rows and rejects ambiguous batch transport', async () => {
  const duplicate = { ...decision, workspaceId: 'ws-other' };
  const merged = mergeGovernanceResources([decision], [duplicate], true);
  assert.equal(merged.length, 2);
  assert.notEqual(governanceResourceKey(decision), governanceResourceKey(duplicate));
  await assert.rejects(
    requireCanonicalDecisionMutationTargets([decision, duplicate], async () => snapshot('decision')),
    /无法区分重复资源 ID/
  );
});
