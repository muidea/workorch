import type { Approval, ApprovalPage, ArtifactPage, ContextBundle, Decision, DecisionPage, DoD, EventRecord, Goal, MCPCapability, MCPPrompt, MCPResource, MCPServer, MCPTool, MemoryItem, MemoryRecallTrace, PatchPage, ResultPage, Run, RunObservability, RunSnapshot, RuntimeCheckpointPage, ServiceConfiguration, ServiceStatus, Skill, Soul, StepPage, TaskDecomposition, TaskProgress, ToolCallPage, UpdateLLMConfigurationRequest, UpdateServerConfigurationRequest, UpdateWebSearchConfigurationRequest, ValidationResultPage, Workspace, WorkspaceExecutionActivation, WorkspaceExecutionActivationRequest, WorkspaceSetupDraft, WorkspaceSetupDraftSession, WorkspaceSnapshot, WorkspaceSoulBindings, WorkspaceTemplate } from './types';

const apiBase = '/api/v1/workorch';

const inflightGets = new Map<string, Promise<unknown>>();

export function createWorkorchEventStream(query?: { workspaceId?: string; runId?: string; lastEventId?: string }) {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value !== undefined && value !== null && value !== '') {
      search.set(key, String(value));
    }
  }
  const suffix = search.toString() ? `?${search.toString()}` : '';
  return new EventSource(`${apiBase}/events/stream${suffix}`);
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const method = init?.method?.toUpperCase() || 'GET';
  const dedupeKey = method === 'GET' ? url : '';

  if (dedupeKey && inflightGets.has(dedupeKey)) {
    return inflightGets.get(dedupeKey) as Promise<T>;
  }

  const promise = doRequest<T>(url, init);

  if (dedupeKey) {
    inflightGets.set(dedupeKey, promise);
    promise.then(
      () => inflightGets.delete(dedupeKey),
      () => inflightGets.delete(dedupeKey)
    );
  }

  return promise;
}

async function doRequest<T>(url: string, init?: RequestInit): Promise<T> {
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

function querySuffix(extra?: Record<string, string | number | undefined>) {
 const search = new URLSearchParams();
  for (const [key, value] of Object.entries(extra ?? {})) {
    if (value !== undefined && value !== null && value !== '') {
      search.set(key, String(value));
    }
  }
  return search.toString() ? `?${search.toString()}` : '';
}

function runPageSuffix(query?: { cursorAfterId?: string; limit?: number }) {
  return querySuffix({
    cursorAfterId: query?.cursorAfterId,
    limit: query?.limit
  });
}

function runPath(workspaceId: string, runId: string, action?: string) {
  const encodedRunId = encodeURIComponent(runId);
  const suffix = action ? `/${action}` : '';
  return `${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/runs/${encodedRunId}${suffix}`;
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
  updateServerConfiguration: async (configuration: UpdateServerConfigurationRequest) => {
    const body = await request<{ configuration: ServiceConfiguration }>(`${apiBase}/configuration/server`, {
      method: 'PUT',
      body: JSON.stringify(configuration)
    });
    return body.configuration;
  },
  updateWebSearchConfiguration: async (configuration: UpdateWebSearchConfigurationRequest) => {
    const body = await request<{ configuration: ServiceConfiguration }>(`${apiBase}/configuration/web-search`, {
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
  createWorkspace: async (title: string, description?: string, deliverableDirectory?: string, templateId?: string) => {
    const body = await request<{ workspace: Workspace }>(`${apiBase}/workspaces`, {
      method: 'POST',
      body: JSON.stringify({ title, description, deliverableDirectory, templateId })
    });
    return body.workspace;
  },
  createWorkspaceSetupDraftSession: async (requestBody: { templateId?: string; title?: string; description?: string; deliverableDirectory?: string; dialogLanguage?: string }) => {
    const body = await request<{ draftSession: WorkspaceSetupDraftSession }>(`${apiBase}/workspace-setup-draft-sessions`, {
      method: 'POST',
      body: JSON.stringify(requestBody)
    });
    return body.draftSession;
  },
  appendWorkspaceSetupDraftMessage: async (sessionId: string, requestBody: { templateId?: string; title?: string; description?: string; deliverableDirectory?: string; content: string; draft?: WorkspaceSetupDraft; dialogLanguage?: string }) => {
    const body = await request<{ draftSession: WorkspaceSetupDraftSession }>(`${apiBase}/workspace-setup-draft-sessions/${encodeURIComponent(sessionId)}/messages`, {
      method: 'POST',
      body: JSON.stringify(requestBody)
    });
    return body.draftSession;
  },
  regenerateWorkspaceSetupDraft: async (sessionId: string, requestBody: { templateId?: string; title?: string; description?: string; deliverableDirectory?: string; content?: string; draft?: WorkspaceSetupDraft; dialogLanguage?: string }) => {
    const body = await request<{ draftSession: WorkspaceSetupDraftSession }>(`${apiBase}/workspace-setup-draft-sessions/${encodeURIComponent(sessionId)}/regenerate`, {
      method: 'POST',
      body: JSON.stringify(requestBody)
    });
    return body.draftSession;
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
  updateGoal: async (workspaceId: string, goalId: string, text: string) => {
    const body = await request<{ goal: Goal }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/goals/${encodeURIComponent(goalId)}`, {
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
  updateDoD: async (workspaceId: string, dodId: string, items: string[]) => {
    const body = await request<{ dod: DoD }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/dods/${encodeURIComponent(dodId)}`, {
      method: 'PUT',
      body: JSON.stringify({ items })
    });
    return body.dod;
  },
  listRuns: async () => {
    const body = await request<{ runs: Run[] }>(`${apiBase}/runs`);
    return body.runs ?? [];
  },
  readRunSnapshot: async (runId: string, workspaceId: string) => {
    const body = await request<{ snapshot: RunSnapshot }>(runPath(workspaceId, runId, 'snapshot'));
    return body.snapshot;
  },
  listRunSteps: async (runId: string, query: { workspaceId: string; cursorAfterId?: string; limit?: number }) => {
    const suffix = runPageSuffix(query);
    const body = await request<{ steps?: StepPage['steps']; page: StepPage }>(`${runPath(query.workspaceId, runId, 'steps')}${suffix}`);
    return {
      ...body.page,
      steps: body.steps ?? body.page.steps ?? []
    };
  },
  listRunToolCalls: async (runId: string, query: { workspaceId: string; cursorAfterId?: string; limit?: number }) => {
    const suffix = runPageSuffix(query);
    const body = await request<{ toolCalls?: ToolCallPage['toolCalls']; page: ToolCallPage }>(`${runPath(query.workspaceId, runId, 'tool-calls')}${suffix}`);
    return {
      ...body.page,
      toolCalls: body.toolCalls ?? body.page.toolCalls ?? []
    };
  },
  listRunRuntimeCheckpoints: async (runId: string, query: { workspaceId: string; cursorAfterId?: string; limit?: number }) => {
    const suffix = runPageSuffix(query);
    const body = await request<{ runtimeCheckpoints?: RuntimeCheckpointPage['runtimeCheckpoints']; page: RuntimeCheckpointPage }>(`${runPath(query.workspaceId, runId, 'runtime-checkpoints')}${suffix}`);
    return {
      ...body.page,
      runtimeCheckpoints: body.runtimeCheckpoints ?? body.page.runtimeCheckpoints ?? []
    };
  },
  listRunResults: async (runId: string, query: { workspaceId: string; cursorAfterId?: string; limit?: number }) => {
    const suffix = runPageSuffix(query);
    const body = await request<{ results?: ResultPage['results']; page: ResultPage }>(`${runPath(query.workspaceId, runId, 'results')}${suffix}`);
    return {
      ...body.page,
      results: body.results ?? body.page.results ?? []
    };
  },
  listRunPatches: async (runId: string, query: { workspaceId: string; cursorAfterId?: string; limit?: number }) => {
    const suffix = runPageSuffix(query);
    const body = await request<{ patches?: PatchPage['patches']; page: PatchPage }>(`${runPath(query.workspaceId, runId, 'patches')}${suffix}`);
    return {
      ...body.page,
      patches: body.patches ?? body.page.patches ?? []
    };
  },
  listRunValidationResults: async (runId: string, query: { workspaceId: string; cursorAfterId?: string; limit?: number }) => {
    const suffix = runPageSuffix(query);
    const body = await request<{ validationResults?: ValidationResultPage['validationResults']; page: ValidationResultPage }>(`${runPath(query.workspaceId, runId, 'validation-results')}${suffix}`);
    return {
      ...body.page,
      validationResults: body.validationResults ?? body.page.validationResults ?? []
    };
  },
  listRunArtifacts: async (runId: string, query: { workspaceId: string; cursorAfterId?: string; limit?: number }) => {
    const suffix = runPageSuffix(query);
    const body = await request<{ artifacts?: ArtifactPage['artifacts']; page: ArtifactPage }>(`${runPath(query.workspaceId, runId, 'artifacts')}${suffix}`);
    return {
      ...body.page,
      artifacts: body.artifacts ?? body.page.artifacts ?? []
    };
  },
  readRunObservability: async (runId: string, workspaceId: string) => {
    const body = await request<{ observability: RunObservability }>(runPath(workspaceId, runId, 'observability'));
    return body.observability;
  },
  readRunMemoryRecalls: async (runId: string, workspaceId: string) => {
    const body = await request<{ memoryRecallTraces: MemoryRecallTrace[] }>(runPath(workspaceId, runId, 'memory-recalls'));
    return body.memoryRecallTraces ?? [];
  },
  readWorkspaceMemoryRecalls: async (workspaceId: string) => {
    const body = await request<{ memoryRecallTraces: MemoryRecallTrace[] }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/memory-recalls`);
    return body.memoryRecallTraces ?? [];
  },
  artifactContentURL: (uri: string) => `${apiBase}/artifacts/content?${new URLSearchParams({ uri }).toString()}`,
  runtimePayloadURL: (ref: string) => `${apiBase}/runtime/payloads?${new URLSearchParams({ ref }).toString()}`,
  readRunTaskProgress: async (runId: string, workspaceId: string) => {
    const body = await request<{ taskProgress: TaskProgress }>(runPath(workspaceId, runId, 'task-progress'));
    return body.taskProgress;
  },
  readRunTaskDecompositions: async (runId: string, workspaceId: string) => {
    const body = await request<{ taskDecompositions: TaskDecomposition[] }>(runPath(workspaceId, runId, 'task-decompositions'));
    return body.taskDecompositions ?? [];
  },
  readRunContextBundle: async (runId: string, workspaceId: string) => {
    const body = await request<{ contextBundle: ContextBundle }>(runPath(workspaceId, runId, 'context-bundle'));
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
  startRun: async (runId: string, priority: number | undefined, workspaceId: string) => {
    const body = await request<{ run: Run }>(`${runPath(workspaceId, runId, 'start')}${querySuffix({ priority })}`, { method: 'POST' });
    return body.run;
  },
  resumeRun: async (runId: string, priority: number | undefined, workspaceId: string) => {
    const body = await request<{ run: Run }>(`${runPath(workspaceId, runId, 'resume')}${querySuffix({ priority })}`, { method: 'POST' });
    return body.run;
  },
  activateWorkspaceExecution: async (workspaceId: string, goalIdOrRequest?: string | WorkspaceExecutionActivationRequest, doDId?: string) => {
    const requestBody: WorkspaceExecutionActivationRequest = typeof goalIdOrRequest === 'object'
      ? goalIdOrRequest
      : { goalId: goalIdOrRequest, doDId };
    const activationRequest: WorkspaceExecutionActivationRequest = !requestBody.dialogLanguage && typeof navigator !== 'undefined'
      ? { ...requestBody, dialogLanguage: navigator.language }
      : requestBody;
    const body = await request<{ activation: WorkspaceExecutionActivation }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/execution-activations`, {
      method: 'POST',
      body: JSON.stringify(activationRequest)
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
  deleteWorkspaceTemplate: async (templateId: string) => {
    await request<Record<string, never>>(`${apiBase}/workspace-templates/${encodeURIComponent(templateId)}`, {
      method: 'DELETE'
    });
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
  updateMCPServer: async (serverId: string, server: MCPServer) => {
    const body = await request<{ server: MCPServer }>(`${apiBase}/mcp/servers/${encodeURIComponent(serverId)}`, {
      method: 'PUT',
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
  listMCPCapabilities: async (query?: { serverId?: string; kind?: string; category?: string; stage?: string; riskLevel?: string; enabled?: boolean; q?: string }) => {
    const search = new URLSearchParams();
    for (const [key, value] of Object.entries(query ?? {})) {
      if (value !== undefined && value !== null && value !== '') {
        search.set(key, String(value));
      }
    }
    const suffix = search.toString() ? `?${search.toString()}` : '';
    const body = await request<{ capabilities: MCPCapability[] }>(`${apiBase}/mcp/capabilities${suffix}`);
    return body.capabilities ?? [];
  },
  createMCPCapability: async (capability: MCPCapability) => {
    const body = await request<{ capability: MCPCapability }>(`${apiBase}/mcp/capabilities`, {
      method: 'POST',
      body: JSON.stringify(capability)
    });
    return body.capability;
  },
  updateMCPCapability: async (capabilityId: string, capability: MCPCapability) => {
    const body = await request<{ capability: MCPCapability }>(`${apiBase}/mcp/capabilities/${encodeURIComponent(capabilityId)}`, {
      method: 'PUT',
      body: JSON.stringify(capability)
    });
    return body.capability;
  },
  listMemory: async (workspaceId: string, query?: string) => {
    const search = new URLSearchParams({ workspaceId });
    if (query) {
      search.set('query', query);
    }
    const body = await request<{ memory: MemoryItem[] }>(`${apiBase}/memory?${search.toString()}`);
    return body.memory ?? [];
  },
  listDecisionPage: async (query?: { workspaceId?: string; runId?: string; cursorAfterId?: string; limit?: number }) => {
    const search = new URLSearchParams();
    for (const [key, value] of Object.entries(query ?? {})) {
      if (value !== undefined && value !== null && value !== '') {
        search.set(key, String(value));
      }
    }
    const suffix = search.toString() ? `?${search.toString()}` : '';
    const body = await request<{ decisions?: DecisionPage['decisions']; page: DecisionPage }>(`${apiBase}/decisions${suffix}`);
    return {
      ...body.page,
      decisions: body.decisions ?? body.page.decisions ?? []
    };
  },
  listDecisions: async () => {
    const body = await workorchApi.listDecisionPage();
    return body.decisions ?? [];
  },
  readDecision: async (ticketId: string) => {
    const body = await request<{ decision: Decision }>(`${apiBase}/decisions/${encodeURIComponent(ticketId)}`);
    return body.decision;
  },
  listApprovals: async () => {
    const body = await workorchApi.listApprovalPage();
    return body.approvals ?? [];
  },
  listApprovalPage: async (query?: { workspaceId?: string; runId?: string; cursorAfterId?: string; limit?: number }) => {
    const search = new URLSearchParams();
    for (const [key, value] of Object.entries(query ?? {})) {
      if (value !== undefined && value !== null && value !== '') {
        search.set(key, String(value));
      }
    }
    const suffix = search.toString() ? `?${search.toString()}` : '';
    const body = await request<{ approvals?: ApprovalPage['approvals']; page: ApprovalPage }>(`${apiBase}/approvals${suffix}`);
    return {
      ...body.page,
      approvals: body.approvals ?? body.page.approvals ?? []
    };
  },
  readApproval: async (approvalId: string) => {
    const body = await request<{ approval: Approval }>(`${apiBase}/approvals/${encodeURIComponent(approvalId)}`);
    return body.approval;
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
