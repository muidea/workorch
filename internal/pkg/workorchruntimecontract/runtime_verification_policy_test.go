package workorchruntimecontract

import (
	"encoding/json"
	"testing"
)

func TestVerificationPolicyOwnerModalityHashReloadAndValidation(t *testing.T) {
	base := RuntimeValidationObservation{RecipeID: "recipe", ObservationProtocolRef: "owner/native/v1", ExecutedChecks: []string{"check"}, PassedChecks: []string{"check"}}
	receipt := RuntimeOperationReceipt{ExecutionKey: "execution", Fence: 1, OperationID: "operation", Outcome: RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: RuntimeEffectNone, Evidence: RuntimeOperationEvidence{Validation: &base}}
	fact := EvidenceFact{ID: "fact", Kind: EvidenceKindValidation, Validation: &base}
	baseReceiptHash, baseFactHash := StableRuntimeOperationReceiptHash(receipt), StableRuntimeEvidenceFactHash(fact)
	for _, class := range []RuntimeValidationProofClass{RuntimeValidationProofStatic, RuntimeValidationProofExecuted, RuntimeValidationProofBehavioral} {
		observation := base
		observation.ProofClass = class
		if err := observation.ValidateFailureDetails(); err != nil {
			t.Fatalf("class %q invalid: %v", class, err)
		}
		receipt.Evidence.Validation = &observation
		fact.Validation = &observation
		if StableRuntimeOperationReceiptHash(receipt) == baseReceiptHash || StableRuntimeEvidenceFactHash(fact) == baseFactHash {
			t.Fatal("classified observation did not bind both receipt and evidence fact identities")
		}
		cloned := receipt.Clone()
		if cloned.Evidence.Validation.ProofClass != class {
			t.Fatal("owner receipt clone lost proof modality")
		}
		payload, err := json.Marshal(receipt)
		if err != nil {
			t.Fatal(err)
		}
		var reloaded RuntimeOperationReceipt
		if err = json.Unmarshal(payload, &reloaded); err != nil || reloaded.Validate() != nil ||
			StableRuntimeOperationReceiptHash(reloaded) != StableRuntimeOperationReceiptHash(receipt) {
			t.Fatalf("owner proof modality did not survive reload: %v", err)
		}
	}
	base.ProofClass = "llm/fabricated"
	if base.ValidateFailureDetails() == nil {
		t.Fatal("accepted unknown producer-authored proof class")
	}
	base.ProofClass = RuntimeValidationProofBehavioral
	base.OutputTruncated = true
	if base.ValidateFailureDetails() == nil {
		t.Fatal("accepted truncated behavioral attestation")
	}
}

func TestVerificationPolicyVersionHashCloneAndSourceFence(t *testing.T) {
	base := RuntimeProfileContract{ID: "profile", EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{ID: "*", Scope: "dodItem"}}}
	baseHash := StableRuntimeProfileContractsHash([]RuntimeProfileContract{base})
	policy := &RuntimeVerificationPolicy{Version: RuntimeVerificationPolicyVersion, MinimumEvidenceLevel: 2, MinIndependentSources: 2, RequireBoundContext: true}
	base.EvidenceRequirements[0].VerificationPolicy = policy
	if baseHash == StableRuntimeProfileContractsHash([]RuntimeProfileContract{base}) {
		t.Fatal("profile verification policy did not enter stable hash")
	}
	focus := RuntimeDefinitionFocus{ID: "focus", AssessmentScope: "dodItem", AssessmentID: "dod-001", VerificationPolicy: policy}
	if err := ValidateRuntimeDefinitionVerificationPolicies([]RuntimeDefinitionFocus{focus}, []RuntimeProfileContract{base}); err != nil {
		t.Fatal(err)
	}
	missing := focus
	missing.VerificationPolicy = nil
	if err := ValidateRuntimeDefinitionVerificationPolicies([]RuntimeDefinitionFocus{missing}, []RuntimeProfileContract{base}); err == nil {
		t.Fatal("fresh load accepted downgraded definition focus")
	}
	cloned := cloneRuntimeDefinitionFocusRaw(focus)
	cloned.VerificationPolicy.MinimumEvidenceLevel = 0
	if focus.VerificationPolicy.MinimumEvidenceLevel != 2 {
		t.Fatal("raw clone shared policy pointer")
	}
	binding := CoverageEvidenceBinding{ID: "binding", VerificationPolicy: policy}
	bindingHash := StableCoverageEvidenceBindingHash(binding)
	binding.VerificationPolicy = nil
	if bindingHash == StableCoverageEvidenceBindingHash(binding) {
		t.Fatal("binding hash omitted verification policy")
	}
	for _, invalid := range []RuntimeVerificationPolicy{
		{MinimumEvidenceLevel: 1},
		{Version: RuntimeVerificationPolicyVersion, MinimumEvidenceLevel: -1},
		{Version: RuntimeVerificationPolicyVersion, MinimumEvidenceLevel: 6},
		{Version: RuntimeVerificationPolicyVersion, MinIndependentSources: 7},
	} {
		if invalid.Validate() == nil {
			t.Fatalf("accepted invalid policy: %+v", invalid)
		}
	}
	payload, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	var loaded RuntimeProfileContract
	if err := json.Unmarshal(payload, &loaded); err != nil || !SameRuntimeVerificationPolicy(loaded.EvidenceRequirements[0].VerificationPolicy, policy) {
		t.Fatalf("verification policy changed on reload: %v %+v", err, loaded)
	}
}
