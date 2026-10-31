package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCurrentOperationsEncodingSurvivesReloadAndCorrection(t *testing.T) {
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
	if fresh.OperationsContextEncoding != rc.RuntimeOperationsContextEncoding {
		t.Fatal("fresh intent did not use current operations encoding")
	}
	{
		renderer := rc.RuntimeOperationsContextEncoding
		saved := aggregate
		expected := fresh
		expected.OperationsContextEncoding = renderer
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
			if got.OperationsContextEncoding != renderer || got.CorrectionAttempt != attempt || got.WireContractHash != fresh.WireContractHash {
				t.Fatal("reload/correction changed sealed rendering or wire")
			}
		}
		// The current encoding is compiled from code, never selected from an
		// archived request session. Invalid identities still fail the closed
		// ExpectedDocument contract before any provider submission.
		valid := fresh
		valid.LogicalRequestID, valid.StablePrefixHash = "current-request", "stable-prefix"
		valid.MaxOutputTokens = 1024
		valid.Hash = rc.StableRuntimeExpectedDocumentHash(valid)
		if err := valid.Validate(); err != nil {
			t.Fatalf("current expected document: %v", err)
		}
		for _, unsupported := range []string{"", "unsupported-encoding"} {
			invalid := valid
			invalid.OperationsContextEncoding = unsupported
			invalid.Hash = rc.StableRuntimeExpectedDocumentHash(invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatal("noncurrent encoding accepted")
			}
		}
	}
}
