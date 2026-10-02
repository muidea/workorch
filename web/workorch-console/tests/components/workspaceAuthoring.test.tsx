import { StrictMode } from 'react';
import { App as AntApp } from 'antd';
const App = AntApp;
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createHash } from 'node:crypto';
import { workorchApi } from '../../src/api';
import { WorkspaceAuthoringEditor } from '../../src/pages/components/WorkspaceAuthoringEditor';
import { WorkspaceSetupModal } from '../../src/pages/components/WorkspaceSetupModal';
import { AuthoringDiagnosticDetails } from '../../src/pages/components/AuthoringDiagnosticDetails';
import type { DefinitionAuthoringView } from '../../src/types';

vi.mock('../../src/api', () => ({ workorchApi: {
  listWorkspaceTemplates: vi.fn(), readDefinitionAuthoring: vi.fn(), readAuthoringDiagnostic: vi.fn(),
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
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
const button = (name: string) => {
  const element = screen.getByText(name, { exact: true }).closest('button');
  if (!element) throw new Error(`按钮不存在：${name}`);
  return element;
};
const disabled = (name: string) => button(name).disabled;

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(workorchApi.listWorkspaceTemplates).mockResolvedValue([{ id: 't', name: '模板' } as never]);
  vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(ready());
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockImplementation(async (workspaceId, permitId) => ({
    workspaceId, permitId, phase: 'proposal', failureCode: 'invalid_proposal', parseError: '默认解析错误',
    parseErrorSHA256: createHash('sha256').update('默认解析错误').digest('hex'), parseErrorBytes: Buffer.byteLength('默认解析错误'), parseErrorSaved: true,
    payloadBase64: '', payloadSHA256: createHash('sha256').digest('hex'), payloadBytes: 0, payloadSaved: true,
  }));
});

afterEach(() => {
  expect((workorchApi as unknown as { submitWorkspaceExecution: ReturnType<typeof vi.fn> }).submitWorkspaceExecution).not.toHaveBeenCalled();
});

describe('持久规格编辑实际挂载', () => {
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
    vi.mocked(workorchApi.saveDefinitionAuthoring).mockResolvedValue({ ...ready(), input: { goalText: '修订目标' }, workspaceRevision: 9, inputHash: 'new' });
    vi.mocked(workorchApi.compileDefinitionAuthoring).mockResolvedValue(ready());
    render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    await screen.findByDisplayValue('原目标');
    fireEvent.change(screen.getByLabelText('目标'), { target: { value: '修订目标' } });
    fireEvent.click(button('保存修订并审查'));
    await waitFor(() => expect(workorchApi.compileDefinitionAuthoring).toHaveBeenCalledWith('ws'));
    const [, , , revision, hash, requestId] = vi.mocked(workorchApi.saveDefinitionAuthoring).mock.calls[0];
    expect(revision).toBe(8);
    expect(hash).toBe('input');
    expect(requestId).toBeTruthy();
    expect(workorchApi.createDefinitionAuthoring).not.toHaveBeenCalled();
  });

  it('内存 pendingConfirmation 存在时同 scope GET 不替换或丢弃原确认身份', async () => {
    const pending = deferred<DefinitionAuthoringView>();
    vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValueOnce(ready());
    vi.mocked(workorchApi.confirmDefinitionAuthoring).mockReturnValueOnce(pending.promise);
    const mounted = render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
    await screen.findByDisplayValue('原目标');
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

function browserCrypto() {
  Object.defineProperty(globalThis, 'crypto', { configurable: true, value: {
    subtle: { digest: async (_algorithm: string, data: ArrayBuffer | ArrayBufferView) => {
      const bytes = ArrayBuffer.isView(data) ? new Uint8Array(data.buffer, data.byteOffset, data.byteLength) : new Uint8Array(data as ArrayBuffer);
      const hash = createHash('sha256').update(bytes).digest();
      return Uint8Array.from(hash).buffer;
    } },
  } as unknown as Crypto });
}

it('editor 失败记录内嵌展示诊断原文而不主动加载', async () => {
  vi.mocked(workorchApi.readDefinitionAuthoring).mockResolvedValue(diagnosticView());
  render(<AntApp><WorkspaceAuthoringEditor open workspaceId="ws" onClose={vi.fn()} onAdmitted={vi.fn()} /></AntApp>);
  expect(await screen.findByText((_, element) => element?.tagName === 'PRE' && element.textContent?.includes('敏感标记: <img src=x onerror=alert(1)>'))).toBeTruthy();
  expect(workorchApi.readAuthoringDiagnostic).not.toHaveBeenCalled();
});

it('失败诊断原样显示多行 Unicode 和敏感 marker，按需校验原始响应且不自动运行', async () => {
  browserCrypto();
  const parseError = '解析错误\\n敏感标记: <img src=x onerror=alert(1)>\\n结束';
  const raw = '第一行 Ω\\n敏感标记: <script>raw</script>\\n末行\\n';
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
  fireEvent.click(screen.getByRole('button', { name: '按需加载并校验原始响应' }));
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
  fireEvent.click(screen.getByRole('button', { name: '按需加载并校验原始响应' }));
  await screen.findByText('原始响应不可用或完整性校验失败');
  expect(screen.getByText(/原始响应 SHA-256 不匹配/)).toBeTruthy();
  expect(screen.queryByText('different raw')).toBeNull();
  expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
});

it('旧 failed 记录明确显示无诊断，且不加载或重发 provider', () => {
  render(<AuthoringDiagnosticDetails workspaceId="ws" phase="proposal" failureCode="invalid_proposal" />);
  expect(screen.getByText(/此旧失败记录没有保存诊断/)).toBeTruthy();
  expect(screen.getByText(/原始原因未记录，且不会重新调用 provider/)).toBeTruthy();
  expect(screen.queryByRole('button', { name: /加载.*原始响应/ })).toBeNull();
  expect(workorchApi.readAuthoringDiagnostic).not.toHaveBeenCalled();
  expect(workorchApi.compileDefinitionAuthoring).not.toHaveBeenCalled();
  expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
});

it('diagnostic fetch 在 workspace/permit 切换后迟到，不泄漏或覆盖新诊断', async () => {
  const pending = deferred<Awaited<ReturnType<typeof workorchApi.readAuthoringDiagnostic>>>();
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockReturnValueOnce(pending.promise);
  const base = {
    parseError: '旧 parse', parseErrorSHA256: createHash('sha256').update('旧 parse').digest('hex'), parseErrorBytes: Buffer.byteLength('旧 parse'), parseErrorSaved: true,
    payloadBase64: '', payloadSHA256: createHash('sha256').digest('hex'), payloadBytes: 0, payloadSaved: true,
  };
  const mounted = render(<AuthoringDiagnosticDetails workspaceId="old-ws" permitId="old-permit" phase="proposal" failureCode="invalid_proposal" diagnostic={base} />);
  fireEvent.click(screen.getByRole('button', { name: '按需加载并校验原始响应' }));
  mounted.rerender(<AuthoringDiagnosticDetails workspaceId="new-ws" permitId="new-permit" phase="proposal" failureCode="invalid_proposal" diagnostic={{ ...base, parseError: '新 parse', parseErrorSHA256: createHash('sha256').update('新 parse').digest('hex'), parseErrorBytes: Buffer.byteLength('新 parse') }} />);
  await act(async () => pending.resolve({ workspaceId: 'old-ws', permitId: 'old-permit', phase: 'proposal', failureCode: 'invalid_proposal', ...base }));
  expect(screen.getByText((_, element) => element?.tagName === 'PRE' && element.textContent === '新 parse')).toBeTruthy();
  expect(screen.queryByText(/旧 parse/)).toBeNull();
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
  fireEvent.click(screen.getByRole('button', { name: '按需加载并校验原始响应' }));
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
  fireEvent.click(screen.getByRole('button', { name: '按需加载并校验原始响应' }));
  rerender(<AuthoringDiagnosticDetails {...props} diagnostic={{ ...result }} />);
  finish(result);
  await screen.findByRole('button', { name: '下载已校验原始字节' });
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
  fireEvent.click(screen.getByRole('button', { name: '按需加载并校验原始响应' }));
  await screen.findByText('原始响应不可用或完整性校验失败');
  expect(screen.getByText(/字节数不匹配/)).toBeTruthy();
  expect(screen.queryByText('payload')).toBeNull();
});