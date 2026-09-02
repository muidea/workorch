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

func candidateCorrectionWireRuleIdentity(t testing.TB, manifest runtimecontract.RuntimeCandidateContractManifest, kind string, operationIndex int) *runtimecontract.RuntimeCandidateWireRuleIdentity {
	t.Helper()
	wire, err := runtimecontract.BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		t.Fatal(err)
	}
	rule, found := runtimecontract.RuntimeCandidateWireRuleForAuxiliaryKind(wire, kind)
	if !found {
		t.Fatalf("candidate correction fixture has no wire rule for %q", kind)
	}
	identity, err := runtimecontract.RuntimeCandidateWireRuleIdentityForOperation(wire, rule, operationIndex)
	if err != nil {
		t.Fatal(err)
	}
	return identity
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

func TestRuntimeCandidateCorrectionInputAcceptsExactNullRequiredContainer(t *testing.T) {
	t.Parallel()
	manifest, _, _ := candidateParameterEntryCorrectionPair(t)
	wireRule := candidateCorrectionWireRuleIdentity(t, manifest, runtimecontract.RuntimeBlockKindPatchApply, 0)
	input := RuntimeCandidateCorrectionInput{
		ManifestHash: manifest.Hash,
		Objectives:   []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField},
		Witness: runtimecontract.RuntimeCandidateCorrectionWitness{
			Dimension: runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField,
			Path:      "$.operations[0].parameters", RejectedValue: "null", AlternativeValue: runtimecontract.RuntimeCandidateCorrectionFieldPresent,
		},
	}.Normalized()
	observation := runtimecontract.RuntimeCandidateCorrectionObservation{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: input.Witness.Path, Actual: "null", Allowed: []string{"array"}, Required: []string{"parameters"},
			WireRule: wireRule, ManifestHash: manifest.Hash, Objectives: append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), input.Objectives...),
		},
		SealedCorrectionHash: runtimecontract.StableRuntimeCandidateSealedCorrectionHash(input.ManifestHash, input.Objectives, &input.Witness),
	}.Normalized()
	if err := input.ValidateAgainstObservation(manifest, observation); err != nil {
		t.Fatalf("exact null-container correction was rejected: %v", err)
	}

	for name, mutate := range map[string]func(*RuntimeCandidateCorrectionInput, *runtimecontract.RuntimeCandidateCorrectionObservation){
		"actual drift": func(_ *RuntimeCandidateCorrectionInput, observation *runtimecontract.RuntimeCandidateCorrectionObservation) {
			observation.Diagnostic.Actual = "object"
		},
		"type rule removed": func(_ *RuntimeCandidateCorrectionInput, observation *runtimecontract.RuntimeCandidateCorrectionObservation) {
			observation.Diagnostic.Allowed = []string{"business-value"}
		},
		"witness drift": func(input *RuntimeCandidateCorrectionInput, _ *runtimecontract.RuntimeCandidateCorrectionObservation) {
			input.Witness.RejectedValue = runtimecontract.RuntimeCandidateCorrectionFieldAbsent
		},
	} {
		t.Run(name, func(t *testing.T) {
			changedInput := input.Normalized()
			changedObservation := observation.Clone()
			mutate(&changedInput, &changedObservation)
			if err := changedInput.ValidateAgainstObservation(manifest, changedObservation); err == nil {
				t.Fatal("drifted null-container correction crossed the LLM owner fence")
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
	wireRule := candidateCorrectionWireRuleIdentity(t, manifest, runtimecontract.RuntimeBlockKindPatchApply, 0)
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
			WireRule:                wireRule,
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

func TestRuntimeCandidateManifestCorrectionProofUsesExactWireRuleAcrossOperationKinds(t *testing.T) {
	operationA := runtimecontract.RuntimeCandidateOperationManifest{
		Kind: "kind/a", AllowedOperationFields: []string{"kind", "parameters", "targetRefs"},
		RequiredOperationFields: []string{"kind", "parameters", "targetRefs"}, RequireTargetRefs: true,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"a", "b"},
		},
		Parameters: []runtimecontract.RuntimeOperationParameterContract{
			{Name: "requiredValue", Type: runtimecontract.RuntimeOperationParameterTypeString, Required: true},
			{Name: "optionalValue", Type: runtimecontract.RuntimeOperationParameterTypeString, Required: false},
		},
	}
	operationB := runtimecontract.RuntimeCandidateOperationManifest{
		Kind: "kind/b", AllowedOperationFields: []string{"kind", "parameters", "targetRefs", "optionalEnvelope"},
		RequiredOperationFields: []string{"kind", "parameters"},
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"a", "b"},
		},
		Parameters: []runtimecontract.RuntimeOperationParameterContract{
			{Name: "requiredValue", Type: runtimecontract.RuntimeOperationParameterTypeString, Required: true},
			{Name: "optionalValue", Type: runtimecontract.RuntimeOperationParameterTypeString, Required: false},
		},
	}
	manifest := runtimecontract.RuntimeCandidateContractManifest{
		Revision: runtimecontract.RuntimeCandidateContractManifestRevision, State: runtimecontract.ConvergenceStateNeedStateChange,
		MaxOperationCount: 2, Operations: []runtimecontract.RuntimeCandidateOperationManifest{operationA, operationB},
	}
	manifest = manifest.Normalized()
	manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)
	diagnostic := func(kind, path string) *runtimecontract.RuntimeCandidateFailureDiagnostic {
		return &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: path, ManifestHash: manifest.Hash, WireRule: candidateCorrectionWireRuleIdentity(t, manifest, kind, 0),
		}
	}
	if runtimeCandidateManifestForbidsCorrectionField(runtimecontract.RuntimeCandidateContractManifest{}, nil, "$.operations[0].id", "id") {
		t.Fatal("manifest without an operation authorization proved a forbidden operation field")
	}
	if !runtimeCandidateManifestRequiresCorrectionField(manifest, diagnostic("kind/a", "$.operations[0].targetRefs"), "$.operations[0].targetRefs", "targetRefs") {
		t.Fatal("selected kind/a target requirement lost correction authority")
	}
	spoofedActual := diagnostic("kind/a", "$.operations[0].targetRefs")
	spoofedActual.Actual = "kind/b"
	if !runtimeCandidateManifestRequiresCorrectionField(manifest, spoofedActual, "$.operations[0].targetRefs", "targetRefs") {
		t.Fatal("diagnostic actual text overrode the exact kind/a wire rule")
	}
	if runtimeCandidateManifestRequiresCorrectionField(manifest, diagnostic("kind/b", "$.operations[0].targetRefs"), "$.operations[0].targetRefs", "targetRefs") {
		t.Fatal("kind/a target requirement crossed the exact kind/b wire rule")
	}
	if !runtimeCandidateManifestRequiresCorrectionField(manifest, diagnostic("kind/a", "$.operations[0].parameters[name=requiredValue]"), "$.operations[0].parameters[name=requiredValue]", "requiredValue") {
		t.Fatal("selected operation required parameter lost correction authority")
	}
	if runtimeCandidateManifestRequiresCorrectionField(manifest, diagnostic("kind/a", "$.operations[0].parameters[name=optionalValue]"), "$.operations[0].parameters[name=optionalValue]", "optionalValue") {
		t.Fatal("declared optional parameter was accepted as required")
	}
	if runtimeCandidateManifestForbidsCorrectionField(manifest, diagnostic("kind/a", "$.operations[0].parameters[name=optionalValue]"), "$.operations[0].parameters[name=optionalValue]", "optionalValue") {
		t.Fatal("declared optional parameter was accepted as forbidden")
	}
	if runtimeCandidateManifestForbidsCorrectionField(manifest, diagnostic("kind/b", "$.operations[0].targetRefs"), "$.operations[0].targetRefs", "targetRefs") {
		t.Fatal("selected kind/b provider field was treated as forbidden")
	}
	if !runtimeCandidateManifestForbidsCorrectionField(manifest, diagnostic("kind/b", "$.operations[0].optionalEnvelope"), "$.operations[0].optionalEnvelope", "optionalEnvelope") {
		t.Fatal("kind-level field outside the selected wire rule gained correction authority")
	}
	if !runtimeCandidateManifestForbidsCorrectionField(manifest, diagnostic("kind/a", "$.operations[0].parameters[name=unknownValue]"), "$.operations[0].parameters[name=unknownValue]", "unknownValue") {
		t.Fatal("undeclared parameter in selected closed operation kind was not forbidden")
	}
	input := RuntimeCandidateCorrectionInput{
		ManifestHash: manifest.Hash,
		Objectives:   []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField},
		Witness: runtimecontract.RuntimeCandidateCorrectionWitness{
			Dimension: runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField, Path: "$.operations[0].targetRefs",
			RejectedValue: runtimecontract.RuntimeCandidateCorrectionFieldAbsent, AlternativeValue: runtimecontract.RuntimeCandidateCorrectionFieldPresent,
		},
	}.Normalized()
	observation := runtimecontract.RuntimeCandidateCorrectionObservation{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		Diagnostic: diagnostic("kind/a", input.Witness.Path),
	}.Normalized()
	observation.Diagnostic.Actual = "missing_targetRefs"
	observation.Diagnostic.Required = []string{"targetRefs"}
	observation.Diagnostic.Objectives = append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), input.Objectives...)
	observation.SealedCorrectionHash = runtimecontract.StableRuntimeCandidateSealedCorrectionHash(input.ManifestHash, input.Objectives, &input.Witness)
	if err := input.ValidateAgainstObservation(manifest, observation); err != nil {
		t.Fatalf("exact kind/a C4 correction was rejected: %v", err)
	}
	driftedObservation := observation.Clone()
	driftedObservation.Diagnostic.WireRule = candidateCorrectionWireRuleIdentity(t, manifest, "kind/b", 0)
	if err := input.ValidateAgainstObservation(manifest, driftedObservation); err == nil {
		t.Fatal("kind/a required-field correction crossed a valid but unrelated kind/b wire rule")
	}
	derivedTargetManifest := manifest.Clone()
	derivedTargetManifest.Operations[0].TargetCardinality = runtimecontract.RuntimeOperationTargetCardinalityOne
	derivedTargetManifest.Operations[0].Target.ExactTargetRefs = []string{"only"}
	derivedTargetManifest = derivedTargetManifest.Normalized()
	derivedTargetManifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(derivedTargetManifest)
	derivedTargetDiagnostic := &runtimecontract.RuntimeCandidateFailureDiagnostic{
		Path: "$.operations[0].targetRefs", ManifestHash: derivedTargetManifest.Hash,
		WireRule: candidateCorrectionWireRuleIdentity(t, derivedTargetManifest, "kind/a", 0),
	}
	if runtimeCandidateManifestRequiresCorrectionField(derivedTargetManifest, derivedTargetDiagnostic, derivedTargetDiagnostic.Path, "targetRefs") {
		t.Fatal("locally derived singleton target was exposed as a provider-required field")
	}
	if !runtimeCandidateManifestForbidsCorrectionField(derivedTargetManifest, derivedTargetDiagnostic, derivedTargetDiagnostic.Path, "targetRefs") {
		t.Fatal("locally derived singleton target was not fenced from provider correction")
	}
	manifest.Operations[1].AllowsDynamicParameters = true
	manifest.Operations[1].DynamicParameterPrefixes = []string{"dynamic."}
	manifest = manifest.Normalized()
	manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)
	if runtimeCandidateManifestForbidsCorrectionField(manifest, diagnostic("kind/b", "$.operations[0].parameters[name=dynamic.value]"), "$.operations[0].parameters[name=dynamic.value]", "dynamic.value") {
		t.Fatal("parameter authorized dynamically by the selected kind was treated as forbidden")
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

func TestRuntimeCandidateCorrectionInputValidatesOwnerConstraintAgainstManifest(t *testing.T) {
	contract, found := runtimecontract.RuntimeOperationContractForKind(runtimecontract.RuntimeBlockKindPatchApply)
	if !found {
		t.Fatal("patch/apply contract is missing")
	}
	preview, found := runtimecontract.RuntimeOperationContractForKind(runtimecontract.RuntimeBlockKindPatchPreview)
	if !found {
		t.Fatal("patch/preview contract is missing")
	}
	manifest := runtimecontract.RuntimeCandidateContractManifest{
		Revision:                  runtimecontract.RuntimeCandidateContractManifestRevision,
		State:                     runtimecontract.ConvergenceStateNeedStateChange,
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		Operations: []runtimecontract.RuntimeCandidateOperationManifest{
			{
				Kind: contract.Kind, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				Parameters: contract.Parameters, Variants: contract.Variants,
				TargetCardinality:         runtimecontract.RuntimeOperationTargetCardinalityOne,
				OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
			},
			{
				Kind: preview.Kind, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				Parameters: preview.Parameters, Variants: preview.Variants,
				TargetCardinality:         runtimecontract.RuntimeOperationTargetCardinalityOne,
				OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
			},
		},
	}.Normalized()
	manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)
	input := RuntimeCandidateCorrectionInput{
		ManifestHash: manifest.Hash,
		Objectives:   []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionConformOwnerConstraint},
		Witness: runtimecontract.RuntimeCandidateCorrectionWitness{
			Dimension:        runtimecontract.RuntimeCandidateCorrectionWitnessOwnerConstraint,
			OperationKind:    runtimecontract.RuntimeBlockKindPatchApply,
			Path:             "$.operations[0].parameters[name=diff]",
			ConstraintRef:    "patch/unified_diff/diff/single_file",
			RejectedValue:    string(runtimecontract.RuntimeOperationCandidateActualShapeInvalidSyntax),
			AlternativeValue: string(runtimecontract.RuntimeOperationCandidateRequiredShapeUnifiedDiffSingleFile),
		},
	}.Normalized()
	observation := runtimecontract.RuntimeCandidateCorrectionObservation{
		Class: runtimecontract.RuntimeCandidateFailureClassContract,
		Code:  "candidate_owner_constraint_nonconformant", Signature: "candidate-owner-constraint-nonconformant",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			OperationID: "patch-1", Path: input.Witness.Path, Actual: input.Witness.RejectedValue,
			ConstraintRef: input.Witness.ConstraintRef, RequiredShape: input.Witness.AlternativeValue,
			ManifestHash: manifest.Hash, Objectives: append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), input.Objectives...),
		},
		SealedCorrectionHash: runtimecontract.StableRuntimeCandidateSealedCorrectionHash(manifest.Hash, input.Objectives, &input.Witness),
	}.Normalized()
	if err := input.ValidateAgainstObservation(manifest, observation); err != nil {
		t.Fatalf("owner constraint correction: %v", err)
	}

	drifted := manifest.Clone()
	for operationIndex := range drifted.Operations {
		for variantIndex := range drifted.Operations[operationIndex].Variants {
			drifted.Operations[operationIndex].Variants[variantIndex].CandidateConstraints = nil
		}
	}
	drifted.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(drifted)
	driftedInput := input.Normalized()
	driftedInput.ManifestHash = drifted.Hash
	driftedObservation := observation.Clone()
	driftedObservation.Diagnostic.ManifestHash = drifted.Hash
	driftedObservation.SealedCorrectionHash = runtimecontract.StableRuntimeCandidateSealedCorrectionHash(drifted.Hash, driftedInput.Objectives, &driftedInput.Witness)
	if err := driftedInput.ValidateAgainstObservation(drifted, driftedObservation); err == nil {
		t.Fatal("owner constraint correction survived removal of its manifest constraint")
	}
}

func TestRuntimeCandidateCorrectionInputRejectsSemanticWitnessForMissingRequiredFieldObservation(t *testing.T) {
	t.Parallel()
	manifest, _, _ := candidateParameterEntryCorrectionPair(t)
	wireRule := candidateCorrectionWireRuleIdentity(t, manifest, runtimecontract.RuntimeBlockKindPatchApply, 0)
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
			WireRule: wireRule, ManifestHash: manifest.Hash, Objectives: append([]runtimecontract.RuntimeCandidateCorrectionObjective(nil), input.Objectives...),
		},
		SealedCorrectionHash: runtimecontract.StableRuntimeCandidateSealedCorrectionHash(input.ManifestHash, input.Objectives, &input.Witness),
	}.Normalized()
	if err := input.ValidateAgainstObservation(manifest, observation); err == nil || !strings.Contains(err.Error(), "schema-only authority") {
		t.Fatalf("missing-field semantic witness error = %v", err)
	}
}
