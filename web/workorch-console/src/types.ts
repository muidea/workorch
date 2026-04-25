export type ServiceStatus = {
  service: string;
  status: string;
};

export type Workspace = {
  id: string;
  title: string;
  state: string;
  activeRunId?: string;
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

export type Decision = {
  id: string;
  workspaceId: string;
  runId: string;
  stepId?: string;
  reason: string;
  state: string;
  resolutionId?: string;
  resolutionType?: string;
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
