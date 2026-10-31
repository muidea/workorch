package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOperationsRendererSelectionSurvivesReloadAndCorrection(t *testing.T) {
	ctx := context.Background()
	state := &inMemoryRuntimeStateRepository{}
	controller, err := runtime.NewRuntimeController(state)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := runtimeTestControllerInitialize(controller, ctx, runtimeCandidateSealFileEditFacts(t), runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, state, aggregate)
	manifest, err := runtime.BuildRuntimeCandidateContractManifestForAggregate(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := runtimeOperationsExpectedDocument(aggregate, manifest, 0)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.OperationsRenderContractRef != rc.RuntimeOperationsRenderContractV4 {
		t.Fatal("fresh intent did not select v4")
	}
	for _, renderer := range []string{"", rc.RuntimeOperationsRenderContractV2, rc.RuntimeOperationsRenderContractV3, rc.RuntimeOperationsRenderContractV4} {
		saved := aggregate
		expected := fresh
		expected.OperationsRenderContractRef = renderer
		saved.CandidateRequestSessions = append(saved.CandidateRequestSessions, rc.RuntimeCandidateRequestSession{DocumentKind: rc.RuntimeProviderDocumentOperations, Transaction: aggregate.Decision.AuthorityTransaction(), ManifestHash: manifest.Hash, ExpectedDocument: &expected})
		bytes, _ := json.Marshal(saved)
		var reloaded runtime.RuntimeAggregate
		if err := json.Unmarshal(bytes, &reloaded); err != nil {
			t.Fatal(err)
		}
		for _, attempt := range []int{0, 1} {
			got, err := runtimeOperationsExpectedDocument(reloaded, manifest, attempt)
			if err != nil {
				t.Fatal(err)
			}
			if got.OperationsRenderContractRef != renderer || got.CorrectionAttempt != attempt || got.WireContractHash != fresh.WireContractHash {
				t.Fatal("reload/correction changed sealed rendering or wire")
			}
		}
		foreign := saved.CandidateRequestSessions[0].Clone()
		foreign.Transaction.AuthoritySnapshotID = "other-authority"
		foreign.ExpectedDocument.OperationsRenderContractRef = "foreign"
		saved.CandidateRequestSessions = append(saved.CandidateRequestSessions, foreign)
		if _, err := runtimeOperationsExpectedDocument(saved, manifest, 0); err != nil {
			t.Fatalf("foreign authority contaminated renderer: %v", err)
		}
		conflict := saved.CandidateRequestSessions[0].Clone()
		conflict.ExpectedDocument.OperationsRenderContractRef = "conflicting"
		saved.CandidateRequestSessions = append(saved.CandidateRequestSessions, conflict)
		if _, err := runtimeOperationsExpectedDocument(saved, manifest, 1); err == nil {
			t.Fatal("inconsistent renderer lineage accepted")
		}
	}
}
