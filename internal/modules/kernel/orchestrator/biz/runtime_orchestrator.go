package orchestrator

import (
	"context"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeOrchestrator struct {
	evidence            RuntimeEvidenceRepositories
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

// ApplyRuntimeV2InternalEvent is the sole mutation entry point used by the
// Orchestrator Biz. The event is owner-local (never an EventHub envelope): it
// is created only after an external typed result has been validated against
// its workspace/run/causation/sealed authority scope.
func (o *RuntimeOrchestrator) ApplyRuntimeV2InternalEvent(ctx context.Context, event runtimev2.RuntimeInternalEvent) (runtimev2.RuntimeInternalEventResult, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.RuntimeInternalEventResult{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.ApplyInternalEvent(ctx, event)
}

func (o *RuntimeOrchestrator) LoadRuntimeV2(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtimev2.RuntimeAggregate, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.Load(ctx, workspaceID, runID)
}

func (o *RuntimeOrchestrator) QueryRuntimeV2OperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID, operationID, executionKey string, fence uint64, grant *runtimemodel.RuntimeOperationGovernanceGrant) (runtimemodel.RuntimeOperationReceipt, bool, error) {
	if o == nil || o.runtimeV2Controller == nil {
		return runtimemodel.RuntimeOperationReceipt{}, false, common.ErrInvalidArgument
	}
	return o.runtimeV2Controller.QueryOperationReceipt(ctx, workspaceID, runID, dispatchID, operationID, executionKey, fence, grant)
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
