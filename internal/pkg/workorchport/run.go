package workorchport

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ActivateRunRequest is retained only as an owner-neutral DTO used by local
// test fixtures. Runtime ingress never accepts or transports it: all
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
	// RunCommandIntentResume is the strict Runtime continuation intent.
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

type WorkspaceRunCommand struct {
	WorkspaceID common.WorkspaceID
	Intent      RunCommandIntent
	Reason      string
	Source      string
	RequestID   string
	Priority    int
}

// RunCommandResult is the process-facing projection of the Workspace owner's
// immutable resume/restart admission fact. It does not carry a Run entity or
// request a Run lifecycle state transition.
type RunCommandResult runtimecontract.RuntimeExecutionIntentAdmission

func (r RunCommandResult) Validate() error {
	if err := (runtimecontract.RuntimeExecutionIntentAdmission(r)).Validate(); err != nil {
		return fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	return nil
}

type RunBackend interface {
	Command(ctx context.Context, req WorkspaceRunCommand) (*RunCommandResult, error)
	Get(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*projectioncontract.Run, error)
	List(ctx context.Context, wsID common.WorkspaceID) ([]projectioncontract.Run, error)
	Status(ctx context.Context, wsID common.WorkspaceID, id common.RunID) (*projectioncontract.RunStatus, error)
}

// RunStepKind returns the canonical resource kind from a run's initial step.
func RunStepKind() string { return "llm/workspace_execution" }
