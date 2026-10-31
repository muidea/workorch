package runtime

import (
	"encoding/json"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSemanticCacheRendererSurvivesReloadAndSameContractCorrection(t *testing.T) {
	_, _, base, fresh := semanticRequestIntentFixture(t)
	if fresh.SemanticRenderContractRef != rc.RuntimeSemanticRenderContractV4 {
		t.Fatal("new semantic intent did not choose cache layout")
	}
	for _, version := range []string{"", rc.RuntimeSemanticRenderContractV2, rc.RuntimeSemanticRenderContractV3, rc.RuntimeSemanticRenderContractV4} {
		t.Run(version, func(t *testing.T) {
			a := base
			expected := fresh.Clone()
			expected.SemanticRenderContractRef = version
			expected.Hash = rc.StableRuntimeExpectedDocumentHash(expected)
			a.CandidateRequestSessions = []rc.RuntimeCandidateRequestSession{{DocumentKind: rc.RuntimeProviderDocumentSemanticPlan, LogicalRequestID: expected.LogicalRequestID, Transaction: a.Decision.AuthorityTransaction(), ManifestHash: expected.ManifestHash, ExpectedDocument: &expected}}
			raw, err := json.Marshal(a)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded RuntimeAggregate
			if err = json.Unmarshal(raw, &reloaded); err != nil {
				t.Fatal(err)
			}
			same, err := SemanticExpectedDocument(reloaded)
			if err != nil || same.Hash != expected.Hash {
				t.Fatal("reload upgraded sealed semantic request", err)
			}
			reloaded.SemanticPlanning.Attempt = 1
			reloaded.SemanticPlanning.CorrectionDirective = &rc.RuntimeSemanticCorrectionDirective{Hash: "same-manifest-correction"}
			key, err := SemanticRequestIdentityHash(reloaded.SemanticPlanning)
			if err != nil {
				t.Fatal(err)
			}
			reloaded.SemanticPlanning.LogicalRequestID = rc.StableRuntimeCandidateRequestSessionID(rc.RuntimeProviderDocumentSemanticPlan, reloaded.Facts.WorkspaceID, reloaded.Facts.RunID, reloaded.Decision.AuthorityTransaction(), fresh.StablePrefixHash, fresh.ManifestHash, key)
			correction, err := SemanticExpectedDocument(reloaded)
			if err != nil || correction.SemanticRenderContractRef != version || correction.CorrectionAttempt != 1 {
				t.Fatal("correction changed original renderer", err)
			}
			conflict := correction.Clone()
			conflict.SemanticRenderContractRef = "foreign-version"
			conflict.Hash = rc.StableRuntimeExpectedDocumentHash(conflict)
			reloaded.CandidateRequestSessions = append(reloaded.CandidateRequestSessions, rc.RuntimeCandidateRequestSession{DocumentKind: rc.RuntimeProviderDocumentSemanticPlan, LogicalRequestID: correction.LogicalRequestID, Transaction: reloaded.Decision.AuthorityTransaction(), ManifestHash: correction.ManifestHash, ExpectedDocument: &conflict})
			if _, err := SemanticExpectedDocument(reloaded); err == nil {
				t.Fatal("mixed same-contract renderer lineage accepted")
			}
		})
	}
}

func TestSemanticCacheRendererKeepsNilLegacySessionAndRejectsForeignScope(t *testing.T) {
	_, _, a, expected := semanticRequestIntentFixture(t)
	a.CandidateRequestSessions = []rc.RuntimeCandidateRequestSession{{DocumentKind: rc.RuntimeProviderDocumentSemanticPlan, LogicalRequestID: expected.LogicalRequestID, Transaction: a.Decision.AuthorityTransaction(), ManifestHash: expected.ManifestHash}}
	legacy, err := SemanticExpectedDocument(a)
	if err != nil || legacy.SemanticRenderContractRef != "" {
		t.Fatal("nil legacy session upgraded", err)
	}
	for _, field := range []string{"kind", "transaction", "manifest"} {
		t.Run(field, func(t *testing.T) {
			foreign := a
			foreign.CandidateRequestSessions = append([]rc.RuntimeCandidateRequestSession(nil), a.CandidateRequestSessions...)
			switch field {
			case "kind":
				foreign.CandidateRequestSessions[0].DocumentKind = rc.RuntimeProviderDocumentOperations
			case "transaction":
				foreign.CandidateRequestSessions[0].Transaction.AuthoritySnapshotID = "foreign-authority"
			case "manifest":
				foreign.CandidateRequestSessions[0].ManifestHash = "foreign-manifest"
			}
			if _, err := SemanticExpectedDocument(foreign); err == nil {
				t.Fatal("same logical ID accepted foreign renderer scope")
			}
		})
	}
	// A different generation/request ID cannot select the renderer for a fresh
	// intent, even when the transaction/manifest are otherwise equal.
	a.CandidateRequestSessions[0].LogicalRequestID = "retired-generation"
	fresh, err := SemanticExpectedDocument(a)
	if err != nil || fresh.SemanticRenderContractRef != rc.RuntimeSemanticRenderContractV4 {
		t.Fatal("retired request prevented fresh renderer selection", err)
	}
}

func TestSemanticCacheRendererProofBatchCorrectionKeepsSourceVersion(t *testing.T) {
	_, _, base, _ := semanticRequestIntentFixture(t)
	base.SemanticPlanning.Batches = []rc.RuntimeSemanticPlanBatch{{BatchID: "proof-batch", Sequence: 2}}
	base.SemanticPlanning.ActiveBatchID = "proof-batch"
	key, err := SemanticRequestIdentityHash(base.SemanticPlanning)
	if err != nil {
		t.Fatal(err)
	}
	base.SemanticPlanning.LogicalRequestID = rc.StableRuntimeCandidateRequestSessionID(rc.RuntimeProviderDocumentSemanticPlan, base.Facts.WorkspaceID, base.Facts.RunID, base.Decision.AuthorityTransaction(), base.SemanticPlanning.StablePrefixHash, base.SemanticPlanning.Manifest.Hash, key)
	for _, version := range []string{"", rc.RuntimeSemanticRenderContractV2, rc.RuntimeSemanticRenderContractV3, rc.RuntimeSemanticRenderContractV4} {
		a := base
		expected, err := SemanticExpectedDocument(a)
		if err != nil || expected.PlanStep != rc.RuntimeSemanticPlanStepProofBatch || expected.BatchSequence != 2 {
			t.Fatal("proof-batch intent projection changed", err)
		}
		expected.SemanticRenderContractRef = version
		expected.Hash = rc.StableRuntimeExpectedDocumentHash(expected)
		a.CandidateRequestSessions = []rc.RuntimeCandidateRequestSession{{DocumentKind: rc.RuntimeProviderDocumentSemanticPlan, LogicalRequestID: expected.LogicalRequestID, Transaction: a.Decision.AuthorityTransaction(), ManifestHash: expected.ManifestHash, ExpectedDocument: &expected}}
		a.SemanticPlanning.Attempt = 1
		a.SemanticPlanning.CorrectionDirective = &rc.RuntimeSemanticCorrectionDirective{Hash: "proof-batch-correction"}
		key, err := SemanticRequestIdentityHash(a.SemanticPlanning)
		if err != nil {
			t.Fatal(err)
		}
		a.SemanticPlanning.LogicalRequestID = rc.StableRuntimeCandidateRequestSessionID(rc.RuntimeProviderDocumentSemanticPlan, a.Facts.WorkspaceID, a.Facts.RunID, a.Decision.AuthorityTransaction(), expected.StablePrefixHash, expected.ManifestHash, key)
		got, err := SemanticExpectedDocument(a)
		if err != nil || got.SemanticRenderContractRef != version || got.BatchSequence != 2 {
			t.Fatal("proof correction changed source renderer or batch", err)
		}
	}
}
