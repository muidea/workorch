export type ServiceStatus = {
  service: string;
  status: string;
};

export type ServiceConfiguration = {
  server: ServerConfiguration;
  repository: RepositoryConfiguration;
  llm: LLMConfiguration;
};

export type ServerConfiguration = {
  addr?: string;
  readHeaderTimeout?: string;
  readTimeout?: string;
  writeTimeout?: string;
  idleTimeout?: string;
  shutdownTimeout?: string;
  maxConcurrentRuns?: number;
};

export type RepositoryConfiguration = {
  backend?: string;
  dataDir?: string;
};

export type LLMConfiguration = {
  defaultProviderId?: string;
  defaultEmbeddingProviderId?: string;
  providers?: LLMProviderConfiguration[];
};

export type LLMProviderConfiguration = {
  id: string;
  provider?: string;
  baseUrl?: string;
  model?: string;
  timeout?: string;
  disabled: boolean;
  ready: boolean;
  reason?: string;
  default: boolean;
  apiKeyConfigured: boolean;
  chatCompletions: boolean;
  streaming: boolean;
  nonChatOperations?: string[];
};

export type UpdateLLMConfigurationRequest = {
  defaultProviderId?: string;
  defaultEmbeddingProviderId?: string;
  providers: UpdateLLMProviderConfiguration[];
};

export type UpdateLLMProviderConfiguration = {
  id: string;
  provider?: string;
  baseUrl?: string;
  apiKey?: string;
  model?: string;
  timeout?: string;
  disabled?: boolean;
  chatCompletions?: boolean;
  streaming?: boolean;
  nonChatOperations?: string[];
};

export type Workspace = {
  id: string;
  title: string;
  description?: string;
  workingDirectory: string;
  state: string;
  activeRunId?: string;
};

export type Goal = {
  id: string;
  workspaceId: string;
  text: string;
};

export type DoD = {
  id: string;
  workspaceId: string;
  items: string[];
  state: string;
};

export type Run = {
  id: string;
  workspaceId: string;
  goalId?: string;
  doDId?: string;
  initialStepKind?: string;
  executionContext?: Record<string, unknown>;
  state: string;
  priority?: number;
  blockingTicketId?: string;
  blockingApprovalId?: string;
  failureReason?: string;
};

export type WorkspaceExecutionActivationRequest = {
  goalId?: string;
  doDId?: string;
  llmProviderId?: string;
  soulId?: string;
  skillIds?: string[];
  mcpServerIds?: string[];
  memoryIds?: string[];
  priority?: number;
};

export type Soul = {
  id: string;
  name: string;
  description?: string;
};

export type Skill = {
  id: string;
  name: string;
  version: string;
  description?: string;
  requiredMCPServerRefs?: string[];
  requiredToolKinds?: string[];
};

export type MCPServer = {
  id: string;
  name: string;
  transport: string;
  endpoint?: string;
  enabled: boolean;
  allowedTools?: string[];
  allowedResources?: string[];
  allowedPrompts?: string[];
  defaultApprovalPolicy?: string;
  sessionPolicy?: string;
  timeoutMillis?: number;
};

export type MCPTool = {
  serverId: string;
  name: string;
};

export type MCPResource = {
  serverId: string;
  uri: string;
};

export type MCPPrompt = {
  serverId: string;
  name: string;
};

export type MemoryItem = {
  id: string;
  workspaceId: string;
  runId?: string;
  sourceType: string;
  sourceRef?: string;
  memoryType: string;
  summary: string;
  tags?: string[];
  embeddingProviderId?: string;
  embeddingModel?: string;
  embeddingText?: string;
  vectorReady?: boolean;
  vectorDimension?: number;
  vectorScore?: number;
};

export type StepView = {
  id: string;
  runId: string;
  kind?: string;
  state: string;
  input?: Record<string, unknown>;
  output?: Record<string, unknown>;
};

export type ToolCall = {
  id: string;
  workspaceId: string;
  runId: string;
  stepId?: string;
  toolName: string;
  state: string;
  exitSummary?: string;
  error?: string;
};

export type Patch = {
  id: string;
  workspaceId: string;
  runId: string;
  stepId?: string;
  patchType: string;
  targetRef?: string;
  summary?: string;
  state: string;
};

export type ValidationResult = {
  id: string;
  workspaceId: string;
  runId: string;
  stepId?: string;
  validationKind: string;
  targetRef?: string;
  state: string;
  summary?: string;
};

export type RuntimeCheckpoint = {
  id: string;
  workspaceId: string;
  runId: string;
  checkpointType: string;
  currentStepId?: string;
  runStateSnapshot: string;
  blockingTicketId?: string;
  blockingApprovalId?: string;
};

export type RunProgress = {
  state: string;
  currentStepId?: string;
  currentStepKind?: string;
  currentStepState?: string;
  stepTotal: number;
  stepDispatched: number;
  stepCompleted: number;
  stepFailed: number;
  blocked: boolean;
  terminal: boolean;
};

export type RuntimeEvidenceSummary = {
  toolCallTotal: number;
  toolCallRunning: number;
  toolCallCompleted: number;
  toolCallFailed: number;
  patchTotal: number;
  patchApplied: number;
  patchFailed: number;
  validationTotal: number;
  validationPassed: number;
  validationFailed: number;
  validationWarning: number;
  runtimeCheckpointTotal: number;
  eventTotal: number;
};

export type RuntimeTimelineItem = {
  source: string;
  type: string;
  id?: string;
  topic?: string;
  runState?: string;
  stepId?: string;
  stepState?: string;
  summary?: string;
  occurredAt?: string;
  data?: Record<string, unknown>;
  output?: Record<string, unknown>;
};

export type RunObservability = {
  run: Run;
  progress: RunProgress;
  evidence: RuntimeEvidenceSummary;
  latestCheckpoint?: RuntimeCheckpoint;
  timeline?: RuntimeTimelineItem[];
};

export type RunSnapshot = {
  workspace: Workspace;
  run: Run;
  steps?: StepView[];
  toolCalls?: ToolCall[];
  patches?: Patch[];
  validationResults?: ValidationResult[];
  runtimeCheckpoints?: RuntimeCheckpoint[];
  decisions?: Decision[];
  approvals?: Approval[];
};

export type WorkspaceSnapshot = {
  workspace: Workspace;
  runs?: Run[];
  decisions?: Decision[];
  approvals?: Approval[];
};

export type WorkspaceExecutionActivation = {
  workspace: Workspace;
  run: Run;
  snapshot: WorkspaceSnapshot;
};

export type Decision = {
  id: string;
  workspaceId: string;
  runId: string;
  stepId?: string;
  reason: string;
  decisionKind?: string;
  question?: string;
  context?: Record<string, unknown>;
  state: string;
  resolutionId?: string;
  resolutionType?: string;
  resolutionPayload?: Record<string, unknown>;
};

export type Approval = {
  id: string;
  workspaceId: string;
  runId: string;
  stepId?: string;
  reason: string;
  state: string;
};
