package runtime

import (
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCandidateCorrectionFrontierExhaustsSingleTargetAndSealsConcreteAlternative(t *testing.T) {
	aggregate, candidate := aggregateWithActiveRejectedCandidate(t)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	candidate = runtimeTestMustDeriveCandidateForManifest(t, manifest, candidate)
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract,
		Code:  candidateAdmissionCodeReplaysRejectedShape, Signature: "replay", Reason: "replays rejected target shape",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			ManifestHash: manifest.Hash, RejectedShapeHash: runtimecontract.StableRuntimeCandidateSemanticShape(candidate),
			Objectives: []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionChangeReplayedShape},
		},
	}
	frontier := candidateCorrectionFrontier{candidate: &candidate, rejectedOperationShapes: ActiveRejectedOperationShapes(aggregate)}
	_, exhausted, err := sealCandidateRejectionWithFrontier(failure, manifest, frontier)
	if err != nil || exhausted == nil || exhausted.Disposition != runtimecontract.RuntimeCandidateCorrectionRejectedFinal || exhausted.TerminalCode != "candidate_authority_exhausted" {
		t.Fatalf("single-target frontier did not exhaust: directive=%+v err=%v", exhausted, err)
	}

	withAlternative := manifest.Clone()
	if len(withAlternative.Operations) != 1 {
		t.Fatalf("target frontier fixture operations=%+v", withAlternative.Operations)
	}
	withAlternative.Operations[0].Target.ExactTargetRefs = append(withAlternative.Operations[0].Target.ExactTargetRefs, "support.go")
	withAlternative.Hash = ""
	withAlternative = withAlternative.Normalized()
	withAlternative.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(withAlternative)
	candidate = runtimeTestMustDeriveCandidateForManifest(t, withAlternative, candidate)
	failure.Diagnostic.ManifestHash = withAlternative.Hash
	failure.Diagnostic.RejectedShapeHash = runtimecontract.StableRuntimeCandidateSemanticShape(candidate)
	frontier = candidateCorrectionFrontier{candidate: &candidate, rejectedOperationShapes: ActiveRejectedOperationShapes(aggregate)}
	_, correction, err := sealCandidateRejectionWithFrontier(failure, withAlternative, frontier)
	targetField, targetFieldFound, targetFieldErr := runtimecontract.RuntimeOperationCorrectionFieldForRole(withAlternative.Operations[0], runtimecontract.RuntimeOperationCorrectionRoleTarget)
	if err != nil || correction == nil || correction.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || correction.Witness == nil ||
		correction.Witness.Dimension != runtimecontract.RuntimeCandidateCorrectionWitnessTarget || correction.Witness.AlternativeValue != "support.go" ||
		targetFieldErr != nil || !targetFieldFound || correction.Witness.Path != targetField.CandidatePath(0) ||
		correction.Witness.RejectedShapeHash == correction.Witness.AlternativeShapeHash {
		t.Fatalf("finite target alternative did not seal concrete witness: directive=%+v err=%v", correction, err)
	}

	transaction := aggregate.Decision.AuthorityTransaction()
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction)
	proposal := candidate
	failureCopy := *failure
	// The fixture's completed dispatch retains its historical accepted attempt.
	// Start a clean current-authority attempt ledger for the restart fence under
	// test; rejected-shape lineage remains in completed evidence.
	aggregate.CandidateAttempts = nil
	aggregate.CandidateAttempts = append(aggregate.CandidateAttempts,
		runtimecontract.RuntimeCandidateAttemptRecord{
			AuthorityContractID: contractID, Transaction: transaction, Attempt: 0, StablePrefixHash: "frontier-prefix",
			ManifestHash: withAlternative.Hash, LogicalRequestID: "frontier-attempt-0", State: runtimecontract.RuntimeCandidateAttemptStateFailed,
			Proposal: &proposal, Failure: &failureCopy, Correction: correction.Clone(),
		},
		runtimecontract.RuntimeCandidateAttemptRecord{
			AuthorityContractID: contractID, Transaction: transaction, Attempt: 1, StablePrefixHash: "frontier-prefix",
			ManifestHash: withAlternative.Hash, LogicalRequestID: "frontier-attempt-1", State: runtimecontract.RuntimeCandidateAttemptStatePending,
		},
	)
	alternative := candidate
	alternative.StateChange = &runtimecontract.StateChangeCandidate{Operations: append([]runtimecontract.StateChangingOperation(nil), candidate.StateChange.Operations...)}
	alternative.StateChange.Operations[0].Operation.TargetRefs = []string{"support.go"}
	if err := validateAttemptCandidateCrossesPersistedWitness(aggregate, withAlternative, alternative); err != nil {
		t.Fatalf("sealed target alternative rejected on attempt 1: %v", err)
	}
	idOnly := candidate
	idOnly.StateChange = &runtimecontract.StateChangeCandidate{Operations: append([]runtimecontract.StateChangingOperation(nil), candidate.StateChange.Operations...)}
	idOnly.StateChange.Operations[0].Operation.ID = "provider-new-id"
	summary := "different display wording"
	idOnly.StateChange.Operations[0].Operation.Parameters = append(idOnly.StateChange.Operations[0].Operation.Parameters, runtimecontract.OperationParameter{Name: "summary", String: &summary})
	if err := validateAttemptCandidateCrossesPersistedWitness(aggregate, withAlternative, idOnly); err == nil {
		t.Fatal("operation ID/summary-only candidate crossed a target witness")
	}
	drifted := aggregate
	drifted.CandidateAttempts = append([]runtimecontract.RuntimeCandidateAttemptRecord(nil), aggregate.CandidateAttempts...)
	drifted.CandidateAttempts[0].Correction = aggregate.CandidateAttempts[0].Correction.Clone()
	drifted.CandidateAttempts[0].Correction.Witness.AlternativeValue = "another.go"
	if err := validatePersistedCandidateCorrection(drifted, withAlternative); err == nil {
		t.Fatal("restart accepted a persisted witness whose exact alternative drifted")
	}
	staleManifest := withAlternative.Clone()
	staleManifest.Operations[0].Target.ExactTargetRefs = []string{"main.go"}
	staleManifest.Hash = ""
	staleManifest = staleManifest.Normalized()
	staleManifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(staleManifest)
	if err := validatePersistedCandidateCorrection(aggregate, staleManifest); err == nil {
		t.Fatal("restart accepted a correction witness after its manifest alternative was removed")
	}
}

func TestCandidateCorrectionFrontierPrioritizesMissingRequiredFieldOverSemanticTarget(t *testing.T) {
	aggregate, candidate := aggregateWithActiveRejectedCandidate(t)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Operations) != 1 {
		t.Fatalf("target frontier fixture operations=%+v", manifest.Operations)
	}
	manifest.Operations[0].Target.ExactTargetRefs = append(manifest.Operations[0].Target.ExactTargetRefs, "README.md")
	manifest.Hash = ""
	manifest = manifest.Normalized()
	manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)
	candidate = runtimeTestMustDeriveCandidateForManifest(t, manifest, candidate)
	partial := candidate
	partial.StateChange = &runtimecontract.StateChangeCandidate{Operations: append([]runtimecontract.StateChangingOperation(nil), candidate.StateChange.Operations...)}
	partial.StateChange.Operations[0].Operation.TargetRefs = nil
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid", Reason: "required targetRefs field is missing",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations[0].targetRefs", Actual: "missing_targetRefs",
			Allowed:  []string{"bindingRefs", "kind", "parameters", "targetRefs"},
			Required: []string{"bindingRefs", "kind", "parameters", "targetRefs"}, ManifestHash: manifest.Hash,
		},
	}
	sealed, correction, err := sealCandidateRejectionWithFrontier(failure, manifest, candidateCorrectionFrontier{candidate: &partial})
	if err != nil || sealed == nil || correction == nil || correction.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || correction.Witness == nil {
		t.Fatalf("missing required target correction was not sealed: failure=%+v correction=%+v err=%v", sealed, correction, err)
	}
	if correction.Witness.Dimension != runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField ||
		correction.Witness.Path != failure.Diagnostic.Path || correction.Witness.RejectedValue != candidateCorrectionFieldAbsent ||
		correction.Witness.AlternativeValue != candidateCorrectionFieldPresent || correction.Witness.RejectedShapeHash != "" || correction.Witness.AlternativeShapeHash != "" {
		t.Fatalf("missing field was converted into a semantic witness: %+v", correction.Witness)
	}
	if !reflect.DeepEqual(correction.Objectives, []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField}) {
		t.Fatalf("strict field vocabulary was treated as semantic targets: %+v", correction.Objectives)
	}
	if err := runtimecontract.ValidateRuntimeCandidateFailureCorrectionPair(sealed, correction); err != nil {
		t.Fatalf("sealed missing-field correction does not survive persisted lineage validation: %v", err)
	}
	if !candidateCrossesCorrectionWitness(candidate, manifest, *correction.Witness) || candidateCrossesCorrectionWitness(partial, manifest, *correction.Witness) {
		t.Fatal("schema field presence witness did not distinguish corrected and missing targetRefs")
	}
}

func TestCandidateCorrectionFrontierSealsDeclaredParameterEntryReencode(t *testing.T) {
	aggregate, candidate := aggregateWithActiveRejectedCandidate(t)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	encodings := []runtimecontract.RuntimeCandidateParameterEntryEncodingViolation{{
		OperationIndex: 0, ParameterIndex: 1, OperationKind: runtimecontract.RuntimeBlockKindPatchApply, ContainerParameterName: "content",
		MisplacedParameters: []runtimecontract.RuntimeCandidateMisplacedParameter{
			{Name: "baseHash", Type: runtimecontract.RuntimeOperationParameterTypeString},
			{Name: "coverageTargets", Type: runtimecontract.RuntimeOperationParameterTypeStringList},
			{Name: "summary", Type: runtimecontract.RuntimeOperationParameterTypeString},
		},
	}}
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		Reason: "provider parameter entry has unknown sibling fields",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations[0].parameters[1].baseHash", Actual: "baseHash",
			Allowed:                 []string{"name", "string", "integer", "boolean", "stringList"},
			ViolationKind:           runtimecontract.RuntimeCandidatePayloadViolationParameterEntryEncoding,
			ParameterEntryEncodings: encodings, ManifestHash: manifest.Hash,
		},
	}
	sealed, correction, err := SealCandidateRejection(failure, manifest)
	if err != nil || sealed == nil || correction == nil || correction.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || correction.Witness == nil {
		t.Fatalf("parameter-entry rejection did not seal one correction: failure=%+v correction=%+v err=%v", sealed, correction, err)
	}
	if !reflect.DeepEqual(correction.Objectives, []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionReencodeParameterEntries}) ||
		correction.Witness.Dimension != runtimecontract.RuntimeCandidateCorrectionWitnessParameterEntryEncoding ||
		correction.Witness.Path != "$.operations" ||
		correction.Witness.RejectedValue != runtimecontract.RuntimeCandidateParameterEncodingInlineSiblings ||
		correction.Witness.AlternativeValue != runtimecontract.RuntimeCandidateParameterEncodingNamedEntries ||
		!reflect.DeepEqual(correction.Witness.ParameterEntryEncodings, encodings) {
		t.Fatalf("parameter-entry correction authority=%+v", correction)
	}
	if err := runtimecontract.ValidateRuntimeCandidateFailureCorrectionPair(sealed, correction); err != nil {
		t.Fatalf("parameter-entry failure/correction lineage rejected: %v", err)
	}

	coverageTargets := []string{"cwi-1"}
	summary := "bounded change summary"
	corrected := candidate
	corrected.StateChange = &runtimecontract.StateChangeCandidate{Operations: append([]runtimecontract.StateChangingOperation(nil), candidate.StateChange.Operations...)}
	corrected.StateChange.Operations[0].Operation.Parameters = append(
		append([]runtimecontract.OperationParameter(nil), candidate.StateChange.Operations[0].Operation.Parameters...),
		runtimecontract.OperationParameter{Name: "coverageTargets", StringList: coverageTargets},
		runtimecontract.OperationParameter{Name: "summary", String: &summary},
	)
	if !candidateCrossesCorrectionWitness(corrected, manifest, *correction.Witness) {
		t.Fatal("strictly decoded independent named entries did not cross parameter-entry witness")
	}
	missing := corrected
	missing.StateChange = &runtimecontract.StateChangeCandidate{Operations: append([]runtimecontract.StateChangingOperation(nil), corrected.StateChange.Operations...)}
	missing.StateChange.Operations[0].Operation.Parameters = append([]runtimecontract.OperationParameter(nil), corrected.StateChange.Operations[0].Operation.Parameters[:len(corrected.StateChange.Operations[0].Operation.Parameters)-1]...)
	if candidateCrossesCorrectionWitness(missing, manifest, *correction.Witness) {
		t.Fatal("dropping a misplaced parameter crossed re-encode witness")
	}
	wrongType := corrected
	wrongType.StateChange = &runtimecontract.StateChangeCandidate{Operations: append([]runtimecontract.StateChangingOperation(nil), corrected.StateChange.Operations...)}
	wrongType.StateChange.Operations[0].Operation.Parameters = append([]runtimecontract.OperationParameter(nil), corrected.StateChange.Operations[0].Operation.Parameters...)
	for index := range wrongType.StateChange.Operations[0].Operation.Parameters {
		if wrongType.StateChange.Operations[0].Operation.Parameters[index].Name == "coverageTargets" {
			value := "cwi-1"
			wrongType.StateChange.Operations[0].Operation.Parameters[index].StringList = nil
			wrongType.StateChange.Operations[0].Operation.Parameters[index].String = &value
		}
	}
	if candidateCrossesCorrectionWitness(wrongType, manifest, *correction.Witness) {
		t.Fatal("wrong typed independent entry crossed re-encode witness")
	}

	drifted := correction.Clone()
	drifted.Witness.ParameterEntryEncodings[0].MisplacedParameters[0].Type = runtimecontract.RuntimeOperationParameterTypeBoolean
	if err := runtimecontract.ValidateRuntimeCandidateFailureCorrectionPair(sealed, drifted); err == nil {
		t.Fatal("tampered parameter-entry witness survived persisted lineage validation")
	}
}

func TestCandidateSchemaFieldPresenceWitnessUsesFieldPresenceNotLiteralValue(t *testing.T) {
	aggregate, candidate := aggregateWithActiveRejectedCandidate(t)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	candidate = runtimeTestMustDeriveCandidateForManifest(t, manifest, candidate)
	operationManifest := manifest.Operations[0]
	targetField, targetFound, err := runtimecontract.RuntimeOperationCorrectionFieldForRole(operationManifest, runtimecontract.RuntimeOperationCorrectionRoleTarget)
	if err != nil || !targetFound {
		t.Fatalf("target correction field missing: field=%+v err=%v", targetField, err)
	}
	bindingField, bindingFound, err := runtimecontract.RuntimeOperationCorrectionFieldForRole(operationManifest, runtimecontract.RuntimeOperationCorrectionRoleEvidenceBinding)
	if err != nil || !bindingFound {
		t.Fatalf("binding correction field missing: field=%+v err=%v", bindingField, err)
	}
	for _, testCase := range []struct {
		name    string
		path    string
		prepare func(*runtimecontract.RuntimeOperation)
		remove  func(*runtimecontract.RuntimeOperation)
	}{
		{name: "kind", path: "$.operations[0].kind", remove: func(operation *runtimecontract.RuntimeOperation) { operation.Kind = "" }},
		{name: "bindingRefs", path: bindingField.CandidatePath(0), remove: func(operation *runtimecontract.RuntimeOperation) { operation.BindingRefs = nil }},
		{name: "targetRefs", path: targetField.CandidatePath(0), remove: func(operation *runtimecontract.RuntimeOperation) { operation.TargetRefs = nil }},
		{name: "parameters", path: "$.operations[0].parameters", remove: func(operation *runtimecontract.RuntimeOperation) { operation.Parameters = nil }},
		{
			name: "requiredOperationRef", path: "$.operations[0].requiredOperationRef",
			prepare: func(operation *runtimecontract.RuntimeOperation) { operation.RequiredOperationRef = "requirement-1" },
			remove:  func(operation *runtimecontract.RuntimeOperation) { operation.RequiredOperationRef = "" },
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			witness := runtimecontract.RuntimeCandidateCorrectionWitness{
				Dimension: runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField, Path: testCase.path,
				RejectedValue: candidateCorrectionFieldAbsent, AlternativeValue: candidateCorrectionFieldPresent,
			}
			present := candidate
			present.StateChange = &runtimecontract.StateChangeCandidate{Operations: append([]runtimecontract.StateChangingOperation(nil), candidate.StateChange.Operations...)}
			operation := present.StateChange.Operations[0].Operation
			if testCase.prepare != nil {
				testCase.prepare(&operation)
			}
			present.StateChange.Operations[0].Operation = operation
			if !candidateCrossesCorrectionWitness(present, manifest, witness) {
				t.Fatalf("present %s was compared with the literal field_present token", testCase.name)
			}
			missing := present
			missing.StateChange = &runtimecontract.StateChangeCandidate{Operations: append([]runtimecontract.StateChangingOperation(nil), present.StateChange.Operations...)}
			operation = missing.StateChange.Operations[0].Operation
			testCase.remove(&operation)
			missing.StateChange.Operations[0].Operation = operation
			if candidateCrossesCorrectionWitness(missing, manifest, witness) {
				t.Fatalf("missing %s crossed a field-presence witness", testCase.name)
			}
		})
	}
}

func TestCandidateCorrectionFrontierSealsExactEvidenceBindingAlternative(t *testing.T) {
	aggregate, candidate := aggregateWithActiveRejectedCandidate(t)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	candidate = runtimeTestMustDeriveCandidateForManifest(t, manifest, candidate)
	if candidate.StateChange == nil || len(candidate.StateChange.Operations) != 1 {
		t.Fatalf("unexpected candidate=%+v", candidate)
	}
	operation := candidate.StateChange.Operations[0].Operation
	operationManifest, found := candidateCorrectionOperationManifest(manifest, operation.Kind)
	if !found {
		t.Fatal("operation manifest missing")
	}
	authorization, err := runtimecontract.RuntimeCandidateOperationBindingAuthorizationFor(operationManifest, operation)
	if err != nil || len(authorization.AllowedBindingRefs) != 1 {
		t.Fatalf("binding authorization=%+v err=%v", authorization, err)
	}
	allowed := authorization.AllowedBindingRefs[0]
	wrong := candidate
	wrong.StateChange = &runtimecontract.StateChangeCandidate{Operations: append([]runtimecontract.StateChangingOperation(nil), candidate.StateChange.Operations...)}
	wrong.StateChange.Operations[0].Operation.BindingRefs = []string{"binding/wrong"}
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema,
		Code:  "candidate_schema_invalid", Signature: "candidate-schema-invalid", Reason: "binding is outside sealed manifest",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations[0].bindingRefs", Actual: "binding/wrong", Allowed: []string{allowed}, ManifestHash: manifest.Hash,
		},
	}
	frontier := candidateCorrectionFrontier{candidate: &wrong}
	sealed, correction, err := sealCandidateRejectionWithFrontier(failure, manifest, frontier)
	bindingField, bindingFound, bindingErr := runtimecontract.RuntimeOperationCorrectionFieldForRole(operationManifest, runtimecontract.RuntimeOperationCorrectionRoleEvidenceBinding)
	if err != nil || sealed == nil || correction == nil || correction.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || correction.Witness == nil ||
		correction.Witness.Dimension != runtimecontract.RuntimeCandidateCorrectionWitnessEvidenceBinding || correction.Witness.AlternativeValue != allowed ||
		bindingErr != nil || !bindingFound || correction.Witness.Path != bindingField.CandidatePath(0) ||
		correction.Witness.RejectedShapeHash == correction.Witness.AlternativeShapeHash ||
		!reflect.DeepEqual(correction.Objectives, []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSelectEvidenceBinding}) {
		t.Fatalf("evidence-binding correction sealed=%+v directive=%+v err=%v", sealed, correction, err)
	}
	if !candidateCrossesCorrectionWitness(candidate, manifest, *correction.Witness) {
		t.Fatal("exact allowed binding did not cross its sealed witness")
	}
	if candidateCrossesCorrectionWitness(wrong, manifest, *correction.Witness) {
		t.Fatal("rejected binding crossed its sealed witness")
	}
	sameFailure := *failure
	sameDiagnostic := *failure.Diagnostic
	sameDiagnostic.Actual = allowed
	sameFailure.Diagnostic = &sameDiagnostic
	final, noAlternative, err := sealCandidateRejectionWithFrontier(&sameFailure, manifest, candidateCorrectionFrontier{candidate: &candidate})
	if err != nil || final == nil || noAlternative == nil || noAlternative.Disposition != runtimecontract.RuntimeCandidateCorrectionRejectedFinal || noAlternative.TerminalCode != "candidate_authority_exhausted" {
		t.Fatalf("same binding incorrectly opened correction frontier: final=%+v correction=%+v err=%v", final, noAlternative, err)
	}

	transaction := aggregate.Decision.AuthorityTransaction()
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction)
	failureCopy := *sealed
	proposal := wrong
	// This fixture was originally built from a previously rejected operation.
	// Binding correction itself must be tested against an otherwise empty
	// rejected-operation frontier so the exact allowed binding remains legal on
	// restart.
	aggregate.Facts.Evidence.Rejected = nil
	aggregate.CompletedDispatches = nil
	aggregate.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{
		{
			AuthorityContractID: contractID, Transaction: transaction, Attempt: 0, StablePrefixHash: "binding-prefix",
			ManifestHash: manifest.Hash, LogicalRequestID: "binding-attempt-0", State: runtimecontract.RuntimeCandidateAttemptStateFailed,
			Proposal: &proposal, Failure: &failureCopy, Correction: correction.Clone(),
		},
		{
			AuthorityContractID: contractID, Transaction: transaction, Attempt: 1, StablePrefixHash: "binding-prefix",
			ManifestHash: manifest.Hash, LogicalRequestID: "binding-attempt-1", State: runtimecontract.RuntimeCandidateAttemptStatePending,
		},
	}
	if err := validateAttemptCandidateCrossesPersistedWitness(aggregate, manifest, candidate); err != nil {
		t.Fatalf("attempt 1 exact binding rejected: %v", err)
	}
}

func TestCandidateSchemaCorrectionDoesNotTreatDeclaredOptionalParameterAsForbidden(t *testing.T) {
	manifest := runtimecontract.RuntimeCandidateOperationManifest{
		Parameters: []runtimecontract.RuntimeOperationParameterContract{{Name: "optional", Required: false}},
	}
	if candidateCorrectionParameterRemovalAllowed(manifest, "optional") {
		t.Fatal("declared optional parameter authorized a forbidden-field removal")
	}
	manifest.ForbiddenParameters = []string{"optional"}
	if !candidateCorrectionParameterRemovalAllowed(manifest, "optional") {
		t.Fatal("explicitly forbidden declared parameter did not authorize removal")
	}
}

func TestCandidateCorrectionFrontierRequiresAlternativeManagedValidationRecipe(t *testing.T) {
	aggregate := validationAggregate(t)
	candidate := runtimeTestBindCandidateToCurrentFocus(t, aggregate, validationCandidate())
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	candidate = runtimeTestMustDeriveCandidateForManifest(t, manifest, candidate)
	currentOperation := candidate.Validation.Operations[0].Operation
	frontier := candidateCorrectionFrontier{
		candidate:               &candidate,
		rejectedOperationShapes: []runtimecontract.RuntimeRejectedOperationShape{{Operation: currentOperation}},
	}
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract,
		Code:  candidateAdmissionCodeReplaysRejectedShape, Signature: "recipe-replay", Reason: "replays rejected recipe",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			ManifestHash: manifest.Hash, RejectedShapeHash: runtimecontract.StableRuntimeCandidateSemanticShape(candidate),
			Objectives: []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionChangeReplayedShape},
		},
	}
	withAlternative := manifest.Clone()
	for index := range withAlternative.Operations {
		if withAlternative.Operations[index].Kind == runtimecontract.RuntimeBlockKindManagedValidationSession {
			withAlternative.Operations[index].ValidationRecipes = append(withAlternative.Operations[index].ValidationRecipes, runtimecontract.RuntimeValidationRecipe{ID: "test-managed-validation-alternative-witness", Command: "true"})
		}
	}
	withAlternative.Hash = ""
	withAlternative = withAlternative.Normalized()
	withAlternative.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(withAlternative)
	candidate = runtimeTestMustDeriveCandidateForManifest(t, withAlternative, candidate)
	alternativeCandidate := candidate
	alternativeCandidate.Validation = &runtimecontract.ValidationCandidate{Operations: append([]runtimecontract.ValidationOperation(nil), candidate.Validation.Operations...)}
	alternativeCandidate.Validation.Operations[0].Operation.Parameters = candidateCorrectionSetStringParameter(
		alternativeCandidate.Validation.Operations[0].Operation.Parameters, "recipeId", "test-managed-validation-alternative-witness",
	)
	alternativeCandidate, deriveErr := runtimecontract.DeriveRuntimeCandidateOperationAuthority(withAlternative, alternativeCandidate)
	if deriveErr != nil {
		t.Fatalf("derive alternative validation recipe candidate: %v", deriveErr)
	}
	if validateErr := runtimecontract.ValidateRuntimeCandidateAgainstManifest(withAlternative, alternativeCandidate); validateErr != nil {
		t.Fatalf("validate alternative validation recipe candidate: %v", validateErr)
	}
	frontier.candidate = &candidate
	failure.Diagnostic.ManifestHash = withAlternative.Hash
	failure.Diagnostic.RejectedShapeHash = runtimecontract.StableRuntimeCandidateSemanticShape(candidate)
	_, correction, err := sealCandidateRejectionWithFrontier(failure, withAlternative, frontier)
	var managedValidation runtimecontract.RuntimeCandidateOperationManifest
	for _, operation := range withAlternative.Operations {
		if operation.Kind == runtimecontract.RuntimeBlockKindManagedValidationSession {
			managedValidation = operation
			break
		}
	}
	recipeField, recipeFieldFound, recipeFieldErr := runtimecontract.RuntimeOperationCorrectionFieldForRole(managedValidation, runtimecontract.RuntimeOperationCorrectionRoleValidationRecipe)
	if err != nil || correction == nil || correction.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || correction.Witness == nil ||
		correction.Witness.Dimension != runtimecontract.RuntimeCandidateCorrectionWitnessValidationRecipe || correction.Witness.AlternativeValue == "test-managed-validation" ||
		recipeFieldErr != nil || !recipeFieldFound || correction.Witness.Path != recipeField.CandidatePath(0) {
		t.Fatalf("managed validation alternative recipe witness=%+v err=%v", correction, err)
	}

	singleRecipe := manifest.Clone()
	singleRecipeCandidate := runtimeTestMustDeriveCandidateForManifest(t, singleRecipe, candidate)
	singleRecipeFrontier := frontier
	singleRecipeFrontier.candidate = &singleRecipeCandidate
	failure.Diagnostic.ManifestHash = singleRecipe.Hash
	failure.Diagnostic.RejectedShapeHash = runtimecontract.StableRuntimeCandidateSemanticShape(singleRecipeCandidate)
	_, exhausted, err := sealCandidateRejectionWithFrontier(failure, singleRecipe, singleRecipeFrontier)
	if err != nil || exhausted == nil || exhausted.Disposition != runtimecontract.RuntimeCandidateCorrectionRejectedFinal || exhausted.TerminalCode != "candidate_authority_exhausted" {
		t.Fatalf("single-recipe frontier did not exhaust: directive=%+v err=%v", exhausted, err)
	}
}

func TestCandidateCorrectionFrontierSealsOperationKindFromTypedDimension(t *testing.T) {
	aggregate, candidate := aggregateWithActiveRejectedCandidate(t)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Operations) != 1 || manifest.Operations[0].Kind != runtimecontract.RuntimeBlockKindPatchApply {
		t.Fatalf("unexpected operation manifest: %+v", manifest.Operations)
	}
	alternativeAuthorization := manifest.Operations[0]
	alternativeAuthorization.Kind = runtimecontract.RuntimeBlockKindPatchPreview
	manifest.Operations = append(manifest.Operations, alternativeAuthorization)
	manifest.Hash = ""
	manifest = manifest.Normalized()
	manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)
	candidate = runtimeTestMustDeriveCandidateForManifest(t, manifest, candidate)

	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract,
		Code:  candidateAdmissionCodeReplaysRejectedShape, Signature: "kind-replay", Reason: "replays rejected kind",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			ManifestHash: manifest.Hash, RejectedShapeHash: runtimecontract.StableRuntimeCandidateSemanticShape(candidate),
			Objectives: []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionChangeReplayedShape},
		},
	}
	frontier := candidateCorrectionFrontier{
		candidate:               &candidate,
		rejectedOperationShapes: []runtimecontract.RuntimeRejectedOperationShape{{Operation: candidate.StateChange.Operations[0].Operation}},
	}
	_, correction, err := sealCandidateRejectionWithFrontier(failure, manifest, frontier)
	kindField := runtimecontract.RuntimeOperationKindCorrectionField()
	if err != nil || correction == nil || correction.Witness == nil ||
		correction.Witness.Dimension != runtimecontract.RuntimeCandidateCorrectionWitnessOperationKind ||
		correction.Witness.AlternativeValue != runtimecontract.RuntimeBlockKindPatchPreview ||
		correction.Witness.Path != kindField.CandidatePath(0) {
		t.Fatalf("typed operation-kind correction witness=%+v err=%v", correction, err)
	}
	alternative := candidate
	alternative.StateChange = &runtimecontract.StateChangeCandidate{Operations: append([]runtimecontract.StateChangingOperation(nil), candidate.StateChange.Operations...)}
	alternative.StateChange.Operations[0].Operation.Kind = runtimecontract.RuntimeBlockKindPatchPreview
	if !candidateCrossesCorrectionWitness(alternative, manifest, *correction.Witness) {
		t.Fatal("typed operation-kind alternative did not cross its sealed witness")
	}
}

func TestCandidateCorrectionFrontierSealsOnlyContractDeclaredClosedParameter(t *testing.T) {
	aggregate := validationAggregate(t)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Operations) != 1 {
		t.Fatalf("unexpected operation manifest: %+v", manifest.Operations)
	}
	contract, found := runtimecontract.RuntimeOperationContractForKind(runtimecontract.StepKindFileList)
	if !found {
		t.Fatal("file list operation contract missing")
	}
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(runtimecontract.StepKindFileList)
	if !found {
		t.Fatal("file list operation descriptor missing")
	}
	fileListAuthorization := manifest.Operations[0]
	fileListAuthorization.Kind = runtimecontract.StepKindFileList
	fileListAuthorization.OwnerModuleID = descriptor.OwnerModuleID
	fileListAuthorization.Parameters = contract.Parameters
	fileListAuthorization.RequireOneOfParameters = contract.RequireOneOfParameters
	fileListAuthorization.TargetCardinality = descriptor.TargetCardinality
	fileListAuthorization.RequireTargetRefs = contract.RequireTargetRefs
	fileListAuthorization.RequiredOperationFields = []string{"kind", "parameters"}
	fileListAuthorization.BindingAuthorizations = []runtimecontract.RuntimeOperationBindingAuthorization{{}}
	fileListAuthorization.ForbiddenParameters = contract.ForbiddenParameters
	fileListAuthorization.Variants = contract.Variants
	fileListAuthorization.ValidationRecipes = nil
	manifest.Operations = append(manifest.Operations, fileListAuthorization)
	manifest.Hash = ""
	manifest = manifest.Normalized()
	manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)

	recursive := false
	candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "list-workspace", Kind: runtimecontract.StepKindFileList,
		Parameters: []runtimecontract.OperationParameter{{Name: "recursive", Boolean: &recursive}},
	}}}}}
	candidate = runtimeTestMustDeriveCandidateForManifest(t, manifest, candidate)
	alternativeCandidate := candidate
	alternativeCandidate.Validation = &runtimecontract.ValidationCandidate{Operations: append([]runtimecontract.ValidationOperation(nil), candidate.Validation.Operations...)}
	alternativeCandidate.Validation.Operations[0].Operation.Parameters = candidateCorrectionSetBooleanParameter(
		alternativeCandidate.Validation.Operations[0].Operation.Parameters, "recursive", true,
	)
	alternativeCandidate, deriveErr := runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, alternativeCandidate)
	if deriveErr != nil {
		t.Fatalf("derive closed-parameter alternative candidate: %v", deriveErr)
	}
	if validateErr := runtimecontract.ValidateRuntimeCandidateAgainstManifest(manifest, alternativeCandidate); validateErr != nil {
		t.Fatalf("validate closed-parameter alternative candidate: %v", validateErr)
	}
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract,
		Code:  candidateAdmissionCodeReplaysRejectedShape, Signature: "closed-parameter-replay", Reason: "replays rejected closed parameter",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			ManifestHash: manifest.Hash, RejectedShapeHash: runtimecontract.StableRuntimeCandidateSemanticShape(candidate),
			Objectives: []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionChangeReplayedShape},
		},
	}
	frontier := candidateCorrectionFrontier{
		candidate:               &candidate,
		rejectedOperationShapes: []runtimecontract.RuntimeRejectedOperationShape{{Operation: candidate.Validation.Operations[0].Operation}},
	}
	_, correction, err := sealCandidateRejectionWithFrontier(failure, manifest, frontier)
	closedFields, metadataErr := runtimecontract.RuntimeOperationCorrectionFieldsForRole(fileListAuthorization, runtimecontract.RuntimeOperationCorrectionRoleClosedParameter)
	if metadataErr != nil || len(closedFields) != 1 {
		t.Fatalf("closed parameter metadata=%+v err=%v", closedFields, metadataErr)
	}
	if err != nil || correction == nil || correction.Witness == nil ||
		correction.Witness.Dimension != runtimecontract.RuntimeCandidateCorrectionWitnessClosedParameter ||
		correction.Witness.Path != closedFields[0].CandidatePath(0) || correction.Witness.AlternativeValue != "true" {
		t.Fatalf("typed closed-parameter correction witness=%+v err=%v", correction, err)
	}
	alternative := candidate
	alternative.Validation = &runtimecontract.ValidationCandidate{Operations: append([]runtimecontract.ValidationOperation(nil), candidate.Validation.Operations...)}
	alternative.Validation.Operations[0].Operation.Parameters = candidateCorrectionSetBooleanParameter(
		alternative.Validation.Operations[0].Operation.Parameters, closedFields[0].Name, true,
	)
	if !candidateCrossesCorrectionWitness(alternative, manifest, *correction.Witness) {
		t.Fatal("typed closed-parameter alternative did not cross its sealed witness")
	}
}
