package orchestrator

import (
	"context"
	"errors"
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

var errCandidateRequestFragmentCAS = errors.New("injected candidate request fragment CAS failure")

type rejectCandidateRequestFragmentCASRepository struct {
	delegate runtime.RuntimeStateRepository
	reject   bool
}

func (r *rejectCandidateRequestFragmentCASRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *rejectCandidateRequestFragmentCASRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	if r.reject && len(aggregate.CandidateRequestSessions) == 1 && len(aggregate.CandidateRequestSessions[0].Fragments) == 1 {
		r.reject = false
		return errCandidateRequestFragmentCAS
	}
	return r.delegate.Save(ctx, aggregate, expectedRevision)
}

func TestRuntimeCandidateRequestContentStoresOpaqueCompleteDocumentIdempotently(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewArtifactContinuationRepository()
	aggregate := runtimeAggregate(t, 1)
	logicalRequestID := "candidate-request-content-1"
	physicalCallID := "candidate-call-content-1"
	document := []byte(`{"operations":[]}`)
	id, hash, err := persistRuntimeCandidateRequestDocument(ctx, repo, aggregate, logicalRequestID, 1, physicalCallID, document)
	if err != nil || id == "" || hash == "" {
		t.Fatalf("persist complete document = %q/%q err=%v", id, hash, err)
	}
	stored, err := loadRuntimeCandidateRequestDocument(ctx, repo, logicalRequestID)
	if err != nil || string(stored) != string(document) {
		t.Fatalf("stored complete document = %q err=%v", stored, err)
	}
	item, err := repo.Get(ctx, id)
	if err != nil || item.ArtifactID != logicalRequestID || item.CandidateAttempt != 0 || item.OutputMode != runtimeCandidateRequestContentMode || !item.FinalReceived || item.RollingHash != hash {
		t.Fatalf("content metadata = %+v err=%v", item, err)
	}
	againID, againHash, err := persistRuntimeCandidateRequestDocument(ctx, repo, aggregate, logicalRequestID, 1, physicalCallID, append([]byte(nil), document...))
	if err != nil || againID != id || againHash != hash {
		t.Fatalf("identical complete replay = %q/%q err=%v", againID, againHash, err)
	}
	if _, _, err := persistRuntimeCandidateRequestDocument(ctx, repo, aggregate, logicalRequestID, 1, physicalCallID, []byte(`{"operations":[{}]}`)); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("different complete replay error = %v, want conflict", err)
	}
}

func TestRuntimeCandidateRequestContentChunkRejectsConflictingReplay(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewArtifactContinuationRepository()
	aggregate := runtimeAggregate(t, 1)
	fragment, err := persistRuntimeCandidateRequestContentChunk(ctx, repo, aggregate, "candidate-request-content-2", 1, "candidate-call-content-2", []byte(`{"operations":[`), true)
	if err != nil || fragment.Sequence != 1 || fragment.ContentHash == "" || !fragment.ReachedProviderLimit || fragment.PhysicalCallID != "candidate-call-content-2" {
		t.Fatalf("persist fragment = %+v err=%v", fragment, err)
	}
	if _, err := persistRuntimeCandidateRequestContentChunk(ctx, repo, aggregate, "candidate-request-content-2", 1, "candidate-call-content-2", []byte(`{"operations":[`), true); err != nil {
		t.Fatalf("identical fragment replay = %v", err)
	}
	if _, err := persistRuntimeCandidateRequestContentChunk(ctx, repo, aggregate, "candidate-request-content-2", 1, "candidate-call-content-2", []byte(`different`), true); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("conflicting fragment replay error = %v, want conflict", err)
	}
}

func TestRuntimeCandidateRequestContentBridgePersistsChunkBeforeSessionCAS(t *testing.T) {
	ctx := context.Background()
	contents := memory.NewArtifactContinuationRepository()
	initial := runtimeAggregate(t, 1)
	state := &inMemoryRuntimeStateRepository{aggregate: &initial}
	crashing := &rejectCandidateRequestFragmentCASRepository{delegate: state, reject: true}
	owner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: contents})
	if err := owner.ConfigureRuntimeRepository(crashing); err != nil {
		t.Fatal(err)
	}
	module := &Biz{bizPtr: owner}
	prepared, session, err := owner.EnsureRuntimeCandidateRequestSession(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, "stable-prefix", "manifest-v1", "")
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := owner.MarkRuntimeCandidateRequestSubmitted(ctx, prepared.Facts.WorkspaceID, prepared.Facts.RunID, session.LogicalRequestID, 1, "physical-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := module.persistAndRecordRuntimeCandidateRequestFragment(ctx, submitted, session.LogicalRequestID, 1, "physical-1", []byte(`{"operations":[`), false); !errors.Is(err, errCandidateRequestFragmentCAS) {
		t.Fatalf("fragment CAS error = %v, want injected error", err)
	}

	contentID := runtimecontract.StableRuntimeCandidateRequestContentID(session.LogicalRequestID)
	if _, content, err := contents.GetChunk(ctx, contentID, 1); err != nil || content != `{"operations":[` {
		t.Fatalf("chunk before CAS = %q err=%v", content, err)
	}
	beforeCAS, err := state.Load(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(beforeCAS.CandidateRequestSessions) != 1 || len(beforeCAS.CandidateRequestSessions[0].Fragments) != 0 || len(beforeCAS.CandidateAttempts) != 0 {
		t.Fatalf("CAS-precrash aggregate = %+v", beforeCAS)
	}

	restartedOwner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: contents})
	if err := restartedOwner.ConfigureRuntimeRepository(state); err != nil {
		t.Fatal(err)
	}
	restarted := &Biz{bizPtr: restartedOwner}
	continued, fragment, err := restarted.persistAndRecordRuntimeCandidateRequestFragment(ctx, beforeCAS, session.LogicalRequestID, 1, "physical-1", []byte(`{"operations":[`), false)
	if err != nil {
		t.Fatal(err)
	}
	if fragment.ReachedProviderLimit || len(continued.CandidateAttempts) != 0 || continued.Facts.Repair.RepairAttempts != 0 || continued.ActiveDispatch != nil {
		t.Fatalf("fragment changed candidate/repair/dispatch state: fragment=%+v aggregate=%+v", fragment, continued)
	}
	stored := continued.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionContinuationPending || stored.NextSequence != 2 || len(stored.Fragments) != 1 || stored.Fragments[0].PhysicalCallID != "physical-1" {
		t.Fatalf("continued session = %+v", stored)
	}
	if _, _, err := restarted.persistAndRecordRuntimeCandidateRequestFragment(ctx, continued, session.LogicalRequestID, 1, "physical-1", []byte(`different`), false); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("conflicting replay error = %v, want conflict", err)
	}
}

func TestRuntimeCandidateRequestContentBridgeCompletesDocumentWithoutCandidateAttempt(t *testing.T) {
	ctx := context.Background()
	contents := memory.NewArtifactContinuationRepository()
	initial := runtimeAggregate(t, 1)
	state := &inMemoryRuntimeStateRepository{aggregate: &initial}
	owner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: contents})
	if err := owner.ConfigureRuntimeRepository(state); err != nil {
		t.Fatal(err)
	}
	module := &Biz{bizPtr: owner}
	prepared, session, err := owner.EnsureRuntimeCandidateRequestSession(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, "stable-prefix", "manifest-v1", "")
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := owner.MarkRuntimeCandidateRequestSubmitted(ctx, prepared.Facts.WorkspaceID, prepared.Facts.RunID, session.LogicalRequestID, 1, "physical-1")
	if err != nil {
		t.Fatal(err)
	}
	document := []byte(`{"operations":[]}`)
	completed, id, hash, err := module.persistAndCompleteRuntimeCandidateRequestDocument(ctx, submitted, session.LogicalRequestID, 1, "physical-1", document)
	if err != nil || id != runtimecontract.StableRuntimeCandidateRequestContentID(session.LogicalRequestID) || hash == "" {
		t.Fatalf("complete request document = aggregate=%+v id=%q hash=%q err=%v", completed, id, hash, err)
	}
	stored, err := loadCompletedRuntimeCandidateRequestDocument(ctx, contents, completed, session.LogicalRequestID)
	if err != nil || string(stored) != string(document) {
		t.Fatalf("complete request document content = %q err=%v", stored, err)
	}
	if len(completed.CandidateAttempts) != 0 || completed.Facts.Repair.RepairAttempts != 0 || completed.ActiveDispatch != nil || len(completed.CandidateRequestSessions) != 1 || completed.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
		t.Fatalf("complete document allocated candidate state: %+v", completed)
	}
}

func TestRuntimeCandidateRequestContentBridgeEOFDispositionDoesNotChangeSessionTransition(t *testing.T) {
	ctx := context.Background()
	for _, reachedProviderLimit := range []bool{false, true} {
		t.Run(map[bool]string{false: "mid_token_eof", true: "provider_limit_eof"}[reachedProviderLimit], func(t *testing.T) {
			contents := memory.NewArtifactContinuationRepository()
			initial := runtimeAggregate(t, 1)
			state := &inMemoryRuntimeStateRepository{aggregate: &initial}
			owner := NewRuntimeOrchestratorWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: contents})
			if err := owner.ConfigureRuntimeRepository(state); err != nil {
				t.Fatal(err)
			}
			module := &Biz{bizPtr: owner}
			prepared, session, err := owner.EnsureRuntimeCandidateRequestSession(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, "stable-prefix", "manifest-v1", "")
			if err != nil {
				t.Fatal(err)
			}
			submitted, err := owner.MarkRuntimeCandidateRequestSubmitted(ctx, prepared.Facts.WorkspaceID, prepared.Facts.RunID, session.LogicalRequestID, 1, "physical-1")
			if err != nil {
				t.Fatal(err)
			}
			continued, fragment, err := module.persistAndRecordRuntimeCandidateRequestFragment(ctx, submitted, session.LogicalRequestID, 1, "physical-1", []byte(`{"operations":[`), reachedProviderLimit)
			if err != nil {
				t.Fatal(err)
			}
			storedSession := continued.CandidateRequestSessions[0]
			if fragment.ReachedProviderLimit != reachedProviderLimit || storedSession.State != runtimecontract.RuntimeCandidateRequestSessionContinuationPending || storedSession.NextSequence != 2 || storedSession.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeDocumentIncomplete || len(continued.CandidateAttempts) != 0 || continued.ActiveDispatch != nil {
				t.Fatalf("EOF changed session/candidate transition: fragment=%+v aggregate=%+v", fragment, continued)
			}
			if _, err := loadCompletedRuntimeCandidateRequestDocument(ctx, contents, continued, session.LogicalRequestID); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("read incomplete document error = %v, want conflict", err)
			}
			continuation, physicalCallID, err := prepareRuntimeCandidateRequestContinuation(ctx, contents, continued, session.LogicalRequestID, 6)
			if err != nil || continuation.ID != runtimecontract.StableRuntimeCandidateRequestContentID(session.LogicalRequestID) || continuation.Sequence != 2 || continuation.PreviousChunkTail != "ons\":[" || physicalCallID != runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 2) {
				t.Fatalf("next continuation = %+v physical=%q err=%v", continuation, physicalCallID, err)
			}
		})
	}
}
