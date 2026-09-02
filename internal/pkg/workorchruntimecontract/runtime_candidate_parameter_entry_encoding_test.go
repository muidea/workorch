package workorchruntimecontract

import "testing"

func TestRuntimeCandidateParameterEntryEncodingBindsCloneAndCorrectionHashes(t *testing.T) {
	encodings := []RuntimeCandidateParameterEntryEncodingViolation{{
		OperationIndex: 0, ParameterIndex: 1, OperationKind: RuntimeBlockKindPatchApply, ContainerParameterName: "content",
		MisplacedParameters: []RuntimeCandidateMisplacedParameter{
			{Name: "baseHash", Type: RuntimeOperationParameterTypeString},
			{Name: "coverageTargets", Type: RuntimeOperationParameterTypeStringList},
		},
	}}
	witness := RuntimeCandidateCorrectionWitness{
		Dimension: RuntimeCandidateCorrectionWitnessParameterEntryEncoding, Path: "$.operations",
		RejectedValue:           RuntimeCandidateParameterEncodingInlineSiblings,
		AlternativeValue:        RuntimeCandidateParameterEncodingNamedEntries,
		ParameterEntryEncodings: encodings,
	}.Normalized()
	objectives := []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionReencodeParameterEntries}
	diagnostic := (&RuntimeCandidateFailureDiagnostic{
		Path: "$.operations[0].parameters[1].baseHash", Actual: "baseHash",
		WireRule: &RuntimeCandidateWireRuleIdentity{
			WireContractHash: "wire-contract-1", OperationIndex: 0, OperationKind: RuntimeBlockKindPatchApply,
		},
		ViolationKind:           RuntimeCandidatePayloadViolationParameterEntryEncoding,
		ParameterEntryEncodings: encodings, ManifestHash: "manifest-1", Objectives: objectives,
	}).Clone()
	observation := RuntimeCandidateCorrectionObservation{
		Class: RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		Diagnostic: diagnostic,
	}
	observation.SealedCorrectionHash = StableRuntimeCandidateSealedCorrectionHash("manifest-1", objectives, &witness)
	if err := observation.Validate(); err != nil {
		t.Fatal(err)
	}
	sealedHash := observation.SealedCorrectionHash
	requestHash := StableRuntimeCandidateRequestCorrectionHash(&observation, "manifest-1", objectives, &witness)
	if sealedHash == "" || requestHash == "" {
		t.Fatalf("parameter-entry correction hashes are empty: sealed=%q request=%q", sealedHash, requestHash)
	}

	clone := witness.Clone()
	clone.ParameterEntryEncodings[0].MisplacedParameters[0].Type = RuntimeOperationParameterTypeBoolean
	if witness.ParameterEntryEncodings[0].MisplacedParameters[0].Type != RuntimeOperationParameterTypeString {
		t.Fatal("parameter-entry witness clone aliases nested facts")
	}
	if changed := StableRuntimeCandidateSealedCorrectionHash("manifest-1", objectives, clone); changed == "" || changed == sealedHash {
		t.Fatalf("nested parameter type did not change sealed hash: %q", changed)
	}
	changedObservation := observation.Clone()
	changedObservation.Diagnostic.ParameterEntryEncodings[0].MisplacedParameters[0].Type = RuntimeOperationParameterTypeBoolean
	if changed := StableRuntimeCandidateRequestCorrectionHash(&changedObservation, "manifest-1", objectives, clone); changed == "" || changed == requestHash {
		t.Fatalf("nested diagnostic/witness type did not change request hash: %q", changed)
	}
	if observation.Diagnostic.ParameterEntryEncodings[0].MisplacedParameters[0].Type != RuntimeOperationParameterTypeString {
		t.Fatal("parameter-entry observation clone aliases nested diagnostic facts")
	}
}
