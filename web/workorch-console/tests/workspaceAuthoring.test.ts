import assert from 'node:assert/strict';
import test from 'node:test';
import { enterAuthoringWorkspace, AuthoringQueryFence, mergeAuthoringObservation, AuthoringSessionFence, authoringCanEnterAdmitted, authoringCanConfirm, authoringCanReplayConfirmation, authoringEditorValues, authoringRequireNoPendingConfirmation, authoringNeedsSave, authoringNeedsPolling, parseAuthoringEditorRecovery, parseAuthoringLocalDraft, restoreAuthoringRecovery, restoreAuthoringEditorRecovery, parseAuthoringRecovery } from '../src/workspaceAuthoring.ts';
import type { DefinitionAuthoringView } from '../src/types.ts';

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

test('损坏或旧保存恢复记录不能被当作新建空态', () => {
  assert.equal(restoreAuthoringRecovery(null), undefined);
  for (const text of ['', 'null', '{}', '{', JSON.stringify({ request: { requestId: 'original', title: '任务', templateId: 't', input: { goalText: '目标' } }, workspaceId: 'ws', pendingSave: { input: { goalText: '修订' }, templateId: 't', expectedInputHash: 'original', expectedWorkspaceRevision: 1 } })]) {
    assert.throws(() => restoreAuthoringRecovery(text), /禁止创建替代事务/);
  }
});

test('详情页损坏恢复保留原记录且不能作为空态继续提交', () => {
  assert.equal(restoreAuthoringEditorRecovery(null, 'ws'), undefined);
  for (const text of ['', 'null', '{}', '{', JSON.stringify({ workspaceId: 'other' }), JSON.stringify({ workspaceId: 'ws', pendingSave: { input: { goalText: '修订' }, templateId: 't', expectedInputHash: 'original', expectedWorkspaceRevision: 1 } })]) {
    assert.throws(() => restoreAuthoringEditorRecovery(text, 'ws'), /禁止提交替代事务/);
  }
  assert.deepEqual(restoreAuthoringEditorRecovery(JSON.stringify({ workspaceId: 'ws' }), 'ws'), { workspaceId: 'ws' });
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

test('本地未发送编辑允许空字段恢复，不修改初始创建身份', () => {
  const draft = { title: '', templateId: '新模板', goalText: '', requirements: '未保存验收', resources: 'input.txt' };
  assert.deepEqual(parseAuthoringLocalDraft(JSON.stringify(draft)), draft);
  for (const text of [null, '{}', 'null', '{', JSON.stringify({ ...draft, resources: [1] })]) assert.equal(parseAuthoringLocalDraft(text), undefined);
});

test('待保存请求保留原 CAS 身份，拒绝损坏及双重事务', () => {
  const saved = { request: { requestId: 'stable', title: '名称', templateId: 'template', input: { goalText: '原目标' } }, workspaceId: 'ws' };
  const pendingSave = { requestId: 'exact-save', input: { goalText: '修订目标', resourceRefs: ['input.txt'], acceptanceRequirements: ['验收'] }, templateId: 'next', expectedWorkspaceRevision: 8, expectedInputHash: 'old-input' };
  assert.deepEqual(parseAuthoringRecovery(JSON.stringify({ ...saved, pendingSave })), { ...saved, pendingSave });
  assert.equal(parseAuthoringRecovery(JSON.stringify({ request: saved.request, pendingSave })), undefined);
  assert.equal(parseAuthoringRecovery(JSON.stringify({ ...saved, pendingSave: { ...pendingSave, expectedInputHash: '' } })), undefined);
  assert.equal(parseAuthoringRecovery(JSON.stringify({ ...saved, pendingSave: { ...pendingSave, input: { goalText: '目标', resourceRefs: [1] } } })), undefined);
  assert.equal(parseAuthoringRecovery(JSON.stringify({ ...saved, pendingSave, pendingConfirmation: { inputHash: 'input', derivationPermitId: 'permit', proposalHash: 'proposal', specificationHash: 'spec', expectedWorkspaceRevision: 8 } })), undefined);
});

test('详情页恢复绑定工作空间，无本地副本时展示原 pendingSave 输入', () => {
  const pendingSave = { requestId: 'exact-save', input: { goalText: '修订目标', acceptanceRequirements: ['验收一', '验收二'], resourceRefs: ['input.txt'] }, templateId: 'next', expectedWorkspaceRevision: 8, expectedInputHash: 'old-input' };
  const saved = { workspaceId: 'ws', pendingSave };
  const parsed = parseAuthoringEditorRecovery(JSON.stringify(saved), 'ws');
  assert.deepEqual(parsed, saved);
  assert.deepEqual(authoringEditorValues(parsed!), { templateId: 'next', goalText: '修订目标', requirements: '验收一\n验收二', resources: 'input.txt' });
  const values = { templateId: '', goalText: '', requirements: '尚未发送的修改' };
  assert.deepEqual(authoringEditorValues({ ...saved, values }), values);
  assert.equal(parseAuthoringEditorRecovery(JSON.stringify(saved), 'other'), undefined);
  for (const text of ['null', '[]', '{}', '{', JSON.stringify({ ...saved, pendingSave: { ...pendingSave, expectedInputHash: '' } }), JSON.stringify({ ...saved, values: { ...values, resources: [1] } })]) assert.equal(parseAuthoringEditorRecovery(text, 'ws'), undefined);
});

test('详情页原确认直接使用共同事务合同，不需要伪造创建请求', () => {
  const pendingConfirmation = { inputHash: 'input', derivationPermitId: 'permit', proposalHash: 'proposal', specificationHash: 'spec', expectedWorkspaceRevision: 8 };
  const saved = { workspaceId: 'ws', pendingConfirmation };
  const parsed = parseAuthoringEditorRecovery(JSON.stringify(saved), 'ws');
  assert.deepEqual(parsed, saved);
  assert.equal(authoringCanReplayConfirmation(ready(), parsed), true);
  assert.equal(authoringCanReplayConfirmation({ ...ready(), inputHash: 'new' }, parsed), false);
  assert.equal(parseAuthoringEditorRecovery(JSON.stringify({ ...saved, pendingConfirmation: { ...pendingConfirmation, proposalHash: '' } }), 'ws'), undefined);
});

test('恢复保留发送前创建身份与原 payload，拒绝损坏记录', () => {
  const saved = { request: { requestId: 'stable', title: '名称', templateId: 'template', input: { goalText: '原目标' } }, workspaceId: 'ws' };
  assert.deepEqual(parseAuthoringRecovery(JSON.stringify(saved)), saved);
  assert.equal(parseAuthoringRecovery('{'), undefined);
  assert.equal(parseAuthoringRecovery('{}'), undefined);
  assert.equal(parseAuthoringRecovery(null), undefined);
  for (const patch of [{ pendingSave: null }, { pendingConfirmation: false }, { workspaceId: 1 }, { request: { ...saved.request, requestId: '' } }, { request: { ...saved.request, input: { goalText: '目标', resourceRefs: [1] } } }]) assert.equal(parseAuthoringRecovery(JSON.stringify({ ...saved, ...patch })), undefined);
  const pendingConfirmation = { inputHash: 'input', derivationPermitId: 'permit', proposalHash: 'proposal', specificationHash: 'spec', expectedWorkspaceRevision: 8 };
  assert.deepEqual(parseAuthoringRecovery(JSON.stringify({ ...saved, pendingConfirmation })), { ...saved, pendingConfirmation });
  assert.equal(parseAuthoringRecovery(JSON.stringify({ request: saved.request, pendingConfirmation })), undefined);
  assert.equal(parseAuthoringRecovery(JSON.stringify({ ...saved, pendingConfirmation: { ...pendingConfirmation, specificationHash: '' } })), undefined);
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
