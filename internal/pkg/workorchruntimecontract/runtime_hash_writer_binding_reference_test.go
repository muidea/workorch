package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

func referenceRuntimeBufferedHashStrings(w hashStringWriter, values []string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(values)))
	w.Write(size[:])
	for _, value := range values {
		runtimeWriteHashStringReference(w, value)
	}
}

func referenceRuntimeBufferedCoverageBindingHash(binding CoverageEvidenceBinding) string {
	h := sha256.New()
	runtimeWriteHashStringReference(h, binding.ID)
	runtimeWriteHashStringReference(h, string(binding.CoverageWorkItemID))
	runtimeWriteHashStringReference(h, binding.FocusID)
	runtimeWriteHashStringReference(h, binding.DefinitionRevision)
	for _, proof := range binding.Proofs {
		runtimeWriteHashStringReference(h, proof.EvidenceKind)
		referenceRuntimeBufferedHashStrings(h, RuntimeEffectScopesAsStrings(proof.RequiredEffectScopes))
		referenceRuntimeBufferedHashStrings(h, proof.AllowedOperationKinds)
		for _, variant := range proof.AllowedOperationVariants {
			runtimeWriteHashStringReference(h, variant.Kind)
			runtimeWriteHashStringReference(h, variant.VariantID)
		}
		runtimeWriteHashStringReference(h, proof.ValidationRecipeID)
		runtimeWriteHashStringReference(h, proof.ValidationProofRequirementHash)
		referenceRuntimeBufferedHashStrings(h, proof.RequiredCheckRefs)
		for _, attestation := range proof.ValidationCheckAttestations {
			runtimeWriteHashStringReference(h, attestation.RequiredCheckRef)
			runtimeWriteHashStringReference(h, attestation.ObservationProtocolRef)
			runtimeWriteHashStringReference(h, attestation.NativeCheckRef)
			writeRuntimeSupplementalNativeCheckHash(h, attestation)
		}
		referenceRuntimeBufferedHashStrings(h, proof.MetricConstraints)
		referenceRuntimeBufferedHashStrings(h, proof.ArtifactRefs)
	}
	for _, assertion := range binding.LineageAssertions {
		runtimeWriteHashStringReference(h, "lineage-assertion")
		runtimeWriteHashStringReference(h, StableRuntimeLineageAssertionRuleHash(assertion.Rule))
		runtimeWriteHashStringReference(h, assertion.RuleHash)
		runtimeWriteHashStringReference(h, assertion.DeclarationRef)
		runtimeWriteHashStringReference(h, string(assertion.CoverageWorkItemID))
		runtimeWriteHashStringReference(h, assertion.SourceScope)
		runtimeWriteHashStringReference(h, assertion.SourceID)
		runtimeWriteHashStringReference(h, assertion.SourceTextHash)
	}
	if binding.VerificationPolicy != nil {
		runtimeWriteHashStringReference(h, "verification-policy")
		runtimeWriteHashStringReference(h, StableRuntimeVerificationPolicyHash(*binding.VerificationPolicy))
	}
	referenceRuntimeBufferedHashStrings(h, binding.TargetRefs)
	runtimeWriteHashStringReference(h, binding.TargetSelectorRef)
	runtimeWriteHashStringReference(h, binding.TargetSelectorOwnerRef)
	runtimeWriteHashStringReference(h, binding.TargetSelectorSubjectRef)
	referenceRuntimeBufferedHashStrings(h, binding.TargetSelectorCandidateRefs)
	referenceRuntimeBufferedHashStrings(h, binding.ProspectiveTargetAllOfConstraintRefs)
	referenceRuntimeBufferedHashStrings(h, binding.ProspectiveTargetRefs)
	runtimeWriteHashStringReference(h, binding.DeclarationRef)
	referenceRuntimeBufferedHashStrings(h, binding.PrerequisiteRefs)
	return hex.EncodeToString(h.Sum(nil))
}
