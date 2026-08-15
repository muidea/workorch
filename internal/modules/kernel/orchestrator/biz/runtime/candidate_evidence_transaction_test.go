package runtime

import (
	"context"
	"fmt"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type evidenceCommitCountingRepository struct {
	aggregate RuntimeAggregate
	loads     int
	saves     int
}

func (r *evidenceCommitCountingRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	r.loads++
	if r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return RuntimeAggregate{}, common.ErrNotFound
	}
	return r.aggregate, nil
}

func (r *evidenceCommitCountingRepository) Save(_ context.Context, aggregate RuntimeAggregate, expectedRevision uint64) error {
	r.saves++
	if r.aggregate.StateRevision != expectedRevision {
		return fmt.Errorf("%w: runtime aggregate revision changed", common.ErrConflict)
	}
	r.aggregate = aggregate
	return nil
}

func (r *evidenceCommitCountingRepository) resetCounts() {
	r.loads = 0
	r.saves = 0
}

func TestCommitEvidenceKeepsOneLoadAndOneTransitionCAS(t *testing.T) {
	seed := &durableRuntimeStateRepository{aggregate: stateChangeAggregate(t)}
	sealed := sealCandidateForTest(t, seed, stateChangeCandidate())
	repository := &evidenceCommitCountingRepository{aggregate: seed.aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	evidence := ExecutionEvidence{
		EventID: "event-transaction-count", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "patch-1", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)

	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil || committed.Disposition != CommitDispositionCommitted {
		t.Fatalf("commit evidence = %+v err=%v", committed, err)
	}
	if repository.loads != 1 || repository.saves != 1 {
		t.Fatalf("committed owner calls: loads=%d saves=%d, want 1/1", repository.loads, repository.saves)
	}

	repository.resetCounts()
	duplicate, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate {
		t.Fatalf("duplicate evidence = %+v err=%v", duplicate, err)
	}
	if repository.loads != 1 || repository.saves != 0 {
		t.Fatalf("duplicate owner calls: loads=%d saves=%d, want 1/0", repository.loads, repository.saves)
	}

	repository.resetCounts()
	stale, err := coordinator.commitEvidence(context.Background(), ExecutionEvidence{
		EventID: "event-transaction-stale", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: "authority-stale",
		OperationID: "patch-1", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	})
	if err != nil || stale.Disposition != CommitDispositionStale {
		t.Fatalf("stale evidence = %+v err=%v", stale, err)
	}
	if repository.loads != 1 || repository.saves != 1 {
		t.Fatalf("stale owner calls: loads=%d saves=%d, want 1/1", repository.loads, repository.saves)
	}
}
