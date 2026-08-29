package runtime

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *candidateDispatchComponent) commitEvidence(ctx context.Context, evidence ExecutionEvidence) (EvidenceCommitResult, error) {
	if c == nil || c.repository == nil {
		return EvidenceCommitResult{}, common.ErrInvalidArgument
	}
	if err := evidence.Validate(); err != nil {
		return EvidenceCommitResult{}, err
	}
	aggregate, err := c.repository.Load(ctx, evidence.WorkspaceID, evidence.RunID)
	if err != nil {
		return EvidenceCommitResult{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return EvidenceCommitResult{}, err
	}
	// Every ordinary owner result is promoted only from its persisted receipt.
	// Canonicalize before the duplicate fence so live response and fresh-load
	// recovery converge on one durable evidence identity.
	evidence = canonicalPersistedOwnerReceiptEvidenceIdentity(aggregate, evidence)
	// Apply the owner-local receipt transition before validating the complete
	// next aggregate. RuntimeTransitionCoordinator validates the post-transition
	// aggregate before the only CAS write, so partial state is impossible.
	if containsString(aggregate.ProcessedEventIDs, evidence.EventID) {
		return EvidenceCommitResult{Disposition: CommitDispositionDuplicate, Aggregate: aggregate}, nil
	}
	expectedRevision := aggregate.StateRevision
	currentSnapshotID := aggregate.Decision.AuthoritySnapshot.ID
	if evidence.AuthoritySnapshotID != currentSnapshotID {
		recordStaleExecutionEvidence(&aggregate, evidence, currentSnapshotID)
		aggregate.StateRevision++
		return c.commitEvidenceTransition(ctx, aggregate, expectedRevision, CommitDispositionStale, false)
	}

	admission, err := admitExecutionEvidenceIdentity(aggregate, evidence)
	if err != nil {
		return EvidenceCommitResult{}, err
	}
	fact, finding, err := deriveExecutionEvidenceFact(aggregate, admission)
	if err != nil {
		return EvidenceCommitResult{}, err
	}
	// Accepted coverage-proof projection consumes the evidence-committed fence
	// while it appends lineage, and its fact needs no later enrichment. Rejected
	// evidence instead receives its exact owner failure/signature during apply,
	// so that branch must bind the final fact below.
	if admission.Evidence.Status == ExecutionEvidenceStatusAccepted {
		aggregate.ActiveDispatch.OperationExecutions = markOperationExecutionEvidenceCommitted(
			aggregate.ActiveDispatch.OperationExecutions,
			admission.Evidence.OperationID,
			admission.Evidence.OwnerReceiptRef,
			admission.Evidence.OwnerReceiptHash,
			fact,
		)
	}
	if err := applyExecutionEvidenceByStatus(&aggregate, admission, fact, finding); err != nil {
		return EvidenceCommitResult{}, err
	}
	// Receipt identity, the final persisted EvidenceFact and the execution
	// ledger move together in this aggregate transaction. Rejected evidence is
	// enriched with the exact owner failure/signature by the SM-2 projection,
	// so bind the ledger only after that final fact has been appended.
	if admission.Evidence.Status == ExecutionEvidenceStatusRejected {
		persistedFact, found := persistedExecutionEvidenceFact(aggregate.Facts, admission.Evidence.Status, fact.ID)
		if !found {
			return EvidenceCommitResult{}, common.ErrInvalidArgument
		}
		aggregate.ActiveDispatch.OperationExecutions = markOperationExecutionEvidenceCommitted(
			aggregate.ActiveDispatch.OperationExecutions,
			admission.Evidence.OperationID,
			admission.Evidence.OwnerReceiptRef,
			admission.Evidence.OwnerReceiptHash,
			persistedFact,
		)
	}
	dispatchCompleted, err := finalizeExecutionEvidence(&aggregate, admission, fact)
	if err != nil {
		return EvidenceCommitResult{}, err
	}
	aggregate.StateRevision++
	return c.commitEvidenceTransition(ctx, aggregate, expectedRevision, CommitDispositionCommitted, dispatchCompleted)
}

func persistedExecutionEvidenceFact(facts runtimecontract.RuntimeFacts, status runtimecontract.RuntimeOperationExecutionStatus, id common.EvidenceRecordID) (runtimecontract.EvidenceFact, bool) {
	var values []runtimecontract.EvidenceFact
	switch status {
	case ExecutionEvidenceStatusAccepted:
		values = facts.Evidence.Accepted
	case ExecutionEvidenceStatusRejected:
		values = facts.Evidence.Rejected
	default:
		return runtimecontract.EvidenceFact{}, false
	}
	for _, fact := range values {
		if fact.ID == id {
			return fact, true
		}
	}
	return runtimecontract.EvidenceFact{}, false
}
