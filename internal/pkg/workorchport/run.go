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
	SealedLLMTarget     *runtimecontract.ProviderModelTarget
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
	// RunCommandIntentResume is the only executable Runtime V2 user
	// continuation. Historical restart/replan values may be displayed from
	// persisted records, but callers must not submit them as commands.
	RunCommandIntentResume RunCommandIntent = "resume"
	// RunCommandIntentRestart is retained only to decode/display historical
	// records. NormalizeRunCommandIntent rejects it for command submission.
	RunCommandIntentRestart RunCommandIntent = "restart"
	// RunCommandIntentReplan is a Decision ingress audit intent, never a
	// Runtime V2 workspace-run command.
	RunCommandIntentReplan RunCommandIntent = "replan"
)

func NormalizeRunCommandIntent(intent RunCommandIntent) (RunCommandIntent, error) {
	switch intent {
	case "":
		return RunCommandIntentResume, nil
	case RunCommandIntentResume:
		return RunCommandIntentResume, nil
	case RunCommandIntentRestart, RunCommandIntentReplan:
		return "", fmt.Errorf("%w: runtime continuation command must be resume, got %q", common.ErrInvalidArgument, intent)
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
	ConvergenceState              string
	SuggestedNextAction           string
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
