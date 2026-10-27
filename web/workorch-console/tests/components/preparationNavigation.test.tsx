import { App, ConfigProvider } from 'antd';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ReactNode } from 'react';
import { workorchApi } from '../../src/api';
import { WorkspaceDetailPage } from '../../src/pages/WorkspaceDetailPage';
import type { ServiceConfiguration, WorkspaceSnapshot } from '../../src/types';

const callbacks = vi.hoisted(() => ({ refresh: undefined as undefined | ((event: unknown) => void) }));
vi.mock('../../src/hooks', () => ({ useSSE: (callback: (event: unknown) => void) => { callbacks.refresh = callback; } }));
vi.mock('../../src/api', () => ({ workorchApi: {
  listWorkspaceProjection: vi.fn(), readWorkspaceSnapshotProjection: vi.fn(), listSouls: vi.fn(),
  readWorkspaceSoulBindings: vi.fn(), listSkills: vi.fn(), listMCPServers: vi.fn(), listMemory: vi.fn(),
  readConfiguration: vi.fn(), readStartupPreparation: vi.fn(), checkStartupPreparation: vi.fn(),
  recoverStartupPreparation: vi.fn(), submitWorkspaceExecution: vi.fn(),
} }));
vi.mock('../../src/pages/components/WorkspaceAuthoringEditor', () => ({ WorkspaceAuthoringEditor: () => null }));
vi.mock('../../src/pages/components/WorkspaceAuthoringCandidate', () => ({ WorkspaceAuthoringCandidate: () => null }));
vi.mock('../../src/pages/components/WorkspaceAuthoringConfirmation', () => ({ WorkspaceAuthoringConfirmation: () => null }));
vi.mock('../../src/pages/components/AuthoringSummary', () => ({ AuthoringSummary: () => null }));
vi.mock('../../src/helpers', async importOriginal => ({
  ...await importOriginal<typeof import('../../src/helpers')>(),
  // Metric-card styling is outside this navigation contract. Keep the values
  // visible while avoiding dozens of unrelated Ant Design card layouts.
  metricGrid: (items: { key: string; label: ReactNode; value: ReactNode }[]) => <div>{items.map(item => <div key={item.key}>{item.label}: {item.value}</div>)}</div>,
}));

function snapshot(id: string): WorkspaceSnapshot {
  return { workspace: { id, title: id, state: 'active', revision: 10 }, goals: [{ id: 'goal', text: 'original goal' }], dods: [{ id: 'dod', state: 'ready', items: ['original acceptance'] }], runs: [] } as unknown as WorkspaceSnapshot;
}
function configuration(modelId = 'fresh-model'): ServiceConfiguration {
  return { llm: { defaultChat: { providerId: 'ai-proxy', modelId }, catalog: { providerId: 'ai-proxy', models: [{ providerId: 'ai-proxy', modelId, capabilityReady: true, operations: ['responses'] }] } } } as unknown as ServiceConfiguration;
}
function deferred<T>() { let resolve!: (value: T) => void; let reject!: (error: Error) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; }); return { promise, resolve, reject }; }
const page = (id = 'ws') => <ConfigProvider theme={{ token: { motion: false } }}><App><WorkspaceDetailPage selectedWorkspaceId={id} /></App></ConfigProvider>;
// The full page contains many tables. Scope queries to button text instead of
// calculating accessible names for every hidden table cell in jsdom.
const button = (text: string) => screen.getByText(text, { selector: 'button span' }).closest('button')!;
const closeModal = () => document.querySelector<HTMLButtonElement>('.ant-modal:not([style*="display: none"]) button[aria-label="Close"]')!;

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(workorchApi.listWorkspaceProjection).mockResolvedValue({ workspaces: [{ id: 'ws', title: 'ws' }, { id: 'ws2', title: 'ws2' }] } as Awaited<ReturnType<typeof workorchApi.listWorkspaceProjection>>);
  vi.mocked(workorchApi.readWorkspaceSnapshotProjection).mockImplementation(async id => ({ snapshot: snapshot(id), projection: { state: 'ready' } } as Awaited<ReturnType<typeof workorchApi.readWorkspaceSnapshotProjection>>));
  vi.mocked(workorchApi.listSouls).mockResolvedValue([]);
  vi.mocked(workorchApi.readWorkspaceSoulBindings).mockResolvedValue({});
  vi.mocked(workorchApi.listSkills).mockResolvedValue([]);
  vi.mocked(workorchApi.listMCPServers).mockResolvedValue([]);
  vi.mocked(workorchApi.listMemory).mockResolvedValue([]);
  vi.mocked(workorchApi.readStartupPreparation).mockResolvedValue({ workspaceId: 'ws', workspaceRevision: 10, operationState: 'not_checked', currentValidity: 'not_checked' });
  vi.mocked(workorchApi.readConfiguration).mockResolvedValue(configuration());
});

describe('STEP04 整页导航没有隐式外部库存读取', () => {
  it('详情开页、SSE 刷新、准备开关和只读刷新不读取模型目录或执行 SkillHub 库存', async () => {
    render(page());
    await screen.findByText('original goal');
    await act(async () => callbacks.refresh?.({ topic: '/workorchd/workspace/projection/changed', payload: { workspaceId: 'ws', revision: 12 } }));
    await waitFor(() => expect(vi.mocked(workorchApi.readWorkspaceSnapshotProjection).mock.calls.length).toBeGreaterThan(1));
    await act(async () => fireEvent.click(button('激活前准备')));
    await waitFor(() => expect(workorchApi.readStartupPreparation).toHaveBeenCalledTimes(1));
    await act(async () => fireEvent.click(button('只读刷新准备结果')));
    await act(async () => fireEvent.click(closeModal()));
    await act(async () => fireEvent.click(button('激活前准备')));
    await waitFor(() => expect(workorchApi.readStartupPreparation).toHaveBeenCalledTimes(3));
    expect(workorchApi.readConfiguration).not.toHaveBeenCalled();
    expect(workorchApi.listSkills).not.toHaveBeenCalled();
    expect(workorchApi.checkStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.recoverStartupPreparation).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
  });

  it('仅显式新执行编辑读取配置；等待期间禁止提交，关闭丢弃迟到配置，重开必须新读', async () => {
    const old = deferred<ServiceConfiguration>();
    vi.mocked(workorchApi.readConfiguration).mockReturnValueOnce(old.promise);
    render(page()); await screen.findByText('original goal');
    await act(async () => fireEvent.click(button('创建新执行')));
    expect(workorchApi.readConfiguration).toHaveBeenCalledTimes(1);
    expect(screen.getByText('正在读取新执行配置；尚未提交执行意图')).toBeTruthy();
    expect(button('创建新的后台执行').disabled).toBe(true);
    await act(async () => fireEvent.click(closeModal()));
    await act(async () => old.resolve(configuration('old-model')));
    expect(screen.queryByText(/系统默认：old-model/)).toBeNull();
    await act(async () => fireEvent.click(button('创建新执行')));
    expect(workorchApi.readConfiguration).toHaveBeenCalledTimes(2);
    expect(await screen.findByText(/系统默认：fresh-model/)).toBeTruthy();
    expect(button('创建新的后台执行').disabled).toBe(false);
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
    expect(workorchApi.listSkills).not.toHaveBeenCalled();
  });

  it('切换工作空间丢弃原编辑配置，新读取失败仍禁止提交', async () => {
    const old = deferred<ServiceConfiguration>();
    vi.mocked(workorchApi.readConfiguration).mockReturnValueOnce(old.promise).mockRejectedValueOnce(new Error('configuration unavailable'));
    const view = render(page()); await screen.findByText('original goal');
    await act(async () => fireEvent.click(button('创建新执行')));
    view.rerender(page('ws2'));
    await waitFor(() => expect(workorchApi.readWorkspaceSnapshotProjection).toHaveBeenLastCalledWith('ws2'));
    await act(async () => old.resolve(configuration('old-workspace-model')));
    expect(screen.queryByText(/系统默认：old-workspace-model/)).toBeNull();
    await act(async () => fireEvent.click(button('创建新执行')));
    expect(await screen.findByText('configuration unavailable')).toBeTruthy();
    expect(button('创建新的后台执行').disabled).toBe(true);
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
  });

  it('技能库存只在明确菜单动作查询，不连带读取模型或提交执行', async () => {
    render(page()); await screen.findByText('original goal');
    expect(workorchApi.listSkills).not.toHaveBeenCalled();
    await act(async () => fireEvent.mouseEnter(button('更多操作')));
    const query = await screen.findByText('查询技能库存（SkillHub status，可能保存状态／访问已配服务）');
    await act(async () => fireEvent.click(query));
    await waitFor(() => expect(workorchApi.listSkills).toHaveBeenCalledExactlyOnceWith('ws'));
    expect(workorchApi.readConfiguration).not.toHaveBeenCalled();
    expect(workorchApi.submitWorkspaceExecution).not.toHaveBeenCalled();
  });

  it('技能库存迟到响应不能进入另一个工作空间', async () => {
    const old = deferred<Awaited<ReturnType<typeof workorchApi.listSkills>>>();
    vi.mocked(workorchApi.listSkills).mockReturnValueOnce(old.promise);
    const view = render(page()); await screen.findByText('original goal');
    await act(async () => fireEvent.mouseEnter(button('更多操作')));
    const query = await screen.findByText('查询技能库存（SkillHub status，可能保存状态／访问已配服务）');
    await act(async () => fireEvent.click(query));
    view.rerender(page('ws2'));
    await waitFor(() => expect(workorchApi.readWorkspaceSnapshotProjection).toHaveBeenLastCalledWith('ws2'));
    await act(async () => old.resolve([{ id: 'old-skill' }] as Awaited<ReturnType<typeof workorchApi.listSkills>>));
    expect(screen.getByText('技能（Skill）（显式库存查询）: 未加载')).toBeTruthy();
    expect(workorchApi.listSkills).toHaveBeenCalledExactlyOnceWith('ws');
    expect(workorchApi.readConfiguration).not.toHaveBeenCalled();
  });
});
