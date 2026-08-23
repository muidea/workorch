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
  maxDocumentRecoveryAttempts?: number;
};

export type ProviderModelTarget = {
  providerId: string;
  modelId: string;
};

export type ProviderInferenceSelection = {
  target: ProviderModelTarget;
  reasoningEffort?: string;
};

export type LLMContextBudgetPolicyView = {
  contextUtilizationRatio?: number;
  estimatedTokensPerByte?: number;
};

export type ProviderModelDescriptorView = {
  providerId: string;
  modelId: string;
  operations?: string[];
  capabilityReady: boolean;
  reasoning?: {
    supported: boolean;
    valid: boolean;
    defaultEffort?: string;
    efforts?: string[];
    invalidReason?: string;
  };
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
  defaultChat: ProviderModelTarget;
  defaultEmbedding?: ProviderModelTarget;
  budget: LLMContextBudgetPolicyView;
  catalog?: ProviderCatalogSummary;
  restartRequired: boolean;
};

export type UpdateLLMConfigurationRequest = {
  apiKey?: string;
  defaultChatModelId?: string;
  defaultEmbeddingModelId?: string;
  clearDefaultEmbedding?: boolean;
  budget?: {
    contextUtilizationRatio?: number;
    estimatedTokensPerByte?: number;
  };
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
	revision: number;
  id: string;
  title: string;
  description?: string;
  deliverableDirectory: string;
  state: string;
  activeRunId?: string;
  // Mutable workspace preference. It applies only to the next activation.
  defaultLLMSelection?: ProviderInferenceSelection;
  defaultSoulId?: string;
  soulBindings?: Record<string, string>;
	contextHints?: Record<string, unknown>;
	evidencePolicies?: Record<string, unknown>[];
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
  blockingTicketId?: string;
  blockingApprovalId?: string;
  failureReason?: string;
  // Immutable Runtime activation input; never send it back on Resume.
  sealedLLMSelection?: ProviderInferenceSelection;
  // Read-only Run-owner disposition. It is not an Orchestrator action and
  // must never be returned as an activation input.
  runtimeActivation?: RuntimeActivationStatus;
};

export type RuntimeActivationStatus = {
  disposition: string;
  reason: string;
  actualSchemaVersion: number;
  supportedSchemaVersion: number;
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

export type ConvergenceStatus = {
  workspaceId?: string;
  runId?: string;
  runState?: string;
  currentSliceId?: string;
  currentFocusId?: string;
  convergenceState?: string;
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

type RunCommandStatusBase = {
  workspaceId: string;
  runId?: string;
  intent: WorkspaceExecutionIntent;
  requestId: string;
  requestReason?: string;
  source?: string;
  outcome: RuntimeExecutionIntentOutcome;
  admissionReason: RuntimeExecutionIntentReason;
  diagnostic: string;
  revision: number;
};

export type RunCommandStatus = RunCommandStatusBase & (
  | { state: 'accepted'; acceptedAt: string; rejectedAt?: never }
  | { state: 'rejected'; acceptedAt?: never; rejectedAt: string }
);

export type RunDisplayOutcome = {
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

export type WorkspaceDisplayOutcome = {
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

export type WorkspaceExecutionIntent = 'resume' | 'restart';

export type RuntimeExecutionIntentOutcome =
  | 'reconciliation_requested'
  | 'replacement_pending'
  | 'restart_required'
  | 'fresh_activation_required'
  | 'definition_changed_requires_restart'
  | 'governance_required'
  | 'data_reset_required';

export type RuntimeExecutionIntentReason =
  | 'workspace_active_run_absent'
  | 'selected_run_absent'
  | 'selected_run_terminal'
  | 'runtime_state_incompatible'
  | 'definition_changed'
  | 'runtime_bootstrap_missing'
  | 'current_run_continuation_accepted'
  | 'pending_run_activated'
  | 'governance_pending'
  | 'run_not_resumable'
  | 'duplicate_restart'
  | 'new_run_activated'
  | 'predecessor_supersede_requested'
  | 'development_data_incompatible'
  | 'definition_admission_missing'
  | 'definition_admission_mismatch'
  | 'definition_clarification_pending';

export type WorkspaceExecutionAdmission = {
  service: string;
  backend?: string;
  dataRoot?: string;
  action?: string;
  accepted: boolean;
  state: 'accepted' | 'rejected';
  workspaceId: string;
  runId?: string;
  requestId: string;
  intent: WorkspaceExecutionIntent;
  outcome: RuntimeExecutionIntentOutcome;
  reason: RuntimeExecutionIntentReason;
  requestReason?: string;
  source?: string;
  diagnostic?: string;
  error?: string;
};

export type WorkspaceExecutionIntentRequest = {
  intent?: WorkspaceExecutionIntent;
  requestId?: string;
  reason?: string;
  source?: string;
};

// Form-only configuration for an explicit fresh activation. Model and Soul
// values are persisted through their Workspace-owner APIs before the immutable
// restart intent is submitted. Resume never consumes these fields.
export type WorkspaceExecutionActivationRequest = {
  intent: WorkspaceExecutionIntent;
  llmTarget?: ProviderModelTarget;
  reasoningEffort?: string;
  soulId?: string;
  soulBindings?: Record<string, string>;
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
  contextHints?: Record<string, unknown>;
  evidencePolicies?: Record<string, unknown>[];
};

export type CreateWorkspaceRequest = {
  title: string;
  description?: string;
  deliverableDirectory?: string;
  templateId?: string;
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
  environmentRefs?: Record<string, string>;
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
  effectScope?: string;
  idempotencyClass?: string;
  validationProofApplicability?: string;
  schema?: Record<string, unknown>;
  uri?: string;
  enabled: boolean;
  approvalPolicy?: string;
  source: string;
  sourceRef: string;
  revision: number;
};

export type MCPCapabilityMutation = Omit<MCPCapability, 'source' | 'sourceRef' | 'revision'>;

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
  logicalRequestId?: string;
  physicalCallId?: string;
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
  physicalResponseMode: 'streaming' | 'non_streaming';
  providerSubmitted: boolean;
  /** @deprecated compatibility-only derivation of physicalResponseMode */
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
  recoveryBudget?: RuntimeRecoveryBudget;
  // Persisted SM-2 authority. This is the only displayed source for the
  // current Runtime action; assessment audit observations are non-authoritative.
  repairDirective?: RuntimeRepairDirective;
  semanticPlanning?: RuntimeSemanticPlanning;
  audit?: RuntimeAudit;
  summary: AcceptanceSummary;
};

export type RuntimeRecoveryBudget = {
  maxProviderTransportAttempts?: number;
  providerTransportAttempts?: number;
  maxOperationRecoveryAttempts?: number;
  operationRecoveryAttempts?: number;
  maxCASReloadAttempts?: number;
  casReloadAttempts?: number;
  maxTerminalDeliveryAttempts?: number;
  terminalDeliveryAttempts?: number;
};

export type RuntimeSemanticPlanning = {
  workspaceId: string;
  runId: string;
  definitionRevision: string;
  stateRevision: number;
  state: 'semantic_contract_required' | 'planning' | 'feasibility' | 'sealed' | 'rejected' | string;
  phase: string;
  attempt: number;
  catalogRefreshAttempts: number;
  logicalRequestId?: string;
  manifestHash?: string;
  documentHash?: string;
  feasibilityBatchId?: string;
  feasibilitySetHash?: string;
  sealedBindingSetHash?: string;
  items?: RuntimeSemanticPlanningItem[];
  rejections?: RuntimeOriginalDiagnostic[];
  feasibilityGaps?: RuntimeSemanticFeasibilityGap[];
  failure?: RuntimeOriginalDiagnostic;
};

export type RuntimeSemanticPlanningItem = {
  coverageWorkItemId: string;
  status: string;
  proofs?: RuntimeSemanticPlanningProof[];
};

export type RuntimeSemanticPlanningProof = {
  evidenceKind?: string;
  proposalHash?: string;
  ownerModuleId?: string;
  operationKind?: string;
  status: string;
  targetRefs?: string[];
  validationRecipeId?: string;
  requiredCheckRefs?: string[];
  observedRevision?: string;
  diagnostic?: RuntimeOriginalDiagnostic;
};

export type RuntimeSemanticFeasibilityGap = {
  refreshGeneration: number;
  manifestHash: string;
  documentHash: string;
  feasibilityBatchId: string;
  feasibilitySetHash: string;
  items?: RuntimeSemanticPlanningItem[];
  rejections?: RuntimeOriginalDiagnostic[];
};

export type RuntimeOriginalDiagnostic = {
  schemaVersion: string;
  ownerModuleId: string;
  originalClass: string;
  originalCode: string;
  originalMessage: string;
  signature: string;
  operationId?: string;
  targetRefs?: string[];
  expectedRevision?: string;
  observedRevision?: string;
  fieldViolations?: RuntimeFieldViolation[];
  detailsRef?: string;
  detailsHash?: string;
  detailsBytes?: number;
  contentType?: string;
  omittedReason?: string;
  truncated?: boolean;
  redacted?: boolean;
  targetObservation?: RuntimeFailureTargetObservation;
  validationDiagnostic?: RuntimeValidationFailureDiagnostic;
  validationFailureObservation?: RuntimeValidationFailureObservation;
  governanceDependency?: RuntimeExternalDependency;
};

export type RuntimeFieldViolation = {
  path: string;
  actual?: string;
  allowed?: string[];
  required?: string[];
  forbidden?: string[];
};

export type RuntimeFailureTargetObservation = {
  ownerModuleId: string;
  subjectRef: string;
  targetRole: string;
  observedRevision: string;
  targetRefs: string[];
  diagnosticClass?: string;
  evidenceRefs?: string[];
  contentHash?: string;
};

export type RuntimeValidationFailureDiagnostic = {
  schemaVersion?: string;
  class?: string;
  reason?: string;
  recommendedEvidenceTargetType?: string;
  stage?: string;
  operation?: string;
  commandKind?: string;
  exitCode?: number;
  timedOut?: boolean;
  commandPolicyViolation?: boolean;
  unavailableCommand?: string;
  targetRefs?: string[];
  observationType?: string;
  evidenceMode?: string;
};

export type RuntimeValidationFailureObservation = {
  workspaceId: string;
  runId: string;
  authoritySnapshotId: string;
  dispatchId: string;
  operationId: string;
  failureSignature: string;
  targetRole: string;
  diagnosticClass: string;
  candidateTargetRefs: string[];
  validationScopeTargetRefs: string[];
  diagnosticHash: string;
};

export type RuntimeExternalDependency = {
  kind: string;
  subjectRef: string;
  reason: string;
};

export type RuntimeRepairDirective = {
  id?: string;
  focusId?: string;
  trigger?: string;
  requiredAction?: string;
  requiredEffectScopes?: string[];
  requiredEvidenceKinds?: string[];
  allowedOperationKinds?: string[];
  coverageWorkItemIds?: string[];
  targetRefs?: string[];
  closure?: RuntimeRepairClosure;
  supersedesEvidenceRevision?: string;
  repairAttempt?: number;
};

export type RuntimeRepairClosure = {
  id?: string;
  gapSignature?: string;
  baselineEvidenceRevision?: string;
  coverageWorkItemIds?: string[];
  requiredEvidenceKinds?: string[];
  groundedTargetRefs?: string[];
  requireValidationAfterStateChange?: boolean;
};

// Audit is display-only. In particular, gap.missingEvidenceKinds is an LLM
// claim and cannot be used to select action, retries or any command.
export type RuntimeAudit = {
  repair: RuntimeAuditRepair;
  assessments?: RuntimeAuditAssessment[];
  failure?: RuntimeAuditFailure;
  // Immutable activation declarations, derived SM-2 credit, and owner facts
  // are intentionally separate. They are audit data only and never commands.
  coverageBindings?: RuntimeAuditCoverageBinding[];
  dispatches?: RuntimeAuditDispatch[];
  acceptedEvidence?: RuntimeAuditEvidence[];
  rejectedEvidence?: RuntimeAuditEvidence[];
  operationReceipts?: RuntimeAuditOperationReceipt[];
};

export type RuntimeAuditCoverageBinding = {
  id?: string;
  coverageWorkItemId?: string;
  focusId?: string;
  definitionRevision?: string;
  hash?: string;
  proofs?: RuntimeAuditCoverageProof[];
  targetRefs?: string[];
  targetSelectorRef?: string;
  targetSelectorOwnerRef?: string;
  targetSelectorSubjectRef?: string;
  prospectiveTargetRef?: string;
  declarationRef?: string;
  prerequisiteRefs?: string[];
};

export type RuntimeAuditCoverageProof = {
  evidenceKind?: string;
  requiredEffectScopes?: string[];
  allowedOperationKinds?: string[];
  allowedOperationVariants?: string[];
  validationRecipeId?: string;
  requiredCheckRefs?: string[];
  metricConstraints?: string[];
  artifactRefs?: string[];
};

// A bounded, read-only view of the SM-3 sealed operation authorization. It
// lets operators correlate bindingRefs with the later receipt/evidence chain;
// it cannot invoke, retry, or modify an operation.
export type RuntimeAuditDispatch = {
  id?: string;
  authoritySnapshotId?: string;
  state?: string;
  candidateHash?: string;
  completedOperations?: number;
  operations?: RuntimeAuditSealedOperation[];
};

export type RuntimeAuditSealedOperation = {
  id?: string;
  kind?: string;
  bindingRefs?: string[];
  targetRefs?: string[];
};

// CoverageWorkItemIds here are already SM-2-derived ledger credit. Candidate
// coverage claims are neither represented nor accepted by this API shape.
export type RuntimeAuditEvidence = {
  id?: string;
  kind?: string;
  authoritySnapshotId?: string;
  dispatchId?: string;
  operationId?: string;
  revision?: string;
  failureClass?: string;
  failureCode?: string;
  failureSignature?: string;
  targetRefs?: string[];
  bindingRefs?: string[];
  coverageWorkItemIds?: string[];
  quality?: string;
  zeroWork?: boolean;
  evidenceReason?: string;
  evidenceRefs?: string[];
  evidenceTags?: string[];
  validationRecipeId?: string;
  executedChecks?: string[];
  passedChecks?: string[];
  failedChecks?: string[];
  skippedChecks?: string[];
};

export type RuntimeAuditOperationReceipt = {
  executionKey?: string;
  fence?: number;
  operationId?: string;
  outcome?: string;
  observedEffectScope?: string;
  changed?: boolean;
  feedbackDomain?: string;
  stableCategory?: string;
  originalDiagnostic?: RuntimeOriginalDiagnostic;
  targetRefs?: string[];
  evidenceRefs?: string[];
  evidenceTags?: string[];
  validationRecipeId?: string;
  executedChecks?: string[];
  passedChecks?: string[];
  failedChecks?: string[];
  skippedChecks?: string[];
};

export type RuntimeAuditRepair = {
  focusId?: string;
  sliceId?: string;
  repairAttempts: number;
  validationOnlyRetries: number;
  noProgressCount: number;
  noProgressOperationShapeCount?: number;
  activeGapCoverageWorkItemIds?: string[];
  activeGapFailureClass?: string;
  activeGapFailureCode?: string;
  activeGapFailureSignature?: string;
  lastProgressRevision?: string;
  exhausted?: boolean;
};

export type RuntimeAuditAssessment = {
  id?: string;
  focusId?: string;
  sliceId?: string;
  scope?: string;
  requirementId?: string;
  requirementText?: string;
  claimedDisposition?: string;
  canonicalDisposition?: string;
  coverageWorkItemIds?: string[];
  summary?: string;
  evidenceRefs?: string[];
  artifactRefs?: string[];
  items?: RuntimeAuditAssessmentItem[];
  gap?: RuntimeAuditAssessmentGap;
};

export type RuntimeAuditAssessmentItem = {
  coverageWorkItemId?: string;
  satisfied: boolean;
  reason?: string;
  evidenceRefs?: string[];
  artifactRefs?: string[];
};

export type RuntimeAuditAssessmentGap = {
  coverageWorkItemIds?: string[];
  missingEvidenceKinds?: string[];
  targetRefs?: string[];
  evidenceRefs?: string[];
  reason?: string;
  failureClass?: string;
  failureCode?: string;
  failureSignature?: string;
};

// Failure and originalDiagnostic are audit-only producer facts. They are not
// a recovery directive; the only current action source is repairDirective.
export type RuntimeAuditFailure = {
  class?: string;
  code?: string;
  signature?: string;
  message?: string;
  targetRefs?: string[];
  originalDiagnostic?: RuntimeOriginalDiagnostic;
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
  runtime?: RuntimeProgress;
};

export type RuntimeProgress = {
  stateRevision?: number;
  evidenceSequence?: number;
  completedDispatchCount?: number;
  activeDispatchId?: string;
  candidateAttemptCount?: number;
  acceptedEvidenceCount?: number;
  acceptedValidationEvidenceCount?: number;
  rejectedEvidenceCount?: number;
  missingEvidenceKinds?: string[];
  currentFocusId?: string;
  convergenceState?: string;
  terminalCode?: string;
  failureClass?: string;
  failureCode?: string;
  failureSignature?: string;
  failureMessage?: string;
  repairHintCodes?: string[];
  prerequisiteRefreshState?: string;
  prerequisiteRefreshUsed?: number;
  prerequisiteRefreshLimit?: number;
  prerequisiteResourceChangeRefreshUsed?: number;
  prerequisiteResourceChangeRefreshLimit?: number;
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
  runId?: string;
  stepId?: string;
  blockingTicketId?: string;
  blockingApprovalId?: string;
  checkpointId?: string;
  failureClass?: string;
  failureSignature?: string;
  reason?: string;
  blockingResource?: BlockingResourceRef;
  suggestedNavigation?: string;
};

export type BlockingResourceRef = {
  ownerKind: string;
  resourceId: string;
  workspaceId: string;
  runId: string;
  state: string;
  requiredAction?: string;
  allowedUserActions?: string[];
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

export type GovernanceDisplay = {
  promptMode?: string;
  runId?: string;
  decisionId?: string;
  approvalId?: string;
};

export type ConsistencyDiagnostic = {
  code?: string;
  summary?: string;
  canonicalWorkspaceState?: string;
  canonicalWorkspaceRunId?: string;
  canonicalRunState?: string;
  canonicalRunDecisionId?: string;
  canonicalRunApprovalId?: string;
  runtimeConvergenceState?: string;
  observedDecisionId?: string;
  observedApprovalId?: string;
};

export type RunObservability = {
  run: Run;
  displayOutcome?: RunDisplayOutcome;
  governanceDisplay?: GovernanceDisplay;
  consistencyDiagnostic?: ConsistencyDiagnostic;
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
  displayOutcome?: RunDisplayOutcome;
  governanceDisplay?: GovernanceDisplay;
  consistencyDiagnostic?: ConsistencyDiagnostic;
  taskProgress: TaskProgress;
  runHeartbeat?: RunHeartbeat;
  repairLoop: RepairLoop;
  acceptance: AcceptanceProjection;
  gapBaseline?: GapBaseline;
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
  displayOutcome?: WorkspaceDisplayOutcome;
  governanceDisplay?: GovernanceDisplay;
  consistencyDiagnostic?: ConsistencyDiagnostic;
  taskProgress: TaskProgress;
  runHeartbeat?: RunHeartbeat;
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
  failureClass?: string;
  failureSignature?: string;
  question?: string;
  context?: Record<string, unknown>;
  allowedRecoveryActions?: string[];
  definitionClarification?: DefinitionClarification;
  state: string;
  resolutionId?: string;
  resolutionType?: string;
  resolutionPayload?: Record<string, unknown>;
  continuationAction?: string;
  resumeRunId?: string;
  successorRunId?: string;
  autoGovernance?: AutoGovernanceSummary;
};

export type DefinitionClarification = {
  class: 'workspace_definition_pre_seal' | 'runtime_definition' | string;
  bindingState: 'open' | 'admission_bound' | 'replacement_intent_bound' | 'resolved' | string;
  openBindingHash?: string;
  requiredAction: string;
  candidateId?: string;
  candidateRevision?: number;
  candidateHash?: string;
  violationOrGapCode: string;
  canonicalSignatureVersion?: string;
  canonicalSignature?: string;
  canonicalSignatureHash?: string;
  offendingRefs?: string[];
  definitionRevision?: string;
  startupInputHash?: string;
  authoritySnapshotId?: string;
  clarificationDispatchId?: string;
  focusRef?: string;
  logicalRequestId?: string;
  semanticDocumentHash?: string;
  resolutionId?: string;
  resolutionBindingHash?: string;
  replacementIntentId?: string;
  replacementIntentHash?: string;
  successorDefinitionRevision?: string;
  successorStartupInputHash?: string;
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
