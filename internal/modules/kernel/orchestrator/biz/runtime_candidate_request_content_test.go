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

func TestRuntimeCandidateRequestContentRecoversAssembledBeforeCarrierTransition(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewArtifactContinuationRepository()
	aggregate := runtimeAggregate(t, 1)
	logicalRequestID := "candidate-request-content-crash-window"
	document := []byte(`{"operations":[]}`)
	carrier, err := loadOrCreateRuntimeCandidateRequestContent(ctx, repo, aggregate, logicalRequestID)
	if err != nil {
		t.Fatal(err)
	}
	hash := runtimeCandidateOutputHash(string(document))
	if err := repo.SaveAssembled(ctx, &runtimecontract.ArtifactContinuationAssembledMetadata{
		ContinuationID: carrier.ID,
		TargetRef:      carrier.TargetRef,
		Bytes:          int64(len(document)),
		RollingHash:    hash,
		CreatedAt:      "2026-08-22T00:00:00Z",
	}, string(document)); err != nil {
		t.Fatal(err)
	}

	id, gotHash, err := persistRuntimeCandidateRequestDocument(ctx, repo, aggregate, logicalRequestID, 1, "physical-crash-window", document)
	if err != nil || id != carrier.ID || gotHash != hash {
		t.Fatalf("recover assembled document = %q/%q err=%v", id, gotHash, err)
	}
	stored, err := repo.Get(ctx, carrier.ID)
	if err != nil || stored.State != common.ArtifactContinuationStateApplied || !stored.FinalReceived || stored.RollingHash != hash {
		t.Fatalf("recovered carrier = %+v err=%v", stored, err)
	}
}

func TestCandidateContentInternalEventRequiresExactlyOneFact(t *testing.T) {
	logicalRequestID := "candidate-request-content-envelope"
	_, err := normalizeCandidateContentInternalEvent(candidateContentInternalEvent{
		Kind:           candidateContentInternalEventCreated,
		ContinuationID: runtimecontract.StableRuntimeCandidateRequestContentID(logicalRequestID),
		WorkspaceID:    "workspace-1", RunID: "run-1", LogicalRequestID: logicalRequestID,
		AuthorityTransaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
		Created:              &candidateContentCreatedFact{CreatedAt: "2026-08-22T00:00:00Z", UpdatedAt: "2026-08-22T00:00:00Z"},
		Applied:              &candidateContentAppliedFact{BytesWritten: 1, RollingHash: "hash", UpdatedAt: "2026-08-22T00:00:01Z"},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("multi-payload content event err = %v, want invalid argument", err)
	}
}

func TestRuntimeCandidateRequestContentBridgeCompletesDocumentWithoutCandidateAttempt(t *testing.T) {
	ctx := context.Background()
	contents := memory.NewArtifactContinuationRepository()
	state := &inMemoryRuntimeStateRepository{}
	owner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{ArtifactContinuations: contents})
	if err := owner.ConfigureRuntimeRepository(state); err != nil {
		t.Fatal(err)
	}
	initialized, err := owner.InitializeRuntime(ctx, runtimeCandidateSealExactFileEditFacts(t, "target.txt"), runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initial := runtimeTestPersistSealedSemanticAuthority(t, ctx, state, initialized)
	var snapshotPrerequisite runtimecontract.RuntimePrerequisite
	for _, pending := range initial.Facts.Prerequisites.Pending {
		if pending.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && pending.OwnerModuleID == runtimecontract.RuntimeModuleFile && pending.SubjectRef == "target.txt" {
			snapshotPrerequisite = pending
			break
		}
	}
	if snapshotPrerequisite.ID == "" {
		t.Fatalf("candidate request fixture has no File target-snapshot prerequisite: %+v", initial.Facts.Prerequisites.Pending)
	}
	snapshot := runtimecontract.TargetSnapshot{TargetRef: "target.txt", ContentHash: "candidate-request-target-v1", ContentState: runtimecontract.SnapshotContentStateComplete}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: snapshotPrerequisite.ID, Kind: snapshotPrerequisite.Kind, OwnerModuleID: snapshotPrerequisite.OwnerModuleID,
		ContentHash: snapshot.ContentHash,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: snapshot.TargetRef, Revision: snapshot.ContentHash,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	initial, err = owner.CommitRuntimePrerequisite(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, runtime.PrerequisiteResult{
		PrerequisiteID: snapshotPrerequisite.ID, TargetSnapshot: &snapshot, Observation: &observation,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		initial.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(initial.Facts.Resources),
	)
	if err != nil {
		if initial.Decision.Terminal != nil {
			t.Fatalf("build candidate request manifest: %v (terminal=%+v)", err, *initial.Decision.Terminal)
		}
		t.Fatal(err)
	}
	module := newTestBiz(owner, nil, nil)
	prepared, session, err := owner.EnsureRuntimeCandidateRequestSession(ctx, initial.Facts.WorkspaceID, initial.Facts.RunID, "stable-prefix", manifest.Hash, "")
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
	carrier, err := contents.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	carrier.WorkspaceID = "other-workspace"
	if err := contents.Update(ctx, carrier); err != nil {
		t.Fatal(err)
	}
	if _, err := loadCompletedRuntimeCandidateRequestDocument(ctx, contents, completed, session.LogicalRequestID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("cross-authority complete carrier error = %v, want conflict", err)
	}
	carrier.WorkspaceID = completed.Facts.WorkspaceID
	if err := contents.Update(ctx, carrier); err != nil {
		t.Fatal(err)
	}

	// A document_complete aggregate is an owner-contract assertion, not
	// permission to reinterpret corrupt carrier bytes as an SM-3 candidate.
	// Simulate assembled-storage corruption while leaving the owner-private
	// carrier event intact: admission must detect the two-record mismatch and
	// terminal without allocating candidate attempt 0.
	metadata, _, err := contents.GetAssembled(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := `{} {}`
	metadata.Bytes = int64(len(corrupt))
	metadata.RollingHash = runtimeCandidateOutputHash(corrupt)
	if err := contents.SaveAssembled(ctx, metadata, corrupt); err != nil {
		t.Fatal(err)
	}
	failed, _, accepted, err := module.admitRuntimeCompletedCandidateDocument(ctx, completed, manifest, session.LogicalRequestID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if accepted || len(failed.CandidateAttempts) != 0 || failed.Decision.State != runtimecontract.ConvergenceStateFailed ||
		failed.Facts.Failure == nil || failed.Facts.Failure.Code != "provider_document_content_invalid" ||
		failed.Facts.Failure.Class != runtimecontract.FailureClassRuntimeContractMisalignment {
		t.Fatalf("corrupt complete carrier crossed into candidate admission: accepted=%t failure=%+v attempts=%+v", accepted, failed.Facts.Failure, failed.CandidateAttempts)
	}
}
