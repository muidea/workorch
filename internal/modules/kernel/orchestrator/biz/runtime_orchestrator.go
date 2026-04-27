package orchestrator

import (
	"context"
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
	stepInputKeyGraphPreviousNodeID    = "graphPreviousNodeId"
	stepInputKeyGraphCompletedNodes    = "graphCompletedNodes"
	stepInputKeyGraphBatch             = "graphBatch"
	stepOutputKeyBlockDecisionReason   = "blockDecisionReason"
	stepOutputKeyBlockDecisionTicketID = "blockDecisionTicketId"
	stepOutputKeyBlockDecisionKind     = "blockDecisionKind"
	stepOutputKeyBlockDecisionQuestion = "blockDecisionQuestion"
	stepOutputKeyBlockDecisionContext  = "blockDecisionContext"
	stepOutputKeyBlockApprovalReason   = "blockApprovalReason"
	stepOutputKeyBlockApprovalID       = "blockApprovalId"
	defaultInitialStepKind             = "legacy/minimal"
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
	ToolCalls          repository.ToolCallRepository
	Patches            repository.PatchRepository
	ValidationResults  repository.ValidationResultRepository
	RuntimeCheckpoints repository.RuntimeCheckpointRepository
	MemoryItems        repository.MemoryItemRepository
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
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}
	if err := o.publishRunStateChanged(ctx, r); err != nil {
		return err
	}
	ws.ActiveRunID = runID
	if err := o.workspaces.Update(ctx, ws); err != nil {
		return err
	}
	if err := o.publishWorkspaceStateChanged(ctx, ws, runID); err != nil {
		return err
	}
	return o.DispatchNextStep(ctx, runID)
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
	if r.State != common.RunStateCreated && r.State != common.RunStateRunning {
		return common.ErrConflict
	}
	r.State = common.RunStateBlockedOnDecision
	r.BlockingTicketID = ticketID
	r.BlockingApprovalID = ""
	r.FailureReason = ""
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}
	return o.publishRunStateChanged(ctx, r)
}

func (o *RuntimeOrchestrator) BlockOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	if runID == "" || approvalID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	if r.State != common.RunStateCreated && r.State != common.RunStateRunning {
		return common.ErrConflict
	}
	r.State = common.RunStateBlockedOnApproval
	r.BlockingApprovalID = approvalID
	r.BlockingTicketID = ""
	r.FailureReason = ""
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}
	return o.publishRunStateChanged(ctx, r)
}

func (o *RuntimeOrchestrator) ResumeRun(ctx context.Context, runID common.RunID) error {
	if runID == "" {
		return common.ErrInvalidArgument
	}
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}

	var resumeAfterStepID common.StepID
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
				r.BlockingTicketID = ""
			case decisioncommon.ResolutionTypeRejectCurrentPath:
				return o.FailRun(ctx, runID, fmt.Sprintf("decision ticket %s rejected", ticket.ID))
			case decisioncommon.ResolutionTypeReplan:
				return o.SupersedeRun(ctx, runID)
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
			r.BlockingApprovalID = ""
		case common.ApprovalStateRejected:
			return o.FailRun(ctx, runID, fmt.Sprintf("approval %s rejected", approval.ID))
		default:
			return common.ErrBlocked
		}
	default:
		return common.ErrConflict
	}

	r.State = common.RunStateRunning
	r.FailureReason = ""
	if err := o.runs.Update(ctx, r); err != nil {
		return err
	}
	if err := o.publishRunStateChanged(ctx, r); err != nil {
		return err
	}
	specs, err := o.nextDispatchSpecsAfterStep(ctx, r, resumeAfterStepID)
	if err != nil {
		return err
	}
	return o.dispatchStepSpecs(ctx, r, specs)
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

	ws, err := o.workspaces.Get(ctx, r.WorkspaceID)
	if err != nil {
		return err
	}
	if ws.ActiveRunID == runID {
		ws.ActiveRunID = ""
		if err := o.workspaces.Update(ctx, ws); err != nil {
			return err
		}
		return o.publishWorkspaceStateChanged(ctx, ws, runID)
	}
	return nil
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
	return o.publishRunStateChanged(ctx, r)
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

func (o *RuntimeOrchestrator) closeRun(ctx context.Context, runID common.RunID, state common.RunState, reason string) error {
	r, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
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

	ws, err := o.workspaces.Get(ctx, r.WorkspaceID)
	if err != nil {
		return err
	}
	if ws.ActiveRunID == runID {
		ws.ActiveRunID = ""
		if err := o.workspaces.Update(ctx, ws); err != nil {
			return err
		}
		return o.publishWorkspaceStateChanged(ctx, ws, runID)
	}
	return nil
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
	if err := o.recordRunCheckpoint(ctx, run); err != nil {
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
		if step.Input == nil {
			step.Input = cloneStepPayload(input)
		}
		if err := o.steps.Update(ctx, step); err != nil {
			return err
		}
	}
	if err := o.publishStepState(ctx, run, step, events.TopicStepDispatched); err != nil {
		return err
	}
	toolCall, err := o.recordToolCallStarted(ctx, run, step)
	if err != nil {
		return err
	}
	if err := o.recordStepCheckpoint(ctx, run, step, "step_dispatched"); err != nil {
		return err
	}

	workspace, err := o.workspaces.Get(ctx, run.WorkspaceID)
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
		step.State = common.StepStateFailed
		step.Output = runtimeFailureOutput(result.Output, err)
		_ = o.recordToolCallFailed(ctx, toolCall, err, step.Output)
		_ = o.recordValidationResult(ctx, run, step, common.ValidationResultStateFailed, "runtime step failed", string(toolCall.ID), err.Error())
		_ = o.recordStepCheckpoint(ctx, run, step, "step_failed")
		_ = o.steps.Update(ctx, step)
		_ = o.publishStepState(ctx, run, step, events.TopicStepStateChanged)
		return o.FailRun(ctx, run.ID, err.Error())
	}
	output := result.Output
	if err := o.applyRuntimePatches(ctx, run, step, toolCall, output); err != nil {
		step.State = common.StepStateFailed
		step.Output = runtimeFailureOutput(output, err)
		_ = o.recordToolCallFailed(ctx, toolCall, err, step.Output)
		_ = o.recordValidationResult(ctx, run, step, common.ValidationResultStateFailed, "runtime patch application failed", string(toolCall.ID), err.Error())
		_ = o.recordStepCheckpoint(ctx, run, step, "step_failed")
		_ = o.steps.Update(ctx, step)
		_ = o.publishStepState(ctx, run, step, events.TopicStepStateChanged)
		return o.FailRun(ctx, run.ID, err.Error())
	}
	memoryRefs, err := o.recordMemoryWrites(ctx, run, step, output)
	if err != nil {
		step.State = common.StepStateFailed
		step.Output = runtimeFailureOutput(output, err)
		_ = o.recordToolCallFailed(ctx, toolCall, err, step.Output)
		_ = o.recordValidationResult(ctx, run, step, common.ValidationResultStateFailed, "runtime memory write failed", string(toolCall.ID), err.Error())
		_ = o.recordStepCheckpoint(ctx, run, step, "step_failed")
		_ = o.steps.Update(ctx, step)
		_ = o.publishStepState(ctx, run, step, events.TopicStepStateChanged)
		return o.FailRun(ctx, run.ID, err.Error())
	}
	if len(memoryRefs) > 0 {
		output[stepOutputKeyMemoryRefs] = memoryRefs
	}

	step.State = common.StepStateCompleted
	step.Output = output
	if err := o.recordToolCallCompleted(ctx, toolCall, output); err != nil {
		return err
	}
	if err := o.recordValidationResult(ctx, run, step, common.ValidationResultStatePassed, "runtime step completed", string(toolCall.ID), ""); err != nil {
		return err
	}
	if err := o.steps.Update(ctx, step); err != nil {
		return err
	}
	if err := o.publishStepState(ctx, run, step, events.TopicStepStateChanged); err != nil {
		return err
	}
	if err := o.recordStepCheckpoint(ctx, run, step, "step_completed"); err != nil {
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
		return o.BlockOnDecision(ctx, run.ID, ticketID)
	}
	if reason, approvalID := approvalBlockSpec(output); reason != "" {
		if nextStepID == "" && !o.hasGraphContinuation(ctx, run, step) {
			return o.FailRun(ctx, run.ID, fmt.Sprintf("runtime step %s blocked without next step", step.ID))
		}
		if approvalID == "" {
			approvalID = common.ApprovalID(runtimeChildID(step.ID, "approval"))
		}
		if err := o.createApproval(ctx, run, step, approvalID, reason); err != nil {
			return err
		}
		return o.BlockOnApproval(ctx, run.ID, approvalID)
	}
	if graphBatchStep(step) {
		return nil
	}
	if nextStepID != "" {
		return o.dispatchStep(ctx, run, nextStepID, nextKind, nextStepInputFromOutput(run, step))
	}
	if graphSpecs := o.readyGraphStepSpecs(ctx, run, step); len(graphSpecs) > 0 {
		return o.dispatchGraphStepBatch(ctx, run, graphSpecs)
	}
	if shouldDispatchGraphFollowUp(run, step, output) {
		return o.dispatchStep(ctx, run, graphFollowUpStepID(run.ID, step), initialStepKind(run), graphFollowUpStepInput(run, step))
	}
	if err := completionDoDAssessmentError(run, output); err != nil {
		step.State = common.StepStateFailed
		step.Output = runtimeFailureOutput(output, err)
		_ = o.recordToolCallFailed(ctx, toolCall, err, step.Output)
		_ = o.recordValidationResult(ctx, run, step, common.ValidationResultStateFailed, "runtime completion gate failed", string(toolCall.ID), err.Error())
		_ = o.recordStepCheckpoint(ctx, run, step, "step_failed")
		_ = o.steps.Update(ctx, step)
		_ = o.publishStepState(ctx, run, step, events.TopicStepStateChanged)
		return o.FailRun(ctx, run.ID, err.Error())
	}
	if err := o.ValidateRun(ctx, run.ID); err != nil {
		return err
	}
	return o.CompleteRun(ctx, run.ID)
}

type runtimePatchRequest struct {
	TargetRef string
	Content   string
	Diff      string
	Summary   string
	PatchType string
}

func (o *RuntimeOrchestrator) applyRuntimePatches(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	output map[string]any,
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
	workspace, err := o.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil {
		return err
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
		if err := applyWorkspacePatch(workspace.WorkingDirectory, request); err != nil {
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
	refs := make([]string, 0, len(requests))
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
		if err := o.evidence.MemoryItems.Create(ctx, item); err != nil {
			if err != common.ErrConflict {
				return nil, err
			}
			if err := o.evidence.MemoryItems.Update(ctx, item); err != nil {
				return nil, err
			}
		}
		refs = append(refs, string(memoryID))
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
	summary, _ := output["summary"].(string)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return requests
	}
	return append(requests, runtimeMemoryWriteRequest{
		MemoryType: memorycommon.MemoryTypeRunSummary,
		Summary:    summary,
		Content:    summary,
		Tags:       []string{"run_summary"},
	})
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
	targetRef = strings.TrimSpace(targetRef)
	if targetRef == "" {
		return runtimePatchRequest{}, common.ErrInvalidArgument
	}
	if patchType == "" {
		patchType = "file_write"
	}
	if patchType != "file_write" && patchType != "unified_diff" {
		return runtimePatchRequest{}, common.ErrInvalidArgument
	}
	if patchType == "unified_diff" && strings.TrimSpace(diff) == "" {
		return runtimePatchRequest{}, common.ErrInvalidArgument
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
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(content), 0o644)
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
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
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

func (o *RuntimeOrchestrator) recordToolCallCompleted(ctx context.Context, toolCall *runModel.ToolCall, output map[string]any) error {
	if toolCall == nil || toolCall.ID == "" || o.evidence.ToolCalls == nil {
		return nil
	}
	toolCall.State = common.ToolCallStateCompleted
	toolCall.EndedAt = runtimeEvidenceTimestamp()
	toolCall.ExitSummary = "runtime step completed"
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
	}
	return ret
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

func (o *RuntimeOrchestrator) recordRunCheckpoint(ctx context.Context, run *runModel.Run) error {
	if run == nil || o.evidence.RuntimeCheckpoints == nil {
		return nil
	}
	currentStepID := o.latestStepID(ctx, run.ID)
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
		if specs := o.readyGraphStepSpecs(ctx, run, last); len(specs) > 0 {
			return specs, nil
		}
		nextStepID, nextKind := nextStepSpec(last.Output)
		if nextStepID != "" {
			return []stepDispatchSpec{{
				ID:    nextStepID,
				Kind:  nextKind,
				Input: nextStepInputFromOutput(run, last),
			}}, nil
		}
		if shouldDispatchGraphFollowUp(run, last, last.Output) {
			return []stepDispatchSpec{{
				ID:    graphFollowUpStepID(run.ID, last),
				Kind:  initialStepKind(run),
				Input: graphFollowUpStepInput(run, last),
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
	return nil, common.ErrConflict
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
	if len(specs) == 0 {
		return common.ErrConflict
	}
	if len(specs) == 1 {
		return o.dispatchStep(ctx, run, specs[0].ID, specs[0].Kind, specs[0].Input)
	}
	return o.dispatchGraphStepBatch(ctx, run, specs)
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
	if o == nil || run == nil || last == nil {
		return nil
	}
	graph, ok := runtimeGraphSpecFromStep(last)
	if !ok || len(graph.Nodes) == 0 {
		return nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil
	}
	completed := completedGraphNodes(steps)
	started := startedGraphNodes(steps)
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
		input := nextGraphStepInput(run, last, graph, node, completed, branch)
		ret = append(ret, stepDispatchSpec{
			ID:    graphStepID(run.ID, node.ID),
			Kind:  node.Kind,
			Input: input,
		})
	}
	return ret
}

func (o *RuntimeOrchestrator) dispatchGraphStepBatch(ctx context.Context, run *runModel.Run, specs []stepDispatchSpec) error {
	if len(specs) == 0 {
		return common.ErrConflict
	}
	if len(specs) == 1 {
		return o.dispatchStep(ctx, run, specs[0].ID, specs[0].Kind, specs[0].Input)
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
			errCh <- o.dispatchStep(ctx, &runCopy, spec.ID, spec.Kind, spec.Input)
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil && !errors.Is(err, common.ErrConflict) {
			return err
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
	last := latestCompletedGraphStep(steps)
	if last == nil {
		return common.ErrConflict
	}
	if nextSpecs := o.readyGraphStepSpecs(ctx, currentRun, last); len(nextSpecs) > 0 {
		return o.dispatchGraphStepBatch(ctx, currentRun, nextSpecs)
	}
	if shouldDispatchGraphFollowUp(currentRun, last, last.Output) {
		return o.dispatchStep(ctx, currentRun, graphFollowUpStepID(currentRun.ID, last), initialStepKind(currentRun), graphFollowUpStepInput(currentRun, last))
	}
	if err := completionDoDAssessmentError(currentRun, last.Output); err != nil {
		last.State = common.StepStateFailed
		last.Output = runtimeFailureOutput(last.Output, err)
		_ = o.steps.Update(ctx, last)
		return o.FailRun(ctx, currentRun.ID, err.Error())
	}
	if err := o.ValidateRun(ctx, currentRun.ID); err != nil {
		return err
	}
	return o.CompleteRun(ctx, currentRun.ID)
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

func completedGraphNodes(steps []*runModel.Step) map[string]bool {
	ret := map[string]bool{}
	for _, step := range steps {
		if step == nil || step.State != common.StepStateCompleted {
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

func startedGraphNodes(steps []*runModel.Step) map[string]bool {
	ret := map[string]bool{}
	for _, step := range steps {
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
) map[string]any {
	input := nextStepInput(run, last.ID)
	for key, value := range node.Input {
		input[key] = value
	}
	input[stepOutputKeyExecutionGraph] = graph
	input[stepInputKeyGraphNodeID] = node.ID
	input[stepInputKeyGraphPreviousNodeID] = graphNodeID(last)
	input[stepInputKeyGraphCompletedNodes] = completedGraphNodeList(completed)
	if branch != "" {
		input[stepOutputKeyGraphBranch] = branch
	}
	return input
}

func graphStepID(runID common.RunID, nodeID string) common.StepID {
	return common.StepID(fmt.Sprintf("%s/block/%s", runID, nodeID))
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

func graphFollowUpStepID(runID common.RunID, step *runModel.Step) common.StepID {
	nodeID := runtimeChildID(common.StepID(graphNodeID(step)), "")
	if nodeID == "" {
		nodeID = runtimeChildID(step.ID, "graph")
	}
	return common.StepID(fmt.Sprintf("%s/zz-llm-after-graph-%s", runID, nodeID))
}

func graphFollowUpStepInput(run *runModel.Run, step *runModel.Step) map[string]any {
	input := nextStepInput(run, step.ID)
	input["graphFollowUp"] = true
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
		input[stepOutputKeyExecutionGraph] = graph
	}
	if step.Output != nil {
		input["previousStepOutput"] = cloneStepPayload(step.Output)
	}
	return input
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
	return nextStepInput(run, "")
}

func nextStepInputFromOutput(run *runModel.Run, previousStep *runModel.Step) map[string]any {
	if previousStep == nil {
		return nextStepInput(run, "")
	}
	input := nextStepInput(run, previousStep.ID)
	if nextInput := mapFromValue(previousStep.Output[stepOutputKeyNextInput]); nextInput != nil {
		for key, value := range nextInput {
			input[key] = value
		}
	}
	return input
}

type runtimeDecisionBlockSpec struct {
	Reason       string
	TicketID     common.TicketID
	DecisionKind string
	Question     string
	Context      map[string]any
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

func approvalBlockSpec(output map[string]any) (string, common.ApprovalID) {
	if output == nil {
		return "", ""
	}
	reason, _ := output[stepOutputKeyBlockApprovalReason].(string)
	approvalID, _ := output[stepOutputKeyBlockApprovalID].(string)
	return reason, common.ApprovalID(approvalID)
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
	if !requiresDoDAssessmentCompletionGate(run) {
		return nil
	}
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		return fmt.Errorf("%w: dodAssessment is required for llm-driven completion", common.ErrInvalidArgument)
	}
	satisfied, ok := assessment["satisfied"].(bool)
	if !ok {
		return fmt.Errorf("%w: dodAssessment.satisfied is required", common.ErrInvalidArgument)
	}
	if !satisfied {
		return fmt.Errorf("%w: dodAssessment is not satisfied", common.ErrInvalidArgument)
	}
	if err := validateDoDAssessmentItems(assessment["items"]); err != nil {
		return err
	}
	return nil
}

func requiresDoDAssessmentCompletionGate(run *runModel.Run) bool {
	return run != nil && run.DoDID != "" && initialStepKind(run) != defaultInitialStepKind
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
		Payload:      events.ApprovalPayload(approval.Reason, approval.State),
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
