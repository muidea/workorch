package orchestrator

import (
	"context"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeOrchestrator struct {
	runs                repository.RunRepository
	workspaces          repository.WorkspaceRepository
	tickets             repository.DecisionTicketRepository
	resolutions         repository.DecisionResolutionRepository
	approvals           repository.ApprovalRepository
	evidence            RuntimeEvidenceRepositories
	runState            *runStateSubsystem
	runtimeV2Controller *runtimev2.RuntimeController
}

func (o *RuntimeOrchestrator) ConfigureRuntimeV2Repository(repository runtimev2.RuntimeStateRepository) error {
	if o == nil || repository == nil {
		return common.ErrInvalidArgument
	}
	controller, err := runtimev2.NewRuntimeController(repository)
	if err != nil {
		return err
	}
	o.runtimeV2Controller = controller
	return nil
}

func (o *RuntimeOrchestrator) RuntimeV2RepositoryConfigured() bool {
	return o != nil && o.runtimeV2Controller != nil
}

func (o *RuntimeOrchestrator) InitializeRuntimeV2(ctx context.Context, facts runtimemodel.RuntimeFacts, definition runtimemodel.RuntimeDefinitionContext, requirements []runtimev2.RuntimeRequirement) (runtimev2.RuntimeAggregate, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.Initialize(ctx, facts, definition, requirements)
}

func (o *RuntimeOrchestrator) LoadRuntimeV2(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtimev2.RuntimeAggregate, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.Load(ctx, workspaceID, runID)
}

func (o *RuntimeOrchestrator) CommitRuntimeV2Prerequisite(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, result runtimev2.PrerequisiteResult) (runtimev2.RuntimeAggregate, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.CommitPrerequisite(ctx, workspaceID, runID, result)
}

func (o *RuntimeOrchestrator) RecordRuntimeV2FeasibilityFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimemodel.RuntimeFeasibilityFact) (runtimev2.RuntimeAggregate, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.RecordFeasibilityFact(ctx, workspaceID, runID, fact)
}

func (o *RuntimeOrchestrator) RecordRuntimeV2FailureFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimemodel.FailureFact) (runtimev2.RuntimeAggregate, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.RecordFailureFact(ctx, workspaceID, runID, fact)
}

func (o *RuntimeOrchestrator) ConsumeRuntimeV2RecoveryBudget(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimemodel.RuntimeRecoveryBudgetKind) (runtimev2.RuntimeAggregate, bool, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.ConsumeRecoveryBudget(ctx, workspaceID, runID, kind)
}

func (o *RuntimeOrchestrator) BeginRuntimeV2CandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stablePrefixHash string) (runtimev2.RuntimeAggregate, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.BeginCandidateAttempt(ctx, workspaceID, runID, attempt, stablePrefixHash)
}

func (o *RuntimeOrchestrator) AdmitRuntimeV2CandidateProposal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtimev2.CandidateAdmissionCommitResult, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.CandidateAdmissionCommitResult{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.AdmitCandidateProposal(ctx, workspaceID, runID, attempt, candidate)
}

func (o *RuntimeOrchestrator) RecordRuntimeV2CandidateFeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, record runtimemodel.RuntimeCandidateFeasibilityRecord) (runtimev2.RuntimeAggregate, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.RecordCandidateFeasibility(ctx, workspaceID, runID, attempt, record)
}

func (o *RuntimeOrchestrator) InvalidateRuntimeV2CandidateAttemptForPrerequisiteStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stale runtimemodel.RuntimePrerequisiteStale) (runtimev2.RuntimeAggregate, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.InvalidateCandidateAttemptForPrerequisiteStale(ctx, workspaceID, runID, attempt, stale)
}

func (o *RuntimeOrchestrator) QueryRuntimeV2OperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID, operationID, executionKey string, fence uint64, grant *runtimemodel.RuntimeOperationGovernanceGrant) (runtimemodel.RuntimeOperationReceipt, bool, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimemodel.RuntimeOperationReceipt{}, false, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.QueryOperationReceipt(ctx, workspaceID, runID, dispatchID, operationID, executionKey, fence, grant)
}

func (o *RuntimeOrchestrator) CommitRuntimeV2OperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, fence uint64, receipt runtimemodel.RuntimeOperationReceipt) (runtimev2.RuntimeAggregate, runtimemodel.RuntimeOperationReceipt, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.RuntimeAggregate{}, runtimemodel.RuntimeOperationReceipt{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.CommitOperationReceipt(ctx, workspaceID, runID, dispatchID, fence, receipt)
}

func (o *RuntimeOrchestrator) AdmitAndSealRuntimeV2CandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, candidate runtimemodel.RuntimeCandidate) (runtimev2.CandidateAdmissionCommitResult, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.CandidateAdmissionCommitResult{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.AdmitAndSealCandidateAttempt(ctx, workspaceID, runID, attempt, candidate)
}

func (o *RuntimeOrchestrator) FailRuntimeV2CandidateAttempt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, failure runtimemodel.RuntimeCandidateFailure) (runtimev2.RuntimeAggregate, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.FailCandidateAttempt(ctx, workspaceID, runID, attempt, failure)
}

func (o *RuntimeOrchestrator) CommitRuntimeV2Evidence(ctx context.Context, evidence runtimev2.ExecutionEvidence) (runtimev2.EvidenceCommitResult, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.EvidenceCommitResult{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.CommitEvidence(ctx, evidence)
}

func (o *RuntimeOrchestrator) CommitRuntimeV2Assessment(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtimev2.RuntimeAggregate, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.CommitAssessment(ctx, workspaceID, runID)
}

func (o *RuntimeOrchestrator) CommitRuntimeV2GovernanceResolution(ctx context.Context, resolution runtimemodel.RuntimeGovernanceResolution) (runtimev2.EvidenceCommitResult, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.EvidenceCommitResult{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.CommitGovernanceResolution(ctx, resolution)
}

func (o *RuntimeOrchestrator) BindRuntimeV2GovernanceRequest(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, request runtimemodel.RuntimeGovernanceRequestRecord) (runtimev2.RuntimeAggregate, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.BindGovernanceRequest(ctx, workspaceID, runID, request)
}

func (o *RuntimeOrchestrator) ExecuteRuntimeV2Dispatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, executor runtimev2.RuntimeOperationExecutor) (runtimev2.RuntimeAggregate, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.ExecuteActiveDispatch(ctx, workspaceID, runID, executor)
}

type RuntimeEvidenceRepositories struct {
	Results               repository.ResultRepository
	Artifacts             repository.ArtifactRepository
	ToolCalls             repository.ToolCallRepository
	Patches               repository.PatchRepository
	ValidationResults     repository.ValidationResultRepository
	EvidenceRecords       repository.EvidenceRecordRepository
	CoverageWorkItems     repository.CoverageWorkItemRepository
	ArtifactContinuations repository.ArtifactContinuationRepository
	MemoryItems           repository.MemoryItemRepository
	MemoryRecallTraces    repository.MemoryRecallTraceRepository
	AutoGovernanceTraces  repository.AutoGovernanceTraceRepository
}

func (o *RuntimeOrchestrator) EvidenceRepositories() RuntimeEvidenceRepositories {
	if o == nil {
		return RuntimeEvidenceRepositories{}
	}
	return o.evidence
}

func NewRuntimeOrchestrator(
	runRepo repository.RunRepository,
	workspaceRepo repository.WorkspaceRepository,
	ticketRepo repository.DecisionTicketRepository,
	approvalRepo repository.ApprovalRepository,
) *RuntimeOrchestrator {
	return NewRuntimeOrchestratorWithResolutionsAndEvidence(runRepo, workspaceRepo, ticketRepo, nil, approvalRepo, RuntimeEvidenceRepositories{})
}

func NewRuntimeOrchestratorWithResolutionsAndEvidence(
	runRepo repository.RunRepository,
	workspaceRepo repository.WorkspaceRepository,
	ticketRepo repository.DecisionTicketRepository,
	resolutionRepo repository.DecisionResolutionRepository,
	approvalRepo repository.ApprovalRepository,
	evidence RuntimeEvidenceRepositories,
) *RuntimeOrchestrator {
	return &RuntimeOrchestrator{
		runs:        runRepo,
		workspaces:  workspaceRepo,
		tickets:     ticketRepo,
		resolutions: resolutionRepo,
		approvals:   approvalRepo,
		evidence:    evidence,
		runState:    newRunStateSubsystem(runRepo, workspaceRepo),
	}
}

func (o *RuntimeOrchestrator) runStateManager() *runStateSubsystem {
	if o == nil {
		return newRunStateSubsystem(nil, nil)
	}
	if o.runState == nil {
		o.runState = newRunStateSubsystem(o.runs, o.workspaces)
	} else {
		o.runState.runs = o.runs
		o.runState.workspaces = o.workspaces
	}
	return o.runState
}
