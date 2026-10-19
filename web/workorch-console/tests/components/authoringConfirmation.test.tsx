import { App, ConfigProvider } from 'antd';
import { StyleProvider } from '@ant-design/cssinjs';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { workorchApi } from '../../src/api';
import type { DefinitionAuthoringView } from '../../src/types';
import { WorkspaceAuthoringConfirmation } from '../../src/pages/components/WorkspaceAuthoringConfirmation';

vi.mock('../../src/api', () => ({ workorchApi: { readDefinitionAuthoring: vi.fn(), confirmDefinitionAuthoring: vi.fn(), createDefinitionAuthoring: vi.fn(), saveDefinitionAuthoring: vi.fn(), compileDefinitionAuthoring: vi.fn(), submitWorkspaceExecution: vi.fn() } }));
const exact = { inputHash: 'input', derivationPermitId: 'derive', proposalHash: 'proposal', specificationHash: 'specification', expectedWorkspaceRevision: 20 };
const ready = (id = 'ws', revision = 20): DefinitionAuthoringView => ({ workspaceId: id, workspaceRevision: revision, state: 'ready_to_confirm', templateCurrent: true, inputHash: 'input', derivationPermitId: 'derive', input: { goalText: '完整原文\n目标' }, proposal: {
  goalText: '完整原文\n目标', derivationPermitId: 'derive', proposalHash: 'proposal', specificationHash: 'specification', items: [{ ref: 'source', sourceKind: 'USER_EXPLICIT', text: '完整原准则' }],
  reviews: ['dimension_coverage', 'negative_path', 'regression', 'resource_environment', 'traceability', 'verifier_feasibility'].map(kind => ({ kind, status: 'PASS', specificationHash: 'proposal', reason: `独立审查-${kind}` })),
} });
const admitted = (): DefinitionAuthoringView => ({ ...ready(), workspaceRevision: 23, state: 'admitted', confirmationPhase: 'materialized', candidateHash: 'candidate', activationState: 'eligible', startupInputHash: 'startup' });
const blocked = (): DefinitionAuthoringView => ({ ...ready(), workspaceRevision: 22, state: 'clarification_required', confirmationPhase: 'materialized', candidateHash: 'candidate', admissionDisposition: 'clarification_required', admissionViolationCode: 'profile_gap', admissionOffendingRefs: ['source'], confirmationRecovery: { ...exact, expectedWorkspaceRevision: 22 } });
const wrap = (id?: string) => <StyleProvider container={document.createElement('div')}><ConfigProvider theme={{ hashed: false, token: { motion: false } }}><App><WorkspaceAuthoringConfirmation workspaceId={id} onClose={vi.fn()} /></App></ConfigProvider></StyleProvider>;
const confirmButton = () => screen.getByRole('button', { name: '确认当前精确合同并准入' }) as HTMLButtonElement;
const recoverButton = () => screen.getByRole('button', { name: '显式恢复原确认／票据交付' }) as HTMLButtonElement;
const review = () => fireEvent.click(screen.getByRole('checkbox'));
const forbidden = () => { for (const call of [workorchApi.createDefinitionAuthoring, workorchApi.saveDefinitionAuthoring, workorchApi.compileDefinitionAuthoring, workorchApi.submitWorkspaceExecution]) expect(call).not.toHaveBeenCalled(); };
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(done => { resolve = done; }); return { resolve, promise }; }

describe('STEP03 独立精确确认', () => {
  beforeEach(() => { vi.resetAllMocks(); vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(ready()); });
  afterEach(cleanup);
  it('开页刷新只读；审阅后仅五字段确认，双击单飞，成功独立回读', async () => {
    const post = deferred<DefinitionAuthoringView>();
    vi.mocked(workorchApi.confirmDefinitionAuthoring).mockReturnValue(post.promise);
    render(wrap('ws')); await act(async () => {});
    expect(confirmButton().disabled).toBe(true);
    expect(screen.getByLabelText('已保存 Goal 原文').textContent).toBe('完整原文\n目标');
    await act(async () => fireEvent.click(screen.getByText('只读刷新确认结果')));
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
    await act(async () => { review(); });
    await act(async () => { fireEvent.click(confirmButton()); fireEvent.click(confirmButton()); });
    expect(workorchApi.confirmDefinitionAuthoring).toHaveBeenCalledExactlyOnceWith(ready(), exact);
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(admitted());
    await act(async () => post.resolve(admitted()));
    expect(screen.getByText(/合同已确认、封存且 eligible/)).toBeTruthy();
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(3);
    expect(recoverButton().disabled).toBe(true); forbidden();
  });
  it('丢响应后GET对账，显式重放保持原revision；绝不自动POST', async () => {
    vi.mocked(workorchApi.confirmDefinitionAuthoring).mockRejectedValue(new Error('response lost'));
    render(wrap('ws')); await act(async () => {});
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue({ ...ready(), workspaceRevision: 21, state: 'admitting', confirmationPhase: 'materialized', confirmationRecovery: { ...exact, expectedWorkspaceRevision: 21 } });
    await act(async () => { review(); });
    await act(async () => fireEvent.click(confirmButton()));
    expect(workorchApi.confirmDefinitionAuthoring).toHaveBeenCalledTimes(1);
    expect(screen.getByLabelText('冻结的原确认意图').textContent).toContain('"expectedWorkspaceRevision": 20');
    await act(async () => { review(); });
    await act(async () => fireEvent.click(recoverButton()));
    expect(workorchApi.confirmDefinitionAuthoring).toHaveBeenLastCalledWith(expect.objectContaining({ workspaceRevision: 21 }), exact); forbidden();
  });
  it('409后漂移只读，不确认最新版本；倒退GET不解除阻塞或重置原意图', async () => {
    vi.mocked(workorchApi.confirmDefinitionAuthoring).mockRejectedValue(new Error('409 conflict'));
    render(wrap('ws')); await act(async () => {});
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue({ ...ready(), workspaceRevision: 24, inputHash: 'new-input' });
    await act(async () => { review(); }); await act(async () => fireEvent.click(confirmButton()));
    expect(recoverButton().disabled).toBe(true);
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(ready());
    await act(async () => fireEvent.click(screen.getByText('只读刷新确认结果')));
    expect(recoverButton().disabled).toBe(true);
    expect(screen.getByText('new-input')).toBeTruthy();
    expect(workorchApi.confirmDefinitionAuthoring).toHaveBeenCalledTimes(1); forbidden();
  });
  it('重开仅采纳owner精确恢复；票据缺失可显式补交且仍阻塞', async () => {
    const before = blocked();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(before);
    render(wrap('ws')); await act(async () => {});
    expect(screen.getByText(/票据交付未完成/)).toBeTruthy();
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
    const after = { ...before, blockingTicketId: 'same-ticket' };
    vi.mocked(workorchApi.confirmDefinitionAuthoring).mockResolvedValue(after);
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(after);
    await act(async () => { review(); }); await act(async () => fireEvent.click(recoverButton()));
    expect(workorchApi.confirmDefinitionAuthoring).toHaveBeenCalledExactlyOnceWith(before, before.confirmationRecovery);
    expect(screen.getByText('已验证 ticket：same-ticket')).toBeTruthy();
    expect(screen.queryByText(/合同已确认、封存且 eligible/)).toBeNull();
    expect(recoverButton().disabled).toBe(true); forbidden();
  });
  it.each([
    { confirmationRecovery: undefined }, { confirmationPhase: 'prepared' }, { candidateHash: '' },
    { confirmationRecovery: { ...exact, specificationHash: 'forged', expectedWorkspaceRevision: 22 } },
    { confirmationRecovery: { ...exact, expectedWorkspaceRevision: NaN } }, { templateCurrent: false },
  ])('异常窄恢复信息保持只读 %j', async patch => {
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue({ ...blocked(), ...patch });
    render(wrap('ws')); await act(async () => {});
    expect(confirmButton().disabled).toBe(true);
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled(); forbidden();
  });
  it('pending有startup也不是eligible；重开不自动恢复', async () => {
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue({ ...admitted(), state: 'admitting', activationState: 'pending_clarification_resolution', confirmationRecovery: { ...exact, expectedWorkspaceRevision: 23 } });
    render(wrap('ws')); await act(async () => {});
    expect(screen.getByText(/仍等待精确澄清 resolution/)).toBeTruthy();
    expect(screen.queryByText(/合同已确认、封存且 eligible/)).toBeNull();
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled(); forbidden();
  });
  it('未决原确认不能被同Workspace的另一个eligible版本冒充成功', async () => {
    vi.mocked(workorchApi.confirmDefinitionAuthoring).mockRejectedValue(new Error('lost'));
    render(wrap('ws')); await act(async () => {});
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue({ ...admitted(), inputHash: 'different' });
    await act(async () => { review(); }); await act(async () => fireEvent.click(confirmButton()));
    expect(screen.queryByText(/合同已确认、封存且 eligible/)).toBeNull();
    expect(recoverButton().disabled).toBe(true); forbidden();
  });
  it('迟到旧GET不能污染跨Workspace的新会话', async () => {
    const read = deferred<DefinitionAuthoringView>();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockReturnValueOnce(read.promise).mockResolvedValue(ready('new', 30));
    const mounted = render(wrap('old')); await act(async () => {});
    mounted.rerender(wrap('new')); await act(async () => {});
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(1);
    await act(async () => read.resolve(admitted()));
    expect(screen.getByText('new')).toBeTruthy();
    expect(screen.queryByText(/合同已确认、封存且 eligible/)).toBeNull();
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled(); forbidden();
  });
  it('迟到POST不可污染新Workspace会话，关闭重开只读', async () => {
    const post = deferred<DefinitionAuthoringView>();
    vi.mocked(workorchApi.confirmDefinitionAuthoring).mockReturnValue(post.promise);
    const mounted = render(wrap('ws')); await act(async () => {});
    await act(async () => { review(); }); await act(async () => fireEvent.click(confirmButton()));
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(ready('new'));
    mounted.rerender(wrap('new')); await act(async () => {});
    await act(async () => post.resolve(admitted()));
    expect(screen.queryByText(/合同已确认、封存且 eligible/)).toBeNull();
    expect(screen.getByText('new')).toBeTruthy();
    expect(confirmButton().disabled).toBe(true);
    mounted.rerender(wrap(undefined)); await act(async () => {});
    mounted.rerender(wrap('new')); await act(async () => {});
    expect(workorchApi.confirmDefinitionAuthoring).toHaveBeenCalledTimes(1); forbidden();
  });
});
