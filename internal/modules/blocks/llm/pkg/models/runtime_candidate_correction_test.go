package models

import (
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func candidateCorrectionTestManifest() runtimecontract.RuntimeCandidateContractManifest {
	manifest := runtimecontract.RuntimeCandidateContractManifest{
		Revision:               runtimecontract.RuntimeCandidateContractManifestRevision,
		State:                  runtimecontract.ConvergenceStateNeedStateChange,
		RequiredTopLevelFields: []string{"operations"},
		AllowedTopLevelFields:  []string{"operations"},
		ForbiddenTopLevelFields: []string{
			"id", "kind", "requiredOperationRef", "dependsOn", "coverageWorkItemIds", "targetRefs", "parameters",
		},
	}
	manifest = manifest.Normalized()
	manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)
	return manifest
}

func candidateSchemaCorrectionPair(manifest runtimecontract.RuntimeCandidateContractManifest) (RuntimeCandidateCorrectionInput, runtimecontract.RuntimeCandidateCorrectionObservation) {
	input := RuntimeCandidateCorrectionInput{
		ManifestHash: manifest.Hash,
		Objectives:   []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField},
		Witness: runtimecontract.RuntimeCandidateCorrectionWitness{
			Dimension: runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField,
			Path:      "$.operations", RejectedValue: runtimeCandidateCorrectionFieldAbsent, AlternativeValue: runtimeCandidateCorrectionFieldPresent,
		},
	}.Normalized()
	observation := runtimecontract.RuntimeCandidateCorrectionObservation{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations", Actual: "missing_operations", Required: []string{"operations"}, ManifestHash: manifest.Hash,
			Objectives: append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), input.Objectives...),
		},
		SealedCorrectionHash: runtimecontract.StableRuntimeCandidateSealedCorrectionHash(input.ManifestHash, input.Objectives, &input.Witness),
	}.Normalized()
	return input, observation
}

func TestRuntimeCandidateCorrectionInputValidatesExactSealedObservation(t *testing.T) {
	t.Parallel()
	manifest := candidateCorrectionTestManifest()
	input, observation := candidateSchemaCorrectionPair(manifest)
	if err := input.ValidateAgainstObservation(manifest, observation); err != nil {
		t.Fatal(err)
	}

	tests := map[string]func(*RuntimeCandidateCorrectionInput, *runtimecontract.RuntimeCandidateCorrectionObservation, *runtimecontract.RuntimeCandidateContractManifest){
		"objective": func(input *RuntimeCandidateCorrectionInput, _ *runtimecontract.RuntimeCandidateCorrectionObservation, _ *runtimecontract.RuntimeCandidateContractManifest) {
			input.Objectives = []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionRemoveForbiddenField}
		},
		"witness": func(input *RuntimeCandidateCorrectionInput, _ *runtimecontract.RuntimeCandidateCorrectionObservation, _ *runtimecontract.RuntimeCandidateContractManifest) {
			input.Witness.AlternativeValue = "another_present_value"
		},
		"manifest input": func(input *RuntimeCandidateCorrectionInput, _ *runtimecontract.RuntimeCandidateCorrectionObservation, _ *runtimecontract.RuntimeCandidateContractManifest) {
			input.ManifestHash = "stale-manifest"
		},
		"observation path": func(_ *RuntimeCandidateCorrectionInput, observation *runtimecontract.RuntimeCandidateCorrectionObservation, _ *runtimecontract.RuntimeCandidateContractManifest) {
			observation.Diagnostic.Path = "$.unexpected"
		},
		"manifest contents": func(_ *RuntimeCandidateCorrectionInput, _ *runtimecontract.RuntimeCandidateCorrectionObservation, manifest *runtimecontract.RuntimeCandidateContractManifest) {
			manifest.RequiredTopLevelFields = nil
			manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(*manifest)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changedInput := input.Normalized()
			changedObservation := observation.Clone()
			changedManifest := manifest.Clone()
			mutate(&changedInput, &changedObservation, &changedManifest)
			if err := changedInput.ValidateAgainstObservation(changedManifest, changedObservation); err == nil {
				t.Fatal("mismatched candidate correction pair was accepted")
			}
		})
	}
}

func candidateParameterEntryCorrectionPair(t *testing.T) (runtimecontract.RuntimeCandidateContractManifest, RuntimeCandidateCorrectionInput, runtimecontract.RuntimeCandidateCorrectionObservation) {
	t.Helper()
	operationContract, found := runtimecontract.RuntimeOperationContractForKind(runtimecontract.RuntimeBlockKindPatchApply)
	if !found {
		t.Fatal("patch/apply contract is missing")
	}
	manifest := runtimecontract.RuntimeCandidateContractManifest{
		Revision: runtimecontract.RuntimeCandidateContractManifestRevision,
		State:    runtimecontract.ConvergenceStateNeedStateChange, MaxOperationCount: 1,
		RequiredTopLevelFields: []string{"operations"}, AllowedTopLevelFields: []string{"operations"},
		Operations: []runtimecontract.RuntimeCandidateOperationManifest{{
			Kind: operationContract.Kind, AllowedOperationFields: []string{"kind", "bindingRefs", "targetRefs", "parameters"},
			RequiredOperationFields: []string{"kind", "targetRefs", "parameters"},
			Parameters:              operationContract.Parameters, RequireOneOfParameters: operationContract.RequireOneOfParameters,
			TargetCardinality: runtimecontract.RuntimeOperationTargetCardinalityOne, RequireTargetRefs: true,
			ForbiddenParameters: operationContract.ForbiddenParameters, Variants: operationContract.Variants, Rules: operationContract.Rules,
		}},
	}
	manifest = manifest.Normalized()
	manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)
	encodings := []runtimecontract.RuntimeCandidateParameterEntryEncodingViolation{{
		OperationIndex: 0, ParameterIndex: 1, OperationKind: runtimecontract.RuntimeBlockKindPatchApply, ContainerParameterName: "content",
		MisplacedParameters: []runtimecontract.RuntimeCandidateMisplacedParameter{
			{Name: "baseHash", Type: runtimecontract.RuntimeOperationParameterTypeString},
			{Name: "coverageTargets", Type: runtimecontract.RuntimeOperationParameterTypeStringList},
			{Name: "summary", Type: runtimecontract.RuntimeOperationParameterTypeString},
		},
	}}
	input := RuntimeCandidateCorrectionInput{
		ManifestHash: manifest.Hash,
		Objectives:   []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionReencodeParameterEntries},
		Witness: runtimecontract.RuntimeCandidateCorrectionWitness{
			Dimension: runtimecontract.RuntimeCandidateCorrectionWitnessParameterEntryEncoding, Path: "$.operations",
			RejectedValue:           runtimecontract.RuntimeCandidateParameterEncodingInlineSiblings,
			AlternativeValue:        runtimecontract.RuntimeCandidateParameterEncodingNamedEntries,
			ParameterEntryEncodings: encodings,
		},
	}.Normalized()
	observation := runtimecontract.RuntimeCandidateCorrectionObservation{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations[0].parameters[1].baseHash", Actual: "baseHash",
			Allowed:                 []string{"name", "string", "integer", "boolean", "stringList"},
			ViolationKind:           runtimecontract.RuntimeCandidatePayloadViolationParameterEntryEncoding,
			ParameterEntryEncodings: encodings, ManifestHash: manifest.Hash,
			Objectives: []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionReencodeParameterEntries},
		},
	}.Normalized()
	observation.SealedCorrectionHash = runtimecontract.StableRuntimeCandidateSealedCorrectionHash(input.ManifestHash, input.Objectives, &input.Witness)
	return manifest, input, observation
}

func TestRuntimeCandidateCorrectionInputValidatesParameterEntryEncodingAgainstExactManifest(t *testing.T) {
	manifest, input, observation := candidateParameterEntryCorrectionPair(t)
	if err := input.ValidateAgainstObservation(manifest, observation); err != nil {
		t.Fatal(err)
	}

	tests := map[string]func(*runtimecontract.RuntimeCandidateParameterEntryEncodingViolation){
		"operation index": func(value *runtimecontract.RuntimeCandidateParameterEntryEncodingViolation) { value.OperationIndex = 1 },
		"operation kind": func(value *runtimecontract.RuntimeCandidateParameterEntryEncodingViolation) {
			value.OperationKind = "shell/command"
		},
		"container": func(value *runtimecontract.RuntimeCandidateParameterEntryEncodingViolation) {
			value.ContainerParameterName = "invented"
		},
		"parameter name": func(value *runtimecontract.RuntimeCandidateParameterEntryEncodingViolation) {
			value.MisplacedParameters[0].Name = "invented"
		},
		"parameter type": func(value *runtimecontract.RuntimeCandidateParameterEntryEncodingViolation) {
			value.MisplacedParameters[0].Type = runtimecontract.RuntimeOperationParameterTypeBoolean
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changedInput := input.Normalized()
			changedObservation := observation.Clone()
			mutate(&changedInput.Witness.ParameterEntryEncodings[0])
			changedObservation.Diagnostic.ParameterEntryEncodings = changedInput.Witness.ParameterEntryEncodings
			changedObservation.SealedCorrectionHash = runtimecontract.StableRuntimeCandidateSealedCorrectionHash(changedInput.ManifestHash, changedInput.Objectives, &changedInput.Witness)
			if err := changedInput.ValidateAgainstObservation(manifest, changedObservation); err == nil {
				t.Fatal("manifest-drifted parameter-entry correction reached provider preflight")
			}
		})
	}

	duplicateInput := input.Normalized()
	duplicateObservation := observation.Clone()
	duplicateInput.Witness.ParameterEntryEncodings = append(duplicateInput.Witness.ParameterEntryEncodings, duplicateInput.Witness.ParameterEntryEncodings[0].Clone())
	duplicateObservation.Diagnostic.ParameterEntryEncodings = duplicateInput.Witness.ParameterEntryEncodings
	duplicateObservation.SealedCorrectionHash = runtimecontract.StableRuntimeCandidateSealedCorrectionHash(duplicateInput.ManifestHash, duplicateInput.Objectives, &duplicateInput.Witness)
	if err := duplicateInput.ValidateAgainstObservation(manifest, duplicateObservation); err == nil {
		t.Fatal("duplicate parameter-entry correction reached provider preflight")
	}
}

func TestRuntimeCandidateCorrectionInputRejectsAllowedButOptionalFieldAsRequired(t *testing.T) {
	t.Parallel()
	manifest := candidateCorrectionTestManifest()
	manifest.AllowedTopLevelFields = append(manifest.AllowedTopLevelFields, "optional")
	manifest = manifest.Normalized()
	manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)
	input := RuntimeCandidateCorrectionInput{
		ManifestHash: manifest.Hash,
		Objectives:   []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField},
		Witness: runtimecontract.RuntimeCandidateCorrectionWitness{
			Dimension: runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField,
			Path:      "$.optional", RejectedValue: runtimeCandidateCorrectionFieldAbsent, AlternativeValue: runtimeCandidateCorrectionFieldPresent,
		},
	}.Normalized()
	observation := runtimecontract.RuntimeCandidateCorrectionObservation{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.optional", Actual: "missing_optional", Required: []string{"optional"}, ManifestHash: manifest.Hash,
			Objectives: append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), input.Objectives...),
		},
		SealedCorrectionHash: runtimecontract.StableRuntimeCandidateSealedCorrectionHash(input.ManifestHash, input.Objectives, &input.Witness),
	}.Normalized()
	if err := input.ValidateAgainstObservation(manifest, observation); err == nil {
		t.Fatal("allowed but optional field was accepted as required correction authority")
	}
}

func TestRuntimeCandidateManifestCorrectionProofIsFailClosedAcrossOperationKinds(t *testing.T) {
	operationA := runtimecontract.RuntimeCandidateOperationManifest{
		Kind: "kind/a", AllowedOperationFields: []string{"kind", "parameters", "targetRefs"},
		RequiredOperationFields: []string{"kind", "parameters", "targetRefs"}, RequireTargetRefs: true,
		Parameters: []runtimecontract.RuntimeOperationParameterContract{
			{Name: "requiredValue", Required: true},
			{Name: "optionalValue", Required: false},
		},
	}
	operationB := runtimecontract.RuntimeCandidateOperationManifest{
		Kind: "kind/b", AllowedOperationFields: []string{"kind", "parameters", "targetRefs", "optionalEnvelope"},
		RequiredOperationFields: []string{"kind", "parameters"},
		Parameters: []runtimecontract.RuntimeOperationParameterContract{
			{Name: "requiredValue", Required: true},
			{Name: "optionalValue", Required: false},
		},
	}
	manifest := runtimecontract.RuntimeCandidateContractManifest{Operations: []runtimecontract.RuntimeCandidateOperationManifest{operationA, operationB}}
	if runtimeCandidateManifestForbidsCorrectionField(runtimecontract.RuntimeCandidateContractManifest{}, "$.operations[0].id", "id") {
		t.Fatal("manifest without an operation authorization proved a forbidden operation field")
	}
	if runtimeCandidateManifestRequiresCorrectionField(manifest, "$.operations[0].targetRefs", "targetRefs") {
		t.Fatal("requirement from one operation kind crossed a multi-kind correction boundary")
	}
	if !runtimeCandidateManifestRequiresCorrectionField(manifest, "$.operations[0].parameters[name=requiredValue]", "requiredValue") {
		t.Fatal("field required by every operation kind lost correction authority")
	}
	if runtimeCandidateManifestRequiresCorrectionField(manifest, "$.operations[0].parameters[name=optionalValue]", "optionalValue") {
		t.Fatal("declared optional parameter was accepted as required")
	}
	if runtimeCandidateManifestForbidsCorrectionField(manifest, "$.operations[0].parameters[name=optionalValue]", "optionalValue") {
		t.Fatal("declared optional parameter was accepted as forbidden")
	}
	if runtimeCandidateManifestForbidsCorrectionField(manifest, "$.operations[0].optionalEnvelope", "optionalEnvelope") {
		t.Fatal("operation field allowed by one authorized kind was treated as universally forbidden")
	}
	if !runtimeCandidateManifestForbidsCorrectionField(manifest, "$.operations[0].parameters[name=unknownValue]", "unknownValue") {
		t.Fatal("undeclared parameter in every closed operation kind was not forbidden")
	}
	manifest.Operations[1].AllowsDynamicParameters = true
	manifest.Operations[1].DynamicParameterPrefixes = []string{"dynamic."}
	if runtimeCandidateManifestForbidsCorrectionField(manifest, "$.operations[0].parameters[name=dynamic.value]", "dynamic.value") {
		t.Fatal("parameter authorized dynamically by one possible kind was treated as universally forbidden")
	}
}

func TestRuntimeCandidateCorrectionInputDirectlyProvesDocumentShape(t *testing.T) {
	t.Parallel()
	manifest := candidateCorrectionTestManifest()
	input := RuntimeCandidateCorrectionInput{
		ManifestHash: manifest.Hash,
		Objectives:   []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionConformExpectedDocument},
		Witness: runtimecontract.RuntimeCandidateCorrectionWitness{
			Dimension: runtimecontract.RuntimeCandidateCorrectionWitnessDocumentShape, Path: "$",
			RejectedValue: runtimecontract.RuntimeCandidateDocumentShapeInvalid, AlternativeValue: runtimecontract.RuntimeCandidateDocumentShapeConformant,
		},
	}.Normalized()
	observation := runtimecontract.RuntimeCandidateCorrectionObservation{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_document_nonconformant", Signature: "candidate-document-nonconformant",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$", Actual: runtimecontract.RuntimeCandidateDocumentShapeInvalid, ManifestHash: manifest.Hash,
			Objectives: append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), input.Objectives...),
		},
		SealedCorrectionHash: runtimecontract.StableRuntimeCandidateSealedCorrectionHash(input.ManifestHash, input.Objectives, &input.Witness),
	}.Normalized()
	if err := input.ValidateAgainstObservation(manifest, observation); err != nil {
		t.Fatal(err)
	}
	observation.Diagnostic.Actual = runtimecontract.RuntimeCandidateDocumentShapeEmpty
	if err := input.ValidateAgainstObservation(manifest, observation); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("document witness mismatch error = %v", err)
	}
}

func TestRuntimeCandidateCorrectionInputUsesSealedHashForPrivateReplayFrontier(t *testing.T) {
	t.Parallel()
	manifest := candidateCorrectionTestManifest()
	input := RuntimeCandidateCorrectionInput{
		ManifestHash: manifest.Hash,
		Objectives:   []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionChangeReplayedShape},
		Witness: runtimecontract.RuntimeCandidateCorrectionWitness{
			Dimension: runtimecontract.RuntimeCandidateCorrectionWitnessTarget, OperationKind: runtimecontract.RuntimeBlockKindPatchApply,
			Path: "$.operations[0].targetRefs", RejectedValue: "main.go", AlternativeValue: "support.go",
			RejectedShapeHash: "rejected-shape", AlternativeShapeHash: "alternative-shape",
		},
	}.Normalized()
	observation := runtimecontract.RuntimeCandidateCorrectionObservation{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_replays_rejected_shape", Signature: "candidate-replay",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			ManifestHash: manifest.Hash, RejectedShapeHash: input.Witness.RejectedShapeHash,
			Objectives: append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), input.Objectives...),
		},
		SealedCorrectionHash: runtimecontract.StableRuntimeCandidateSealedCorrectionHash(input.ManifestHash, input.Objectives, &input.Witness),
	}.Normalized()
	if err := input.ValidateAgainstObservation(manifest, observation); err != nil {
		t.Fatal(err)
	}
	wrongInput := input.Normalized()
	wrongInput.Objectives = []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSelectOperationKind}
	wrongObservation := observation.Clone()
	wrongObservation.Diagnostic.Objectives = append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), wrongInput.Objectives...)
	wrongObservation.SealedCorrectionHash = runtimecontract.StableRuntimeCandidateSealedCorrectionHash(wrongInput.ManifestHash, wrongInput.Objectives, &wrongInput.Witness)
	if err := wrongInput.ValidateAgainstObservation(manifest, wrongObservation); err == nil || !strings.Contains(err.Error(), "incompatible with witness dimension") {
		t.Fatalf("same wrong objective/witness mapping error = %v", err)
	}
	input.Witness.AlternativeValue = "another.go"
	input.Witness.AlternativeShapeHash = "another-shape"
	if err := input.ValidateAgainstObservation(manifest, observation); err == nil {
		t.Fatal("replay witness outside the sealed private frontier was accepted")
	}
}

func TestRuntimeCandidateCorrectionInputRejectsSemanticWitnessForMissingRequiredFieldObservation(t *testing.T) {
	t.Parallel()
	manifest := candidateCorrectionTestManifest()
	input := RuntimeCandidateCorrectionInput{
		ManifestHash: manifest.Hash,
		Objectives:   []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField},
		Witness: runtimecontract.RuntimeCandidateCorrectionWitness{
			Dimension: runtimecontract.RuntimeCandidateCorrectionWitnessTarget, OperationKind: runtimecontract.RuntimeBlockKindPatchApply,
			Path: "$.operations[0].targetRefs", RejectedValue: "", AlternativeValue: "README.md",
			RejectedShapeHash: "rejected-shape", AlternativeShapeHash: "alternative-shape",
		},
	}.Normalized()
	observation := runtimecontract.RuntimeCandidateCorrectionObservation{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations[0].targetRefs", Actual: "missing_targetRefs", Required: []string{"targetRefs"},
			ManifestHash: manifest.Hash, Objectives: append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), input.Objectives...),
		},
		SealedCorrectionHash: runtimecontract.StableRuntimeCandidateSealedCorrectionHash(input.ManifestHash, input.Objectives, &input.Witness),
	}.Normalized()
	if err := input.ValidateAgainstObservation(manifest, observation); err == nil || !strings.Contains(err.Error(), "schema-only authority") {
		t.Fatalf("missing-field semantic witness error = %v", err)
	}
}
