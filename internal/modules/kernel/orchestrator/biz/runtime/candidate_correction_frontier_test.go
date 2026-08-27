package runtime

import (
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
	candidate := validationCandidate()
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
	fileListAuthorization.RequiredOperationFields = []string{"bindingRefs", "kind", "parameters"}
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
		BindingRefs: []string{"binding/test/runtime/focus-1/cwi-1"},
		Parameters:  []runtimecontract.OperationParameter{{Name: "recursive", Boolean: &recursive}},
	}}}}}
	candidate = runtimeTestMustDeriveCandidateForManifest(t, manifest, candidate)
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
