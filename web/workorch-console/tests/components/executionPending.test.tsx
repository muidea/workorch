import { App, ConfigProvider } from 'antd';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { workorchApi } from '../../src/api';
import { WorkspaceBoardPage } from '../../src/pages/WorkspaceBoardPage';

vi.mock('../../src/api', () => ({ workorchApi: { listWorkspaceProjection: vi.fn(), readWorkspaceSnapshotProjection: vi.fn(), submitWorkspaceExecution: vi.fn() } }));
vi.mock('../../src/hooks', () => ({ useSSE: vi.fn() }));
vi.mock('../../src/pages/components/WorkspaceSetupModal', () => ({ WorkspaceSetupModal: () => null }));
vi.mock('../../src/pages/components/WorkspaceDraftDetails', () => ({ WorkspaceDraftDetails: () => null }));
vi.mock('../../src/pages/components/WorkspaceAuthoringCandidate', () => ({ WorkspaceAuthoringCandidate: () => null }));
vi.mock('../../src/pages/components/WorkspaceAuthoringConfirmation', () => ({ WorkspaceAuthoringConfirmation: () => null }));

describe('未决执行不是接受或拒绝', () => {
  it('Board 显示原身份，不宣称没有转发，不自动重试', async () => {
    vi.mocked(workorchApi.listWorkspaceProjection).mockResolvedValue({ workspaces: [{ id: 'ws', title: '测试工作空间', state: 'active' }] } as Awaited<ReturnType<typeof workorchApi.listWorkspaceProjection>>);
    vi.mocked(workorchApi.readWorkspaceSnapshotProjection).mockResolvedValue({ snapshot: { workspace: { id: 'ws', state: 'active' }, goals: [{ id: 'goal' }], dods: [{ id: 'dod', state: 'ready' }], runs: [] } } as unknown as Awaited<ReturnType<typeof workorchApi.readWorkspaceSnapshotProjection>>);
    vi.mocked(workorchApi.submitWorkspaceExecution).mockResolvedValue({ service: 'workorch', workspaceId: 'ws', requestId: 'current', intent: 'restart', outcome: 'fresh_intent_unresolved', reason: 'fresh_intent_result_unknown', accepted: false, state: 'pending', pendingRequestId: 'original', pendingStartupInputHash: 'startup', pendingPayloadHash: 'a'.repeat(64) });
    render(<ConfigProvider theme={{ token: { motion: false } }}><App><WorkspaceBoardPage onOpenWorkspace={vi.fn()} /></App></ConfigProvider>);
    await act(async () => {});
    await act(async () => fireEvent.click(screen.getByRole('button', { name: '提交执行' })));
    expect(await screen.findByText('原执行请求 original 结果未决；请保留原请求身份，不要换请求号重试。')).toBeTruthy();
    expect(screen.queryByText(/Workspace owner 拒绝执行请求|Workspace owner 已受理|本次未转发新执行/)).toBeNull();
    expect(workorchApi.submitWorkspaceExecution).toHaveBeenCalledExactlyOnceWith('ws', { intent: 'restart' });
    await act(async () => {});
    expect(workorchApi.submitWorkspaceExecution).toHaveBeenCalledTimes(1);
  });
});
