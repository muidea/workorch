package runtime

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestVerificationPolicySealedOwnerModalityAndScope(t *testing.T) {
	aggregate, _ := assessmentGateFixture(t, "behavior", "TestBehavior")
	binding := aggregate.Facts.Focus.CoverageEvidenceBindings[0]
	policy := &runtimecontract.RuntimeVerificationPolicy{Version: runtimecontract.RuntimeVerificationPolicyVersion, MinimumEvidenceLevel: 4, MinIndependentSources: 1, RequireBoundContext: true}
	binding.VerificationPolicy = policy
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	aggregate.Facts.Focus.VerificationPolicy = policy
	aggregate.Facts.Focus.CoverageEvidenceBindings = []runtimecontract.CoverageEvidenceBinding{binding}
	runtimeTestSynchronizeCanonicalCWILedger(t, &aggregate)
	fact := &aggregate.Facts.Evidence.Accepted[0]
	fact.AuthoritySnapshotID = "sealed-authority"
	fact.TargetRefs = []string{"target.txt"}
	observation := *fact.Validation
	observation.ProofClass = runtimecontract.RuntimeValidationProofBehavioral
	fact.Validation = &observation
	recipeID := "test-recipe"
	aggregate.CompletedDispatches[0].Candidate.Validation.Operations[0].Operation.Parameters = []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}}
	recipe := runtimecontract.RuntimeValidationRecipe{ID: observation.RecipeID, Command: "go", Args: []string{"test", "-v", "./..."}}
	dispatch := &aggregate.CompletedDispatches[0]
	dispatch.ValidationRecipes = []runtimecontract.RuntimeValidationRecipe{recipe}
	receipt := runtimecontract.RuntimeOperationReceipt{ExecutionKey: "execution-claim", Fence: 1, OperationID: fact.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
		Evidence: runtimecontract.RuntimeOperationEvidence{Validation: &observation}}
	aggregate.OperationReceipts = []runtimecontract.RuntimeOperationReceipt{receipt}
	dispatch.OperationExecutions = []runtimecontract.RuntimeOperationExecutionRecord{{
		OperationID: fact.OperationID, ExecutionKey: receipt.ExecutionKey, Fence: receipt.Fence,
		State:            runtimecontract.OperationExecutionEvidenceCommitted,
		OwnerReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(receipt),
		EvidenceFactID:   fact.ID, EvidenceFactHash: runtimecontract.StableRuntimeEvidenceFactHash(*fact),
	}}
	refs := []string{string(fact.ID)}
	if !runtimeVerificationPolicySatisfied(aggregate, binding, "cwi-claim", refs) {
		t.Fatal("sealed recipe, exact owner receipt and native behavioral observation did not meet level 4")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*RuntimeAggregate)
	}{
		{"missing recipe", func(a *RuntimeAggregate) { a.CompletedDispatches[0].ValidationRecipes = nil }},
		{"wrong recipe", func(a *RuntimeAggregate) { a.CompletedDispatches[0].ValidationRecipes[0].ID = "other" }},
		{"operation recipe mismatch", func(a *RuntimeAggregate) {
			other := "other"
			a.CompletedDispatches[0].Candidate.Validation.Operations[0].Operation.Parameters[0].String = &other
		}},
		{"receipt grade mismatch", func(a *RuntimeAggregate) {
			a.OperationReceipts[0].Evidence.Validation.ProofClass = runtimecontract.RuntimeValidationProofStatic
		}},
		{"fact grade mismatch", func(a *RuntimeAggregate) {
			a.Facts.Evidence.Accepted[0].Validation.ProofClass = runtimecontract.RuntimeValidationProofExecuted
		}},
		{"stale fence", func(a *RuntimeAggregate) { a.OperationReceipts[0].Fence++ }},
		{"missing context", func(a *RuntimeAggregate) { a.Facts.Evidence.Accepted[0].AuthoritySnapshotID = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := aggregate
			copy.OperationReceipts = append([]runtimecontract.RuntimeOperationReceipt(nil), aggregate.OperationReceipts...)
			copy.Facts.Evidence.Accepted = append([]runtimecontract.EvidenceFact(nil), aggregate.Facts.Evidence.Accepted...)
			copy.CompletedDispatches = append([]SealedDispatch(nil), aggregate.CompletedDispatches...)
			copy.CompletedDispatches[0].ValidationRecipes = append([]runtimecontract.RuntimeValidationRecipe(nil), aggregate.CompletedDispatches[0].ValidationRecipes...)
			copy.CompletedDispatches[0].Candidate.Validation = &runtimecontract.ValidationCandidate{Operations: append([]runtimecontract.ValidationOperation(nil), aggregate.CompletedDispatches[0].Candidate.Validation.Operations...)}
			copy.CompletedDispatches[0].Candidate.Validation.Operations[0].Operation.Parameters = append([]runtimecontract.OperationParameter(nil), aggregate.CompletedDispatches[0].Candidate.Validation.Operations[0].Operation.Parameters...)
			copy.OperationReceipts[0].Evidence.Validation = pointerValidationObservation(*aggregate.OperationReceipts[0].Evidence.Validation)
			copy.Facts.Evidence.Accepted[0].Validation = pointerValidationObservation(*aggregate.Facts.Evidence.Accepted[0].Validation)
			tc.mutate(&copy)
			if runtimeVerificationPolicySatisfied(copy, binding, "cwi-claim", refs) {
				t.Fatal("stale or unsealed modality earned behavioral grade")
			}
		})
	}
	policy.MinimumEvidenceLevel = 3
	if !runtimeVerificationPolicySatisfied(aggregate, binding, "cwi-claim", refs) {
		t.Fatal("behavioral validation did not satisfy executed minimum")
	}
	policy.MinimumEvidenceLevel = 2
	if !runtimeVerificationPolicySatisfied(aggregate, binding, "cwi-claim", refs) {
		t.Fatal("behavioral validation did not satisfy static minimum")
	}
	policy.MinimumEvidenceLevel = 5
	if runtimeVerificationPolicySatisfied(aggregate, binding, "cwi-claim", refs) {
		t.Fatal("behavioral observation claimed unproven independent validation")
	}
}

func pointerValidationObservation(value runtimecontract.RuntimeValidationObservation) *runtimecontract.RuntimeValidationObservation {
	return &value
}

func TestVerificationPolicyClosureUsesOwnerReceiptNotEvidenceCount(t *testing.T) {
	aggregate, _ := assessmentGateFixture(t, "behavior", "TestBehavior")
	policy := &runtimecontract.RuntimeVerificationPolicy{Version: runtimecontract.RuntimeVerificationPolicyVersion, MinimumEvidenceLevel: 1, MinIndependentSources: 1, RequireBoundContext: true}
	binding := aggregate.Facts.Focus.CoverageEvidenceBindings[0]
	binding.VerificationPolicy = policy
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	aggregate.Facts.Focus.VerificationPolicy = policy
	aggregate.Facts.Focus.CoverageEvidenceBindings = []runtimecontract.CoverageEvidenceBinding{binding}
	runtimeTestSynchronizeCanonicalCWILedger(t, &aggregate)
	fact := &aggregate.Facts.Evidence.Accepted[0]
	if runtimeVerificationPolicySatisfied(aggregate, binding, "cwi-claim", []string{string(fact.ID)}) {
		t.Fatal("uncommitted synthetic evidence claimed a verification grade")
	}
	fact.AuthoritySnapshotID = "sealed-authority"
	fact.TargetRefs = []string{"target.txt"}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "execution-claim", Fence: 1, OperationID: fact.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
		Evidence: runtimecontract.RuntimeOperationEvidence{Validation: fact.Validation},
	}
	aggregate.OperationReceipts = []runtimecontract.RuntimeOperationReceipt{receipt}
	aggregate.CompletedDispatches[0].OperationExecutions = []runtimecontract.RuntimeOperationExecutionRecord{{
		OperationID: fact.OperationID, ExecutionKey: receipt.ExecutionKey, Fence: receipt.Fence,
		State:            runtimecontract.OperationExecutionEvidenceCommitted,
		OwnerReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(receipt),
		EvidenceFactID:   fact.ID, EvidenceFactHash: runtimecontract.StableRuntimeEvidenceFactHash(*fact),
	}}
	if !runtimeVerificationPolicySatisfied(aggregate, binding, "cwi-claim", []string{string(fact.ID)}) {
		t.Fatal("exact owner receipt did not meet owner-backed inspection level 1")
	}
	for _, mutation := range []struct {
		name  string
		apply func(*RuntimeAggregate)
	}{
		{"stale receipt", func(a *RuntimeAggregate) { a.OperationReceipts[0].Fence++ }},
		{"unbound context", func(a *RuntimeAggregate) { a.Facts.Evidence.Accepted[0].AuthoritySnapshotID = "" }},
		{"fabricated source", func(a *RuntimeAggregate) { a.Facts.Evidence.Accepted[0].SourceRef = "other-owner" }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			copy := aggregate
			copy.Facts.Evidence.Accepted = append([]runtimecontract.EvidenceFact(nil), aggregate.Facts.Evidence.Accepted...)
			copy.OperationReceipts = append([]runtimecontract.RuntimeOperationReceipt(nil), aggregate.OperationReceipts...)
			mutation.apply(&copy)
			if mutation.name == "fabricated source" {
				copy.CompletedDispatches = append([]SealedDispatch(nil), aggregate.CompletedDispatches...)
				copy.CompletedDispatches[0].OperationExecutions = append([]runtimecontract.RuntimeOperationExecutionRecord(nil), aggregate.CompletedDispatches[0].OperationExecutions...)
				copy.CompletedDispatches[0].OperationExecutions[0].EvidenceFactHash = runtimecontract.StableRuntimeEvidenceFactHash(copy.Facts.Evidence.Accepted[0])
				if !runtimeVerificationPolicySatisfied(copy, binding, "cwi-claim", []string{string(fact.ID)}) {
					t.Fatal("source label may not change owner-derived grade")
				}
				return
			}
			if runtimeVerificationPolicySatisfied(copy, binding, "cwi-claim", []string{string(fact.ID)}) {
				t.Fatal("stale or unbound owner receipt met verification policy")
			}
		})
	}
	for _, minimum := range []int{2, 3, 4, 5} {
		policy.MinimumEvidenceLevel = minimum
		policy.MinIndependentSources = 1
		if runtimeVerificationPolicySatisfied(aggregate, binding, "cwi-claim", []string{string(fact.ID), string(fact.ID)}) {
			t.Fatalf("unclassified owner observation was promoted to level %d", minimum)
		}
	}
	policy.MinimumEvidenceLevel = 3
	policy.MinIndependentSources = 1
	validation := *fact.Validation
	validation.ProofClass = runtimecontract.RuntimeValidationProofStatic
	fact.Validation = &validation
	receipt.Evidence.Validation = &validation
	aggregate.OperationReceipts[0] = receipt
	aggregate.CompletedDispatches[0].OperationExecutions[0].OwnerReceiptHash = runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	aggregate.CompletedDispatches[0].OperationExecutions[0].EvidenceFactHash = runtimecontract.StableRuntimeEvidenceFactHash(*fact)
	recipeID := validation.RecipeID
	aggregate.CompletedDispatches[0].Candidate.Validation.Operations[0].Operation.Parameters = []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}}
	aggregate.CompletedDispatches[0].ValidationRecipes = []runtimecontract.RuntimeValidationRecipe{{ID: recipeID, Command: "go", Args: []string{"test", "./..."}}}
	policy.MinimumEvidenceLevel = 2
	if !runtimeVerificationPolicySatisfied(aggregate, binding, "cwi-claim", []string{string(fact.ID)}) {
		t.Fatal("exact static owner attestation did not meet static validation level")
	}
	policy.MinimumEvidenceLevel = 3
	if runtimeVerificationPolicySatisfied(aggregate, binding, "cwi-claim", []string{string(fact.ID), string(fact.ID)}) {
		t.Fatal("even multiple static owners cannot upgrade static validation to executed validation")
	}
	policy.MinimumEvidenceLevel = 1
	policy.MinIndependentSources = 2
	if runtimeVerificationPolicySatisfied(aggregate, binding, "cwi-claim", []string{string(fact.ID), string(fact.ID)}) {
		t.Fatal("duplicate receipt from one owner met two independent sources")
	}
}
