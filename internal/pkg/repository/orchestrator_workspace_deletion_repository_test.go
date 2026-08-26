package repository

import (
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestOrchestratorEvidenceOwnerBindingsHaveDistinctNonZeroIdentity(t *testing.T) {
	first := newOrchestratorEvidenceOwnerBinding()
	second := newOrchestratorEvidenceOwnerBinding()
	if first == nil || second == nil || first == second || first.marker == 0 || second.marker == 0 {
		t.Fatalf("owner bindings are not unique non-zero identities: first=%p second=%p", first, second)
	}
}

func TestOrchestratorWorkspaceEvidencePurgeReceiptRequiresExactObservedDeletion(t *testing.T) {
	scope := deletioncontract.Scope{
		WorkspaceID: "ws-receipt", DeletionID: "delete-receipt", DeletionGeneration: 1,
		Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-receipt",
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	observations := EmptyOrchestratorWorkspaceEvidencePurgeObservations()
	for idx := range observations {
		observations[idx].ObservedBefore = uint64(idx + 1)
		observations[idx].Deleted = uint64(idx + 1)
	}
	receipt, err := NewOrchestratorWorkspaceEvidencePurgeReceipt(scope, observations)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.ValidateFor(scope); err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func(*OrchestratorWorkspaceEvidencePurgeReceipt){
		"missing surface": func(value *OrchestratorWorkspaceEvidencePurgeReceipt) {
			value.Observations = value.Observations[:len(value.Observations)-1]
		},
		"no-op sink":    func(value *OrchestratorWorkspaceEvidencePurgeReceipt) { value.Observations[0].Deleted-- },
		"remaining row": func(value *OrchestratorWorkspaceEvidencePurgeReceipt) { value.Observations[0].Remaining++ },
		"forged proof":  func(value *OrchestratorWorkspaceEvidencePurgeReceipt) { value.ProofHash = "forged" },
	} {
		t.Run(name, func(t *testing.T) {
			value := receipt
			value.Observations = append([]OrchestratorWorkspaceEvidencePurgeObservation(nil), receipt.Observations...)
			mutate(&value)
			if value.ValidateFor(scope) == nil {
				t.Fatalf("invalid receipt accepted: %+v", value)
			}
		})
	}
}

func TestOrchestratorEmptyPurgeReceiptProofMatchesTypedOutwardContract(t *testing.T) {
	scope := deletioncontract.Scope{
		WorkspaceID: "ws-empty-receipt", DeletionID: "delete-empty-receipt", DeletionGeneration: 1,
		Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-empty-receipt",
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	receipt, err := NewOrchestratorWorkspaceEvidencePurgeReceipt(scope, EmptyOrchestratorWorkspaceEvidencePurgeObservations())
	if err != nil {
		t.Fatal(err)
	}
	if receipt.ProofHash != orchestratorevents.StableOrchestratorDeletionPurgeProofHash(scope) {
		t.Fatalf("repository proof %q != outward proof %q", receipt.ProofHash, orchestratorevents.StableOrchestratorDeletionPurgeProofHash(scope))
	}
}
