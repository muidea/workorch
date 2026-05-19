import type { Run, WorkspaceSnapshot, Goal, DoD } from '../types';

// 运行状态判断
export function isBlockedRunState(state?: string): boolean {
  return state === 'blocked_on_decision' || state === 'blocked_on_approval';
}

export function isCompletedRunState(state?: string): boolean {
  return state === 'completed' || state === 'accepted';
}

export function isFailedRunState(state?: string): boolean {
  return state === 'failed';
}

export function isTerminalRunState(state?: string): boolean {
  return isCompletedRunState(state) || isFailedRunState(state) || state === 'cancelled' || state === 'superseded';
}

// 待处理数量统计
export function pendingCount(items?: Array<{ state: string }>): number {
  return items?.filter((item) => item.state === 'open' || item.state === 'pending').length ?? 0;
}

// 运行状态摘要
export function runStateSummary(runs?: Run[]): { total: number; running: number; completed: number; failed: number; blocked: number } {
  const total = runs?.length ?? 0;
  const running = runs?.filter((r) => r.state === 'running').length ?? 0;
  const completed = runs?.filter((r) => isCompletedRunState(r.state)).length ?? 0;
  const failed = runs?.filter((r) => isFailedRunState(r.state)).length ?? 0;
  const blocked = runs?.filter((r) => isBlockedRunState(r.state)).length ?? 0;
  return { total, running, completed, failed, blocked };
}

// 从快照中提取活跃或最新运行
export function activeOrLatestRun(snapshot?: WorkspaceSnapshot): Run | undefined {
  if (!snapshot) {
    return undefined;
  }
  const { workspace, runs } = snapshot;
  if (workspace.activeRunId) {
    return runs?.find((r) => r.id === workspace.activeRunId);
  }
  return runs?.[0];
}

// 从快照中提取工作空间投影运行
export function workspaceProjectionRun(snapshot?: WorkspaceSnapshot): Run | undefined {
  if (!snapshot) {
    return undefined;
  }
  const { workspace, runs } = snapshot;
  if (workspace.activeRunId) {
    const activeRun = runs?.find((r) => r.id === workspace.activeRunId);
    if (activeRun) {
      return activeRun;
    }
  }
  const latestRun = runs?.[0];
  if (latestRun && !isTerminalRunState(latestRun.state)) {
    return latestRun;
  }
  return undefined;
}

// 工作空间投影运行标签
export function workspaceProjectionRunLabel(snapshot?: WorkspaceSnapshot): string {
  const run = workspaceProjectionRun(snapshot);
  return run ? `运行 ${run.id.slice(0, 10)}...` : '无活跃运行';
}

// 就绪的 DoD 列表
export function readyDoDs(dods: DoD[]): DoD[] {
  return dods.filter((item) => item.state === 'ready' || item.state === 'satisfied');
}

// 工作空间进度项
export function workspaceProcessItems(
  snapshot?: WorkspaceSnapshot,
  goals?: Goal[],
  dods?: DoD[]
): Array<{ key: string; label: string; value: string }> {
  const items: Array<{ key: string; label: string; value: string }> = [];

  if (goals && goals.length > 0) {
    items.push({ key: 'goals', label: '目标', value: `${goals.length} 个` });
  }

  if (dods && dods.length > 0) {
    const ready = readyDoDs(dods).length;
    items.push({ key: 'dods', label: '验收标准', value: `${ready}/${dods.length} 就绪` });
  }

  if (snapshot?.runs && snapshot.runs.length > 0) {
    const summary = runStateSummary(snapshot.runs);
    items.push({ key: 'runs', label: '运行', value: `${summary.total} 个` });
  }

  return items;
}
