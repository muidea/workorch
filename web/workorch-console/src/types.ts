export type ServiceStatus = {
  service: string;
  status: string;
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
  state: string;
  blockingTicketId?: string;
  blockingApprovalId?: string;
  failureReason?: string;
};

export type StepView = {
  id: string;
  runId: string;
  kind?: string;
  state: string;
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

export type SliceDescriptor = {
  id: string;
  title: string;
};

export type SliceStatus = {
  id: string;
  title: string;
  slice: string;
  workspaceId?: string;
  workspaceState?: string;
  runId?: string;
  runState?: string;
  stepId?: string;
  stepState?: string;
  blockingTicketId?: string;
  blockingApprovalId?: string;
  stepCount: number;
  observedCount: number;
};
