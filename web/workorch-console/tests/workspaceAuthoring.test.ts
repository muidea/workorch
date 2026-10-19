import assert from 'node:assert/strict';
import test from 'node:test';
import { enterAuthoringWorkspace, AuthoringAcceptedPreparationObservation, AUTHORING_PREPARATION_ACCEPTED_OBSERVATION_TIMEOUT_MS, AuthoringQueryFence, mergeAuthoringObservation, AuthoringSessionFence, authoringCanEnterAdmitted, authoringCanConfirm, authoringCanReplayConfirmation, authoringConfirmationRecovery, authoringTicketDeliveryRecovery, authoringCanRecoverExact, authoringRequireNoPendingConfirmation, authoringNeedsSave, authoringNeedsPolling } from '../src/workspaceAuthoring.ts';
import type { DefinitionAuthoringView } from '../src/types.ts';

test('STEP03 materialized 缺票据的窄恢复不扩大通用澄清重试', () => {
  const exact = { inputHash: 'input', derivationPermitId: 'permit', proposalHash: 'proposal', specificationHash: 'spec', expectedWorkspaceRevision: 10 };
  const view = { ...ready(), workspaceRevision: 10, state: 'clarification_required', confirmationPhase: 'materialized', candidateHash: 'candidate', admissionDisposition: 'clarification_required', admissionViolationCode: 'profile_gap', confirmationRecovery: exact };
  assert.equal(authoringConfirmationRecovery(view, 'ws'), undefined);
  assert.deepEqual(authoringTicketDeliveryRecovery(view, 'ws'), exact);
  assert.equal(authoringCanRecoverExact(view, 'ws', { ...exact, expectedWorkspaceRevision: 8 }), true);
  for (const patch of [{ confirmationPhase: 'prepared' }, { candidateHash: '' }, { admissionDisposition: 'admitted_all_of' }, { admissionViolationCode: '' }, { templateCurrent: false }, { blockingTicketId: 'verified-ticket' }, { confirmationRecovery: { ...exact, expectedWorkspaceRevision: 11 } }, { confirmationRecovery: { ...exact, proposalHash: 'forged' } }]) {
    assert.equal(authoringTicketDeliveryRecovery({ ...view, ...patch }, 'ws'), undefined);
  }
  assert.equal(authoringTicketDeliveryRecovery(view, 'other'), undefined);
  assert.equal(authoringCanRecoverExact(view, 'ws', { ...exact, expectedWorkspaceRevision: NaN }), false);
});

test('父级刷新期间关闭或重开创作会话不得迟到导航', async () => {
  const session = new AuthoringSessionFence();
  const token = session.begin();
  let release!: () => void;
  const refreshed = new Promise<void>(resolve => { release = resolve; });
  const navigated: string[] = [];
  const pending = enterAuthoringWorkspace('ws', () => refreshed, () => session.matches(token), id => navigated.push(id));
  session.begin();
  release();
  await pending;
  assert.deepEqual(navigated, []);
  await enterAuthoringWorkspace('ws', async () => {}, () => true, id => navigated.push(id));
  assert.deepEqual(navigated, ['ws']);
});

test('查询代次失效与 effect 重建不产生重叠物理 GET', async () => {
  const queries = new AuthoringQueryFence();
  let release!: () => void;
  let active = 0;
  let maximum = 0;
  const first = queries.read(async () => {
    maximum = Math.max(maximum, ++active);
    await new Promise<void>(resolve => { release = resolve; });
    active--;
    return 'old';
  });
  queries.invalidate();
  const second = queries.read(async () => { maximum = Math.max(maximum, ++active); active--; return 'new'; });
  await Promise.resolve();
  assert.equal(active, 1);
  release();
  assert.deepEqual(await Promise.all([first, second]), ['old', 'new']);
  assert.equal(maximum, 1);
  await assert.rejects(queries.read(async () => { throw new Error('failed'); }));
  assert.equal(await queries.read(async () => 'recovered'), 'recovered');
});

test('关闭会话后排队查询不发 HTTP，新会话仍可读取', async () => {
  const queries = new AuthoringQueryFence();
  const session = new AuthoringSessionFence();
  const old = session.begin();
  let release!: () => void;
  const first = queries.read(() => new Promise<void>(resolve => { release = resolve; }));
  let calls = 0;
  const stale = Array.from({ length: 3 }, () => queries.read(async () => { calls++; }, () => session.matches(old)));
  const rejected = Promise.all(stale.map(pending => assert.rejects(pending, /查询已失效/)));
  const fresh = session.begin();
  const current = queries.read(async () => { calls++; return 'fresh'; }, () => session.matches(fresh));
  release();
  await first;
  await rejected;
  assert.equal(await current, 'fresh');
  assert.equal(calls, 1);
});

const ready = (): DefinitionAuthoringView => ({
  workspaceId: 'ws', workspaceRevision: 8, state: 'ready_to_confirm', templateCurrent: true,
  inputHash: 'input', derivationPermitId: 'permit',
  proposal: { goalText: '目标', items: [{ ref: 'item', text: '验收', sourceKind: 'DERIVED' }], proposalHash: 'proposal', specificationHash: 'spec', derivationPermitId: 'permit', reviews: ['dimension_coverage', 'negative_path', 'regression', 'resource_environment', 'traceability', 'verifier_feasibility'].map(kind => ({ kind, status: 'PASS', specificationHash: 'proposal' })) }
});

test('旧查询失败晚于写成功时不回写错误，准入中只观察至恢复出口', async () => {
  const queries = new AuthoringQueryFence();
  const query = queries.begin();
  let reject!: (reason: Error) => void;
  const response = new Promise<void>((_, fail) => { reject = fail; });
  let failure = '';
  const pending = response.catch(error => { if (queries.matches(query)) failure = error.message; });
  queries.invalidate();
  reject(new Error('过期查询失败'));
  await pending;
  assert.equal(failure, '');
  const admitting = { ...ready(), state: 'admitting' };
  assert.equal(authoringNeedsPolling(admitting), true);
  const admitted = mergeAuthoringObservation(admitting, { ...admitting, state: 'admitted', workspaceRevision: 12, activationState: 'eligible', startupInputHash: 'startup' });
  assert.equal(authoringNeedsPolling(admitted), false);
  assert.equal(authoringCanEnterAdmitted(admitted, 'ws'), true);
});

test('同会话旧查询不能覆盖写回执，revision 不倒退且接受标记不跨输入', async () => {
  const queries = new AuthoringQueryFence();
  const query = queries.begin();
  let resolve!: (value: DefinitionAuthoringView) => void;
  const pending = new Promise<DefinitionAuthoringView>(done => { resolve = done; });
  let view = ready();
  const observed = pending.then(next => { if (queries.matches(query)) view = mergeAuthoringObservation(view, next); });
  queries.invalidate();
  view = { ...ready(), state: 'admitted', workspaceRevision: 12 };
  resolve(ready());
  await observed;
  assert.equal(view.state, 'admitted');
  assert.equal(mergeAuthoringObservation(view, ready()), view);
  assert.equal(mergeAuthoringObservation(view, { ...ready(), workspaceId: 'other', workspaceRevision: 20 }), view);
  const accepted = { ...ready(), state: 'draft', compilationAccepted: true };
  assert.equal(mergeAuthoringObservation(accepted, { ...ready(), state: 'draft' }).compilationAccepted, true);
  assert.equal(mergeAuthoringObservation(accepted, { ...ready(), state: 'draft', workspaceRevision: 9, inputHash: 'new' }).compilationAccepted, undefined);
  const first = queries.begin();
  const second = queries.begin();
  assert.equal(queries.matches(first), false);
  assert.equal(queries.matches(second), true);
});

test('preparation_failed stops normal polling; explicit compile observation is session-local and bounded', () => {
  const diagnostic = {
    diagnosticId: 'diag', workspaceId: 'ws', inputHash: 'input', templateSnapshotHash: 'template', observedWorkspaceRevision: 8,
    stage: 'proposal' as const, targetRequestId: 'request', phaseCode: 'provider_unavailable', causeSHA256: 'hash', causeBytes: 0, causeSaved: false, current: true,
  };
  const failed = { ...ready(), state: 'preparation_failed', preparationDiagnostics: [diagnostic] };
  assert.equal(authoringNeedsPolling(failed), false);
  const observation = new AuthoringAcceptedPreparationObservation();
  observation.begin(failed, 10);
  assert.equal(authoringNeedsPolling(failed, true), true);
  assert.deepEqual(observation.observe({ ...failed }, 11), { active: true, freshLedger: false, exhausted: false });
  assert.deepEqual(observation.observe({ ...failed, preparationDiagnostics: [{ ...diagnostic, diagnosticId: 'new-diag' }] }, 20), { active: false, freshLedger: true, exhausted: false });
  observation.begin(failed, 100);
  assert.deepEqual(observation.observe(failed, 100 + AUTHORING_PREPARATION_ACCEPTED_OBSERVATION_TIMEOUT_MS), { active: false, freshLedger: false, exhausted: true });
  assert.equal(authoringNeedsPolling(failed), false);
});

test('准备观察独立截止幂等，重复 begin 只重置页面期限而不授权业务调用', () => {
  const failed = { ...ready(), state: 'preparation_failed' };
  const observation = new AuthoringAcceptedPreparationObservation();
  assert.equal(observation.isActive(), false);
  observation.begin(failed, 100);
  assert.equal(observation.isActive(), true);
  assert.equal(observation.expire(100 + AUTHORING_PREPARATION_ACCEPTED_OBSERVATION_TIMEOUT_MS - 1), false);
  observation.begin(failed, 5000);
  assert.equal(observation.expire(100 + AUTHORING_PREPARATION_ACCEPTED_OBSERVATION_TIMEOUT_MS), false);
  assert.equal(observation.expire(5000 + AUTHORING_PREPARATION_ACCEPTED_OBSERVATION_TIMEOUT_MS), true);
  assert.equal(observation.expire(5000 + AUTHORING_PREPARATION_ACCEPTED_OBSERVATION_TIMEOUT_MS), false);
  assert.equal(observation.observe(failed, 30_000).active, false);
  assert.equal(observation.observe(failed, 30_001).freshLedger, false);
  assert.equal(observation.observe({ ...failed, state: 'reviewing' }, 30_002).freshLedger, true);
  assert.equal(authoringNeedsPolling(failed), false);
  observation.begin(failed, 40_000);
  assert.equal(observation.observe({ ...failed, state: 'reviewing' }, 40_001).freshLedger, true);
  assert.equal(observation.expire(60_000), false);
});

test('same old current diagnostic after accepted compile does not stop polling before bounded deadline', () => {
  const oldFailure = { ...ready(), state: 'preparation_failed', preparationDiagnostics: [{
    diagnosticId: 'same-id', workspaceId: 'ws', inputHash: 'input', templateSnapshotHash: 'template', observedWorkspaceRevision: 8,
    stage: 'review' as const, targetRequestId: 'request', phaseCode: 'failed', causeSHA256: 'hash', causeBytes: 0, causeSaved: false, current: true,
  }] };
  const observation = new AuthoringAcceptedPreparationObservation();
  observation.begin(oldFailure, 1);
  const sameFailure = observation.observe(oldFailure, 1000);
  assert.equal(sameFailure.active, true);
  assert.equal(sameFailure.freshLedger, false);
  assert.equal(authoringNeedsPolling(oldFailure, sameFailure.active), true);
  assert.deepEqual(observation.observe({ ...oldFailure, state: 'reviewing' }, 1200), { active: false, freshLedger: true, exhausted: false });
});

test('preparation_failed 阻止确认；current=false 的历史诊断不阻止 later ready', () => {
  const diagnostic = {
    diagnosticId: 'diag', workspaceId: 'ws', inputHash: 'input', templateSnapshotHash: 'template', observedWorkspaceRevision: 8,
    stage: 'proposal' as const, targetRequestId: 'request', phaseCode: 'provider_unavailable', causeSHA256: 'hash', causeBytes: 0, causeSaved: false, current: true,
  };
  assert.equal(authoringCanConfirm({ ...ready(), preparationDiagnostics: [diagnostic] }), false);
  assert.equal(authoringCanConfirm({ ...ready(), preparationDiagnostics: [{ ...diagnostic, current: false }] }), true);
});

test('已准入恢复出口要求 exact Workspace、eligible 与 startup seal', () => {
  const admitted = { ...ready(), state: 'admitted', activationState: 'eligible', startupInputHash: 'startup' };
  assert.equal(authoringCanEnterAdmitted(admitted, 'ws'), true);
  assert.equal(authoringCanEnterAdmitted(admitted, 'other'), false);
  for (const patch of [{ state: 'admitting' }, { activationState: 'pending' }, { startupInputHash: '' }]) assert.equal(authoringCanEnterAdmitted({ ...admitted, ...patch }, 'ws'), false);
});

test('关闭重开后旧写回执不得修改新会话或清理恢复记录', async () => {
  const fence = new AuthoringSessionFence();
  const token = fence.begin();
  let release!: () => void;
  const response = new Promise<void>(resolve => { release = resolve; });
  let applied = false;
  const pending = response.then(() => { if (fence.matches(token)) applied = true; });
  fence.begin();
  const reopened = fence.begin();
  release();
  await pending;
  assert.equal(applied, false);
  assert.equal(fence.matches(reopened), true);
  assert.equal(fence.current(), reopened);
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
  for (const state of ['deriving', 'derived', 'reviewing', 'outcome_unresolved', 'confirming', 'admitting']) assert.equal(authoringNeedsPolling({ ...ready(), state }), true);
  assert.equal(authoringNeedsPolling(ready()), false);
});

test('服务端 confirmationRecovery 仅在 scope、阶段、四 hashes 和 current revision 精确匹配时可显式恢复', () => {
  const exact = { inputHash: 'input', derivationPermitId: 'permit', proposalHash: 'proposal', specificationHash: 'spec', expectedWorkspaceRevision: 8 };
  const view = { ...ready(), confirmationRecovery: exact };
  assert.deepEqual(authoringConfirmationRecovery(view, 'ws'), exact);
  assert.equal(authoringConfirmationRecovery(view), undefined);
  for (const patch of [
    { workspaceId: 'other' }, { state: 'draft' }, { templateCurrent: false },
    { workspaceRevision: 9 }, { inputHash: 'changed' }, { derivationPermitId: 'changed' },
    { proposal: { ...view.proposal, proposalHash: 'changed' } },
    { proposal: { ...view.proposal, specificationHash: 'changed' } },
    { confirmationRecovery: { ...exact, expectedWorkspaceRevision: 0 } },
    { confirmationRecovery: { ...exact, proposalHash: '' } },
  ]) assert.equal(authoringConfirmationRecovery({ ...view, ...patch }, 'ws'), undefined);
  assert.deepEqual(authoringConfirmationRecovery({ ...view, state: 'confirming' }, 'ws'), authoringConfirmationRecovery({ ...view, state: 'confirming', confirmationRecovery: exact }, 'ws'));
  assert.equal(authoringConfirmationRecovery({ ...view, state: 'admitting' }, 'ws')?.expectedWorkspaceRevision, 8);
  assert.equal(authoringConfirmationRecovery({ ...view, state: 'confirming', workspaceRevision: 9 }, 'ws')?.expectedWorkspaceRevision, 8);
});


test('原保存恢复后输入一致直接审查，不以 edited 或新请求消耗回执', () => {
 const input = { goalText: '目标', acceptanceRequirements: ['验收'], resourceRefs: ['input.txt'] };
 const view = { ...ready(), templateId: 't', input };
 assert.equal(authoringNeedsSave(view, input, 't'), false);
 assert.equal(authoringNeedsSave(view, { goalText: ' 目标 ', acceptanceRequirements: [' 验收 '], resourceRefs: ['input.txt'] }, ' t '), false);
 for (const changed of [{ ...input, goalText: '新目标' }, { ...input, acceptanceRequirements: ['新验收'] }, { ...input, resourceRefs: ['other.txt'] }]) assert.equal(authoringNeedsSave(view, changed, 't'), true);
 assert.equal(authoringNeedsSave(view, input, 'other'), true);
 assert.equal(authoringNeedsSave({ ...view, templateCurrent: false }, input, 't'), true);
 assert.equal(authoringNeedsSave({ ...view, input: undefined }, input, 't'), true);
});

test('未决确认阻止替代修订或审查，原 hashes 与 expected revision 不变', () => {
 const pendingConfirmation = { inputHash: 'input', derivationPermitId: 'permit', proposalHash: 'proposal', specificationHash: 'spec', expectedWorkspaceRevision: 8 };
 const recovery = { workspaceId: 'ws', pendingConfirmation };
 const original = JSON.stringify(recovery);
 assert.throws(() => authoringRequireNoPendingConfirmation(recovery), /先恢复确认/);
 assert.equal(JSON.stringify(recovery), original);
 assert.equal(authoringCanReplayConfirmation({ ...ready(), workspaceRevision: 12 }, recovery), true);
 authoringRequireNoPendingConfirmation(undefined);
 authoringRequireNoPendingConfirmation({ workspaceId: 'ws' });
});
