package orchestrator

import (
	"context"
	"fmt"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// SM-4 only coordinates concrete owner reads. It cannot judge the response,
// change receipt provenance, grant credit, or choose a reopen transition.
func (m *Biz) driveRuntimeEvidenceApplicability(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, bool, error) {
	request, required, err := runtime.NextRuntimeEvidenceApplicabilityRequest(aggregate)
	if err != nil || !required {
		return aggregate, false, err
	}
	source, err := runtime.CompileRuntimeOperationReadSource(aggregate, request.DispatchID, request.OperationID, request.SourceReceiptHash)
	if err != nil {
		return runtime.RuntimeAggregate{}, false, err
	}
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(source.Operation.Kind)
	if !found {
		return runtime.RuntimeAggregate{}, false, fmt.Errorf("versioned evidence applicability has no concrete owner query for %q", source.Operation.Kind)
	}
	var current *runtimecontract.RuntimeOwnerRevisionSet
	var observed bool
	switch descriptor.OwnerModuleID {
	case runtimecontract.RuntimeModuleFile:
		current, observed, err = m.queryRuntimeFileMutationApplicabilityFromAggregate(ctx, aggregate, request.DispatchID, request.OperationID, request.SourceReceiptHash, request.RequestID)
	case runtimecontract.RuntimeModuleShell:
		current, observed, err = m.queryRuntimeShellApplicabilityFromAggregate(ctx, aggregate, request.DispatchID, request.OperationID, request.SourceReceiptHash, request.RequestID)
	default:
		return runtime.RuntimeAggregate{}, false, fmt.Errorf("versioned evidence applicability has no concrete owner query for %q", source.Operation.Kind)
	}
	if err != nil {
		return runtime.RuntimeAggregate{}, false, err // failed read is never a missing observation
	}
	observation := runtimecontract.RuntimeEvidenceApplicabilityObservation{Request: request, RecordedAtStateRevision: request.IssuedAtStateRevision + 1, Missing: !observed, CurrentDependencies: current}
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventEvidenceApplicabilityObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{EvidenceApplicability: &observation}})
	return updated.Aggregate, err == nil, err
}
