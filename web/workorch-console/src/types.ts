export type ServiceConfiguration = {
  server: ServerConfiguration;
  repository: RepositoryConfiguration;
  runtime: RuntimeConfiguration;
  llm: LLMConfiguration;
  webSearch: WebSearchConfiguration;
  governance: GovernanceConfiguration;
};

export type ServerConfiguration = {
  addr?: string;
  readHeaderTimeout?: string;
  readTimeout?: string;
  writeTimeout?: string;
  idleTimeout?: string;
  shutdownTimeout?: string;
  maxConcurrentRuns?: number;
  defaultDialogLanguage?: string;
};

export type RepositoryConfiguration = {
  backend?: string;
  dataDir?: string;
};

export type RuntimeConfiguration = {
  defaultSoulId?: string;
  maxRuntimeRepairAttempts?: number;
  maxProviderRetryAttempts?: number;
  shellCommandTimeout?: string;
  git?: RuntimeGitConfiguration;
  llmOutput?: RuntimeLLMOutputConfiguration;
};

export type RuntimeGitConfiguration = {
  defaultTimeout?: string;
};

export type RuntimeLLMOutputConfiguration = {
  maxPatchContentBytes?: number;
  maxGraphBytes?: number;
  maxGraphFileWrites?: number;
  schemaInvalidLargePreviewThreshold?: number;
  maxArtifactChunkBytes?: number;
  previousChunkTailBytes?: number;
  maxChunkRetryAttempts?: number;
  maxArtifactPlanAttempts?: number;
  maxArtifactReplanAttempts?: number;
};

export type ProviderModelTarget = {
  providerId: string;
  modelId: string;
};

export type AIProxyConfigurationView = {
  baseUrl?: string;
  apiKeyConfigured: boolean;
  timeout?: string;
  catalogRequestTimeout?: string;
  catalogTTL?: string;
  catalogMaxStaleAge?: string;
  maxCatalogResponseBytes?: number;
  maxCatalogModels?: number;
};

export type LLMContextBudgetPolicyView = {
  contextUtilizationRatio?: number;
  estimatedTokensPerByte?: number;
  defaultMaxOutputTokens?: number;
  maxRequestBytes?: number;
};

export type CustomProviderModelView = {
  id: string;
  contextWindowTokens: number;
  maxOutputTokens: number;
  operations: string[];
  temperature?: number;
  reasoningEffort?: string;
  thinkingBudgetTokens?: number;
  cacheMode?: string;
  promptCacheKey?: boolean;
  messageCacheControl?: boolean;
  cacheUsageFields?: string[];
};

export type CustomProviderConfigurationView = {
  id: string;
  provider: string;
  apiSurface?: string;
  baseUrl?: string;
  timeout?: string;
  apiKeyConfigured: boolean;
  models: CustomProviderModelView[];
};

export type CustomProviderModelDraft = {
  id: string;
  contextWindowTokens?: number;
  maxOutputTokens?: number;
  operations?: string[];
  temperature?: number;
  reasoningEffort?: string;
  thinkingBudgetTokens?: number;
  cacheMode?: string;
  promptCacheKey?: boolean;
  messageCacheControl?: boolean;
  cacheUsageFields?: string[];
};

export type CustomProviderDraft = {
  id: string;
  provider: string;
  apiSurface?: string;
  baseUrl?: string;
  apiKey?: string;
  /** When true, explicitly clear the saved API key for this provider. */
  clearApiKey?: boolean;
  timeout?: string;
  models: CustomProviderModelDraft[];
};

export type ProviderModelDescriptorView = {
  providerId: string;
  modelId: string;
  routeOwner?: string;
  contextWindowTokens?: number;
  maxOutputTokens?: number;
  operations?: string[];
  capabilityReady: boolean;
  unavailableReason?: string;
  capabilityRevision?: string;
};

export type ProviderCatalogSummary = {
  providerId: string;
  state?: string;
  revision?: string;
  fetchedAt?: string;
  expiresAt?: string;
  stale?: boolean;
  source?: string;
  errorCode?: string;
  errorMessage?: string;
  models?: ProviderModelDescriptorView[];
};

export type LLMConfiguration = {
  aiProxy: AIProxyConfigurationView;
  defaultChat: ProviderModelTarget;
  defaultEmbedding?: ProviderModelTarget;
  budget: LLMContextBudgetPolicyView;
  customProviders?: CustomProviderConfigurationView[];
  catalog?: ProviderCatalogSummary;
};

export type UpdateLLMConfigurationRequest = {
  aiProxy?: {
    baseURL?: string;
    apiKey?: string;
    timeout?: string;
    catalogRequestTimeout?: string;
    catalogTTL?: string;
    catalogMaxStaleAge?: string;
    maxCatalogResponseBytes?: number;
    maxCatalogModels?: number;
  };
  defaultChat?: {
    providerID?: string;
    modelID?: string;
  };
  defaultEmbedding?: {
    providerID?: string;
    modelID?: string;
  };
  clearDefaultEmbedding?: boolean;
  clearAIProxyApiKey?: boolean;
  clearCustomProviderApiKeys?: string[];
  budget?: {
    contextUtilizationRatio?: number;
    estimatedTokensPerByte?: number;
    defaultMaxOutputTokens?: number;
    maxRequestBytes?: number;
  };
  customProviders?: Record<string, {
    provider?: string;
    apiSurface?: string;
    baseURL?: string;
    apiKey?: string;
    timeout?: string;
    models?: Record<string, {
      contextWindowTokens?: number;
      maxOutputTokens?: number;
      operations?: string[];
      temperature?: number;
      reasoningEffort?: string;
      thinkingBudgetTokens?: number;
      cache?: {
        mode?: string;
        promptCacheKey?: boolean;
        messageCacheControl?: boolean;
        usageFields?: string[];
      };
    }>;
  }>;
};

export type UpdateServerConfigurationRequest = {
  defaultDialogLanguage?: string;
};

export type WebSearchConfiguration = {
  defaultProviderId?: string;
  providers?: WebSearchProviderConfiguration[];
  providerTemplates?: WebSearchProviderTemplate[];
};

export type WebSearchProviderConfiguration = {
  id: string;
  provider: string;
  searchEndpoint?: string;
  fetchEndpoint?: string;
  headers?: Record<string, string>;
  timeout?: string;
  maxResults?: number;
  maxFetchBytes?: number;
  maxRedirects?: number;
  allowDomains?: string[];
  denyDomains?: string[];
  disabled: boolean;
  ready: boolean;
  reason?: string;
  default: boolean;
  apiKeyConfigured: boolean;
};

export type WebSearchProviderTemplate = {
  templateId: string;
  name: string;
  description?: string;
  id: string;
  provider: string;
  searchEndpoint?: string;
  fetchEndpoint?: string;
  timeout?: string;
  maxResults?: number;
  maxFetchBytes?: number;
  maxRedirects?: number;
};

export type UpdateWebSearchConfigurationRequest = {
  defaultProviderId?: string;
  providers: UpdateWebSearchProviderConfiguration[];
};

export type UpdateWebSearchProviderConfiguration = {
  id: string;
  provider?: string;
  searchEndpoint?: string;
  fetchEndpoint?: string;
  headers?: Record<string, string>;
  apiKey?: string;
  timeout?: string;
  maxResults?: number;
  maxFetchBytes?: number;
  maxRedirects?: number;
  allowDomains?: string[];
  denyDomains?: string[];
  disabled?: boolean;
};

export type GovernanceConfiguration = {
  autoTicket: GovernanceAutoTicketConfiguration;
  autoApproval: GovernanceAutoApprovalConfiguration;
  budget: GovernanceBudgetConfiguration;
};

export type GovernanceAutoTicketConfiguration = {
  enabled: boolean;
  minConfidence: number;
  maxPerRun: number;
};

export type GovernanceAutoApprovalConfiguration = {
  enabled: boolean;
  minConfidence: number;
  maxPerRun: number;
  allowReadOnly: boolean;
  allowWorkspaceWrite: boolean;
  requireHumanFor?: string[];
};

export type GovernanceBudgetConfiguration = {
  maxConsecutiveFailures: number;
  maxRepeatedSamePrompt: number;
};

export type Workspace = {
  id: string;
  title: string;
  description?: string;
  deliverableDirectory: string;
  state: string;
  activeRunId?: string;
  // Mutable workspace preference. It applies only to the next activation.
  defaultLLMTarget?: ProviderModelTarget;
  defaultSoulId?: string;
  soulBindings?: Record<string, string>;
};

export type ProjectionStatus = {
  state: string;
  globalRevision: number;
  workspaceRevision?: number;
  sourceEventRevision?: number;
  projectedSourceRevision?: number;
  workspaceTotal?: number;
  updatedAt?: string;
  rebuiltAt?: string;
  degraded?: boolean;
  staleReason?: string;
  error?: string;
};

export type ServiceStatus = {
  service?: string;
  status?: string;
  projection?: ProjectionStatus;
  [key: string]: unknown;
};

export type WorkspaceListProjection = {
  projection: ProjectionStatus;
  workspaces: Workspace[];
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

export type GoalDoDQualityIssue = {
  code: string;
  severity: string;
  target: string;
  message: string;
  suggestion?: string;
};

export type GoalDoDQualityCheck = {
  workspaceId: string;
  goalId?: string;
  doDId?: string;
  goalText?: string;
  doDItems?: string[];
  executable: boolean;
  verifiable: boolean;
  executionScore: number;
  verificationScore: number;
  status: string;
  missingConstraints?: string[];
  suggestions?: string[];
  issues?: GoalDoDQualityIssue[];
};

export type GoalDoDQualityCheckRequest = {
  goalId?: string;
  doDId?: string;
  goalText?: string;
  doDItems?: string[];
};

export type Run = {
  id: string;
  workspaceId: string;
  goalId?: string;
  doDId?: string;
  initialStepKind?: string;
  state: string;
  priority?: number;
  blockingTicketId?: string;
  blockingApprovalId?: string;
  failureReason?: string;
  // Immutable Runtime V2 activation input; never send it back on Resume.
  sealedLLMTarget?: ProviderModelTarget;
};

export type RunHeartbeat = {
  workspaceId?: string;
  runId?: string;
  state?: string;
  activityState?: string;
  latestStepId?: string;
  latestStepKind?: string;
  latestStepState?: string;
  latestEvidenceRef?: string;
  latestEventTopic?: string;
  latestEventRevision?: number;
  latestEventAt?: string;
  latestLLMUsageAt?: string;
  latestResultAt?: string;
  latestArtifactAt?: string;
  latestPatchAt?: string;
  latestEffectiveProgressAt?: string;
  ineffectiveRetry?: boolean;
  stalled?: boolean;
  summary?: string;
};

export type RuntimeExecution = {
  stepId?: string;
  stepKind?: string;
  stepState?: string;
  requestId?: string;
  queueKey?: string;
  acceptedAt?: string;
  lastProgressAt?: string;
  lastProgressEvent?: string;
  lastProgressStage?: string;
  lastProgressSummary?: string;
  workerId?: string;
  workerStartedAt?: string;
  workerHeartbeatAt?: string;
  workerStoppedAt?: string;
  workerLostAt?: string;
  doneAt?: string;
  failedAt?: string;
  finalizedAt?: string;
  finalizeEvent?: string;
  pending?: boolean;
  gap?: string;
};

export type ConvergenceStatus = {
  workspaceId?: string;
  runId?: string;
  runState?: string;
  currentSliceId?: string;
  currentFocusId?: string;
  nextActionKind?: string;
  currentGap?: string;
  missingEvidenceKinds?: string[];
  acceptedEvidenceCount?: number;
  rejectedEvidenceCount?: number;
  closedCoverageItemCount?: number;
  remainingCoverageItemCount?: number;
  assessmentDisposition?: string;
  repairAttempts?: number;
  maxRepairAttempts?: number;
  repeatedFailureSignatureCount?: number;
  latestAcceptedEvidenceRef?: string;
  nextActivationRequired?: boolean;
  gapSummary?: string;
  latestEffectiveProgressAt?: string;
  lastProgressAt?: string;
  latestContractViolationRule?: string;
  latestRecovery?: string;
  latestRejectedDiagnosticRef?: string;
  decisionCause?: string;
  retryAttempt?: number;
  retryMaxAttempts?: number;
  retrySignatureRepeatCount?: number;
  retryBudgetScope?: Record<string, unknown>;
  narrowedRetryAttempted?: boolean;
  managedValidationAvailable?: boolean;
  stalled?: boolean;
  ineffectiveRetry?: boolean;
};

export type RunCommandStatus = {
  workspaceId?: string;
  runId?: string;
  state?: string;
  intent?: string;
  reason?: string;
  requestId?: string;
  idempotencyKey?: string;
  source?: string;
  definitionRevision?: string;
  decisionId?: string;
  acceptedAt?: string;
  rejectedAt?: string;
  error?: string;
  revision?: number;
};

export type RunOutcome = {
  runId?: string;
  state: string;
  rawState?: string;
  accepted: boolean;
  terminal: boolean;
  resultId?: string;
  summary?: string;
  reason?: string;
  evidenceRefs?: string[];
  artifactRefs?: string[];
};

export type WorkspaceOutcome = {
  workspaceId?: string;
  state: string;
  rawWorkspaceState?: string;
  runId?: string;
  resultId?: string;
  accepted: boolean;
  terminal: boolean;
  summary?: string;
  reason?: string;
  evidenceRefs?: string[];
  artifactRefs?: string[];
};

export type WorkspaceExecutionActivationRequest = {
  runId?: string;
  goalId?: string;
  doDId?: string;
  llmTarget?: ProviderModelTarget;
  soulId?: string;
  soulBindings?: Record<string, string>;
  skillIds?: string[];
  mcpServerIds?: string[];
  capabilityIds?: string[];
  memoryIds?: string[];
  dialogLanguage?: string;
  priority?: number;
  // Console has one runtime continuation verb. Replan remains a Decision
  // resolution intent and is intentionally not a workspace execution command.
  intent?: 'resume';
  requestId?: string;
  reason?: string;
  source?: string;
  definitionRevision?: string;
  decisionId?: string;
};

export type Soul = {
  id: string;
  name: string;
  description?: string;
  businessDomains?: string[];
  soulMd?: string;
  allowedSkillRefs?: string[];
  allowedMCPServerRefs?: string[];
  allowedCapabilityRefs?: string[];
  allowedCapabilityCategories?: string[];
  allowedToolKinds?: string[];
};

export type WorkspaceSoulBindings = {
  workspaceId?: string;
  defaultSoulId?: string;
  bindings?: Record<string, string>;
  priorityPolicy?: string[];
  availableStages?: string[];
};

export type WorkspaceTemplate = {
  id: string;
  name: string;
  description?: string;
  goalText?: string;
  doDItems?: string[];
  defaultSoulId?: string;
  soulBindings?: Record<string, string>;
};

export type WorkspaceSetupDraftWorkspace = {
  title?: string;
  description?: string;
  deliverableDirectory?: string;
};

export type WorkspaceSetupDraftMessage = {
  role: string;
  content: string;
};

export type WorkspaceSetupDraft = {
  goalText?: string;
  doDItems?: string[];
  assumptions?: string[];
  clarifyingQuestions?: string[];
  warnings?: string[];
  confidence?: string;
  needsClarification?: boolean;
  generationSummary?: string;
  templateContextUsed?: boolean;
  previousDraftApplied?: boolean;
};

export type WorkspaceSetupDraftSession = {
  id: string;
  templateId?: string;
  workspace?: WorkspaceSetupDraftWorkspace;
  messages?: WorkspaceSetupDraftMessage[];
  draft?: WorkspaceSetupDraft;
  state: string;
  dialogLanguage?: string;
  createdAt: string;
  updatedAt: string;
};

export type Skill = {
  id: string;
  name: string;
  version: string;
  description?: string;
  instruction?: string;
  source?: string;
  scope?: string;
  workspaceId?: string;
  status?: string;
  sourceRepository?: string;
  sourcePath?: string;
  targetPath?: string;
  localPath?: string;
  repoPath?: string;
  globalPath?: string;
  projectPath?: string;
  contentHash?: string;
  sourceHash?: string;
  appliedHash?: string;
  actualHash?: string;
  agent?: string;
  managedBy?: string;
  sourceSkillId?: string;
  requiredMCPServerRefs?: string[];
  requiredCapabilityRefs?: string[];
  requiredToolKinds?: string[];
};

export type MCPServer = {
  id: string;
  name: string;
  transport: string;
  endpoint?: string;
  command?: string;
  args?: string[];
  env?: Record<string, string>;
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

export type MCPCapability = {
  id: string;
  serverId: string;
  kind: 'tool' | 'resource' | 'prompt' | string;
  name: string;
  displayName: string;
  description?: string;
  category?: string;
  tags?: string[];
  usageHint?: string;
  inputSummary?: string;
  outputSummary?: string;
  stageHints?: string[];
  riskLevel?: string;
  schema?: Record<string, unknown>;
  uri?: string;
  enabled: boolean;
  approvalPolicy?: string;
  status?: string;
};

export type MemoryItem = {
  id: string;
  workspaceId: string;
  runId?: string;
  sourceType: string;
  sourceRef?: string;
  memoryType: string;
  content?: string;
  summary: string;
  tags?: string[];
  importance?: number;
  confidence?: number;
  visibility?: string;
  embeddingProviderId?: string;
  embeddingModel?: string;
  embeddingText?: string;
  vectorReady?: boolean;
  vectorDimension?: number;
  vectorScore?: number;
};

export type MemoryRecallHit = {
  id: string;
  summary?: string;
  vectorScore?: number;
  vectorDimension?: number;
};

export type MemoryRecallTrace = {
  id: string;
  workspaceId: string;
  runId: string;
  stepId?: string;
  graphNodeId?: string;
  stage?: string;
  source: string;
  policy?: string;
  status: string;
  providerId?: string;
  operation?: string;
  queryTextPreview?: string;
  queryTextSha256?: string;
  minScore?: number;
  limit?: number;
  embeddingModel?: string;
  vectorDimension?: number;
  retrievedMemoryIds?: string[];
  retrievedMemoryHits?: MemoryRecallHit[];
  reason?: string;
  occurredAt?: string;
  evidenceRef?: string;
};

export type StepView = {
  id: string;
  runId: string;
  kind?: string;
  state: string;
  input?: Record<string, unknown>;
  output?: Record<string, unknown>;
};

export type StepPage = {
  runId: string;
  steps?: StepView[];
  limit: number;
  cursorAfterId?: string;
  nextCursorAfterId?: string;
  hasMore: boolean;
};

export type ToolCall = {
  id: string;
  workspaceId: string;
  runId: string;
  stepId?: string;
  toolName: string;
  arguments?: Record<string, unknown>;
  state: string;
  exitSummary?: string;
  output?: Record<string, unknown>;
  error?: string;
};

export type ToolCallPage = {
  runId: string;
  toolCalls?: ToolCall[];
  limit: number;
  cursorAfterId?: string;
  nextCursorAfterId?: string;
  hasMore: boolean;
};

export type Patch = {
  id: string;
  workspaceId: string;
  runId: string;
  stepId?: string;
  toolCallId?: string;
  approvalId?: string;
  revertApprovalId?: string;
  patchType: string;
  targetRef?: string;
  summary?: string;
  state: string;
  addedLines?: number;
  deletedLines?: number;
  beforeHash?: string;
  afterHash?: string;
  createdAt?: string;
  appliedAt?: string;
  revertedAt?: string;
};

export type PatchPage = {
  runId: string;
  patches?: Patch[];
  limit: number;
  cursorAfterId?: string;
  nextCursorAfterId?: string;
  hasMore: boolean;
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

export type ValidationResultPage = {
  runId: string;
  validationResults?: ValidationResult[];
  limit: number;
  cursorAfterId?: string;
  nextCursorAfterId?: string;
  hasMore: boolean;
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

export type RuntimeCheckpointPage = {
  runId: string;
  runtimeCheckpoints?: RuntimeCheckpoint[];
  limit: number;
  cursorAfterId?: string;
  nextCursorAfterId?: string;
  hasMore: boolean;
};

export type Result = {
  id: string;
  workspaceId: string;
  runId: string;
  goalId?: string;
  doDId?: string;
  state: string;
  title?: string;
  summary?: string;
  sourceType?: string;
  sourceRef?: string;
  evidenceRefs?: string[];
  artifactRefs?: string[];
  doDAssessment?: DoDAssessment;
  summaryDetail?: ResultSummary;
  createdAt?: string;
  updatedAt?: string;
};

export type ResultPage = {
  runId: string;
  results?: Result[];
  limit: number;
  cursorAfterId?: string;
  nextCursorAfterId?: string;
  hasMore: boolean;
};

export type ResultSummary = {
  goalOutcome?: string;
  doDItems?: DoDAssessmentItem[];
  implementationSummary?: string;
  verificationSummary?: string;
  evidenceRefs?: string[];
  artifactRefs?: string[];
  knownLimitations?: string[];
  residualRisks?: string[];
  nextActions?: string[];
};

export type DoDAssessment = {
  satisfied: boolean;
  items?: DoDAssessmentItem[];
  summary?: string;
};

export type DoDAssessmentItem = {
  item: string;
  state: string;
  evidenceRefs?: string[];
  failureReason?: string;
  confidence?: number;
  sourceNote?: string;
};

export type Artifact = {
  id: string;
  workspaceId: string;
  runId?: string;
  resultId?: string;
  stepId?: string;
  toolCallId?: string;
  patchId?: string;
  validationResultId?: string;
  artifactType: string;
  category?: string;
  name?: string;
  summary?: string;
  uri?: string;
  contentType?: string;
  state: string;
  evidenceRefs?: string[];
  metadata?: Record<string, unknown>;
  createdAt?: string;
  updatedAt?: string;
};

export type ArtifactContinuation = {
  id: string;
  workspaceId: string;
  runId: string;
  rootStepId?: string;
  artifactId: string;
  targetRef: string;
  operation?: string;
  state: string;
  expectedSequence?: number;
  receivedSequences?: number[];
  finalReceived?: boolean;
  bytesWritten?: number;
  rollingHash?: string;
  lastChunkHash?: string;
  coverageTargets?: string[];
  evidenceRefs?: string[];
  failureReason?: string;
  createdAt?: string;
  updatedAt?: string;
  progress?: Record<string, unknown>;
};

export type ArtifactPage = {
  runId: string;
  artifacts?: Artifact[];
  limit: number;
  cursorAfterId?: string;
  nextCursorAfterId?: string;
  hasMore: boolean;
};

export type WebSourceTrace = {
  query?: string;
  providerId?: string;
  searchedAt?: string;
  sources?: WebSource[];
  fetchedSources?: FetchedWebSource[];
  evidenceRefs?: string[];
  approvalId?: string;
  policyDecision?: string;
};

export type WebSource = {
  title?: string;
  url: string;
  snippet?: string;
  source?: string;
  publishedAt?: string;
  rank?: number;
  evidenceRef?: string;
};

export type FetchedWebSource = {
  url: string;
  finalUrl?: string;
  title?: string;
  source?: string;
  retrievedAt?: string;
  contentHash?: string;
  truncated?: boolean;
  evidenceRef?: string;
};

export type LLMContextBudgetTrace = {
  stepId: string;
  providerId?: string;
  model?: string;
  capabilityRevision?: string;
  configurationRevision?: string;
  maxBytes: number;
  actualBytes: number;
  maxTokens?: number;
  estimatedInputTokens?: number;
  estimatedOutputTokens?: number;
  actualTokens?: number;
  shrinkLevel?: number;
  truncated: boolean;
  rules?: string[];
  sourceRefs?: string[];
  includedSections?: string[];
  normalizedSections?: string[];
  compressedSections?: string[];
  omittedSections?: string[];
  goalDoDCompressed: boolean;
  overBudget: boolean;
  p0Overflow: boolean;
  profileMode?: string;
  capabilitySource?: string;
  contextUtilizationRatio?: number;
  maxRequestBytes?: number;
  stageBudgetRatio?: number;
  stageBudgetSource?: string;
  tracePolicy?: string;
  promptCaptured: boolean;
  estimator?: string;
  profile?: Record<string, unknown>;
  cachePlan?: Record<string, unknown>;
};

export type LLMContextBudgetSummary = {
  traceTotal: number;
  truncatedCount?: number;
  overBudgetCount?: number;
  p0OverflowCount?: number;
  goalDoDCompressedCount?: number;
  promptCapturedCount?: number;
  maxShrinkLevel?: number;
  maxContextUtilizationRatio?: number;
  avgContextUtilizationRatio?: number;
  maxInputTokens?: number;
  maxRequestBytes?: number;
  recentWindowSize?: number;
  recentTraceTotal?: number;
  recentTruncatedCount?: number;
  recentOverBudgetCount?: number;
  recentMaxContextUtilizationRatio?: number;
  recentAvgContextUtilizationRatio?: number;
  recentMaxInputTokens?: number;
  contextUtilizationTrend?: string;
};

export type LLMUsageTrace = {
  id: string;
  workspaceId: string;
  runId: string;
  stepId?: string;
  providerId?: string;
  provider?: string;
  model?: string;
  operation?: string;
  inputTokens?: number;
  outputTokens?: number;
  totalTokens?: number;
  estimated: boolean;
  usageSource?: string;
  requestBytes?: number;
  responseBytes?: number;
  streaming: boolean;
  contextBudget?: Record<string, unknown>;
  cacheUsage?: Record<string, unknown>;
  startedAt?: string;
  completedAt?: string;
  durationMs?: number;
  status?: string;
  failureClass?: string;
  occurredAt?: string;
  evidenceRef?: string;
};

export type LLMUsageSummary = {
  entryTotal: number;
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  estimatedEntries: number;
  providerNativeEntries: number;
  failedEntries: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  cacheCreationTokens?: number;
  cacheableInputTokens?: number;
  nonCacheableInputTokens?: number;
  cacheReadInputRatio?: number;
  cacheableInputRatio?: number;
  nonCacheableInputRatio?: number;
  cacheEstimatedEntries?: number;
  cacheProviderEntries?: number;
  providerModelBreakdown?: LLMUsageProviderModelBreakdown[];
};

export type LLMUsageProviderModelBreakdown = {
  providerId?: string;
  provider?: string;
  model?: string;
  entryTotal: number;
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  cacheReadInputRatio?: number;
  estimatedEntries: number;
  failedEntries: number;
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
  resultTotal: number;
  artifactTotal: number;
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

export type RuntimeCapability = {
  id: string;
  serverId?: string;
  kind?: string;
  name?: string;
  displayName?: string;
  description?: string;
  category?: string;
  tags?: string[];
  riskLevel?: string;
  enabled: boolean;
  approvalPolicy?: string;
  status?: string;
  useCount?: number;
  evidenceRefs?: string[];
};

export type AcceptanceSlice = {
  id?: string;
  title?: string;
  state?: string;
  priority?: number;
  scope?: string;
  coverageWorkItemIds?: string[];
  requiredEvidenceKinds?: string[];
  completionPolicy?: string;
  resultPolicy?: string;
  createdAt?: string;
  activatedAt?: string;
  closedAt?: string;
};

export type CoverageFocus = {
  id?: string;
  sliceId?: string;
  coverageWorkItemIds?: string[];
  failureClass?: string;
  failureSignature?: string;
  requiredEvidenceKinds?: string[];
  acceptedEvidenceRefs?: string[];
  rejectedEvidenceRefs?: string[];
  targetRefs?: string[];
  state?: string;
  attempt?: number;
};

export type ConvergenceBudget = {
  state?: string;
  decision?: string;
  maxRepairAttemptsPerFocus?: number;
  maxLLMRoundsPerSlice?: number;
  maxRepeatedFailureSignature?: number;
  maxValidationOnlyRetries?: number;
  maxNoCoverageDeltaRounds?: number;
  maxContextBytesPerRepair?: number;
  repairAttempts?: number;
  llmRounds?: number;
  repeatedFailureSignatures?: number;
  validationOnlyRetries?: number;
  noCoverageDeltaRounds?: number;
  lastDecisionReason?: string;
};

export type AcceptanceSummary = {
  acceptedSlices: number;
  activeSlices: number;
  openSlices: number;
  failedSlices: number;
};

export type AcceptanceProjection = {
  activeSliceId?: string;
  slices?: AcceptanceSlice[];
  currentFocus?: CoverageFocus;
  budget?: ConvergenceBudget;
  summary: AcceptanceSummary;
};

export type GapTodoItem = {
  id?: string;
  source?: string;
  sourceRef?: string;
  coverageWorkItemIds?: string[];
  requiredEvidenceKinds?: string[];
  status?: string;
  priority?: number;
  reason?: string;
  metadata?: Record<string, unknown>;
};

export type GapNextAction = {
  kind?: string;
  reason?: string;
};

export type GapBaseline = {
  workspaceId?: string;
  runId?: string;
  definitionRevision?: string;
  baselineRevision?: string;
  currentFocus?: Record<string, unknown>;
  openCoverageWorkItems?: Record<string, unknown>[];
  acceptedEvidenceRefs?: string[];
  rejectedEvidenceRefs?: string[];
  todoItems?: GapTodoItem[];
  nextAction?: GapNextAction;
};

export type TaskProgress = {
  workspaceId: string;
  runId?: string;
  state: string;
  summary?: string;
  currentMilestone?: string;
  milestones?: ProgressMilestone[];
  items?: ProgressItem[];
  blockers?: ProgressBlocker[];
  recovery: RecoveryPlan;
  repairLoop: RepairLoop;
  evidenceRefs?: string[];
  runtimeV2?: RuntimeV2Progress;
};

export type RuntimeV2Progress = {
  stateRevision?: number;
  evidenceSequence?: number;
  completedDispatchCount?: number;
  activeDispatchId?: string;
  candidateAttemptCount?: number;
  acceptedEvidenceCount?: number;
  rejectedEvidenceCount?: number;
  missingEvidenceKinds?: string[];
  currentFocusId?: string;
  currentAction?: string;
  failureClass?: string;
  failureCode?: string;
  failureSignature?: string;
  failureMessage?: string;
  repairHintCodes?: string[];
  prerequisiteRefreshState?: string;
  prerequisiteRefreshUsed?: number;
  prerequisiteRefreshLimit?: number;
  prerequisiteInvalidations?: RuntimePrerequisiteRefreshInvalidation[];
};

export type RuntimePrerequisiteRefreshInvalidation = {
  signature?: string;
  source?: string;
  prerequisiteIds?: string[];
  observationIds?: string[];
  dispatchId?: string;
  operationId?: string;
  receiptHash?: string;
  ownerModuleId?: string;
  resourceDomain?: string;
  subjectRef?: string;
  expectedRevision?: string;
  observedRevision?: string;
  reason?: string;
  resourceChanges?: RuntimePrerequisiteResourceChange[];
};

export type RuntimePrerequisiteResourceChange = {
  ownerModuleId?: string;
  resourceDomain?: string;
  subjectRef?: string;
  beforeRevision?: string;
  afterRevision?: string;
};

export type TaskDecomposition = {
  id: string;
  workspaceId: string;
  runId: string;
  rootStepId: string;
  state: string;
  summary?: string;
  graphBranch?: string;
  nodeTotal: number;
  nodeCompleted: number;
  nodeFailed: number;
  nodeRunning: number;
  nodes?: TaskDecompositionNode[];
  evidenceRefs?: string[];
};

export type TaskDecompositionNode = {
  id: string;
  kind: string;
  title?: string;
  state: string;
  dependsOn?: string[];
  branch?: string;
  stepId?: string;
  stepState?: string;
  retryOf?: string;
  retryAttempt?: number;
  violationType?: string;
  violatedRule?: string;
  failureClass?: string;
  failureSignature?: string;
  targetScope?: string;
  blockingTicketId?: string;
  evidenceRefs?: string[];
  coverageTargets?: string[];
  input?: Record<string, unknown>;
};

export type TaskPlan = {
  id: string;
  workspaceId: string;
  runId: string;
  anchorStepId: string;
  revisionStepId?: string;
  revision: number;
  state: string;
  stage?: string;
  sourceOutputState?: string;
  summary?: string;
  graphBranch?: string;
  nodeTotal: number;
  nodeCompleted: number;
  nodeFailed: number;
  nodeRunning: number;
  nodes?: TaskDecompositionNode[];
  retryOf?: string;
  retryAttempt?: number;
  violationType?: string;
  violatedRule?: string;
  failureClass?: string;
  failureSignature?: string;
  targetScope?: string;
  blockingTicketId?: string;
  evidenceRefs?: string[];
  occurredAt?: string;
  sourceProjection?: string;
};

export type ExecutionSection = {
  id: string;
  workspaceId: string;
  runId: string;
  title: string;
  state: string;
  graphRootStepId?: string;
  graphNodeId?: string;
  stepId?: string;
  stepKind?: string;
  relatedDoDItems?: string[];
  plannedStepRefs?: string[];
  actualEvidenceRefs?: string[];
  decisionRefs?: string[];
  approvalRefs?: string[];
  toolCallRefs?: string[];
  patchRefs?: string[];
  validationRefs?: string[];
  resultRefs?: string[];
  artifactRefs?: string[];
  memoryRecallRefs?: string[];
  memoryIds?: string[];
  summary?: string;
};

export type ProgressMilestone = {
  id: string;
  title: string;
  state: string;
  evidenceRefs?: string[];
};

export type ProgressItem = {
  id: string;
  kind: string;
  title: string;
  state: string;
  evidenceRefs?: string[];
};

export type ProgressBlocker = {
  id: string;
  kind: string;
  state: string;
  reason?: string;
  evidenceRef?: string;
};

export type RecoveryPlan = {
  mode: string;
  nextAction?: string;
  runId?: string;
  stepId?: string;
  blockingTicketId?: string;
  blockingApprovalId?: string;
  checkpointId?: string;
  failureClass?: string;
  failureSignature?: string;
  reason?: string;
};

export type RepairLoop = {
  state: string;
  attemptCount: number;
  lastAttempt?: number;
  lastRepairStepId?: string;
  lastFailureStepId?: string;
  lastFailureClass?: string;
  lastFailureSignature?: string;
  lastFailureTargetScope?: string;
  lastFailureAt?: string;
  structuredFailureSignals?: Record<string, unknown>;
  repairInputCompaction?: Record<string, unknown>;
  lastRepairInputBytes?: number;
  repeatedFailureReason?: string;
  nextAction?: string;
  escalated: boolean;
  escalationReason?: string;
  escalationTicketId?: string;
  evidenceRefs?: string[];
};

export type ContextBundle = {
  workspaceId: string;
  runId?: string;
  mode: string;
  summary?: string;
  compressedContext: CompressedContext;
  taskProgress: TaskProgress;
  recovery: RecoveryPlan;
  latestCheckpoint?: RuntimeCheckpoint;
  sections?: ContextSection[];
  recentEvidence?: ContextEvidence[];
  compressionCandidates?: ContextCompressionCandidate[];
};

export type CompressedContext = {
  version: string;
  mode: string;
  workspaceId: string;
  runId?: string;
  summary?: string;
  currentMilestone?: string;
  milestones?: ProgressMilestone[];
  blockers?: ProgressBlocker[];
  recovery: RecoveryPlan;
  recentEvidence?: ContextEvidence[];
  resultRefs?: string[];
  sourceSectionRefs?: string[];
  sourceEvidenceRefs?: string[];
  retentionPolicy?: string;
};

export type ContextSection = {
  id: string;
  kind: string;
  title: string;
  priority: number;
  state?: string;
  summary?: string;
  evidenceRefs?: string[];
  data?: Record<string, unknown>;
};

export type ContextEvidence = {
  id: string;
  kind: string;
  state?: string;
  summary?: string;
  occurredAt?: string;
  evidenceRefs?: string[];
  data?: Record<string, unknown>;
};

export type ContextCompressionCandidate = {
  sectionId: string;
  reason: string;
  retention: string;
  sourceRefs?: string[];
  suggestedMode?: string;
};

export type EventRecord = {
  id: string;
  version: string;
  topic: string;
  occurredAt?: string;
  sourceModule?: string;
  subjectType?: string;
  subjectId?: string;
  workspaceId?: string;
  runId?: string;
  stepId?: string;
  payload?: EventPayload;
  payloadSummary?: EventPayload;
  payloadRef?: string;
  revision?: number;
  stateSummary?: WorkorchEventStateSummary;
};

export type EventPayloadValueKind = 'null' | 'string' | 'bool' | 'int' | 'uint' | 'float' | 'object' | 'array';

export type EventPayloadField = {
  name: string;
  value: EventPayloadValue;
};

export type EventPayloadValue = {
  kind: EventPayloadValueKind;
  string?: string;
  bool?: boolean;
  int?: number;
  uint?: number;
  float?: number;
  object?: EventPayloadField[];
  array?: EventPayloadValue[];
};

export type EventPayload = {
  fields?: EventPayloadField[];
};

export type WorkorchEventStateSummary = {
  workspaceState?: string;
  activeRunId?: string;
  runState?: string;
  progressSummary?: string;
  blockingTicketId?: string;
  blockingApprovalId?: string;
  resultState?: string;
  outcomeSummary?: string;
  revision?: number;
};

export type RunObservability = {
  run: Run;
  outcome?: RunOutcome;
  progress: RunProgress;
  taskProgress: TaskProgress;
  repairLoop: RepairLoop;
  acceptance: AcceptanceProjection;
  taskDecompositions?: TaskDecomposition[];
  taskPlans?: TaskPlan[];
  executionSections?: ExecutionSection[];
  memoryRecallTraces?: MemoryRecallTrace[];
  webSources?: WebSourceTrace[];
  llmContextBudgets?: LLMContextBudgetTrace[];
  llmContextBudgetSummary: LLMContextBudgetSummary;
  llmUsageTraces?: LLMUsageTrace[];
  llmUsageSummary: LLMUsageSummary;
  evidence: RuntimeEvidenceSummary;
  latestCheckpoint?: RuntimeCheckpoint;
  selectedCapabilities?: RuntimeCapability[];
  usedCapabilities?: RuntimeCapability[];
  timeline?: RuntimeTimelineItem[];
};

export type RunSnapshot = {
  workspace: Workspace;
  run: Run;
  outcome?: RunOutcome;
  taskProgress: TaskProgress;
  runHeartbeat?: RunHeartbeat;
  repairLoop: RepairLoop;
  acceptance: AcceptanceProjection;
  gapBaseline?: GapBaseline;
  runtimeExecution?: RuntimeExecution;
  convergenceStatus?: ConvergenceStatus;
  taskDecompositions?: TaskDecomposition[];
  taskPlans?: TaskPlan[];
  executionSections?: ExecutionSection[];
  memoryRecallTraces?: MemoryRecallTrace[];
  webSources?: WebSourceTrace[];
  llmContextBudgets?: LLMContextBudgetTrace[];
  llmContextBudgetSummary: LLMContextBudgetSummary;
  llmUsageTraces?: LLMUsageTrace[];
  llmUsageSummary: LLMUsageSummary;
  steps?: StepView[];
  toolCalls?: ToolCall[];
  patches?: Patch[];
  validationResults?: ValidationResult[];
  runtimeCheckpoints?: RuntimeCheckpoint[];
  results?: Result[];
  artifacts?: Artifact[];
  artifactContinuations?: ArtifactContinuation[];
  selectedCapabilities?: RuntimeCapability[];
  usedCapabilities?: RuntimeCapability[];
  eventRecords?: EventRecord[];
  decisions?: Decision[];
  approvals?: Approval[];
};

export type WorkspaceSnapshot = {
  workspace: Workspace;
  outcome?: WorkspaceOutcome;
  taskProgress: TaskProgress;
  runHeartbeat?: RunHeartbeat;
  runtimeExecution?: RuntimeExecution;
  convergenceStatus?: ConvergenceStatus;
  runCommand?: RunCommandStatus;
  contextBundle?: ContextBundle;
  coverageSummary?: Record<string, unknown>;
  gapBaseline?: GapBaseline;
  acceptance: AcceptanceProjection;
  taskDecompositions?: TaskDecomposition[];
  taskPlans?: TaskPlan[];
  executionSections?: ExecutionSection[];
  goals?: Goal[];
  dods?: DoD[];
  runs?: Run[];
  results?: Result[];
  artifacts?: Artifact[];
  artifactContinuations?: ArtifactContinuation[];
  patches?: Patch[];
  webSources?: WebSourceTrace[];
  llmContextBudgets?: LLMContextBudgetTrace[];
  llmContextBudgetSummary: LLMContextBudgetSummary;
  llmUsageTraces?: LLMUsageTrace[];
  llmUsageSummary: LLMUsageSummary;
  eventRecords?: EventRecord[];
  decisions?: Decision[];
  approvals?: Approval[];
};

export type WorkspaceSnapshotProjection = {
  projection: ProjectionStatus;
  snapshot: WorkspaceSnapshot;
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
  allowedRecoveryActions?: string[];
  state: string;
  resolutionId?: string;
  resolutionType?: string;
  resolutionPayload?: Record<string, unknown>;
  continuationAction?: string;
  resumeRunId?: string;
  successorRunId?: string;
  autoGovernance?: AutoGovernanceSummary;
};

export type DecisionPage = {
  workspaceId?: string;
  runId?: string;
  decisions?: Decision[];
  limit: number;
  cursorAfterId?: string;
  nextCursorAfterId?: string;
  hasMore: boolean;
};

export type Approval = {
  id: string;
  workspaceId: string;
  runId: string;
  stepId?: string;
  reason: string;
  state: string;
  autoGovernance?: AutoGovernanceSummary;
};

export type ApprovalPage = {
  workspaceId?: string;
  runId?: string;
  approvals?: Approval[];
  limit: number;
  cursorAfterId?: string;
  nextCursorAfterId?: string;
  hasMore: boolean;
};

export type AutoGovernanceSummary = {
  traceId: string;
  action: string;
  status: string;
  riskLevel?: string;
  policyMatched?: string;
  policyBlocked?: string;
  confidence?: number;
  reason?: string;
  evidenceRefs?: string[];
  promptFingerprint?: string;
  occurredAt?: string;
};

export type ControlPlaneProps = {
  selectedWorkspaceId?: string;
  onOpenWorkspace: (workspaceId: string) => void;
};

export type WorkspaceBoardItem = {
  snapshot: WorkspaceSnapshot;
};
