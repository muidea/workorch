import { describe, expect, it } from 'vitest';
import { workspaceProjectionRun, workspaceProjectionRunLabel } from '../../src/helpers';
import type { WorkspaceSnapshot } from '../../src/types';

describe('Workspace 与 Run 状态分离', () => {
  it('Workspace active 及进度状态不构造 Run', () => {
    const snapshot = { workspace: { id: 'ws', state: 'active' }, taskProgress: { state: 'active' }, runs: [] } as unknown as WorkspaceSnapshot;
    expect(workspaceProjectionRun(snapshot)).toBeUndefined();
    expect(workspaceProjectionRunLabel(snapshot)).toBe('尚未启动执行');
  });

  it('跨 Workspace 的同名 Run 不提供生命周期', () => {
    const snapshot = { workspace: { id: 'ws', activeRunId: 'run', state: 'active' }, taskProgress: { runId: 'run', state: 'running' }, runs: [{ id: 'run', workspaceId: 'other', state: 'failed' }] } as unknown as WorkspaceSnapshot;
    expect(workspaceProjectionRunLabel(snapshot)).toBe('尚未启动执行');
  });

  it('实际 Run owner 状态优先于进度及旧结果', () => {
    const snapshot = { workspace: { id: 'ws', activeRunId: 'new' }, displayOutcome: { runId: 'old', state: 'completed' }, taskProgress: { state: 'completed' }, runs: [{ id: 'old', workspaceId: 'ws', state: 'completed' }, { id: 'new', workspaceId: 'ws', state: 'running' }] } as unknown as WorkspaceSnapshot;
    expect(workspaceProjectionRunLabel(snapshot)).toBe('new / 运行中 (running)');
  });
});
