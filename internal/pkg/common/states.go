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
type EvidenceQuality string
type CheckpointType string
type ResultState string
type ArtifactState string
type ArtifactContinuationState string

const (
	WorkspaceStateActive            WorkspaceState = "active"
	WorkspaceStateBlockedOnApproval WorkspaceState = "blocked_on_approval"
	WorkspaceStateSuperseded        WorkspaceState = "superseded"
)

const (
	DoDStateDraft     DoDState = "draft"
	DoDStateReady     DoDState = "ready"
	DoDStateSatisfied DoDState = "satisfied"
)

const (
	RunStatePending           RunState = "pending"
	RunStateActive            RunState = "active"
	RunStateBlockedOnDecision RunState = "blocked_on_decision"
	RunStateBlockedOnApproval RunState = "blocked_on_approval"
	RunStateCompleted         RunState = "completed"
	RunStateFailed            RunState = "failed"
	RunStateCancelled         RunState = "cancelled"
	RunStateSuperseded        RunState = "superseded"
)

const (
	StepStatePending   StepState = "pending"
	StepStateExecuting StepState = "executing"
	StepStateSucceeded StepState = "succeeded"
	StepStateFailed    StepState = "failed"
	StepStateSkipped   StepState = "skipped"
	StepStateCancelled StepState = "cancelled"
)

const (
	DecisionTicketStateOpen       DecisionTicketState = "open"
	DecisionTicketStateResolved   DecisionTicketState = "resolved"
	DecisionTicketStateSuperseded DecisionTicketState = "superseded"
)

const (
	ApprovalStatePending    ApprovalState = "pending"
	ApprovalStateApproved   ApprovalState = "approved"
	ApprovalStateRejected   ApprovalState = "rejected"
	ApprovalStateSuperseded ApprovalState = "superseded"
)

const (
	ToolCallStatePending   ToolCallState = "pending"
	ToolCallStateRunning   ToolCallState = "running"
	ToolCallStateCompleted ToolCallState = "completed"
	ToolCallStateFailed    ToolCallState = "failed"
)

const (
	PatchStateCreated   PatchState = "created"
	PatchStatePreviewed PatchState = "previewed"
	PatchStateApplied   PatchState = "applied"
	PatchStateFailed    PatchState = "failed"
	PatchStateReverted  PatchState = "reverted"
)

const (
	ValidationResultStatePassed       ValidationResultState = "passed"
	ValidationResultStateFailed       ValidationResultState = "failed"
	ValidationResultStateInconclusive ValidationResultState = "inconclusive"
	ValidationResultStateWarning      ValidationResultState = "warning"
)

const (
	EvidenceQualityValid        EvidenceQuality = "valid"
	EvidenceQualityInvalid      EvidenceQuality = "invalid"
	EvidenceQualityInconclusive EvidenceQuality = "inconclusive"
	EvidenceQualitySkipped      EvidenceQuality = "skipped"
)

const (
	CheckpointTypeRunState CheckpointType = "run_state"
	CheckpointTypeBlocked  CheckpointType = "blocked"
	CheckpointTypeRecovery CheckpointType = "recovery"
)

const (
	ResultStateDraft      ResultState = "draft"
	ResultStateAccepted   ResultState = "accepted"
	ResultStateRejected   ResultState = "rejected"
	ResultStateSuperseded ResultState = "superseded"
)

const (
	ArtifactStateCreated   ArtifactState = "created"
	ArtifactStateAvailable ArtifactState = "available"
	ArtifactStateFailed    ArtifactState = "failed"
)

const (
	ArtifactContinuationStateActive  ArtifactContinuationState = "active"
	ArtifactContinuationStateSealed  ArtifactContinuationState = "sealed"
	ArtifactContinuationStateAborted ArtifactContinuationState = "aborted"

	ArtifactContinuationStatePlanning   ArtifactContinuationState = "planning"
	ArtifactContinuationStateChunking   ArtifactContinuationState = "chunking"
	ArtifactContinuationStateAssembling ArtifactContinuationState = "assembling"
	ArtifactContinuationStateApplying   ArtifactContinuationState = "applying"
	ArtifactContinuationStateApplied    ArtifactContinuationState = "applied"
	ArtifactContinuationStateFailed     ArtifactContinuationState = "failed"
)
