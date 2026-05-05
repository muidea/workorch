package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type RuntimeAdapter interface {
	ExecuteRuntimeStep(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error)
}

const (
	stepOutputKeyNextStep              = "nextStep"
	stepOutputKeyNextKind              = "nextKind"
	stepOutputKeyNextInput             = "nextInput"
	stepOutputKeyPatches               = "patches"
	stepOutputKeyMemoryWrites          = "memoryWrites"
	stepOutputKeyMemoryRefs            = "memoryRefs"
	stepOutputKeyDoDAssessment         = "dodAssessment"
	stepOutputKeyLLMDecision           = "llmDecision"
	stepOutputKeyExecutionGraph        = "executionGraph"
	stepOutputKeyGraphBranch           = "graphBranch"
	stepInputKeyGraphNodeID            = "graphNodeId"
	stepInputKeyGraphRootStepID        = "graphRootStepId"
	stepInputKeyGraphPreviousNodeID    = "graphPreviousNodeId"
	stepInputKeyGraphCompletedNodes    = "graphCompletedNodes"
	stepInputKeyGraphBatch             = "graphBatch"
	stepInputKeyGraphEvidence          = "graphEvidence"
	stepInputKeyWorkspaceFileManifest  = "workspaceFileManifest"
	stepInputKeySoulStage              = "soulStage"
	stepInputKeyRuntimeRepairFollowUp  = "runtimeRepairFollowUp"
	stepInputKeyRuntimeRepairAttempt   = "runtimeRepairAttempt"
	stepOutputKeyBlockDecisionReason   = "blockDecisionReason"
	stepOutputKeyBlockDecisionTicketID = "blockDecisionTicketId"
	stepOutputKeyBlockDecisionKind     = "blockDecisionKind"
	stepOutputKeyBlockDecisionQuestion = "blockDecisionQuestion"
	stepOutputKeyBlockDecisionContext  = "blockDecisionContext"
	stepOutputKeyBlockApprovalReason   = "blockApprovalReason"
	stepOutputKeyBlockApprovalID       = "blockApprovalId"
	stepOutputKeyBlockApprovalAction   = "blockApprovalAction"
	stepOutputKeyBlockApprovalContext  = "blockApprovalContext"
	llmWorkspaceExecutionStepKind      = "llm/workspace_execution"
	defaultInitialStepKind             = llmWorkspaceExecutionStepKind
	soulStagePlanning                  = "planning"
	soulStageExecution                 = "execution"
	soulStageDecision                  = "decision"
	soulStageApproval                  = "approval"
	soulStageValidation                = "validation"
	maxRuntimeRepairAttempts           = 3
)

type RuntimeOrchestrator struct {
	runtime     RuntimeAdapter
	runs        repository.RunRepository
	steps       repository.StepRepository
	workspaces  repository.WorkspaceRepository
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
	approvals   repository.ApprovalRepository
	evidence    RuntimeEvidenceRepositories
	eventHub    events.Hub
}

type RuntimeEvidenceRepositories struct {
	Results            repository.ResultRepository
	Artifacts          repository.ArtifactRepository
	ToolCalls          repository.ToolCallRepository
	Patches            repository.PatchRepository
	ValidationResults  repository.ValidationResultRepository
	RuntimeCheckpoints repository.RuntimeCheckpointRepository
	MemoryItems        repository.MemoryItemRepository
	MemoryRecallTraces repository.MemoryRecallTraceRepository
}

type stepDispatchSpec struct {
	ID    common.StepID
	Kind  string
	Input map[string]any
}

func NewRuntimeOrchestrator(
	runtime RuntimeAdapter,
	runRepo repository.RunRepository,
	stepRepo repository.StepRepository,
	workspaceRepo repository.WorkspaceRepository,
	ticketRepo repository.DecisionTicketRepository,
	approvalRepo repository.ApprovalRepository,
	eventHub events.Hub,
) *RuntimeOrchestrator {
	return NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, nil, approvalRepo, eventHub)
}

func NewRuntimeOrchestratorWithResolutions(
	runtime RuntimeAdapter,
	runRepo repository.RunRepository,
	stepRepo repository.StepRepository,
	workspaceRepo repository.WorkspaceRepository,
	ticketRepo repository.DecisionTicketRepository,
	resolutionRepo repository.DecisionResolutionRepository,
	approvalRepo repository.ApprovalRepository,
	eventHub events.Hub,
) *RuntimeOrchestrator {
	return NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{},
		eventHub,
	)
}

func NewRuntimeOrchestratorWithResolutionsAndEvidence(
	runtime RuntimeAdapter,
	runRepo repository.RunRepository,
	stepRepo repository.StepRepository,
	workspaceRepo repository.WorkspaceRepository,
	ticketRepo repository.DecisionTicketRepository,
	resolutionRepo repository.DecisionResolutionRepository,
	approvalRepo repository.ApprovalRepository,
	evidence RuntimeEvidenceRepositories,
	eventHub events.Hub,
) *RuntimeOrchestrator {
	if eventHub == nil {
		eventHub = events.NewMemoryHub()
	}
	return &RuntimeOrchestrator{
		runtime:     runtime,
		runs:        runRepo,
		steps:       stepRepo,
		workspaces:  workspaceRepo,
		tickets:     ticketRepo,
		resolutions: resolutionRepo,
		approvals:   approvalRepo,
		evidence:    evidence,
		eventHub:    eventHub,
	}
}

func (o *RuntimeOrchestrator) StartRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}

	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	if r.State != common.RunStateCreated {
		return common.ErrConflict
	}
	ws, err := o.workspaces.Get(ctx, r.WorkspaceID)
	if err != nil {
		return err
	}
	if ws.ActiveRunID != "" && ws.ActiveRunID != runID {
		return common.ErrConflict
	}

	r.State = common.RunStateRunning
	r.FailureReason = ""
	ws, err = o.syncRunAndWorkspaceForActiveState(ctx, r, common.WorkspaceStateRunning, ws)
	if err != nil {
		return err
	}
	if err := o.publishRunStateChanged(ctx, r); err != nil {
		return err
	}
	if err := o.publishWorkspaceStateChanged(ctx, ws, runID); err != nil {
		return err
	}
	specs, err := o.nextDispatchSpecs(ctx, r)
	if err != nil {
		return err
	}
	return o.dispatchStepSpecsWithWorkspace(ctx, r, specs, ws)
}

func (o *RuntimeOrchestrator) DispatchNextStep(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	if o.runtime == nil {
		return common.ErrInvalidArgument
	}

	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	specs, err := o.nextDispatchSpecs(ctx, r)
	if err != nil {
		return err
	}
	return o.dispatchStepSpecs(ctx, r, specs)
}

func (o *RuntimeOrchestrator) BlockOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	if runID == "" || ticketID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	return o.blockOnDecision(ctx, r, nil, ticketID)
}

func (o *RuntimeOrchestrator) blockOnDecision(ctx context.Context, r *runModel.Run, workspace *workspaceModel.Workspace, ticketID common.TicketID) error {
	return o.blockOnDecisionAtStep(ctx, r, workspace, ticketID, "")
}

func (o *RuntimeOrchestrator) blockOnDecisionAtStep(ctx context.Context, r *runModel.Run, workspace *workspaceModel.Workspace, ticketID common.TicketID, currentStepID common.StepID) error {
	if r == nil || r.ID == "" || ticketID == "" {
		return common.ErrInvalidArgument
	}
	if r.State != common.RunStateCreated && r.State != common.RunStateRunning {
		return common.ErrConflict
	}
	r.State = common.RunStateBlockedOnDecision
	r.BlockingTicketID = ticketID
	r.BlockingApprovalID = ""
	r.FailureReason = ""
	workspace, err := o.syncRunAndWorkspaceForActiveState(ctx, r, common.WorkspaceStateWaitingDecision, workspace)
	if err != nil {
		return err
	}
	if err := o.publishRunStateChangedAtStep(ctx, r, currentStepID); err != nil {
		return err
	}
	return o.publishWorkspaceStateChanged(ctx, workspace, r.ID)
}

func (o *RuntimeOrchestrator) BlockOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	if runID == "" || approvalID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	return o.blockOnApproval(ctx, r, nil, approvalID)
}

func (o *RuntimeOrchestrator) blockOnApproval(ctx context.Context, r *runModel.Run, workspace *workspaceModel.Workspace, approvalID common.ApprovalID) error {
	return o.blockOnApprovalAtStep(ctx, r, workspace, approvalID, "")
}

func (o *RuntimeOrchestrator) blockOnApprovalAtStep(ctx context.Context, r *runModel.Run, workspace *workspaceModel.Workspace, approvalID common.ApprovalID, currentStepID common.StepID) error {
	if r == nil || r.ID == "" || approvalID == "" {
		return common.ErrInvalidArgument
	}
	if r.State != common.RunStateCreated && r.State != common.RunStateRunning {
		return common.ErrConflict
	}
	r.State = common.RunStateBlockedOnApproval
	r.BlockingApprovalID = approvalID
	r.BlockingTicketID = ""
	r.FailureReason = ""
	workspace, err := o.syncRunAndWorkspaceForActiveState(ctx, r, common.WorkspaceStateBlocked, workspace)
	if err != nil {
		return err
	}
	if err := o.publishRunStateChangedAtStep(ctx, r, currentStepID); err != nil {
		return err
	}
	return o.publishWorkspaceStateChanged(ctx, workspace, r.ID)
}

func (o *RuntimeOrchestrator) ResumeRun(ctx context.Context, runID common.RunID) error {
	return o.ResumeRunWithExecutionContextPatch(ctx, runID, nil)
}

func (o *RuntimeOrchestrator) ResumeRunWithExecutionContextPatch(ctx context.Context, runID common.RunID, executionContextPatch map[string]any) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	return o.ResumeLoadedRunWithExecutionContextPatch(ctx, r, executionContextPatch)
}

func (o *RuntimeOrchestrator) ResumeLoadedRunWithExecutionContextPatch(ctx context.Context, r *runModel.Run, executionContextPatch map[string]any) error {
	if r == nil || r.ID == "" {
		return common.ErrInvalidArgument
	}
	var resumeAfterStepID common.StepID
	var resumeInput map[string]any
	switch r.State {
	case common.RunStateBlockedOnDecision:
		if r.BlockingTicketID == "" || o.tickets == nil {
			return common.ErrConflict
		}
		ticket, err := o.tickets.Get(ctx, r.BlockingTicketID)
		if err != nil {
			return err
		}
		if ticket.State != common.DecisionTicketStateResolved {
			return common.ErrBlocked
		}
		resumeAfterStepID = ticket.StepID
		if o.resolutions != nil {
			resolution, err := o.resolutions.GetByTicketID(ctx, r.BlockingTicketID)
			if err != nil {
				return err
			}
			switch resolution.Type {
			case decisioncommon.ResolutionTypeProvideMissingInfo:
				resumeInput = decisionResolutionInput(ticket, resolution)
				r.BlockingTicketID = ""
			case decisioncommon.ResolutionTypeRejectCurrentPath:
				mergeRunExecutionContextPatch(r, executionContextPatch)
				return o.failLoadedRun(ctx, r, fmt.Sprintf("decision ticket %s rejected", ticket.ID))
			case decisioncommon.ResolutionTypeReplan:
				mergeRunExecutionContextPatch(r, executionContextPatch)
				return o.supersedeLoadedRun(ctx, r)
			default:
				return common.ErrInvalidArgument
			}
		} else {
			r.BlockingTicketID = ""
		}
	case common.RunStateBlockedOnApproval:
		if r.BlockingApprovalID == "" || o.approvals == nil {
			return common.ErrConflict
		}
		approval, err := o.approvals.Get(ctx, r.BlockingApprovalID)
		if err != nil {
			return err
		}
		switch approval.State {
		case common.ApprovalStateApproved:
			resumeAfterStepID = approval.StepID
			resumeInput = approvalResolutionInput(approval)
			r.BlockingApprovalID = ""
		case common.ApprovalStateRejected:
			mergeRunExecutionContextPatch(r, executionContextPatch)
			return o.failLoadedRun(ctx, r, fmt.Sprintf("approval %s rejected", approval.ID))
		default:
			return common.ErrBlocked
		}
	case common.RunStateRunning:
		recoverStepID, recoverInput, err := o.runningDecisionRecoveryContext(ctx, r)
		if err != nil {
			return err
		}
		if recoverStepID == "" {
			return nil
		}
		resumeAfterStepID = recoverStepID
		resumeInput = recoverInput
	default:
		return common.ErrConflict
	}

	r.State = common.RunStateRunning
	r.FailureReason = ""
	mergeRunExecutionContextPatch(r, executionContextPatch)
	workspace, err := o.syncRunAndWorkspaceForActiveState(ctx, r, common.WorkspaceStateRunning, nil)
	if err != nil {
		return err
	}
	if err := o.publishRunStateChanged(ctx, r); err != nil {
		return err
	}
	if err := o.publishWorkspaceStateChanged(ctx, workspace, r.ID); err != nil {
		return err
	}
	specs, err := o.nextDispatchSpecsAfterStep(ctx, r, resumeAfterStepID)
	if err != nil {
		return o.failRunningResume(ctx, r.ID, err)
	}
	mergeResumeInput(specs, resumeInput)
	if err := o.dispatchStepSpecsWithWorkspace(ctx, r, specs, workspace); err != nil {
		return o.failRunningResume(ctx, r.ID, err)
	}
	return nil
}

func (o *RuntimeOrchestrator) failRunningResume(ctx context.Context, runID common.RunID, cause error) error {
	if cause == nil {
		return nil
	}
	current, getErr := o.runs.Get(ctx, runID)
	if getErr != nil {
		return errors.Join(cause, getErr)
	}
	if current.State != common.RunStateRunning {
		return cause
	}
	if err := o.FailRun(ctx, runID, cause.Error()); err != nil {
		return errors.Join(cause, err)
	}
	return nil
}

func (o *RuntimeOrchestrator) runningDecisionRecoveryContext(
	ctx context.Context,
	run *runModel.Run,
) (common.StepID, map[string]any, error) {
	if o == nil || run == nil || o.steps == nil || o.tickets == nil || o.resolutions == nil {
		return "", nil, common.ErrConflict
	}
	if run.BlockingTicketID != "" || run.BlockingApprovalID != "" {
		return "", nil, common.ErrConflict
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return "", nil, err
	}
	last := terminalExecutionStep(steps)
	if last == nil || last.State != common.StepStateFailed {
		return "", nil, common.ErrConflict
	}
	ticket, resolution, err := o.resolvedDecisionForStep(ctx, run.ID, last.ID)
	if err != nil {
		return "", nil, err
	}
	switch resolution.Type {
	case decisioncommon.ResolutionTypeProvideMissingInfo:
		return last.ID, decisionResolutionInput(ticket, resolution), nil
	case decisioncommon.ResolutionTypeRejectCurrentPath:
		return "", nil, o.FailRun(ctx, run.ID, fmt.Sprintf("decision ticket %s rejected", ticket.ID))
	case decisioncommon.ResolutionTypeReplan:
		return "", nil, o.SupersedeRun(ctx, run.ID)
	default:
		return "", nil, common.ErrInvalidArgument
	}
}

func (o *RuntimeOrchestrator) resolvedDecisionForStep(
	ctx context.Context,
	runID common.RunID,
	stepID common.StepID,
) (*decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	tickets, err := o.tickets.ListByRunID(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	for _, ticket := range tickets {
		if ticket == nil ||
			ticket.StepID != stepID ||
			ticket.State != common.DecisionTicketStateResolved {
			continue
		}
		resolution, err := o.resolutions.GetByTicketID(ctx, ticket.ID)
		if err != nil {
			return nil, nil, err
		}
		return ticket, resolution, nil
	}
	return nil, nil, common.ErrConflict
}

func (o *RuntimeOrchestrator) CompleteRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	if r.State != common.RunStateRunning && r.State != common.RunStateValidating {
		return common.ErrConflict
	}
	r.State = common.RunStateCompleted
	r.BlockingTicketID = ""
	r.BlockingApprovalID = ""
	r.FailureReason = ""
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}
	if err := o.publishRunStateChanged(ctx, r); err != nil {
		return err
	}
	return o.releaseWorkspaceForTerminalRun(ctx, r, common.WorkspaceStateCompleted)
}

func (o *RuntimeOrchestrator) ValidateRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	if r.State != common.RunStateRunning {
		return common.ErrConflict
	}
	r.State = common.RunStateValidating
	r.FailureReason = ""
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}
	if err := o.publishRunStateChanged(ctx, r); err != nil {
		return err
	}
	return o.syncWorkspaceForActiveRun(ctx, r, common.WorkspaceStateValidating)
}

func (o *RuntimeOrchestrator) validateAndCompleteRun(ctx context.Context, run *runModel.Run, currentStepID common.StepID) error {
	return o.validateAndCompleteRunWithWorkspace(ctx, run, currentStepID, nil)
}

func (o *RuntimeOrchestrator) validateAndCompleteRunWithWorkspace(ctx context.Context, run *runModel.Run, currentStepID common.StepID, workspace *workspaceModel.Workspace) error {
	if run == nil || run.ID == "" {
		return common.ErrInvalidArgument
	}
	if run.State != common.RunStateRunning {
		return common.ErrConflict
	}
	var err error
	workspace, err = o.syncRunAndWorkspaceForValidation(ctx, run, workspace)
	if err != nil {
		return err
	}
	if err := o.publishRunStateChangedAtStep(ctx, run, currentStepID); err != nil {
		return err
	}
	if err := o.publishWorkspaceStateChanged(ctx, workspace, run.ID); err != nil {
		return err
	}

	run.State = common.RunStateCompleted
	run.BlockingTicketID = ""
	run.BlockingApprovalID = ""
	run.FailureReason = ""
	workspace.ActiveRunID = ""
	workspace.State = common.WorkspaceStateCompleted
	if err := runParallel(
		func() error {
			return o.runs.Update(ctx, run)
		},
		func() error {
			return o.workspaces.Update(ctx, workspace)
		},
	); err != nil {
		return err
	}
	if err := o.publishRunStateChangedAtStep(ctx, run, currentStepID); err != nil {
		return err
	}
	return o.publishWorkspaceStateChanged(ctx, workspace, run.ID)
}

func (o *RuntimeOrchestrator) syncRunAndWorkspaceForValidation(ctx context.Context, run *runModel.Run, workspace *workspaceModel.Workspace) (*workspaceModel.Workspace, error) {
	if run == nil {
		return nil, common.ErrInvalidArgument
	}
	ws := workspace
	if ws == nil {
		var err error
		ws, err = o.workspaces.Get(ctx, run.WorkspaceID)
		if err != nil {
			return nil, err
		}
	} else if ws.ID != run.WorkspaceID {
		return nil, common.ErrConflict
	}
	if ws.ActiveRunID != "" && ws.ActiveRunID != run.ID {
		activeRun, err := o.runs.Get(ctx, ws.ActiveRunID)
		if err != nil && !errors.Is(err, common.ErrNotFound) {
			return nil, err
		}
		if activeRun != nil && activeRun.WorkspaceID == ws.ID && !isTerminalRunState(activeRun.State) {
			return nil, common.ErrConflict
		}
	}
	run.State = common.RunStateValidating
	run.FailureReason = ""
	ws.ActiveRunID = run.ID
	ws.State = common.WorkspaceStateValidating
	if err := runParallel(
		func() error {
			return o.runs.Update(ctx, run)
		},
		func() error {
			return o.workspaces.Update(ctx, ws)
		},
	); err != nil {
		return nil, err
	}
	return ws, nil
}

func (o *RuntimeOrchestrator) CancelRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	return o.closeRun(ctx, runID, common.RunStateCancelled, "")
}

func (o *RuntimeOrchestrator) SupersedeRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	return o.closeRun(ctx, runID, common.RunStateSuperseded, "")
}

func (o *RuntimeOrchestrator) FailRun(ctx context.Context, runID common.RunID, reason string) error {
	if runID == "" || reason == "" {
		return common.ErrInvalidArgument
	}
	return o.closeRun(ctx, runID, common.RunStateFailed, reason)
}

func (o *RuntimeOrchestrator) failLoadedRun(ctx context.Context, run *runModel.Run, reason string) error {
	if reason == "" {
		return common.ErrInvalidArgument
	}
	return o.closeLoadedRun(ctx, run, common.RunStateFailed, reason)
}

func (o *RuntimeOrchestrator) supersedeLoadedRun(ctx context.Context, run *runModel.Run) error {
	return o.closeLoadedRun(ctx, run, common.RunStateSuperseded, "")
}

func (o *RuntimeOrchestrator) closeRun(ctx context.Context, runID common.RunID, state common.RunState, reason string) error {
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	return o.closeLoadedRun(ctx, r, state, reason)
}

func (o *RuntimeOrchestrator) closeLoadedRun(ctx context.Context, r *runModel.Run, state common.RunState, reason string) error {
	if r == nil || r.ID == "" {
		return common.ErrInvalidArgument
	}
	if !canCloseRunAs(r.State, state) {
		return common.ErrConflict
	}
	r.State = state
	r.BlockingTicketID = ""
	r.BlockingApprovalID = ""
	r.FailureReason = reason
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}
	if err := o.publishRunStateChanged(ctx, r); err != nil {
		return err
	}

	return o.releaseWorkspaceForTerminalRun(ctx, r, workspaceStateForTerminalRun(state))
}

func mergeRunExecutionContextPatch(run *runModel.Run, patch map[string]any) {
	if run == nil || len(patch) == 0 {
		return
	}
	if run.ExecutionContext == nil {
		run.ExecutionContext = map[string]any{}
	}
	for key, value := range patch {
		if key == "" {
			continue
		}
		run.ExecutionContext[key] = value
	}
}

func (o *RuntimeOrchestrator) releaseWorkspaceForTerminalRun(ctx context.Context, run *runModel.Run, state common.WorkspaceState) error {
	if run == nil {
		return common.ErrInvalidArgument
	}
	ws, err := o.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil {
		return err
	}
	canRelease, err := o.canTerminalRunReleaseWorkspace(ctx, ws, run)
	if err != nil {
		return err
	}
	if !canRelease {
		return nil
	}
	ws.ActiveRunID = ""
	ws.State = state
	if err := o.workspaces.Update(ctx, ws); err != nil {
		return err
	}
	return o.publishWorkspaceStateChanged(ctx, ws, run.ID)
}

func (o *RuntimeOrchestrator) syncWorkspaceForActiveRun(ctx context.Context, run *runModel.Run, state common.WorkspaceState) error {
	ws, err := o.syncWorkspaceForActiveRunState(ctx, run, state)
	if err != nil {
		return err
	}
	return o.publishWorkspaceStateChanged(ctx, ws, run.ID)
}

func (o *RuntimeOrchestrator) syncWorkspaceForActiveRunWithWorkspace(ctx context.Context, run *runModel.Run, state common.WorkspaceState, workspace *workspaceModel.Workspace) error {
	ws, err := o.syncWorkspaceForActiveRunStateWithWorkspace(ctx, run, state, workspace)
	if err != nil {
		return err
	}
	return o.publishWorkspaceStateChanged(ctx, ws, run.ID)
}

func (o *RuntimeOrchestrator) syncWorkspaceForActiveRunState(ctx context.Context, run *runModel.Run, state common.WorkspaceState) (*workspaceModel.Workspace, error) {
	return o.syncWorkspaceForActiveRunStateWithWorkspace(ctx, run, state, nil)
}

func (o *RuntimeOrchestrator) syncWorkspaceForActiveRunStateWithWorkspace(ctx context.Context, run *runModel.Run, state common.WorkspaceState, workspace *workspaceModel.Workspace) (*workspaceModel.Workspace, error) {
	if run == nil {
		return nil, common.ErrInvalidArgument
	}
	ws := workspace
	if ws == nil {
		var err error
		ws, err = o.workspaces.Get(ctx, run.WorkspaceID)
		if err != nil {
			return nil, err
		}
	} else if ws.ID != run.WorkspaceID {
		return nil, common.ErrConflict
	}
	if ws.ActiveRunID != "" && ws.ActiveRunID != run.ID {
		activeRun, err := o.runs.Get(ctx, ws.ActiveRunID)
		if err != nil && !errors.Is(err, common.ErrNotFound) {
			return nil, err
		}
		if activeRun != nil && activeRun.WorkspaceID == ws.ID && !isTerminalRunState(activeRun.State) {
			return nil, common.ErrConflict
		}
	}
	ws.ActiveRunID = run.ID
	ws.State = state
	if err := o.workspaces.Update(ctx, ws); err != nil {
		return nil, err
	}
	return ws, nil
}

func (o *RuntimeOrchestrator) syncRunAndWorkspaceForActiveState(ctx context.Context, run *runModel.Run, state common.WorkspaceState, workspace *workspaceModel.Workspace) (*workspaceModel.Workspace, error) {
	if run == nil {
		return nil, common.ErrInvalidArgument
	}
	ws := workspace
	if ws == nil {
		var err error
		ws, err = o.workspaces.Get(ctx, run.WorkspaceID)
		if err != nil {
			return nil, err
		}
	} else if ws.ID != run.WorkspaceID {
		return nil, common.ErrConflict
	}
	if ws.ActiveRunID != "" && ws.ActiveRunID != run.ID {
		activeRun, err := o.runs.Get(ctx, ws.ActiveRunID)
		if err != nil && !errors.Is(err, common.ErrNotFound) {
			return nil, err
		}
		if activeRun != nil && activeRun.WorkspaceID == ws.ID && !isTerminalRunState(activeRun.State) {
			return nil, common.ErrConflict
		}
	}
	ws.ActiveRunID = run.ID
	ws.State = state
	if err := runParallel(
		func() error {
			return o.runs.Update(ctx, run)
		},
		func() error {
			return o.workspaces.Update(ctx, ws)
		},
	); err != nil {
		return nil, err
	}
	return ws, nil
}

func (o *RuntimeOrchestrator) canTerminalRunReleaseWorkspace(ctx context.Context, workspace *workspaceModel.Workspace, run *runModel.Run) (bool, error) {
	if workspace == nil || run == nil || workspace.ID != run.WorkspaceID {
		return false, nil
	}
	if workspace.ActiveRunID == "" || workspace.ActiveRunID == run.ID {
		return true, nil
	}
	activeRun, err := o.runs.Get(ctx, workspace.ActiveRunID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return true, nil
		}
		return false, err
	}
	if activeRun.WorkspaceID != workspace.ID {
		return true, nil
	}
	return isTerminalRunState(activeRun.State), nil
}

func workspaceStateForTerminalRun(state common.RunState) common.WorkspaceState {
	switch state {
	case common.RunStateCompleted:
		return common.WorkspaceStateCompleted
	case common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return common.WorkspaceStateFailed
	default:
		return common.WorkspaceStateActive
	}
}

func isTerminalRunState(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func runParallel(tasks ...func() error) error {
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var ret error
	recordErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		defer errMu.Unlock()
		if ret == nil {
			ret = err
		}
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		wg.Add(1)
		go func(task func() error) {
			defer wg.Done()
			recordErr(task())
		}(task)
	}
	wg.Wait()
	return ret
}

func canCloseRunAs(current common.RunState, target common.RunState) bool {
	switch target {
	case common.RunStateFailed:
		switch current {
		case common.RunStateCreated, common.RunStateRunning, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval, common.RunStateValidating:
			return true
		}
	case common.RunStateCancelled:
		switch current {
		case common.RunStateCreated, common.RunStateRunning, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval, common.RunStateValidating:
			return true
		}
	case common.RunStateSuperseded:
		switch current {
		case common.RunStateRunning, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval, common.RunStateValidating:
			return true
		}
	}
	return false
}

func (o *RuntimeOrchestrator) dispatchStep(
	ctx context.Context,
	run *runModel.Run,
	stepID common.StepID,
	kind string,
	input map[string]any,
) error {
	return o.dispatchStepWithWorkspace(ctx, run, stepID, kind, input, nil)
}

func (o *RuntimeOrchestrator) dispatchStepWithWorkspace(
	ctx context.Context,
	run *runModel.Run,
	stepID common.StepID,
	kind string,
	input map[string]any,
	workspace *workspaceModel.Workspace,
) error {
	step, err := o.steps.Get(ctx, stepID)
	if err != nil {
		if err != common.ErrNotFound {
			return err
		}
		step = &runModel.Step{
			ID:    stepID,
			RunID: run.ID,
			Kind:  kind,
			State: common.StepStateDispatched,
			Input: cloneStepPayload(input),
		}
		if err := o.steps.Create(ctx, step); err != nil {
			return err
		}
	} else {
		step.State = common.StepStateDispatched
		if kind != "" {
			step.Kind = kind
		}
		if input != nil {
			step.Input = cloneStepPayload(input)
		} else if step.Input == nil {
			step.Input = map[string]any{}
		}
		if err := o.steps.Update(ctx, step); err != nil {
			return err
		}
	}
	if workspace == nil {
		workspace, err = o.workspaces.Get(ctx, run.WorkspaceID)
		if err != nil {
			return err
		}
	} else if workspace.ID != run.WorkspaceID {
		return common.ErrConflict
	}
	o.enrichLLMStepInputContext(ctx, run, step, workspace)
	if err := o.publishStepState(ctx, run, step, events.TopicStepDispatched); err != nil {
		return err
	}
	toolCall, err := o.recordStepDispatchEvidence(ctx, run, step)
	if err != nil {
		return err
	}

	result, err := o.runtime.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		WorkspaceID:               run.WorkspaceID,
		WorkspaceWorkingDirectory: workspace.WorkingDirectory,
		RunID:                     run.ID,
		StepID:                    step.ID,
		Kind:                      step.Kind,
		Input:                     cloneStepPayload(step.Input),
	})
	if err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, result.Output, "runtime step failed", err)
	}
	output := result.Output
	if err := o.applyRuntimePatches(ctx, run, step, toolCall, output, workspace.WorkingDirectory); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch application failed", err)
	}
	memoryRefs, err := o.recordMemoryWrites(ctx, run, step, output)
	if err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime memory write failed", err)
	}
	if len(memoryRefs) > 0 {
		output[stepOutputKeyMemoryRefs] = memoryRefs
	}

	step.State = common.StepStateCompleted
	step.Output = output
	if err := o.recordCompletedRuntimeStepEvidence(ctx, run, step, toolCall, output); err != nil {
		return err
	}
	if err := o.persistCompletedStepState(ctx, run, step); err != nil {
		return err
	}
	if err := o.publishStepState(ctx, run, step, events.TopicStepStateChanged); err != nil {
		return err
	}

	nextStepID, nextKind := nextStepSpec(output)
	if decisionSpec := decisionBlockSpec(output); decisionSpec.Reason != "" {
		if nextStepID == "" && !o.hasGraphContinuation(ctx, run, step) {
			return o.FailRun(ctx, run.ID, fmt.Sprintf("runtime step %s blocked without next step", step.ID))
		}
		ticketID := decisionSpec.TicketID
		if ticketID == "" {
			ticketID = common.TicketID(runtimeChildID(step.ID, "decision"))
		}
		decisionSpec.TicketID = ticketID
		if err := o.createDecisionTicket(ctx, run, step, decisionSpec); err != nil {
			return err
		}
		return o.blockOnDecisionAtStep(ctx, run, workspace, ticketID, step.ID)
	}
	if approvalSpec := approvalBlockSpec(output); approvalSpec.Reason != "" {
		if nextStepID == "" && !o.hasGraphContinuation(ctx, run, step) {
			return o.FailRun(ctx, run.ID, fmt.Sprintf("runtime step %s blocked without next step", step.ID))
		}
		if approvalSpec.ApprovalID == "" {
			approvalSpec.ApprovalID = common.ApprovalID(runtimeChildID(step.ID, "approval"))
		}
		if err := o.createApproval(ctx, run, step, approvalSpec.ApprovalID, approvalSpec.Reason, approvalSpec.Action, approvalSpec.Context); err != nil {
			return err
		}
		return o.blockOnApprovalAtStep(ctx, run, workspace, approvalSpec.ApprovalID, step.ID)
	}
	if graphBatchStep(step) {
		return nil
	}
	if nextStepID != "" {
		return o.dispatchStepWithWorkspace(ctx, run, nextStepID, nextKind, nextStepInputFromOutput(run, step), workspace)
	}
	graphSpecs, graphSteps := o.readyGraphStepSpecsWithSteps(ctx, run, step)
	if len(graphSpecs) > 0 {
		return o.dispatchGraphStepBatchWithWorkspace(ctx, run, graphSpecs, workspace)
	}
	if shouldDispatchGraphFollowUp(run, step, output) {
		var input map[string]any
		if len(graphSteps) > 0 {
			input = o.graphFollowUpStepInputFromSteps(run, step, graphSteps)
		} else {
			input = o.graphFollowUpStepInput(ctx, run, step)
		}
		return o.dispatchStepWithWorkspace(ctx, run, graphFollowUpStepID(run.ID, step), initialStepKind(run), input, workspace)
	}
	if shouldDispatchLLMCompletionFollowUp(run, step, output) {
		return o.dispatchStepWithWorkspace(ctx, run, llmCompletionFollowUpStepID(run.ID, step), initialStepKind(run), llmCompletionFollowUpStepInput(run, step), workspace)
	}
	assessment, summary, err := validatedCompletionDoDAssessment(run, output)
	if err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime completion gate failed", err)
	}
	if err := o.recordCompletionEvidenceFromValidatedDoDAssessment(ctx, run, step, assessment, summary, workspace); err != nil {
		return err
	}
	return o.validateAndCompleteRunWithWorkspace(ctx, run, step.ID, workspace)
}

func (o *RuntimeOrchestrator) nextDispatchSpec(
	ctx context.Context,
	run *runModel.Run,
) (common.StepID, string, map[string]any, error) {
	specs, err := o.nextDispatchSpecs(ctx, run)
	if err != nil {
		return "", "", nil, err
	}
	if len(specs) == 0 {
		return "", "", nil, common.ErrConflict
	}
	return specs[0].ID, specs[0].Kind, specs[0].Input, nil
}

func (o *RuntimeOrchestrator) nextDispatchSpecs(
	ctx context.Context,
	run *runModel.Run,
) ([]stepDispatchSpec, error) {
	if run == nil {
		return nil, common.ErrInvalidArgument
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	if len(steps) == 0 {
		return []stepDispatchSpec{{
			ID:    common.StepID(fmt.Sprintf("%s/step-1", run.ID)),
			Kind:  initialStepKind(run),
			Input: initialStepInput(run),
		}}, nil
	}

	last := terminalExecutionStep(steps)
	if last == nil {
		return nil, common.ErrConflict
	}
	return o.nextDispatchSpecsFromStep(ctx, run, last)
}

func (o *RuntimeOrchestrator) nextDispatchSpecsAfterStep(
	ctx context.Context,
	run *runModel.Run,
	stepID common.StepID,
) ([]stepDispatchSpec, error) {
	if stepID == "" {
		return o.nextDispatchSpecs(ctx, run)
	}
	if run == nil {
		return nil, common.ErrInvalidArgument
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	for _, step := range steps {
		if step != nil && step.ID == stepID {
			return o.nextDispatchSpecsFromStep(ctx, run, step)
		}
	}
	return nil, common.ErrConflict
}

func (o *RuntimeOrchestrator) nextDispatchSpecsFromStep(
	ctx context.Context,
	run *runModel.Run,
	last *runModel.Step,
) ([]stepDispatchSpec, error) {
	if run == nil || last == nil {
		return nil, common.ErrInvalidArgument
	}
	if last.State == common.StepStateCompleted {
		specs, steps := o.readyGraphStepSpecsWithSteps(ctx, run, last)
		if len(specs) > 0 {
			return specs, nil
		}
		nextStepID, nextKind := nextStepSpec(last.Output)
		if nextStepID != "" {
			uniqueStepID, err := o.uniqueDispatchStepID(ctx, run.ID, nextStepID)
			if err != nil {
				return nil, err
			}
			return []stepDispatchSpec{{
				ID:    uniqueStepID,
				Kind:  nextKind,
				Input: nextStepInputFromOutput(run, last),
			}}, nil
		}
		if shouldDispatchGraphFollowUp(run, last, last.Output) {
			var input map[string]any
			if len(steps) > 0 {
				input = o.graphFollowUpStepInputFromSteps(run, last, steps)
			} else {
				input = o.graphFollowUpStepInput(ctx, run, last)
			}
			return []stepDispatchSpec{{
				ID:    graphFollowUpStepID(run.ID, last),
				Kind:  initialStepKind(run),
				Input: input,
			}}, nil
		}
		if shouldDispatchLLMCompletionFollowUp(run, last, last.Output) {
			return []stepDispatchSpec{{
				ID:    llmCompletionFollowUpStepID(run.ID, last),
				Kind:  initialStepKind(run),
				Input: llmCompletionFollowUpStepInput(run, last),
			}}, nil
		}
	}
	if last.State == common.StepStateDispatched {
		return []stepDispatchSpec{{
			ID:    last.ID,
			Kind:  last.Kind,
			Input: cloneStepPayload(last.Input),
		}}, nil
	}
	if last.State == common.StepStateFailed && requiresDoDAssessmentCompletionGate(run) {
		attempt := o.runtimeRepairAttemptCount(ctx, run.ID) + 1
		stepID, err := o.uniqueDispatchStepID(ctx, run.ID, runtimeRepairFollowUpStepID(run.ID, last, attempt))
		if err != nil {
			return nil, err
		}
		return []stepDispatchSpec{{
			ID:    stepID,
			Kind:  initialStepKind(run),
			Input: o.runtimeRepairFollowUpStepInput(ctx, run, last, attempt, nil),
		}}, nil
	}
	return nil, common.ErrConflict
}

func (o *RuntimeOrchestrator) uniqueDispatchStepID(ctx context.Context, runID common.RunID, base common.StepID) (common.StepID, error) {
	if o == nil || o.steps == nil || runID == "" || base == "" {
		return base, nil
	}
	existing, err := o.steps.Get(ctx, base)
	if errors.Is(err, common.ErrNotFound) {
		return base, nil
	}
	if err != nil {
		return "", err
	}
	if existing != nil && existing.RunID != runID {
		return "", common.ErrConflict
	}
	for idx := 2; ; idx++ {
		candidate := common.StepID(fmt.Sprintf("%s-%d", base, idx))
		existing, err := o.steps.Get(ctx, candidate)
		if errors.Is(err, common.ErrNotFound) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
		if existing != nil && existing.RunID != runID {
			return "", common.ErrConflict
		}
	}
}

func terminalExecutionStep(steps []*runModel.Step) *runModel.Step {
	if len(steps) == 0 {
		return nil
	}
	referenced := map[common.StepID]bool{}
	for _, step := range steps {
		if previousStepID := previousExecutionStepID(step); previousStepID != "" {
			referenced[previousStepID] = true
		}
	}
	var terminals []*runModel.Step
	for _, step := range steps {
		if step == nil || referenced[step.ID] {
			continue
		}
		terminals = append(terminals, step)
	}
	if len(terminals) == 0 {
		return nil
	}
	if len(terminals) == 1 {
		return terminals[0]
	}
	for _, step := range terminals {
		if step.State == common.StepStateDispatched {
			return step
		}
	}
	if graphStep := latestCompletedGraphStep(terminals); graphStep != nil {
		return graphStep
	}
	return terminals[len(terminals)-1]
}

func previousExecutionStepID(step *runModel.Step) common.StepID {
	if step == nil || step.Input == nil {
		return ""
	}
	previousStepID, _ := step.Input["previousStepId"].(string)
	return common.StepID(strings.TrimSpace(previousStepID))
}

func (o *RuntimeOrchestrator) dispatchStepSpecs(ctx context.Context, run *runModel.Run, specs []stepDispatchSpec) error {
	return o.dispatchStepSpecsWithWorkspace(ctx, run, specs, nil)
}

func (o *RuntimeOrchestrator) dispatchStepSpecsWithWorkspace(ctx context.Context, run *runModel.Run, specs []stepDispatchSpec, workspace *workspaceModel.Workspace) error {
	if len(specs) == 0 {
		return common.ErrConflict
	}
	if len(specs) == 1 {
		return o.dispatchStepWithWorkspace(ctx, run, specs[0].ID, specs[0].Kind, specs[0].Input, workspace)
	}
	return o.dispatchGraphStepBatchWithWorkspace(ctx, run, specs, workspace)
}

func mergeResumeInput(specs []stepDispatchSpec, resumeInput map[string]any) {
	if len(resumeInput) == 0 {
		return
	}
	for idx := range specs {
		if specs[idx].Input == nil {
			specs[idx].Input = map[string]any{}
		}
		for key, value := range resumeInput {
			specs[idx].Input[key] = value
		}
	}
}
