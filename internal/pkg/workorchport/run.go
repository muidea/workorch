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
	RuntimeActivation   *RuntimeActivationStatusInfo
	// ExecutionDisposition and ExecutionDiagnostic are the authoritative
	// Workspace-owner admission outcome returned by a resume/restart command.
	// They are display metadata, not Run state and not a lifecycle input.
	ExecutionDisposition string
	ExecutionDiagnostic  string
}

// ActivateRunRequest is retained only as an owner-neutral DTO used by local
// test fixtures. Runtime V2 ingress never accepts or transports it: all
// activation inputs are compiled from current Workspace owner state.
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
	// RunCommandIntentResume is the strict Runtime V2 continuation intent.
	// It never replaces an active Run implicitly.
	RunCommandIntentResume RunCommandIntent = "resume"
	// RunCommandIntentRestart is an explicit workspace-scoped fresh activation.
	// It must not be addressed to a particular Run: ordinary active Runs follow
	// their terminal/supersede owner path, while a non-resumable Run is released
	// by the Workspace owner from its persisted Run-owned fact.
	RunCommandIntentRestart RunCommandIntent = "restart"
)

func NormalizeRunCommandIntent(intent RunCommandIntent) (RunCommandIntent, error) {
	switch intent {
	case "":
		return RunCommandIntentResume, nil
	case RunCommandIntentResume:
		return RunCommandIntentResume, nil
	case RunCommandIntentRestart:
		return RunCommandIntentRestart, nil
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

// RuntimeActivationStatusInfo is a read-only Run-owner administrative status.
// It tells clients whether the persisted Run can accept a strict continuation;
// it is neither an Orchestrator action nor a projection-owned state.
type RuntimeActivationStatusInfo struct {
	Disposition            string
	ActualSchemaVersion    uint32
	SupportedSchemaVersion uint32
}

type WorkspaceRunCommand struct {
	WorkspaceID common.WorkspaceID
	Intent      RunCommandIntent
	Reason      string
	Source      string
	RequestID   string
	Priority    int
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
