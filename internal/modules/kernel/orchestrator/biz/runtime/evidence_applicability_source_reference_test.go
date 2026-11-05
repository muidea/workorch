package runtime

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"reflect"
)

// Frozen complete request/read composition before sharing its original source.
func referenceBeforeSingleSourceCompileRuntimeEvidenceApplicabilityRequest(aggregate RuntimeAggregate, evidenceID common.EvidenceRecordID) (runtimecontract.RuntimeEvidenceApplicabilityRequest, error) {
	var fact runtimecontract.EvidenceFact
	matches := 0
	for _, value := range aggregate.Facts.Evidence.Accepted {
		if value.ID == evidenceID {
			fact, matches = value, matches+1
		}
	}
	if matches != 1 || fact.ApplicabilityDependencies == nil {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, fmt.Errorf("%w: applicability requires unique versioned accepted source", common.ErrConflict)
	}
	var receiptHash string
	for _, receipt := range aggregate.OperationReceipts {
		if fact.SourceRef == "runtime-receipt/"+receipt.ExecutionKey && receipt.OperationID == fact.OperationID {
			if receiptHash != "" {
				return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, common.ErrConflict
			}
			receiptHash = runtimecontract.StableRuntimeOperationReceiptHash(receipt)
		}
	}
	source, err := compileRuntimeOperationReadSource(aggregate, fact.DispatchID, fact.OperationID, receiptHash)
	if err != nil {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, err
	}
	if !reflect.DeepEqual(source.ApplicabilitySource, fact.ApplicabilityDependencies) || !reflect.DeepEqual(&source.OriginalDependencies, fact.OwnerRevisionSet) {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, fmt.Errorf("%w: applicability source payload mismatch", common.ErrConflict)
	}
	request := (runtimecontract.RuntimeEvidenceApplicabilityRequest{
		ProtocolRef:           runtimecontract.RuntimeEvidenceApplicabilityObservationV1,
		IssuedAtStateRevision: aggregate.StateRevision, EvidenceRevision: aggregate.Facts.EvidenceRevision,
		SourceEvidenceID: fact.ID, SourceEvidenceHash: runtimecontract.StableRuntimeEvidenceFactHash(fact), SourceReceiptHash: receiptHash,
		DispatchID: fact.DispatchID, OperationID: fact.OperationID,
		ReconciliationRequestHash: aggregate.Reconciliation.RequestHash,
		PurposeRef:                fact.ApplicabilityDependencies.PurposeRef,
	}).WithID()
	return request, request.ValidateRaw()
}

// Only complete cold/owner-local gates may enter this pure continuation.
func referenceBeforeSingleSourceNextRuntimeEvidenceApplicabilityRequest(aggregate RuntimeAggregate) (runtimecontract.RuntimeEvidenceApplicabilityRequest, bool, error) {
	if aggregate.ActiveDispatch != nil || aggregate.Termination != nil || aggregate.TerminalMaterialSeal != nil || isTerminalConvergenceState(aggregate.Decision.State) {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, false, nil
	}
	if runtimeApplicabilityHasUnsettledProvider(aggregate) {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, false, nil
	}
	for _, fact := range aggregate.Facts.Evidence.Accepted {
		if fact.ApplicabilityDependencies == nil || runtimecontract.RuntimeEvidenceIsHistoricalEffect(fact) || runtimecontract.RuntimeEvidenceHasStaleApplicability(fact, aggregate.Facts.EvidenceApplicabilityObservations, aggregate.Facts.EvidenceRevision) {
			continue
		}
		if !runtimeApplicabilityEvidenceAttributable(aggregate.Facts, fact, aggregate.CoverageProofLineage) {
			continue
		}
		observed := false
		for _, observation := range aggregate.Facts.EvidenceApplicabilityObservations {
			if observation.Request.SourceEvidenceID == fact.ID && observation.Request.SourceEvidenceHash == runtimecontract.StableRuntimeEvidenceFactHash(fact) && observation.Request.EvidenceRevision == aggregate.Facts.EvidenceRevision && observation.Request.ReconciliationRequestHash == aggregate.Reconciliation.RequestHash {
				observed = true
			}
		}
		if observed {
			continue
		}
		request, err := referenceBeforeSingleSourceCompileRuntimeEvidenceApplicabilityRequest(aggregate, fact.ID)
		return request, err == nil, err
	}
	return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, false, nil
}

// The owner result boundary derives detached query data from its admitted image.
// The private observation event owns the changed-input source/fence admission.

func referenceBeforeSingleSourceCompileRuntimeEvidenceApplicabilityRead(aggregate RuntimeAggregate) (runtimecontract.RuntimeEvidenceApplicabilityRequest, RuntimeOperationReadSource, bool, error) {
	request, required, err := referenceBeforeSingleSourceNextRuntimeEvidenceApplicabilityRequest(aggregate)
	if err != nil || !required {
		return request, RuntimeOperationReadSource{}, false, err
	}
	source, err := compileRuntimeOperationReadSource(aggregate, request.DispatchID, request.OperationID, request.SourceReceiptHash)
	if err != nil {
		return runtimecontract.RuntimeEvidenceApplicabilityRequest{}, RuntimeOperationReadSource{}, false, err
	}
	return request, source, true, nil
}
