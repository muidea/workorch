package workorchruntimecontract

import "testing"

func TestRuntimeValidationExecutionConjunctionDeclarationIdentityCannotBeDropped(t *testing.T) {
	_, attestation := validationExecutionConjunctionFixture()
	other := attestation
	other.RequiredCheckRef, other.NativeCheckRef, other.ExecutionStagesHash = "other-required", "other-native", ""
	if ValidateRuntimeValidationCheckAttestationBindings([]RuntimeValidationCheckAttestationBinding{attestation, other}, []string{"required", "other-required"}) == nil {
		t.Fatal("同一 requirement 混入缺合取的 native mapping")
	}
	for _, raw := range []string{"bad", " " + attestation.ExecutionStagesHash, attestation.ExecutionStagesHash + " "} {
		invalid := attestation
		invalid.ExecutionStagesHash = raw
		if invalid.Validate() == nil || invalid.Normalized().ExecutionStagesHash != raw {
			t.Fatal("非法合取声明被接受或 normalization 改写")
		}
	}
	base := CoverageEvidenceBinding{ID: "binding", CoverageWorkItemID: "cwi", FocusID: "focus", DefinitionRevision: "definition", Proofs: []CoverageEvidenceProof{{EvidenceKind: EvidenceKindValidation, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectEphemeralSession}, AllowedOperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "recipe", ValidationProofRequirementHash: "requirement", RequiredCheckRefs: []string{"required"}, ValidationCheckAttestations: []RuntimeValidationCheckAttestationBinding{attestation}}}}
	changed := base.Clone()
	changed.Proofs[0].ValidationCheckAttestations[0].ExecutionStagesHash = ""
	if StableCoverageEvidenceBindingHash(base) == StableCoverageEvidenceBindingHash(changed) {
		t.Fatal("binding hash 未封存 owner 合取声明")
	}
	reviewBinding := RuntimeValidationProofRequirementBinding{CheckAttestations: []RuntimeValidationCheckAttestationBinding{attestation}}
	oldReview := StableRuntimeValidationInputReviewSubjectHash(RuntimeValidationInputReviewSubject{}, reviewBinding)
	reviewBinding.CheckAttestations[0].ExecutionStagesHash = ""
	if StableRuntimeValidationInputReviewSubjectHash(RuntimeValidationInputReviewSubject{}, reviewBinding) == oldReview {
		t.Fatal("input review hash 未绑定 exact 合取执行声明")
	}
	catalog := RuntimeOperationAdmissibility{ValidationSurfaces: []RuntimeValidationSurface{{ID: "recipe", ProofRequirementBindings: []RuntimeValidationProofRequirementBinding{{RequirementHash: "requirement", CheckAttestations: []RuntimeValidationCheckAttestationBinding{attestation}}}, ProofRequirementPreparations: []RuntimeValidationProofPreparation{{RequirementHash: "requirement", CheckAttestations: []RuntimeValidationCheckAttestationBinding{attestation}}}}}}
	catalogHash := StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	for _, preparation := range []bool{false, true} {
		changed := catalog.Clone()
		if preparation {
			changed.ValidationSurfaces[0].ProofRequirementPreparations[0].CheckAttestations[0].ExecutionStagesHash = ""
		} else {
			changed.ValidationSurfaces[0].ProofRequirementBindings[0].CheckAttestations[0].ExecutionStagesHash = ""
		}
		if StableRuntimeOperationAdmissibilitySnapshotID(changed) == catalogHash {
			t.Fatal("catalog ready/preparation hash 未绑定合取声明")
		}
	}
	identity := CoverageEvidenceProofIdentity{ValidationCheckAttestations: []RuntimeValidationCheckAttestationBinding{attestation}}
	identityHash := StableCoverageEvidenceProofIdentityHash(identity)
	identity.ValidationCheckAttestations[0].ExecutionStagesHash = ""
	if StableCoverageEvidenceProofIdentityHash(identity) == identityHash {
		t.Fatal("proof lineage identity 未绑定合取声明")
	}
	reconciliation := RuntimeCWIReconciliationContract{Bindings: []RuntimeCWIReconciliationBindingRef{{ValidationCheckAttestations: []RuntimeValidationCheckAttestationBinding{attestation}}}}
	reconciliationHash := StableRuntimeCWIReconciliationContractHash(reconciliation)
	reconciliation.Bindings[0].ValidationCheckAttestations[0].ExecutionStagesHash = ""
	if StableRuntimeCWIReconciliationContractHash(reconciliation) == reconciliationHash {
		t.Fatal("reconciliation identity 未绑定合取声明")
	}
}
