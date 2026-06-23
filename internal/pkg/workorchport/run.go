package workorchport

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RunInfo struct {
	ID                 common.RunID
	WorkspaceID        common.WorkspaceID
	GoalID             common.GoalID
	DoDID              common.DoDID
	DefinitionRevision string
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
	RequestID             string
	Reason                string
	Source                string
	DefinitionRevision    string
	DecisionID            common.TicketID
	ExecutionContextPatch map[string]any
}

type RunCommandIntent string

const (
	RunCommandIntentResume  RunCommandIntent = "resume"
	RunCommandIntentRestart RunCommandIntent = "restart"
	RunCommandIntentReplan  RunCommandIntent = "replan"
	RunCommandIntentRecover RunCommandIntent = "recover"
)

func NormalizeRunCommandIntent(intent RunCommandIntent) (RunCommandIntent, error) {
	switch intent {
	case "":
		return RunCommandIntentResume, nil
	case RunCommandIntentResume, RunCommandIntentRestart, RunCommandIntentReplan, RunCommandIntentRecover:
		return intent, nil
	default:
		return "", fmt.Errorf("%w: unsupported run command intent %q", common.ErrInvalidArgument, intent)
	}
}

type WorkspaceRunCommand struct {
	WorkspaceID           common.WorkspaceID
	ExpectedRunID         common.RunID
	Intent                RunCommandIntent
	Reason                string
	Source                string
	RequestID             string
	DefinitionRevision    string
	DecisionID            common.TicketID
	Priority              int
	ExecutionContextPatch map[string]any
	Activation            ActivateRunRequest
}

type RunStatusInfo struct {
	ID                   common.RunID
	WorkspaceID          common.WorkspaceID
	State                common.RunState
	CurrentStepID        common.StepID
	CurrentStepKind      string
	CurrentStepState     common.StepState
	StepCount            int
	DecisionCount        int
	ApprovalCount        int
	BlockingTicketID     common.TicketID
	BlockingApprovalID   common.ApprovalID
	FailureReason        string
	RuntimeExecution     *RuntimeExecutionInfo
	SchedulerDiagnostics *SchedulerDiagnosticsInfo
	Acceptance           AcceptanceProjectionInfo
}

type RuntimeExecutionInfo struct {
	StepID              common.StepID
	StepKind            string
	StepState           common.StepState
	RequestID           string
	QueueKey            string
	ContractViolation   *runtimemodel.ContractViolation
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
	Command(ctx context.Context, req WorkspaceRunCommand) (*RunInfo, error)
	Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*RunInfo, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]RunInfo, error)
	Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*RunStatusInfo, error)
}

// RunStepKind returns the canonical resource kind from a run's initial step.
func RunStepKind() string { return "llm/workspace_execution" }
