import { act, fireEvent, render as mount, screen } from '@testing-library/react';
import { ConfigProvider } from 'antd';
import { useLayoutEffect } from 'react';
import type { ReactNode } from 'react';
import { createHash, webcrypto } from 'node:crypto';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { AuthoringDiagnosticDetails } from '../../src/pages/components/AuthoringDiagnosticDetails';
import { workorchApi } from '../../src/api';
import type { AuthoringDiagnostic, AuthoringDiagnosticMetadata } from '../../src/types';

vi.mock('../../src/api', () => ({ workorchApi: { readAuthoringDiagnostic: vi.fn() } }));

const hash = (text: string) => createHash('sha256').update(text).digest('hex');
const observation = (text: string, complete = true) => ({ observed: true, complete, saved: true, observedBytes: Buffer.byteLength(text), sha256: hash(text), contentBase64: Buffer.from(text).toString('base64'), ...(complete ? {} : { reason: 'stream_incomplete' }) });
const diagnostic = (): AuthoringDiagnostic => ({
  workspaceId: 'ws', permitId: 'permit', phase: 'proposal', failureCode: 'invalid_proposal',
  parseError: '原始错误', parseErrorSHA256: hash('原始错误'), parseErrorBytes: Buffer.byteLength('原始错误'), parseErrorSaved: true,
  payloadBase64: Buffer.from('parser input').toString('base64'), payloadBytes: Buffer.byteLength('parser input'), payloadSHA256: hash('parser input'), payloadSaved: true,
});
const metadata = (value: AuthoringDiagnostic): AuthoringDiagnosticMetadata => {
  const observationMetadata = (content: AuthoringDiagnostic['providerDocument']) => {
    if (!content) return undefined;
    const { contentBase64: _bytes, ...rest } = content;
    return rest;
  };
  const { workspaceId: _workspace, permitId: _permit, phase: _phase, failureCode: _failure, parseError: _error, payloadBase64: _payload, ...rest } = value;
  return { ...rest, providerDocument: observationMetadata(value.providerDocument), providerWire: observationMetadata(value.providerWire), providerCause: observationMetadata(value.providerCause) };
};
const render = (ui: ReactNode) => mount(ui, {
  wrapper: ({ children }) => <ConfigProvider theme={{ token: { motion: false } }} wave={{ disabled: true }}>{children}</ConfigProvider>,
});
const load = () => {
  const element = Array.from(document.querySelectorAll<HTMLButtonElement>('button')).find(item => item.textContent === '按需加载并校验诊断原文');
  if (!element) throw new Error('诊断加载按钮不存在');
  fireEvent.click(element);
};

beforeEach(() => { vi.resetAllMocks(); vi.stubGlobal('crypto', webcrypto); });
afterEach(() => vi.unstubAllGlobals());

it('真实 metadata-only 列表不误报原文缺失；仅显式读取后校验并显示全部原文', async () => {
  const value = { ...diagnostic(), providerDocument: observation('parser input'), providerWire: observation('raw wire') };
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(value);
  render(<AuthoringDiagnosticDetails workspaceId="ws" permitId="permit" phase="proposal" failureCode="invalid_proposal" diagnostic={metadata(value)} />);
  await act(async () => {});
  expect(screen.queryByText(/完整性校验失败/)).toBeNull();
  expect(screen.queryByText('原始错误')).toBeNull();
  expect(workorchApi.readAuthoringDiagnostic).not.toHaveBeenCalled();
  load();
  expect(await screen.findByText('原始错误')).toBeTruthy();
  expect(screen.getAllByText('parser input')).toHaveLength(2);
  expect(screen.getByText('raw wire')).toBeTruthy();
  expect(screen.getByText('下载已校验原始字节')).toBeTruthy();
  expect(workorchApi.readAuthoringDiagnostic).toHaveBeenCalledExactlyOnceWith('ws', 'permit');
});

it('仅解析错误保存而 payload 和 Provider 原文未保存时仍可显式读取错误', async () => {
  const value = { ...diagnostic(), payloadSaved: false, notSavedReason: 'storage boundary' };
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(value);
  render(<AuthoringDiagnosticDetails workspaceId="ws" permitId="permit" phase="proposal" failureCode="invalid_proposal" diagnostic={metadata(value)} />);
  load();
  expect(await screen.findByText('原始错误')).toBeTruthy();
  expect(screen.queryByText('下载已校验原始字节')).toBeNull();
});

it.each([
  ['error hash', { parseErrorSHA256: hash('other') }],
  ['error bytes', { parseErrorBytes: 123 }],
  ['error saved', { parseErrorSaved: false }],
  ['payload hash', { payloadSHA256: hash('other') }],
  ['payload bytes', { payloadBytes: 123 }],
  ['payload saved', { payloadSaved: false }],
  ['provider code', { providerCode: 'different' }],
  ['not saved reason', { notSavedReason: 'different' }],
  ['provider wire', { providerWire: observation('different') }],
] as const)('metadata-only 读取仍拒绝 %s 漂移', async (_name, changed) => {
  const value = diagnostic();
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(value);
  render(<AuthoringDiagnosticDetails workspaceId="ws" permitId="permit" phase="proposal" failureCode="invalid_proposal" diagnostic={{ ...metadata(value), ...changed }} />);
  load();
  expect(await screen.findByText('诊断元数据与失败记录不匹配，已拒绝显示。')).toBeTruthy();
  expect(screen.queryByText('原始错误')).toBeNull();
  expect(screen.queryByText('parser input')).toBeNull();
});

it.each([
  ['parse error', { parseError: '篡改错误' }],
  ['payload', { payloadBase64: Buffer.from('forged input').toString('base64') }],
] as const)('metadata-only 场景仍独立校验 %s 原字节', async (_name, changed) => {
  const value = diagnostic();
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue({ ...value, ...changed });
  render(<AuthoringDiagnosticDetails workspaceId="ws" permitId="permit" phase="proposal" failureCode="invalid_proposal" diagnostic={metadata(value)} />);
  load();
  expect(await screen.findByText('诊断原文不可用或完整性校验失败')).toBeTruthy();
  expect(screen.queryByText('篡改错误')).toBeNull();
  expect(screen.queryByText('forged input')).toBeNull();
  expect(screen.queryByText('下载已校验原始字节')).toBeNull();
});

it('无 Provider metadata 只标注解析器 payload，不回填完整 Provider 文档', async () => {
  const value = diagnostic();
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(value);
  render(<AuthoringDiagnosticDetails workspaceId="ws" permitId="permit" phase="proposal" failureCode="invalid_proposal" diagnostic={value} />);
  expect(screen.getByText(/providerDocument 未提供观测 metadata/)).toBeTruthy();
  expect(screen.getByText(/providerWire 未提供观测 metadata/)).toBeTruthy();
  expect(screen.getByText(/providerCause 未提供观测 metadata/)).toBeTruthy();
  load();
  expect(await screen.findByText('parser input')).toBeTruthy();
  expect(screen.queryByText('下载已校验 Provider 文档 bytes')).toBeNull();
});

it('parser payload 与已保存完整 Provider 文档各自 hash 正确但 bytes 不同仍拒绝', async () => {
  const value = { ...diagnostic(), providerDocument: observation('different provider document') };
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(value);
  render(<AuthoringDiagnosticDetails workspaceId="ws" permitId="permit" phase="proposal" failureCode="invalid_proposal" diagnostic={value} />);
  load();
  expect(await screen.findByText(/解析器输入 payload 与 Provider 文档的原始 bytes 不一致/)).toBeTruthy();
  expect(screen.queryByText('different provider document')).toBeNull();
  expect(screen.queryByText('下载已校验 Provider 文档 bytes')).toBeNull();
});

it('三个独立观察可共存，partial wire 不因可下载而被标为完整', async () => {
  const value = { ...diagnostic(), providerDocument: observation('parser input'), providerWire: observation('partial raw wire', false), providerCause: observation('raw cause') };
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(value);
  render(<AuthoringDiagnosticDetails workspaceId="ws" permitId="permit" phase="proposal" failureCode="invalid_proposal" diagnostic={value} />);
  expect(screen.getByText(/Provider wire仅部分观察.*不视为完整 provider 输出/)).toBeTruthy();
  load();
  expect(await screen.findByText('partial raw wire')).toBeTruthy();
  expect(screen.getByText('raw cause')).toBeTruthy();
  for (const text of ['下载已校验 Provider 文档 bytes', '下载已校验 Provider wire bytes', '下载已校验传输错误原文 bytes']) expect(screen.getByText(text)).toBeTruthy();
});

it('已加载内容在 scope 切换的 layout 阶段就不可见或下载', async () => {
  const value = diagnostic();
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(value);
  let checkLayout = false;
  let visibleInLayout = false;
  const Inspector = ({ phase }: { phase: string }) => {
    useLayoutEffect(() => {
      if (checkLayout) visibleInLayout = Boolean(Array.from(document.querySelectorAll('pre')).find(item => item.textContent === 'parser input'))
        || Boolean(Array.from(document.querySelectorAll('button')).find(item => item.textContent === '下载已校验原始字节'));
    }, [phase]);
    return <AuthoringDiagnosticDetails workspaceId="ws" permitId="permit" phase={phase} failureCode="invalid_proposal" diagnostic={value} />;
  };
  const mounted = render(<Inspector phase="proposal" />);
  load();
  expect(await screen.findByText('parser input')).toBeTruthy();
  checkLayout = true;
  mounted.rerender(<Inspector phase="review" />);
  expect(visibleInLayout).toBe(false);
  expect(screen.queryByText('下载已校验原始字节')).toBeNull();
});

it('阶段 scope 在 SHA-256 异步校验期间变化时旧内容不显示', async () => {
  let release!: (value: ArrayBuffer) => void;
  const firstHash = new Promise<ArrayBuffer>(resolve => { release = resolve; });
  const digest = vi.fn().mockReturnValueOnce(firstHash).mockImplementation((algorithm, bytes) => webcrypto.subtle.digest(algorithm, bytes));
  vi.stubGlobal('crypto', { subtle: { digest } });
  const value = { ...diagnostic(), parseErrorSaved: false };
  vi.mocked(workorchApi.readAuthoringDiagnostic).mockResolvedValue(value);
  const props = { workspaceId: 'ws', permitId: 'permit', failureCode: 'invalid_proposal', diagnostic: value };
  const mounted = render(<AuthoringDiagnosticDetails {...props} phase="proposal" />);
  load();
  await act(async () => {});
  mounted.rerender(<AuthoringDiagnosticDetails {...props} phase="review" />);
  const bytes = await webcrypto.subtle.digest('SHA-256', Buffer.from('parser input'));
  await act(async () => { release(bytes as ArrayBuffer); });
  expect(screen.queryByText('parser input')).toBeNull();
  expect(screen.queryByText('下载已校验原始字节')).toBeNull();
});
