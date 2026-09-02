package runtime

import (
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCandidateRejectionCodeDispositionMatrixIsTotal(t *testing.T) {
	t.Parallel()
	codes := []struct {
		code         string
		disposition  runtimecontract.RuntimeCandidateCorrectionDisposition
		terminalCode string
	}{
		{code: "candidate_contract_invalid", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_schema_invalid", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_replays_rejected_shape", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_partially_replays_rejected_shape", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_replays_no_progress_shape", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_action_evidence_mismatch", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_target_outside_grounding", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_creation_scope_missing_file_target", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_owner_constraint_nonconformant", disposition: runtimecontract.RuntimeCandidateCorrectionCorrectableOnce},
		{code: "candidate_replays_rejected_candidate_shape", disposition: runtimecontract.RuntimeCandidateCorrectionRejectedFinal, terminalCode: "candidate_retry_exhausted"},
		{code: "candidate_authority_exhausted", disposition: runtimecontract.RuntimeCandidateCorrectionRejectedFinal, terminalCode: "candidate_authority_exhausted"},
		{code: "candidate_retry_exhausted", disposition: runtimecontract.RuntimeCandidateCorrectionRejectedFinal, terminalCode: "candidate_retry_exhausted"},
		{code: "workspace_resolution_failed", disposition: runtimecontract.RuntimeCandidateCorrectionRejectedFinal, terminalCode: "workspace_resolution_failed"},
		{code: "operation_candidate_conformance_probe_invalid", disposition: runtimecontract.RuntimeCandidateCorrectionRejectedFinal, terminalCode: "operation_candidate_conformance_probe_invalid"},
		{code: "operation_feasibility_probe_invalid", disposition: runtimecontract.RuntimeCandidateCorrectionRejectedFinal, terminalCode: "operation_feasibility_probe_invalid"},
		{code: "runtime_contract_defect", disposition: runtimecontract.RuntimeCandidateCorrectionRejectedFinal, terminalCode: "runtime_contract_defect"},
	}
	for _, testCase := range codes {
		disposition, ok := CandidateRejectionCodeDisposition(testCase.code)
		if !ok || disposition != testCase.disposition {
			t.Fatalf("code %q disposition = %q ok=%t, want %q", testCase.code, disposition, ok, testCase.disposition)
		}
		if got := CandidateRejectionTerminalCode(testCase.code); got != testCase.terminalCode {
			t.Fatalf("code %q terminal = %q, want %q", testCase.code, got, testCase.terminalCode)
		}
	}
	if _, ok := CandidateRejectionCodeDisposition("candidate_correction_undefined"); ok {
		t.Fatal("candidate_correction_undefined must not remain an ordinary rejection code")
	}
	if _, ok := CandidateRejectionCodeDisposition("operation_target_not_regular"); ok {
		t.Fatal("owner feasibility code must not enter the candidate correction matrix")
	}
}

func TestCandidateCorrectionObjectivesRequireAllowedOperationKind(t *testing.T) {
	t.Parallel()
	failure := runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		Reason: "provider-private decoder detail",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations[0].kind", Actual: "shell/command", Allowed: []string{"file/write", "patch/apply"},
		},
	}
	objectives := CandidateCorrectionObjectivesForFailure(&failure)
	if len(objectives) != 1 || objectives[0] != runtimecontract.RuntimeCandidateCorrectionSelectOperationKind {
		t.Fatalf("objectives = %+v, want select allowed operation kind", objectives)
	}
}

func TestCandidateCorrectionObjectivesRequireAllowedEvidenceBinding(t *testing.T) {
	t.Parallel()
	failure := runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		Reason: "provider selected a binding outside the sealed manifest",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations[0].bindingRefs", Actual: "binding/wrong", Allowed: []string{"binding/allowed"},
		},
	}
	objectives := CandidateCorrectionObjectivesForFailure(&failure)
	if !reflect.DeepEqual(objectives, []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSelectEvidenceBinding}) {
		t.Fatalf("objectives = %+v, want select allowed evidence binding", objectives)
	}
}

func TestCandidateCorrectionObjectivesKeepMissingFieldsSchemaOnly(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name, path, actual, required string
	}{
		{name: "target", path: "$.operations[0].targetRefs", actual: "missing_targetRefs", required: "targetRefs"},
		{name: "kind", path: "$.operations[0].kind", actual: "missing_kind", required: "kind"},
		{name: "binding", path: "$.operations[0].bindingRefs", actual: "empty_bindingRefs", required: "bindingRefs"},
		{name: "parameter", path: "$.operations[0].parameters[name=content]", actual: "missing_content", required: "content"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			failure := runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
				Reason: "required field is missing",
				Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
					Path: testCase.path, Actual: testCase.actual,
					Allowed: []string{"bindingRefs", "kind", "parameters", "targetRefs"}, Required: []string{testCase.required},
				},
			}
			objectives := CandidateCorrectionObjectivesForFailure(&failure)
			want := []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField}
			if !reflect.DeepEqual(objectives, want) {
				t.Fatalf("missing field objectives = %+v, want %+v", objectives, want)
			}
		})
	}
}

func TestCandidateCorrectionObjectivesDoNotTreatDocumentRequirementAsMissingField(t *testing.T) {
	t.Parallel()
	failure := runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_document_nonconformant", Signature: "candidate-document-nonconformant",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$", Actual: runtimecontract.RuntimeCandidateDocumentShapeNonObjectJSONValue,
			Required: []string{runtimecontract.RuntimeCandidateDocumentShapeConformant},
		},
	}
	if objectives := CandidateCorrectionObjectivesForFailure(&failure); !reflect.DeepEqual(objectives, []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionConformExpectedDocument}) {
		t.Fatalf("document requirement was projected as a missing field: %+v", objectives)
	}
}

func TestCandidateCorrectionObjectivesRemoveForbiddenProviderIdentityOnly(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"id", "dependsOn"} {
		t.Run(field, func(t *testing.T) {
			failure := runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
				Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
					Path: "$.operations[0]." + field, Actual: field,
					Allowed:    []string{"kind", "bindingRefs", "targetRefs", "parameters"},
					Forbidden:  []string{field},
					Required:   []string{"dependsOn"},
					Objectives: []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSatisfyDependency},
				},
			}
			objectives := CandidateCorrectionObjectivesForFailure(&failure)
			want := []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionRemoveForbiddenField}
			if !reflect.DeepEqual(objectives, want) {
				t.Fatalf("provider identity %s objectives = %+v, want %+v", field, objectives, want)
			}
		})
	}
}

func TestSealCandidateRejectionCorrectableRequiresObjectives(t *testing.T) {
	aggregate, candidate := aggregateWithActiveRejectedCandidate(t)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Operations[0].Target.ExactTargetRefs = append(manifest.Operations[0].Target.ExactTargetRefs, "support.go")
	manifest.Hash = ""
	manifest = manifest.Normalized()
	manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)
	candidate = runtimeTestMustDeriveCandidateForManifest(t, manifest, candidate)
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: candidateAdmissionCodeReplaysRejectedShape, Signature: "candidate-replay",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations[0].targetRefs", Actual: "main.go", Allowed: []string{"main.go", "support.go"}, ManifestHash: manifest.Hash,
			RejectedShapeHash: runtimecontract.StableRuntimeCandidateSemanticShape(candidate),
		},
	}
	sealed, directive, err := sealCandidateRejectionWithFrontier(failure, manifest, candidateCorrectionFrontier{
		candidate: &candidate, rejectedOperationShapes: ActiveRejectedOperationShapes(aggregate),
	})
	if err != nil || sealed == nil || directive == nil {
		t.Fatalf("seal correctable = sealed=%+v directive=%+v err=%v", sealed, directive, err)
	}
	if directive.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || directive.TerminalCode != "" ||
		directive.Witness == nil || directive.Witness.Dimension != runtimecontract.RuntimeCandidateCorrectionWitnessTarget || directive.Witness.AlternativeValue != "support.go" ||
		!reflect.DeepEqual(directive.Objectives, []runtimecontract.RuntimeCandidateCorrectionObjective{
			runtimecontract.RuntimeCandidateCorrectionChangeReplayedShape,
			runtimecontract.RuntimeCandidateCorrectionReplaceTarget,
		}) {
		t.Fatalf("correctable directive = %+v", directive)
	}
	if CandidateFailureIsRejectedFinal(sealed) {
		t.Fatalf("correctable classification drifted: %+v", sealed)
	}
}

func TestSealCandidateRejectionWithoutObjectiveBecomesContractDefect(t *testing.T) {
	t.Parallel()
	manifest := runtimecontract.RuntimeCandidateContractManifest{Hash: "manifest-hash"}
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_target_outside_grounding", Signature: "outside",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{Path: "$.operations[0].targetRefs", Actual: "other.go"},
	}
	sealed, directive, err := SealCandidateRejection(failure, manifest)
	if err != nil || sealed == nil || directive == nil {
		t.Fatalf("seal incomplete correctable = sealed=%+v directive=%+v err=%v", sealed, directive, err)
	}
	if sealed.Code != "runtime_contract_defect" || directive.Disposition != runtimecontract.RuntimeCandidateCorrectionRejectedFinal ||
		directive.TerminalCode != "runtime_contract_defect" || len(directive.Objectives) != 0 {
		t.Fatalf("incomplete correctable must become runtime_contract_defect: sealed=%+v directive=%+v", sealed, directive)
	}
	if !CandidateFailureIsRejectedFinal(sealed) {
		t.Fatalf("defect classification drifted: %+v", sealed)
	}
}

func TestSealCandidateRejectionRequiresConcreteWitnessAndPreservesRequiredFieldCorrection(t *testing.T) {
	t.Parallel()

	manifest := runtimecontract.RuntimeCandidateContractManifest{
		Hash: "manifest-hash", RequiredTopLevelFields: []string{"operations"}, AllowedTopLevelFields: []string{"operations"},
	}
	replay := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_replays_rejected_shape", Signature: "replay", Reason: "same semantic operation",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			ManifestHash: "manifest-hash", RejectedShapeHash: "shape-1",
			Objectives: []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionChangeReplayedShape},
		},
	}
	_, exhausted, err := SealCandidateRejection(replay, manifest)
	if err != nil || exhausted == nil || exhausted.Disposition != runtimecontract.RuntimeCandidateCorrectionRejectedFinal || exhausted.TerminalCode != "candidate_authority_exhausted" || exhausted.Witness != nil {
		t.Fatalf("objective without witness must exhaust candidate authority: directive=%+v err=%v", exhausted, err)
	}

	missing := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "schema", Reason: "operations is missing",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations", Actual: "missing_operations", Required: []string{"operations"}, Allowed: []string{"operations"}, ManifestHash: "manifest-hash",
		},
	}
	_, correctable, err := SealCandidateRejection(missing, manifest)
	if err != nil || correctable == nil || correctable.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || correctable.Witness == nil ||
		correctable.Witness.Dimension != runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField || correctable.Witness.AlternativeValue != candidateCorrectionFieldPresent {
		t.Fatalf("manifest-required schema field lost correction witness: directive=%+v err=%v", correctable, err)
	}

	aggregate, _ := aggregateWithActiveRejectedCandidate(t)
	manifest, err = runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := runtimecontract.BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		t.Fatal(err)
	}
	rule, found := runtimecontract.RuntimeCandidateWireRuleForAuxiliaryKind(wire, runtimecontract.RuntimeBlockKindPatchApply)
	if !found {
		t.Fatal("required-field fixture has no patch/apply wire rule")
	}
	wireRule, err := runtimecontract.RuntimeCandidateWireRuleIdentityForOperation(wire, rule, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		path     string
		actual   string
		required string
	}{
		{path: "$.operations[0].targetRefs", actual: "missing_targetRefs", required: "targetRefs"},
		{path: "$.operations[0].bindingRefs", actual: "missing_bindingRefs", required: "bindingRefs"},
		{path: "$.operations[0].parameters[name=content]", actual: "missing_content", required: "content"},
	} {
		failure := &runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "schema", Reason: "required candidate field is missing",
			Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
				Path: testCase.path, Actual: testCase.actual, Required: []string{testCase.required}, WireRule: wireRule.Clone(), ManifestHash: manifest.Hash,
			},
		}
		sealed, directive, err := SealCandidateRejection(failure, manifest)
		if err != nil || directive == nil || directive.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || directive.Witness == nil ||
			directive.Witness.Path != testCase.path || directive.Witness.RejectedValue != candidateCorrectionFieldAbsent || directive.Witness.AlternativeValue != candidateCorrectionFieldPresent ||
			!reflect.DeepEqual(directive.Objectives, []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField}) ||
			runtimecontract.ValidateRuntimeCandidateFailureCorrectionPair(sealed, directive) != nil {
			t.Fatalf("required field %s lost exact correction witness: directive=%+v err=%v", testCase.path, directive, err)
		}
	}
}

func TestSealCandidateRejectionUsesTypedDocumentAndRawRemovalWitnesses(t *testing.T) {
	documentManifest := runtimecontract.RuntimeCandidateContractManifest{Hash: "manifest-hash"}
	documentFailure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_document_nonconformant", Signature: "document-shape", Reason: "one JSON object is required",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$", Actual: runtimecontract.RuntimeCandidateDocumentShapeNonObjectJSONValue, ManifestHash: documentManifest.Hash,
			Objectives: []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionConformExpectedDocument},
		},
	}
	_, directive, err := SealCandidateRejection(documentFailure, documentManifest)
	if err != nil || directive == nil || directive.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || directive.Witness == nil ||
		directive.Witness.Dimension != runtimecontract.RuntimeCandidateCorrectionWitnessDocumentShape || directive.Witness.RejectedValue != runtimecontract.RuntimeCandidateDocumentShapeNonObjectJSONValue {
		t.Fatalf("document-shape directive=%+v err=%v", directive, err)
	}

	aggregate, _ := aggregateWithActiveRejectedCandidate(t)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	wireRule := runtimeTestCandidateAuxiliaryWireRuleIdentity(t, manifest, runtimecontract.RuntimeBlockKindPatchApply, 0)

	for _, field := range []string{"id", "dependsOn"} {
		failure := &runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "raw-field", Reason: "raw canonical field is forbidden",
			Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
				Path: "$.operations[0]." + field, Actual: field, Forbidden: []string{field},
				WireRule: wireRule.Clone(), ManifestHash: manifest.Hash,
			},
		}
		_, correction, err := SealCandidateRejection(failure, manifest)
		if err != nil || correction == nil || correction.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || correction.Witness == nil ||
			correction.Witness.Dimension != runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField || correction.Witness.AlternativeValue != candidateCorrectionFieldAbsent {
			t.Fatalf("raw %s removal directive=%+v err=%v", field, correction, err)
		}
	}
}

func TestSealCandidateRejectionUnknownCodeBecomesContractDefect(t *testing.T) {
	t.Parallel()
	manifest := runtimecontract.RuntimeCandidateContractManifest{Hash: "manifest-hash"}
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_correction_undefined", Signature: "undefined",
		Reason: "legacy catch-all must not remain ordinary",
	}
	sealed, directive, err := SealCandidateRejection(failure, manifest)
	if err != nil || sealed == nil || directive == nil {
		t.Fatalf("seal unknown = sealed=%+v directive=%+v err=%v", sealed, directive, err)
	}
	if sealed.Code != "runtime_contract_defect" || directive.TerminalCode != "runtime_contract_defect" {
		t.Fatalf("unknown code must become runtime_contract_defect: sealed=%+v directive=%+v", sealed, directive)
	}
}

func TestSealCandidateRejectionRejectedFinalHasTerminal(t *testing.T) {
	t.Parallel()
	manifest := runtimecontract.RuntimeCandidateContractManifest{Hash: "manifest-hash"}
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_replays_rejected_candidate_shape", Signature: "replay-candidate",
	}
	sealed, directive, err := SealCandidateRejection(failure, manifest)
	if err != nil || sealed == nil || directive == nil {
		t.Fatalf("seal final = sealed=%+v directive=%+v err=%v", sealed, directive, err)
	}
	if directive.Disposition != runtimecontract.RuntimeCandidateCorrectionRejectedFinal || directive.TerminalCode != "candidate_retry_exhausted" ||
		len(directive.Objectives) != 0 {
		t.Fatalf("final directive = %+v sealed=%+v", directive, sealed)
	}
}
