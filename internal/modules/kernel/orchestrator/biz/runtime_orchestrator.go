package orchestrator

import (
	"context"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeOrchestrator struct {
	evidence          RuntimeEvidenceRepositories
	runtimeController *runtime.RuntimeController
}

func (o *RuntimeOrchestrator) ConfigureRuntimeRepository(repository runtime.RuntimeStateRepository) error {
	if o == nil || repository == nil {
		return common.ErrInvalidArgument
	}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		return err
	}
	o.runtimeController = controller
	return nil
}

func (o *RuntimeOrchestrator) RuntimeRepositoryConfigured() bool {
	return o != nil && o.runtimeController != nil
}

// ApplyRuntimeInternalEvent is the sole mutation entry point used by the
// Orchestrator Biz. The event is owner-local (never an EventHub envelope): it
// is created only after an external typed result has been validated against
// its workspace/run/causation/sealed authority scope.
func (o *RuntimeOrchestrator) ApplyRuntimeInternalEvent(ctx context.Context, event runtime.RuntimeInternalEvent) (runtime.RuntimeInternalEventResult, error) {
	if o == nil || o.runtimeController == nil {
		return runtime.RuntimeInternalEventResult{}, common.ErrInvalidArgument
	}
	return o.runtimeController.ApplyInternalEvent(ctx, event)
}

func (o *RuntimeOrchestrator) LoadRuntime(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	if o == nil || o.runtimeController == nil {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return o.runtimeController.Load(ctx, workspaceID, runID)
}

func (o *RuntimeOrchestrator) QueryRuntimeOperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID, operationID, executionKey string, fence uint64, grant *runtimemodel.RuntimeOperationGovernanceGrant) (runtimemodel.RuntimeOperationReceipt, bool, error) {
	if o == nil || o.runtimeController == nil {
		return runtimemodel.RuntimeOperationReceipt{}, false, common.ErrInvalidArgument
	}
	return o.runtimeController.QueryOperationReceipt(ctx, workspaceID, runID, dispatchID, operationID, executionKey, fence, grant)
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

func NewRuntimeOrchestrator() *RuntimeOrchestrator {
	return NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{})
}

func NewRuntimeOrchestratorWithEvidence(evidence RuntimeEvidenceRepositories) *RuntimeOrchestrator {
	return &RuntimeOrchestrator{evidence: evidence}
}
