import type { Approval, ContextBundle, Decision, DoD, EventRecord, Goal, MCPPrompt, MCPResource, MCPServer, MCPTool, MemoryItem, Run, RunObservability, RunSnapshot, ServiceConfiguration, ServiceStatus, Skill, Soul, TaskDecomposition, TaskProgress, UpdateLLMConfigurationRequest, Workspace, WorkspaceExecutionActivation, WorkspaceExecutionActivationRequest, WorkspaceSnapshot, WorkspaceSoulBindings, WorkspaceTemplate } from './types';

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

async function requestText(url: string, init?: RequestInit): Promise<string> {
  const response = await fetch(url, {
    ...init,
    headers: {
      ...init?.headers
    }
  });

  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}`);
  }

  return response.text();
}

function priorityQuery(priority?: number) {
  if (priority === undefined || priority === null || Number.isNaN(priority)) {
    return '';
  }
  return `?${new URLSearchParams({ priority: String(priority) }).toString()}`;
}

export const workorchApi = {
  healthz: () => request<ServiceStatus>('/healthz'),
  readyz: () => request<ServiceStatus>('/readyz'),
  metrics: () => requestText('/metrics', { headers: { Accept: 'text/plain' } }),
  readConfiguration: async () => {
    const body = await request<{ configuration: ServiceConfiguration }>(`${apiBase}/configuration`);
    return body.configuration;
  },
  updateLLMConfiguration: async (configuration: UpdateLLMConfigurationRequest) => {
    const body = await request<{ configuration: ServiceConfiguration }>(`${apiBase}/configuration/llm`, {
      method: 'PUT',
      body: JSON.stringify(configuration)
    });
    return body.configuration;
  },
  listWorkspaces: async () => {
    const body = await request<{ workspaces: Workspace[] }>(`${apiBase}/workspaces`);
    return body.workspaces ?? [];
  },
  readWorkspaceSnapshot: async (workspaceId: string) => {
    const body = await request<{ snapshot: WorkspaceSnapshot }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/snapshot`);
    return body.snapshot;
  },
  readWorkspaceTaskProgress: async (workspaceId: string) => {
    const body = await request<{ taskProgress: TaskProgress }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/task-progress`);
    return body.taskProgress;
  },
  readWorkspaceTaskDecompositions: async (workspaceId: string) => {
    const body = await request<{ taskDecompositions: TaskDecomposition[] }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/task-decompositions`);
    return body.taskDecompositions ?? [];
  },
  readWorkspaceSoulBindings: async (workspaceId: string) => {
    const body = await request<{ soulBindings: WorkspaceSoulBindings }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/soul-bindings`);
    return body.soulBindings;
  },
  updateWorkspaceSoulBindings: async (workspaceId: string, soulBindings: WorkspaceSoulBindings) => {
    const body = await request<{ soulBindings: WorkspaceSoulBindings }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/soul-bindings`, {
      method: 'PUT',
      body: JSON.stringify(soulBindings)
    });
    return body.soulBindings;
  },
  readWorkspaceContextBundle: async (workspaceId: string) => {
    const body = await request<{ contextBundle: ContextBundle }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/context-bundle`);
    return body.contextBundle;
  },
  createWorkspace: async (title: string, description?: string, workingDirectory?: string) => {
    const body = await request<{ workspace: Workspace }>(`${apiBase}/workspaces`, {
      method: 'POST',
      body: JSON.stringify({ title, description, workingDirectory })
    });
    return body.workspace;
  },
  deleteWorkspace: async (workspaceId: string, purge?: boolean) => {
    const suffix = purge ? `?${new URLSearchParams({ purge: 'true' }).toString()}` : '';
    await request<Record<string, never>>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}${suffix}`, {
      method: 'DELETE'
    });
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
  updateGoal: async (goalId: string, text: string) => {
    const body = await request<{ goal: Goal }>(`${apiBase}/goals/${encodeURIComponent(goalId)}`, {
      method: 'PUT',
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
  updateDoD: async (dodId: string, items: string[]) => {
    const body = await request<{ dod: DoD }>(`${apiBase}/dods/${encodeURIComponent(dodId)}`, {
      method: 'PUT',
      body: JSON.stringify({ items })
    });
    return body.dod;
  },
  listRuns: async () => {
    const body = await request<{ runs: Run[] }>(`${apiBase}/runs`);
    return body.runs ?? [];
  },
  readRunSnapshot: async (runId: string) => {
    const body = await request<{ snapshot: RunSnapshot }>(`${apiBase}/runs/${encodeURIComponent(runId)}/snapshot`);
    return body.snapshot;
  },
  readRunObservability: async (runId: string) => {
    const body = await request<{ observability: RunObservability }>(`${apiBase}/runs/${encodeURIComponent(runId)}/observability`);
    return body.observability;
  },
  artifactContentURL: (uri: string) => `${apiBase}/artifacts/content?${new URLSearchParams({ uri }).toString()}`,
  readRunTaskProgress: async (runId: string) => {
    const body = await request<{ taskProgress: TaskProgress }>(`${apiBase}/runs/${encodeURIComponent(runId)}/task-progress`);
    return body.taskProgress;
  },
  readRunTaskDecompositions: async (runId: string) => {
    const body = await request<{ taskDecompositions: TaskDecomposition[] }>(`${apiBase}/runs/${encodeURIComponent(runId)}/task-decompositions`);
    return body.taskDecompositions ?? [];
  },
  readRunContextBundle: async (runId: string) => {
    const body = await request<{ contextBundle: ContextBundle }>(`${apiBase}/runs/${encodeURIComponent(runId)}/context-bundle`);
    return body.contextBundle;
  },
  listEventRecords: async (query?: { workspaceId?: string; runId?: string; topic?: string; subjectType?: string; subjectId?: string; cursorAfterId?: string; limit?: number }) => {
    const search = new URLSearchParams();
    for (const [key, value] of Object.entries(query ?? {})) {
      if (value !== undefined && value !== null && value !== '') {
        search.set(key, String(value));
      }
    }
    const suffix = search.toString() ? `?${search.toString()}` : '';
    const body = await request<{ eventRecords: EventRecord[] }>(`${apiBase}/events${suffix}`);
    return body.eventRecords ?? [];
  },
  createRun: async (workspaceId: string, runId: string, goalId?: string, doDId?: string) => {
    const body = await request<{ run: Run }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/runs`, {
      method: 'POST',
      body: JSON.stringify({ runId, goalId, doDId })
    });
    return body.run;
  },
  startRun: async (runId: string, priority?: number) => {
    const body = await request<{ run: Run }>(`${apiBase}/runs/${encodeURIComponent(runId)}/start${priorityQuery(priority)}`, { method: 'POST' });
    return body.run;
  },
  resumeRun: async (runId: string, priority?: number) => {
    const body = await request<{ run: Run }>(`${apiBase}/runs/${encodeURIComponent(runId)}/resume${priorityQuery(priority)}`, { method: 'POST' });
    return body.run;
  },
  activateWorkspaceExecution: async (workspaceId: string, goalIdOrRequest?: string | WorkspaceExecutionActivationRequest, doDId?: string) => {
    const requestBody: WorkspaceExecutionActivationRequest = typeof goalIdOrRequest === 'object'
      ? goalIdOrRequest
      : { goalId: goalIdOrRequest, doDId };
    const body = await request<{ activation: WorkspaceExecutionActivation }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/execution-activations`, {
      method: 'POST',
      body: JSON.stringify(requestBody)
    });
    return body.activation;
  },
  listSouls: async () => {
    const body = await request<{ souls: Soul[] }>(`${apiBase}/souls`);
    return body.souls ?? [];
  },
  createSoul: async (soul: Soul) => {
    const body = await request<{ soul: Soul }>(`${apiBase}/souls`, {
      method: 'POST',
      body: JSON.stringify(soul)
    });
    return body.soul;
  },
  updateSoul: async (soulId: string, soul: Soul) => {
    const body = await request<{ soul: Soul }>(`${apiBase}/souls/${encodeURIComponent(soulId)}`, {
      method: 'PUT',
      body: JSON.stringify(soul)
    });
    return body.soul;
  },
  deleteSoul: async (soulId: string) => {
    await request<Record<string, never>>(`${apiBase}/souls/${encodeURIComponent(soulId)}`, {
      method: 'DELETE'
    });
  },
  listWorkspaceTemplates: async () => {
    const body = await request<{ workspaceTemplates: WorkspaceTemplate[] }>(`${apiBase}/workspace-templates`);
    return body.workspaceTemplates ?? [];
  },
  createWorkspaceTemplate: async (template: WorkspaceTemplate) => {
    const body = await request<{ workspaceTemplate: WorkspaceTemplate }>(`${apiBase}/workspace-templates`, {
      method: 'POST',
      body: JSON.stringify(template)
    });
    return body.workspaceTemplate;
  },
  updateWorkspaceTemplate: async (templateId: string, template: WorkspaceTemplate) => {
    const body = await request<{ workspaceTemplate: WorkspaceTemplate }>(`${apiBase}/workspace-templates/${encodeURIComponent(templateId)}`, {
      method: 'PUT',
      body: JSON.stringify(template)
    });
    return body.workspaceTemplate;
  },
  listSkills: async (workspaceId?: string) => {
    const search = new URLSearchParams();
    if (workspaceId) {
      search.set('workspaceId', workspaceId);
    }
    const suffix = search.toString() ? `?${search.toString()}` : '';
    const body = await request<{ skills: Skill[] }>(`${apiBase}/skills${suffix}`);
    return body.skills ?? [];
  },
  listMCPServers: async () => {
    const body = await request<{ servers: MCPServer[] }>(`${apiBase}/mcp/servers`);
    return body.servers ?? [];
  },
  createMCPServer: async (server: MCPServer) => {
    const body = await request<{ server: MCPServer }>(`${apiBase}/mcp/servers`, {
      method: 'POST',
      body: JSON.stringify(server)
    });
    return body.server;
  },
  listMCPServerTools: async (serverId: string) => {
    const body = await request<{ tools: MCPTool[] }>(`${apiBase}/mcp/servers/${encodeURIComponent(serverId)}/tools`);
    return body.tools ?? [];
  },
  listMCPServerResources: async (serverId: string) => {
    const body = await request<{ resources: MCPResource[] }>(`${apiBase}/mcp/servers/${encodeURIComponent(serverId)}/resources`);
    return body.resources ?? [];
  },
  listMCPServerPrompts: async (serverId: string) => {
    const body = await request<{ prompts: MCPPrompt[] }>(`${apiBase}/mcp/servers/${encodeURIComponent(serverId)}/prompts`);
    return body.prompts ?? [];
  },
  listMemory: async (workspaceId: string, query?: string) => {
    const search = new URLSearchParams({ workspaceId });
    if (query) {
      search.set('query', query);
    }
    const body = await request<{ memory: MemoryItem[] }>(`${apiBase}/memory?${search.toString()}`);
    return body.memory ?? [];
  },
  listDecisions: async () => {
    const body = await request<{ decisions: Decision[] }>(`${apiBase}/decisions`);
    return body.decisions ?? [];
  },
  resolveDecision: async (ticketId: string, resolutionType = 'provide_missing_info', resolutionPayload?: Record<string, unknown>) => {
    const body = await request<{ decision: Decision }>(`${apiBase}/decisions/${encodeURIComponent(ticketId)}/resolve`, {
      method: 'POST',
      body: JSON.stringify({ resolutionType, resolutionPayload })
    });
    return body.decision;
  },
  batchResolveDecisions: async (ticketIds: string[], resolutionType = 'provide_missing_info', resolutionPayload?: Record<string, unknown>) => {
    const body = await request<{ decisions: Decision[] }>(`${apiBase}/decisions/resolve-batch`, {
      method: 'POST',
      body: JSON.stringify({ ticketIds, resolutionType, resolutionPayload })
    });
    return body.decisions ?? [];
  },
  rejectDecision: async (ticketId: string) => {
    return workorchApi.resolveDecision(ticketId, 'reject_current_path', { operator: 'console' });
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
  batchResolveApprovals: async (approvalIds: string[], action: 'approve' | 'reject') => {
    const body = await request<{ approvals: Approval[] }>(`${apiBase}/approvals/resolve-batch`, {
      method: 'POST',
      body: JSON.stringify({ approvalIds, action })
    });
    return body.approvals ?? [];
  }
};
