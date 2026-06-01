export type ServiceStatus = {
  service: string;
  status: string;
};

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
  llmOutput?: RuntimeLLMOutputConfiguration;
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

export type LLMConfiguration = {
  defaultProviderId?: string;
  defaultEmbeddingProviderId?: string;
  modelCapabilities?: LLMModelCapabilityConfiguration[];
  providers?: LLMProviderConfiguration[];
  providerTemplates?: LLMProviderTemplate[];
};

export type LLMModelCapabilityConfiguration = {
  id: string;
  provider?: string;
  modelPattern: string;
  contextWindowTokens: number;
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
  failoverEnabled: boolean;
  failoverProviders?: string[];
  inferenceProfile?: ProviderInferenceProfile;
  contextProfile?: ProviderContextProfile;
};

export type ProviderInferenceProfile = {
  temperature?: number;
  maxOutputTokens?: number;
  reasoningEffort?: string;
  thinkingBudgetTokens?: number;
};

export type ProviderContextProfile = {
  profileMode?: string;
  capabilitySource?: string;
  contextWindowTokens?: number;
  maxInputTokens?: number;
  reservedOutputTokens?: number;
  safetyRatio?: number;
  contextUtilizationRatio?: number;
  bytesHardLimit?: number;
  maxRequestBytes?: number;
  estimatedTokensPerByte?: number;
};

export type LLMProviderTemplate = {
  templateId: string;
  name: string;
  description?: string;
  id: string;
  provider: string;
  baseUrl?: string;
  model?: string;
  timeout?: string;
  disabled: boolean;
  chatCompletions: boolean;
  streaming: boolean;
  nonChatOperations?: string[];
};

export type UpdateLLMConfigurationRequest = {
  defaultProviderId?: string;
  defaultEmbeddingProviderId?: string;
  modelCapabilities?: LLMModelCapabilityConfiguration[];
  providers: UpdateLLMProviderConfiguration[];
};

export type UpdateServerConfigurationRequest = {
  defaultDialogLanguage?: string;
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
  failoverEnabled?: boolean;
  failoverProviders?: string[];
  inferenceProfile?: UpdateLLMProviderInferenceProfile;
  contextProfile?: UpdateLLMProviderContextProfile;
};

export type UpdateLLMProviderInferenceProfile = {
  temperature?: number;
  maxOutputTokens?: number;
  reasoningEffort?: string;
  thinkingBudgetTokens?: number;
};

export type UpdateLLMProviderContextProfile = {
  profileMode?: string;
  contextWindowTokens?: number;
  maxInputTokens?: number;
  reservedOutputTokens?: number;
  safetyRatio?: number;
  contextUtilizationRatio?: number;
  bytesHardLimit?: number;
  maxRequestBytes?: number;
  estimatedTokensPerByte?: number;
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
  defaultSoulId?: string;
  soulBindings?: Record<string, string>;
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
  goalId?: string;
  doDId?: string;
  llmProviderId?: string;
  soulId?: string;
  soulBindings?: Record<string, string>;
  skillIds?: string[];
  mcpServerIds?: string[];
  capabilityIds?: string[];
  memoryIds?: string[];
  dialogLanguage?: string;
  priority?: number;
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
  summary: string;
  tags?: string[];
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
  p0Overflow: boolean;
  tracePolicy?: string;
  promptCaptured: boolean;
  estimator?: string;
  profile?: Record<string, unknown>;
  cachePlan?: Record<string, unknown>;
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
  evidenceRefs?: string[];
  input?: Record<string, unknown>;
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
  payload?: Record<string, unknown>;
  revision?: number;
  stateSummary?: WorkorchEventStateSummary;
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
  taskDecompositions?: TaskDecomposition[];
  executionSections?: ExecutionSection[];
  memoryRecallTraces?: MemoryRecallTrace[];
  webSources?: WebSourceTrace[];
  llmContextBudgets?: LLMContextBudgetTrace[];
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
  repairLoop: RepairLoop;
  taskDecompositions?: TaskDecomposition[];
  executionSections?: ExecutionSection[];
  memoryRecallTraces?: MemoryRecallTrace[];
  webSources?: WebSourceTrace[];
  llmContextBudgets?: LLMContextBudgetTrace[];
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
  coverageSummary?: Record<string, unknown>;
  taskDecompositions?: TaskDecomposition[];
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
  llmUsageTraces?: LLMUsageTrace[];
  llmUsageSummary: LLMUsageSummary;
  eventRecords?: EventRecord[];
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
  goals: Goal[];
  dods: DoD[];
};
