package orchestrator

import contract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// Deterministic scripted proposal; production never fills missing provider
// fields. Decoder negative tests separately reject absent or stale intent.
func implementationWorkIntentForTest(m contract.RuntimeSemanticPlanManifest) *contract.RuntimeImplementationIntent {
	for _, o := range m.ProofObligations {
		if w := o.ImplementationWork; w != nil && w.PurposeChoiceOnly && w.Scope.Content != nil {
			return &contract.RuntimeImplementationIntent{DeclarationRef: o.DeclarationRef, ContentHash: w.Scope.Content.Hash(), RemainingWork: "original requirement still needs bounded implementation", Reason: "scripted purpose compares exact provided source content"}
		}
	}
	return nil
}
