package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	memorycommon "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
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

func (o *RuntimeOrchestrator) publishRunStateChanged(ctx context.Context, run *runModel.Run) error {
	return o.publishRunStateChangedAtStep(ctx, run, "")
}

func (o *RuntimeOrchestrator) publishRunStateChangedAtStep(ctx context.Context, run *runModel.Run, currentStepID common.StepID) error {
	if err := o.recordRunCheckpointAtStep(ctx, run, currentStepID); err != nil {
		return err
	}
	if o.eventHub == nil || run == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicRunStateChanged,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "run",
		SubjectID:    string(run.ID),
		WorkspaceID:  string(run.WorkspaceID),
		RunID:        string(run.ID),
		Payload: events.RunPayload(
			run.GoalID,
			run.DoDID,
			run.State,
			run.BlockingTicketID,
			run.BlockingApprovalID,
			run.FailureReason,
		),
	})
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

type runtimePatchRequest struct {
	TargetRef string
	Content   string
	Diff      string
	Summary   string
	PatchType string
}

type runtimeFailureDiagnosticError interface {
	RuntimeFailureDiagnostics() map[string]any
}

type workspacePathConflictError struct {
	Operation    string
	TargetRef    string
	ConflictRef  string
	ConflictKind string
}

func (e *workspacePathConflictError) Error() string {
	if e == nil {
		return ""
	}
	operation := strings.TrimSpace(e.Operation)
	if operation == "" {
		operation = "workspace patch"
	}
	conflictKind := strings.TrimSpace(e.ConflictKind)
	if conflictKind == "" {
		conflictKind = "file"
	}
	return fmt.Sprintf("%s path conflict: target %q requires %q as a directory, but it is an existing %s", operation, e.TargetRef, e.ConflictRef, conflictKind)
}

func (e *workspacePathConflictError) RuntimeFailureDiagnostics() map[string]any {
	if e == nil {
		return nil
	}
	return map[string]any{
		"kind":         "workspace_path_conflict",
		"operation":    e.Operation,
		"targetRef":    e.TargetRef,
		"conflictRef":  e.ConflictRef,
		"conflictKind": e.ConflictKind,
		"guidance":     "Choose a targetRef whose parent path is a directory, or write the file at the existing project root. Do not treat an existing file as a directory.",
	}
}

func (o *RuntimeOrchestrator) applyRuntimePatches(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	output map[string]any,
	workingDirectory string,
) error {
	requests, err := runtimePatchRequests(output)
	if err != nil {
		return err
	}
	if len(requests) == 0 {
		return nil
	}
	if o.evidence.Patches == nil {
		return common.ErrInvalidArgument
	}
	for idx, request := range requests {
		patchID := common.PatchID(fmt.Sprintf("%s/patch/%d", step.ID, idx+1))
		patch := &runModel.Patch{
			ID:          patchID,
			WorkspaceID: run.WorkspaceID,
			RunID:       run.ID,
			StepID:      step.ID,
			ToolCallID:  toolCall.ID,
			PatchType:   request.PatchType,
			TargetRef:   request.TargetRef,
			Content:     request.Content,
			Diff:        request.Diff,
			Summary:     request.Summary,
			State:       common.PatchStateCreated,
			CreatedAt:   runtimeEvidenceTimestamp(),
		}
		if err := o.evidence.Patches.Create(ctx, patch); err != nil {
			if err != common.ErrConflict {
				return err
			}
			if err := o.evidence.Patches.Update(ctx, patch); err != nil {
				return err
			}
		}
		if err := applyWorkspacePatch(workingDirectory, request); err != nil {
			patch.State = common.PatchStateFailed
			_ = o.evidence.Patches.Update(ctx, patch)
			return err
		}
		patch.State = common.PatchStateApplied
		patch.AppliedAt = runtimeEvidenceTimestamp()
		if err := o.evidence.Patches.Update(ctx, patch); err != nil {
			return err
		}
	}
	return nil
}

type runtimeMemoryWriteRequest struct {
	MemoryType string
	Summary    string
	Content    string
	Tags       []string
}

type resultEvidenceDetail struct {
	Ref      string
	Kind     string
	Command  string
	ExitCode string
	Stdout   string
	Stderr   string
	Summary  string
}

type resultPatchDetail struct {
	ID        common.PatchID
	PatchType string
	TargetRef string
	Summary   string
	State     common.PatchState
	Content   string
	Diff      string
}

func (o *RuntimeOrchestrator) recordMemoryWrites(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	output map[string]any,
) ([]string, error) {
	requests, err := runtimeMemoryWriteRequests(output)
	if err != nil {
		return nil, err
	}
	if len(requests) == 0 {
		return nil, nil
	}
	if o.evidence.MemoryItems == nil {
		return nil, common.ErrInvalidArgument
	}
	refs := make([]string, len(requests))
	tasks := make([]func() error, 0, len(requests))
	for idx, request := range requests {
		memoryID := common.MemoryID(fmt.Sprintf("%s/memory/%d", step.ID, idx+1))
		item := &memorymodel.MemoryItem{
			ID:          memoryID,
			WorkspaceID: run.WorkspaceID,
			RunID:       run.ID,
			SourceType:  memorycommon.MemorySourceRun,
			SourceRef:   string(run.ID),
			MemoryType:  request.MemoryType,
			Content:     request.Content,
			Summary:     request.Summary,
			Tags:        append([]string(nil), request.Tags...),
			Importance:  50,
			Confidence:  50,
			Visibility:  "workspace",
		}
		refs[idx] = string(memoryID)
		tasks = append(tasks, func() error {
			topic := events.TopicMemoryCreated
			if err := o.evidence.MemoryItems.Create(ctx, item); err != nil {
				if err != common.ErrConflict {
					return err
				}
				if err := o.evidence.MemoryItems.Update(ctx, item); err != nil {
					return err
				}
				topic = events.TopicMemoryUpdated
			}
			return o.publishMemoryChanged(ctx, topic, item)
		})
	}
	if err := runParallel(tasks...); err != nil {
		return nil, err
	}
	return refs, nil
}

func runtimeMemoryWriteRequests(output map[string]any) ([]runtimeMemoryWriteRequest, error) {
	if output == nil || output[stepOutputKeyMemoryWrites] == nil {
		return runtimeRunSummaryMemoryWrite(output), nil
	}
	var ret []runtimeMemoryWriteRequest
	switch items := output[stepOutputKeyMemoryWrites].(type) {
	case []map[string]any:
		ret = make([]runtimeMemoryWriteRequest, 0, len(items)+1)
		for _, item := range items {
			request, err := runtimeMemoryWriteRequestFromMap(item)
			if err != nil {
				return nil, err
			}
			ret = append(ret, request)
		}
	case []any:
		ret = make([]runtimeMemoryWriteRequest, 0, len(items)+1)
		for _, item := range items {
			mapped, ok := item.(map[string]any)
			if !ok {
				return nil, common.ErrInvalidArgument
			}
			request, err := runtimeMemoryWriteRequestFromMap(mapped)
			if err != nil {
				return nil, err
			}
			ret = append(ret, request)
		}
	default:
		return nil, common.ErrInvalidArgument
	}
	return appendRunSummaryMemoryWrite(ret, output), nil
}

func runtimeMemoryWriteRequestFromMap(item map[string]any) (runtimeMemoryWriteRequest, error) {
	memoryType, _ := item["memoryType"].(string)
	summary, _ := item["summary"].(string)
	content, _ := item["content"].(string)
	memoryType = strings.TrimSpace(memoryType)
	summary = strings.TrimSpace(summary)
	if memoryType == "" || summary == "" || !memorycommon.IsSupportedMemoryType(memoryType) {
		return runtimeMemoryWriteRequest{}, common.ErrInvalidArgument
	}
	return runtimeMemoryWriteRequest{
		MemoryType: memoryType,
		Summary:    summary,
		Content:    content,
		Tags:       stringSliceFromValue(item["tags"]),
	}, nil
}

func runtimeRunSummaryMemoryWrite(output map[string]any) []runtimeMemoryWriteRequest {
	return appendRunSummaryMemoryWrite(nil, output)
}

func appendRunSummaryMemoryWrite(requests []runtimeMemoryWriteRequest, output map[string]any) []runtimeMemoryWriteRequest {
	if output == nil {
		return requests
	}
	stage, _ := output["stage"].(string)
	if stage != "llm_dod_assessment" || hasRunSummaryMemoryWrite(requests) {
		return requests
	}
	summary := runSummaryMemorySummary(output)
	if summary == "" {
		return requests
	}
	return append(requests, runtimeMemoryWriteRequest{
		MemoryType: memorycommon.MemoryTypeRunSummary,
		Summary:    summary,
		Content:    runSummaryMemoryContent(output, summary),
		Tags:       []string{"run_summary"},
	})
}

func runSummaryMemorySummary(output map[string]any) string {
	if output == nil {
		return ""
	}
	if summary := strings.TrimSpace(stringFromMap(output, "summary")); summary != "" {
		return summary
	}
	if assessment := doDAssessmentFromOutput(output); assessment != nil {
		return strings.TrimSpace(stringFromMap(assessment, "summary"))
	}
	return ""
}

func runSummaryMemoryContent(output map[string]any, summary string) string {
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		return summary
	}
	parts := []string{summary}
	for _, key := range []string{"goalOutcome", "implementationSummary", "verificationSummary"} {
		value := strings.TrimSpace(stringFromMap(assessment, key))
		if value != "" && value != summary {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, "\n\n")
}

func completionRunSummaryMemoryContent(
	run *runModel.Run,
	summary string,
	assessment map[string]any,
	evidenceRefs []string,
	patchDetails []resultPatchDetail,
	evidenceDetails []resultEvidenceDetail,
) string {
	parts := []string{summary}
	if run != nil {
		parts = append(parts, fmt.Sprintf("Run: %s", run.ID))
		if run.GoalID != "" {
			parts = append(parts, fmt.Sprintf("Goal: %s", run.GoalID))
		}
		if run.DoDID != "" {
			parts = append(parts, fmt.Sprintf("DoD: %s", run.DoDID))
		}
	}
	for _, key := range []string{"goalOutcome", "implementationSummary", "verificationSummary"} {
		value := strings.TrimSpace(stringFromMap(assessment, key))
		if value != "" && value != summary {
			parts = append(parts, fmt.Sprintf("%s: %s", key, value))
		}
	}
	if items, ok := doDAssessmentItemMaps(assessment["items"]); ok && len(items) > 0 {
		lines := []string{"DoD items:"}
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("- [%s] %s", firstNonEmptyOrchestratorString(stringFromMap(item, "state"), "unknown"), stringFromMap(item, "item")))
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	if len(evidenceRefs) > 0 {
		parts = append(parts, "Evidence refs: "+strings.Join(evidenceRefs, ", "))
	}
	if len(evidenceDetails) > 0 {
		lines := []string{"Verification commands:"}
		for _, detail := range evidenceDetails {
			if detail.Command == "" && detail.ExitCode == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("- %s exitCode=%s", firstNonEmptyOrchestratorString(detail.Command, detail.Kind, detail.Ref), firstNonEmptyOrchestratorString(detail.ExitCode, "unknown")))
		}
		if len(lines) > 1 {
			parts = append(parts, strings.Join(lines, "\n"))
		}
	}
	if len(patchDetails) > 0 {
		lines := []string{"Patch targets:"}
		for _, patch := range patchDetails {
			lines = append(lines, fmt.Sprintf("- %s %s %s", patch.PatchType, patch.TargetRef, patch.State))
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	return strings.Join(parts, "\n\n")
}

func hasRunSummaryMemoryWrite(requests []runtimeMemoryWriteRequest) bool {
	for _, request := range requests {
		if request.MemoryType == memorycommon.MemoryTypeRunSummary {
			return true
		}
	}
	return false
}

func runtimePatchRequests(output map[string]any) ([]runtimePatchRequest, error) {
	if output == nil || output[stepOutputKeyPatches] == nil {
		return nil, nil
	}
	switch items := output[stepOutputKeyPatches].(type) {
	case []map[string]any:
		ret := make([]runtimePatchRequest, 0, len(items))
		for _, item := range items {
			request, err := runtimePatchRequestFromMap(item)
			if err != nil {
				return nil, err
			}
			ret = append(ret, request)
		}
		return ret, nil
	case []any:
		ret := make([]runtimePatchRequest, 0, len(items))
		for _, item := range items {
			mapped, ok := item.(map[string]any)
			if !ok {
				return nil, common.ErrInvalidArgument
			}
			request, err := runtimePatchRequestFromMap(mapped)
			if err != nil {
				return nil, err
			}
			ret = append(ret, request)
		}
		return ret, nil
	default:
		return nil, common.ErrInvalidArgument
	}
}

func runtimePatchRequestFromMap(item map[string]any) (runtimePatchRequest, error) {
	targetRef, _ := item["targetRef"].(string)
	content, _ := item["content"].(string)
	diff, _ := item["diff"].(string)
	summary, _ := item["summary"].(string)
	patchType, _ := item["patchType"].(string)
	operation, _ := item["operation"].(string)
	targetRef = strings.TrimSpace(targetRef)
	if targetRef == "" {
		return runtimePatchRequest{}, common.ErrInvalidArgument
	}
	if isReservedWorkspacePatchTarget(targetRef) {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch targetRef %q is reserved for WorkOrch-managed artifacts; return final query/report content in dodAssessment instead of writing report-only deliverables into the workspace", common.ErrInvalidArgument, targetRef)
	}
	if patchType == "" {
		patchType = strings.TrimSpace(operation)
	}
	if patchType == "" {
		patchType = "file_write"
	}
	switch patchType {
	case "create", "write", "replace", "file":
		patchType = "file_write"
	}
	if patchType != "file_write" && patchType != "unified_diff" {
		return runtimePatchRequest{}, common.ErrInvalidArgument
	}
	if patchType == "unified_diff" && strings.TrimSpace(diff) == "" {
		return runtimePatchRequest{}, common.ErrInvalidArgument
	}
	if patchType == "file_write" && containsUnresolvedRuntimeTemplate(content) {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch content contains unresolved runtime template placeholders", common.ErrInvalidArgument)
	}
	if patchType == "unified_diff" && containsUnresolvedRuntimeTemplate(diff) {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch diff contains unresolved runtime template placeholders", common.ErrInvalidArgument)
	}
	if summary == "" {
		summary = "workspace patch apply"
	}
	return runtimePatchRequest{
		TargetRef: targetRef,
		Content:   content,
		Diff:      diff,
		Summary:   summary,
		PatchType: patchType,
	}, nil
}

func isReservedWorkspacePatchTarget(targetRef string) bool {
	cleanTarget := filepath.ToSlash(filepath.Clean(strings.TrimSpace(targetRef)))
	cleanTarget = strings.TrimPrefix(cleanTarget, "./")
	if cleanTarget == "." || cleanTarget == "" {
		return false
	}
	parts := strings.Split(strings.ToLower(cleanTarget), "/")
	if len(parts) == 0 {
		return false
	}
	switch parts[0] {
	case ".workorch-data", "delivery":
		return true
	default:
		return isReservedWorkspaceReportPatchTarget(cleanTarget)
	}
}

func isReservedWorkspaceReportPatchTarget(cleanTarget string) bool {
	base := strings.ToLower(pathBaseLower(cleanTarget))
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".md", ".txt", ".adoc", ".rst", ".json":
	default:
		return false
	}
	stem := strings.TrimSuffix(base, ext)
	stem = strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(stem)
	reportTerms := strings.FieldsFunc(stem, func(r rune) bool { return r == '_' })
	hasTerm := func(term string) bool {
		for _, item := range reportTerms {
			if item == term {
				return true
			}
		}
		return false
	}
	switch {
	case hasTerm("decision") && hasTerm("record"):
		return true
	case hasTerm("approval") && hasTerm("record"):
		return true
	case hasTerm("delivery") && hasTerm("summary"):
		return true
	case hasTerm("deliverable") && hasTerm("manifest"):
		return true
	case hasTerm("workspace") && hasTerm("deliverable") && hasTerm("manifest"):
		return true
	case hasTerm("result") && hasTerm("summary"):
		return true
	case hasTerm("run") && hasTerm("summary"):
		return true
	case hasTerm("acceptance") && hasTerm("evidence"):
		return true
	case hasTerm("evidence") && hasTerm("chain"):
		return true
	case hasTerm("dod") && hasTerm("assessment"):
		return true
	case hasTerm("final") && (hasTerm("summary") || hasTerm("result") || hasTerm("report")):
		return true
	default:
		return false
	}
}

func containsUnresolvedRuntimeTemplate(value string) bool {
	return strings.Contains(value, "{{") && strings.Contains(value, "}}")
}

func applyWorkspacePatch(workingDirectory string, request runtimePatchRequest) error {
	switch request.PatchType {
	case "file_write":
		return applyWorkspaceFileWrite(workingDirectory, request.TargetRef, request.Content)
	case "unified_diff":
		return applyWorkspaceUnifiedDiff(workingDirectory, request.TargetRef, request.Diff)
	default:
		return common.ErrInvalidArgument
	}
}

func applyWorkspaceFileWrite(workingDirectory string, targetRef string, content string) error {
	target, err := workspaceTargetPath(workingDirectory, targetRef)
	if err != nil {
		return err
	}
	if err := ensureWorkspacePatchParentDirectory(workingDirectory, targetRef, target, "file_write"); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(content), 0o644)
}

func ensureWorkspacePatchParentDirectory(workingDirectory string, targetRef string, target string, operation string) error {
	base, err := filepath.Abs(workingDirectory)
	if err != nil {
		return err
	}
	parent := filepath.Dir(target)
	if parent == base {
		return nil
	}
	relParent, err := filepath.Rel(base, parent)
	if err != nil {
		return err
	}
	current := base
	for _, part := range strings.Split(relParent, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Stat(current)
		if statErr == nil {
			if !info.IsDir() {
				conflictRef, relErr := filepath.Rel(base, current)
				if relErr != nil {
					conflictRef = filepath.Clean(targetRef)
				}
				return &workspacePathConflictError{
					Operation:    operation,
					TargetRef:    filepath.ToSlash(filepath.Clean(targetRef)),
					ConflictRef:  filepath.ToSlash(conflictRef),
					ConflictKind: "file",
				}
			}
			continue
		}
		if !os.IsNotExist(statErr) {
			return statErr
		}
		break
	}
	return os.MkdirAll(parent, 0o755)
}

func workspaceTargetPath(workingDirectory string, targetRef string) (string, error) {
	if strings.TrimSpace(workingDirectory) == "" || strings.TrimSpace(targetRef) == "" {
		return "", common.ErrInvalidArgument
	}
	if filepath.IsAbs(targetRef) {
		return "", common.ErrInvalidArgument
	}
	cleanTarget := filepath.Clean(targetRef)
	if cleanTarget == "." || cleanTarget == ".." || strings.HasPrefix(cleanTarget, ".."+string(filepath.Separator)) {
		return "", common.ErrInvalidArgument
	}
	base, err := filepath.Abs(workingDirectory)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(base, cleanTarget))
	if err != nil {
		return "", err
	}
	if target != base && !strings.HasPrefix(target, base+string(filepath.Separator)) {
		return "", common.ErrInvalidArgument
	}
	return target, nil
}

var unifiedDiffHunkHeaderPattern = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)

type unifiedDiffFilePatch struct {
	OldRef string
	NewRef string
	Hunks  []unifiedDiffHunk
}

type unifiedDiffHunk struct {
	OldStart int
	Lines    []string
}

func applyWorkspaceUnifiedDiff(workingDirectory string, targetRef string, diff string) error {
	target, err := workspaceTargetPath(workingDirectory, targetRef)
	if err != nil {
		return err
	}
	filePatch, err := parseSingleFileUnifiedDiff(diff)
	if err != nil {
		return err
	}
	if !diffTargetMatches(targetRef, filePatch) {
		return common.ErrInvalidArgument
	}

	currentContent, err := os.ReadFile(target)
	if err != nil {
		if !os.IsNotExist(err) || filePatch.OldRef != "/dev/null" {
			return err
		}
		currentContent = nil
	}
	nextContent, err := applyUnifiedDiffFilePatch(string(currentContent), filePatch)
	if err != nil {
		return err
	}
	if err := ensureWorkspacePatchParentDirectory(workingDirectory, targetRef, target, "unified_diff"); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(nextContent), 0o644)
}

func parseSingleFileUnifiedDiff(diff string) (unifiedDiffFilePatch, error) {
	lines := splitDiffLines(diff)
	if len(lines) < 3 {
		return unifiedDiffFilePatch{}, common.ErrInvalidArgument
	}
	idx := 0
	for idx < len(lines) && !strings.HasPrefix(lines[idx], "--- ") {
		idx++
	}
	if idx+2 > len(lines) || !strings.HasPrefix(lines[idx], "--- ") || !strings.HasPrefix(lines[idx+1], "+++ ") {
		return unifiedDiffFilePatch{}, common.ErrInvalidArgument
	}
	filePatch := unifiedDiffFilePatch{
		OldRef: normalizeUnifiedDiffPath(strings.TrimSpace(strings.TrimPrefix(lines[idx], "--- "))),
		NewRef: normalizeUnifiedDiffPath(strings.TrimSpace(strings.TrimPrefix(lines[idx+1], "+++ "))),
	}
	idx += 2
	for idx < len(lines) {
		if strings.HasPrefix(lines[idx], "--- ") {
			return unifiedDiffFilePatch{}, common.ErrInvalidArgument
		}
		if strings.TrimSpace(lines[idx]) == "" {
			idx++
			continue
		}
		if !strings.HasPrefix(lines[idx], "@@ ") {
			return unifiedDiffFilePatch{}, common.ErrInvalidArgument
		}
		hunk, nextIdx, err := parseUnifiedDiffHunk(lines, idx)
		if err != nil {
			return unifiedDiffFilePatch{}, err
		}
		filePatch.Hunks = append(filePatch.Hunks, hunk)
		idx = nextIdx
	}
	if filePatch.NewRef == "" || len(filePatch.Hunks) == 0 {
		return unifiedDiffFilePatch{}, common.ErrInvalidArgument
	}
	return filePatch, nil
}

func parseUnifiedDiffHunk(lines []string, idx int) (unifiedDiffHunk, int, error) {
	matches := unifiedDiffHunkHeaderPattern.FindStringSubmatch(lines[idx])
	if matches == nil {
		return unifiedDiffHunk{}, idx, common.ErrInvalidArgument
	}
	oldStart, err := strconv.Atoi(matches[1])
	if err != nil {
		return unifiedDiffHunk{}, idx, common.ErrInvalidArgument
	}
	hunk := unifiedDiffHunk{OldStart: oldStart}
	idx++
	for idx < len(lines) {
		line := lines[idx]
		if strings.HasPrefix(line, "@@ ") || strings.HasPrefix(line, "--- ") {
			break
		}
		if line == `\ No newline at end of file` {
			idx++
			continue
		}
		if line == "" {
			return unifiedDiffHunk{}, idx, common.ErrInvalidArgument
		}
		switch line[0] {
		case ' ', '-', '+':
			hunk.Lines = append(hunk.Lines, line)
		default:
			return unifiedDiffHunk{}, idx, common.ErrInvalidArgument
		}
		idx++
	}
	if len(hunk.Lines) == 0 {
		return unifiedDiffHunk{}, idx, common.ErrInvalidArgument
	}
	return hunk, idx, nil
}

func applyUnifiedDiffFilePatch(content string, filePatch unifiedDiffFilePatch) (string, error) {
	sourceLines, sourceHadFinalNewline := splitTextLines(content)
	output := make([]string, 0, len(sourceLines))
	cursor := 0
	for _, hunk := range filePatch.Hunks {
		start := hunk.OldStart - 1
		if start < 0 {
			start = 0
		}
		if start < cursor || start > len(sourceLines) {
			return "", common.ErrInvalidArgument
		}
		output = append(output, sourceLines[cursor:start]...)
		cursor = start
		for _, diffLine := range hunk.Lines {
			switch diffLine[0] {
			case ' ':
				if cursor >= len(sourceLines) || sourceLines[cursor] != diffLine[1:] {
					return "", common.ErrInvalidArgument
				}
				output = append(output, sourceLines[cursor])
				cursor++
			case '-':
				if cursor >= len(sourceLines) || sourceLines[cursor] != diffLine[1:] {
					return "", common.ErrInvalidArgument
				}
				cursor++
			case '+':
				output = append(output, diffLine[1:])
			}
		}
	}
	output = append(output, sourceLines[cursor:]...)
	return joinTextLines(output, sourceHadFinalNewline || filePatch.OldRef == "/dev/null"), nil
}

func splitDiffLines(diff string) []string {
	normalized := strings.ReplaceAll(diff, "\r\n", "\n")
	normalized = strings.TrimSuffix(normalized, "\n")
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, "\n")
}

func splitTextLines(content string) ([]string, bool) {
	if content == "" {
		return nil, false
	}
	hadFinalNewline := strings.HasSuffix(content, "\n")
	trimmed := strings.TrimSuffix(content, "\n")
	if trimmed == "" {
		return []string{""}, hadFinalNewline
	}
	return strings.Split(trimmed, "\n"), hadFinalNewline
}

func joinTextLines(lines []string, finalNewline bool) string {
	if len(lines) == 0 {
		return ""
	}
	ret := strings.Join(lines, "\n")
	if finalNewline {
		ret += "\n"
	}
	return ret
}

func normalizeUnifiedDiffPath(input string) string {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return ""
	}
	input = fields[0]
	if input == "/dev/null" {
		return input
	}
	input = strings.TrimPrefix(input, "a/")
	input = strings.TrimPrefix(input, "b/")
	return filepath.Clean(input)
}

func diffTargetMatches(targetRef string, filePatch unifiedDiffFilePatch) bool {
	cleanTarget := filepath.Clean(targetRef)
	if filePatch.NewRef != "/dev/null" {
		return cleanTarget == filePatch.NewRef
	}
	return cleanTarget == filePatch.OldRef
}

func (o *RuntimeOrchestrator) recordToolCallStarted(ctx context.Context, run *runModel.Run, step *runModel.Step) (*runModel.ToolCall, error) {
	if run == nil || step == nil || o.evidence.ToolCalls == nil {
		return &runModel.ToolCall{}, nil
	}
	now := runtimeEvidenceTimestamp()
	toolCall := &runModel.ToolCall{
		ID:          common.ToolCallID(fmt.Sprintf("%s/tool-call/runtime-execute", step.ID)),
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		StepID:      step.ID,
		ToolName:    step.Kind,
		Arguments:   cloneStepPayload(step.Input),
		State:       common.ToolCallStateRunning,
		StartedAt:   now,
	}
	if toolCall.ToolName == "" {
		toolCall.ToolName = "workorch/runtime-step"
	}
	if err := o.evidence.ToolCalls.Create(ctx, toolCall); err != nil {
		if err != common.ErrConflict {
			return nil, err
		}
		existing, getErr := o.evidence.ToolCalls.Get(ctx, toolCall.ID)
		if getErr != nil {
			return nil, getErr
		}
		existing.WorkspaceID = toolCall.WorkspaceID
		existing.RunID = toolCall.RunID
		existing.StepID = toolCall.StepID
		existing.ToolName = toolCall.ToolName
		existing.Arguments = toolCall.Arguments
		existing.State = common.ToolCallStateRunning
		existing.StartedAt = now
		existing.EndedAt = ""
		existing.ExitSummary = ""
		existing.Output = nil
		existing.Error = ""
		if err := o.evidence.ToolCalls.Update(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}
	return toolCall, nil
}

func (o *RuntimeOrchestrator) recordStepDispatchEvidence(ctx context.Context, run *runModel.Run, step *runModel.Step) (*runModel.ToolCall, error) {
	var toolCall *runModel.ToolCall
	var toolCallMu sync.Mutex
	err := runParallel(
		func() error {
			item, err := o.recordToolCallStarted(ctx, run, step)
			toolCallMu.Lock()
			toolCall = item
			toolCallMu.Unlock()
			return err
		},
		func() error {
			return o.recordStepCheckpoint(ctx, run, step, "step_dispatched")
		},
	)
	if err != nil {
		return nil, err
	}
	if toolCall == nil {
		return &runModel.ToolCall{}, nil
	}
	return toolCall, nil
}

func (o *RuntimeOrchestrator) recordToolCallCompleted(ctx context.Context, toolCall *runModel.ToolCall, output map[string]any, dialogLanguage string) error {
	if toolCall == nil || toolCall.ID == "" || o.evidence.ToolCalls == nil {
		return nil
	}
	toolCall.State = common.ToolCallStateCompleted
	toolCall.EndedAt = runtimeEvidenceTimestamp()
	toolCall.ExitSummary = localizedStepCompletionSummary(dialogLanguage)
	toolCall.Output = cloneStepPayload(output)
	toolCall.Error = ""
	if err := o.evidence.ToolCalls.Update(ctx, toolCall); err != nil {
		if err == common.ErrNotFound {
			return o.evidence.ToolCalls.Create(ctx, toolCall)
		}
		return err
	}
	return nil
}

func (o *RuntimeOrchestrator) recordCompletedRuntimeStepEvidence(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, output map[string]any) error {
	evidenceRef := ""
	if toolCall != nil {
		evidenceRef = string(toolCall.ID)
	}
	language := runDialogLanguage(run)
	return runParallel(
		func() error {
			return o.recordToolCallCompleted(ctx, toolCall, output, language)
		},
		func() error {
			return o.recordValidationResult(ctx, run, step, common.ValidationResultStatePassed, localizedStepCompletionSummary(language), evidenceRef, "")
		},
	)
}

func (o *RuntimeOrchestrator) persistCompletedStepState(ctx context.Context, run *runModel.Run, step *runModel.Step) error {
	return runParallel(
		func() error {
			return o.steps.Update(ctx, step)
		},
		func() error {
			return o.recordStepCheckpoint(ctx, run, step, "step_completed")
		},
	)
}

func (o *RuntimeOrchestrator) recordToolCallFailed(ctx context.Context, toolCall *runModel.ToolCall, execErr error, output map[string]any) error {
	if toolCall == nil || toolCall.ID == "" || o.evidence.ToolCalls == nil {
		return nil
	}
	toolCall.State = common.ToolCallStateFailed
	toolCall.EndedAt = runtimeEvidenceTimestamp()
	toolCall.ExitSummary = "runtime step failed"
	toolCall.Output = cloneStepPayload(output)
	if execErr != nil {
		toolCall.Error = execErr.Error()
	}
	if err := o.evidence.ToolCalls.Update(ctx, toolCall); err != nil {
		if err == common.ErrNotFound {
			return o.evidence.ToolCalls.Create(ctx, toolCall)
		}
		return err
	}
	return nil
}

func runtimeFailureOutput(output map[string]any, execErr error) map[string]any {
	ret := cloneStepPayload(output)
	if ret == nil {
		ret = map[string]any{}
	}
	if execErr != nil {
		ret["reason"] = execErr.Error()
		ret["error"] = execErr.Error()
		var diagnosticErr runtimeFailureDiagnosticError
		if errors.As(execErr, &diagnosticErr) {
			if diagnostics := diagnosticErr.RuntimeFailureDiagnostics(); len(diagnostics) > 0 {
				ret["runtimeDiagnostics"] = diagnostics
			}
		}
	}
	return ret
}

func (o *RuntimeOrchestrator) handleRuntimeStepFailure(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	output map[string]any,
	summary string,
	execErr error,
) error {
	if run == nil || step == nil {
		return common.ErrInvalidArgument
	}
	step.State = common.StepStateFailed
	step.Output = runtimeFailureOutput(output, execErr)
	classification := classifyRuntimeFailure(step, step.Output, summary, execErr)
	attachRuntimeFailureClassification(step.Output, classification)
	_ = o.recordToolCallFailed(ctx, toolCall, execErr, step.Output)
	errText := ""
	if execErr != nil {
		errText = execErr.Error()
	}
	evidenceRef := ""
	if toolCall != nil {
		evidenceRef = string(toolCall.ID)
	}
	_ = o.recordValidationResult(ctx, run, step, common.ValidationResultStateFailed, summary, evidenceRef, errText)
	_ = o.recordStepCheckpoint(ctx, run, step, "step_failed")
	_ = o.steps.Update(ctx, step)
	_ = o.publishStepState(ctx, run, step, events.TopicStepStateChanged)
	if graphBatchStep(step) {
		if execErr != nil {
			return execErr
		}
		return common.ErrInvalidArgument
	}
	return o.dispatchRuntimeRepairFollowUp(ctx, run, step, execErr)
}

func (o *RuntimeOrchestrator) dispatchRuntimeRepairFollowUp(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	execErr error,
) error {
	if o.shouldBlockOnDuplicateRuntimeFailure(ctx, run, failedStep) {
		return o.blockOnDuplicateRuntimeFailure(ctx, run, failedStep, execErr)
	}
	if o.shouldEscalateRuntimeRepairExhaustion(ctx, run, failedStep) {
		return o.blockOnRuntimeRepairExhaustion(ctx, run, failedStep, execErr)
	}
	if !o.shouldAttemptRuntimeRepair(ctx, run, failedStep) {
		reason := "runtime step failed"
		if execErr != nil {
			reason = execErr.Error()
		} else if failedStep != nil && failedStep.Output != nil {
			reason = stringFromMap(failedStep.Output, "error")
			if reason == "" {
				reason = stringFromMap(failedStep.Output, "reason")
			}
		}
		return o.FailRun(ctx, run.ID, reason)
	}
	attempt := o.runtimeRepairAttemptCount(ctx, run.ID) + 1
	input := o.runtimeRepairFollowUpStepInput(ctx, run, failedStep, attempt, execErr)
	return o.dispatchStep(ctx, run, runtimeRepairFollowUpStepID(run.ID, failedStep, attempt), initialStepKind(run), input)
}

func (o *RuntimeOrchestrator) shouldEscalateRuntimeRepairExhaustion(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) bool {
	if o == nil || run == nil || failedStep == nil || o.tickets == nil {
		return false
	}
	if !requiresDoDAssessmentCompletionGate(run) || isLLMProviderRuntimeFailure(run, failedStep) {
		return false
	}
	return o.runtimeRepairAttemptCount(ctx, run.ID) >= maxRuntimeRepairAttempts
}

func (o *RuntimeOrchestrator) blockOnRuntimeRepairExhaustion(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	execErr error,
) error {
	if run == nil || failedStep == nil {
		return common.ErrInvalidArgument
	}
	reason := fmt.Sprintf("runtime repair attempts exhausted after %d attempts", maxRuntimeRepairAttempts)
	if execErr != nil {
		reason = fmt.Sprintf("%s: %s", reason, execErr.Error())
	}
	spec := runtimeDecisionBlockSpec{
		TicketID:     common.TicketID(runtimeChildID(failedStep.ID, "repair-exhausted-decision")),
		Reason:       reason,
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Automatic runtime repair attempts were exhausted. Provide missing implementation guidance, choose replan, or reject the current path.",
		Context: map[string]any{
			"attempts":         maxRuntimeRepairAttempts,
			"failureClass":     stringFromMap(failedStep.Output, failureOutputKeyClass),
			"failureSignature": stringFromMap(failedStep.Output, failureOutputKeySignature),
			"failedStepId":     string(failedStep.ID),
			"failedStepKind":   failedStep.Kind,
			"failedStepInput":  compactStepInputForLLMContext(failedStep.Input),
			"failedStepOutput": compactStepOutputForLLMContext(failedStep.Output),
		},
	}
	if err := o.createDecisionTicket(ctx, run, failedStep, spec); err != nil {
		return err
	}
	return o.BlockOnDecision(ctx, run.ID, spec.TicketID)
}

func (o *RuntimeOrchestrator) shouldAttemptRuntimeRepair(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) bool {
	if o == nil || run == nil || failedStep == nil || o.runtime == nil || o.steps == nil {
		return false
	}
	if !requiresDoDAssessmentCompletionGate(run) {
		return false
	}
	if o.runtimeRepairAttemptCount(ctx, run.ID) >= maxRuntimeRepairAttempts {
		return false
	}
	if boolValue(failedStep.Input[stepInputKeyRuntimeRepairFollowUp]) && failedStep.Kind == initialStepKind(run) {
		return false
	}
	return !isLLMProviderRuntimeFailure(run, failedStep)
}

func isLLMProviderRuntimeFailure(run *runModel.Run, step *runModel.Step) bool {
	if run == nil || step == nil || step.Kind != initialStepKind(run) {
		return false
	}
	stage := stringFromMap(step.Output, "stage")
	switch stage {
	case "llm_provider_unavailable",
		"llm_provider_client_unavailable",
		"llm_provider_operation_unsupported",
		"llm_provider_error",
		"llm_provider_stream_error",
		"llm_provider_request_failed",
		"llm_schema_invalid",
		"llm_decision_invalid",
		"llm_failed":
		return true
	default:
		return false
	}
}

func (o *RuntimeOrchestrator) runtimeRepairAttemptCount(ctx context.Context, runID common.RunID) int {
	if o == nil || o.steps == nil || runID == "" {
		return 0
	}
	steps, err := o.steps.ListByRun(ctx, runID)
	if err != nil {
		return 0
	}
	count := 0
	for _, step := range steps {
		if step != nil && boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) {
			count++
		}
	}
	return count
}

func runtimeRepairFollowUpStepID(runID common.RunID, failedStep *runModel.Step, attempt int) common.StepID {
	base := ""
	if failedStep != nil {
		base = runtimeChildID(failedStep.ID, "repair")
	}
	if base == "" {
		base = "repair"
	}
	if attempt < 1 {
		attempt = 1
	}
	return common.StepID(fmt.Sprintf("%s/zz-llm-repair-%s-%d", runID, base, attempt))
}

func (o *RuntimeOrchestrator) runtimeRepairFollowUpStepInput(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	attempt int,
	execErr error,
) map[string]any {
	previousStepID := common.StepID("")
	if failedStep != nil {
		previousStepID = failedStep.ID
	}
	input := nextStepInput(run, previousStepID)
	input[stepInputKeySoulStage] = soulStageExecution
	input[stepInputKeyRuntimeRepairFollowUp] = true
	input[stepInputKeyRuntimeRepairAttempt] = attempt
	input["maxRuntimeRepairAttempts"] = maxRuntimeRepairAttempts
	if execErr != nil {
		input["failureReason"] = execErr.Error()
	}
	if failedStep == nil {
		return input
	}
	input["failedStepId"] = string(failedStep.ID)
	input["failedStepKind"] = failedStep.Kind
	input["failedStepState"] = string(failedStep.State)
	input["failedStepInput"] = compactStepInputForLLMContext(failedStep.Input)
	input["failedStepOutput"] = compactStepOutputForLLMContext(failedStep.Output)
	if class := stringFromMap(failedStep.Output, failureOutputKeyClass); class != "" {
		input[failureOutputKeyClass] = class
	}
	if signature := stringFromMap(failedStep.Output, failureOutputKeySignature); signature != "" {
		input[failureOutputKeySignature] = signature
	}
	input["previousStepKind"] = failedStep.Kind
	input["previousStepOutput"] = compactStepOutputForLLMContext(failedStep.Output)
	if nodeID := graphNodeID(failedStep); nodeID != "" {
		input[stepInputKeyGraphPreviousNodeID] = nodeID
		input["failedGraphNodeId"] = nodeID
	}
	if completed := graphCompletedNodesFromStep(failedStep); len(completed) > 0 {
		input[stepInputKeyGraphCompletedNodes] = completed
	}
	if branch := graphBranch(failedStep); branch != "" {
		input[stepOutputKeyGraphBranch] = branch
	}
	if graph, ok := runtimeGraphSpecFromStep(failedStep); ok {
		input[stepOutputKeyExecutionGraph] = compactExecutionGraphForLLMContext(graph)
		input[stepInputKeyGraphRootStepID] = string(graphRootStepID(failedStep))
	}
	if evidence := o.completedGraphEvidence(ctx, run, failedStep); len(evidence) > 0 {
		input[stepInputKeyGraphEvidence] = evidence
	}
	if shouldIncludeWorkspaceManifestInRuntimeRepair(failedStep) {
		manifest := o.workspaceFileManifest(ctx, run)
		if len(manifest) > 0 {
			input[stepInputKeyWorkspaceFileManifest] = manifest
		}
	}
	return input
}

func shouldIncludeWorkspaceManifestInRuntimeRepair(failedStep *runModel.Step) bool {
	if failedStep == nil || failedStep.Output == nil {
		return false
	}
	if stringFromMap(failedStep.Output, failureOutputKeyClass) == failureClassWorkspacePath {
		return true
	}
	diagnostics := mapFromValue(failedStep.Output["runtimeDiagnostics"])
	if stringFromMap(diagnostics, "kind") == "workspace_path_conflict" {
		return true
	}
	failedOutput := mapFromValue(failedStep.Output["failedStepOutput"])
	if stringFromMap(failedOutput, failureOutputKeyClass) == failureClassWorkspacePath {
		return true
	}
	if diagnostics := mapFromValue(failedOutput["runtimeDiagnostics"]); stringFromMap(diagnostics, "kind") == "workspace_path_conflict" {
		return true
	}
	return false
}

func (o *RuntimeOrchestrator) workspaceFileManifest(ctx context.Context, run *runModel.Run) []map[string]any {
	if o == nil || o.workspaces == nil || run == nil {
		return nil
	}
	workspace, err := o.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil {
		return nil
	}
	return workspaceFileManifestFromDirectory(workspace.WorkingDirectory)
}

func workspaceFileManifestFromDirectory(workingDirectory string) []map[string]any {
	files, err := workspaceManifestFiles(workingDirectory)
	if err != nil || len(files) == 0 {
		return nil
	}
	manifest := make([]map[string]any, 0, len(files))
	for _, file := range files {
		item := map[string]any{
			"path": file.Path,
			"kind": "file",
			"size": file.Size,
		}
		if file.Executable {
			item["executable"] = true
		}
		manifest = append(manifest, item)
	}
	return manifest
}

func stringFromMap(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}

func (o *RuntimeOrchestrator) recordValidationResult(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	state common.ValidationResultState,
	summary string,
	evidenceRef string,
	errText string,
) error {
	if run == nil || step == nil || o.evidence.ValidationResults == nil {
		return nil
	}
	now := runtimeEvidenceTimestamp()
	item := &runModel.ValidationResult{
		ID:             common.ValidationResultID(fmt.Sprintf("%s/validation/runtime-step", step.ID)),
		WorkspaceID:    run.WorkspaceID,
		RunID:          run.ID,
		StepID:         step.ID,
		ValidationKind: "runtime_step",
		TargetRef:      string(step.ID),
		State:          state,
		Summary:        summary,
		EvidenceRef:    evidenceRef,
		StartedAt:      now,
		EndedAt:        now,
	}
	if errText != "" {
		item.Summary = fmt.Sprintf("%s: %s", summary, errText)
	}
	if err := o.evidence.ValidationResults.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return err
		}
		return o.evidence.ValidationResults.Update(ctx, item)
	}
	return nil
}

func (o *RuntimeOrchestrator) recordRunResultFromDoDAssessment(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	output map[string]any,
	workspace *workspaceModel.Workspace,
) error {
	if run == nil || step == nil || o.evidence.Results == nil || !requiresDoDAssessmentCompletionGate(run) {
		return nil
	}
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		return nil
	}
	summary, _ := assessment["summary"].(string)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	return o.recordRunResultFromValidatedDoDAssessment(ctx, run, step, assessment, summary, workspace)
}

func (o *RuntimeOrchestrator) recordRunResultFromValidatedDoDAssessment(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	assessment map[string]any,
	summary string,
	workspace *workspaceModel.Workspace,
) error {
	if run == nil || step == nil || o.evidence.Results == nil || !requiresDoDAssessmentCompletionGate(run) || assessment == nil {
		return nil
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	now := runtimeEvidenceTimestamp()
	evidenceRefs := doDAssessmentEvidenceRefs(step.ID, assessment)
	patchDetails := o.resultPatchDetails(ctx, run.ID)
	expectedArtifactRefs := expectedCompletionArtifactRefs(run, patchDetails)
	manifestCh := make(chan string, 1)
	go func() {
		manifestCh <- o.workspaceDeliverableManifestContent(ctx, run, step, assessment, evidenceRefs, workspace, patchDetails)
	}()
	type artifactWriteResult struct {
		refs []common.ArtifactID
		err  error
	}
	summaryArtifactCh := make(chan artifactWriteResult, 1)
	go func() {
		refs, err := o.recordResultSummaryArtifact(ctx, run, step, summary, assessment, evidenceRefs, expectedArtifactRefs, now)
		summaryArtifactCh <- artifactWriteResult{refs: refs, err: err}
	}()
	manifest := <-manifestCh
	manifestArtifactCh := make(chan artifactWriteResult, 1)
	go func() {
		ref, err := o.recordWorkspaceDeliverableManifestArtifact(ctx, run, step, manifest, evidenceRefs, now)
		var refs []common.ArtifactID
		if ref != "" {
			refs = []common.ArtifactID{ref}
		}
		manifestArtifactCh <- artifactWriteResult{refs: refs, err: err}
	}()
	patchArtifactCh := make(chan artifactWriteResult, 1)
	go func() {
		refs, err := o.recordPatchDiffArtifact(ctx, run, step, patchDetails, now)
		patchArtifactCh <- artifactWriteResult{refs: refs, err: err}
	}()
	summaryArtifact := <-summaryArtifactCh
	if summaryArtifact.err != nil {
		<-manifestArtifactCh
		<-patchArtifactCh
		return summaryArtifact.err
	}
	manifestArtifact := <-manifestArtifactCh
	if manifestArtifact.err != nil {
		<-patchArtifactCh
		return manifestArtifact.err
	}
	patchArtifact := <-patchArtifactCh
	if patchArtifact.err != nil {
		return patchArtifact.err
	}
	artifactRefs := append([]common.ArtifactID(nil), summaryArtifact.refs...)
	artifactRefs = append(artifactRefs, manifestArtifact.refs...)
	artifactRefs = append(artifactRefs, patchArtifact.refs...)
	dodAssessment := resultDoDAssessmentFromMap(assessment)
	item := &resultmodel.Result{
		ID:            common.ResultID(fmt.Sprintf("%s/result/dod-assessment", run.ID)),
		WorkspaceID:   run.WorkspaceID,
		RunID:         run.ID,
		GoalID:        run.GoalID,
		DoDID:         run.DoDID,
		State:         common.ResultStateAccepted,
		Title:         "Accepted Result",
		Summary:       summary,
		SourceType:    resultcommon.SourceTypeDoDAssessment,
		SourceRef:     string(step.ID),
		EvidenceRefs:  evidenceRefs,
		ArtifactRefs:  artifactRefs,
		DoDAssessment: dodAssessment,
		SummaryDetail: resultSummaryFromDoDAssessment(run, summary, assessment, dodAssessment, evidenceRefs, artifactRefs),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := o.evidence.Results.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return err
		}
		existing, getErr := o.evidence.Results.Get(ctx, item.ID)
		if getErr != nil {
			return getErr
		}
		existing.WorkspaceID = item.WorkspaceID
		existing.RunID = item.RunID
		existing.GoalID = item.GoalID
		existing.DoDID = item.DoDID
		existing.State = item.State
		existing.Title = item.Title
		existing.Summary = item.Summary
		existing.SourceType = item.SourceType
		existing.SourceRef = item.SourceRef
		existing.EvidenceRefs = item.EvidenceRefs
		existing.ArtifactRefs = item.ArtifactRefs
		existing.DoDAssessment = item.DoDAssessment
		existing.SummaryDetail = item.SummaryDetail
		existing.UpdatedAt = now
		if err := o.evidence.Results.Update(ctx, existing); err != nil {
			return err
		}
		return o.publishResultAccepted(ctx, existing)
	}
	return o.publishResultAccepted(ctx, item)
}

func (o *RuntimeOrchestrator) recordCompletionEvidenceFromValidatedDoDAssessment(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	assessment map[string]any,
	summary string,
	workspace *workspaceModel.Workspace,
) error {
	return runParallel(
		func() error {
			return o.recordRunResultFromValidatedDoDAssessment(ctx, run, step, assessment, summary, workspace)
		},
		func() error {
			return o.recordSkillFeedbackMemoryFromValidatedDoDAssessment(ctx, run, step, assessment, summary)
		},
		func() error {
			return o.recordRunSummaryMemoryFromValidatedDoDAssessment(ctx, run, step, assessment, summary)
		},
	)
}

func (o *RuntimeOrchestrator) recordSkillFeedbackMemory(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	output map[string]any,
) error {
	if run == nil || step == nil || !requiresDoDAssessmentCompletionGate(run) {
		return nil
	}
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		return nil
	}
	summary, _ := assessment["summary"].(string)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	return o.recordSkillFeedbackMemoryFromValidatedDoDAssessment(ctx, run, step, assessment, summary)
}

func (o *RuntimeOrchestrator) recordSkillFeedbackMemoryFromValidatedDoDAssessment(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	assessment map[string]any,
	summary string,
) error {
	if run == nil || step == nil || o.evidence.MemoryItems == nil || !requiresDoDAssessmentCompletionGate(run) || assessment == nil {
		return nil
	}
	skillIDs := stringSliceFromValue(run.ExecutionContext["skillIds"])
	if len(skillIDs) == 0 {
		return nil
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	item := &memorymodel.MemoryItem{
		ID:          common.MemoryID(fmt.Sprintf("%s/memory/skill-feedback", run.ID)),
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		SourceType:  memorycommon.MemorySourceRun,
		SourceRef:   string(step.ID),
		MemoryType:  memorycommon.MemoryTypeSkillFeedback,
		Content: fmt.Sprintf(
			"Run %s completed with selected skills: %s.\nDoD summary: %s",
			run.ID,
			strings.Join(skillIDs, ", "),
			summary,
		),
		Summary:    fmt.Sprintf("Selected skills completed run: %s", strings.Join(skillIDs, ", ")),
		Tags:       []string{"skill_feedback", "run_completed"},
		Importance: 50,
		Confidence: 50,
		Visibility: "workspace",
	}
	if err := o.evidence.MemoryItems.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return err
		}
		existing, getErr := o.evidence.MemoryItems.Get(ctx, item.ID)
		if getErr != nil {
			return getErr
		}
		existing.WorkspaceID = item.WorkspaceID
		existing.RunID = item.RunID
		existing.SourceType = item.SourceType
		existing.SourceRef = item.SourceRef
		existing.MemoryType = item.MemoryType
		existing.Content = item.Content
		existing.Summary = item.Summary
		existing.Tags = item.Tags
		existing.Importance = item.Importance
		existing.Confidence = item.Confidence
		existing.Visibility = item.Visibility
		if err := o.evidence.MemoryItems.Update(ctx, existing); err != nil {
			return err
		}
		return o.publishMemoryChanged(ctx, events.TopicMemoryUpdated, existing)
	}
	return o.publishMemoryChanged(ctx, events.TopicMemoryCreated, item)
}

func (o *RuntimeOrchestrator) recordRunSummaryMemoryFromValidatedDoDAssessment(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	assessment map[string]any,
	summary string,
) error {
	if run == nil || step == nil || o.evidence.MemoryItems == nil || !requiresDoDAssessmentCompletionGate(run) || assessment == nil {
		return nil
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	evidenceRefs := doDAssessmentEvidenceRefs(step.ID, assessment)
	item := &memorymodel.MemoryItem{
		ID:          common.MemoryID(fmt.Sprintf("%s/memory/run-summary", run.ID)),
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		SourceType:  memorycommon.MemorySourceRun,
		SourceRef:   string(step.ID),
		MemoryType:  memorycommon.MemoryTypeRunSummary,
		Content:     completionRunSummaryMemoryContent(run, summary, assessment, evidenceRefs, o.resultPatchDetails(ctx, run.ID), o.resultEvidenceDetails(ctx, evidenceRefs)),
		Summary:     summary,
		Tags:        []string{"run_summary", "dod_assessment", "code_agent"},
		Importance:  70,
		Confidence:  70,
		Visibility:  "workspace",
	}
	if err := o.evidence.MemoryItems.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return err
		}
		existing, getErr := o.evidence.MemoryItems.Get(ctx, item.ID)
		if getErr != nil {
			return getErr
		}
		existing.WorkspaceID = item.WorkspaceID
		existing.RunID = item.RunID
		existing.SourceType = item.SourceType
		existing.SourceRef = item.SourceRef
		existing.MemoryType = item.MemoryType
		existing.Content = item.Content
		existing.Summary = item.Summary
		existing.Tags = item.Tags
		existing.Importance = item.Importance
		existing.Confidence = item.Confidence
		existing.Visibility = item.Visibility
		if err := o.evidence.MemoryItems.Update(ctx, existing); err != nil {
			return err
		}
		return o.publishMemoryChanged(ctx, events.TopicMemoryUpdated, existing)
	}
	return o.publishMemoryChanged(ctx, events.TopicMemoryCreated, item)
}

func (o *RuntimeOrchestrator) recordResultSummaryArtifact(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	summary string,
	assessment map[string]any,
	evidenceRefs []string,
	artifactRefs []common.ArtifactID,
	now string,
) ([]common.ArtifactID, error) {
	if run == nil || step == nil || o.evidence.Artifacts == nil {
		return nil, nil
	}
	item := &resultmodel.Artifact{
		ID:           common.ArtifactID(fmt.Sprintf("%s/artifact/result-summary", run.ID)),
		WorkspaceID:  run.WorkspaceID,
		RunID:        run.ID,
		ResultID:     common.ResultID(fmt.Sprintf("%s/result/dod-assessment", run.ID)),
		StepID:       step.ID,
		ArtifactType: resultcommon.ArtifactTypeText,
		Category:     resultcommon.ArtifactCategoryFinalSummary,
		Name:         "Accepted Result Summary",
		Summary:      resultSummaryArtifactContent(run, summary, assessment, evidenceRefs, artifactRefs, o.resultEvidenceDetails(ctx, evidenceRefs)),
		URI:          fmt.Sprintf("workorch://runs/%s/results/dod-assessment/summary", run.ID),
		ContentType:  "text/markdown; charset=utf-8",
		State:        common.ArtifactStateAvailable,
		EvidenceRefs: evidenceRefs,
		Metadata: map[string]any{
			"sourceType": resultcommon.SourceTypeDoDAssessment,
			"sourceRef":  string(step.ID),
			"category":   resultcommon.ArtifactCategoryFinalSummary,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := o.evidence.Artifacts.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return nil, err
		}
		existing, getErr := o.evidence.Artifacts.Get(ctx, item.ID)
		if getErr != nil {
			return nil, getErr
		}
		existing.WorkspaceID = item.WorkspaceID
		existing.RunID = item.RunID
		existing.ResultID = item.ResultID
		existing.StepID = item.StepID
		existing.ArtifactType = item.ArtifactType
		existing.Category = item.Category
		existing.Name = item.Name
		existing.Summary = item.Summary
		existing.URI = item.URI
		existing.ContentType = item.ContentType
		existing.State = item.State
		existing.EvidenceRefs = item.EvidenceRefs
		existing.Metadata = item.Metadata
		existing.UpdatedAt = now
		if err := o.evidence.Artifacts.Update(ctx, existing); err != nil {
			return nil, err
		}
		return []common.ArtifactID{existing.ID}, o.publishArtifactCreated(ctx, existing)
	}
	return []common.ArtifactID{item.ID}, o.publishArtifactCreated(ctx, item)
}

func (o *RuntimeOrchestrator) recordWorkspaceDeliverableManifestArtifact(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	manifest string,
	evidenceRefs []string,
	now string,
) (common.ArtifactID, error) {
	if run == nil || step == nil || o.evidence.Artifacts == nil {
		return "", nil
	}
	item := &resultmodel.Artifact{
		ID:           common.ArtifactID(fmt.Sprintf("%s/artifact/deliverable-manifest", run.ID)),
		WorkspaceID:  run.WorkspaceID,
		RunID:        run.ID,
		ResultID:     common.ResultID(fmt.Sprintf("%s/result/dod-assessment", run.ID)),
		StepID:       step.ID,
		ArtifactType: resultcommon.ArtifactTypeReport,
		Category:     resultcommon.ArtifactCategoryRuntimeEvidence,
		Name:         "Workspace Deliverable Manifest",
		Summary:      manifest,
		URI:          fmt.Sprintf("workorch://runs/%s/results/dod-assessment/deliverable-manifest", run.ID),
		ContentType:  "text/markdown; charset=utf-8",
		State:        common.ArtifactStateAvailable,
		EvidenceRefs: evidenceRefs,
		Metadata: map[string]any{
			"sourceType":   resultcommon.SourceTypeDoDAssessment,
			"sourceRef":    string(step.ID),
			"manifestType": "workspace_deliverables",
			"category":     resultcommon.ArtifactCategoryRuntimeEvidence,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := o.evidence.Artifacts.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return "", err
		}
		existing, getErr := o.evidence.Artifacts.Get(ctx, item.ID)
		if getErr != nil {
			return "", getErr
		}
		existing.WorkspaceID = item.WorkspaceID
		existing.RunID = item.RunID
		existing.ResultID = item.ResultID
		existing.StepID = item.StepID
		existing.ArtifactType = item.ArtifactType
		existing.Category = item.Category
		existing.Name = item.Name
		existing.Summary = item.Summary
		existing.URI = item.URI
		existing.ContentType = item.ContentType
		existing.State = item.State
		existing.EvidenceRefs = item.EvidenceRefs
		existing.Metadata = item.Metadata
		existing.UpdatedAt = now
		if err := o.evidence.Artifacts.Update(ctx, existing); err != nil {
			return "", err
		}
		return existing.ID, o.publishArtifactCreated(ctx, existing)
	}
	return item.ID, o.publishArtifactCreated(ctx, item)
}

func (o *RuntimeOrchestrator) recordPatchDiffArtifact(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	patchDetails []resultPatchDetail,
	now string,
) ([]common.ArtifactID, error) {
	if run == nil || step == nil || o.evidence.Artifacts == nil || !hasPatchDeliverableContent(patchDetails) {
		return nil, nil
	}
	patchIDs := make([]string, 0, len(patchDetails))
	targetRefs := make([]string, 0, len(patchDetails))
	for _, patch := range patchDetails {
		if patch.ID != "" {
			patchIDs = append(patchIDs, string(patch.ID))
		}
		if patch.TargetRef != "" {
			targetRefs = append(targetRefs, patch.TargetRef)
		}
	}
	item := &resultmodel.Artifact{
		ID:           patchDiffArtifactID(run),
		WorkspaceID:  run.WorkspaceID,
		RunID:        run.ID,
		ResultID:     common.ResultID(fmt.Sprintf("%s/result/dod-assessment", run.ID)),
		StepID:       step.ID,
		ArtifactType: resultcommon.ArtifactTypePatch,
		Category:     resultcommon.ArtifactCategoryRuntimeEvidence,
		Name:         "Code Change Diff",
		Summary:      patchDiffArtifactContent(patchDetails),
		URI:          fmt.Sprintf("workorch://runs/%s/results/dod-assessment/code-change-diff", run.ID),
		ContentType:  "text/markdown; charset=utf-8",
		State:        common.ArtifactStateAvailable,
		EvidenceRefs: patchIDs,
		Metadata: map[string]any{
			"sourceType":   resultcommon.SourceTypeDoDAssessment,
			"sourceRef":    string(step.ID),
			"category":     resultcommon.ArtifactCategoryRuntimeEvidence,
			"artifactKind": "code_change_diff",
			"patchIds":     patchIDs,
			"targetRefs":   targetRefs,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if len(patchDetails) == 1 {
		item.PatchID = patchDetails[0].ID
	}
	if err := o.evidence.Artifacts.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return nil, err
		}
		existing, getErr := o.evidence.Artifacts.Get(ctx, item.ID)
		if getErr != nil {
			return nil, getErr
		}
		existing.WorkspaceID = item.WorkspaceID
		existing.RunID = item.RunID
		existing.ResultID = item.ResultID
		existing.StepID = item.StepID
		existing.PatchID = item.PatchID
		existing.ArtifactType = item.ArtifactType
		existing.Category = item.Category
		existing.Name = item.Name
		existing.Summary = item.Summary
		existing.URI = item.URI
		existing.ContentType = item.ContentType
		existing.State = item.State
		existing.EvidenceRefs = item.EvidenceRefs
		existing.Metadata = item.Metadata
		existing.UpdatedAt = now
		if err := o.evidence.Artifacts.Update(ctx, existing); err != nil {
			return nil, err
		}
		return []common.ArtifactID{existing.ID}, o.publishArtifactCreated(ctx, existing)
	}
	return []common.ArtifactID{item.ID}, o.publishArtifactCreated(ctx, item)
}

func resultSummaryFromDoDAssessment(
	run *runModel.Run,
	summary string,
	assessment map[string]any,
	dodAssessment resultmodel.DoDAssessment,
	evidenceRefs []string,
	artifactRefs []common.ArtifactID,
) resultmodel.ResultSummary {
	ret := resultmodel.ResultSummary{
		GoalOutcome: firstNonEmptyOrchestratorString(
			stringFromMap(assessment, "goalOutcome"),
			stringFromMap(assessment, "implementationSummary"),
			summary,
		),
		DoDItems:              append([]resultmodel.DoDAssessmentItem(nil), dodAssessment.Items...),
		ImplementationSummary: firstNonEmptyOrchestratorString(stringFromMap(assessment, "implementationSummary"), summary),
		VerificationSummary:   firstNonEmptyOrchestratorString(stringFromMap(assessment, "verificationSummary"), verificationSummaryFromEvidence(runDialogLanguage(run), evidenceRefs)),
		EvidenceRefs:          append([]string(nil), evidenceRefs...),
		ArtifactRefs:          append([]common.ArtifactID(nil), artifactRefs...),
		KnownLimitations:      stringSliceFromValue(assessment["knownLimitations"]),
		ResidualRisks:         stringSliceFromValue(assessment["residualRisks"]),
		NextActions:           stringSliceFromValue(assessment["nextActions"]),
	}
	if len(ret.ResidualRisks) == 0 {
		ret.ResidualRisks = localizedDefaultList(runDialogLanguage(run), "residual_risks")
	}
	if len(ret.KnownLimitations) == 0 {
		ret.KnownLimitations = localizedDefaultList(runDialogLanguage(run), "known_limitations")
	}
	if len(ret.NextActions) == 0 {
		ret.NextActions = localizedDefaultList(runDialogLanguage(run), "next_actions")
	}
	return ret
}

func (o *RuntimeOrchestrator) resultPatchDetails(ctx context.Context, runID common.RunID) []resultPatchDetail {
	if o == nil || o.evidence.Patches == nil || runID == "" {
		return nil
	}
	patches, err := o.evidence.Patches.ListByRun(ctx, runID)
	if err != nil || len(patches) == 0 {
		return nil
	}
	ret := make([]resultPatchDetail, 0, len(patches))
	for _, patch := range patches {
		if patch == nil {
			continue
		}
		ret = append(ret, resultPatchDetail{
			ID:        patch.ID,
			PatchType: strings.TrimSpace(patch.PatchType),
			TargetRef: strings.TrimSpace(patch.TargetRef),
			Summary:   strings.TrimSpace(patch.Summary),
			State:     patch.State,
			Content:   patch.Content,
			Diff:      patch.Diff,
		})
	}
	return ret
}

func (o *RuntimeOrchestrator) resultEvidenceDetails(ctx context.Context, evidenceRefs []string) []resultEvidenceDetail {
	if o == nil || o.steps == nil || len(evidenceRefs) == 0 {
		return nil
	}
	ret := make([]resultEvidenceDetail, 0, len(evidenceRefs))
	for _, ref := range evidenceRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		step, err := o.steps.Get(ctx, common.StepID(ref))
		if err != nil || step == nil || len(step.Output) == 0 {
			continue
		}
		detail := resultEvidenceDetail{
			Ref:      ref,
			Kind:     step.Kind,
			Command:  strings.TrimSpace(stringFromMap(step.Output, "command")),
			ExitCode: evidenceValueString(step.Output["exitCode"]),
			Stdout:   boundedEvidenceText(stringFromMap(step.Output, "stdout")),
			Stderr:   boundedEvidenceText(stringFromMap(step.Output, "stderr")),
			Summary:  strings.TrimSpace(stringFromMap(step.Output, "summary")),
		}
		if detail.Stdout == "" && detail.Stderr == "" && detail.Summary == "" {
			continue
		}
		ret = append(ret, detail)
	}
	return ret
}

func resultSummaryArtifactContent(run *runModel.Run, summary string, assessment map[string]any, evidenceRefs []string, artifactRefs []common.ArtifactID, evidenceDetails []resultEvidenceDetail) string {
	var builder strings.Builder
	language := runDialogLanguage(run)
	builder.WriteString("# ")
	builder.WriteString(localizedSummaryLabel(language, "title"))
	builder.WriteString("\n\n## ")
	builder.WriteString(localizedSummaryLabel(language, "goal_outcome"))
	builder.WriteString("\n\n")
	builder.WriteString(firstNonEmptyOrchestratorString(stringFromMap(assessment, "goalOutcome"), summary))
	builder.WriteString("\n\n## ")
	builder.WriteString(localizedSummaryLabel(language, "implementation_summary"))
	builder.WriteString("\n\n")
	builder.WriteString(firstNonEmptyOrchestratorString(stringFromMap(assessment, "implementationSummary"), summary))
	builder.WriteString("\n\n## ")
	builder.WriteString(localizedSummaryLabel(language, "verification_summary"))
	builder.WriteString("\n\n")
	builder.WriteString(firstNonEmptyOrchestratorString(stringFromMap(assessment, "verificationSummary"), verificationSummaryFromEvidence(language, evidenceRefs)))
	writeEvidenceDetailSection(&builder, language, evidenceDetails)
	if items, ok := doDAssessmentItemMaps(assessment["items"]); ok && len(items) > 0 {
		builder.WriteString("\n\n## ")
		builder.WriteString(localizedSummaryLabel(language, "dod_items"))
		builder.WriteString("\n\n")
		builder.WriteString(localizedSummaryLabel(language, "dod_table_header"))
		builder.WriteString("\n")
		builder.WriteString("| --- | --- | --- |\n")
		for _, item := range items {
			builder.WriteString(fmt.Sprintf(
				"| %s | %s | %s |\n",
				markdownTableCell(stringFromMap(item, "item")),
				markdownTableCell(stringFromMap(item, "state")),
				markdownTableCell(strings.Join(evidenceRefsFromValue(item["evidenceRefs"]), ", ")),
			))
		}
	}
	writeStringListSection(&builder, localizedSummaryLabel(language, "evidence_refs"), evidenceRefs)
	writeArtifactRefSection(&builder, localizedSummaryLabel(language, "artifact_refs"), artifactRefs)
	knownLimitations := stringSliceFromValue(assessment["knownLimitations"])
	if len(knownLimitations) == 0 {
		knownLimitations = localizedDefaultList(language, "known_limitations")
	}
	writeStringListSection(&builder, localizedSummaryLabel(language, "known_limitations"), knownLimitations)
	residualRisks := stringSliceFromValue(assessment["residualRisks"])
	if len(residualRisks) == 0 {
		residualRisks = localizedDefaultList(language, "residual_risks")
	}
	writeStringListSection(&builder, localizedSummaryLabel(language, "residual_risks"), residualRisks)
	nextActions := stringSliceFromValue(assessment["nextActions"])
	if len(nextActions) == 0 {
		nextActions = localizedDefaultList(language, "next_actions")
	}
	writeStringListSection(&builder, localizedSummaryLabel(language, "next_actions"), nextActions)
	return strings.TrimSpace(builder.String())
}

func writeArtifactRefSection(builder *strings.Builder, title string, values []common.ArtifactID) {
	if builder == nil || len(values) == 0 {
		return
	}
	builder.WriteString(fmt.Sprintf("\n\n## %s\n\n", title))
	for _, value := range values {
		if value == "" {
			continue
		}
		builder.WriteString("- `")
		builder.WriteString(markdownInlineCode(string(value)))
		builder.WriteString("`\n")
	}
}

func writeEvidenceDetailSection(builder *strings.Builder, language string, details []resultEvidenceDetail) {
	if builder == nil || len(details) == 0 {
		return
	}
	builder.WriteString("\n\n## ")
	builder.WriteString(localizedSummaryLabel(language, "evidence_details"))
	builder.WriteString("\n")
	for _, detail := range details {
		builder.WriteString("\n### ")
		builder.WriteString(markdownHeadingText(firstNonEmptyOrchestratorString(detail.Ref, detail.Kind)))
		builder.WriteString("\n")
		if detail.Kind != "" {
			builder.WriteString("\n- Kind: `")
			builder.WriteString(markdownInlineCode(detail.Kind))
			builder.WriteString("`\n")
		}
		if detail.ExitCode != "" {
			builder.WriteString("- Exit Code: `")
			builder.WriteString(markdownInlineCode(detail.ExitCode))
			builder.WriteString("`\n")
		}
		if detail.Command != "" {
			builder.WriteString("- Command: `")
			builder.WriteString(markdownInlineCode(detail.Command))
			builder.WriteString("`\n")
		}
		if detail.Summary != "" {
			builder.WriteString("\n")
			builder.WriteString(detail.Summary)
			builder.WriteString("\n")
		}
		if detail.Stdout != "" {
			builder.WriteString("\n")
			builder.WriteString(localizedSummaryLabel(language, "stdout"))
			builder.WriteString(":\n\n```text\n")
			builder.WriteString(detail.Stdout)
			builder.WriteString("\n```\n")
		}
		if detail.Stderr != "" {
			builder.WriteString("\n")
			builder.WriteString(localizedSummaryLabel(language, "stderr"))
			builder.WriteString(":\n\n```text\n")
			builder.WriteString(detail.Stderr)
			builder.WriteString("\n```\n")
		}
	}
}

func verificationSummaryFromEvidence(language string, evidenceRefs []string) string {
	if len(evidenceRefs) == 0 {
		if prefersChinese(language) {
			return "验收评估未报告明确的证据引用。"
		}
		return "The acceptance assessment did not report explicit evidence references."
	}
	if prefersChinese(language) {
		return fmt.Sprintf("验收结论由 %d 个证据引用支撑。", len(evidenceRefs))
	}
	return fmt.Sprintf("Acceptance is backed by %d evidence reference(s).", len(evidenceRefs))
}

func localizedStepCompletionSummary(language string) string {
	if prefersChinese(language) {
		return "runtime 步骤已完成"
	}
	return "runtime step completed"
}

func localizedDefaultList(language string, kind string) []string {
	switch kind {
	case "residual_risks":
		if prefersChinese(language) {
			return []string{"验收评估未报告剩余风险。"}
		}
		return []string{"No residual risks were reported by the acceptance assessment."}
	case "known_limitations":
		if prefersChinese(language) {
			return []string{"验收评估未报告已知限制。"}
		}
		return []string{"No known limitations were reported by the acceptance assessment."}
	case "next_actions":
		if prefersChinese(language) {
			return []string{"使用生成的交付物和证据引用进行验收确认。"}
		}
		return []string{"Use the generated Artifact and evidence refs for user acceptance review."}
	default:
		return nil
	}
}

func localizedSummaryLabel(language string, key string) string {
	if prefersChinese(language) {
		switch key {
		case "title":
			return "已验收目标成果摘要"
		case "goal_outcome":
			return "目标成果"
		case "implementation_summary":
			return "实现摘要"
		case "verification_summary":
			return "验证摘要"
		case "evidence_details":
			return "证据输出"
		case "stdout":
			return "标准输出"
		case "stderr":
			return "标准错误"
		case "dod_items":
			return "验收项确认"
		case "dod_table_header":
			return "| 验收项 | 状态 | 证据 |"
		case "evidence_refs":
			return "证据引用"
		case "artifact_refs":
			return "交付物索引"
		case "known_limitations":
			return "已知限制"
		case "residual_risks":
			return "剩余风险"
		case "next_actions":
			return "后续动作"
		}
	}
	switch key {
	case "title":
		return "Accepted Result Summary"
	case "goal_outcome":
		return "Goal Outcome"
	case "implementation_summary":
		return "Implementation Summary"
	case "verification_summary":
		return "Verification Summary"
	case "evidence_details":
		return "Evidence Output"
	case "stdout":
		return "Stdout"
	case "stderr":
		return "Stderr"
	case "dod_items":
		return "DoD Items"
	case "dod_table_header":
		return "| Item | State | Evidence |"
	case "evidence_refs":
		return "Evidence Refs"
	case "artifact_refs":
		return "Artifact Refs"
	case "known_limitations":
		return "Known Limitations"
	case "residual_risks":
		return "Residual Risks"
	case "next_actions":
		return "Next Actions"
	default:
		return key
	}
}

func runDialogLanguage(run *runModel.Run) string {
	if run == nil {
		return ""
	}
	return strings.TrimSpace(stringFromMap(run.ExecutionContext, "dialogLanguage"))
}

func prefersChinese(language string) bool {
	language = strings.ToLower(strings.TrimSpace(language))
	return strings.Contains(language, "zh") || strings.Contains(language, "chinese") || strings.Contains(language, "中文") || strings.Contains(language, "汉语")
}

func writeStringListSection(builder *strings.Builder, title string, values []string) {
	if builder == nil || len(values) == 0 {
		return
	}
	builder.WriteString(fmt.Sprintf("\n\n## %s\n\n", title))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		builder.WriteString(fmt.Sprintf("- %s\n", value))
	}
}

func markdownTableCell(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func markdownInlineCode(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "`", "'")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func markdownHeadingText(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func evidenceValueString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func boundedEvidenceText(value string) string {
	value = strings.TrimSpace(value)
	const limit = 12000
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n... truncated ..."
}

func firstNonEmptyOrchestratorString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (o *RuntimeOrchestrator) workspaceDeliverableManifestContent(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	assessment map[string]any,
	evidenceRefs []string,
	workspace *workspaceModel.Workspace,
	patchDetails []resultPatchDetail,
) string {
	var builder strings.Builder
	builder.WriteString("# Workspace Deliverable Manifest\n\n")
	builder.WriteString(fmt.Sprintf("- Workspace: `%s`\n", run.WorkspaceID))
	builder.WriteString(fmt.Sprintf("- Run: `%s`\n", run.ID))
	builder.WriteString(fmt.Sprintf("- Result: `%s`\n", fmt.Sprintf("%s/result/dod-assessment", run.ID)))
	builder.WriteString(fmt.Sprintf("- Source step: `%s`\n", step.ID))
	if summary, _ := assessment["summary"].(string); strings.TrimSpace(summary) != "" {
		builder.WriteString(fmt.Sprintf("- DoD summary: %s\n", strings.TrimSpace(summary)))
	}
	builder.WriteString("\n## Deliverable Files\n\n")
	files := workspaceDeliverableFilesFromPatchDetails(patchDetails)
	if len(files) == 0 {
		builder.WriteString("- No workspace file deliverables were produced in this run.\n")
	} else {
		builder.WriteString("| Path | Category | Purpose | Size |\n")
		builder.WriteString("| --- | --- | --- | ---: |\n")
		for _, file := range files {
			builder.WriteString(fmt.Sprintf(
				"| `%s` | %s | %s | %d bytes |\n",
				markdownTableCell(file.Path),
				markdownTableCell(file.Category),
				markdownTableCell(artifactPurposeForCategory(file.Category)),
				file.Size,
			))
		}
	}
	writePatchEvidenceSection(&builder, patchDetails)
	builder.WriteString("\n## DoD Relation\n\n")
	if items, ok := doDAssessmentItemMaps(assessment["items"]); ok && len(items) > 0 {
		builder.WriteString("| DoD Item | State | Evidence |\n")
		builder.WriteString("| --- | --- | --- |\n")
		for _, item := range items {
			builder.WriteString(fmt.Sprintf(
				"| %s | %s | %s |\n",
				markdownTableCell(stringFromMap(item, "item")),
				markdownTableCell(stringFromMap(item, "state")),
				markdownTableCell(strings.Join(evidenceRefsFromValue(item["evidenceRefs"]), ", ")),
			))
		}
	} else {
		builder.WriteString("- No item-level DoD relation was reported by the assessment.\n")
	}
	builder.WriteString("\n## Acceptance Evidence\n\n")
	for _, ref := range evidenceRefs {
		builder.WriteString(fmt.Sprintf("- `%s`\n", ref))
	}
	return strings.TrimSpace(builder.String())
}

func workspaceDeliverableFilesFromPatchDetails(patchDetails []resultPatchDetail) []workspaceDeliverableFile {
	if len(patchDetails) == 0 {
		return nil
	}
	files := make([]workspaceDeliverableFile, 0, len(patchDetails))
	seen := map[string]struct{}{}
	for _, patch := range patchDetails {
		target := strings.TrimSpace(patch.TargetRef)
		if target == "" {
			continue
		}
		target = filepath.ToSlash(filepath.Clean(target))
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		size := int64(len(strings.TrimSpace(patch.Content)))
		if size == 0 {
			size = int64(len(strings.TrimSpace(patch.Diff)))
		}
		files = append(files, workspaceDeliverableFile{
			Path:     target,
			Size:     size,
			Category: artifactCategoryForWorkspaceFile(target),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

func writePatchEvidenceSection(builder *strings.Builder, patchDetails []resultPatchDetail) {
	if builder == nil || len(patchDetails) == 0 {
		return
	}
	builder.WriteString("\n## Code Change Evidence\n\n")
	builder.WriteString("| Patch | Type | Target | State | Summary |\n")
	builder.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, patch := range patchDetails {
		builder.WriteString(fmt.Sprintf(
			"| `%s` | %s | `%s` | %s | %s |\n",
			markdownInlineCode(string(patch.ID)),
			markdownTableCell(patch.PatchType),
			markdownInlineCode(patch.TargetRef),
			markdownTableCell(string(patch.State)),
			markdownTableCell(patch.Summary),
		))
	}
	if hasPatchDeliverableContent(patchDetails) {
		builder.WriteString("\n- Full change content is stored in the Code Change Diff artifact.\n")
	}
}

func expectedCompletionArtifactRefs(run *runModel.Run, patchDetails []resultPatchDetail) []common.ArtifactID {
	if run == nil || run.ID == "" {
		return nil
	}
	ret := []common.ArtifactID{
		common.ArtifactID(fmt.Sprintf("%s/artifact/result-summary", run.ID)),
		common.ArtifactID(fmt.Sprintf("%s/artifact/deliverable-manifest", run.ID)),
	}
	if hasPatchDeliverableContent(patchDetails) {
		ret = append(ret, patchDiffArtifactID(run))
	}
	return ret
}

func patchDiffArtifactID(run *runModel.Run) common.ArtifactID {
	if run == nil {
		return ""
	}
	return common.ArtifactID(fmt.Sprintf("%s/artifact/code-change-diff", run.ID))
}

func hasPatchDeliverableContent(patchDetails []resultPatchDetail) bool {
	for _, patch := range patchDetails {
		if strings.TrimSpace(patch.Diff) != "" || strings.TrimSpace(patch.Content) != "" {
			return true
		}
	}
	return false
}

func patchDiffArtifactContent(patchDetails []resultPatchDetail) string {
	var builder strings.Builder
	builder.WriteString("# Code Change Diff\n")
	for _, patch := range patchDetails {
		if strings.TrimSpace(patch.Diff) == "" && strings.TrimSpace(patch.Content) == "" {
			continue
		}
		builder.WriteString("\n## ")
		builder.WriteString(markdownHeadingText(firstNonEmptyOrchestratorString(patch.TargetRef, string(patch.ID))))
		builder.WriteString("\n\n")
		builder.WriteString(fmt.Sprintf("- Patch: `%s`\n", markdownInlineCode(string(patch.ID))))
		builder.WriteString(fmt.Sprintf("- Type: `%s`\n", markdownInlineCode(patch.PatchType)))
		builder.WriteString(fmt.Sprintf("- Target: `%s`\n", markdownInlineCode(patch.TargetRef)))
		builder.WriteString(fmt.Sprintf("- State: `%s`\n", markdownInlineCode(string(patch.State))))
		if patch.Summary != "" {
			builder.WriteString(fmt.Sprintf("- Summary: %s\n", patch.Summary))
		}
		if strings.TrimSpace(patch.Diff) != "" {
			builder.WriteString("\n```diff\n")
			builder.WriteString(strings.TrimRight(patch.Diff, "\n"))
			builder.WriteString("\n```\n")
			continue
		}
		builder.WriteString("\n```text\n")
		builder.WriteString(strings.TrimRight(patch.Content, "\n"))
		builder.WriteString("\n```\n")
	}
	return strings.TrimSpace(builder.String())
}

type workspaceDeliverableFile struct {
	Path       string
	Size       int64
	Executable bool
	Category   string
}

func workspaceManifestFiles(workingDirectory string) ([]workspaceDeliverableFile, error) {
	workingDirectory = strings.TrimSpace(workingDirectory)
	if workingDirectory == "" {
		return nil, common.ErrInvalidArgument
	}
	root, err := filepath.Abs(workingDirectory)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, common.ErrInvalidArgument
	}
	const maxManifestFiles = 200
	files := make([]workspaceDeliverableFile, 0)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if shouldSkipWorkspaceManifestPath(rel, entry) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if len(files) >= maxManifestFiles {
			return filepath.SkipAll
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files = append(files, workspaceDeliverableFile{
			Path:       filepath.ToSlash(rel),
			Size:       info.Size(),
			Executable: info.Mode().Perm()&0o111 != 0,
			Category:   artifactCategoryForWorkspaceFile(rel),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func shouldSkipWorkspaceManifestPath(rel string, entry os.DirEntry) bool {
	normalized := strings.ToLower(filepath.ToSlash(strings.TrimSpace(rel)))
	if normalized == "." || normalized == "" {
		return false
	}
	parts := strings.Split(normalized, "/")
	for _, part := range parts {
		switch part {
		case ".git", ".workorch-data", "node_modules", "vendor", "dist", "build", ".next", ".cache", "coverage", "tmp", "temp":
			return true
		}
	}
	if entry != nil && entry.IsDir() {
		base := filepath.Base(normalized)
		if strings.HasPrefix(base, ".git") {
			return true
		}
	}
	return false
}

func artifactCategoryForWorkspaceFile(path string) string {
	normalized := strings.ToLower(filepath.ToSlash(path))
	base := pathBaseLower(normalized)
	switch {
	case strings.Contains(normalized, "/testdata/"),
		strings.HasSuffix(base, "_test.go"),
		strings.HasSuffix(base, ".test.js"),
		strings.HasSuffix(base, ".spec.js"),
		strings.Contains(base, "test-report"),
		strings.Contains(base, "coverage"):
		return resultcommon.ArtifactCategoryTestReport
	case strings.HasSuffix(base, ".md"),
		strings.HasSuffix(base, ".adoc"),
		strings.HasSuffix(base, ".rst"),
		strings.HasPrefix(base, "readme"),
		strings.HasPrefix(base, "docs"):
		return resultcommon.ArtifactCategoryDocumentation
	case strings.HasSuffix(base, ".toml"),
		strings.HasSuffix(base, ".yaml"),
		strings.HasSuffix(base, ".yml"),
		strings.HasSuffix(base, ".json"),
		strings.HasSuffix(base, ".env"),
		strings.Contains(base, "config"),
		strings.Contains(base, "lock"):
		return resultcommon.ArtifactCategoryConfiguration
	default:
		return resultcommon.ArtifactCategorySource
	}
}

const (
	maxLLMContextStringBytes = 4096
	maxLLMContextSliceItems  = 20
	maxLLMContextMapEntries  = 32
)

func compactStepInputForLLMContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := compactLLMContextMap(input, 0)
	delete(ret, stepInputKeyExecutionContextPackage)
	delete(ret, stepInputKeyWorkspaceFileManifest)
	return ret
}

func compactStepOutputForLLMContext(output map[string]any) map[string]any {
	if len(output) == 0 {
		return nil
	}
	return compactLLMContextMap(output, 0)
}

func compactWorkspaceFileManifest(manifest []map[string]any) []map[string]any {
	if len(manifest) == 0 {
		return nil
	}
	limit := min(len(manifest), maxLLMContextSliceItems)
	ret := make([]map[string]any, 0, limit)
	for idx := 0; idx < limit; idx++ {
		ret = append(ret, compactLLMContextMap(manifest[idx], 0))
	}
	return ret
}

func compactExecutionGraphForLLMContext(graph runtimemodel.BlockExecutionGraph) runtimemodel.BlockExecutionGraph {
	if len(graph.Nodes) == 0 {
		return graph
	}
	nodes := make([]runtimemodel.BlockExecutionNode, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		compactNode := runtimemodel.BlockExecutionNode{
			ID:   strings.TrimSpace(node.ID),
			Kind: strings.TrimSpace(node.Kind),
		}
		if len(node.DependsOn) > 0 {
			limit := min(len(node.DependsOn), 10)
			compactNode.DependsOn = append([]string(nil), node.DependsOn[:limit]...)
		}
		if branch := strings.TrimSpace(node.Branch); branch != "" {
			compactNode.Branch = truncateLLMContextString(branch)
		}
		if len(node.Input) > 0 {
			compactNode.Input = compactLLMContextMap(node.Input, 0)
		}
		nodes = append(nodes, compactNode)
	}
	return runtimemodel.BlockExecutionGraph{Nodes: nodes}
}

func compactLLMContextMap(input map[string]any, depth int) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]any, min(len(input), maxLLMContextMapEntries))
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if len(ret) >= maxLLMContextMapEntries {
			break
		}
		if shouldSkipLLMContextKey(key) {
			continue
		}
		ret[key] = compactLLMContextValue(input[key], depth+1)
	}
	return ret
}

func compactLLMContextValue(value any, depth int) any {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return truncateLLMContextString(v)
	case map[string]any:
		if depth >= 3 {
			return compactLLMContextLeafSummary(v)
		}
		return compactLLMContextMap(v, depth)
	case []map[string]any:
		if depth >= 3 {
			return compactLLMContextLeafSummary(map[string]any{"items": len(v)})
		}
		limit := min(len(v), maxLLMContextSliceItems)
		ret := make([]any, 0, limit)
		for idx := 0; idx < limit; idx++ {
			ret = append(ret, compactLLMContextMap(v[idx], depth))
		}
		return ret
	case []any:
		if depth >= 3 {
			return compactLLMContextLeafSummary(map[string]any{"items": len(v)})
		}
		limit := min(len(v), maxLLMContextSliceItems)
		ret := make([]any, 0, limit)
		for idx := 0; idx < limit; idx++ {
			ret = append(ret, compactLLMContextValue(v[idx], depth))
		}
		return ret
	case []string:
		limit := min(len(v), maxLLMContextSliceItems)
		ret := make([]string, 0, limit)
		for idx := 0; idx < limit; idx++ {
			ret = append(ret, truncateLLMContextString(v[idx]))
		}
		return ret
	default:
		return v
	}
}

func compactLLMContextLeafSummary(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]any, min(len(input), maxLLMContextMapEntries))
	for key, value := range input {
		if shouldSkipLLMContextKey(key) {
			continue
		}
		switch v := value.(type) {
		case string:
			ret[key] = truncateLLMContextString(v)
		case []string:
			ret[key] = len(v)
		case []any:
			ret[key] = len(v)
		case []map[string]any:
			ret[key] = len(v)
		case map[string]any:
			ret[key] = len(v)
		default:
			ret[key] = v
		}
		if len(ret) >= maxLLMContextMapEntries {
			break
		}
	}
	return ret
}

func shouldSkipLLMContextKey(key string) bool {
	switch key {
	case stepInputKeyExecutionContextPackage, stepInputKeyWorkspaceFileManifest:
		return true
	default:
		return false
	}
}

func truncateLLMContextString(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxLLMContextStringBytes {
		return value
	}
	return value[:maxLLMContextStringBytes] + "..."
}

func artifactPurposeForCategory(category string) string {
	switch category {
	case resultcommon.ArtifactCategoryConfiguration:
		return "Configuration required to run or integrate the deliverable."
	case resultcommon.ArtifactCategoryDocumentation:
		return "Documentation or usage guidance for acceptance review."
	case resultcommon.ArtifactCategoryTestReport:
		return "Verification material related to the DoD."
	case resultcommon.ArtifactCategoryRuntimeEvidence:
		return "Runtime evidence produced during execution."
	case resultcommon.ArtifactCategoryFinalSummary:
		return "Final user-facing acceptance summary."
	default:
		return "Source or implementation artifact for the accepted outcome."
	}
}

func pathBaseLower(path string) string {
	path = filepath.ToSlash(path)
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

func (o *RuntimeOrchestrator) publishResultAccepted(ctx context.Context, item *resultmodel.Result) error {
	if item == nil || o.eventHub == nil {
		return nil
	}
	artifactRefs := make([]string, 0, len(item.ArtifactRefs))
	for _, ref := range item.ArtifactRefs {
		if ref != "" {
			artifactRefs = append(artifactRefs, string(ref))
		}
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		ID:           fmt.Sprintf("%s/event/accepted", item.ID),
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicResultAccepted,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "result",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		Payload: events.ResultPayload(
			item.GoalID,
			item.DoDID,
			item.State,
			item.Title,
			item.Summary,
			item.SourceType,
			item.SourceRef,
			item.EvidenceRefs,
			artifactRefs,
		),
	})
}

func (o *RuntimeOrchestrator) publishArtifactCreated(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil || o.eventHub == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		ID:           fmt.Sprintf("%s/event/created", item.ID),
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicArtifactCreated,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "artifact",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		Payload: events.ArtifactPayload(
			item.ResultID,
			item.StepID,
			item.ToolCallID,
			item.PatchID,
			item.ValidationResultID,
			item.ArtifactType,
			item.Name,
			item.Summary,
			item.URI,
			item.ContentType,
			item.State,
			item.EvidenceRefs,
		),
	})
}

func (o *RuntimeOrchestrator) publishMemoryChanged(ctx context.Context, topic string, item *memorymodel.MemoryItem) error {
	if item == nil || o.eventHub == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        topic,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "memory",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		Payload:      events.MemoryPayload(item.MemoryType, item.Summary, item.Content, item.SourceType, item.SourceRef, item.Tags, item.Importance, item.Confidence, item.Visibility),
	})
}

func doDAssessmentEvidenceRefs(stepID common.StepID, assessment map[string]any) []string {
	seen := map[string]struct{}{}
	ret := []string{}
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return
		}
		if _, exists := seen[ref]; exists {
			return
		}
		seen[ref] = struct{}{}
		ret = append(ret, ref)
	}
	add(string(stepID))
	items, ok := doDAssessmentItemMaps(assessment["items"])
	if ok {
		for _, item := range items {
			for _, ref := range evidenceRefsFromValue(item["evidenceRefs"]) {
				add(ref)
			}
		}
	}
	sort.Strings(ret)
	return ret
}

func resultDoDAssessmentFromMap(assessment map[string]any) resultmodel.DoDAssessment {
	if assessment == nil {
		return resultmodel.DoDAssessment{}
	}
	summary, _ := assessment["summary"].(string)
	satisfied, _ := assessment["satisfied"].(bool)
	ret := resultmodel.DoDAssessment{
		Satisfied: satisfied,
		Summary:   strings.TrimSpace(summary),
	}
	items, ok := doDAssessmentItemMaps(assessment["items"])
	if !ok {
		return ret
	}
	ret.Items = make([]resultmodel.DoDAssessmentItem, 0, len(items))
	for _, item := range items {
		name, _ := item["item"].(string)
		state, _ := item["state"].(string)
		failureReason, _ := item["failureReason"].(string)
		sourceNote, _ := item["sourceNote"].(string)
		ret.Items = append(ret.Items, resultmodel.DoDAssessmentItem{
			Item:          strings.TrimSpace(name),
			State:         strings.TrimSpace(state),
			EvidenceRefs:  evidenceRefsFromValue(item["evidenceRefs"]),
			FailureReason: strings.TrimSpace(failureReason),
			Confidence:    floatFromValue(item["confidence"]),
			SourceNote:    strings.TrimSpace(sourceNote),
		})
	}
	return ret
}

func floatFromValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		ret, _ := typed.Float64()
		return ret
	default:
		return 0
	}
}

func int64FromValue(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	case json.Number:
		ret, _ := typed.Int64()
		return ret
	default:
		return 0
	}
}

func boolFromValue(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func (o *RuntimeOrchestrator) recordRunCheckpoint(ctx context.Context, run *runModel.Run) error {
	return o.recordRunCheckpointAtStep(ctx, run, "")
}

func (o *RuntimeOrchestrator) recordRunCheckpointAtStep(ctx context.Context, run *runModel.Run, currentStepID common.StepID) error {
	if run == nil || o.evidence.RuntimeCheckpoints == nil {
		return nil
	}
	if currentStepID == "" {
		currentStepID = o.latestStepID(ctx, run.ID)
	}
	checkpointType := common.CheckpointTypeRunState
	if run.BlockingTicketID != "" || run.BlockingApprovalID != "" {
		checkpointType = common.CheckpointTypeBlocked
	}
	return o.createRuntimeCheckpoint(ctx, run, currentStepID, checkpointType, map[string]any{
		"event": "run_state_changed",
		"state": string(run.State),
	})
}

func (o *RuntimeOrchestrator) recordStepCheckpoint(ctx context.Context, run *runModel.Run, step *runModel.Step, event string) error {
	if run == nil || step == nil || o.evidence.RuntimeCheckpoints == nil {
		return nil
	}
	data := map[string]any{
		"event":     event,
		"stepState": string(step.State),
		"stepKind":  step.Kind,
	}
	if class := stringFromMap(step.Output, failureOutputKeyClass); class != "" {
		data[failureOutputKeyClass] = class
	}
	if signature := stringFromMap(step.Output, failureOutputKeySignature); signature != "" {
		data[failureOutputKeySignature] = signature
	}
	if nodeID := graphNodeID(step); nodeID != "" {
		data[stepInputKeyGraphNodeID] = nodeID
	}
	if branch := graphBranch(step); branch != "" {
		data[stepOutputKeyGraphBranch] = branch
	}
	if completed := graphCompletedNodesFromStep(step); len(completed) > 0 {
		data[stepInputKeyGraphCompletedNodes] = completed
	}
	return o.createRuntimeCheckpoint(ctx, run, step.ID, common.CheckpointTypeRunState, data)
}

func (o *RuntimeOrchestrator) createRuntimeCheckpoint(
	ctx context.Context,
	run *runModel.Run,
	currentStepID common.StepID,
	checkpointType common.CheckpointType,
	data map[string]any,
) error {
	now := runtimeEvidenceTimestamp()
	item := &runModel.RuntimeCheckpoint{
		ID:                 common.CheckpointID(fmt.Sprintf("%s/checkpoint/%d", run.ID, time.Now().UTC().UnixNano())),
		WorkspaceID:        run.WorkspaceID,
		RunID:              run.ID,
		CheckpointType:     checkpointType,
		CurrentStepID:      currentStepID,
		RunStateSnapshot:   run.State,
		BlockingTicketID:   run.BlockingTicketID,
		BlockingApprovalID: run.BlockingApprovalID,
		ThreadRef:          "main",
		OccurredAt:         now,
		Data:               data,
	}
	if err := o.evidence.RuntimeCheckpoints.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return err
		}
		return o.evidence.RuntimeCheckpoints.Update(ctx, item)
	}
	return nil
}

func (o *RuntimeOrchestrator) latestStepID(ctx context.Context, runID common.RunID) common.StepID {
	if o == nil || o.steps == nil || runID == "" {
		return ""
	}
	steps, err := o.steps.ListByRun(ctx, runID)
	if err != nil || len(steps) == 0 {
		return ""
	}
	step := terminalExecutionStep(steps)
	if step == nil {
		return ""
	}
	return step.ID
}

func runtimeEvidenceTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
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

func decisionResolutionInput(ticket *decisionmodel.Ticket, resolution *decisionmodel.Resolution) map[string]any {
	if ticket == nil || resolution == nil {
		return nil
	}
	return map[string]any{
		stepInputKeySoulStage: soulStageDecision,
		"decisionResolution": map[string]any{
			"ticketId":          string(ticket.ID),
			"stepId":            string(ticket.StepID),
			"decisionKind":      ticket.DecisionKind,
			"question":          ticket.Question,
			"reason":            ticket.Reason,
			"resolutionId":      string(resolution.ID),
			"resolutionType":    resolution.Type,
			"resolutionPayload": cloneStepPayload(resolution.Payload),
		},
	}
}

func approvalResolutionInput(approval *approvalmodel.Approval) map[string]any {
	if approval == nil {
		return nil
	}
	return map[string]any{
		stepInputKeySoulStage: soulStageApproval,
		"approvalResolution": map[string]any{
			"approvalId": string(approval.ID),
			"stepId":     string(approval.StepID),
			"reason":     approval.Reason,
			"action":     approval.Action,
			"context":    cloneStepPayload(approval.Context),
			"state":      string(approval.State),
		},
	}
}

func (o *RuntimeOrchestrator) hasGraphContinuation(ctx context.Context, run *runModel.Run, step *runModel.Step) bool {
	return len(o.readyGraphStepSpecs(ctx, run, step)) > 0
}

func (o *RuntimeOrchestrator) nextGraphStepSpec(
	ctx context.Context,
	run *runModel.Run,
	last *runModel.Step,
) (common.StepID, string, map[string]any, bool) {
	specs := o.readyGraphStepSpecs(ctx, run, last)
	if len(specs) == 0 {
		return "", "", nil, false
	}
	return specs[0].ID, specs[0].Kind, specs[0].Input, true
}

func (o *RuntimeOrchestrator) readyGraphStepSpecs(
	ctx context.Context,
	run *runModel.Run,
	last *runModel.Step,
) []stepDispatchSpec {
	specs, _ := o.readyGraphStepSpecsWithSteps(ctx, run, last)
	return specs
}

func (o *RuntimeOrchestrator) readyGraphStepSpecsWithSteps(
	ctx context.Context,
	run *runModel.Run,
	last *runModel.Step,
) ([]stepDispatchSpec, []*runModel.Step) {
	if o == nil || run == nil || last == nil {
		return nil, nil
	}
	graph, ok := runtimeGraphSpecFromStep(last)
	if !ok || len(graph.Nodes) == 0 {
		return nil, nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil, nil
	}
	return o.readyGraphStepSpecsFromSteps(ctx, run, last, graph, steps), steps
}

func (o *RuntimeOrchestrator) readyGraphStepSpecsFromSteps(
	ctx context.Context,
	run *runModel.Run,
	last *runModel.Step,
	graph runtimemodel.BlockExecutionGraph,
	steps []*runModel.Step,
) []stepDispatchSpec {
	rootStepID := graphDispatchRootStepID(last)
	completed := completedGraphNodes(steps, rootStepID)
	started := startedGraphNodes(steps, rootStepID)
	branch := selectedGraphBranch(steps)
	ret := make([]stepDispatchSpec, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if node.ID == "" || completed[node.ID] || started[node.ID] {
			continue
		}
		if node.Branch != "" && node.Branch != branch {
			continue
		}
		if !graphNodeDependenciesSatisfied(node, completed) {
			continue
		}
		input := nextGraphStepInput(run, last, graph, node, completed, branch, steps, rootStepID)
		input[stepInputKeyGraphRootStepID] = string(rootStepID)
		ret = append(ret, stepDispatchSpec{
			ID:    graphStepIDForRootFromSteps(steps, run.ID, rootStepID, node.ID),
			Kind:  node.Kind,
			Input: input,
		})
	}
	return ret
}

func (o *RuntimeOrchestrator) dispatchGraphStepBatch(ctx context.Context, run *runModel.Run, specs []stepDispatchSpec) error {
	return o.dispatchGraphStepBatchWithWorkspace(ctx, run, specs, nil)
}

func (o *RuntimeOrchestrator) dispatchGraphStepBatchWithWorkspace(ctx context.Context, run *runModel.Run, specs []stepDispatchSpec, workspace *workspaceModel.Workspace) error {
	if len(specs) == 0 {
		return common.ErrConflict
	}
	if len(specs) == 1 {
		return o.dispatchStepWithWorkspace(ctx, run, specs[0].ID, specs[0].Kind, specs[0].Input, workspace)
	}

	errCh := make(chan error, len(specs))
	var wg sync.WaitGroup
	for _, spec := range specs {
		spec := spec
		spec.Input = cloneStepPayload(spec.Input)
		if spec.Input == nil {
			spec.Input = map[string]any{}
		}
		spec.Input[stepInputKeyGraphBatch] = true
		runCopy := *run
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- o.dispatchStepWithWorkspace(ctx, &runCopy, spec.ID, spec.Kind, spec.Input, workspace)
		}()
	}
	wg.Wait()
	close(errCh)
	var firstErr error
	for err := range errCh {
		if err != nil && !errors.Is(err, common.ErrConflict) {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	currentRun, err := o.runs.Get(ctx, run.ID)
	if err != nil {
		return err
	}
	if currentRun.State != common.RunStateRunning {
		return nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return err
	}
	if firstErr != nil {
		if failed := latestFailedRepairableGraphStep(steps); failed != nil {
			return o.dispatchRuntimeRepairFollowUp(ctx, currentRun, failed, firstErr)
		}
		return o.FailRun(ctx, currentRun.ID, firstErr.Error())
	}
	last := latestCompletedGraphStep(steps)
	if last == nil {
		return common.ErrConflict
	}
	if graph, ok := runtimeGraphSpecFromStep(last); ok {
		if nextSpecs := o.readyGraphStepSpecsFromSteps(ctx, currentRun, last, graph, steps); len(nextSpecs) > 0 {
			return o.dispatchGraphStepBatchWithWorkspace(ctx, currentRun, nextSpecs, workspace)
		}
	}
	if shouldDispatchGraphFollowUp(currentRun, last, last.Output) {
		return o.dispatchStepWithWorkspace(ctx, currentRun, graphFollowUpStepID(currentRun.ID, last), initialStepKind(currentRun), o.graphFollowUpStepInputFromSteps(currentRun, last, steps), workspace)
	}
	if shouldDispatchLLMCompletionFollowUp(currentRun, last, last.Output) {
		return o.dispatchStepWithWorkspace(ctx, currentRun, llmCompletionFollowUpStepID(currentRun.ID, last), initialStepKind(currentRun), llmCompletionFollowUpStepInput(currentRun, last), workspace)
	}
	assessment, summary, err := validatedCompletionDoDAssessment(currentRun, last.Output)
	if err != nil {
		last.State = common.StepStateFailed
		last.Output = runtimeFailureOutput(last.Output, err)
		_ = o.steps.Update(ctx, last)
		return o.FailRun(ctx, currentRun.ID, err.Error())
	}
	if err := o.recordCompletionEvidenceFromValidatedDoDAssessment(ctx, currentRun, last, assessment, summary, workspace); err != nil {
		return err
	}
	return o.validateAndCompleteRunWithWorkspace(ctx, currentRun, last.ID, workspace)
}

func runtimeGraphSpecFromStep(step *runModel.Step) (runtimemodel.BlockExecutionGraph, bool) {
	if step == nil {
		return runtimemodel.BlockExecutionGraph{}, false
	}
	if graph, ok := runtimeGraphSpecFromValue(step.Output[stepOutputKeyExecutionGraph]); ok {
		return graph, true
	}
	return runtimeGraphSpecFromValue(step.Input[stepOutputKeyExecutionGraph])
}

func runtimeGraphSpecFromValue(value any) (runtimemodel.BlockExecutionGraph, bool) {
	switch typed := value.(type) {
	case runtimemodel.BlockExecutionGraph:
		return typed, len(typed.Nodes) > 0
	case map[string]any:
		nodes, ok := runtimeGraphNodesFromValue(typed["nodes"])
		if !ok {
			return runtimemodel.BlockExecutionGraph{}, false
		}
		return runtimemodel.BlockExecutionGraph{Nodes: nodes}, true
	default:
		return runtimemodel.BlockExecutionGraph{}, false
	}
}

func runtimeGraphNodesFromValue(value any) ([]runtimemodel.BlockExecutionNode, bool) {
	switch typed := value.(type) {
	case []runtimemodel.BlockExecutionNode:
		return append([]runtimemodel.BlockExecutionNode(nil), typed...), len(typed) > 0
	case []map[string]any:
		ret := make([]runtimemodel.BlockExecutionNode, 0, len(typed))
		for _, item := range typed {
			node, ok := runtimeGraphNodeFromMap(item)
			if !ok {
				return nil, false
			}
			ret = append(ret, node)
		}
		return ret, len(ret) > 0
	case []any:
		ret := make([]runtimemodel.BlockExecutionNode, 0, len(typed))
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			node, ok := runtimeGraphNodeFromMap(mapped)
			if !ok {
				return nil, false
			}
			ret = append(ret, node)
		}
		return ret, len(ret) > 0
	default:
		return nil, false
	}
}

func runtimeGraphNodeFromMap(item map[string]any) (runtimemodel.BlockExecutionNode, bool) {
	id, _ := item["id"].(string)
	kind, _ := item["kind"].(string)
	branch, _ := item["branch"].(string)
	id = strings.TrimSpace(id)
	kind = strings.TrimSpace(kind)
	if id == "" || kind == "" {
		return runtimemodel.BlockExecutionNode{}, false
	}
	return runtimemodel.BlockExecutionNode{
		ID:        id,
		Kind:      kind,
		DependsOn: stringSliceFromValue(item["dependsOn"]),
		Branch:    strings.TrimSpace(branch),
		Input:     mapFromValue(item["input"]),
	}, true
}

func stringSliceFromValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil
			}
			text = strings.TrimSpace(text)
			if text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func mapFromValue(value any) map[string]any {
	typed, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return cloneStepPayload(typed)
}

func completedGraphNodes(steps []*runModel.Step, rootStepID common.StepID) map[string]bool {
	ret := map[string]bool{}
	for _, step := range steps {
		if step == nil || step.State != common.StepStateCompleted {
			continue
		}
		if !sameGraphRoot(step, rootStepID) {
			continue
		}
		nodeID, _ := step.Input[stepInputKeyGraphNodeID].(string)
		nodeID = strings.TrimSpace(nodeID)
		if nodeID != "" {
			ret[nodeID] = true
		}
	}
	return ret
}

func startedGraphNodes(steps []*runModel.Step, rootStepID common.StepID) map[string]bool {
	ret := map[string]bool{}
	for _, step := range steps {
		if !sameGraphRoot(step, rootStepID) {
			continue
		}
		nodeID := graphNodeID(step)
		if nodeID != "" {
			ret[nodeID] = true
		}
	}
	return ret
}

func selectedGraphBranch(steps []*runModel.Step) string {
	branch := ""
	for _, step := range steps {
		if step == nil || step.State != common.StepStateCompleted {
			continue
		}
		if value, _ := step.Output[stepOutputKeyGraphBranch].(string); strings.TrimSpace(value) != "" {
			branch = strings.TrimSpace(value)
			continue
		}
		if value, _ := step.Input[stepOutputKeyGraphBranch].(string); strings.TrimSpace(value) != "" {
			branch = strings.TrimSpace(value)
		}
	}
	return branch
}

func sameGraphRoot(step *runModel.Step, rootStepID common.StepID) bool {
	if step == nil || rootStepID == "" {
		return false
	}
	return graphRootStepID(step) == rootStepID
}

func graphRootStepID(step *runModel.Step) common.StepID {
	if step == nil {
		return ""
	}
	if step.Input != nil {
		root, _ := step.Input[stepInputKeyGraphRootStepID].(string)
		if strings.TrimSpace(root) != "" {
			return common.StepID(strings.TrimSpace(root))
		}
	}
	if graphNodeID(step) == "" {
		return step.ID
	}
	if previousStepID := previousExecutionStepID(step); previousStepID != "" {
		return previousStepID
	}
	return step.ID
}

func graphDispatchRootStepID(step *runModel.Step) common.StepID {
	if step == nil {
		return ""
	}
	if graphNodeID(step) == "" {
		if _, ok := runtimeGraphSpecFromValue(step.Output[stepOutputKeyExecutionGraph]); ok {
			return step.ID
		}
	}
	return graphRootStepID(step)
}

func graphNodeDependenciesSatisfied(node runtimemodel.BlockExecutionNode, completed map[string]bool) bool {
	for _, dependency := range node.DependsOn {
		if !completed[dependency] {
			return false
		}
	}
	return true
}

func nextGraphStepInput(
	run *runModel.Run,
	last *runModel.Step,
	graph runtimemodel.BlockExecutionGraph,
	node runtimemodel.BlockExecutionNode,
	completed map[string]bool,
	branch string,
	steps []*runModel.Step,
	rootStepID common.StepID,
) map[string]any {
	input := nextStepInput(run, last.ID)
	renderedNodeInput, _ := renderRuntimeTemplateValue(node.Input, graphRuntimeTemplateContext(steps, rootStepID)).(map[string]any)
	for key, value := range renderedNodeInput {
		input[key] = value
	}
	input[stepInputKeySoulStage] = soulStageExecution
	input[stepOutputKeyExecutionGraph] = graph
	input[stepInputKeyGraphNodeID] = node.ID
	input[stepInputKeyGraphRootStepID] = string(graphDispatchRootStepID(last))
	input[stepInputKeyGraphPreviousNodeID] = graphNodeID(last)
	input[stepInputKeyGraphCompletedNodes] = completedGraphNodeList(completed)
	if branch != "" {
		input[stepOutputKeyGraphBranch] = branch
	}
	return input
}

var runtimeTemplatePattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_-]+)\.([A-Za-z0-9_.-]+)\s*\}\}`)

func graphRuntimeTemplateContext(steps []*runModel.Step, rootStepID common.StepID) map[string]map[string]any {
	ret := make(map[string]map[string]any)
	for _, step := range steps {
		if step == nil || step.State != common.StepStateCompleted {
			continue
		}
		if rootStepID != "" && !sameGraphRoot(step, rootStepID) {
			continue
		}
		nodeID := graphNodeID(step)
		if nodeID == "" || step.Output == nil {
			continue
		}
		ret[nodeID] = cloneStepPayload(step.Output)
	}
	return ret
}

func renderRuntimeTemplateValue(value any, context map[string]map[string]any) any {
	switch typed := value.(type) {
	case map[string]any:
		ret := make(map[string]any, len(typed))
		for key, item := range typed {
			ret[key] = renderRuntimeTemplateValue(item, context)
		}
		return ret
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, renderRuntimeTemplateValue(item, context))
		}
		return ret
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			rendered, _ := renderRuntimeTemplateValue(item, context).(map[string]any)
			ret = append(ret, rendered)
		}
		return ret
	case []string:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, renderRuntimeTemplateString(item, context))
		}
		return ret
	case string:
		return renderRuntimeTemplateString(typed, context)
	default:
		return value
	}
}

func renderRuntimeTemplateString(value string, context map[string]map[string]any) string {
	if strings.TrimSpace(value) == "" || len(context) == 0 {
		return value
	}
	return runtimeTemplatePattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := runtimeTemplatePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		output := context[parts[1]]
		if output == nil {
			return match
		}
		resolved, ok := nestedMapValue(output, strings.Split(parts[2], "."))
		if !ok {
			return match
		}
		return evidenceValueString(resolved)
	})
}

func nestedMapValue(input map[string]any, path []string) (any, bool) {
	if len(path) == 0 {
		return nil, false
	}
	var current any = input
	for _, part := range path {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		mapped, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = mapped[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func graphStepID(runID common.RunID, nodeID string) common.StepID {
	return common.StepID(fmt.Sprintf("%s/block/%s", runID, nodeID))
}

func graphStepIDForRoot(ctx context.Context, steps repository.StepRepository, runID common.RunID, rootStepID common.StepID, nodeID string) common.StepID {
	base := graphStepID(runID, nodeID)
	if steps == nil || rootStepID == "" {
		return base
	}
	existing, err := steps.Get(ctx, base)
	if errors.Is(err, common.ErrNotFound) || err != nil || existing == nil || graphRootStepID(existing) == rootStepID {
		return base
	}
	rootSuffix := runtimeChildID(rootStepID, nodeID)
	if rootSuffix == "" {
		return base
	}
	return common.StepID(fmt.Sprintf("%s/block/%s", runID, rootSuffix))
}

func graphStepIDForRootFromSteps(steps []*runModel.Step, runID common.RunID, rootStepID common.StepID, nodeID string) common.StepID {
	base := graphStepID(runID, nodeID)
	if rootStepID == "" {
		return base
	}
	for _, step := range steps {
		if step == nil || step.ID != base {
			continue
		}
		if graphRootStepID(step) == rootStepID {
			return base
		}
		rootSuffix := runtimeChildID(rootStepID, nodeID)
		if rootSuffix == "" {
			return base
		}
		return common.StepID(fmt.Sprintf("%s/block/%s", runID, rootSuffix))
	}
	return base
}

func graphNodeID(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	nodeID, _ := step.Input[stepInputKeyGraphNodeID].(string)
	return strings.TrimSpace(nodeID)
}

func graphBranch(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	if branch, _ := step.Output[stepOutputKeyGraphBranch].(string); strings.TrimSpace(branch) != "" {
		return strings.TrimSpace(branch)
	}
	branch, _ := step.Input[stepOutputKeyGraphBranch].(string)
	return strings.TrimSpace(branch)
}

func graphCompletedNodesFromStep(step *runModel.Step) []string {
	if step == nil {
		return nil
	}
	return stringSliceFromValue(step.Input[stepInputKeyGraphCompletedNodes])
}

func graphBatchStep(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	value, _ := step.Input[stepInputKeyGraphBatch].(bool)
	return value
}

func latestCompletedGraphStep(steps []*runModel.Step) *runModel.Step {
	for idx := len(steps) - 1; idx >= 0; idx-- {
		step := steps[idx]
		if step != nil && step.State == common.StepStateCompleted && graphNodeID(step) != "" {
			return step
		}
	}
	return nil
}

func latestFailedRepairableGraphStep(steps []*runModel.Step) *runModel.Step {
	for idx := len(steps) - 1; idx >= 0; idx-- {
		step := steps[idx]
		if step != nil && step.State == common.StepStateFailed && graphNodeID(step) != "" {
			return step
		}
	}
	return nil
}

func completedGraphNodeList(completed map[string]bool) []string {
	ret := make([]string, 0, len(completed))
	for nodeID := range completed {
		ret = append(ret, nodeID)
	}
	sort.Strings(ret)
	return ret
}

func shouldDispatchGraphFollowUp(run *runModel.Run, step *runModel.Step, output map[string]any) bool {
	if !requiresDoDAssessmentCompletionGate(run) || step == nil || output == nil {
		return false
	}
	if graphNodeID(step) == "" {
		return false
	}
	return doDAssessmentFromOutput(output) == nil
}

func shouldDispatchLLMCompletionFollowUp(run *runModel.Run, step *runModel.Step, output map[string]any) bool {
	if !requiresDoDAssessmentCompletionGate(run) || step == nil || output == nil {
		return false
	}
	if graphNodeID(step) != "" {
		return false
	}
	if doDAssessmentFromOutput(output) != nil {
		return false
	}
	if step.Kind != initialStepKind(run) {
		return true
	}
	return hasCompletionFollowUpSideEffect(output)
}

func hasCompletionFollowUpSideEffect(output map[string]any) bool {
	return outputItemCount(output[stepOutputKeyPatches]) > 0 ||
		outputItemCount(output[stepOutputKeyMemoryWrites]) > 0 ||
		outputItemCount(output[stepOutputKeyMemoryRefs]) > 0
}

func outputItemCount(value any) int {
	switch typed := value.(type) {
	case []map[string]any:
		return len(typed)
	case []string:
		return len(typed)
	case []any:
		return len(typed)
	default:
		return 0
	}
}

func boolValue(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func llmCompletionFollowUpStepID(runID common.RunID, step *runModel.Step) common.StepID {
	stepID := runtimeChildID(step.ID, "completion")
	if stepID == "" {
		stepID = "completion"
	}
	return common.StepID(fmt.Sprintf("%s/zz-llm-after-step-%s", runID, stepID))
}

func llmCompletionFollowUpStepInput(run *runModel.Run, step *runModel.Step) map[string]any {
	input := nextStepInput(run, step.ID)
	input[stepInputKeySoulStage] = soulStageValidation
	input["completionFollowUp"] = true
	inheritWorkspaceFileManifest(input, step)
	inheritStableExecutionContextPackage(input, step)
	if step.Kind != "" {
		input["previousStepKind"] = step.Kind
	}
	if step.Output != nil {
		input["previousStepOutput"] = compactStepOutputForLLMContext(step.Output)
	}
	return input
}

func graphFollowUpStepID(runID common.RunID, step *runModel.Step) common.StepID {
	nodeID := runtimeChildID(common.StepID(graphNodeID(step)), "")
	if nodeID == "" {
		nodeID = runtimeChildID(step.ID, "graph")
	}
	if rootStepID := graphRootStepID(step); rootStepID != "" {
		nodeID = runtimeChildID(rootStepID, nodeID)
	}
	return common.StepID(fmt.Sprintf("%s/zz-llm-after-graph-%s", runID, nodeID))
}

func (o *RuntimeOrchestrator) graphFollowUpStepInput(ctx context.Context, run *runModel.Run, step *runModel.Step) map[string]any {
	input := graphFollowUpStepInput(run, step)
	evidence := o.completedGraphEvidence(ctx, run, step)
	return enrichGraphFollowUpInputWithEvidence(input, evidence)
}

func (o *RuntimeOrchestrator) graphFollowUpStepInputFromSteps(run *runModel.Run, step *runModel.Step, steps []*runModel.Step) map[string]any {
	input := graphFollowUpStepInput(run, step)
	evidence := o.completedGraphEvidenceFromSteps(step, steps)
	return enrichGraphFollowUpInputWithEvidence(input, evidence)
}

func enrichGraphFollowUpInputWithEvidence(input map[string]any, evidence []map[string]any) map[string]any {
	if len(evidence) == 0 {
		return input
	}
	input[stepInputKeyGraphEvidence] = evidence
	completed := make([]string, 0, len(evidence))
	for _, item := range evidence {
		nodeID, _ := item["nodeId"].(string)
		nodeID = strings.TrimSpace(nodeID)
		if nodeID != "" {
			completed = append(completed, nodeID)
		}
	}
	if len(completed) > 0 {
		input[stepInputKeyGraphCompletedNodes] = completed
	}
	return input
}

func graphFollowUpStepInput(run *runModel.Run, step *runModel.Step) map[string]any {
	input := nextStepInput(run, step.ID)
	input[stepInputKeySoulStage] = soulStageValidation
	input["graphFollowUp"] = true
	inheritWorkspaceFileManifest(input, step)
	inheritStableExecutionContextPackage(input, step)
	if nodeID := graphNodeID(step); nodeID != "" {
		input[stepInputKeyGraphPreviousNodeID] = nodeID
	}
	if completed := graphCompletedNodesFromStep(step); len(completed) > 0 {
		input[stepInputKeyGraphCompletedNodes] = completed
	}
	if branch := graphBranch(step); branch != "" {
		input[stepOutputKeyGraphBranch] = branch
	}
	if graph, ok := runtimeGraphSpecFromStep(step); ok {
		input[stepOutputKeyExecutionGraph] = compactExecutionGraphForLLMContext(graph)
		input[stepInputKeyGraphRootStepID] = string(graphRootStepID(step))
	}
	return input
}

func (o *RuntimeOrchestrator) completedGraphEvidence(ctx context.Context, run *runModel.Run, anchor *runModel.Step) []map[string]any {
	if o == nil || run == nil || anchor == nil {
		return nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil
	}
	return o.completedGraphEvidenceFromSteps(anchor, steps)
}

func (o *RuntimeOrchestrator) completedGraphEvidenceFromSteps(anchor *runModel.Step, steps []*runModel.Step) []map[string]any {
	graph, ok := runtimeGraphSpecFromStep(anchor)
	if !ok || len(graph.Nodes) == 0 {
		return nil
	}
	completedByNode := map[string]*runModel.Step{}
	for _, step := range steps {
		if step == nil || step.State != common.StepStateCompleted {
			continue
		}
		nodeID := graphNodeID(step)
		if nodeID == "" {
			continue
		}
		completedByNode[nodeID] = step
	}
	evidence := make([]map[string]any, 0, len(completedByNode))
	for _, node := range graph.Nodes {
		step := completedByNode[node.ID]
		if step == nil {
			continue
		}
		evidence = append(evidence, map[string]any{
			"nodeId": node.ID,
			"stepId": string(step.ID),
			"kind":   step.Kind,
			"output": compactStepOutputForLLMContext(step.Output),
		})
	}
	return evidence
}

func nextStepSpec(output map[string]any) (common.StepID, string) {
	if output == nil {
		return "", ""
	}
	nextStep, _ := output[stepOutputKeyNextStep].(string)
	nextKind, _ := output[stepOutputKeyNextKind].(string)
	return common.StepID(nextStep), nextKind
}

func initialStepKind(run *runModel.Run) string {
	if run != nil && strings.TrimSpace(run.InitialStepKind) != "" {
		return strings.TrimSpace(run.InitialStepKind)
	}
	return defaultInitialStepKind
}

func initialStepInput(run *runModel.Run) map[string]any {
	input := nextStepInput(run, "")
	if input == nil {
		return nil
	}
	if initialStepKind(run) == llmWorkspaceExecutionStepKind {
		input[stepInputKeySoulStage] = soulStagePlanning
	}
	return input
}

func nextStepInputFromOutput(run *runModel.Run, previousStep *runModel.Step) map[string]any {
	if previousStep == nil {
		return nextStepInput(run, "")
	}
	input := nextStepInput(run, previousStep.ID)
	inheritWorkspaceFileManifest(input, previousStep)
	inheritStableExecutionContextPackage(input, previousStep)
	if nextInput := mapFromValue(previousStep.Output[stepOutputKeyNextInput]); nextInput != nil {
		for key, value := range nextInput {
			input[key] = value
		}
	}
	return input
}

func inheritWorkspaceFileManifest(input map[string]any, previousStep *runModel.Step) {
	if input == nil || previousStep == nil {
		return
	}
	manifest := mapSliceFromValue(previousStep.Input[stepInputKeyWorkspaceFileManifest])
	if len(manifest) == 0 {
		if pkg := mapFromValue(previousStep.Input[stepInputKeyExecutionContextPackage]); pkg != nil {
			manifest = mapSliceFromValue(pkg[stepInputKeyWorkspaceFileManifest])
		}
	}
	if len(manifest) == 0 {
		return
	}
	input[stepInputKeyWorkspaceFileManifest] = compactWorkspaceFileManifest(manifest)
}

func inheritStableExecutionContextPackage(input map[string]any, previousStep *runModel.Step) {
	if input == nil || previousStep == nil {
		return
	}
	stablePayload := stableExecutionContextPackagePayload(stableExecutionContextPackageFromStep(previousStep))
	if len(stablePayload) == 0 {
		return
	}
	input[stepInputKeyExecutionContextPackage] = stablePayload
}

type runtimeDecisionBlockSpec struct {
	Reason       string
	TicketID     common.TicketID
	DecisionKind string
	Question     string
	Context      map[string]any
}

type runtimeApprovalBlockSpec struct {
	Reason     string
	ApprovalID common.ApprovalID
	Action     string
	Context    map[string]any
}

func decisionBlockSpec(output map[string]any) runtimeDecisionBlockSpec {
	if output == nil {
		return runtimeDecisionBlockSpec{}
	}
	reason, _ := output[stepOutputKeyBlockDecisionReason].(string)
	ticketID, _ := output[stepOutputKeyBlockDecisionTicketID].(string)
	decisionKind, _ := output[stepOutputKeyBlockDecisionKind].(string)
	question, _ := output[stepOutputKeyBlockDecisionQuestion].(string)
	return runtimeDecisionBlockSpec{
		Reason:       reason,
		TicketID:     common.TicketID(ticketID),
		DecisionKind: decisionKind,
		Question:     question,
		Context:      mapFromValue(output[stepOutputKeyBlockDecisionContext]),
	}
}

func approvalBlockSpec(output map[string]any) runtimeApprovalBlockSpec {
	if output == nil {
		return runtimeApprovalBlockSpec{}
	}
	reason, _ := output[stepOutputKeyBlockApprovalReason].(string)
	approvalID, _ := output[stepOutputKeyBlockApprovalID].(string)
	action, _ := output[stepOutputKeyBlockApprovalAction].(string)
	return runtimeApprovalBlockSpec{
		Reason:     reason,
		ApprovalID: common.ApprovalID(approvalID),
		Action:     action,
		Context:    mapFromValue(output[stepOutputKeyBlockApprovalContext]),
	}
}

func runtimeChildID(parent common.StepID, suffix string) string {
	base := strings.TrimSpace(string(parent))
	base = strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(base)
	suffix = strings.TrimSpace(suffix)
	if base == "" {
		return suffix
	}
	if suffix == "" {
		return base
	}
	return base + "-" + suffix
}

func completionDoDAssessmentError(run *runModel.Run, output map[string]any) error {
	_, _, err := validatedCompletionDoDAssessment(run, output)
	return err
}

func validatedCompletionDoDAssessment(run *runModel.Run, output map[string]any) (map[string]any, string, error) {
	if !requiresDoDAssessmentCompletionGate(run) {
		return nil, "", nil
	}
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		return nil, "", fmt.Errorf("%w: dodAssessment is required for llm-driven completion", common.ErrInvalidArgument)
	}
	satisfied, ok := assessment["satisfied"].(bool)
	if !ok {
		return nil, "", fmt.Errorf("%w: dodAssessment.satisfied is required", common.ErrInvalidArgument)
	}
	if !satisfied {
		return nil, "", fmt.Errorf("%w: dodAssessment is not satisfied", common.ErrInvalidArgument)
	}
	summary, _ := assessment["summary"].(string)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil, "", fmt.Errorf("%w: dodAssessment.summary is required for llm-driven completion", common.ErrInvalidArgument)
	}
	if err := validateDoDAssessmentItems(assessment["items"]); err != nil {
		return nil, "", err
	}
	return assessment, summary, nil
}

func requiresDoDAssessmentCompletionGate(run *runModel.Run) bool {
	return run != nil && run.DoDID != "" && initialStepKind(run) == llmWorkspaceExecutionStepKind
}

func doDAssessmentFromOutput(output map[string]any) map[string]any {
	if output == nil {
		return nil
	}
	if assessment := mapFromValue(output[stepOutputKeyDoDAssessment]); assessment != nil {
		return assessment
	}
	decision := mapFromValue(output[stepOutputKeyLLMDecision])
	if decision == nil {
		return nil
	}
	return mapFromValue(decision[stepOutputKeyDoDAssessment])
}

func validateDoDAssessmentItems(value any) error {
	items, ok := doDAssessmentItemMaps(value)
	if !ok || len(items) == 0 {
		return fmt.Errorf("%w: dodAssessment.items are required", common.ErrInvalidArgument)
	}
	for _, item := range items {
		name, _ := item["item"].(string)
		state, _ := item["state"].(string)
		name = strings.TrimSpace(name)
		state = strings.TrimSpace(state)
		if name == "" || state == "" {
			return fmt.Errorf("%w: dodAssessment item and state are required", common.ErrInvalidArgument)
		}
		if state == "satisfied" && len(evidenceRefsFromValue(item["evidenceRefs"])) == 0 {
			return fmt.Errorf("%w: satisfied dodAssessment item requires evidenceRefs", common.ErrInvalidArgument)
		}
	}
	return nil
}

func doDAssessmentItemMaps(value any) ([]map[string]any, bool) {
	switch typed := value.(type) {
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneStepPayload(item))
		}
		return ret, true
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			ret = append(ret, cloneStepPayload(mapped))
		}
		return ret, true
	default:
		return nil, false
	}
}

func evidenceRefsFromValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil
			}
			text = strings.TrimSpace(text)
			if text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func nextStepInput(run *runModel.Run, previousStepID common.StepID) map[string]any {
	input := runExecutionInput(run, previousStepID)
	if input == nil {
		return nil
	}
	if previousStepID == "" {
		delete(input, "previousStepId")
	}
	return input
}

func runExecutionInput(run *runModel.Run, previousStepID common.StepID) map[string]any {
	if run == nil {
		return nil
	}
	input := map[string]any{
		"workspaceId":    string(run.WorkspaceID),
		"previousStepId": string(previousStepID),
	}
	if run.GoalID != "" {
		input["goalId"] = string(run.GoalID)
	}
	if run.DoDID != "" {
		input["doDId"] = string(run.DoDID)
	}
	if len(run.ExecutionContext) > 0 {
		input["executionContext"] = cloneStepPayload(run.ExecutionContext)
	}
	return input
}

func cloneStepPayload(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func (o *RuntimeOrchestrator) createDecisionTicket(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	spec runtimeDecisionBlockSpec,
) error {
	if run == nil || step == nil || spec.TicketID == "" || spec.Reason == "" {
		return common.ErrInvalidArgument
	}
	if o.tickets == nil {
		return common.ErrInvalidArgument
	}

	ticket := &decisionmodel.Ticket{
		ID:           spec.TicketID,
		WorkspaceID:  run.WorkspaceID,
		RunID:        run.ID,
		StepID:       step.ID,
		Reason:       spec.Reason,
		DecisionKind: spec.DecisionKind,
		Question:     spec.Question,
		Context:      cloneStepPayload(spec.Context),
		State:        common.DecisionTicketStateOpen,
	}
	if err := o.tickets.Create(ctx, ticket); err != nil {
		return err
	}
	if o.eventHub == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicTicketCreated,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "decision_ticket",
		SubjectID:    string(ticket.ID),
		WorkspaceID:  string(ticket.WorkspaceID),
		RunID:        string(ticket.RunID),
		StepID:       string(ticket.StepID),
		Payload:      events.DecisionTicketPayload(ticket.Reason, ticket.State),
	})
}

func (o *RuntimeOrchestrator) createApproval(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	approvalID common.ApprovalID,
	reason string,
	action string,
	approvalContext map[string]any,
) error {
	if run == nil || step == nil || approvalID == "" || reason == "" {
		return common.ErrInvalidArgument
	}
	if o.approvals == nil {
		return common.ErrInvalidArgument
	}

	approval := &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		StepID:      step.ID,
		Reason:      reason,
		Action:      action,
		Context:     cloneStepPayload(approvalContext),
		State:       common.ApprovalStatePending,
	}
	if err := o.approvals.Create(ctx, approval); err != nil {
		return err
	}
	if o.eventHub == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicApprovalCreated,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "approval",
		SubjectID:    string(approval.ID),
		WorkspaceID:  string(approval.WorkspaceID),
		RunID:        string(approval.RunID),
		StepID:       string(approval.StepID),
		Payload:      events.ApprovalPayload(approval.Reason, approval.Action, approval.Context, approval.State),
	})
}

func (o *RuntimeOrchestrator) publishWorkspaceStateChanged(ctx context.Context, workspace *workspaceModel.Workspace, runID common.RunID) error {
	if o.eventHub == nil || workspace == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicWorkspaceStateChanged,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "workspace",
		SubjectID:    string(workspace.ID),
		WorkspaceID:  string(workspace.ID),
		RunID:        string(runID),
		Payload:      events.WorkspacePayload(workspace.Title, workspace.Description, workspace.WorkingDirectory, workspace.State, workspace.ActiveRunID),
	})
}

func (o *RuntimeOrchestrator) publishStepState(ctx context.Context, run *runModel.Run, step *runModel.Step, topic string) error {
	if o.eventHub == nil || run == nil || step == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        topic,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "step",
		SubjectID:    string(step.ID),
		WorkspaceID:  string(run.WorkspaceID),
		RunID:        string(run.ID),
		StepID:       string(step.ID),
		Payload:      events.StepPayload(step.Kind, step.State, step.Input, step.Output),
	})
}
