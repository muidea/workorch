import type { Approval, Decision, DoD, Goal, Run, ServiceStatus, SliceDescriptor, SliceStatus, Workspace } from './types';

const apiBase = '/api/v1/workorch';

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers
    }
  });

  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`;
    try {
      const body = await response.json();
      if (body?.error) {
        message = body.error;
      }
    } catch {
      // Keep the HTTP status as the error message when the response is not JSON.
    }
    throw new Error(message);
  }

  const text = await response.text();
  return (text ? JSON.parse(text) : {}) as T;
}

export const workorchApi = {
  healthz: () => request<ServiceStatus>('/healthz'),
  readyz: () => request<ServiceStatus>('/readyz'),
  metrics: () => request<string>('/metrics', { headers: { Accept: 'text/plain' } }),
  listWorkspaces: async () => {
    const body = await request<{ workspaces: Workspace[] }>(`${apiBase}/workspaces`);
    return body.workspaces ?? [];
  },
  createWorkspace: async (title: string, description?: string, workingDirectory?: string) => {
    const body = await request<{ workspace: Workspace }>(`${apiBase}/workspaces`, {
      method: 'POST',
      body: JSON.stringify({ title, description, workingDirectory })
    });
    return body.workspace;
  },
  listGoals: async (workspaceId: string) => {
    const body = await request<{ goals: Goal[] }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/goals`);
    return body.goals ?? [];
  },
  defineGoal: async (workspaceId: string, text: string) => {
    const body = await request<{ goal: Goal }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/goals`, {
      method: 'POST',
      body: JSON.stringify({ text })
    });
    return body.goal;
  },
  listDoDs: async (workspaceId: string) => {
    const body = await request<{ dods: DoD[] }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/dods`);
    return body.dods ?? [];
  },
  defineDoD: async (workspaceId: string, items: string[]) => {
    const body = await request<{ dod: DoD }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/dods`, {
      method: 'POST',
      body: JSON.stringify({ items })
    });
    return body.dod;
  },
  listRuns: async () => {
    const body = await request<{ runs: Run[] }>(`${apiBase}/runs`);
    return body.runs ?? [];
  },
  createRun: async (workspaceId: string, runId: string, goalId?: string, doDId?: string) => {
    const body = await request<{ run: Run }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/runs`, {
      method: 'POST',
      body: JSON.stringify({ runId, goalId, doDId })
    });
    return body.run;
  },
  startRun: async (runId: string) => {
    const body = await request<{ run: Run }>(`${apiBase}/runs/${encodeURIComponent(runId)}/start`, { method: 'POST' });
    return body.run;
  },
  resumeRun: async (runId: string) => {
    const body = await request<{ run: Run }>(`${apiBase}/runs/${encodeURIComponent(runId)}/resume`, { method: 'POST' });
    return body.run;
  },
  listDecisions: async () => {
    const body = await request<{ decisions: Decision[] }>(`${apiBase}/decisions`);
    return body.decisions ?? [];
  },
  resolveDecision: async (ticketId: string) => {
    const body = await request<{ decision: Decision }>(`${apiBase}/decisions/${encodeURIComponent(ticketId)}/resolve`, { method: 'POST' });
    return body.decision;
  },
  rejectDecision: async (ticketId: string) => {
    const body = await request<{ decision: Decision }>(`${apiBase}/decisions/${encodeURIComponent(ticketId)}/reject`, { method: 'POST' });
    return body.decision;
  },
  listApprovals: async () => {
    const body = await request<{ approvals: Approval[] }>(`${apiBase}/approvals`);
    return body.approvals ?? [];
  },
  approveApproval: async (approvalId: string) => {
    const body = await request<{ approval: Approval }>(`${apiBase}/approvals/${encodeURIComponent(approvalId)}/approve`, { method: 'POST' });
    return body.approval;
  },
  rejectApproval: async (approvalId: string) => {
    const body = await request<{ approval: Approval }>(`${apiBase}/approvals/${encodeURIComponent(approvalId)}/reject`, { method: 'POST' });
    return body.approval;
  },
  listSlices: async () => {
    const body = await request<{ slices: SliceDescriptor[] }>(`${apiBase}/slices`);
    return body.slices ?? [];
  },
  listSliceStatuses: async () => {
    const body = await request<{ statuses: SliceStatus[] }>(`${apiBase}/slices/statuses`);
    return body.statuses ?? [];
  },
  runSlice: async (sliceId: string) => request(`${apiBase}/slices/${encodeURIComponent(sliceId)}/runs`, { method: 'POST' })
};
