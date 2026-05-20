package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
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
	stepOutputKeyNextStep               = "nextStep"
	stepOutputKeyNextKind               = "nextKind"
	stepOutputKeyNextInput              = "nextInput"
	stepOutputKeyPatches                = "patches"
	stepOutputKeyMemoryWrites           = "memoryWrites"
	stepOutputKeyMemoryRefs             = "memoryRefs"
	stepOutputKeyDoDAssessment          = "dodAssessment"
	stepOutputKeyLLMDecision            = "llmDecision"
	stepOutputKeyExecutionGraph         = "executionGraph"
	stepOutputKeyGraphBranch            = "graphBranch"
	stepInputKeyGraphNodeID             = "graphNodeId"
	stepInputKeyGraphRootStepID         = "graphRootStepId"
	stepInputKeyGraphPreviousNodeID     = "graphPreviousNodeId"
	stepInputKeyGraphCompletedNodes     = "graphCompletedNodes"
	stepInputKeyGraphBatch              = "graphBatch"
	stepInputKeyGraphEvidence           = "graphEvidence"
	stepInputKeyRunEvidenceSummary      = "runEvidenceSummary"
	stepInputKeyProcessEvidenceTimeline = "processEvidenceTimeline"
	stepInputKeyCurrentProgressDigest   = "currentProgressDigest"
	stepInputKeyWorkspaceFileManifest   = "workspaceFileManifest"
	stepInputKeySoulStage               = "soulStage"
	stepInputKeyRuntimeRepairFollowUp   = "runtimeRepairFollowUp"
	stepInputKeyRuntimeRepairAttempt    = "runtimeRepairAttempt"
	stepInputKeyRepairOfStepID          = "repairOfStepId"
	stepInputKeyRepairOfRootStepID      = "repairOfRootStepId"
	stepInputKeyRepairOfFailureClass    = "repairOfFailureClass"
	stepInputKeyRepairOfFailureSig      = "repairOfFailureSignature"
	stepInputKeyProviderRetryFollowUp   = "providerRetryFollowUp"
	stepInputKeyProviderRetryAttempt    = "providerRetryAttempt"
	stepInputKeyProviderRetryRootStep   = "providerRetryRootStepId"
	stepInputKeyRecoveryAction          = "recoveryAction"
	stepInputKeyRecoveryRootStepID      = "recoveryRootStepId"
	stepInputKeyRecoveryLineageType     = "recoveryLineageType"
	stepInputKeyRecoveryBudgetScope     = "recoveryBudgetScope"
	stepInputKeyNoActionableFollowUp    = "noActionableContinuationFollowUp"
	stepInputKeyAssessmentOnly          = "assessmentOnly"
	stepInputKeyGraphLimitFollowUp      = "graphExpansionLimitFollowUp"
	stepOutputKeyBlockDecisionReason    = "blockDecisionReason"
	stepOutputKeyBlockDecisionTicketID  = "blockDecisionTicketId"
	stepOutputKeyBlockDecisionKind      = "blockDecisionKind"
	stepOutputKeyBlockDecisionQuestion  = "blockDecisionQuestion"
	stepOutputKeyBlockDecisionContext   = "blockDecisionContext"
	stepOutputKeyBlockApprovalReason    = "blockApprovalReason"
	stepOutputKeyBlockApprovalID        = "blockApprovalId"
	stepOutputKeyBlockApprovalAction    = "blockApprovalAction"
	stepOutputKeyBlockApprovalContext   = "blockApprovalContext"
	llmWorkspaceExecutionStepKind       = "llm/workspace_execution"
	defaultInitialStepKind              = llmWorkspaceExecutionStepKind
	soulStagePlanning                   = "planning"
	soulStageExecution                  = "execution"
	soulStageDecision                   = "decision"
	soulStageApproval                   = "approval"
	soulStageValidation                 = "validation"
	defaultMaxRuntimeRepairAttempts     = 10
	maxProviderRetryAttempts            = 10
	maxGraphFollowUpExpansionAttempts   = 2
)

type RuntimePolicy struct {
	MaxRuntimeRepairAttempts int
	MaxProviderRetryAttempts int
}

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
	policy      RuntimePolicy
}

type RuntimeEvidenceRepositories struct {
	Results              repository.ResultRepository
	Artifacts            repository.ArtifactRepository
	ToolCalls            repository.ToolCallRepository
	Patches              repository.PatchRepository
	ValidationResults    repository.ValidationResultRepository
	RuntimeCheckpoints   repository.RuntimeCheckpointRepository
	MemoryItems          repository.MemoryItemRepository
	MemoryRecallTraces   repository.MemoryRecallTraceRepository
	AutoGovernanceTraces repository.AutoGovernanceTraceRepository
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
	policies ...RuntimePolicy,
) *RuntimeOrchestrator {
	if eventHub == nil {
		eventHub = events.NewMemoryHub()
	}
	policy := NormalizeRuntimePolicy(RuntimePolicy{})
	if len(policies) > 0 {
		policy = NormalizeRuntimePolicy(policies[0])
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
		policy:      policy,
	}
}

func NormalizeRuntimePolicy(policy RuntimePolicy) RuntimePolicy {
	if policy.MaxRuntimeRepairAttempts <= 0 {
		policy.MaxRuntimeRepairAttempts = defaultMaxRuntimeRepairAttempts
	}
	if policy.MaxProviderRetryAttempts <= 0 {
		policy.MaxProviderRetryAttempts = maxProviderRetryAttempts
	}
	return policy
}

func (o *RuntimeOrchestrator) maxRuntimeRepairAttempts() int {
	if o == nil || o.policy.MaxRuntimeRepairAttempts <= 0 {
		return defaultMaxRuntimeRepairAttempts
	}
	return o.policy.MaxRuntimeRepairAttempts
}

func (o *RuntimeOrchestrator) maxProviderRetryAttempts() int {
	if o == nil || o.policy.MaxProviderRetryAttempts <= 0 {
		return maxProviderRetryAttempts
	}
	return o.policy.MaxProviderRetryAttempts
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
		activeRun, err := o.runs.Get(ctx, ws.ActiveRunID)
		if errors.Is(err, common.ErrNotFound) {
			return common.ErrConflict
		}
		if err != nil {
			return err
		}
		if activeRun != nil && activeRun.WorkspaceID == ws.ID && !isTerminalRunState(activeRun.State) {
			return common.ErrConflict
		}
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
	if err := o.dispatchStepSpecsWithWorkspace(ctx, r, specs, ws); err != nil {
		return errors.Join(err, o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID))
	}
	return o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID)
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
			if o.runtimeRepairExhaustionBlockerHasSuccessEvidence(ctx, ticket) {
				r.BlockingTicketID = ""
				break
			}
			if o.duplicateRuntimeFailureBlockerCanAutoResume(ctx, r, ticket) {
				resumeAfterStepID = ticket.StepID
				r.BlockingTicketID = ""
				break
			}
			if o.runtimeRepairExhaustionBlockerCanAutoResume(ctx, r, ticket) {
				ticket.State = common.DecisionTicketStateDismissed
				if err := o.tickets.Update(ctx, ticket); err != nil {
					return err
				}
				resumeAfterStepID = ticket.StepID
				r.BlockingTicketID = ""
				break
			}
			if o.knownReadableDecisionBlockerCanAutoRepair(ctx, r, ticket) {
				ticket.State = common.DecisionTicketStateDismissed
				if err := o.tickets.Update(ctx, ticket); err != nil {
					return err
				}
				resumeAfterStepID = ticket.StepID
				resumeInput = map[string]any{"knownReadableDecisionRepair": true}
				r.BlockingTicketID = ""
				break
			}
			return common.ErrBlocked
		}
		resumeAfterStepID = ticket.StepID
		if o.resolutions != nil {
			resolution, err := o.resolutions.GetByTicketID(ctx, r.BlockingTicketID)
			if err != nil {
				return err
			}
			switch effectiveDecisionResolutionType(ticket, resolution) {
			case decisioncommon.ResolutionTypeProvideMissingInfo:
				resumeInput = decisionResolutionInput(ticket, resolution)
				r.BlockingTicketID = ""
			case decisioncommon.ResolutionTypeRejectCurrentPath:
				mergeRunExecutionContextPatch(r, executionContextPatch)
				return o.failLoadedRun(ctx, r, fmt.Sprintf("decision ticket %s rejected", ticket.ID))
			case decisioncommon.ResolutionTypeReplan:
				mergeRunExecutionContextPatch(r, executionContextPatch)
				return o.replanLoadedRun(ctx, r, ticket, resolution)
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
			if !errors.Is(err, common.ErrConflict) {
				return err
			}
			specs, specErr := o.runningFailedTerminalStepRecoverySpecs(ctx, r)
			if specErr != nil {
				return specErr
			}
			if len(specs) == 0 {
				return err
			}
			r.State = common.RunStateRunning
			r.FailureReason = ""
			mergeRunExecutionContextPatch(r, executionContextPatch)
			workspace, syncErr := o.syncRunAndWorkspaceForActiveState(ctx, r, common.WorkspaceStateRunning, nil)
			if syncErr != nil {
				return syncErr
			}
			if publishErr := o.publishRunStateChanged(ctx, r); publishErr != nil {
				return publishErr
			}
			if publishErr := o.publishWorkspaceStateChanged(ctx, workspace, r.ID); publishErr != nil {
				return publishErr
			}
			if dispatchErr := o.dispatchStepSpecsWithWorkspace(ctx, r, specs, workspace); dispatchErr != nil {
				return errors.Join(o.failRunningResume(ctx, r.ID, dispatchErr), o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID))
			}
			return o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID)
		}
		if recoverStepID == "" {
			if len(executionContextPatch) > 0 {
				mergeRunExecutionContextPatch(r, executionContextPatch)
				workspace, err := o.syncRunAndWorkspaceForActiveState(ctx, r, common.WorkspaceStateRunning, nil)
				if err != nil {
					return err
				}
				if err := o.publishRunStateChanged(ctx, r); err != nil {
					return err
				}
				return o.publishWorkspaceStateChanged(ctx, workspace, r.ID)
			}
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
	specs, err := o.nextDispatchSpecsAfterResolvedDecision(ctx, r, resumeAfterStepID, resumeInput)
	if err != nil {
		return o.failRunningResume(ctx, r.ID, err)
	}
	mergeResumeInput(specs, resumeInput)
	if err := o.dispatchStepSpecsWithWorkspace(ctx, r, specs, workspace); err != nil {
		return errors.Join(o.failRunningResume(ctx, r.ID, err), o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID))
	}
	return o.reconcileWorkspaceActiveRun(ctx, r.WorkspaceID)
}

func (o *RuntimeOrchestrator) runningFailedTerminalStepRecoverySpecs(ctx context.Context, run *runModel.Run) ([]stepDispatchSpec, error) {
	if o == nil || run == nil || o.steps == nil {
		return nil, common.ErrConflict
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	last := terminalExecutionStep(steps)
	if last == nil || last.State != common.StepStateFailed {
		return nil, nil
	}
	return o.nextDispatchSpecsFromStep(ctx, run, last)
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
	switch effectiveDecisionResolutionType(ticket, resolution) {
	case decisioncommon.ResolutionTypeProvideMissingInfo:
		return last.ID, decisionResolutionInput(ticket, resolution), nil
	case decisioncommon.ResolutionTypeRejectCurrentPath:
		return "", nil, o.FailRun(ctx, run.ID, fmt.Sprintf("decision ticket %s rejected", ticket.ID))
	case decisioncommon.ResolutionTypeReplan:
		return "", nil, o.replanLoadedRun(ctx, run, ticket, resolution)
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

func (o *RuntimeOrchestrator) replanLoadedRun(ctx context.Context, run *runModel.Run, ticket *decisionmodel.Ticket, resolution *decisionmodel.Resolution) error {
	if o == nil || run == nil || run.ID == "" || o.runs == nil {
		return common.ErrInvalidArgument
	}
	successorID, err := o.nextReplanRunID(ctx, run.ID)
	if err != nil {
		return err
	}
	successorContext := cloneStepPayload(run.ExecutionContext)
	if successorContext == nil {
		successorContext = map[string]any{}
	}
	if history := mapSliceFromValue(successorContext["governanceHistory"]); len(history) > 0 {
		successorContext["governanceHistory"] = compactGovernanceHistoryForRuntimeContext(history, 6)
	}
	delete(successorContext, "replanContinuation")
	replanContext := map[string]any{
		"predecessorRunId": string(run.ID),
		"reason":           "decision_replan",
	}
	if ticket != nil {
		replanContext["ticketId"] = string(ticket.ID)
		replanContext["ticketStepId"] = string(ticket.StepID)
		replanContext["ticketReason"] = ticket.Reason
		if len(ticket.Context) > 0 {
			ticketContext := compactFailureContextForRuntimeContext(ticket.Context)
			replanContext["ticketContext"] = ticketContext
			replanContext["failureContext"] = ticketContext
		}
	}
	if resolution != nil {
		replanContext["resolutionId"] = string(resolution.ID)
		replanContext["resolutionType"] = resolution.Type
		replanContext["resolutionPayload"] = compactDecisionPayloadForRuntimeContext(resolution.Payload)
	}
	successorContext["replanContinuation"] = compactReplanContinuationForRuntimeContext(replanContext)
	successor := &runModel.Run{
		ID:               successorID,
		WorkspaceID:      run.WorkspaceID,
		GoalID:           run.GoalID,
		DoDID:            run.DoDID,
		InitialStepKind:  run.InitialStepKind,
		ExecutionContext: successorContext,
		State:            common.RunStateCreated,
		Priority:         run.Priority,
	}
	if successor.InitialStepKind == "" {
		successor.InitialStepKind = initialStepKind(run)
	}
	if err := o.closeLoadedRun(ctx, run, common.RunStateSuperseded, ""); err != nil {
		return err
	}
	if err := o.runs.Create(ctx, successor); err != nil {
		return err
	}
	return o.StartRun(ctx, successor.ID)
}

func (o *RuntimeOrchestrator) nextReplanRunID(ctx context.Context, predecessor common.RunID) (common.RunID, error) {
	if o == nil || o.runs == nil || predecessor == "" {
		return "", common.ErrInvalidArgument
	}
	base := runtimeReadableIDPart(string(predecessor))
	if base == "" {
		base = "run"
	}
	for seq := 1; seq <= 100; seq++ {
		candidate := common.RunID(fmt.Sprintf("%s-replan-%03d", base, seq))
		if _, err := o.runs.Get(ctx, candidate); errors.Is(err, common.ErrNotFound) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", common.ErrConflict
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
	if goalID, _ := patch["goalId"].(string); strings.TrimSpace(goalID) != "" {
		run.GoalID = common.GoalID(strings.TrimSpace(goalID))
	}
	if dodID, _ := patch["doDId"].(string); strings.TrimSpace(dodID) != "" {
		run.DoDID = common.DoDID(strings.TrimSpace(dodID))
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

func (o *RuntimeOrchestrator) reconcileWorkspaceActiveRun(ctx context.Context, workspaceID common.WorkspaceID) error {
	if o == nil || workspaceID == "" {
		return nil
	}
	ws, err := o.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return err
	}
	if ws.ActiveRunID == "" {
		return nil
	}
	activeRun, err := o.runs.Get(ctx, ws.ActiveRunID)
	if err != nil {
		if !errors.Is(err, common.ErrNotFound) {
			return err
		}
		ws.ActiveRunID = ""
		if isActiveWorkspaceState(ws.State) {
			ws.State = common.WorkspaceStateActive
		}
		if err := o.workspaces.Update(ctx, ws); err != nil {
			return err
		}
		return o.publishWorkspaceStateChanged(ctx, ws, "")
	}
	if activeRun.WorkspaceID != ws.ID || !isTerminalRunState(activeRun.State) {
		return nil
	}
	ws.ActiveRunID = ""
	ws.State = workspaceStateForTerminalRun(activeRun.State)
	if err := o.workspaces.Update(ctx, ws); err != nil {
		return err
	}
	return o.publishWorkspaceStateChanged(ctx, ws, activeRun.ID)
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

func isActiveWorkspaceState(state common.WorkspaceState) bool {
	switch state {
	case common.WorkspaceStateRunning, common.WorkspaceStateWaitingDecision, common.WorkspaceStateBlocked, common.WorkspaceStateValidating:
		return true
	default:
		return false
	}
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
		if step.RunID != run.ID {
			return common.ErrConflict
		}
		switch step.State {
		case common.StepStateCompleted, common.StepStateFailed:
			return fmt.Errorf("%w: runtime step %s is already terminal", common.ErrConflict, step.ID)
		}
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

	runtimeInput := cloneStepPayload(step.Input)
	if err := validatePatchStepInputBeforeRuntime(step.Kind, runtimeInput); err != nil {
		output := map[string]any{
			"status": "failed",
			"stage":  "patch_dispatch_invalid",
			"error":  err.Error(),
		}
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch dispatch gate failed", err)
	}
	injectRuntimeEvidencePolicies(runtimeInput, run.ExecutionContext)
	result, err := o.runtime.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		WorkspaceID:                   run.WorkspaceID,
		WorkspaceDeliverableDirectory: workspace.DeliverableDirectory,
		WorkspaceInternalDirectory:    workspace.InternalDirectory,
		RunID:                         run.ID,
		StepID:                        step.ID,
		Kind:                          step.Kind,
		Input:                         runtimeInput,
	})
	if err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, result.Output, "runtime step failed", err)
	}
	output := result.Output
	if err := validateRepairPatchOutputTargetRelevance(step, output); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch output contract failed", err)
	}
	if err := o.publishPatchPreviewFromOutput(ctx, run, step, toolCall, output); err != nil {
		return err
	}
	language := runDialogLanguage(run)
	patchApproval, err := o.patchApprovalBlockSpec(ctx, run.ID, language, output)
	if err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch approval gate failed", err)
	}
	if patchApproval.Reason != "" {
		applyPatchApprovalBlockOutput(output, patchApproval)
	} else if err := o.applyRuntimePatches(ctx, run, step, toolCall, output, workspace.DeliverableDirectory); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch application failed", err)
	}
	patchRevertApproval, err := o.patchRevertApprovalBlockSpec(ctx, run.ID, language, output)
	if err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch revert approval gate failed", err)
	}
	if patchRevertApproval.Reason != "" {
		applyPatchRevertApprovalBlockOutput(output, patchRevertApproval)
	} else if err := o.applyRuntimePatchRevert(ctx, run, step, toolCall, output, workspace.DeliverableDirectory); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch revert failed", err)
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

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime execution contract failed", err)
	}

	nextStepID, nextKind := nextStepSpec(output)
	if decisionSpec := decisionBlockSpec(output); decisionSpec.Reason != "" {
		decisionSpec.Context = o.enrichDecisionContextWithReadableWorkspaceResources(ctx, run, workspace, decisionSpec.Context, decisionSpec.Question, decisionSpec.Reason)
		if assessmentOnlyDecisionShouldRepair(step, decisionSpec) || continuableDecisionShouldRepair(step, decisionSpec) {
			if assessmentOnlyDecisionShouldRepair(step, decisionSpec) {
				output["assessmentOnlyDecisionConvertedToRepair"] = true
			} else {
				output["continuableDecisionConvertedToRepair"] = true
			}
			output["convertedDecision"] = convertedDecisionPayload(decisionSpec)
			output[stepOutputKeyDoDAssessment] = assessmentOnlyDecisionRepairAssessment(run, step, decisionSpec)
			delete(output, stepOutputKeyBlockDecisionReason)
			delete(output, stepOutputKeyBlockDecisionTicketID)
			delete(output, stepOutputKeyBlockDecisionKind)
			delete(output, stepOutputKeyBlockDecisionQuestion)
			delete(output, stepOutputKeyBlockDecisionContext)
			return o.handleRuntimeStepFailure(
				ctx,
				run,
				step,
				toolCall,
				output,
				"runtime completion gate failed",
				fmt.Errorf("%w: decision requests a continuable candidate choice", common.ErrInvalidArgument),
			)
		}
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
		if err := o.recordRejectedResultForDecisionBlock(ctx, run, step, output, decisionSpec); err != nil {
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
	if handled, err := o.validateGraphExpansionBoundary(ctx, run, step, toolCall, output, workspace); handled || err != nil {
		return err
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
		return o.dispatchUniqueStepWithWorkspace(ctx, run, graphFollowUpStepID(run.ID, step), initialStepKind(run), input, workspace)
	}
	if shouldDispatchLLMCompletionFollowUp(run, step, output) {
		return o.dispatchUniqueStepWithWorkspace(ctx, run, llmCompletionFollowUpStepID(run.ID, step), initialStepKind(run), o.llmCompletionFollowUpStepInput(ctx, run, step), workspace)
	}
	if shouldDispatchNoActionableContinuationFollowUp(run, step, output) {
		return o.dispatchUniqueStepWithWorkspace(ctx, run, noActionableContinuationFollowUpStepID(run.ID, step), initialStepKind(run), o.noActionableContinuationFollowUpStepInput(ctx, run, step), workspace)
	}
	if completionState := llmDecisionCompletionStateFromOutput(output); completionState != "" && completionState != "completed" {
		return o.handleRuntimeStepFailure(
			ctx,
			run,
			step,
			toolCall,
			output,
			"runtime completion gate failed",
			fmt.Errorf("%w: completionState %s has no actionable continuation", common.ErrInvalidArgument, completionState),
		)
	}
	output = normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	var evidenceNormalizeErr error
	output, evidenceNormalizeErr = o.normalizeCompletionGateDoDAssessmentEvidenceRefs(ctx, run, output)
	if evidenceNormalizeErr != nil {
		return evidenceNormalizeErr
	}
	step.Output = output
	if err := o.steps.Update(ctx, step); err != nil {
		return err
	}
	assessment, summary, err := validatedCompletionDoDAssessment(run, output)
	if err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime completion gate failed", err)
	}
	if err := o.validateDoDAssessmentEvidenceQuality(ctx, run, assessment); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime completion gate failed", err)
	}
	if err := o.recordCompletionEvidenceFromValidatedDoDAssessment(ctx, run, step, assessment, summary, workspace); err != nil {
		return err
	}
	return o.validateAndCompleteRunWithWorkspace(ctx, run, step.ID, workspace)
}

func (o *RuntimeOrchestrator) validateGraphExpansionBoundary(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	output map[string]any,
	workspace *workspaceModel.Workspace,
) (bool, error) {
	if run == nil || step == nil || output == nil || !requiresDoDAssessmentCompletionGate(run) {
		return false, nil
	}
	if llmExecutionGraphNodeReturnedOnlyGraph(run, step, output) {
		graph, _ := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
		err := fmt.Errorf("%w: execution-stage llm graph node returned executionGraph without concrete output", common.ErrInvalidArgument)
		output["executionStageContractViolation"] = true
		output["ignoredExecutionGraph"] = true
		output["ignoredExecutionGraphNodeCount"] = len(graph.Nodes)
		output["ignoredExecutionGraphReason"] = err.Error()
		delete(output, stepOutputKeyExecutionGraph)
		step.Output = output
		_ = o.steps.Update(ctx, step)
		return true, o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime execution contract failed", err)
	}
	if boolValue(step.Input["providerRetryAssessmentOnly"]) || boolValue(step.Input[stepInputKeyAssessmentOnly]) {
		if ignoredGraph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
			err := fmt.Errorf("%w: assessment-only step returned executionGraph", common.ErrInvalidArgument)
			if boolValue(step.Input["providerRetryAssessmentOnly"]) {
				err = fmt.Errorf("%w: provider retry assessment-only step returned executionGraph", common.ErrInvalidArgument)
			}
			output["assessmentOnlyContractViolation"] = true
			output["ignoredExecutionGraph"] = true
			output["ignoredExecutionGraphNodeCount"] = len(ignoredGraph.Nodes)
			output["ignoredExecutionGraphReason"] = err.Error()
			delete(output, stepOutputKeyExecutionGraph)
			if doDAssessmentFromOutput(output) != nil {
				step.Output = output
				_ = o.steps.Update(ctx, step)
				return false, nil
			}
			if !boolValue(step.Input["assessmentOnlyContractCorrection"]) {
				output["contractCorrectionStepDispatched"] = true
				step.Output = output
				_ = o.steps.Update(ctx, step)
				input := assessmentOnlyContractCorrectionStepInput(step, output, err)
				return true, o.dispatchUniqueStepWithWorkspace(ctx, run, assessmentOnlyContractCorrectionStepID(run.ID, step), initialStepKind(run), input, workspace)
			}
			return true, o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime completion gate failed", err)
		}
	}
	if !graphFollowUpExpansionStep(step) {
		return false, nil
	}
	count := o.graphFollowUpExpansionCount(ctx, run.ID)
	if count <= maxGraphFollowUpExpansionAttempts {
		return false, nil
	}
	output["graphFollowUpExpansionLimitExceeded"] = true
	output["graphFollowUpExpansionCount"] = count
	output["graphFollowUpExpansionLimit"] = maxGraphFollowUpExpansionAttempts
	output["ignoredExecutionGraph"] = true
	step.Output = output
	_ = o.steps.Update(ctx, step)
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return true, err
	}
	input := o.graphExpansionLimitAssessmentFollowUpStepInputFromSteps(ctx, run, step, steps, count)
	return true, o.dispatchUniqueStepWithWorkspace(ctx, run, graphExpansionLimitAssessmentFollowUpStepID(run.ID, step), initialStepKind(run), input, workspace)
}

func validateLLMExecutionOutputContract(run *runModel.Run, step *runModel.Step, output map[string]any) error {
	if run == nil || step == nil || output == nil || step.Kind != initialStepKind(run) {
		return nil
	}
	stage := runtimeStageFromStepInput(step.Input)
	if err := validateLLMExecutionGraphOutputContract(step, output); err != nil {
		return err
	}
	if err := validateLLMNextStepPatchTargetRelevance(step, output); err != nil {
		return err
	}
	if err := validatePrimaryDiagnosticPatchOutputContract(step, output); err != nil {
		return err
	}
	if err := validateLLMSoulGraphCapabilityContract(run, step, output); err != nil {
		return err
	}
	if err := validateLLMFollowUpGraphStateChangingContract(run, step, output); err != nil {
		return err
	}
	if err := validateLLMCoverageGapRepairIntent(step, output); err != nil {
		return err
	}
	if err := validateLLMRepairStrategyReassessmentContract(step, output); err != nil {
		return err
	}
	if err := validateScopedStateChangingRepairVerificationContract(step, output); err != nil {
		return err
	}
	if err := validateRuntimeRepairFailureDiagnosticsContract(step); err != nil {
		return err
	}
	if boolValue(step.Input["stateChangingOutputRequired"]) {
		if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
			if allowPatchRepairValidationEvidenceGraph(step, graph) {
				return nil
			}
			if err := validateStateChangingExecutionGraph(graph); err != nil {
				return err
			}
			return nil
		}
		if hasStateChangingLLMExecutionOutput(output) || doDAssessmentFromOutput(output) != nil {
			return nil
		}
		if spec := decisionBlockSpec(output); spec.Reason != "" {
			return nil
		}
		if spec := approvalBlockSpec(output); spec.Reason != "" {
			return nil
		}
		return fmt.Errorf("%w: state-changing llm repair step returned no state-changing output", common.ErrInvalidArgument)
	}
	if graphNodeID(step) == "" {
		return nil
	}
	if stage != runtimeStageExecution {
		return nil
	}
	if llmExecutionGraphNodeReturnedOnlyGraph(run, step, output) {
		return fmt.Errorf("%w: execution-stage llm graph node returned executionGraph without concrete output", common.ErrInvalidArgument)
	}
	if !graphNodeRequiresStateChangingOutput(step) {
		return nil
	}
	if hasStateChangingLLMExecutionOutput(output) || doDAssessmentFromOutput(output) != nil {
		return nil
	}
	if spec := decisionBlockSpec(output); spec.Reason != "" {
		return nil
	}
	if spec := approvalBlockSpec(output); spec.Reason != "" {
		return nil
	}
	return fmt.Errorf("%w: state-changing execution-stage llm graph node returned no state-changing output", common.ErrInvalidArgument)
}

func validateScopedStateChangingRepairVerificationContract(step *runModel.Step, output map[string]any) error {
	if step == nil || step.Input == nil || output == nil {
		return nil
	}
	if !stateChangingRepairContractActive(step) {
		return nil
	}
	if runtimeStageFromStepInput(step.Input) != runtimeStageRepair {
		return nil
	}
	if !strategyFailedSurfaceRequiresBoundedVerification(
		strings.TrimSpace(stringFromMap(step.Input, "failureTargetScope")),
		mapFromValue(step.Input["strategyReassessment"]),
	) {
		return nil
	}
	if !strategyOutputGraphUsesUnboundedVerification(output) {
		return nil
	}
	return fmt.Errorf("%w: scoped state-changing runtime repair requires bounded verification; unbounded verification without explicit target scope is not allowed", common.ErrInvalidArgument)
}

func validateRuntimeRepairFailureDiagnosticsContract(step *runModel.Step) error {
	if step == nil || step.Input == nil {
		return nil
	}
	if !stateChangingRepairContractActive(step) {
		return nil
	}
	if strings.TrimSpace(stringFromMap(step.Input, "failedStepId")) == "" &&
		strings.TrimSpace(stringFromMap(step.Input, stepInputKeyRepairOfStepID)) == "" {
		return nil
	}
	diagnostics := mapFromValue(step.Input["lastFailureDiagnostics"])
	if len(diagnostics) == 0 {
		failedOutput := mapFromValue(step.Input["failedStepOutput"])
		if len(failedOutput) > 0 {
			diagnostics = map[string]any{
				"failureSignature": firstNonEmptyString(
					stringFromMap(step.Input, failureOutputKeySignature),
					stringFromMap(failedOutput, failureOutputKeySignature),
				),
				"failureClass": firstNonEmptyString(
					stringFromMap(step.Input, failureOutputKeyClass),
					stringFromMap(failedOutput, failureOutputKeyClass),
				),
				"error": firstNonEmptyString(
					stringFromMap(failedOutput, "error"),
					stringFromMap(failedOutput, "reason"),
				),
			}
		}
	}
	if strings.TrimSpace(firstNonEmptyString(
		stringFromMap(diagnostics, "failureSignature"),
		stringFromMap(diagnostics, "failureClass"),
		stringFromMap(diagnostics, "error"),
	)) == "" {
		return fmt.Errorf("%w: state-changing runtime repair requires lastFailureDiagnostics or equivalent failedStepOutput diagnostics", common.ErrInvalidArgument)
	}
	return nil
}

func validateLLMSoulGraphCapabilityContract(run *runModel.Run, step *runModel.Step, output map[string]any) error {
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok || len(graph.Nodes) == 0 {
		return nil
	}
	soul := runtimeSoulForStep(run, step)
	if len(soul) == 0 {
		return nil
	}
	if err := validateGraphAllowedToolKinds(graph, stringSliceFromValue(soul["allowedToolKinds"])); err != nil {
		return err
	}
	if err := validateGraphAllowedMCPServers(graph, stringSliceFromValue(soul["allowedMCPServerRefs"])); err != nil {
		return err
	}
	return nil
}

func runtimeSoulForStep(run *runModel.Run, step *runModel.Step) map[string]any {
	if run == nil || step == nil || len(run.ExecutionContext) == 0 {
		return nil
	}
	stage := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(step.Input, stepInputKeySoulStage),
		stringFromMap(step.Input, stepInputKeyRuntimeStage),
	))
	routing := mapFromValue(run.ExecutionContext["soulRouting"])
	bindings := mapFromValue(routing["bindings"])
	if stage != "" {
		if binding := mapFromValue(bindings[stage]); len(binding) > 0 {
			if soul := mapFromValue(binding["soul"]); len(soul) > 0 {
				return soul
			}
		}
	}
	return mapFromValue(run.ExecutionContext["soul"])
}

func validateGraphAllowedToolKinds(graph runtimemodel.BlockExecutionGraph, allowed []string) error {
	if len(allowed) == 0 || stringListAllows(allowed, "*") {
		return nil
	}
	allowedSet := stringSet(allowed)
	for _, node := range graph.Nodes {
		kind := strings.TrimSpace(node.Kind)
		if kind == "" || allowedSet[kind] {
			continue
		}
		return fmt.Errorf("%w: executionGraph node %q kind %q is not allowed by current soul allowedToolKinds", common.ErrInvalidArgument, strings.TrimSpace(node.ID), kind)
	}
	return nil
}

func validateGraphAllowedMCPServers(graph runtimemodel.BlockExecutionGraph, allowed []string) error {
	if len(allowed) == 0 || stringListAllows(allowed, "*") {
		return nil
	}
	allowedSet := stringSet(allowed)
	for _, node := range graph.Nodes {
		kind := strings.TrimSpace(node.Kind)
		if kind != "mcp/tool_call" && kind != "mcp/resource_read" {
			continue
		}
		serverID := strings.TrimSpace(stringFromMap(node.Input, "serverId"))
		if serverID == "" {
			serverID = strings.TrimSpace(stringFromMap(mapFromValue(node.Input["input"]), "serverId"))
		}
		if serverID == "" || allowedSet[serverID] {
			continue
		}
		return fmt.Errorf("%w: executionGraph node %q MCP server %q is not allowed by current soul allowedMCPServerRefs", common.ErrInvalidArgument, strings.TrimSpace(node.ID), serverID)
	}
	return nil
}

func stringSet(values []string) map[string]bool {
	ret := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			ret[value] = true
		}
	}
	return ret
}

func stringListAllows(values []string, expected string) bool {
	expected = strings.TrimSpace(expected)
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}

func validateLLMExecutionGraphOutputContract(step *runModel.Step, output map[string]any) error {
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok {
		return nil
	}
	for _, node := range graph.Nodes {
		kind := strings.TrimSpace(node.Kind)
		switch kind {
		case "patch/preview", "patch/apply":
			request, err := runtimePatchRequestFromMap(node.Input)
			if err != nil {
				return fmt.Errorf("%w: executionGraph %s input invalid: %v", common.ErrInvalidArgument, kind, err)
			}
			if err := validateRepairPatchRequestGranularity(step, request); err != nil {
				return err
			}
			if kind == "patch/apply" {
				if err := validateRepairPatchTargetRelevance(step, request); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateLLMNextStepPatchTargetRelevance(step *runModel.Step, output map[string]any) error {
	if !repairPatchTargetRelevanceRequired(step) {
		return nil
	}
	_, nextKind := nextStepSpec(output)
	if strings.TrimSpace(nextKind) != "patch/apply" {
		return nil
	}
	nextInput := mapFromValue(output[stepOutputKeyNextInput])
	request, err := runtimePatchRequestFromMap(nextInput)
	if err != nil {
		return fmt.Errorf("%w: nextStep patch/apply input invalid: %v", common.ErrInvalidArgument, err)
	}
	if err := validateRepairPatchTargetRelevance(step, request); err != nil {
		return err
	}
	return nil
}

func validateRepairPatchRequestGranularity(step *runModel.Step, request runtimePatchRequest) error {
	if !stateChangingRepairContractActive(step) {
		return nil
	}
	if strings.TrimSpace(request.PatchType) != "file_write" {
		return nil
	}
	if len(mapSliceFromValue(step.Input["patchRepairContext"])) == 0 && len(mapSliceFromValue(step.Input["failureTargetRepairContext"])) == 0 {
		return nil
	}
	target := filepath.Clean(strings.TrimSpace(request.TargetRef))
	if target == "." || target == "" {
		return nil
	}
	for _, item := range repairPatchContextItems(step) {
		if item == nil || !boolValue(item["exists"]) {
			continue
		}
		itemTarget := filepath.Clean(strings.TrimSpace(stringFromMap(item, "targetRef")))
		if itemTarget != target {
			continue
		}
		if repairPatchContextAllowsSafeFileWrite(item, request) {
			return nil
		}
		return fmt.Errorf("%w: runtime repair patch/apply file_write cannot replace existing target %q when current repair context is available; use a minimal unified_diff or exact replacement against the current snapshot", common.ErrInvalidArgument, request.TargetRef)
	}
	return nil
}

func repairPatchContextAllowsSafeFileWrite(item map[string]any, request runtimePatchRequest) bool {
	if len(item) == 0 || strings.TrimSpace(request.PatchType) != "file_write" {
		return false
	}
	if !boolValue(item["contentComplete"]) || !boolValue(item["safeForFileWrite"]) {
		return false
	}
	if boolValue(item["redacted"]) || boolValue(item["truncated"]) || boolValue(item["isBinary"]) {
		return false
	}
	contentHash := strings.TrimSpace(stringFromMap(item, "contentHash"))
	if contentHash == "" || strings.TrimSpace(request.BaseHash) == "" || strings.TrimSpace(request.BaseHash) != contentHash {
		return false
	}
	if containsRuntimeRedactionMarker(request.Content) || containsUnresolvedRuntimeTemplate(request.Content) {
		return false
	}
	return true
}

func validateRepairPatchOutputTargetRelevance(step *runModel.Step, output map[string]any) error {
	if !repairPatchTargetRelevanceRequired(step) {
		return nil
	}
	requests, err := runtimePatchRequests(output)
	if err != nil {
		return err
	}
	for _, request := range requests {
		if err := validateRepairPatchTargetRelevance(step, request); err != nil {
			return err
		}
	}
	return validatePrimaryDiagnosticPatchOutputContract(step, output)
}

func validateRepairPatchTargetRelevance(step *runModel.Step, request runtimePatchRequest) error {
	if !repairPatchTargetRelevanceRequired(step) {
		return nil
	}
	targetRefs := repairPatchRelevantTargetRefs(step)
	if len(targetRefs) == 0 {
		return nil
	}
	if repairPatchTargetRefMatches(request.TargetRef, targetRefs) {
		return validateRepeatedRepairRootCauseTargetConsistency(step, request)
	}
	if repairPatchRequestDeclaresRelatedFailureTargets(request, targetRefs) {
		return validateRepeatedRepairRootCauseTargetConsistency(step, request)
	}
	if primaryDiagnosticRequestHasInferredRelatedShapeRepair(step, request, primaryDiagnosticRepairContractForStep(step)) {
		return validateRepeatedRepairRootCauseTargetConsistency(step, request)
	}
	if repairPatchTargetRefIsRelatedPrerequisite(step.Input, request.TargetRef, targetRefs) {
		return validateRepeatedRepairRootCauseTargetConsistency(step, request)
	}
	return fmt.Errorf("%w: runtime repair patch/apply target %q is not related to failure target scope %v; patch a current failure target or return a failed assessment/blocker", common.ErrInvalidArgument, request.TargetRef, limitedStrings(targetRefs, 5))
}

func validatePrimaryDiagnosticPatchOutputContract(step *runModel.Step, output map[string]any) error {
	contract := primaryDiagnosticRepairContractForStep(step)
	if len(contract) == 0 || !boolValue(contract["required"]) {
		return nil
	}
	requests, err := primaryDiagnosticPatchRequestsFromOutput(output)
	if err != nil {
		return err
	}
	if len(requests) == 0 {
		return nil
	}
	for _, request := range requests {
		if primaryDiagnosticPatchRequestAddressesContract(step, request, contract) {
			return nil
		}
	}
	return fmt.Errorf("%w: primary diagnostic repair requires patch/apply to address line-referenced resource shape blockers before secondary changes", common.ErrInvalidArgument)
}

func primaryDiagnosticPatchRequestsFromOutput(output map[string]any) ([]runtimePatchRequest, error) {
	if output == nil {
		return nil, nil
	}
	var ret []runtimePatchRequest
	if requests, err := runtimePatchRequests(output); err != nil {
		return nil, err
	} else {
		ret = append(ret, requests...)
	}
	if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
		for _, node := range graph.Nodes {
			if strings.TrimSpace(node.Kind) != "patch/apply" {
				continue
			}
			request, err := runtimePatchRequestFromMap(node.Input)
			if err != nil {
				return nil, fmt.Errorf("%w: executionGraph patch/apply input invalid: %v", common.ErrInvalidArgument, err)
			}
			ret = append(ret, request)
		}
	}
	if _, nextKind := nextStepSpec(output); strings.TrimSpace(nextKind) == "patch/apply" {
		request, err := runtimePatchRequestFromMap(mapFromValue(output[stepOutputKeyNextInput]))
		if err != nil {
			return nil, fmt.Errorf("%w: nextStep patch/apply input invalid: %v", common.ErrInvalidArgument, err)
		}
		ret = append(ret, request)
	}
	return ret, nil
}

func primaryDiagnosticRepairContractForStep(step *runModel.Step) map[string]any {
	if !stateChangingRepairContractActive(step) {
		return nil
	}
	if contract := mapFromValue(step.Input["primaryDiagnosticRepair"]); len(contract) > 0 {
		return contract
	}
	actionPackage := mapFromValue(step.Input["repairActionPackage"])
	if contract := mapFromValue(actionPackage["primaryDiagnosticRepair"]); len(contract) > 0 {
		return contract
	}
	if signals := mapFromValue(step.Input["structuredFailureSignals"]); len(signals) > 0 {
		return primaryDiagnosticRepairContractFromSignals(signals)
	}
	if signals := mapFromValue(actionPackage["structuredFailureSignals"]); len(signals) > 0 {
		return primaryDiagnosticRepairContractFromSignals(signals)
	}
	return nil
}

func primaryDiagnosticPatchRequestAddressesContract(step *runModel.Step, request runtimePatchRequest, contract map[string]any) bool {
	if len(contract) == 0 {
		return true
	}
	lineRefs := mapSliceFromValue(contract["lineRefs"])
	targetRefs := primaryDiagnosticTargetRefsFromContract(contract)
	if !primaryDiagnosticRequestTargetsDiagnosticScope(request, targetRefs) {
		return primaryDiagnosticRequestHasInferredRelatedShapeRepair(step, request, contract)
	}
	if primaryDiagnosticRequestHasMatchingDeclaration(request, contract) {
		return true
	}
	if primaryDiagnosticRequestDiffOverlapsLineRefs(request, lineRefs) {
		return true
	}
	if primaryDiagnosticRequestFileWriteChangesLineRefs(step, request, lineRefs) {
		return true
	}
	if primaryDiagnosticRequestMentionsIdentifiers(request, contract) &&
		(repairPatchTargetRefMatches(request.TargetRef, targetRefs) || repairPatchRequestDeclaresRelatedFailureTargets(request, targetRefs)) {
		return true
	}
	return false
}

func primaryDiagnosticTargetRefsFromContract(contract map[string]any) []string {
	if len(contract) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		value = normalizeRepairPatchTargetRef(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, value := range stringSliceFromValue(contract["targetRefs"]) {
		add(value)
	}
	for _, ref := range mapSliceFromValue(contract["lineRefs"]) {
		add(stringFromMap(ref, "targetRef"))
	}
	return ret
}

func primaryDiagnosticRequestHasInferredRelatedShapeRepair(step *runModel.Step, request runtimePatchRequest, contract map[string]any) bool {
	if len(contract) == 0 || !boolValue(contract["required"]) {
		return false
	}
	targetRefs := primaryDiagnosticTargetRefsFromContract(contract)
	if len(targetRefs) == 0 || repairPatchTargetRefMatches(request.TargetRef, targetRefs) {
		return false
	}
	if !primaryDiagnosticRequestTargetsRelatedResourceNeighborhood(step, request, targetRefs) {
		return false
	}
	identifiers := primaryDiagnosticMeaningfulIdentifiers(contract)
	if len(identifiers) == 0 {
		return false
	}
	changedText := primaryDiagnosticPatchChangedText(request)
	if changedText == "" {
		return false
	}
	return primaryDiagnosticTextMentionsAny(changedText, identifiers)
}

func primaryDiagnosticRequestTargetsRelatedResourceNeighborhood(step *runModel.Step, request runtimePatchRequest, targetRefs []string) bool {
	if len(targetRefs) == 0 {
		return false
	}
	for _, targetRef := range targetRefs {
		if repairPatchTargetsShareDirectory(request.TargetRef, targetRef) {
			return true
		}
	}
	if step == nil || step.Input == nil {
		return false
	}
	actionPackage := mapFromValue(step.Input["repairActionPackage"])
	for _, item := range mapSliceFromValue(actionPackage["workspaceFileManifest"]) {
		if !repairPatchTargetRefMatches(stringFromMap(item, "path"), []string{request.TargetRef}) {
			continue
		}
		for _, targetRef := range targetRefs {
			if repairPatchTargetsShareDirectory(request.TargetRef, targetRef) {
				return true
			}
		}
	}
	return false
}

func primaryDiagnosticMeaningfulIdentifiers(contract map[string]any) []string {
	seen := map[string]bool{}
	var ret []string
	for _, value := range stringSliceFromValue(contract["identifiers"]) {
		trimmed := strings.TrimSpace(value)
		normalized := strings.ToLower(trimmed)
		if trimmed == "" || len([]rune(trimmed)) < 3 || seen[normalized] {
			continue
		}
		if primaryDiagnosticMetadataIdentifier(normalized) {
			continue
		}
		seen[normalized] = true
		ret = append(ret, trimmed)
	}
	return ret
}

func primaryDiagnosticMetadataIdentifier(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return true
	}
	switch value {
	case "fail", "failed", "failure", "failureclass", "failuresignature", "failuretargets", "exitcode", "status", "error", "reason", "stdout", "stderr":
		return true
	}
	return strings.Contains(value, "failure") ||
		strings.Contains(value, "failed") ||
		strings.HasPrefix(value, "exit") ||
		strings.HasSuffix(value, ".test")
}

func primaryDiagnosticPatchChangedText(request runtimePatchRequest) string {
	switch strings.TrimSpace(request.PatchType) {
	case "file_write":
		return primaryDiagnosticNonCommentText(request.Content)
	case "unified_diff":
		filePatch, err := parseSingleFileUnifiedDiff(request.Diff)
		if err != nil {
			return primaryDiagnosticNonCommentText(request.Diff)
		}
		var lines []string
		for _, hunk := range filePatch.Hunks {
			for _, line := range hunk.Lines {
				if line == "" {
					continue
				}
				switch line[0] {
				case '+', '-':
					lines = append(lines, strings.TrimSpace(line[1:]))
				}
			}
		}
		return primaryDiagnosticNonCommentText(strings.Join(lines, "\n"))
	default:
		return ""
	}
}

func primaryDiagnosticNonCommentText(value string) string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || primaryDiagnosticLooksCommentOnly(line) {
			continue
		}
		lines = append(lines, line)
	}
	return strings.ToLower(strings.Join(lines, "\n"))
}

func primaryDiagnosticLooksCommentOnly(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "//") ||
		strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, "--") ||
		strings.HasPrefix(line, "/*") ||
		strings.HasPrefix(line, "*") ||
		strings.HasPrefix(line, "<!--")
}

func primaryDiagnosticRequestTargetsDiagnosticScope(request runtimePatchRequest, targetRefs []string) bool {
	if len(targetRefs) == 0 {
		return true
	}
	if repairPatchTargetRefMatches(request.TargetRef, targetRefs) {
		return true
	}
	return repairPatchRequestDeclaresRelatedFailureTargets(request, targetRefs)
}

func primaryDiagnosticRequestDiffOverlapsLineRefs(request runtimePatchRequest, lineRefs []map[string]any) bool {
	if strings.TrimSpace(request.PatchType) != "unified_diff" || len(lineRefs) == 0 {
		return false
	}
	filePatch, err := parseSingleFileUnifiedDiff(request.Diff)
	if err != nil {
		return false
	}
	for _, ref := range lineRefs {
		if !repairPatchTargetRefMatches(request.TargetRef, []string{stringFromMap(ref, "targetRef")}) {
			continue
		}
		line := intFromAny(ref["line"])
		if line <= 0 {
			continue
		}
		for _, hunk := range filePatch.Hunks {
			if primaryDiagnosticHunkOverlapsLine(hunk, line) {
				return true
			}
		}
	}
	return false
}

func primaryDiagnosticHunkOverlapsLine(hunk unifiedDiffHunk, line int) bool {
	if line <= 0 || !hunk.HasOldStart {
		return false
	}
	oldLineCount := 0
	for _, item := range hunk.Lines {
		if item == "" {
			continue
		}
		switch item[0] {
		case ' ', '-':
			oldLineCount++
		}
	}
	if oldLineCount <= 0 {
		oldLineCount = 1
	}
	const margin = 2
	start := hunk.OldStart - margin
	end := hunk.OldStart + oldLineCount + margin
	return line >= start && line <= end
}

func primaryDiagnosticRequestFileWriteChangesLineRefs(step *runModel.Step, request runtimePatchRequest, lineRefs []map[string]any) bool {
	if strings.TrimSpace(request.PatchType) != "file_write" || len(lineRefs) == 0 || strings.TrimSpace(request.Content) == "" {
		return false
	}
	for _, ref := range lineRefs {
		targetRef := stringFromMap(ref, "targetRef")
		if !repairPatchTargetRefMatches(request.TargetRef, []string{targetRef}) {
			continue
		}
		line := intFromAny(ref["line"])
		if line <= 0 {
			continue
		}
		content, ok := currentRepairSnapshotContentForTarget(step, targetRef)
		if !ok {
			continue
		}
		if primaryDiagnosticFileWriteChangesLine(content, request.Content, line) {
			return true
		}
	}
	return false
}

func currentRepairSnapshotContentForTarget(step *runModel.Step, targetRef string) (string, bool) {
	if step == nil || step.Input == nil {
		return "", false
	}
	for _, item := range repairPatchContextItems(step) {
		if item == nil || !repairPatchTargetRefMatches(stringFromMap(item, "targetRef"), []string{targetRef}) {
			continue
		}
		content := stringFromMap(item, "content")
		if content != "" && !containsRuntimeRedactionMarker(content) {
			return content, true
		}
	}
	actionPackage := mapFromValue(step.Input["repairActionPackage"])
	for _, item := range mapSliceFromValue(actionPackage["targetSnapshots"]) {
		if item == nil || !repairPatchTargetRefMatches(stringFromMap(item, "targetRef"), []string{targetRef}) {
			continue
		}
		content := stringFromMap(item, "content")
		if content != "" && !containsRuntimeRedactionMarker(content) {
			return content, true
		}
	}
	return "", false
}

func primaryDiagnosticFileWriteChangesLine(oldContent string, newContent string, oneBasedLine int) bool {
	if oneBasedLine <= 0 {
		return false
	}
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")
	idx := oneBasedLine - 1
	if idx >= len(oldLines) {
		return false
	}
	oldLine := oldLines[idx]
	if idx >= len(newLines) {
		return true
	}
	if newLines[idx] != oldLine {
		return true
	}
	return strings.TrimSpace(oldLine) != "" && !strings.Contains(newContent, oldLine)
}

func primaryDiagnosticRequestHasMatchingDeclaration(request runtimePatchRequest, contract map[string]any) bool {
	targetRefs := stringSliceFromValue(contract["targetRefs"])
	identifiers := stringSliceFromValue(contract["identifiers"])
	lineRefs := mapSliceFromValue(contract["lineRefs"])
	for _, key := range []string{"primaryDiagnosticResolution", "primaryDiagnosticResolutions", "addressesFailureSignals"} {
		if primaryDiagnosticDeclarationMatches(request.Extra[key], request, targetRefs, identifiers, lineRefs) {
			return true
		}
	}
	return false
}

func primaryDiagnosticDeclarationMatches(value any, request runtimePatchRequest, targetRefs []string, identifiers []string, lineRefs []map[string]any) bool {
	switch typed := value.(type) {
	case map[string]any:
		return primaryDiagnosticDeclarationMapMatches(typed, request, targetRefs, identifiers, lineRefs)
	case []map[string]any:
		for _, item := range typed {
			if primaryDiagnosticDeclarationMapMatches(item, request, targetRefs, identifiers, lineRefs) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if primaryDiagnosticDeclarationMatches(item, request, targetRefs, identifiers, lineRefs) {
				return true
			}
		}
	case string:
		text := strings.ToLower(strings.TrimSpace(typed))
		if text == "" {
			return false
		}
		return primaryDiagnosticTextMentionsAny(text, targetRefs) &&
			(primaryDiagnosticTextMentionsAny(text, identifiers) || primaryDiagnosticTextMentionsLineRef(text, lineRefs))
	}
	return false
}

func primaryDiagnosticDeclarationMapMatches(item map[string]any, request runtimePatchRequest, targetRefs []string, identifiers []string, lineRefs []map[string]any) bool {
	if len(item) == 0 {
		return false
	}
	targetMatched := false
	for _, key := range []string{"targetRef", "failureTarget", "relatedFailureTarget"} {
		if repairPatchTargetRefMatches(stringFromMap(item, key), targetRefs) {
			targetMatched = true
			break
		}
	}
	if !targetMatched {
		for _, value := range stringSliceFromValue(item["targetRefs"]) {
			if repairPatchTargetRefMatches(value, targetRefs) {
				targetMatched = true
				break
			}
		}
	}
	if !targetMatched && repairPatchTargetRefMatches(request.TargetRef, targetRefs) {
		targetMatched = true
	}
	if !targetMatched {
		return false
	}
	lineMatched := false
	if line := intFromAny(item["line"]); line > 0 {
		for _, ref := range lineRefs {
			if line == intFromAny(ref["line"]) {
				lineMatched = true
				break
			}
		}
	}
	identifierMatched := primaryDiagnosticTextMentionsAny(strings.ToLower(strings.TrimSpace(strings.Join([]string{
		stringFromAny(item["identifier"]),
		stringFromAny(item["identifiers"]),
		stringFromAny(item["summary"]),
		stringFromAny(item["reason"]),
	}, "\n"))), identifiers)
	return lineMatched || identifierMatched
}

func primaryDiagnosticRequestMentionsIdentifiers(request runtimePatchRequest, contract map[string]any) bool {
	text := strings.ToLower(strings.Join([]string{
		request.Summary,
		request.Content,
		request.Diff,
		stringFromAny(request.Extra["summary"]),
		stringFromAny(request.Extra["reason"]),
	}, "\n"))
	return primaryDiagnosticTextMentionsAny(text, stringSliceFromValue(contract["identifiers"]))
}

func primaryDiagnosticTextMentionsAny(text string, values []string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" && strings.Contains(text, value) {
			return true
		}
	}
	return false
}

func primaryDiagnosticTextMentionsLineRef(text string, lineRefs []map[string]any) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	for _, ref := range lineRefs {
		target := strings.ToLower(strings.TrimSpace(stringFromMap(ref, "targetRef")))
		line := intFromAny(ref["line"])
		if target == "" || line <= 0 {
			continue
		}
		if strings.Contains(text, target) && strings.Contains(text, strconv.Itoa(line)) {
			return true
		}
	}
	return false
}

func validateRepeatedRepairRootCauseTargetConsistency(step *runModel.Step, request runtimePatchRequest) error {
	if step == nil || step.Input == nil {
		return nil
	}
	reassessment := mapFromValue(step.Input["strategyReassessment"])
	if !boolValue(reassessment["required"]) {
		return nil
	}
	if !strategyReassessmentRequiresRootCauseTargetShift(reassessment) {
		return nil
	}
	declaredRootCauseTargets := rootCauseTargetRefsFromStrategyValue(request.Extra["rootCauseTarget"])
	if len(declaredRootCauseTargets) == 0 {
		return nil
	}
	targetRef := normalizeRepairPatchTargetRef(request.TargetRef)
	if targetRef == "" {
		return nil
	}
	if repairPatchTargetRefMatches(targetRef, declaredRootCauseTargets) {
		return nil
	}
	return fmt.Errorf("%w: strategyReassessment.required=true requires patch target %q to match declared rootCauseTarget %q when repeated failed-surface repair must shift targets", common.ErrInvalidArgument, request.TargetRef, strings.TrimSpace(stringFromAny(request.Extra["rootCauseTarget"])))
}

func rootCauseTargetRefsFromStrategyValue(value any) []string {
	text := strings.TrimSpace(stringFromAny(value))
	if text == "" {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		value = normalizeRepairPatchTargetRef(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, target := range repairPatchTargetRefsFromScopeText(text) {
		add(target)
	}
	for _, target := range extractStructuredFailureTargetRefs(text) {
		add(target)
	}
	for _, ref := range strings.Fields(text) {
		ref = strings.Trim(ref, "`'\"()[]{}<>;,")
		if strings.Contains(ref, ".") {
			add(ref)
		}
	}
	return ret
}

func validateLLMRepairStrategyReassessmentContract(step *runModel.Step, output map[string]any) error {
	if step == nil || step.Input == nil || output == nil {
		return nil
	}
	if !stateChangingRepairContractActive(step) {
		return nil
	}
	reassessment := mapFromValue(step.Input["strategyReassessment"])
	if !boolValue(reassessment["required"]) {
		return nil
	}
	candidates, hasPatch := strategyReassessmentDecisionCandidates(output)
	if !hasPatch {
		return nil
	}
	requiredFields := stringSliceFromValue(reassessment["requiredDecisionFields"])
	if len(requiredFields) == 0 {
		requiredFields = []string{"failedSurface", "rootCauseTarget", "repairTargetDecision", "forbiddenRepair", "verificationPlan"}
	}
	expectedSurface := strings.TrimSpace(stringFromMap(reassessment, "failedSurface"))
	for _, candidate := range candidates {
		if len(candidate) == 0 {
			continue
		}
		if !strategyCandidateHasRequiredFields(candidate, requiredFields) {
			continue
		}
		if expectedSurface != "" && !strategyFailedSurfaceMatches(candidate, expectedSurface) {
			continue
		}
		surfaceCorrection := strategyCandidateHasSurfaceContractCorrection(candidate, expectedSurface)
		if !strategyRootCauseDistinguishableFromFailedSurface(candidate, expectedSurface) && !surfaceCorrection {
			continue
		}
		if strategyReassessmentRequiresRootCauseTargetShift(reassessment) &&
			strategyCandidatePatchesFailedSurface(candidate, expectedSurface) && !surfaceCorrection {
			return fmt.Errorf("%w: strategyReassessment.required=true requires repeated failed-surface repairs to patch a distinct root-cause target or return a failed assessment/blocker", common.ErrInvalidArgument)
		}
		if strategyFailedSurfaceRequiresBoundedVerification(expectedSurface, candidate) &&
			(strategyVerificationPlanAppearsUnbounded(strings.TrimSpace(stringFromAny(candidate["verificationPlan"]))) ||
				strategyOutputGraphUsesUnboundedVerification(output)) {
			return fmt.Errorf("%w: strategyReassessment.required=true requires bounded verification for scoped failed surface; unbounded verification without explicit target scope is not allowed in this repair decision", common.ErrInvalidArgument)
		}
		return nil
	}
	return fmt.Errorf("%w: strategyReassessment.required=true requires state-changing patch output to declare failedSurface/rootCauseTarget/repairTargetDecision/forbiddenRepair/verificationPlan aligned with current failed surface", common.ErrInvalidArgument)
}

func strategyCandidateHasSurfaceContractCorrection(candidate map[string]any, expectedSurface string) bool {
	if len(candidate) == 0 {
		return false
	}
	correction := mapFromValue(candidate["surfaceContractCorrection"])
	if len(correction) == 0 {
		return false
	}
	if declaredSurface := strings.TrimSpace(stringFromAny(correction["failedSurface"])); declaredSurface != "" {
		if expectedSurface != "" && !strategyFailedSurfaceTextMatches(declaredSurface, expectedSurface) {
			return false
		}
		if parentSurface := strings.TrimSpace(stringFromAny(candidate["failedSurface"])); parentSurface != "" && !strategyFailedSurfaceTextMatches(declaredSurface, parentSurface) {
			return false
		}
	}
	for _, field := range []string{"whySurfaceIsWrong", "contractConflict", "currentResourceEvidence", "boundedVerificationPlan"} {
		if !surfaceContractCorrectionFieldPresent(correction[field]) {
			return false
		}
	}
	if strategyVerificationPlanAppearsUnbounded(strings.TrimSpace(stringFromAny(correction["boundedVerificationPlan"]))) {
		return false
	}
	return true
}

func surfaceContractCorrectionFieldPresent(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []string:
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				return true
			}
		}
		return false
	case []any:
		for _, item := range typed {
			if surfaceContractCorrectionFieldPresent(item) {
				return true
			}
		}
		return false
	case map[string]any:
		return len(typed) > 0
	case []map[string]any:
		return len(typed) > 0
	default:
		return false
	}
}

func strategyReassessmentDecisionCandidates(output map[string]any) ([]map[string]any, bool) {
	var candidates []map[string]any
	hasPatch := false
	appendCandidate := func(value map[string]any) {
		if len(value) > 0 {
			candidates = append(candidates, value)
		}
	}
	if requests, err := runtimePatchRequests(output); err == nil {
		for _, request := range requests {
			hasPatch = true
			appendCandidate(mapFromValue(request.Extra))
		}
	}
	if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
		for _, node := range graph.Nodes {
			if strings.TrimSpace(node.Kind) != "patch/apply" {
				continue
			}
			if _, err := runtimePatchRequestFromMap(node.Input); err != nil {
				continue
			}
			hasPatch = true
			appendCandidate(mapFromValue(node.Input))
		}
	}
	_, nextKind := nextStepSpec(output)
	if strings.TrimSpace(nextKind) == "patch/apply" {
		nextInput := mapFromValue(output[stepOutputKeyNextInput])
		if _, err := runtimePatchRequestFromMap(nextInput); err == nil {
			hasPatch = true
			appendCandidate(nextInput)
		}
	}
	return candidates, hasPatch
}

func strategyCandidateHasRequiredFields(candidate map[string]any, required []string) bool {
	for _, field := range required {
		if strings.TrimSpace(stringFromAny(candidate[field])) == "" {
			return false
		}
	}
	return true
}

func strategyFailedSurfaceMatches(candidate map[string]any, expected string) bool {
	got := strings.TrimSpace(stringFromAny(candidate["failedSurface"]))
	return strategyFailedSurfaceTextMatches(got, expected)
}

func strategyFailedSurfaceTextMatches(got string, expected string) bool {
	got = normalizeStrategyContractText(got)
	expected = normalizeStrategyContractText(expected)
	if got == "" || expected == "" {
		return false
	}
	return got == expected || strings.Contains(got, expected) || strings.Contains(expected, got)
}

func strategyRootCauseDistinguishableFromFailedSurface(candidate map[string]any, expectedSurface string) bool {
	rootCause := normalizeStrategyContractText(strings.TrimSpace(stringFromAny(candidate["rootCauseTarget"])))
	if rootCause == "" {
		return false
	}
	failedSurface := strings.TrimSpace(stringFromAny(candidate["failedSurface"]))
	if failedSurface == "" {
		failedSurface = expectedSurface
	}
	failedSurface = normalizeStrategyContractText(failedSurface)
	if failedSurface == "" {
		return true
	}
	return rootCause != failedSurface
}

func strategyReassessmentRequiresRootCauseTargetShift(reassessment map[string]any) bool {
	if len(reassessment) == 0 {
		return false
	}
	if int64FromValue(reassessment["targetRepairAttempts"]) >= 2 {
		return true
	}
	reason := strings.ToLower(strings.TrimSpace(stringFromAny(reassessment["reason"])))
	return strings.Contains(reason, "same_target") || strings.Contains(reason, "repeated")
}

func strategyCandidatePatchesFailedSurface(candidate map[string]any, expectedSurface string) bool {
	if len(candidate) == 0 {
		return false
	}
	targetRef := normalizeRepairPatchTargetRef(strings.TrimSpace(stringFromAny(candidate["targetRef"])))
	if targetRef == "" {
		return false
	}
	surface := strings.TrimSpace(stringFromAny(candidate["failedSurface"]))
	if surface == "" {
		surface = expectedSurface
	}
	for _, surfaceTarget := range repairPatchTargetRefsFromScopeText(surface) {
		if repairPatchTargetRefMatches(targetRef, []string{surfaceTarget}) {
			return true
		}
	}
	return false
}

func strategyFailedSurfaceRequiresBoundedVerification(expectedSurface string, candidate map[string]any) bool {
	surface := strings.TrimSpace(stringFromAny(candidate["failedSurface"]))
	if surface == "" {
		surface = expectedSurface
	}
	surface = strings.ToLower(surface)
	if surface == "" {
		return false
	}
	return strings.Contains(surface, "failuretargets=") || strings.Contains(surface, "targetref=")
}

func strategyVerificationPlanAppearsUnbounded(plan string) bool {
	lower := strings.ToLower(strings.TrimSpace(plan))
	if lower == "" {
		return false
	}
	normalized := normalizeStrategyContractText(lower)
	return strings.Contains(lower, "full regression") ||
		strings.Contains(lower, "run all checks") ||
		strings.Contains(lower, "workspace wide") ||
		strings.Contains(lower, "entire workspace") ||
		strings.Contains(lower, "global scan") ||
		strings.Contains(normalized, "fullregression") ||
		strings.Contains(normalized, "runallchecks") ||
		strings.Contains(normalized, "workspacewide") ||
		strings.Contains(normalized, "entireworkspace") ||
		strings.Contains(normalized, "globalscan")
}

func strategyOutputGraphUsesUnboundedVerification(output map[string]any) bool {
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok || len(graph.Nodes) == 0 {
		return false
	}
	for _, node := range graph.Nodes {
		if strings.TrimSpace(node.Kind) != "shell/command" {
			continue
		}
		args := stringSliceFromValue(node.Input["args"])
		hasBroadScope := false
		hasExplicitScope := false
		for _, arg := range args {
			a := strings.ToLower(strings.TrimSpace(arg))
			if a == "" {
				continue
			}
			if a == "./..." || a == "..." || a == "*" || strings.Contains(a, "all") {
				hasBroadScope = true
			}
			if strings.HasPrefix(a, "--scope=") ||
				strings.HasPrefix(a, "--target=") ||
				strings.HasPrefix(a, "--path=") ||
				strings.HasPrefix(a, "--filter=") ||
				strings.HasPrefix(a, "--match=") ||
				strings.HasPrefix(a, "-run") {
				hasExplicitScope = true
			}
		}
		if hasBroadScope && !hasExplicitScope {
			return true
		}
	}
	return false
}

func normalizeStrategyContractText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "-", "", "_", "", "/", "", "\\", "", ".", "")
	return replacer.Replace(value)
}

func repairPatchRequestDeclaresRelatedFailureTargets(request runtimePatchRequest, allowed []string) bool {
	if len(allowed) == 0 {
		return false
	}
	for _, declared := range stringSliceFromValue(request.Extra["relatedFailureTargets"]) {
		if repairPatchTargetRefMatches(declared, allowed) {
			return true
		}
	}
	return false
}

func repairPatchTargetRefIsRelatedPrerequisite(input map[string]any, targetRef string, allowed []string) bool {
	targetRef = normalizeRepairPatchTargetRef(targetRef)
	if len(input) == 0 || targetRef == "" || len(allowed) == 0 {
		return false
	}
	if !repairFailureIndicatesDependencyMetadataPrerequisite(input) || !dependencyMetadataTargetRef(targetRef) {
		return false
	}
	for _, candidate := range allowed {
		if repairPatchTargetsShareDirectory(targetRef, candidate) {
			return true
		}
	}
	return false
}

func repairFailureIndicatesDependencyMetadataPrerequisite(input map[string]any) bool {
	texts := []string{
		stringFromMap(input, "failureReason"),
		stringFromMap(input, "failureTargetScope"),
		stringFromMap(input, failureOutputKeySignature),
		stringFromMap(input, stepInputKeyRepairOfFailureSig),
	}
	for _, key := range []string{"failedStepOutput", "previousStepOutput", "structuredFailureSignals", "repairActionPackage"} {
		if text := flattenCoverageEvidenceText(input[key], 0); text != "" {
			texts = append(texts, text)
		}
	}
	text := strings.ToLower(strings.Join(texts, "\n"))
	if text == "" {
		return false
	}
	hasFailure := strings.Contains(text, "missing") ||
		strings.Contains(text, "not found") ||
		strings.Contains(text, "no such") ||
		strings.Contains(text, "required") ||
		strings.Contains(text, "checksum") ||
		strings.Contains(text, "lock")
	hasPrerequisite := strings.Contains(text, "depend") ||
		strings.Contains(text, "module") ||
		strings.Contains(text, "package") ||
		strings.Contains(text, "manifest") ||
		strings.Contains(text, "metadata") ||
		strings.Contains(text, "checksum") ||
		strings.Contains(text, "lock")
	return hasFailure && hasPrerequisite
}

func dependencyMetadataTargetRef(targetRef string) bool {
	targetRef = normalizeRepairPatchTargetRef(targetRef)
	if targetRef == "" {
		return false
	}
	base := strings.ToLower(filepath.Base(targetRef))
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(base)), ".")
	stem := strings.TrimSuffix(base, "."+ext)
	for _, token := range []string{
		"dependency",
		"dependencies",
		"deps",
		"lock",
		"checksum",
		"checksums",
		"sum",
		"sums",
		"manifest",
		"module",
		"modules",
		"package",
		"packages",
		"requirements",
	} {
		if base == token || stem == token || ext == token ||
			strings.Contains(base, token+".") ||
			strings.Contains(base, "."+token) ||
			strings.Contains(base, "-"+token) ||
			strings.Contains(base, "_"+token) {
			return true
		}
	}
	return false
}

func repairPatchTargetsShareDirectory(targetRef string, candidate string) bool {
	targetRef = normalizeRepairPatchTargetRef(targetRef)
	candidate = normalizeRepairPatchTargetRef(candidate)
	if targetRef == "" || candidate == "" {
		return false
	}
	targetDir := filepath.ToSlash(filepath.Dir(targetRef))
	candidateDir := filepath.ToSlash(filepath.Dir(candidate))
	if targetDir == "." {
		targetDir = ""
	}
	if candidateDir == "." {
		candidateDir = ""
	}
	return targetDir == candidateDir
}

func validateLLMCoverageGapRepairIntent(step *runModel.Step, output map[string]any) error {
	if !coverageGapStateChangingRepairRequired(step) {
		return nil
	}
	if coverageGapRepairOutputHasPatch(output) && coverageGapRepairOutputDeclaresTargets(step, output) {
		return nil
	}
	if !coverageGapRepairOutputHasPatch(output) {
		return nil
	}
	return fmt.Errorf("%w: coverage-gap state-changing repair patch/apply must declare coverageTargets matching missing DoD or Goal coverage items", common.ErrInvalidArgument)
}

func coverageGapRepairOutputHasPatch(output map[string]any) bool {
	if output == nil {
		return false
	}
	if outputItemCount(output[stepOutputKeyPatches]) > 0 ||
		outputItemCount(output["patchRequests"]) > 0 ||
		outputItemCount(mapFromValue(output[stepOutputKeyLLMDecision])["patchRequests"]) > 0 {
		return true
	}
	_, nextKind := nextStepSpec(output)
	if strings.TrimSpace(nextKind) == "patch/apply" {
		return true
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok {
		return false
	}
	for _, node := range graph.Nodes {
		if strings.TrimSpace(node.Kind) == "patch/apply" {
			return true
		}
	}
	return false
}

func coverageGapRepairOutputDeclaresTargets(step *runModel.Step, output map[string]any) bool {
	allowedTargets := coverageGapRepairTargetTokens(step)
	if len(allowedTargets) == 0 {
		return true
	}
	if patchPayloadsDeclareCoverageTargets(output[stepOutputKeyPatches], allowedTargets) ||
		patchPayloadsDeclareCoverageTargets(output["patchRequests"], allowedTargets) ||
		patchPayloadsDeclareCoverageTargets(mapFromValue(output[stepOutputKeyLLMDecision])["patchRequests"], allowedTargets) {
		return true
	}
	_, nextKind := nextStepSpec(output)
	if strings.TrimSpace(nextKind) == "patch/apply" && patchNodeDeclaresCoverageTargets(mapFromValue(output[stepOutputKeyNextInput]), allowedTargets) {
		return true
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if ok {
		for _, node := range graph.Nodes {
			if strings.TrimSpace(node.Kind) != "patch/apply" {
				continue
			}
			if patchNodeDeclaresCoverageTargets(node.Input, allowedTargets) {
				return true
			}
		}
	}
	return false
}

func patchPayloadsDeclareCoverageTargets(value any, allowed []string) bool {
	for _, item := range mapSliceFromValue(value) {
		if patchNodeDeclaresCoverageTargets(item, allowed) {
			return true
		}
	}
	return false
}

func coverageGapStateChangingRepairRequired(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	if !boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) ||
		!boolValue(step.Input["stateChangingOutputRequired"]) ||
		!repairObjectiveRequiresStateChange(stringFromMap(step.Input, "repairObjective")) {
		return false
	}
	return len(mapSliceFromValue(step.Input["missingDoDItems"])) > 0 || len(missingGoalCoverageFromInput(step.Input)) > 0
}

func coverageGapRepairTargetTokens(step *runModel.Step) []string {
	if step == nil || step.Input == nil {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0)
	add := func(value string) {
		value = normalizeCoverageGapRepairToken(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, item := range mapSliceFromValue(step.Input["missingDoDItems"]) {
		add(stringFromMap(item, "item"))
	}
	for _, item := range missingGoalCoverageFromInput(step.Input) {
		add(firstNonEmptyString(
			stringFromMap(item, "goalRequirementId"),
			stringFromMap(item, "item"),
			missingGoalCoverageRequirementText(item),
		))
	}
	return ret
}

func patchNodeDeclaresCoverageTargets(input map[string]any, allowed []string) bool {
	if len(input) == 0 || len(allowed) == 0 {
		return false
	}
	for _, value := range stringSliceFromValue(input["coverageTargets"]) {
		value = normalizeCoverageGapRepairToken(value)
		if value == "" {
			continue
		}
		for _, candidate := range allowed {
			if value == candidate {
				return true
			}
		}
	}
	return false
}

func normalizeCoverageGapRepairToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Join(strings.Fields(value), " ")
	return value
}

func repairPatchTargetRelevanceRequired(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	return stateChangingRepairContractActive(step)
}

func stateChangingRepairContractActive(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	if runtimeStageFromStepInput(step.Input) != runtimeStageRepair {
		return false
	}
	if !boolValue(step.Input["stateChangingOutputRequired"]) {
		return false
	}
	return repairObjectiveRequiresStateChange(stringFromMap(step.Input, "repairObjective"))
}

func repairPatchRelevantTargetRefs(step *runModel.Step) []string {
	if step == nil || step.Input == nil {
		return nil
	}
	return repairPatchRelevantTargetRefsFromInput(step.Input)
}

func repairPatchRelevantTargetRefsFromInput(input map[string]any) []string {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		value = normalizeRepairPatchTargetRef(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	addFromItems := func(items []map[string]any) {
		for _, item := range items {
			add(stringFromMap(item, "targetRef"))
		}
	}
	addFromScopeText := func(value string) {
		for _, targetRef := range repairPatchTargetRefsFromScopeText(value) {
			add(targetRef)
		}
	}

	add(stringFromMap(input, "targetRef"))
	addFromItems(mapSliceFromValue(input["failureTargetRepairContext"]))
	addFromItems(mapSliceFromValue(input["patchRepairContext"]))

	actionPackage := mapFromValue(input["repairActionPackage"])
	addFromItems(mapSliceFromValue(actionPackage["targetSnapshots"]))
	for _, key := range []string{"failureTargetScope", failureOutputKeySignature, stepInputKeyRepairOfFailureSig} {
		addFromScopeText(stringFromMap(input, key))
		addFromScopeText(stringFromMap(actionPackage, key))
	}
	for _, nestedKey := range []string{"failedStepInput", "failedStepOutput", "previousStepOutput"} {
		nested := mapFromValue(input[nestedKey])
		add(stringFromMap(nested, "targetRef"))
		for _, key := range []string{"failureTargetScope", failureOutputKeySignature, stepInputKeyRepairOfFailureSig} {
			addFromScopeText(stringFromMap(nested, key))
		}
	}
	return ret
}

func repairPatchTargetRefsFromScopeText(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		value = normalizeRepairPatchTargetRef(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, segment := range strings.Split(value, "|") {
		key, raw, ok := strings.Cut(segment, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "failuretargets", "targetref", "conflictref":
			for _, item := range strings.Split(raw, ",") {
				add(item)
			}
		}
	}
	return ret
}

func repairPatchTargetRefMatches(targetRef string, allowed []string) bool {
	targetRef = normalizeRepairPatchTargetRef(targetRef)
	if targetRef == "" {
		return false
	}
	for _, candidate := range allowed {
		candidate = normalizeRepairPatchTargetRef(candidate)
		if candidate == "" {
			continue
		}
		if targetRef == candidate || strings.HasSuffix(targetRef, "/"+candidate) {
			return true
		}
	}
	return false
}

func normalizeRepairPatchTargetRef(value string) string {
	value = strings.TrimSpace(strings.Trim(value, `"'()[]{}<>`))
	value = strings.ReplaceAll(value, `\`, `/`)
	value = strings.TrimPrefix(value, "./")
	if value == "" {
		return ""
	}
	value = filepath.ToSlash(filepath.Clean(value))
	value = strings.TrimPrefix(value, "./")
	if value == "." {
		return ""
	}
	return value
}

func repairPatchContextItems(step *runModel.Step) []map[string]any {
	if step == nil || step.Input == nil {
		return nil
	}
	ret := mapSliceFromValue(step.Input["patchRepairContext"])
	ret = append(ret, mapSliceFromValue(step.Input["failureTargetRepairContext"])...)
	return ret
}

func validateLLMFollowUpGraphStateChangingContract(run *runModel.Run, step *runModel.Step, output map[string]any) error {
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok {
		return nil
	}
	stage := runtimeStageFromStepInput(step.Input)
	if boolValue(step.Input[stepInputKeyAssessmentOnly]) || boolValue(step.Input["providerRetryAssessmentOnly"]) {
		return nil
	}
	if graphHasStateChangingPatchNode(graph) {
		return validateStateChangingExecutionGraph(graph)
	}
	if hasStateChangingLLMExecutionOutput(output) {
		return nil
	}
	if stage == runtimeStageVerification && graphIsConcreteValidationEvidenceGraph(graph) {
		return nil
	}
	if llmFollowUpGraphRequiresStateChangingPatch(step) || stage == runtimeStageVerification {
		return fmt.Errorf("%w: validation follow-up llm step returned read-only executionGraph without patch output", common.ErrInvalidArgument)
	}
	return nil
}

func llmFollowUpGraphRequiresStateChangingPatch(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	return boolValue(step.Input["graphFollowUp"]) ||
		boolValue(step.Input["completionFollowUp"]) ||
		boolValue(step.Input["continuationFollowUp"]) ||
		boolValue(step.Input[stepInputKeyGraphLimitFollowUp])
}

func runExecutionContractRequiresStateChange(run *runModel.Run) bool {
	if run == nil {
		return false
	}
	executionContract := mapFromValue(run.ExecutionContext["executionContract"])
	return strings.TrimSpace(stringFromMap(executionContract, "stateChangePolicy")) == "must_produce_state_change"
}

func graphHasStateChangingPatchNode(graph runtimemodel.BlockExecutionGraph) bool {
	for _, node := range graph.Nodes {
		switch strings.TrimSpace(node.Kind) {
		case "patch/apply":
			if _, err := runtimePatchRequestFromMap(node.Input); err == nil {
				return true
			}
		}
	}
	return false
}

func validateStateChangingExecutionGraph(graph runtimemodel.BlockExecutionGraph) error {
	if !graphHasStateChangingPatchNode(graph) {
		return fmt.Errorf("%w: state-changing llm repair step returned executionGraph without patch/apply; read-only, inspection-only, or shell-only graphs cannot satisfy the state-changing contract", common.ErrInvalidArgument)
	}
	if !graphHasUngatedStateChangingPatchNode(graph) {
		return fmt.Errorf("%w: state-changing executionGraph patch/apply depends on llm/workspace_execution before patch", common.ErrInvalidArgument)
	}
	if graphHasPatchApplyDependingOnSameGraphRead(graph) {
		return fmt.Errorf("%w: state-changing executionGraph patch/apply must not depend on same-graph file/read", common.ErrInvalidArgument)
	}
	return nil
}

func graphIsConcreteStateChangingContinuation(graph runtimemodel.BlockExecutionGraph) bool {
	if graphHasNodeKind(graph, llmWorkspaceExecutionStepKind) {
		return false
	}
	return validateStateChangingExecutionGraph(graph) == nil
}

func graphHasNodeKind(graph runtimemodel.BlockExecutionGraph, kind string) bool {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return false
	}
	for _, node := range graph.Nodes {
		if strings.TrimSpace(node.Kind) == kind {
			return true
		}
	}
	return false
}

func allowPatchRepairValidationEvidenceGraph(step *runModel.Step, graph runtimemodel.BlockExecutionGraph) bool {
	if step == nil || step.Input == nil || runtimeStageFromStepInput(step.Input) != runtimeStageRepair {
		return false
	}
	if !boolValue(step.Input["stateChangingOutputRequired"]) || !repairObjectiveRequiresStateChange(stringFromMap(step.Input, "repairObjective")) {
		return false
	}
	if len(mapSliceFromValue(step.Input["patchRepairContext"])) == 0 || !stepRepairsPatchCapabilityFailure(step) {
		return false
	}
	return graphIsConcreteValidationEvidenceGraph(graph)
}

func stepRepairsPatchCapabilityFailure(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	failedKind := strings.TrimSpace(stringFromMap(step.Input, "failedStepKind"))
	if failedKind == "patch/preview" || failedKind == "patch/apply" {
		return true
	}
	failedOutput := mapFromValue(step.Input["failedStepOutput"])
	stage := strings.TrimSpace(stringFromMap(failedOutput, "stage"))
	if stage == "patch_preview" || strings.HasPrefix(stage, "patch_apply") {
		return true
	}
	signature := strings.ToLower(strings.TrimSpace(stringFromMap(failedOutput, failureOutputKeySignature)))
	return strings.Contains(signature, "kind=patch/preview") ||
		strings.Contains(signature, "kind=patch/apply") ||
		strings.Contains(signature, "class="+failureClassPatchApply)
}

func graphIsConcreteValidationEvidenceGraph(graph runtimemodel.BlockExecutionGraph) bool {
	if len(graph.Nodes) == 0 {
		return false
	}
	hasEvidenceProducer := false
	for _, node := range graph.Nodes {
		switch strings.TrimSpace(node.Kind) {
		case "file/read", "file/list", "mcp/resource_read", "mcp/prompt_get":
			continue
		case "shell/command", "mcp/tool_call":
			hasEvidenceProducer = true
		case "patch/preview", "patch/apply", llmWorkspaceExecutionStepKind:
			return false
		default:
			return false
		}
	}
	return hasEvidenceProducer
}

func graphHasUngatedStateChangingPatchNode(graph runtimemodel.BlockExecutionGraph) bool {
	nodes := map[string]runtimemodel.BlockExecutionNode{}
	for _, node := range graph.Nodes {
		id := strings.TrimSpace(node.ID)
		if id != "" {
			nodes[id] = node
		}
	}
	for _, node := range graph.Nodes {
		if strings.TrimSpace(node.Kind) != "patch/apply" {
			continue
		}
		if _, err := runtimePatchRequestFromMap(node.Input); err != nil {
			continue
		}
		if !graphNodeHasAncestorKind(node, nodes, llmWorkspaceExecutionStepKind, map[string]bool{}) {
			return true
		}
	}
	return false
}

func graphNodeHasAncestorKind(node runtimemodel.BlockExecutionNode, nodes map[string]runtimemodel.BlockExecutionNode, kind string, seen map[string]bool) bool {
	for _, dep := range node.DependsOn {
		dep = strings.TrimSpace(dep)
		if dep == "" || seen[dep] {
			continue
		}
		seen[dep] = true
		parent, ok := nodes[dep]
		if !ok {
			continue
		}
		if strings.TrimSpace(parent.Kind) == kind {
			return true
		}
		if graphNodeHasAncestorKind(parent, nodes, kind, seen) {
			return true
		}
	}
	return false
}

func graphHasPatchApplyDependingOnSameGraphRead(graph runtimemodel.BlockExecutionGraph) bool {
	nodes := map[string]runtimemodel.BlockExecutionNode{}
	for _, node := range graph.Nodes {
		id := strings.TrimSpace(node.ID)
		if id != "" {
			nodes[id] = node
		}
	}
	for _, node := range graph.Nodes {
		if strings.TrimSpace(node.Kind) != "patch/apply" {
			continue
		}
		if _, err := runtimePatchRequestFromMap(node.Input); err != nil {
			continue
		}
		if graphNodeHasAncestorKind(node, nodes, "file/read", map[string]bool{}) {
			return true
		}
	}
	return false
}

func llmExecutionGraphNodeReturnedOnlyGraph(run *runModel.Run, step *runModel.Step, output map[string]any) bool {
	if run == nil || step == nil || output == nil {
		return false
	}
	if step.Kind != initialStepKind(run) || graphNodeID(step) == "" {
		return false
	}
	if runtimeStageFromStepInput(step.Input) != runtimeStageExecution {
		return false
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok {
		return false
	}
	if graphIsConcreteStateChangingContinuation(graph) {
		return false
	}
	return !hasConcreteLLMExecutionOutput(output)
}

func hasConcreteLLMExecutionOutput(output map[string]any) bool {
	if output == nil {
		return false
	}
	if nextStepID, _ := nextStepSpec(output); nextStepID != "" {
		return true
	}
	if doDAssessmentFromOutput(output) != nil {
		return true
	}
	if hasStateChangingLLMExecutionOutput(output) {
		return true
	}
	if spec := decisionBlockSpec(output); spec.Reason != "" {
		return true
	}
	if spec := approvalBlockSpec(output); spec.Reason != "" {
		return true
	}
	return false
}

func graphNodeRequiresStateChangingOutput(step *runModel.Step) bool {
	return step != nil && boolValue(step.Input["stateChangingOutputRequired"])
}

func hasStateChangingLLMExecutionOutput(output map[string]any) bool {
	if output == nil {
		return false
	}
	if outputItemCount(output[stepOutputKeyPatches]) > 0 ||
		outputItemCount(output["patchRequests"]) > 0 ||
		outputItemCount(mapFromValue(output[stepOutputKeyLLMDecision])["patchRequests"]) > 0 {
		return true
	}
	_, nextKind := nextStepSpec(output)
	switch strings.TrimSpace(nextKind) {
	case "patch/apply":
		return true
	default:
		return false
	}
}

func validatePatchStepInputBeforeRuntime(kind string, input map[string]any) error {
	switch strings.TrimSpace(kind) {
	case "patch/preview", "patch/apply":
		request, err := runtimePatchRequestFromMap(input)
		if err != nil {
			return err
		}
		if request.PatchType == "unified_diff" {
			if _, err := parseSingleFileUnifiedDiff(request.Diff); err != nil {
				return wrapPatchApplyParseError(request.TargetRef, request.PatchType, err)
			}
		}
		return nil
	default:
		return nil
	}
}

func assessmentOnlyContractCorrectionStepID(runID common.RunID, step *runModel.Step) common.StepID {
	stepID := runtimeChildID(common.StepID(runtimeReadableStepLabel(runID, step.ID)), "assessment-contract-correction")
	if stepID == "" {
		stepID = "assessment-contract-correction"
	}
	return common.StepID(fmt.Sprintf("%s/assessment-correction-%s", runID, stepID))
}

func assessmentOnlyContractCorrectionStepInput(step *runModel.Step, output map[string]any, violation error) map[string]any {
	input := cloneStepPayload(step.Input)
	if input == nil {
		input = map[string]any{}
	}
	input[stepInputKeyAssessmentOnly] = true
	input["assessmentOnlyContractCorrection"] = true
	setRuntimeStage(input, runtimeStageVerification)
	input["assessmentOnlyContractViolationStepId"] = string(step.ID)
	input["assessmentOnlyContractViolationReason"] = errorString(violation)
	input["allowedAssessmentOnlyCompletionStates"] = []string{"completed", "failed", "blocked_on_decision", "blocked_on_approval"}
	stripAssessmentOnlyRecoveryMarkers(input)
	stripAssessmentOnlyCorrectionHistory(input)
	sanitizeAssessmentOnlyCorrectionFields(input)
	for _, key := range []string{"previousStepOutput", "failedStepOutput"} {
		if nested := mapFromValue(input[key]); len(nested) > 0 {
			sanitizeAssessmentOnlyCorrectionFields(nested)
			input[key] = nested
		}
	}
	if sanitized := cloneStepPayload(output); len(sanitized) > 0 {
		sanitizeAssessmentOnlyCorrectionFields(sanitized)
		sanitized["assessmentOnlyContractViolation"] = true
		input["previousInvalidAssessmentOutput"] = compactStepOutputForLLMContext(sanitized)
	}
	return input
}

func stripAssessmentOnlyRecoveryMarkers(input map[string]any) {
	if input == nil {
		return
	}
	for _, key := range []string{
		stepInputKeyProviderRetryFollowUp,
		stepInputKeyProviderRetryAttempt,
		stepInputKeyProviderRetryRootStep,
		"providerRetryAssessmentOnly",
		"providerRetryMode",
		stepInputKeyRuntimeRepairFollowUp,
		stepInputKeyRuntimeRepairAttempt,
		stepInputKeyRepairOfStepID,
		stepInputKeyRepairOfRootStepID,
		stepInputKeyRepairOfFailureClass,
		stepInputKeyRepairOfFailureSig,
		stepInputKeyRecoveryAction,
		stepInputKeyRecoveryRootStepID,
		stepInputKeyRecoveryLineageType,
		stepInputKeyRecoveryBudgetScope,
		"stateChangingOutputRequired",
		"repairObjective",
		"repairGuidance",
		"repairActionPackage",
		"patchRepairContext",
		"patchRepairSuggestions",
		"failureTargetRepairContext",
		"readableResourceRepairContext",
		"structuredFailureSignals",
		"targetRepairAttempts",
		"failureTargetScope",
	} {
		delete(input, key)
	}
}

func stripAssessmentOnlyCorrectionHistory(input map[string]any) {
	if input == nil {
		return
	}
	for _, key := range []string{
		"governanceHistory",
		"replanContinuation",
		"runtimeRepairContinuation",
		"providerRetryContinuation",
		"recoveryContinuation",
		"failedStepInput",
		"failedStepOutput",
		"previousStepOutput",
		"failedExecutionGraph",
		stepInputKeyGraphEvidence,
	} {
		delete(input, key)
	}
}

func sanitizeAssessmentOnlyCorrectionFields(payload map[string]any) {
	sanitizeAssessmentOnlyExecutableFields(payload)
	stripAssessmentOnlyCorrectionDecisionAndRecoveryFields(payload)
}

func stripAssessmentOnlyCorrectionDecisionAndRecoveryFields(payload map[string]any) {
	if payload == nil {
		return
	}
	for _, key := range []string{
		stepOutputKeyLLMDecision,
		"convertedDecision",
		"requiredDecision",
		"requiredApproval",
		stepOutputKeyBlockDecisionReason,
		stepOutputKeyBlockDecisionTicketID,
		stepOutputKeyBlockDecisionKind,
		stepOutputKeyBlockDecisionQuestion,
		stepOutputKeyBlockDecisionContext,
		"governanceHistory",
		"replanContinuation",
		"runtimeRepairContinuation",
		"providerRetryContinuation",
		"recoveryContinuation",
		"resolutionPayload",
		"ticketContext",
		"recoveryPolicy",
		"recoveryAction",
		"repairActionPackage",
		"patchRepairContext",
		"patchRepairSuggestions",
		"failureTargetRepairContext",
		"readableResourceRepairContext",
		"structuredFailureSignals",
	} {
		delete(payload, key)
	}
	for key, value := range payload {
		switch typed := value.(type) {
		case map[string]any:
			stripAssessmentOnlyCorrectionDecisionAndRecoveryFields(typed)
			payload[key] = typed
		case []any:
			for idx, item := range typed {
				if nested, ok := item.(map[string]any); ok {
					stripAssessmentOnlyCorrectionDecisionAndRecoveryFields(nested)
					typed[idx] = nested
				}
			}
			payload[key] = typed
		case []map[string]any:
			for idx := range typed {
				stripAssessmentOnlyCorrectionDecisionAndRecoveryFields(typed[idx])
			}
			payload[key] = typed
		}
	}
}

func sanitizeAssessmentOnlyExecutableFields(payload map[string]any) {
	if payload == nil {
		return
	}
	for _, key := range []string{
		stepOutputKeyExecutionGraph,
		"shellCommands",
		"patchRequests",
		"mcpToolCalls",
		"memoryWrites",
		"nextAction",
		"nextStep",
		"nextKind",
		"nextInput",
		"patches",
		stepOutputKeyMemoryRefs,
	} {
		delete(payload, key)
	}
	for key, value := range payload {
		switch typed := value.(type) {
		case map[string]any:
			sanitizeAssessmentOnlyExecutableFields(typed)
			payload[key] = typed
		case []any:
			for idx, item := range typed {
				if nested, ok := item.(map[string]any); ok {
					sanitizeAssessmentOnlyExecutableFields(nested)
					typed[idx] = nested
				}
			}
			payload[key] = typed
		case []map[string]any:
			for idx := range typed {
				sanitizeAssessmentOnlyExecutableFields(typed[idx])
			}
			payload[key] = typed
		}
	}
}

func (o *RuntimeOrchestrator) recordRejectedCompletionDoDAssessment(ctx context.Context, run *runModel.Run, step *runModel.Step, output map[string]any) error {
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		assessment = syntheticRejectedDoDAssessment(run, step, output)
	}
	if satisfied, _ := assessment["satisfied"].(bool); satisfied {
		rejected := cloneStepPayload(assessment)
		rejected["satisfied"] = false
		if summary := strings.TrimSpace(stringFromMap(rejected, "summary")); summary != "" {
			rejected["sourceSummary"] = summary
		}
		rejected["summary"] = "DoD assessment rejected by runtime completion gate."
		assessment = rejected
	}
	summary, _ := assessment["summary"].(string)
	return o.recordRunResultFromRejectedDoDAssessment(ctx, run, step, assessment, summary)
}

func syntheticRejectedDoDAssessment(run *runModel.Run, step *runModel.Step, output map[string]any) map[string]any {
	reason := syntheticRejectedDoDReason(step, output)
	stepRef := ""
	if step != nil {
		stepRef = string(step.ID)
	}
	item := map[string]any{
		"item":          "runtime execution contract",
		"state":         "failed",
		"evidenceRefs":  []string{stepRef},
		"reason":        reason,
		"failureReason": reason,
	}
	assessment := map[string]any{
		"satisfied":   false,
		"summary":     reason,
		"goalOutcome": reason,
		"items":       []map[string]any{item},
		"goalCoverage": map[string]any{
			"satisfied": false,
			"items": []map[string]any{{
				"goal":         "current Goal requirements",
				"state":        "failed",
				"evidenceRefs": []string{stepRef},
				"reason":       reason,
			}},
		},
	}
	if run != nil && strings.TrimSpace(string(run.GoalID)) != "" {
		assessment["goalId"] = string(run.GoalID)
	}
	return assessment
}

func syntheticRejectedDoDReason(step *runModel.Step, output map[string]any) string {
	for _, key := range []string{"error", "reason", "summary"} {
		if reason := strings.TrimSpace(stringFromMap(output, key)); reason != "" {
			return reason
		}
	}
	if diagnostics := mapFromValue(output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		if reason := strings.TrimSpace(stringFromMap(diagnostics, "reason")); reason != "" {
			return reason
		}
		if kind := strings.TrimSpace(stringFromMap(diagnostics, "kind")); kind != "" {
			return "runtime failure: " + kind
		}
	}
	if step != nil && step.ID != "" {
		return fmt.Sprintf("runtime step %s failed before producing an acceptable DoD assessment", step.ID)
	}
	return "runtime failed before producing an acceptable DoD assessment"
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
	if summary, ok := o.rejectedRunResultSummary(ctx, run.ID); ok {
		reason := strings.TrimSpace(summary)
		if reason == "" {
			reason = "run has rejected result"
		}
		if !isTerminalRunState(run.State) {
			if err := o.failLoadedRun(ctx, run, reason); err != nil {
				return nil, err
			}
		}
		return nil, common.ErrConflict
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	if len(steps) == 0 {
		input := initialStepInput(run)
		if providerID := common.RuntimeLLMProviderIDFromContext(ctx); providerID != "" {
			input["llmProviderId"] = providerID
		}
		return []stepDispatchSpec{{
			ID:    common.StepID(fmt.Sprintf("%s/step-1", run.ID)),
			Kind:  initialStepKind(run),
			Input: input,
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

func (o *RuntimeOrchestrator) nextDispatchSpecsAfterResolvedDecision(
	ctx context.Context,
	run *runModel.Run,
	stepID common.StepID,
	resumeInput map[string]any,
) ([]stepDispatchSpec, error) {
	if boolValue(resumeInput["knownReadableDecisionRepair"]) {
		if specs, ok, err := o.knownReadableDecisionRepairSpecs(ctx, run, stepID); ok || err != nil {
			return specs, err
		}
	}
	if boolValue(resumeInput["providerRetryDecisionRetry"]) {
		if specs, ok, err := o.providerRetrySpecsAfterResolvedDecision(ctx, run, stepID); ok || err != nil {
			return specs, err
		}
	}
	return o.nextDispatchSpecsAfterStep(ctx, run, stepID)
}

func (o *RuntimeOrchestrator) providerRetrySpecsAfterResolvedDecision(
	ctx context.Context,
	run *runModel.Run,
	stepID common.StepID,
) ([]stepDispatchSpec, bool, error) {
	if o == nil || run == nil || stepID == "" {
		return nil, false, nil
	}
	step, err := o.steps.Get(ctx, stepID)
	if err != nil {
		return nil, false, err
	}
	if step == nil || step.State != common.StepStateFailed || !isLLMProviderRuntimeFailure(run, step) {
		return nil, false, nil
	}
	rootStepID := providerRetryRootStepID(step)
	attempt := o.providerRetryAttemptCount(ctx, run.ID, rootStepID) + 1
	stepID, err = o.uniqueDispatchStepID(ctx, run.ID, providerRetryStepID(run.ID, rootStepID, attempt))
	if err != nil {
		return nil, true, err
	}
	input := o.providerRetryStepInput(step, attempt, nil)
	input["decisionAuthorizedRetry"] = true
	return []stepDispatchSpec{{
		ID:    stepID,
		Kind:  step.Kind,
		Input: input,
	}}, true, nil
}

func (o *RuntimeOrchestrator) knownReadableDecisionRepairSpecs(
	ctx context.Context,
	run *runModel.Run,
	stepID common.StepID,
) ([]stepDispatchSpec, bool, error) {
	if o == nil || run == nil || stepID == "" {
		return nil, false, nil
	}
	step, err := o.steps.Get(ctx, stepID)
	if err != nil {
		return nil, false, err
	}
	if step == nil || step.State != common.StepStateCompleted {
		return nil, false, nil
	}
	decisionSpec := decisionBlockSpec(step.Output)
	if decisionSpec.Reason == "" {
		return nil, false, nil
	}
	decisionSpec.Context = o.enrichDecisionContextWithReadableWorkspaceResources(ctx, run, nil, decisionSpec.Context, decisionSpec.Question, decisionSpec.Reason)
	if !knownReadableWorkspaceResourceDecisionContext(decisionSpec.Context) {
		return nil, false, nil
	}
	repairOf := *step
	repairOf.Input = cloneStepPayload(step.Input)
	repairOf.Output = cloneStepPayload(step.Output)
	repairOf.State = common.StepStateFailed
	repairOf.Output["convertedDecision"] = convertedDecisionPayload(decisionSpec)
	repairOf.Output["continuableDecisionConvertedToRepair"] = true
	repairOf.Output["status"] = "failed"
	repairOf.Output["error"] = "decision requested a workspace resource that is already readable"
	attempt := o.runtimeRepairAttemptCountForStep(ctx, run.ID, &repairOf) + 1
	stepID, err = o.uniqueDispatchStepID(ctx, run.ID, runtimeRepairFollowUpStepID(run.ID, &repairOf, attempt))
	if err != nil {
		return nil, true, err
	}
	return []stepDispatchSpec{{
		ID:    stepID,
		Kind:  initialStepKind(run),
		Input: o.runtimeRepairFollowUpStepInput(ctx, run, &repairOf, attempt, common.ErrInvalidArgument),
	}}, true, nil
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
			stepID, err := o.uniqueDispatchStepID(ctx, run.ID, graphFollowUpStepID(run.ID, last))
			if err != nil {
				return nil, err
			}
			return []stepDispatchSpec{{
				ID:    stepID,
				Kind:  initialStepKind(run),
				Input: input,
			}}, nil
		}
		if shouldDispatchLLMCompletionFollowUp(run, last, last.Output) {
			stepID, err := o.uniqueDispatchStepID(ctx, run.ID, llmCompletionFollowUpStepID(run.ID, last))
			if err != nil {
				return nil, err
			}
			return []stepDispatchSpec{{
				ID:    stepID,
				Kind:  initialStepKind(run),
				Input: o.llmCompletionFollowUpStepInput(ctx, run, last),
			}}, nil
		}
		if shouldDispatchNoActionableContinuationFollowUp(run, last, last.Output) {
			stepID, err := o.uniqueDispatchStepID(ctx, run.ID, noActionableContinuationFollowUpStepID(run.ID, last))
			if err != nil {
				return nil, err
			}
			return []stepDispatchSpec{{
				ID:    stepID,
				Kind:  initialStepKind(run),
				Input: o.noActionableContinuationFollowUpStepInput(ctx, run, last),
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
		action := o.recoveryActionForFailedStep(ctx, run, last)
		switch action {
		case runtimeRecoveryActionProviderRetry:
			rootStepID := providerRetryRootStepID(last)
			attempt := o.providerRetryAttemptCount(ctx, run.ID, rootStepID) + 1
			stepID, err := o.uniqueDispatchStepID(ctx, run.ID, providerRetryStepID(run.ID, rootStepID, attempt))
			if err != nil {
				return nil, err
			}
			return []stepDispatchSpec{{
				ID:    stepID,
				Kind:  last.Kind,
				Input: o.providerRetryStepInput(last, attempt, nil),
			}}, nil
		case runtimeRecoveryActionRuntimeRepair:
			attempt := o.runtimeRepairAttemptCountForStep(ctx, run.ID, last) + 1
			stepID, err := o.uniqueDispatchStepID(ctx, run.ID, runtimeRepairFollowUpStepID(run.ID, last, attempt))
			if err != nil {
				return nil, err
			}
			return []stepDispatchSpec{{
				ID:    stepID,
				Kind:  initialStepKind(run),
				Input: o.runtimeRepairFollowUpStepInput(ctx, run, last, attempt, nil),
			}}, nil
		case runtimeRecoveryActionProviderRetryExhausted,
			runtimeRecoveryActionRuntimeRepairExhausted,
			runtimeRecoveryActionDuplicateFailure,
			runtimeRecoveryActionContextIntegrityBlocked:
			if err := o.blockRecoveryActionFromFailedStep(ctx, run, last, nil, action); err != nil {
				return nil, err
			}
			return nil, common.ErrBlocked
		default:
			return nil, common.ErrConflict
		}
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
	for _, key := range []string{
		"previousStepId",
		stepInputKeyRepairOfStepID,
		"failedStepId",
	} {
		previousStepID := strings.TrimSpace(stringFromMap(step.Input, key))
		if previousStepID != "" && common.StepID(previousStepID) != step.ID {
			return common.StepID(previousStepID)
		}
	}
	return ""
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

func (o *RuntimeOrchestrator) dispatchUniqueStepWithWorkspace(
	ctx context.Context,
	run *runModel.Run,
	baseStepID common.StepID,
	kind string,
	input map[string]any,
	workspace *workspaceModel.Workspace,
) error {
	stepID, err := o.uniqueDispatchStepID(ctx, run.ID, baseStepID)
	if err != nil {
		return err
	}
	return o.dispatchStepWithWorkspace(ctx, run, stepID, kind, input, workspace)
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
			if resumeInputShouldPreserveExistingStage(key, specs[idx].Input) {
				continue
			}
			specs[idx].Input[key] = value
		}
	}
}

func resumeInputShouldPreserveExistingStage(key string, input map[string]any) bool {
	switch strings.TrimSpace(key) {
	case stepInputKeySoulStage, stepInputKeyRuntimeStage, stepInputKeyRuntimeStageMode:
	default:
		return false
	}
	return strings.TrimSpace(stringFromMap(input, key)) != ""
}
