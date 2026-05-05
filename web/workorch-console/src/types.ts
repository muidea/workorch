export type ServiceStatus = {
  service: string;
  status: string;
};

export type ServiceConfiguration = {
  server: ServerConfiguration;
  repository: RepositoryConfiguration;
  runtime: RuntimeConfiguration;
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
  defaultDialogLanguage?: string;
};

export type RepositoryConfiguration = {
  backend?: string;
  dataDir?: string;
};

export type RuntimeConfiguration = {
  defaultSoulId?: string;
};

export type LLMConfiguration = {
  defaultProviderId?: string;
  defaultEmbeddingProviderId?: string;
  providers?: LLMProviderConfiguration[];
  providerTemplates?: LLMProviderTemplate[];
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
};

export type Workspace = {
  id: string;
  title: string;
  description?: string;
  workingDirectory: string;
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
  reason?: string;
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
};

export type RunObservability = {
  run: Run;
  outcome?: RunOutcome;
  progress: RunProgress;
  taskProgress: TaskProgress;
  taskDecompositions?: TaskDecomposition[];
  executionSections?: ExecutionSection[];
  memoryRecallTraces?: MemoryRecallTrace[];
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
  taskDecompositions?: TaskDecomposition[];
  executionSections?: ExecutionSection[];
  memoryRecallTraces?: MemoryRecallTrace[];
  steps?: StepView[];
  toolCalls?: ToolCall[];
  patches?: Patch[];
  validationResults?: ValidationResult[];
  runtimeCheckpoints?: RuntimeCheckpoint[];
  results?: Result[];
  artifacts?: Artifact[];
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
  taskDecompositions?: TaskDecomposition[];
  executionSections?: ExecutionSection[];
  runs?: Run[];
  results?: Result[];
  artifacts?: Artifact[];
  patches?: Patch[];
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
};

export type Approval = {
  id: string;
  workspaceId: string;
  runId: string;
  stepId?: string;
  reason: string;
  state: string;
};
