package common

type WorkspaceState string

type DoDState string

type RunState string

type StepState string

type DecisionTicketState string

type ApprovalState string

const (
	WorkspaceStateActive   WorkspaceState = "active"
	WorkspaceStateArchived WorkspaceState = "archived"
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
	RunStateCompleted         RunState = "completed"
	RunStateFailed            RunState = "failed"
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
