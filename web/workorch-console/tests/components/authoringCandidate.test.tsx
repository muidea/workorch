import { App, ConfigProvider } from 'antd';
import { StyleProvider } from '@ant-design/cssinjs';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { workorchApi } from '../../src/api';
import { WorkorchRequestError } from '../../src/creationRecovery';
import type { DefinitionAuthoringView } from '../../src/types';
import { WorkspaceAuthoringCandidate } from '../../src/pages/components/WorkspaceAuthoringCandidate';

vi.mock('../../src/api', () => ({ workorchApi: { readDefinitionAuthoring: vi.fn(), compileDefinitionAuthoring: vi.fn(), createDefinitionAuthoring: vi.fn(), saveDefinitionAuthoring: vi.fn(), confirmDefinitionAuthoring: vi.fn(), submitWorkspaceExecution: vi.fn() } }));
const draft = (id = 'ws', revision = 1): DefinitionAuthoringView => ({ workspaceId: id, workspaceRevision: revision, state: 'draft', inputRevision: revision, inputHash: `input-${revision}`, templateSnapshotHash: 'template-exact', templateId: 't', templateCurrent: true, input: { goalText: `原文\n目标-${revision}`, acceptanceRequirements: ['第一条', '第二条'] } });
const ready = (): DefinitionAuthoringView => ({ ...draft(), workspaceRevision: 20, state: 'ready_to_confirm', derivationPermitId: 'derive', proposal: {
  goalText: '原文\n目标-1', version: 1, goalItemRef: 'goal', goalTextHash: 'goal-hash', items: [{ ref: 'source', sourceKind: 'USER_EXPLICIT', text: '第一条' }], derivationPermitId: 'derive', proposalHash: 'proposal', specificationHash: 'reviewed',
  dimensions: [{ id: 'dimension', activated: true, required: true, resourceIds: [] }], requirements: [{ id: 'requirement', dimensionId: 'dimension', required: true, sourceKind: 'USER_EXPLICIT', sourceItemRefs: ['source'], resourceIds: [], text: '第一条' }], obligations: [{ id: 'obligation', requirementId: 'requirement', dodItemRefs: ['source'], text: '独立义务' }], verifications: [{ id: 'verification', obligationId: 'obligation', status: 'DECLARED', declaration: '诚实能力声明', profileId: 'generic', evidenceRequirementId: '*', scope: 'dodItem' }],
  reviews: ['dimension_coverage', 'negative_path', 'regression', 'resource_environment', 'traceability', 'verifier_feasibility'].map(kind => ({ kind, status: 'PASS', reason: `独立理由 ${kind}`, requirementIds: ['requirement'], evidenceRefs: ['source'], specificationHash: 'proposal' })),
} });
const wrap = (id?: string) => <StyleProvider container={document.createElement('div')}><ConfigProvider theme={{ hashed: false, token: { motion: false } }}><App><WorkspaceAuthoringCandidate workspaceId={id} onClose={vi.fn()} /></App></ConfigProvider></StyleProvider>;
const forbidden = () => { expect(workorchApi.createDefinitionAuthoring).not.toHaveBeenCalled(); expect(workorchApi.saveDefinitionAuthoring).not.toHaveBeenCalled(); expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled(); expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled(); };
const generateButton = () => screen.getByRole('button', { name: '生成已保存输入的合同候选' });
const recoveryButton = () => screen.getByRole('button', { name: '显式恢复前置准备' }) as HTMLButtonElement;
const preparationFailed = (revision = 2): DefinitionAuthoringView => ({ ...draft(), workspaceRevision: revision, state: 'preparation_failed', preparationDiagnostics: [{
  diagnosticId: `preparation-${revision}`, workspaceId: 'ws', inputHash: 'input-1', templateSnapshotHash: 'template-exact', observedWorkspaceRevision: revision,
  stage: 'derivation', targetRequestId: 'derive-request', phaseCode: 'provider_unavailable', causeSHA256: 'cause-hash', causeBytes: 0, causeSaved: false, current: true,
}] });
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(done => { resolve = done; }); return { resolve, promise }; }

describe('STEP02 只读候选正式入口', () => {
  beforeEach(() => { vi.resetAllMocks(); vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(draft()); });
  afterEach(() => { cleanup(); vi.useRealTimers(); });
  it('开页和刷新只读；显式生成提交精确前置身份，双击不重复', async () => {
    const pending = deferred<DefinitionAuthoringView>();
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockReturnValue(pending.promise);
    render(wrap('ws')); await act(async () => {});
    expect(workorchApi.compileDefinitionAuthoring).not.toHaveBeenCalled();
    expect(document.querySelector('textarea')).toBeNull();
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: '只读刷新' })); });
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(2);
    await act(async () => { fireEvent.click(generateButton()); fireEvent.click(generateButton()); });
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledExactlyOnceWith('ws', { expectedInputHash: 'input-1', expectedTemplateSnapshotHash: 'template-exact' });
    await act(async () => pending.resolve(ready()));
    expect(screen.getByText(/候选已达到 ready_to_confirm/)).toBeTruthy();
    expect(screen.getByLabelText('完整要求').textContent).toContain('requirement');
    expect(screen.getByLabelText('完整义务').textContent).toContain('独立义务');
    expect(screen.getByLabelText('逐义务验证声明').textContent).toContain('evidenceRequirement：*');
    expect(screen.getByLabelText('独立审查明细').textContent).toContain('verifier_feasibility');
    expect(screen.getByLabelText('独立审查明细').textContent).toContain('依据引用：source');
    forbidden();
  });
  it('202只表示受理，草稿不被当成成功；观察只有GET', async () => {
    vi.useFakeTimers();
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValue({ ...draft(), compilationAccepted: true });
    render(wrap('ws')); await act(async () => {});
    await act(async () => { fireEvent.click(generateButton()); });
    expect(screen.getByText(/任务已受理，不等于生成成功/)).toBeTruthy();
    expect(screen.queryByText(/候选已达到 ready_to_confirm/)).toBeNull();
    await act(async () => { await vi.advanceTimersByTimeAsync(1600); });
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(2);
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(1); forbidden();
  });
  it('首次受理后轮询读到准备失败，解除受理锁；仅显式恢复且保留精确输入身份', async () => {
    vi.useFakeTimers();
    const failed = preparationFailed();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(draft()).mockResolvedValue(failed);
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValueOnce({ ...draft(), compilationAccepted: true }).mockResolvedValueOnce({ ...failed, compilationAccepted: true });
    render(wrap('ws')); await act(async () => {});
    await act(async () => { fireEvent.click(generateButton()); });
    expect((generateButton() as HTMLButtonElement).disabled).toBe(true);
    await act(async () => { await vi.advanceTimersByTimeAsync(1500); });
    expect(recoveryButton().disabled).toBe(false);
    expect(screen.queryByText(/任务已受理，不等于生成成功/)).toBeNull();
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(2);
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(1);
    await act(async () => { fireEvent.click(recoveryButton()); fireEvent.click(recoveryButton()); });
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(2);
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenLastCalledWith('ws', { expectedInputHash: 'input-1', expectedTemplateSnapshotHash: 'template-exact' });
    expect(recoveryButton().disabled).toBe(true);
    await act(async () => { await vi.advanceTimersByTimeAsync(1500); });
    expect(recoveryButton().disabled).toBe(true); // 同一旧诊断不解除新受理锁。
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(preparationFailed(3));
    await act(async () => { await vi.advanceTimersByTimeAsync(1500); });
    expect(recoveryButton().disabled).toBe(false);
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(2); forbidden();
  });
  it.each([false, true])('首次compile直接返回准备失败（accepted=%s）仍允许显式恢复', async compilationAccepted => {
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValue({ ...preparationFailed(), compilationAccepted });
    render(wrap('ws')); await act(async () => {});
    await act(async () => { fireEvent.click(generateButton()); });
    expect(recoveryButton().disabled).toBe(false);
    expect(screen.queryByText(/任务已受理，不等于生成成功/)).toBeNull();
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(1); forbidden();
  });
  it('已从准备失败显式恢复时，观察窗到期及旧诊断刷新不授予再次受理', async () => {
    vi.useFakeTimers();
    const failed = preparationFailed();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(failed);
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValue({ ...failed, compilationAccepted: true });
    render(wrap('ws')); await act(async () => {});
    expect(recoveryButton().disabled).toBe(false);
    vi.mocked(workorchApi.readDefinitionAuthoring).mockRejectedValue(new Error('owner读取失败'));
    await act(async () => { fireEvent.click(recoveryButton()); });
    await act(async () => { await vi.advanceTimersByTimeAsync(15_000); });
    expect(screen.getByText(/前置准备观察窗结束/)).toBeTruthy();
    expect(recoveryButton().disabled).toBe(true);
    const reads = vi.mocked(workorchApi.readDefinitionAuthoring).mock.calls.length;
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(reads);
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(failed);
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: '只读刷新' })); });
    expect(recoveryButton().disabled).toBe(true);
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(preparationFailed(3));
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: '只读刷新' })); });
    expect(recoveryButton().disabled).toBe(false);
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(1); forbidden();
  });
  it('过期409只回读新版本，不自动重编或保存', async () => {
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(draft()).mockResolvedValue(draft('ws', 3));
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockRejectedValue(new WorkorchRequestError('输入版本冲突', undefined, 409));
    render(wrap('ws')); await act(async () => {});
    await act(async () => { fireEvent.click(generateButton()); });
    expect(screen.getByText('输入版本冲突')).toBeTruthy();
    expect(screen.getByText(/revision 3 · input-3/)).toBeTruthy();
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(1); forbidden();
  });
  it('关闭重开只读找回相同候选和六审', async () => {
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(ready());
    const mounted = render(wrap('ws')); await act(async () => {});
    mounted.rerender(wrap(undefined)); await act(async () => {});
    mounted.rerender(wrap('ws')); await act(async () => {});
    expect(screen.getByLabelText('独立审查明细').textContent).toContain('独立理由 traceability');
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(2);
    expect(workorchApi.compileDefinitionAuthoring).not.toHaveBeenCalled(); forbidden();
  });
  it('跨Workspace迟到查询及倒退revision不能污染当前结果', async () => {
    const old = deferred<DefinitionAuthoringView>();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockReturnValueOnce(old.promise).mockResolvedValue(draft('new', 5));
    const mounted = render(wrap('old')); await act(async () => {});
    mounted.rerender(wrap('new')); await act(async () => {});
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(1); // serial physical GET
    await act(async () => old.resolve(draft('old')));
    expect(screen.getByText(/revision 5 · input-5/)).toBeTruthy();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(draft('new', 2));
    await act(async () => fireEvent.click(screen.getByRole('button', { name: '只读刷新' })));
    expect(screen.getByText(/revision 5 · input-5/)).toBeTruthy(); forbidden();
  });
  it.each(['failed', 'outcome_unresolved', 'review_blocked'])('%s不提供统一重试逃生口', async state => {
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue({ ...ready(), state });
    render(wrap('ws')); await act(async () => {});
    expect((generateButton() as HTMLButtonElement).disabled).toBe(true);
    expect(workorchApi.compileDefinitionAuthoring).not.toHaveBeenCalled(); forbidden();
  });
});
