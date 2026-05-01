package appservice

import (
	"context"
	"encoding/json"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Run struct {
	ID                 string         `json:"id"`
	WorkspaceID        string         `json:"workspaceId"`
	GoalID             string         `json:"goalId,omitempty"`
	DoDID              string         `json:"doDId,omitempty"`
	InitialStepKind    string         `json:"initialStepKind,omitempty"`
	ExecutionContext   map[string]any `json:"executionContext,omitempty"`
	State              string         `json:"state"`
	Priority           int            `json:"priority,omitempty"`
	BlockingTicketID   string         `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string         `json:"blockingApprovalId,omitempty"`
	FailureReason      string         `json:"failureReason,omitempty"`
}

func CreateRunWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, runID string, goalID string, dodID string) (ret Run, err error) {
	service, repo, err := runDependenciesFromSession(session)
	if err != nil {
		return Run{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	if err := validateRunDefinitions(ctx, session, workspaceID, goalID, dodID); err != nil {
		return Run{}, err
	}
	var executionContext map[string]any
	if goalID != "" || dodID != "" {
		executionContext, err = buildWorkspaceExecutionContext(
			ctx,
			session,
			common.WorkspaceID(workspaceID),
			common.GoalID(goalID),
			common.DoDID(dodID),
			ActivateWorkspaceExecutionCommand{},
		)
		if err != nil {
			return Run{}, err
		}
	}

	id := common.RunID(runID)
	if err := service.CreateRun(ctx, runmodule.CreateRunCommand{
		RunID:            id,
		WorkspaceID:      common.WorkspaceID(workspaceID),
		GoalID:           common.GoalID(goalID),
		DoDID:            common.DoDID(dodID),
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: executionContext,
	}); err != nil {
		return Run{}, err
	}

	item, err := repo.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	return mapRun(item), nil
}

func CreateRunWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string, goalID string, dodID string) (Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Run{}, err
	}
	return CreateRunWithSession(ctx, session, workspaceID, runID, goalID, dodID)
}

func CreateRunFromEnv(ctx context.Context, workspaceID string, runID string, goalID string, dodID string) (Run, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Run{}, err
	}
	return CreateRunWithSession(ctx, session, workspaceID, runID, goalID, dodID)
}

func StartRunWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret Run, err error) {
	service, repo, err := runDependenciesFromSession(session)
	if err != nil {
		return Run{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	id := common.RunID(runID)
	if err := attachCompressedContextToRun(ctx, session, id); err != nil {
		return Run{}, err
	}
	if err := service.EnsureStarted(ctx, runmodule.StartRunCommand{RunID: id}); err != nil {
		return Run{}, err
	}

	item, err := repo.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	return mapRun(item), nil
}

func StartRunWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Run{}, err
	}
	return StartRunWithSession(ctx, session, runID)
}

func StartRunFromEnv(ctx context.Context, runID string) (Run, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Run{}, err
	}
	return StartRunWithSession(ctx, session, runID)
}

func ResumeRunWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret Run, err error) {
	service, repo, err := runDependenciesFromSession(session)
	if err != nil {
		return Run{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	id := common.RunID(runID)
	patch, err := compressedContextPatchForRun(ctx, session, id)
	if err != nil {
		return Run{}, err
	}
	if err := service.ResumeRun(ctx, runmodule.ResumeRunCommand{RunID: id, ExecutionContextPatch: patch}); err != nil {
		return Run{}, err
	}

	item, err := repo.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	return mapRun(item), nil
}

func ResumeRunWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Run{}, err
	}
	return ResumeRunWithSession(ctx, session, runID)
}

func ResumeRunFromEnv(ctx context.Context, runID string) (Run, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Run{}, err
	}
	return ResumeRunWithSession(ctx, session, runID)
}

func ReadRunWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret Run, err error) {
	_, repo, err := runDependenciesFromSession(session)
	if err != nil {
		return Run{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	item, err := repo.Get(ctx, common.RunID(runID))
	if err != nil {
		return Run{}, err
	}
	return mapRun(item), nil
}

func ReadRunWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Run{}, err
	}
	return ReadRunWithSession(ctx, session, runID)
}

func ReadRunFromEnv(ctx context.Context, runID string) (Run, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Run{}, err
	}
	return ReadRunWithSession(ctx, session, runID)
}

func ListRunsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret []Run, err error) {
	_, repo, err := runDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items, err := repo.List(ctx)
	if err != nil {
		return nil, err
	}
	ret = make([]Run, 0, len(items))
	for _, item := range items {
		if workspaceID != "" && string(item.WorkspaceID) != workspaceID {
			continue
		}
		ret = append(ret, mapRun(item))
	}
	return ret, nil
}

func ListRunsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]Run, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListRunsWithSession(ctx, session, workspaceID)
}

func ListRunsFromEnv(ctx context.Context, workspaceID string) ([]Run, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return ListRunsWithSession(ctx, session, workspaceID)
}

func attachCompressedContextToRun(ctx context.Context, session *RuntimeSession, runID common.RunID) error {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return err
	}
	item, err := deps.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	return attachCompressedContextToLoadedRun(ctx, deps, item)
}

func compressedContextPatchForRun(ctx context.Context, session *RuntimeSession, runID common.RunID) (map[string]any, error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	item, err := deps.runs.Get(ctx, runID)
	if err != nil {
		return nil, err
	}
	compressed, err := compressedContextForLoadedRun(ctx, deps, item)
	if err != nil {
		return nil, err
	}
	return map[string]any{"compressedContext": compressed}, nil
}

func attachCompressedContextToLoadedRun(ctx context.Context, deps readModelDependencies, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	compressed, err := compressedContextForLoadedRun(ctx, deps, item)
	if err != nil {
		return err
	}
	executionContext := cloneContextMap(item.ExecutionContext)
	if executionContext == nil {
		executionContext = map[string]any{}
	}
	executionContext["compressedContext"] = compressed
	item.ExecutionContext = executionContext
	return deps.runs.Update(ctx, item)
}

func compressedContextForLoadedRun(ctx context.Context, deps readModelDependencies, item *runmodel.Run) (map[string]any, error) {
	if item == nil || item.ID == "" {
		return nil, common.ErrInvalidArgument
	}
	compressed := compressedContextForRunStart(item)
	if compressed != nil {
		return compressed, nil
	}
	compressed, err := compressedContextForBlockedRunResume(ctx, deps, item)
	if err != nil {
		return nil, err
	}
	if compressed != nil {
		return compressed, nil
	}
	snapshot, err := readRunRecoverySnapshot(ctx, deps, item.ID)
	if err != nil {
		return nil, err
	}
	return compressedContextMap(compressedContextFromRunSnapshot(snapshot)), nil
}

func compressedContextForRunStart(item *runmodel.Run) map[string]any {
	if item == nil || item.State != common.RunStateCreated || item.ID == "" || item.WorkspaceID == "" {
		return nil
	}
	return compressedContextMap(CompressedContext{
		Version:          "workorch.compressedContext.v1",
		Mode:             "deterministic_recovery_summary",
		WorkspaceID:      string(item.WorkspaceID),
		RunID:            string(item.ID),
		Summary:          "run created with no execution evidence yet",
		CurrentMilestone: "execution_steps",
		Milestones: []ProgressMilestone{
			{ID: "run_created", Title: "Run created", State: "completed", EvidenceRefs: []string{string(item.ID)}},
			{ID: "execution_steps", Title: "Execution steps", State: "pending"},
			{ID: "governance", Title: "Decision and approval governance", State: "pending"},
			{ID: "validation", Title: "Validation", State: "pending"},
			{ID: "formal_result", Title: "Formal result", State: "pending"},
		},
		Recovery: RecoveryPlan{
			Mode:       "continue",
			NextAction: "continue_run",
			RunID:      string(item.ID),
		},
		SourceEvidenceRefs: []string{string(item.ID)},
		RetentionPolicy:    "retain recovery, blockers, milestones, latest evidence summaries, and formal result refs; reload full evidence by source refs when needed",
	})
}

func compressedContextForBlockedRunResume(ctx context.Context, deps readModelDependencies, item *runmodel.Run) (map[string]any, error) {
	if item == nil || item.ID == "" || item.WorkspaceID == "" {
		return nil, nil
	}
	var blocker ProgressBlocker
	recovery := RecoveryPlan{
		Mode:       "continue",
		NextAction: "continue_run",
		RunID:      string(item.ID),
	}
	summary := ""
	switch item.State {
	case common.RunStateBlockedOnDecision:
		if item.BlockingTicketID == "" || deps.tickets == nil || deps.resolutions == nil {
			return nil, nil
		}
		ticket, err := deps.tickets.Get(ctx, item.BlockingTicketID)
		if err != nil {
			return nil, err
		}
		if ticket.State != common.DecisionTicketStateResolved {
			return nil, nil
		}
		resolution, err := deps.resolutions.GetByTicketID(ctx, item.BlockingTicketID)
		if err != nil {
			return nil, err
		}
		if resolution.Type != "provide_missing_info" {
			return nil, nil
		}
		blocker = ProgressBlocker{
			ID:          string(item.BlockingTicketID),
			Kind:        "decision",
			State:       string(common.DecisionTicketStateResolved),
			Reason:      "decision ticket was resolved; resume run execution",
			EvidenceRef: string(item.BlockingTicketID),
		}
		recovery.BlockingTicketID = string(item.BlockingTicketID)
		recovery.Reason = "decision ticket was resolved; resume run execution"
		summary = "run is resuming after a resolved Decision Ticket"
	case common.RunStateBlockedOnApproval:
		if item.BlockingApprovalID == "" || deps.approvals == nil {
			return nil, nil
		}
		approval, err := deps.approvals.Get(ctx, item.BlockingApprovalID)
		if err != nil {
			return nil, err
		}
		if approval.State != common.ApprovalStateApproved {
			return nil, nil
		}
		blocker = ProgressBlocker{
			ID:          string(item.BlockingApprovalID),
			Kind:        "approval",
			State:       string(common.ApprovalStateApproved),
			Reason:      "approval was resolved; resume run execution",
			EvidenceRef: string(item.BlockingApprovalID),
		}
		recovery.BlockingApprovalID = string(item.BlockingApprovalID)
		recovery.Reason = "approval was resolved; resume run execution"
		summary = "run is resuming after an approved Approval"
	default:
		return nil, nil
	}
	return compressedContextMap(CompressedContext{
		Version:          "workorch.compressedContext.v1",
		Mode:             "deterministic_recovery_summary",
		WorkspaceID:      string(item.WorkspaceID),
		RunID:            string(item.ID),
		Summary:          summary,
		CurrentMilestone: "execution_steps",
		Milestones: []ProgressMilestone{
			{ID: "run_created", Title: "Run created", State: "completed", EvidenceRefs: []string{string(item.ID)}},
			{ID: "execution_steps", Title: "Execution steps", State: "in_progress"},
			{ID: "governance", Title: "Decision and approval governance", State: "in_progress", EvidenceRefs: []string{blocker.ID}},
			{ID: "validation", Title: "Validation", State: "pending"},
			{ID: "formal_result", Title: "Formal result", State: "pending"},
		},
		Blockers:           []ProgressBlocker{blocker},
		Recovery:           recovery,
		SourceSectionRefs:  []string{"run", "governance", "task_progress"},
		SourceEvidenceRefs: []string{string(item.ID), blocker.ID},
		RetentionPolicy:    "retain recovery, blockers, milestones, latest evidence summaries, and formal result refs; reload full evidence by source refs when needed",
	}), nil
}

func readRunContextBundle(ctx context.Context, deps readModelDependencies, runID common.RunID) (ContextBundle, error) {
	snapshot, err := readRunSnapshot(ctx, deps, runID)
	if err != nil {
		return ContextBundle{}, err
	}
	return mapRunContextBundle(snapshot), nil
}

func compressedContextMap(input CompressedContext) map[string]any {
	payload, err := json.Marshal(input)
	if err != nil {
		return nil
	}
	var ret map[string]any
	if err := json.Unmarshal(payload, &ret); err != nil {
		return nil
	}
	return ret
}

func runDependenciesFromSession(session *RuntimeSession) (runmodule.Service, repository.RunRepository, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, nil, err
	}
	if exports.Services.RunService == nil || exports.Repositories.Runs == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	return exports.Services.RunService, exports.Repositories.Runs, nil
}

func validateRunDefinitions(ctx context.Context, session *RuntimeSession, workspaceID string, goalID string, dodID string) error {
	if goalID == "" && dodID == "" {
		return nil
	}
	if goalID == "" || dodID == "" {
		return common.ErrInvalidArgument
	}
	exports, err := session.Exports()
	if err != nil {
		return err
	}
	if exports.Repositories.Workspaces == nil || exports.Repositories.Goals == nil || exports.Repositories.DoDs == nil {
		return common.ErrInvalidArgument
	}
	if _, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspaceID)); err != nil {
		return err
	}
	goal, err := exports.Repositories.Goals.Get(ctx, common.GoalID(goalID))
	if err != nil {
		return err
	}
	if goal.WorkspaceID != common.WorkspaceID(workspaceID) {
		return common.ErrConflict
	}
	dod, err := exports.Repositories.DoDs.Get(ctx, common.DoDID(dodID))
	if err != nil {
		return err
	}
	if dod.WorkspaceID != common.WorkspaceID(workspaceID) {
		return common.ErrConflict
	}
	if dod.State != common.DoDStateReady && dod.State != common.DoDStateSatisfied {
		return common.ErrConflict
	}
	return nil
}

func cloneContextMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func mapRun(item *runmodel.Run) Run {
	if item == nil {
		return Run{}
	}
	return Run{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		GoalID:             string(item.GoalID),
		DoDID:              string(item.DoDID),
		InitialStepKind:    item.InitialStepKind,
		ExecutionContext:   cloneContextMap(item.ExecutionContext),
		State:              string(item.State),
		Priority:           item.Priority,
		BlockingTicketID:   string(item.BlockingTicketID),
		BlockingApprovalID: string(item.BlockingApprovalID),
		FailureReason:      item.FailureReason,
	}
}
