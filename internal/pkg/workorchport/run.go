package workorchport

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RunInfo struct {
	ID                  common.RunID
	WorkspaceID         common.WorkspaceID
	GoalID              common.GoalID
	DoDID               common.DoDID
	DefinitionRevision  string
	PredecessorRunID    common.RunID
	PredecessorResultID common.ResultID
	SuccessorRunID      common.RunID
	State               common.RunState
	Priority            int
	BlockingTicketID    common.TicketID
	BlockingApprovalID  common.ApprovalID
	FailureReason       string
}

type ActivateRunRequest struct {
	GoalID             common.GoalID
	DoDID              common.DoDID
	LLMTarget          *runtimecontract.ProviderModelTarget
	DialogLanguage     string
	SoulID             common.SoulID
	SoulBindings       map[string]common.SoulID
	SkillIDs           []common.SkillID
	MCPServerIDs       []common.MCPServerID
	CapabilityIDs      []common.MCPCapabilityID
	MemoryIDs          []common.MemoryID
	Priority           int
	RequestID          string
	Reason             string
	Source             string
	DefinitionRevision string
	DecisionID         common.TicketID
}

type RunCommandIntent string

const (
	RunCommandIntentResume  RunCommandIntent = "resume"
	RunCommandIntentRestart RunCommandIntent = "restart"
	RunCommandIntentReplan  RunCommandIntent = "replan"
)

func NormalizeRunCommandIntent(intent RunCommandIntent) (RunCommandIntent, error) {
	switch intent {
	case "":
		return RunCommandIntentResume, nil
	case RunCommandIntentResume, RunCommandIntentRestart, RunCommandIntentReplan:
		return intent, nil
	default:
		return "", fmt.Errorf("%w: unsupported run command intent %q", common.ErrInvalidArgument, intent)
	}
}

func ValidateWorkspaceRunCommand(req WorkspaceRunCommand) (RunCommandIntent, error) {
	if req.WorkspaceID == "" {
		return "", fmt.Errorf("%w: workspaceId is required", common.ErrInvalidArgument)
	}
	intent, err := NormalizeRunCommandIntent(req.Intent)
	if err != nil {
		return "", err
	}
	switch intent {
	case RunCommandIntentRestart:
		if req.ExpectedRunID != "" {
			return "", fmt.Errorf("%w: workspace restart must not target runId", common.ErrInvalidArgument)
		}
	case RunCommandIntentReplan:
		if req.ExpectedRunID == "" {
			return "", fmt.Errorf("%w: expectedRunId is required for %s", common.ErrInvalidArgument, intent)
		}
	}
	return intent, nil
}

type WorkspaceRunCommand struct {
	WorkspaceID        common.WorkspaceID
	ExpectedRunID      common.RunID
	Intent             RunCommandIntent
	Reason             string
	Source             string
	RequestID          string
	DefinitionRevision string
	DecisionID         common.TicketID
	Priority           int
	Activation         ActivateRunRequest
}

type RunStatusInfo struct {
	ID                   common.RunID
	WorkspaceID          common.WorkspaceID
	State                common.RunState
	CurrentStepID        common.StepID
	CurrentStepKind      string
	CurrentStepState     common.StepState
	CurrentActionLabel   string
	DiagnosticLineage    common.StepID
	StepCount            int
	DecisionCount        int
	ApprovalCount        int
	BlockingTicketID     common.TicketID
	BlockingApprovalID   common.ApprovalID
	FailureReason        string
	ConvergenceStatus    *ConvergenceStatusInfo
	SchedulerDiagnostics *SchedulerDiagnosticsInfo
	Acceptance           AcceptanceProjectionInfo
}

type ConvergenceStatusInfo struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	RunState                      common.RunState
	CurrentSliceID                string
	CurrentFocusID                string
	NextActionKind                string
	CurrentGap                    string
	MissingEvidenceKinds          []string
	AcceptedEvidenceCount         int
	RejectedEvidenceCount         int
	ClosedCoverageItemCount       int
	RemainingCoverageItemCount    int
	AssessmentDisposition         string
	RepairAttempts                int
	MaxRepairAttempts             int
	RepeatedFailureSignatureCount int
	LatestAcceptedEvidenceRef     string
	NextActivationRequired        bool
	GapSummary                    string
	LatestEffectiveProgressAt     string
	LastProgressAt                string
	DecisionCause                 string
	Stalled                       bool
	IneffectiveRetry              bool
}

type RunBackend interface {
	Command(ctx context.Context, req WorkspaceRunCommand) (*RunInfo, error)
	Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*RunInfo, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]RunInfo, error)
	Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*RunStatusInfo, error)
}

// RunStepKind returns the canonical resource kind from a run's initial step.
func RunStepKind() string { return "llm/workspace_execution" }
