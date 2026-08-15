package runtime

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
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
	// Receipt identity is admitted before the execution ledger and SM-2 facts
	// move together in the same aggregate transaction.
	aggregate.ActiveDispatch.OperationExecutions = markOperationExecutionEvidenceCommitted(
		aggregate.ActiveDispatch.OperationExecutions,
		admission.Evidence.OperationID,
		admission.Evidence.OwnerReceiptRef,
		admission.Evidence.OwnerReceiptHash,
	)
	if err := applyExecutionEvidenceByStatus(&aggregate, admission, fact, finding); err != nil {
		return EvidenceCommitResult{}, err
	}
	dispatchCompleted, err := finalizeExecutionEvidence(&aggregate, admission)
	if err != nil {
		return EvidenceCommitResult{}, err
	}
	aggregate.StateRevision++
	return c.commitEvidenceTransition(ctx, aggregate, expectedRevision, CommitDispositionCommitted, dispatchCompleted)
}
