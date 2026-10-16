import { StrictMode, useLayoutEffect } from 'react';
import { App as AntApp, ConfigProvider } from 'antd';
import { StyleProvider } from '@ant-design/cssinjs';
const App = AntApp;
import { act, fireEvent, render as mount, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import type { RenderOptions } from '@testing-library/react';

// jsdom 不验证 CSS 布局；真实组件照常生成样式，但注入独立容器，避免全局 selector 扫描。
const styleContainer = document.createElement('div');
const render = (ui: ReactNode, options?: RenderOptions) => mount(ui, {
  wrapper: ({ children }) => <StyleProvider container={styleContainer}><ConfigProvider theme={{ hashed: false, token: { motion: false } }} wave={{ disabled: true }}>{children}</ConfigProvider></StyleProvider>,
  ...options,
});
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createHash, webcrypto } from 'node:crypto';
import { workorchApi } from '../../src/api';
import { WorkorchRequestError } from '../../src/creationRecovery';
import { WorkspaceAuthoringEditor } from '../../src/pages/components/WorkspaceAuthoringEditor';
import { WorkspaceSetupModal } from '../../src/pages/components/WorkspaceSetupModal';
import { AuthoringDiagnosticDetails } from '../../src/pages/components/AuthoringDiagnosticDetails';
import { AuthoringPreparationDiagnosticDetails } from '../../src/pages/components/AuthoringPreparationDiagnosticDetails';
import type { DefinitionAuthoringPreparationDiagnostic, DefinitionAuthoringView } from '../../src/types';

vi.mock('../../src/api', () => ({ workorchApi: {
  listWorkspaceTemplates: vi.fn(), readDefinitionAuthoring: vi.fn(), readAuthoringDiagnostic: vi.fn(), readAuthoringPreparationDiagnostic: vi.fn(),
  createDefinitionAuthoring: vi.fn(), saveDefinitionAuthoring: vi.fn(),
  compileDefinitionAuthoring: vi.fn(), confirmDefinitionAuthoring: vi.fn(),
  submitWorkspaceExecution: vi.fn(),
} }));

const ready = (workspaceId = 'ws'): DefinitionAuthoringView => ({
  workspaceId, workspaceRevision: 8, inputRevision: 1, state: 'ready_to_confirm',
  templateId: 't', templateCurrent: true, input: { goalText: '原目标' },
  inputHash: 'input', derivationPermitId: 'permit',
  proposal: {
    goalText: '原目标', items: [{ ref: 'item', text: '验收', sourceKind: 'DERIVED' }],
    proposalHash: 'proposal', specificationHash: 'spec', derivationPermitId: 'permit',
    reviews: ['dimension_coverage', 'negative_path', 'regression', 'resource_environment', 'traceability', 'verifier_feasibility']
      .map(kind => ({ kind, status: 'PASS', specificationHash: 'proposal' })),
  },
});
function preparationDiagnostic(overrides: Partial<DefinitionAuthoringPreparationDiagnostic> = {}): DefinitionAuthoringPreparationDiagnostic {
  const cause = 'provider 原因：暂不可用';
  return {
    diagnosticId: 'preparation-diag', workspaceId: 'ws', inputHash: 'input', templateSnapshotHash: 'template-snapshot', observedWorkspaceRevision: 8,
    stage: 'proposal', targetRequestId: 'target-request', derivationPermitId: 'permit', reviewKind: 'dimension_coverage', proposalHash: 'proposal', phaseCode: 'provider_unavailable',
    resourceRef: 'input.txt', providerBindingHash: 'provider-binding',
    causeSHA256: createHash('sha256').update(cause).digest('hex'), causeBytes: Buffer.byteLength(cause), causeSaved: true, causeBase64: Buffer.from(cause).toString('base64'), current: true,
    ...overrides,
  };
}

function prepFailedView(diagnostics = [preparationDiagnostic()]): DefinitionAuthoringView {
  return { ...ready(), state: 'preparation_failed', preparationDiagnostics: diagnostics };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
const button = (name: string) => {
  const elements = Array.from(document.querySelectorAll<HTMLButtonElement>('button'))
    .filter(item => item.textContent === name && !item.closest('[aria-hidden="true"]'));
  if (elements.length !== 1) throw new Error(`应有唯一当前按钮：${name}，实际 ${elements.length}`);
  return elements[0];
};
const disabled = (name: string) => button(name).disabled;

async function fillSetup() {
  // 先结算模板读取，避免 MutationObserver 在表单重绘期间反复遍历整棵 Modal。
  await act(async () => {});
  expect(screen.getByLabelText('名称')).toBeTruthy();
  // 保留真实 Select 与必填校验；选择模板后，将同步输入合并在一次提交中。
  await act(async () => { fireEvent.mouseDown(document.querySelector('.ant-select-selector')!); });
  const templateOption = screen.getByText('模板', { selector: '.ant-select-item-option-content' });
  await act(async () => {
    fireEvent.click(templateOption);
    fireEvent.change(screen.getByLabelText('名称'), { target: { value: '测试工作空间' } });
    fireEvent.change(screen.getByLabelText('目标'), { target: { value: '原目标' } });
  });
}

async function downloadedCauseBytes(buttonName = '下载已校验原始字节'): Promise<Uint8Array> {
  let blob: Blob | undefined;
  vi.stubGlobal('URL', {
    createObjectURL: vi.fn((value: Blob) => { blob = value; return 'blob:diagnostic'; }),
    revokeObjectURL: vi.fn(),
  });
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
  fireEvent.click(button(buttonName));
  expect(blob).toBeInstanceOf(Blob);
  const buffer = await new Promise<ArrayBuffer>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as ArrayBuffer);
    reader.onerror = () => reject(reader.error);
    reader.readAsArrayBuffer(blob!);
  });
  return new Uint8Array(buffer);
}

beforeEach(() => {
  vi.restoreAllMocks();
  vi.resetAllMocks();
  vi.mocked(workorchApi.listWorkspaceTemplates).mockResolvedValue([{ id: 't', name: '模板' } as never]);
  vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(ready());
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockImplementation(async (workspaceId, permitId) => ({
    workspaceId, permitId, phase: 'proposal', failureCode: 'invalid_proposal', parseError: '默认解析错误',
    parseErrorSHA256: createHash('sha256').update('默认解析错误').digest('hex'), parseErrorBytes: Buffer.byteLength('默认解析错误'), parseErrorSaved: true,
    payloadBase64: '', payloadSHA256: createHash('sha256').digest('hex'), payloadBytes: 0, payloadSaved: true,
  }));
  vi.mocked(workorchApi.readAuthoringPreparationDiagnostic).mockImplementation(async (_workspaceId, diagnosticId) => ({
    ...preparationDiagnostic({ diagnosticId }), causeBase64: '',
  }));
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  expect((workorchApi as unknown as { submitWorkspaceExecution: ReturnType<typeof vi.fn> }).submitWorkspaceExecution).not.toHaveBeenCalled();
});

describe('持久规格编辑实际挂载', () => {
  it('确认前保留 Profile 来源全文及快照身份，不推测或自动确认', async () => {
    const view = ready();
    const original = '完整 Profile 交付准则\n包括原限定词和验收义务。';
    view.proposal!.evidencePurposeMappings = [{ contractRef: 'runtime/profile-deliverable-evidence-purpose/v2', historicalItemRef: 'profile/p/deliverable/d', acceptanceItemRef: 'original-dod', historicalProfileId: 'p', historicalDeliverableId: 'd', historicalProfileHash: 'original-snapshot', historicalText: original }];
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(view);
    render(<App><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn(async () => {})} /></App>);
    await waitFor(() => expect(screen.getByLabelText('跨 CWI 证据用途映射')).toBeTruthy());
    expect(screen.getByText('Profile p · deliverable d · 快照 original-snapshot')).toBeTruthy();
    expect(screen.getByText(original, { normalizer: value => value })).toBeTruthy();
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
  });
  it('确认前完整展示跨 CWI 历史用途映射，查看不产生确认或执行', async () => {
    const view = ready();
    view.proposal!.evidencePurposeMappings = [{ contractRef: 'runtime/cross-cwi-evidence-purpose/v1', historicalItemRef: 'original-create', acceptanceItemRef: 'successor-validation' }];
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(view);
    render(<App><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn(async () => {})} /></App>);
    await waitFor(() => expect(screen.getByLabelText('跨 CWI 证据用途映射')).toBeTruthy());
    expect(screen.getByText('历史行为 original-create → 当前验收 successor-validation · runtime/cross-cwi-evidence-purpose/v1')).toBeTruthy();
    expect(screen.getByText('跨 CWI 用途映射随本规格一并确认：前驱历史行为仅解锁精确后继，终验仍须全部原准则与真实当前普通证明。')).toBeTruthy();
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
  });
  it('确认前展示 exact 跨源映射，查看不自动确认', async () => {
    const view = ready();
    view.proposal!.executionProofMappings = [{ contractRef: 'runtime/cross-source-execution-proof/v1', consumerItemRef: 'consumer-dod', ruleRef: 'two-rounds', sourceItemRef: 'original-dod' }];
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(view);
    vi.mocked(workorchApi.confirmDefinitionAuthoring).mockResolvedValue({ ...view, state: 'admitted' });
    render(<App><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn(async () => {})} /></App>);
    await waitFor(() => expect(screen.getByLabelText('跨源执行证明映射')).toBeTruthy());
    expect(screen.getByText('consumer-dod · two-rounds → 原来源 original-dod · runtime/cross-source-execution-proof/v1')).toBeTruthy();
    expect(screen.getByText('跨源执行证明映射随本规格一并确认；它不代替原准则和普通工程验证。')).toBeTruthy();
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
  });
  it('Goal 来源映射保留完整原文，查看不产生确认或执行', async () => {
    const view = ready();
    const goalText = ' 完整 Goal\r\n两轮与修复，普通工程证明仍 mandatory。 ';
    view.proposal!.executionProofMappings = [{ contractRef: 'runtime/goal-source-execution-proof/v2', consumerItemRef: 'original-goal', ruleRef: 'two-rounds', sourceItemRef: 'original-dod', consumerGoalText: goalText }];
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(view);
    render(<App><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn(async () => {})} /></App>);
    await waitFor(() => expect(screen.getByLabelText('映射的完整 Goal 原文')).toBeTruthy());
    expect(screen.getByLabelText('映射的完整 Goal 原文').textContent).toBe(goalText);
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
  });
  it('已失败请求明确说明重放边界，查看不会自动编译或确认', async () => {
    const failed = { ...ready(), state: 'failed', proposal: undefined, requests: [{ phase: 'proposal', state: 'failed', failureCode: 'invalid_proposal', permitId: 'failed-permit' }] };
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(failed);
    await act(async () => {
      render(<App><WorkspaceAuthoringEditor open workspaceId="ws" onClose={() => {}} onAdmitted={async () => {}} /></App>);
    });
    expect(screen.getByText('原请求已失败；再次编译不会重发该请求')).toBeTruthy();
    expect(screen.getByText(/不要为了重试而制造无意义的输入变化/)).toBeTruthy();
    expect(disabled('确认并准入')).toBe(true);
    expect(workorchApi.compileDefinitionAuthoring).not.toHaveBeenCalled();
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
    expect(workorchApi.saveDefinitionAuthoring).not.toHaveBeenCalled();
  });

  it('新建向导收到持久失败后显示相同恢复边界且不自动再次编译', async () => {
    const failed: DefinitionAuthoringView = { ...ready(), state: 'failed', proposal: undefined };
    vi.mocked(workorchApi.createDefinitionAuthoring).mockResolvedValue({ ...failed, state: 'draft' });
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValue(failed);
    await act(async () => {
      render(<App><WorkspaceSetupModal open onClose={() => {}} onCreated={() => {}} /></App>);
    });
    await fillSetup();
    await act(async () => { fireEvent.click(button('保存并生成／恢复审查')); });
    expect(screen.getByText('原请求已失败；再次编译不会重发该请求')).toBeTruthy();
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(1);
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
    expect(disabled('确认规格并准入')).toBe(true);
  });

  it('显式 compile accepted 后同一旧前置失败保持有界观察，期限后停止且不声明成功', async () => {
    const oldFailure = prepFailedView();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(oldFailure);
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValueOnce({ ...oldFailure, compilationAccepted: true });
    render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    vi.useFakeTimers();
    await act(async () => { fireEvent.click(button('保存修订并审查')); });
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledWith('ws');
    expect(screen.getByText(/本次任务已受理，正在观察 owner 状态/)).toBeTruthy();
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(1);

    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(oldFailure);
    await act(async () => { await vi.advanceTimersByTimeAsync(1500); });
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(2);
    expect(screen.queryByText(/观察期限已到/)).toBeNull();
    expect(disabled('确认并准入')).toBe(true);

    await act(async () => { await vi.advanceTimersByTimeAsync(13_500); });
    expect(screen.getByText(/观察期限已到，最近一次可用 owner 状态为原前置准备失败/)).toBeTruthy();
    const callsAtDeadline = vi.mocked(workorchApi.readDefinitionAuthoring).mock.calls.length;
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(callsAtDeadline);
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
  });

  it('owner 返回 fresh preparation failure 立即停止 accepted poll 并展示新诊断', async () => {
    const oldFailure = prepFailedView();
    const nextFailure = prepFailedView([preparationDiagnostic({ diagnosticId: 'fresh-preparation-failure', phaseCode: 'new_failure' })]);
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(oldFailure);
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValueOnce({ ...oldFailure, compilationAccepted: true });
    render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    vi.useFakeTimers();
    await act(async () => { fireEvent.click(button('保存修订并审查')); });
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(1);
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(nextFailure);
    await act(async () => { await vi.advanceTimersByTimeAsync(1500); });
    expect(screen.getByText('当前前置准备失败 · proposal · dimension_coverage · new_failure')).toBeTruthy();
    expect(screen.queryByText(/正在观察 owner 状态/)).toBeNull();
    const callsAfterFailure = vi.mocked(workorchApi.readDefinitionAuthoring).mock.calls.length;
    await act(async () => { await vi.advanceTimersByTimeAsync(15_000); });
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(callsAfterFailure);
    expect(screen.queryByText(/观察期限已到/)).toBeNull();
  });

  it('GET 连续失败时独立截止结束观察，保留最后已知状态和读取原始错误', async () => {
    const oldFailure = prepFailedView();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(oldFailure);
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValueOnce({ ...oldFailure, compilationAccepted: true });
    render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    vi.useFakeTimers();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockRejectedValue(new Error('owner read 原始错误'));
    await act(async () => { fireEvent.click(button('保存修订并审查')); });
    await act(async () => { await vi.advanceTimersByTimeAsync(15_000); });
    expect(screen.getByText(/观察期限已到，最近一次可用 owner 状态为原前置准备失败/)).toBeTruthy();
    expect(screen.getByText('owner read 原始错误')).toBeTruthy();
    const callsAtDeadline = vi.mocked(workorchApi.readDefinitionAuthoring).mock.calls.length;
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(callsAtDeadline);
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(1);
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
  });

  it('挂起 GET 到期仍结束观察，迟到回执不修改视图或重新轮询', async () => {
    const oldFailure = prepFailedView();
    const pendingRead = deferred<DefinitionAuthoringView>();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(oldFailure).mockReturnValueOnce(pendingRead.promise);
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValueOnce({ ...oldFailure, compilationAccepted: true });
    render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    vi.useFakeTimers();
    await act(async () => { fireEvent.click(button('保存修订并审查')); });
    await act(async () => { await vi.advanceTimersByTimeAsync(1500); });
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(2);
    await act(async () => { await vi.advanceTimersByTimeAsync(13_500); });
    expect(screen.getByText(/观察期限已到/)).toBeTruthy();
    await act(async () => { pendingRead.resolve({ ...ready(), input: { goalText: '迟到读取' } }); });
    expect(screen.getByText('ws · revision 1 · preparation_failed')).toBeTruthy();
    expect(screen.queryByDisplayValue('迟到读取')).toBeNull();
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(2);
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(1);
  });

  it('重复 accepted compile 重建截止计时器，GET 连续失败时仍在新期限结束', async () => {
    const oldFailure = prepFailedView();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(oldFailure);
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValue({ ...oldFailure, compilationAccepted: true });
    render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    vi.useFakeTimers();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockRejectedValue(new Error('读取失败'));
    await act(async () => { fireEvent.click(button('保存修订并审查')); });
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    await act(async () => { fireEvent.click(button('保存修订并审查')); });
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(2);
    await act(async () => { await vi.advanceTimersByTimeAsync(10_000); });
    expect(screen.queryByText(/观察期限已到/)).toBeNull();
    expect(screen.getByText(/正在观察 owner 状态/)).toBeTruthy();
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    expect(screen.getByText(/观察期限已到/)).toBeTruthy();
    const callsAtDeadline = vi.mocked(workorchApi.readDefinitionAuthoring).mock.calls.length;
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(callsAtDeadline);
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(2);
  });

  it('新建向导挂起 GET 时独立截止结束观察，迟到旧视图不能恢复确认', async () => {
    const oldFailure = prepFailedView();
    const pendingRead = deferred<DefinitionAuthoringView>();
    vi.mocked(workorchApi.createDefinitionAuthoring).mockResolvedValueOnce(oldFailure);
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValueOnce({ ...oldFailure, compilationAccepted: true });
    vi.mocked(workorchApi.readDefinitionAuthoring).mockReturnValueOnce(pendingRead.promise);
    render(<AntApp><WorkspaceSetupModal open onClose={vi.fn()} onCreated={vi.fn()} /></AntApp>);
    await fillSetup();
    vi.useFakeTimers();
    await act(async () => { fireEvent.click(button('保存并生成／恢复审查')); });
    expect(screen.getByText(/正在观察 owner 状态/)).toBeTruthy();
    await act(async () => { await vi.advanceTimersByTimeAsync(1500); });
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(1);
    await act(async () => { await vi.advanceTimersByTimeAsync(13_500); });
    expect(screen.getByText(/观察期限已到/)).toBeTruthy();
    await act(async () => { pendingRead.resolve(ready()); });
    expect(screen.getByText('草稿：ws · revision 1 · preparation_failed')).toBeTruthy();
    expect(disabled('确认规格并准入')).toBe(true);
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(1);
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(1);
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
  });

  it('新建向导重复受理后按新期限截止，失败 GET 不触发替代创建或重发', async () => {
    const oldFailure = prepFailedView();
    vi.mocked(workorchApi.createDefinitionAuthoring).mockResolvedValueOnce(oldFailure);
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValue({ ...oldFailure, compilationAccepted: true });
    vi.mocked(workorchApi.readDefinitionAuthoring).mockRejectedValue(new Error('向导读取原始错误'));
    render(<AntApp><WorkspaceSetupModal open onClose={vi.fn()} onCreated={vi.fn()} /></AntApp>);
    await fillSetup();
    vi.useFakeTimers();
    await act(async () => { fireEvent.click(button('保存并生成／恢复审查')); });
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    await act(async () => { fireEvent.click(button('保存并生成／恢复审查')); });
    await act(async () => { await vi.advanceTimersByTimeAsync(10_000); });
    expect(screen.queryByText(/观察期限已到/)).toBeNull();
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    expect(screen.getByText(/观察期限已到/)).toBeTruthy();
    expect(screen.getByText('向导读取原始错误')).toBeTruthy();
    const callsAtDeadline = vi.mocked(workorchApi.readDefinitionAuthoring).mock.calls.length;
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(callsAtDeadline);
    expect(workorchApi.createDefinitionAuthoring).toHaveBeenCalledTimes(1);
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(2);
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
  });

  it('切换 Workspace 后旧准备观察截止不污染新会话', async () => {
    const oldFailure = prepFailedView();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(oldFailure);
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValueOnce({ ...oldFailure, compilationAccepted: true });
    const props = { open: true, onClose: vi.fn(), onAdmitted: vi.fn() };
    const mounted = render(<AntApp><WorkspaceAuthoringEditor {...props} workspaceId="ws" /></AntApp>);
    await screen.findByDisplayValue('原目标');
    vi.useFakeTimers();
    await act(async () => { fireEvent.click(button('保存修订并审查')); });
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue({ ...ready('new'), input: { goalText: '新会话目标' } });
    await act(async () => { mounted.rerender(<AntApp><WorkspaceAuthoringEditor {...props} workspaceId="new" /></AntApp>); });
    expect(screen.getByDisplayValue('新会话目标')).toBeTruthy();
    await act(async () => { await vi.advanceTimersByTimeAsync(15_000); });
    expect(screen.queryByText(/观察期限已到/)).toBeNull();
    expect(screen.getByText('new · revision 1 · ready_to_confirm')).toBeTruthy();
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(2);
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(1);
  });

  it('重新 compile 等待受理期间旧观察截止不推断新任务状态', async () => {
    const oldFailure = prepFailedView();
    const pendingCompile = deferred<DefinitionAuthoringView>();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(oldFailure).mockRejectedValue(new Error('读取失败'));
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValueOnce({ ...oldFailure, compilationAccepted: true }).mockReturnValueOnce(pendingCompile.promise);
    render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    vi.useFakeTimers();
    await act(async () => { fireEvent.click(button('保存修订并审查')); });
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    await act(async () => { fireEvent.click(button('保存修订并审查')); });
    await act(async () => { await vi.advanceTimersByTimeAsync(10_000); });
    expect(screen.queryByText(/观察期限已到/)).toBeNull();
    expect(screen.queryByText(/正在观察 owner 状态/)).toBeNull();
    await act(async () => { pendingCompile.resolve({ ...oldFailure, compilationAccepted: true }); });
    expect(screen.getByText(/正在观察 owner 状态/)).toBeTruthy();
    await act(async () => { await vi.advanceTimersByTimeAsync(15_000); });
    expect(screen.getByText(/观察期限已到/)).toBeTruthy();
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(2);
  });

  it('已受理准备观察关闭须确认，取消保留观察，丢弃后不注入截止提示', async () => {
    const oldFailure = prepFailedView();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(oldFailure);
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValueOnce({ ...oldFailure, compilationAccepted: true });
    const onClose = vi.fn();
    render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={onClose} onAdmitted={vi.fn()} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    vi.useFakeTimers();
    await act(async () => { fireEvent.click(button('保存修订并审查')); });
    await act(async () => { fireEvent.click(document.querySelector('.ant-modal-close')!); });
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getAllByText('关闭并丢弃页面内输入与重放身份？').length).toBeGreaterThan(0);
    await act(async () => { fireEvent.click(screen.getByText('继续编辑')); });
    expect(screen.getByText(/正在观察 owner 状态/)).toBeTruthy();
    await act(async () => { fireEvent.click(document.querySelector('.ant-modal-close')!); });
    await act(async () => { fireEvent.click(screen.getByText('关闭并丢弃')); });
    expect(onClose).toHaveBeenCalledTimes(1);
    await act(async () => { await vi.advanceTimersByTimeAsync(15_000); });
    expect(screen.queryByText(/观察期限已到/)).toBeNull();
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(1);
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(1);
  });

  it('prep failure current 阻止确认；仅保留历史记录时 ready 仍可确认', async () => {
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(prepFailedView());
    const mounted = render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    await screen.findByText(/当前前置准备失败/);
    expect(screen.getByText(/前置准备失败，尚未为该阶段取得 provider permit；修复后显式重试/)).toBeTruthy();
    expect(disabled('确认并准入')).toBe(true);
    expect(workorchApi.readAuthoringPreparationDiagnostic).not.toHaveBeenCalled();
    mounted.unmount();

    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce({ ...ready(), preparationDiagnostics: [preparationDiagnostic({ current: false })] });
    const historyOnly = render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    await screen.findByText(/历史前置准备失败/);
    expect(disabled('确认并准入')).toBe(false);
    historyOnly.unmount();
  });

  it('新建创作展示 owner 前置失败，不自动重发 compile 或启动 Run', async () => {
    const failed = prepFailedView();
    vi.mocked(workorchApi.createDefinitionAuthoring).mockResolvedValueOnce(failed);
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValueOnce(failed);
    render(<AntApp><WorkspaceSetupModal open onClose={vi.fn()} onCreated={vi.fn()} /></AntApp>);
    await fillSetup();
    await act(async () => { fireEvent.click(button('保存并生成／恢复审查')); });
    await screen.findByText(/当前前置准备失败/);
    expect(workorchApi.createDefinitionAuthoring).toHaveBeenCalledTimes(1);
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledTimes(1);
    expect(workorchApi.readDefinitionAuthoring).not.toHaveBeenCalled();
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    expect(workorchApi.readAuthoringDiagnostic).not.toHaveBeenCalled();
    expect(workorchApi.readAuthoringPreparationDiagnostic).not.toHaveBeenCalled();
  });

  it('prep cause 按需读取并校验；workspace 与不可变目标字段错配拒绝展示，current变化不丢已校验 bytes', async () => {
    browserCrypto();
    const cause = '原样前置原因：敏感 marker <raw>';
    const bytes = new TextEncoder().encode(cause);
    const expected = preparationDiagnostic({ causeSHA256: createHash('sha256').update(bytes).digest('hex'), causeBytes: bytes.byteLength });
    const raw = { ...expected, causeBase64: Buffer.from(bytes).toString('base64'), current: false };
    vi.mocked(workorchApi.readAuthoringPreparationDiagnostic).mockResolvedValue(raw);
    const mounted = render(<AuthoringPreparationDiagnosticDetails diagnostic={expected} />);
    expect(screen.queryByText(cause)).toBeNull();
    expect(workorchApi.readAuthoringPreparationDiagnostic).not.toHaveBeenCalled();
    fireEvent.click(button('按需加载并校验前置原因'));
    await waitFor(() => expect(workorchApi.readAuthoringPreparationDiagnostic).toHaveBeenCalledExactlyOnceWith('ws', 'preparation-diag'));
    expect(await screen.findByText((_, element) => element?.tagName === 'PRE' && element.textContent === cause)).toBeTruthy();
    expect(screen.queryByRole('img')).toBeNull();
    mounted.rerender(<AuthoringPreparationDiagnosticDetails diagnostic={{ ...expected, current: false }} />);
    expect(screen.getByText(/已校验原始字节/)).toBeTruthy();
  });

  it.each(Object.entries({
    workspaceId: 'other-workspace', diagnosticId: 'other-diagnostic', inputHash: 'other-input', templateSnapshotHash: 'other-template',
    observedWorkspaceRevision: 9, stage: 'review', targetRequestId: 'other-target', derivationPermitId: 'other-permit',
    reviewKind: 'negative_path', proposalHash: 'other-proposal', phaseCode: 'other-phase', resourceRef: 'other.txt', providerBindingHash: 'other-provider',
  }))('prep cause 身份字段 %s 错配时拒绝展示', async (field, value) => {
    browserCrypto();
    const expected = preparationDiagnostic();
    vi.mocked(workorchApi.readAuthoringPreparationDiagnostic).mockResolvedValueOnce({ ...expected, [field]: value });
    render(<AuthoringPreparationDiagnosticDetails diagnostic={expected} />);
    fireEvent.click(button('按需加载并校验前置原因'));
    await screen.findByText(/workspace、diagnostic、input、template、target、stage 或 phase 身份不匹配/);
    expect(screen.queryByRole('button', { name: '下载已校验原始字节' })).toBeNull();
  });

  it('prep cause 原 bytes 与已匹配 metadata 的 hash/bytes 不符时 fail closed', async () => {
    browserCrypto();
    const expected = preparationDiagnostic();
    vi.mocked(workorchApi.readAuthoringPreparationDiagnostic).mockResolvedValueOnce({ ...expected, causeBase64: Buffer.from('different cause').toString('base64') });
    const second = render(<AuthoringPreparationDiagnosticDetails diagnostic={expected} />);
    fireEvent.click(button('按需加载并校验前置原因'));
    await screen.findByText(/前置原因字节数不匹配/);
    expect(screen.queryByText(/provider 原因/)).toBeNull();
    second.unmount();

    const wrongHash = preparationDiagnostic({ causeSHA256: createHash('sha256').update('expected different cause').digest('hex') });
    vi.mocked(workorchApi.readAuthoringPreparationDiagnostic).mockResolvedValueOnce(wrongHash);
    const third = render(<AuthoringPreparationDiagnosticDetails diagnostic={wrongHash} />);
    fireEvent.click(button('按需加载并校验前置原因'));
    await screen.findByText(/前置原因 SHA-256 不匹配/);
    expect(workorchApi.compileDefinitionAuthoring).not.toHaveBeenCalled();
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    third.unmount();
  });

  it('UTF-8 BOM 在文本和下载原始字节中都保留', async () => {
    browserCrypto();
    const text = '﻿前置原文';
    const bytes = new TextEncoder().encode(text);
    const expected = preparationDiagnostic({ causeSHA256: createHash('sha256').update(bytes).digest('hex'), causeBytes: bytes.byteLength });
    vi.mocked(workorchApi.readAuthoringPreparationDiagnostic).mockResolvedValueOnce({ ...expected, causeBase64: Buffer.from(bytes).toString('base64') });
    render(<AuthoringPreparationDiagnosticDetails diagnostic={expected} />);
    fireEvent.click(button('按需加载并校验前置原因'));
    expect(await screen.findByText((_, element) => element?.tagName === 'PRE' && element.textContent === text)).toBeTruthy();
    expect(Array.from(await downloadedCauseBytes())).toEqual(Array.from(bytes));
  });

  it('零字节且 SHA-256(empty) 的已保存原因可按需校验并显示空文本', async () => {
    browserCrypto();
    const expected = preparationDiagnostic({ causeSHA256: createHash('sha256').digest('hex'), causeBytes: 0 });
    vi.mocked(workorchApi.readAuthoringPreparationDiagnostic).mockResolvedValueOnce({ ...expected, causeBase64: undefined });
    render(<AuthoringPreparationDiagnosticDetails diagnostic={expected} />);
    fireEvent.click(button('按需加载并校验前置原因'));
    await screen.findByText(/原始原因为空（0 字节）/);
    await screen.findByText('下载已校验原始字节');
    expect(Array.from(await downloadedCauseBytes())).toEqual([]);
    expect(screen.queryByText(/完整性校验失败/)).toBeNull();
  });

  it('非 UTF-8 前置原因不显示替换字符，保留原始 bytes 下载；超限未保存显式报告', async () => {
    browserCrypto();
    const bytes = new Uint8Array([0xff, 0xfe, 0x61]);
    const expected = preparationDiagnostic({ causeBytes: bytes.byteLength, causeSHA256: createHash('sha256').update(bytes).digest('hex') });
    vi.mocked(workorchApi.readAuthoringPreparationDiagnostic).mockResolvedValueOnce({ ...expected, causeBase64: Buffer.from(bytes).toString('base64') });
    const mounted = render(<AuthoringPreparationDiagnosticDetails diagnostic={expected} />);
    fireEvent.click(button('按需加载并校验前置原因'));
    await screen.findByText(/不是有效 UTF-8 文本；已验证原字节可下载/);
    expect(screen.queryByText(/�/)).toBeNull();
    expect(Array.from(await downloadedCauseBytes())).toEqual(Array.from(bytes));
    mounted.unmount();

    const unsaved = preparationDiagnostic({ causeSaved: false, notSavedReason: '超过明确存储边界' });
    render(<AuthoringPreparationDiagnosticDetails diagnostic={unsaved} />);
    expect(screen.getByText(/原始原因未保存：超过明确存储边界/)).toBeTruthy();
    expect(screen.queryByRole('button', { name: '按需加载并校验前置原因' })).toBeNull();
    expect(workorchApi.readAuthoringPreparationDiagnostic).toHaveBeenCalledTimes(1);
  });

  it('owner 发布新 proposal 时 memo 列表更新，不保留旧验收或审查', async () => {
    const initial = { ...ready(), state: 'reviewing' };
    const fresh = ready();
    fresh.proposal = {
      ...fresh.proposal!, items: [{ ref: 'new-item', text: '新验收条目', sourceKind: 'DERIVED' }],
      reviews: fresh.proposal!.reviews!.map(item => ({ ...item, reason: '新审查理由' })),
    };
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(initial).mockResolvedValue(fresh);
    vi.useFakeTimers();
    render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    await act(async () => {});
    expect(screen.getByText('验收')).toBeTruthy();
    await act(async () => { await vi.advanceTimersByTimeAsync(1500); });
    expect(screen.queryByText('验收')).toBeNull();
    expect(screen.getByText('新验收条目')).toBeTruthy();
    expect(screen.getAllByText(/新审查理由/)).toHaveLength(6);
    expect(workorchApi.compileDefinitionAuthoring).not.toHaveBeenCalled();
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
  });

  it('缺少任一独立审查不能确认', async () => {
    const incomplete = ready();
    incomplete.proposal!.reviews!.pop();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(incomplete);
    const props = { open: true, workspaceId: 'ws', onClose: vi.fn(), onAdmitted: vi.fn() };
    const mounted = render(<AntApp><WorkspaceAuthoringEditor {...props} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    await screen.findByText('ws · revision 1 · ready_to_confirm');
    expect(disabled('确认并准入')).toBe(true);
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
    mounted.unmount();
  });

  it('本地修改使完整旧规格不可确认', async () => {
    render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    expect(disabled('确认并准入')).toBe(false);
    fireEvent.change(screen.getByLabelText('目标'), { target: { value: '本地修订' } });
    expect(disabled('确认并准入')).toBe(true);
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
  });

  it('切换 workspace 后迟到 GET 不覆盖新会话', async () => {
    const old = deferred<DefinitionAuthoringView>();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockReturnValueOnce(old.promise).mockResolvedValueOnce({ ...ready('new'), input: { goalText: '新目标' } });
    const props = { open: true, onClose: vi.fn(), onAdmitted: vi.fn() };
    const mounted = render(<AntApp><WorkspaceAuthoringEditor {...props} workspaceId="old" /></AntApp>);
    await waitFor(() => expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledWith('old'));
    mounted.rerender(<AntApp><WorkspaceAuthoringEditor {...props} workspaceId="new" /></AntApp>);
    await act(async () => old.resolve({ ...ready('old'), input: { goalText: '迟到旧目标' } }));
    await screen.findByDisplayValue('新目标');
    expect(screen.queryByDisplayValue('迟到旧目标')).toBeNull();
  });

  it('切换 workspace 丢弃旧 scope 的 pendingSave，后续写入仅使用新 owner', async () => {
    const pendingOldSave = deferred<DefinitionAuthoringView>();
    vi.mocked(workorchApi.saveDefinitionAuthoring).mockReturnValueOnce(pendingOldSave.promise).mockResolvedValueOnce({ ...ready('new'), input: { goalText: '新修订' }, workspaceRevision: 9 });
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValue(ready('new'));
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(ready('old')).mockResolvedValueOnce({ ...ready('new'), input: { goalText: '新目标' } });
    const props = { open: true, onClose: vi.fn(), onAdmitted: vi.fn() };
    const mounted = render(<AntApp><WorkspaceAuthoringEditor {...props} workspaceId="old" /></AntApp>);
    await screen.findByDisplayValue('原目标');
    fireEvent.change(screen.getByLabelText('目标'), { target: { value: '旧修订' } });
    fireEvent.click(button('保存修订并审查'));
    await waitFor(() => expect(workorchApi.saveDefinitionAuthoring).toHaveBeenCalledTimes(1));
    mounted.rerender(<AntApp><WorkspaceAuthoringEditor {...props} workspaceId="new" /></AntApp>);
    await screen.findByDisplayValue('新目标');
    await act(async () => pendingOldSave.resolve({ ...ready('old'), inputRevision: 2, input: { goalText: '迟到旧保存' } }));
    fireEvent.change(screen.getByLabelText('目标'), { target: { value: '新修订' } });
    fireEvent.click(button('保存修订并审查'));
    await waitFor(() => expect(workorchApi.saveDefinitionAuthoring).toHaveBeenCalledTimes(2));
    const saveMock = vi.mocked(workorchApi.saveDefinitionAuthoring);
    expect(saveMock.mock.calls[0][0]).toBe('old');
    expect(saveMock.mock.calls[1][0]).toBe('new');
    expect(saveMock.mock.calls[1][3]).toBe(8);
    mounted.unmount();
  });

  it('关闭重开后迟到保存回执不覆盖 fresh 会话', async () => {
    const pending = deferred<DefinitionAuthoringView>();
    vi.mocked(workorchApi.saveDefinitionAuthoring).mockReturnValue(pending.promise);
    const props = { open: true, workspaceId: 'ws', onClose: vi.fn(), onAdmitted: vi.fn() };
    const mounted = render(<AntApp><WorkspaceAuthoringEditor {...props} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    fireEvent.change(screen.getByLabelText('目标'), { target: { value: '修订目标' } });
    fireEvent.click(button('保存修订并审查'));
    await waitFor(() => expect(workorchApi.saveDefinitionAuthoring).toHaveBeenCalledTimes(1));
    fireEvent.click(document.querySelectorAll('.ant-modal-close')[0]);
    expect(screen.getAllByText('关闭并丢弃页面内输入与重放身份？').length).toBeGreaterThan(0);
    fireEvent.click(screen.getByText('继续编辑'));
    expect(screen.getByDisplayValue('修订目标')).toBeTruthy();
    fireEvent.click(document.querySelectorAll('.ant-modal-close')[0]);
    fireEvent.click(screen.getByText('关闭并丢弃'));
    mounted.rerender(<AntApp><WorkspaceAuthoringEditor {...props} open={false} /></AntApp>);
    mounted.rerender(<AntApp><WorkspaceAuthoringEditor {...props} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    await act(async () => pending.resolve({ ...ready(), inputRevision: 2, input: { goalText: '迟到保存目标' } }));
    expect(screen.queryByDisplayValue('迟到保存目标')).toBeNull();
    expect(workorchApi.compileDefinitionAuthoring).not.toHaveBeenCalled();
  });

  it('StrictMode effect 重建不重叠 GET，卸载后不提交确认', async () => {
    const pending = deferred<DefinitionAuthoringView>();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockReturnValue(pending.promise);
    const mounted = render(<StrictMode><AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp></StrictMode>);
    await waitFor(() => expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledTimes(1));
    mounted.unmount();
    await act(async () => pending.resolve(ready()));
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
  });

  it('fresh editor open reads owner GET without consulting browser storage', async () => {
    const getSpy = vi.spyOn(Storage.prototype, 'getItem');
    const setSpy = vi.spyOn(Storage.prototype, 'setItem');
    const removeSpy = vi.spyOn(Storage.prototype, 'removeItem');
    localStorage.setItem('workorch.workspace-authoring.editor.v1/ws', '{');
    getSpy.mockClear(); setSpy.mockClear(); removeSpy.mockClear();
    render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    expect(workorchApi.readDefinitionAuthoring).toHaveBeenCalledWith('ws');
    expect(getSpy).not.toHaveBeenCalled();
    expect(setSpy).not.toHaveBeenCalled();
    expect(removeSpy).not.toHaveBeenCalled();
    getSpy.mockRestore(); setSpy.mockRestore(); removeSpy.mockRestore();
  });

  it('pendingSave 在内存中先按原 request/CAS 重放，再审查且不创建替代 workspace', async () => {
    vi.mocked(workorchApi.saveDefinitionAuthoring)
      .mockRejectedValueOnce(new Error('原保存响应丢失'))
      .mockResolvedValueOnce({ ...ready(), input: { goalText: '修订目标' }, workspaceRevision: 9, inputHash: 'new' });
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValue(ready());
    await act(async () => {
      render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    });
    await act(async () => {
      fireEvent.change(screen.getByLabelText('目标'), { target: { value: '修订目标' } });
      fireEvent.click(button('保存修订并审查'));
    });
    expect(screen.getByText('原保存响应丢失')).toBeTruthy();
    const saveMock = vi.mocked(workorchApi.saveDefinitionAuthoring);
    expect(saveMock).toHaveBeenCalledTimes(1);
    const original = saveMock.mock.calls[0];
    expect(original.slice(0, 5)).toEqual(['ws', { goalText: '修订目标', acceptanceRequirements: [], resourceRefs: [] }, 't', 8, 'input']);
    expect(original[5]).toBeTruthy();
    expect(workorchApi.compileDefinitionAuthoring).not.toHaveBeenCalled();
    expect(disabled('确认并准入')).toBe(true);
    await act(async () => { fireEvent.click(button('保存修订并审查')); });
    expect(saveMock).toHaveBeenCalledTimes(2);
    expect(saveMock.mock.calls[1]).toEqual(original);
    expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledExactlyOnceWith('ws');
    expect(workorchApi.createDefinitionAuthoring).not.toHaveBeenCalled();
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
  });

  it('内存 pendingConfirmation 存在时同 scope GET 不替换或丢弃原确认身份', async () => {
    const pending = deferred<DefinitionAuthoringView>();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(ready());
    vi.mocked(workorchApi.confirmDefinitionAuthoring).mockReturnValueOnce(pending.promise);
    const mounted = render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    await waitFor(() => expect(disabled('确认并准入')).toBe(false));
    fireEvent.click(button('确认并准入'));
    await waitFor(() => expect(workorchApi.confirmDefinitionAuthoring).toHaveBeenCalledTimes(1));
    mounted.rerender(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    expect(screen.getByDisplayValue('原目标')).toBeTruthy();
    expect(disabled('保存修订并审查')).toBe(true);
    expect(button('恢复原确认并准入')).toBeTruthy();
    const confirmMock = vi.mocked(workorchApi.confirmDefinitionAuthoring);
    expect(confirmMock.mock.calls[0][1]).toEqual({ inputHash: 'input', derivationPermitId: 'permit', proposalHash: 'proposal', specificationHash: 'spec', expectedWorkspaceRevision: 8 });
    mounted.unmount();
    await act(async () => pending.resolve({ ...ready(), state: 'confirming' }));
  });

  it('服务端精确 confirmationRecovery 允许显式恢复，未匹配时 fail closed 且不自动确认', async () => {
    const exact = { inputHash: 'input', derivationPermitId: 'permit', proposalHash: 'proposal', specificationHash: 'spec', expectedWorkspaceRevision: 8 };
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue({ ...ready(), state: 'confirming', confirmationRecovery: exact });
    vi.mocked(workorchApi.confirmDefinitionAuthoring).mockResolvedValue({ ...ready(), state: 'admitting' });
    const mounted = render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
    expect(disabled('恢复原确认并准入')).toBe(false);
    fireEvent.click(button('恢复原确认并准入'));
    await waitFor(() => expect(workorchApi.confirmDefinitionAuthoring).toHaveBeenCalledWith(expect.objectContaining({ state: 'confirming' }), exact));
    mounted.unmount();

    vi.resetAllMocks();
    vi.mocked(workorchApi.listWorkspaceTemplates).mockResolvedValue([{ id: 't', name: '模板' } as never]);
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue({ ...ready(), state: 'confirming', confirmationRecovery: { ...exact, proposalHash: 'wrong' } });
    const failClosed = render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    expect(disabled('确认并准入')).toBe(true);
    expect(disabled('保存修订并审查')).toBe(true);
    expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
    failClosed.unmount();
  });

  it('pendingConfirmation 原身份重放，关闭需确认且取消保留输入', async () => {
    vi.mocked(workorchApi.confirmDefinitionAuthoring).mockResolvedValue({ ...ready(), state: 'admitted', activationState: 'eligible', startupInputHash: 'sealed' });
    const refresh = deferred<void>();
    const onAdmitted = vi.fn(() => refresh.promise);
    const onClose = vi.fn();
    render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={onClose} onAdmitted={onAdmitted} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    fireEvent.click(button('确认并准入'));
    await waitFor(() => expect(onAdmitted).toHaveBeenCalledTimes(1));
    expect(workorchApi.confirmDefinitionAuthoring).toHaveBeenCalledWith(ready(), {
      inputHash: 'input', derivationPermitId: 'permit', proposalHash: 'proposal', specificationHash: 'spec', expectedWorkspaceRevision: 8,
    });
    fireEvent.click(document.querySelectorAll('.ant-modal-close')[0]);
    fireEvent.click(screen.getByText('继续编辑'));
    expect(screen.getByDisplayValue('原目标')).toBeTruthy();
    fireEvent.click(document.querySelectorAll('.ant-modal-close')[0]);
    fireEvent.click(screen.getByText('关闭并丢弃'));
    await act(async () => refresh.resolve());
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

it('已持久但目录失败的创建仍重放原 payload，不以 ID 查询跳过目录准备', async () => {
  vi.mocked(workorchApi.createDefinitionAuthoring).mockImplementationOnce(async request => {
    throw new WorkorchRequestError('目录准备失败', { workspaceId: 'ws', requestId: request.requestId, persistence: 'persisted', phase: 'directory_preparation', retrySameRequest: true });
  }).mockResolvedValueOnce({ ...ready(), state: 'draft', proposal: undefined });
  vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValue(ready());
  render(<App><WorkspaceSetupModal open onClose={vi.fn()} onCreated={vi.fn()} /></App>);
  await screen.findByLabelText('名称');
  fireEvent.change(screen.getByLabelText('名称'), { target: { value: '恢复工作空间' } });
  fireEvent.mouseDown(document.querySelector('.ant-select-selector')!);
  fireEvent.click(await screen.findByText('模板', { selector: '.ant-select-item-option-content' }));
  fireEvent.change(screen.getByLabelText('目标'), { target: { value: '原目标' } });
  await act(async () => { fireEvent.click(button('保存并生成／恢复审查')); });
  await screen.findByText('Workspace 身份已持久，创建准备待恢复');
  expect(workorchApi.compileDefinitionAuthoring).not.toHaveBeenCalled();
  fireEvent.click(button('保存并生成／恢复审查'));
  await screen.findByText('草稿：ws · revision 1 · ready_to_confirm');
  expect(workorchApi.createDefinitionAuthoring).toHaveBeenCalledTimes(2);
  expect(vi.mocked(workorchApi.createDefinitionAuthoring).mock.calls[1][0]).toEqual(vi.mocked(workorchApi.createDefinitionAuthoring).mock.calls[0][0]);
  expect(workorchApi.readDefinitionAuthoring).not.toHaveBeenCalled();
  expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
});

it('新建组件 fresh open 忽略旧 storage keys，内存 payload 经 create/save/confirm 重试保留且不自动启动 Run', async () => {
  localStorage.setItem('workorch.workspace-authoring.local-draft.v1', JSON.stringify({ title: '旧名称', templateId: 't', goalText: '旧目标' }));
  localStorage.setItem('workorch.workspace-authoring.v1', '{');
  const getSpy = vi.spyOn(Storage.prototype, 'getItem');
  const setSpy = vi.spyOn(Storage.prototype, 'setItem');
  const removeSpy = vi.spyOn(Storage.prototype, 'removeItem');
  getSpy.mockClear(); setSpy.mockClear(); removeSpy.mockClear();
  vi.mocked(workorchApi.createDefinitionAuthoring).mockRejectedValueOnce(new Error('模拟请求响应丢失')).mockResolvedValueOnce({ ...ready(), state: 'draft', proposal: undefined });
  vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValue(ready());
  vi.mocked(workorchApi.confirmDefinitionAuthoring).mockResolvedValue({ ...ready(), state: 'admitted', activationState: 'eligible', startupInputHash: 'sealed' });
  const onCreated = vi.fn();
  render(<App><WorkspaceSetupModal open onClose={vi.fn()} onCreated={onCreated} /></App>);
  await screen.findByLabelText('名称');
  expect(screen.queryByDisplayValue('旧名称')).toBeNull();
  const firstName = document.getElementById('title') as HTMLInputElement;
  fireEvent.change(firstName, { target: { value: '测试工作空间' } });
  fireEvent.mouseDown(document.querySelector('.ant-select-selector')!);
  fireEvent.click(await screen.findByText('模板', { selector: '.ant-select-item-option-content' }));
  fireEvent.change(screen.getByLabelText('目标'), { target: { value: '原目标' } });
  await act(async () => { fireEvent.click(button('保存并生成／恢复审查')); });
  await screen.findByText('模拟请求响应丢失');
  fireEvent.click(button('保存并生成／恢复审查'));
  await screen.findByText('草稿：ws · revision 1 · ready_to_confirm');
  expect(workorchApi.createDefinitionAuthoring).toHaveBeenCalledTimes(2);
  const firstPayload = vi.mocked(workorchApi.createDefinitionAuthoring).mock.calls[0][0];
  expect(vi.mocked(workorchApi.createDefinitionAuthoring).mock.calls[1][0]).toEqual(firstPayload);
  expect(onCreated).not.toHaveBeenCalled();
  expect(workorchApi.confirmDefinitionAuthoring).not.toHaveBeenCalled();
  await waitFor(() => expect(disabled('确认规格并准入')).toBe(false));
  fireEvent.click(button('确认规格并准入'));
  await waitFor(() => expect(onCreated).toHaveBeenCalledTimes(1));
  expect(workorchApi.confirmDefinitionAuthoring).toHaveBeenCalledWith(ready(), {
    inputHash: 'input', derivationPermitId: 'permit', proposalHash: 'proposal', specificationHash: 'spec', expectedWorkspaceRevision: 8,
  });
  expect(getSpy).not.toHaveBeenCalled();
  expect(setSpy).not.toHaveBeenCalled();
  expect(removeSpy).not.toHaveBeenCalled();
  getSpy.mockRestore(); setSpy.mockRestore(); removeSpy.mockRestore();
});

it('新建组件再次打开为 fresh 表单，不恢复旧 key 且不自动启动 Run', async () => {
  vi.mocked(workorchApi.createDefinitionAuthoring).mockResolvedValue({ ...ready(), state: 'draft', proposal: undefined });
  vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValue(ready());
  const onClose = vi.fn();
  const props = { open: true, onClose, onCreated: vi.fn() };
  const mounted = render(<App><WorkspaceSetupModal {...props} /></App>);
  await screen.findByLabelText('名称');
  fireEvent.change(screen.getByLabelText('名称'), { target: { value: '第一次编辑' } });
  fireEvent.click(document.querySelector('.ant-modal-close')!);
  fireEvent.click(screen.getByText('继续编辑'));
  fireEvent.click(document.querySelector('.ant-modal-close')!);
  fireEvent.click(screen.getByText('关闭并丢弃'));
  mounted.rerender(<App><WorkspaceSetupModal {...props} open={false} /></App>);
  mounted.rerender(<App><WorkspaceSetupModal {...props} /></App>);
  await screen.findByLabelText('名称');
  expect(screen.getByLabelText('名称').getAttribute('value')).not.toBe('第一次编辑');
  expect(workorchApi.createDefinitionAuthoring).not.toHaveBeenCalled();
});

const diagnosticView = (withMetadata = true): DefinitionAuthoringView => ({
  ...ready(), state: 'failed',
  requests: [{ phase: 'proposal', state: 'failed', failureCode: 'invalid_proposal', permitId: 'permit raw/?', ...(withMetadata ? { diagnostic: {
    parseError: '解析错误\\n敏感标记: <img src=x onerror=alert(1)>\\n结束',
    parseErrorSHA256: createHash('sha256').update('解析错误\\n敏感标记: <img src=x onerror=alert(1)>\\n结束').digest('hex'),
    parseErrorBytes: Buffer.byteLength('解析错误\\n敏感标记: <img src=x onerror=alert(1)>\\n结束'), parseErrorSaved: true,
    payloadSHA256: '', payloadBytes: 0, payloadSaved: true,
  } } : {}) }],
});

function FirstCommitObservation({ children, observe }: { children: ReactNode; observe: () => void }) {
  useLayoutEffect(observe, [observe]);
  return children;
}

function browserCrypto() {
  vi.stubGlobal('crypto', webcrypto);
}

it('UTF-8 BOM 在规格原始响应文本和下载原始字节中都保留', async () => {
  browserCrypto();
  const text = '﻿规格原文';
  const bytes = new TextEncoder().encode(text);
  const diagnostic = {
    workspaceId: 'ws', permitId: 'permit', phase: 'proposal', failureCode: 'invalid_proposal',
    parseError: '', parseErrorSHA256: createHash('sha256').digest('hex'), parseErrorBytes: 0, parseErrorSaved: true,
    payloadBase64: Buffer.from(bytes).toString('base64'), payloadSHA256: createHash('sha256').update(bytes).digest('hex'),
    payloadBytes: bytes.byteLength, payloadSaved: true,
  };
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(diagnostic);
  render(<AuthoringDiagnosticDetails workspaceId="ws" permitId="permit" phase="proposal" failureCode="invalid_proposal" diagnostic={diagnostic} />);
  fireEvent.click(button('按需加载并校验诊断原文'));
  expect(await screen.findByText((_, element) => element?.tagName === 'PRE' && element.textContent === text)).toBeTruthy();
  expect(Array.from(await downloadedCauseBytes())).toEqual(Array.from(bytes));
});

it('非 UTF-8 原始规格响应拒绝文本但仍可下载原始 bytes', async () => {
  browserCrypto();
  const bytes = new Uint8Array([0xff, 0xfe, 0x61]);
  const diagnostic = {
    workspaceId: 'ws', permitId: 'permit', phase: 'proposal', failureCode: 'invalid_proposal',
    parseError: '', parseErrorSHA256: createHash('sha256').digest('hex'), parseErrorBytes: 0, parseErrorSaved: true,
    payloadBase64: Buffer.from(bytes).toString('base64'), payloadSHA256: createHash('sha256').update(bytes).digest('hex'),
    payloadBytes: bytes.byteLength, payloadSaved: true,
  };
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(diagnostic);
  render(<AuthoringDiagnosticDetails workspaceId="ws" permitId="permit" phase="proposal" failureCode="invalid_proposal" diagnostic={diagnostic} />);
  fireEvent.click(button('按需加载并校验诊断原文'));
  expect(await screen.findByText(/不是有效 UTF-8 文本；已验证原字节可下载/)).toBeTruthy();
  expect(screen.queryByText(/�/)).toBeNull();
  expect(Array.from(await downloadedCauseBytes())).toEqual(Array.from(bytes));
});

it('Provider document/wire 只显示有据可依的观测状态，并校验、保留 BOM 原字节', async () => {
  browserCrypto();
  const documentText = '﻿完整 provider 文档';
  const wireBytes = new Uint8Array([0xff, 0xfe, 0x61]);
  const documentBytes = new TextEncoder().encode(documentText);
  const base = {
    workspaceId: 'ws', permitId: 'permit', phase: 'proposal', failureCode: 'invalid_proposal', providerCode: 'invalid_response',
    parseError: 'provider returned incomplete output',
    parseErrorSHA256: createHash('sha256').update('provider returned incomplete output').digest('hex'),
    parseErrorBytes: Buffer.byteLength('provider returned incomplete output'), parseErrorSaved: true,
    payloadBase64: '', payloadSHA256: createHash('sha256').digest('hex'), payloadBytes: 0, payloadSaved: true,
    providerDocument: { observed: true, complete: true, saved: true, observedBytes: documentBytes.byteLength, sha256: createHash('sha256').update(documentBytes).digest('hex'), contentBase64: Buffer.from(documentBytes).toString('base64') },
    providerWire: { observed: true, complete: false, saved: true, observedBytes: wireBytes.byteLength, sha256: createHash('sha256').update(wireBytes).digest('hex'), reason: '上游截断', contentBase64: Buffer.from(wireBytes).toString('base64') },
    providerCause: { observed: true, complete: true, saved: true, observedBytes: Buffer.byteLength('connection reset'), sha256: createHash('sha256').update('connection reset').digest('hex'), reason: '传输层观测', contentBase64: Buffer.from('connection reset').toString('base64') },
  };
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(base);
  render(<AuthoringDiagnosticDetails workspaceId="ws" permitId="permit" phase="proposal" failureCode="invalid_proposal" diagnostic={base} />);
  expect(screen.getByText(/Provider 文档已完整观察.*并已保存/)).toBeTruthy();
  expect(screen.getByText(/Provider wire仅部分观察/)).toBeTruthy();
  expect(screen.getByText(/安全失败分类与传输错误原文分离。传输错误原文已完整观察/)).toBeTruthy();
  fireEvent.click(button('按需加载并校验诊断原文'));
  expect(await screen.findByText((_, element) => element?.tagName === 'PRE' && element.textContent === documentText)).toBeTruthy();
  expect(await screen.findByText(/Provider wire 不是有效 UTF-8 文本/)).toBeTruthy();
  expect(Array.from(await downloadedCauseBytes('下载已校验 Provider 文档 bytes'))).toEqual(Array.from(documentBytes));
  expect(Array.from(await downloadedCauseBytes('下载已校验 Provider wire bytes'))).toEqual(Array.from(wireBytes));
  expect(Array.from(await downloadedCauseBytes('下载已校验传输错误原文 bytes'))).toEqual(Array.from(new TextEncoder().encode('connection reset')));
  expect(screen.queryByText(/�/)).toBeNull();
});

it('Provider 未观察及未保存信息不被空 payload 冒充为 provider 输出', async () => {
  browserCrypto();
  const diagnostic = {
    workspaceId: 'ws', permitId: 'permit', phase: 'proposal', failureCode: 'invalid_proposal',
    parseError: 'failure reason', parseErrorSHA256: createHash('sha256').update('failure reason').digest('hex'), parseErrorBytes: 14, parseErrorSaved: true,
    payloadBase64: '', payloadSHA256: createHash('sha256').digest('hex'), payloadBytes: 0, payloadSaved: false, notSavedReason: 'payload 空不是文档',
    providerDocument: { observed: false, complete: false, saved: false, observedBytes: 0, sha256: '', reason: '未到 provider' },
    providerWire: { observed: true, complete: true, saved: false, observedBytes: 12, sha256: 'actual-observation-hash', reason: '存储上限' },
    providerCause: { observed: true, complete: true, saved: false, observedBytes: 11, sha256: 'leaf-error-hash', reason: '诊断存储边界' },
  };
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(diagnostic);
  render(<AuthoringDiagnosticDetails workspaceId="ws" permitId="permit" phase="proposal" failureCode="invalid_proposal" diagnostic={diagnostic} />);
  expect(screen.getByText(/Provider 文档未观察/)).toBeTruthy();
  expect(screen.getByText(/Provider wire已完整观察（12 字节，SHA-256 actual-observation-hash），但未保存：存储上限/)).toBeTruthy();
  expect(screen.getByText(/传输错误原文已完整观察（11 字节，SHA-256 leaf-error-hash），但未保存：诊断存储边界/)).toBeTruthy();
  expect(screen.getByText(/解析器输入 payload 未保存：payload 空不是文档/)).toBeTruthy();
  expect(screen.queryByRole('button', { name: '按需加载并校验诊断原文' })).toBeNull();
  expect(workorchApi.readAuthoringDiagnostic).not.toHaveBeenCalled();
});

it('Provider 文档 SHA-256 错配时 fail closed，不显示或提供下载', async () => {
  browserCrypto();
  const bytes = new TextEncoder().encode('provider document');
  const diagnostic = {
    workspaceId: 'ws', permitId: 'permit', phase: 'proposal', failureCode: 'invalid_proposal',
    parseError: 'failed', parseErrorSHA256: createHash('sha256').update('failed').digest('hex'), parseErrorBytes: 6, parseErrorSaved: true,
    payloadBase64: '', payloadSHA256: createHash('sha256').digest('hex'), payloadBytes: 0, payloadSaved: false,
    providerDocument: { observed: true, complete: true, saved: true, observedBytes: bytes.byteLength, sha256: createHash('sha256').update('other').digest('hex'), contentBase64: Buffer.from(bytes).toString('base64') },
  };
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(diagnostic);
  render(<AuthoringDiagnosticDetails workspaceId="ws" permitId="permit" phase="proposal" failureCode="invalid_proposal" diagnostic={diagnostic} />);
  fireEvent.click(button('按需加载并校验诊断原文'));
  await screen.findByText(/诊断原文不可用或完整性校验失败/);
  expect(screen.getByText(/provider 文档 SHA-256 不匹配/)).toBeTruthy();
  expect(screen.queryByText((_, element) => element?.tagName === 'PRE' && element.textContent === 'provider document')).toBeNull();
  expect(screen.queryByRole('button', { name: '下载已校验 Provider 文档 bytes' })).toBeNull();
});

it('prep cause 失败原文在 scope 切换首帧即隔离，不等待 passive cleanup', async () => {
  const expected = preparationDiagnostic();
  vi.mocked(workorchApi.readAuthoringPreparationDiagnostic).mockRejectedValueOnce(new Error('旧 scope 独有失败 Ω'));
  const unchanged = () => {};
  const mounted = render(<FirstCommitObservation observe={unchanged}><AuthoringPreparationDiagnosticDetails diagnostic={expected} /></FirstCommitObservation>);
  fireEvent.click(button('按需加载并校验前置原因'));
  await screen.findByText('旧 scope 独有失败 Ω');
  let observedFirstCommit = false;
  mounted.rerender(<FirstCommitObservation observe={() => {
    observedFirstCommit = true;
    expect(screen.queryByText('旧 scope 独有失败 Ω')).toBeNull();
    expect(screen.queryByText('前置原因不可用或完整性校验失败')).toBeNull();
  }}><AuthoringPreparationDiagnosticDetails diagnostic={preparationDiagnostic({ workspaceId: 'new-workspace', diagnosticId: 'new-diagnostic' })} /></FirstCommitObservation>);
  expect(observedFirstCommit).toBe(true);
  expect(workorchApi.readAuthoringPreparationDiagnostic).toHaveBeenCalledTimes(1);
});

it('prep cause GET 迟到且切换 scope 后不泄漏；相同 metadata rerender 不丢在途读取', async () => {
  browserCrypto();
  const expected = preparationDiagnostic();
  const deferredRead = deferred<DefinitionAuthoringPreparationDiagnostic>();
  vi.mocked(workorchApi.readAuthoringPreparationDiagnostic).mockReturnValueOnce(deferredRead.promise);
  const observeUnchangedCommit = () => {};
  const mounted = render(<FirstCommitObservation observe={observeUnchangedCommit}><AuthoringPreparationDiagnosticDetails diagnostic={expected} /></FirstCommitObservation>);
  fireEvent.click(button('按需加载并校验前置原因'));
  mounted.rerender(<FirstCommitObservation observe={observeUnchangedCommit}><AuthoringPreparationDiagnosticDetails diagnostic={{ ...expected }} /></FirstCommitObservation>);
  const cause = 'provider 原因：暂不可用';
  await act(async () => deferredRead.resolve({ ...preparationDiagnostic(), causeBase64: Buffer.from(cause).toString('base64') }));
  expect(await screen.findByText((_, element) => element?.tagName === 'PRE' && element.textContent === cause)).toBeTruthy();
  let observedFirstCommit = false;
  mounted.rerender(<FirstCommitObservation observe={() => {
    observedFirstCommit = true;
    expect(screen.queryByText(cause)).toBeNull();
    expect(document.querySelector('pre')).toBeNull();
    expect(document.querySelectorAll('button').length).toBe(1);
  }}><AuthoringPreparationDiagnosticDetails diagnostic={preparationDiagnostic({ workspaceId: 'new-workspace', diagnosticId: 'new-diagnostic', current: false })} /></FirstCommitObservation>);
  expect(observedFirstCommit).toBe(true);
  expect(screen.queryByText(cause)).toBeNull();
});

  it('editor 失败记录内嵌展示诊断原文而不主动加载', async () => {
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(diagnosticView());
    render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    expect(await screen.findByText((_, element) => element?.tagName === 'PRE' && element.textContent?.includes('敏感标记: <img src=x onerror=alert(1)>'))).toBeTruthy();
    expect(workorchApi.readAuthoringDiagnostic).not.toHaveBeenCalled();
  });

it.each([
  { name: '真实 LF/CRLF', parseError: '解析错误\n敏感标记: <img src=x onerror=alert(1)>\n结束', raw: '第一行 Ω\r\n敏感标记: <script>raw</script>\n末行\n' },
  { name: '字面转义字符', parseError: '解析错误\\n敏感标记: <img src=x onerror=alert(1)>\\n结束', raw: '第一行 Ω\\n敏感标记: <script>raw</script>\\n末行\\n' },
])('失败诊断原样显示 $name、Unicode 和敏感 marker，按需校验原始响应且不自动运行', async ({ parseError, raw }) => {
  browserCrypto();
  const bytes = new TextEncoder().encode(raw);
  const diagnostic = {
    workspaceId: 'ws', permitId: 'permit raw/?', phase: 'proposal', failureCode: 'invalid_proposal', parseError,
    parseErrorSHA256: createHash('sha256').update(parseError).digest('hex'), parseErrorBytes: Buffer.byteLength(parseError), parseErrorSaved: true,
    payloadBase64: Buffer.from(bytes).toString('base64'), payloadSHA256: createHash('sha256').update(bytes).digest('hex'), payloadBytes: bytes.byteLength, payloadSaved: true,
  };
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(diagnostic);
  render(<AuthoringDiagnosticDetails workspaceId="ws" permitId="permit raw/?" phase="proposal" failureCode="invalid_proposal" diagnostic={diagnostic} />);
  expect(screen.getByText((_, element) => element?.tagName === 'PRE' && element.textContent === parseError)).toBeTruthy();
  expect(screen.getByText('invalid_proposal · proposal')).toBeTruthy();
  expect(screen.queryByRole('img')).toBeNull();
  expect(workorchApi.readAuthoringDiagnostic).not.toHaveBeenCalled();
  fireEvent.click(button('按需加载并校验诊断原文'));
  await waitFor(() => expect(workorchApi.readAuthoringDiagnostic).toHaveBeenCalledExactlyOnceWith('ws', 'permit raw/?'));
  await waitFor(() => expect(screen.getByText((_, element) => element?.tagName === 'PRE' && element.textContent === raw)).toBeTruthy());
  expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
});

it('原始响应 hash 不匹配时明确报错且不显示伪成功内容', async () => {
  browserCrypto();
  const parseError = '原样错误';
  const diagnostic = {
    workspaceId: 'ws', permitId: 'permit', phase: 'proposal', failureCode: 'invalid_proposal', parseError,
    parseErrorSHA256: createHash('sha256').update(parseError).digest('hex'), parseErrorBytes: Buffer.byteLength(parseError), parseErrorSaved: true,
    payloadBase64: Buffer.from('different raw').toString('base64'), payloadSHA256: createHash('sha256').update('expected').digest('hex'), payloadBytes: Buffer.byteLength('different raw'), payloadSaved: true,
  };
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(diagnostic);
  render(<AuthoringDiagnosticDetails workspaceId="ws" permitId="permit" phase="proposal" failureCode="invalid_proposal" diagnostic={{ ...diagnostic }} />);
  fireEvent.click(button('按需加载并校验诊断原文'));
  await screen.findByText('诊断原文不可用或完整性校验失败');
  expect(screen.getByText(/解析器输入 payload SHA-256 不匹配/)).toBeTruthy();
  expect(screen.queryByText('different raw')).toBeNull();
  expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
});

it('旧 failed 记录明确显示无诊断，且不加载或重发 provider', () => {
  render(<AuthoringDiagnosticDetails workspaceId="ws" phase="proposal" failureCode="invalid_proposal" />);
  expect(screen.getByText(/此失败记录没有可用诊断/)).toBeTruthy();
  expect(screen.getByText(/原始原因未记录，且不会重新调用 provider/)).toBeTruthy();
  expect(screen.queryByRole('button', { name: /加载.*原始响应/ })).toBeNull();
  expect(workorchApi.readAuthoringDiagnostic).not.toHaveBeenCalled();
  expect(workorchApi.compileDefinitionAuthoring).not.toHaveBeenCalled();
  expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
});

it('diagnostic fetch 在 workspace/permit 切换后迟到，不泄漏或覆盖新诊断', async () => {
  browserCrypto();
  const pending = deferred<Awaited<ReturnType<typeof workorchApi.readAuthoringDiagnostic>>>();
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockReturnValueOnce(pending.promise);
  const oldPayload = '旧原始响应\r\nΩ';
  const base = {
    parseError: '旧 parse', parseErrorSHA256: createHash('sha256').update('旧 parse').digest('hex'), parseErrorBytes: Buffer.byteLength('旧 parse'), parseErrorSaved: true,
    payloadBase64: Buffer.from(oldPayload).toString('base64'), payloadSHA256: createHash('sha256').update(oldPayload).digest('hex'), payloadBytes: Buffer.byteLength(oldPayload), payloadSaved: true,
  };
  const mounted = render(<AuthoringDiagnosticDetails workspaceId="old-ws" permitId="old-permit" phase="proposal" failureCode="invalid_proposal" diagnostic={base} />);
  fireEvent.click(button('按需加载并校验诊断原文'));
  mounted.rerender(<AuthoringDiagnosticDetails workspaceId="new-ws" permitId="new-permit" phase="proposal" failureCode="invalid_proposal" diagnostic={{ ...base, parseError: '新 parse', parseErrorSHA256: createHash('sha256').update('新 parse').digest('hex'), parseErrorBytes: Buffer.byteLength('新 parse') }} />);
  await act(async () => pending.resolve({ workspaceId: 'old-ws', permitId: 'old-permit', phase: 'proposal', failureCode: 'invalid_proposal', ...base }));
  expect(screen.getByText((_, element) => element?.tagName === 'PRE' && element.textContent === '新 parse')).toBeTruthy();
  expect(screen.queryByText(/旧 parse/)).toBeNull();
  expect(screen.queryByText((_, element) => element?.tagName === 'PRE' && element.textContent === oldPayload)).toBeNull();
  expect(screen.queryByRole('button', { name: '下载已校验原始字节' })).toBeNull();
});

it('diagnostic fetch workspace/permit 身份不匹配时拒绝显示', async () => {
  browserCrypto();
  const parseError = '身份错误诊断';
  const diagnostic = {
    workspaceId: 'different-ws', permitId: 'permit', phase: 'proposal', failureCode: 'invalid_proposal', parseError,
    parseErrorSHA256: createHash('sha256').update(parseError).digest('hex'), parseErrorBytes: Buffer.byteLength(parseError), parseErrorSaved: true,
    payloadBase64: '', payloadSHA256: createHash('sha256').digest('hex'), payloadBytes: 0, payloadSaved: true,
  };
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(diagnostic);
  render(<AuthoringDiagnosticDetails workspaceId="ws" permitId="permit" phase="proposal" failureCode="invalid_proposal" diagnostic={{ ...diagnostic, workspaceId: undefined, permitId: undefined } as never} />);
  fireEvent.click(button('按需加载并校验诊断原文'));
  await screen.findByText(/workspace 或 permit 身份不匹配/);
  expect(screen.getByText((_, element) => element?.tagName === 'PRE' && element.textContent === parseError)).toBeTruthy();
});

it('相同诊断的周期查询不丢弃正在加载的原始响应', async () => {
  browserCrypto();
  const parseError = '原始错误';
  const payload = '  原始响应\\r\\n';
  const result = {
    workspaceId: 'ws', permitId: 'permit', phase: 'proposal', failureCode: 'invalid_proposal', parseError,
    parseErrorSHA256: createHash('sha256').update(parseError).digest('hex'), parseErrorBytes: Buffer.byteLength(parseError), parseErrorSaved: true,
    payloadBase64: Buffer.from(payload).toString('base64'), payloadSHA256: createHash('sha256').update(payload).digest('hex'), payloadBytes: Buffer.byteLength(payload), payloadSaved: true,
  };
  let finish!: (value: typeof result) => void;
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockImplementation(() => new Promise(resolve => { finish = resolve; }));
  const props = { workspaceId: 'ws', permitId: 'permit', phase: 'proposal', failureCode: 'invalid_proposal' };
  const { rerender } = render(<AuthoringDiagnosticDetails {...props} diagnostic={{ ...result }} />);
  fireEvent.click(button('按需加载并校验诊断原文'));
  rerender(<AuthoringDiagnosticDetails {...props} diagnostic={{ ...result }} />);
  await act(async () => finish(result));
  await screen.findByText('下载已校验原始字节');
  expect(screen.getByText((_, element) => element?.tagName === 'PRE' && element.textContent === payload)).toBeTruthy();
});

it('byte 数不匹配时拒绝原始响应并明确报告', async () => {
  browserCrypto();
  const errorText = '完整诊断错误';
  const diagnostic = {
    workspaceId: 'ws', permitId: 'permit', phase: 'proposal', failureCode: 'invalid_proposal', parseError: errorText,
    parseErrorSHA256: createHash('sha256').update(errorText).digest('hex'), parseErrorBytes: Buffer.byteLength(errorText), parseErrorSaved: true,
    payloadBase64: Buffer.from('payload').toString('base64'), payloadSHA256: createHash('sha256').update('payload').digest('hex'), payloadBytes: 999, payloadSaved: true,
  };
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(diagnostic);
  render(<AuthoringDiagnosticDetails workspaceId="ws" permitId="permit" phase="proposal" failureCode="invalid_proposal" diagnostic={{ ...diagnostic }} />);
  fireEvent.click(button('按需加载并校验诊断原文'));
  await screen.findByText('诊断原文不可用或完整性校验失败');
  expect(screen.getByText(/字节数不匹配/)).toBeTruthy();
  expect(screen.queryByText('payload')).toBeNull();
});
