package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type RunInfo struct {
	ID                 common.RunID
	WorkspaceID        common.WorkspaceID
	GoalID             common.GoalID
	DoDID              common.DoDID
	ExecutionContext   map[string]any
	State              common.RunState
	Priority           int
	BlockingTicketID   common.TicketID
	BlockingApprovalID common.ApprovalID
	FailureReason      string
}

type ActivateRunRequest struct {
	GoalID                common.GoalID
	DoDID                 common.DoDID
	LLMProviderID         string
	DialogLanguage        string
	SoulID                common.SoulID
	SkillIDs              []common.SkillID
	MCPServerIDs          []common.MCPServerID
	Priority              int
	ExecutionContextPatch map[string]any
}

type RunStatusInfo struct {
	ID                 common.RunID
	WorkspaceID        common.WorkspaceID
	State              common.RunState
	CurrentStepID      common.StepID
	CurrentStepKind    string
	CurrentStepState   common.StepState
	StepCount          int
	DecisionCount      int
	ApprovalCount      int
	BlockingTicketID   common.TicketID
	BlockingApprovalID common.ApprovalID
	FailureReason      string
	RuntimeExecution   *RuntimeExecutionInfo
}

type RuntimeExecutionInfo struct {
	StepID              common.StepID
	StepKind            string
	StepState           common.StepState
	RequestID           string
	QueueKey            string
	AcceptedAt          string
	LastProgressAt      string
	LastProgressEvent   string
	LastProgressStage   string
	LastProgressSummary string
	WorkerID            string
	WorkerStartedAt     string
	WorkerHeartbeatAt   string
	WorkerStoppedAt     string
	WorkerLostAt        string
	DoneAt              string
	FailedAt            string
	FinalizedAt         string
	FinalizeEvent       string
	Pending             bool
	Gap                 string
}

type RunBackend interface {
	Activate(ctx context.Context, wsID common.WorkspaceID, req ActivateRunRequest) (*RunInfo, error)
	Restart(ctx context.Context, wsID common.WorkspaceID, req ActivateRunRequest) (*RunInfo, error)
	Resume(ctx context.Context, wsID common.WorkspaceID, id common.RunID, executionContextPatch ...map[string]any) (*RunInfo, error)
	Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*RunInfo, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]RunInfo, error)
	Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*RunStatusInfo, error)
}

// RunStepKind returns the canonical resource kind from a run's initial step.
func RunStepKind() string { return "llm/workspace_execution" }
