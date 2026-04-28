package common

type WorkspaceState string

type DoDState string

type RunState string

type StepState string

type DecisionTicketState string

type ApprovalState string
type ToolCallState string
type PatchState string
type ValidationResultState string
type CheckpointType string
type ResultState string
type ArtifactState string

const (
	WorkspaceStateActive          WorkspaceState = "active"
	WorkspaceStateDraft           WorkspaceState = "draft"
	WorkspaceStateReady           WorkspaceState = "ready"
	WorkspaceStateRunning         WorkspaceState = "running"
	WorkspaceStateWaitingDecision WorkspaceState = "waiting_decision"
	WorkspaceStateBlocked         WorkspaceState = "blocked"
	WorkspaceStateValidating      WorkspaceState = "validating"
	WorkspaceStateCompleted       WorkspaceState = "completed"
	WorkspaceStateFailed          WorkspaceState = "failed"
	WorkspaceStateArchived        WorkspaceState = "archived"
)

const (
	DoDStateDraft     DoDState = "draft"
	DoDStateReady     DoDState = "ready"
	DoDStateSatisfied DoDState = "satisfied"
)

const (
	RunStateCreated           RunState = "created"
	RunStateRunning           RunState = "running"
	RunStateBlockedOnDecision RunState = "blocked_on_decision"
	RunStateBlockedOnApproval RunState = "blocked_on_approval"
	RunStateValidating        RunState = "validating"
	RunStateCompleted         RunState = "completed"
	RunStateFailed            RunState = "failed"
	RunStateCancelled         RunState = "cancelled"
	RunStateSuperseded        RunState = "superseded"
)

const (
	StepStatePending    StepState = "pending"
	StepStateDispatched StepState = "dispatched"
	StepStateCompleted  StepState = "completed"
	StepStateFailed     StepState = "failed"
)

const (
	DecisionTicketStateOpen      DecisionTicketState = "open"
	DecisionTicketStateResolved  DecisionTicketState = "resolved"
	DecisionTicketStateDismissed DecisionTicketState = "dismissed"
)

const (
	ApprovalStatePending  ApprovalState = "pending"
	ApprovalStateApproved ApprovalState = "approved"
	ApprovalStateRejected ApprovalState = "rejected"
	ApprovalStateResolved ApprovalState = "resolved"
)

const (
	ToolCallStatePending   ToolCallState = "pending"
	ToolCallStateRunning   ToolCallState = "running"
	ToolCallStateCompleted ToolCallState = "completed"
	ToolCallStateFailed    ToolCallState = "failed"
)

const (
	PatchStateCreated PatchState = "created"
	PatchStateApplied PatchState = "applied"
	PatchStateFailed  PatchState = "failed"
)

const (
	ValidationResultStatePassed  ValidationResultState = "passed"
	ValidationResultStateFailed  ValidationResultState = "failed"
	ValidationResultStateWarning ValidationResultState = "warning"
)

const (
	CheckpointTypeRunState CheckpointType = "run_state"
	CheckpointTypeBlocked  CheckpointType = "blocked"
	CheckpointTypeRecovery CheckpointType = "recovery"
)

const (
	ResultStateDraft      ResultState = "draft"
	ResultStateAccepted   ResultState = "accepted"
	ResultStateSuperseded ResultState = "superseded"
)

const (
	ArtifactStateCreated   ArtifactState = "created"
	ArtifactStateAvailable ArtifactState = "available"
	ArtifactStateFailed    ArtifactState = "failed"
)
