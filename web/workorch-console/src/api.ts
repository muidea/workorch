import type { Approval, ApprovalPage, ArtifactPage, ContextBundle, CreateWorkspaceRequest, Decision, DecisionPage, DoD, EventRecord, Goal, GoalDoDQualityCheck, GoalDoDQualityCheckRequest, MCPCapability, MCPPrompt, MCPResource, MCPServer, MCPTool, MemoryItem, MemoryRecallTrace, PatchPage, ProviderModelTarget, ResultPage, Run, RunObservability, RunSnapshot, RuntimeCheckpointPage, ServiceConfiguration, ServiceStatus, Skill, Soul, StepPage, TaskDecomposition, TaskProgress, ToolCallPage, UpdateLLMConfigurationRequest, UpdateServerConfigurationRequest, UpdateWebSearchConfigurationRequest, ValidationResultPage, Workspace, WorkspaceExecutionActivation, WorkspaceExecutionActivationRequest, WorkspaceExecutionContractDeclaration, WorkspaceExecutionIntentRequest, WorkspaceListProjection, WorkspaceSetupDraft, WorkspaceSetupDraftSession, WorkspaceSnapshotProjection, WorkspaceSoulBindings, WorkspaceTemplate } from './types';

const apiBase = '/api/v1/workorch';

const inflightGets = new Map<string, Promise<unknown>>();

export function createWorkorchEventStream(query?: { workspaceId?: string; runId?: string; lastEventId?: string; view?: string }) {
  const search = new URLSearchParams();
  search.set('view', query?.view || 'projection');
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

type RunCommandAcceptedResponse = {
  accepted?: boolean;
  workspaceId?: string;
  runId?: string;
  action?: string;
  intent?: string;
  requestId?: string;
  run?: Run;
};

type WorkspaceSetupDraftGenerationRequest = {
  templateId?: string;
  title?: string;
  description?: string;
  deliverableDirectory?: string;
  draft?: WorkspaceSetupDraft;
  dialogLanguage?: string;
};

type WorkspaceSetupDraftMessageRequest = WorkspaceSetupDraftGenerationRequest & {
  content: string;
};

function runCommandRequestId(intent: string, workspaceId: string, runId: string) {
  const suffix = Math.random().toString(36).slice(2, 10);
  return `web-${intent}-${workspaceId || 'workspace'}-${runId || 'run'}-${Date.now()}-${suffix}`;
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
  getLLMModelCatalog: async (providerId = 'ai-proxy') => {
    const query = new URLSearchParams({ providerId });
    const body = await request<{ catalog: import('./types').ProviderCatalogSummary }>(`${apiBase}/configuration/llm/models?${query.toString()}`);
    return body.catalog;
  },
  refreshLLMModelCatalog: async (providerId = 'ai-proxy') => {
    const body = await request<{ catalog: import('./types').ProviderCatalogSummary }>(`${apiBase}/configuration/llm/models/refresh`, {
      method: 'POST',
      body: JSON.stringify({ providerId })
    });
    return body.catalog;
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
  listWorkspaceProjection: async () => {
    const body = await request<WorkspaceListProjection>(`${apiBase}/workspaces`);
    return {
      projection: body.projection,
      workspaces: body.workspaces ?? []
    };
  },
  listWorkspaces: async () => {
    const body = await workorchApi.listWorkspaceProjection();
    return body.workspaces;
  },
  readWorkspaceSnapshotProjection: async (workspaceId: string) => {
    return request<WorkspaceSnapshotProjection>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/snapshot?view=detail`);
  },
  readWorkspaceSnapshot: async (workspaceId: string) => {
    const body = await workorchApi.readWorkspaceSnapshotProjection(workspaceId);
    return body.snapshot;
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
  updateWorkspaceLLMTarget: async (workspaceId: string, target: ProviderModelTarget) => {
    const body = await request<{ workspace: Workspace }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/llm-target`, {
      method: 'PUT',
      body: JSON.stringify({ providerId: target.providerId, modelId: target.modelId })
    });
    return body.workspace;
  },
  updateWorkspaceExecutionContract: async (workspaceId: string, declaration: WorkspaceExecutionContractDeclaration) => {
    const body = await request<{ workspace: Workspace }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/execution-contract`, {
      method: 'PUT',
      body: JSON.stringify(declaration)
    });
    return body.workspace;
  },
  createWorkspace: async (requestBody: CreateWorkspaceRequest) => {
    const body = await request<{ workspace: Workspace }>(`${apiBase}/workspaces`, {
      method: 'POST',
      body: JSON.stringify(requestBody)
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
  appendWorkspaceSetupDraftMessage: async (sessionId: string, requestBody: WorkspaceSetupDraftMessageRequest) => {
    const body = await request<{ draftSession: WorkspaceSetupDraftSession }>(`${apiBase}/workspace-setup-draft-sessions/${encodeURIComponent(sessionId)}/messages`, {
      method: 'POST',
      body: JSON.stringify(requestBody)
    });
    return body.draftSession;
  },
  regenerateWorkspaceSetupDraft: async (sessionId: string, requestBody: WorkspaceSetupDraftGenerationRequest) => {
    const body = await request<{ draftSession: WorkspaceSetupDraftSession }>(`${apiBase}/workspace-setup-draft-sessions/${encodeURIComponent(sessionId)}/regenerate`, {
      method: 'POST',
      body: JSON.stringify(requestBody)
    });
    return body.draftSession;
  },
  deleteWorkspace: async (workspaceId: string, purge?: boolean) => {
    // Workspace delete uses the documented query contract; the backend body path is reserved for non-browser clients.
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
  checkGoalDoDQuality: async (workspaceId: string, requestBody: GoalDoDQualityCheckRequest) => {
    const body = await request<{ qualityCheck: GoalDoDQualityCheck }>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/goal-dod-quality-check`, {
      method: 'POST',
      body: JSON.stringify(requestBody)
    });
    return body.qualityCheck;
  },
  listRuns: async (workspaceId?: string) => {
    const body = await request<{ runs: Run[] }>(`${apiBase}/runs${querySuffix({ workspaceId })}`);
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
  listEventRecords: async (query?: { workspaceId?: string; runId?: string; topic?: string; subjectType?: string; subjectId?: string; cursorAfterId?: string; limit?: number; includePayload?: boolean }) => {
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
  resumeWorkspaceExecution: async (workspaceId: string, goalIdOrRequest?: string | WorkspaceExecutionActivationRequest, doDId?: string) => {
    const requestBody: WorkspaceExecutionActivationRequest = typeof goalIdOrRequest === 'object'
      ? goalIdOrRequest
      : { goalId: goalIdOrRequest, doDId };
    const commandIntent = requestBody.intent ?? 'resume';
    // The activation endpoint is deliberately intent-only.  Context, model,
    // Soul, skill and capability selection are owner state; never forward the
    // legacy activation form fields even when an older caller still supplies
    // them locally.
    const commandRequest: WorkspaceExecutionIntentRequest = {
      intent: commandIntent,
      requestId: requestBody.requestId || runCommandRequestId(commandIntent, workspaceId, 'workspace'),
      reason: requestBody.reason || 'user_requested',
      source: requestBody.source || 'web',
      ...(requestBody.priority === undefined ? {} : { priority: requestBody.priority })
    };
    const body = await request<{ activation?: WorkspaceExecutionActivation } & RunCommandAcceptedResponse>(`${apiBase}/workspaces/${encodeURIComponent(workspaceId)}/execution-activations`, {
      method: 'POST',
      body: JSON.stringify(commandRequest)
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
      search.set('q', query);
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
  readDecision: async (workspaceId: string | undefined, ticketId: string) => {
    const suffix = querySuffix({ workspaceId });
    const body = await request<{ decision: Decision }>(`${apiBase}/decisions/${encodeURIComponent(ticketId)}${suffix}`);
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
  readApproval: async (workspaceId: string | undefined, approvalId: string) => {
    const suffix = querySuffix({ workspaceId });
    const body = await request<{ approval: Approval }>(`${apiBase}/approvals/${encodeURIComponent(approvalId)}${suffix}`);
    return body.approval;
  },
  resolveDecision: async (workspaceId: string, ticketId: string, resolutionType = 'provide_missing_info', resolutionPayload?: Record<string, unknown>) => {
    const suffix = querySuffix({ workspaceId });
    const body = await request<{ decision: Decision }>(`${apiBase}/decisions/${encodeURIComponent(ticketId)}/resolve${suffix}`, {
      method: 'POST',
      body: JSON.stringify({ resolutionType, resolutionPayload })
    });
    return body.decision;
  },
  batchResolveDecisions: async (workspaceId: string, ticketIds: string[], resolutionType = 'provide_missing_info', resolutionPayload?: Record<string, unknown>) => {
    const suffix = querySuffix({ workspaceId });
    const body = await request<{ decisions: Decision[] }>(`${apiBase}/decisions/resolve-batch${suffix}`, {
      method: 'POST',
      body: JSON.stringify({ ticketIds, resolutionType, resolutionPayload })
    });
    return body.decisions ?? [];
  },
  rejectDecision: async (workspaceId: string, ticketId: string) => {
    return workorchApi.resolveDecision(workspaceId, ticketId, 'reject_current_path', { operator: 'console' });
  },
  approveApproval: async (workspaceId: string, approvalId: string) => {
    const suffix = querySuffix({ workspaceId });
    const body = await request<{ approval: Approval }>(`${apiBase}/approvals/${encodeURIComponent(approvalId)}/approve${suffix}`, { method: 'POST' });
    return body.approval;
  },
  rejectApproval: async (workspaceId: string, approvalId: string) => {
    const suffix = querySuffix({ workspaceId });
    const body = await request<{ approval: Approval }>(`${apiBase}/approvals/${encodeURIComponent(approvalId)}/reject${suffix}`, { method: 'POST' });
    return body.approval;
  },
  batchResolveApprovals: async (workspaceId: string, approvalIds: string[], action: 'approve' | 'reject') => {
    const suffix = querySuffix({ workspaceId });
    const body = await request<{ approvals: Approval[] }>(`${apiBase}/approvals/resolve-batch${suffix}`, {
      method: 'POST',
      body: JSON.stringify({ approvalIds, action })
    });
    return body.approvals ?? [];
  }
};
