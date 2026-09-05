package runtime

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeValidationExecutionBindingComesOnlyFromSealedCWIAndOwnerCatalog(t *testing.T) {
	dispatch, operation, catalog, wanted := runtimeValidationExecutionBindingFixture(t)

	selected, err := runtimeValidationProofBindingForOperation(dispatch, operation, &catalog)
	if err != nil || !sameRuntimeValidationExecutionProofBinding(selected, wanted) {
		t.Fatalf("select sealed validation proof binding: selected=%+v want=%+v err=%v", selected, wanted, err)
	}
	// Recovery reconstructs the same detached authorization from persisted
	// dispatch facts; it does not mint a new mapping or consult a candidate.
	restarted, err := runtimeValidationProofBindingForOperation(dispatch, operation, &catalog)
	if err != nil || !sameRuntimeValidationExecutionProofBinding(restarted, selected) {
		t.Fatalf("restart changed sealed validation proof binding: first=%+v restarted=%+v err=%v", selected, restarted, err)
	}

	stateChange := operation
	stateChange.Kind = runtimecontract.StepKindFileWrite
	stateChange.BindingRefs = nil
	if empty, err := runtimeValidationProofBindingForOperation(dispatch, stateChange, nil); err != nil || empty.RequirementHash != "" {
		t.Fatalf("non-managed operation received validation proof authority: binding=%+v err=%v", empty, err)
	}

	tests := []struct {
		name   string
		mutate func(*SealedDispatch, *runtimecontract.RuntimeOperation, *runtimecontract.RuntimeOperationAdmissibility)
		want   string
	}{
		{
			name: "missing persisted binding",
			mutate: func(_ *SealedDispatch, operation *runtimecontract.RuntimeOperation, _ *runtimecontract.RuntimeOperationAdmissibility) {
				operation.BindingRefs = nil
			},
			want: "exactly one persisted CWI binding",
		},
		{
			name: "owner attestation drift",
			mutate: func(_ *SealedDispatch, _ *runtimecontract.RuntimeOperation, catalog *runtimecontract.RuntimeOperationAdmissibility) {
				binding := &catalog.ValidationSurfaces[0].ProofRequirementBindings[0]
				binding.CheckAttestations[0].NativeCheckRef = "owner/native/drifted"
				catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
			},
			want: "does not match exact owner surface binding",
		},
		{
			name: "other CWI owner binding",
			mutate: func(_ *SealedDispatch, _ *runtimecontract.RuntimeOperation, catalog *runtimecontract.RuntimeOperationAdmissibility) {
				binding := &catalog.ValidationSurfaces[0].ProofRequirementBindings[0]
				binding.CoverageWorkItemID = "cwi-other"
				catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
			},
			want: "does not match exact owner surface binding",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			driftedDispatch := dispatch
			driftedDispatch.CoverageBindings = runtimecontract.CloneCoverageEvidenceBindings(dispatch.CoverageBindings)
			driftedOperation := operation.Normalized()
			driftedCatalog := catalog.Normalized()
			test.mutate(&driftedDispatch, &driftedOperation, &driftedCatalog)
			_, err := runtimeValidationProofBindingForOperation(driftedDispatch, driftedOperation, &driftedCatalog)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("drift err=%v, want %q", err, test.want)
			}
		})
	}
}

func runtimeValidationExecutionBindingFixture(t *testing.T) (SealedDispatch, runtimecontract.RuntimeOperation, runtimecontract.RuntimeOperationAdmissibility, runtimecontract.RuntimeValidationProofRequirementBinding) {
	t.Helper()
	item := runtimecontract.RuntimeCoverageItem{
		ID: "cwi-execution-proof", SliceID: "slice-execution-proof", SourceScope: "dodItem", SourceID: "dod-execution-proof", TargetText: "the exact CWI passes its native validation",
	}
	focus := runtimecontract.RuntimeDefinitionFocus{
		ID: "focus-execution-proof", SliceID: item.SliceID, AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AcceptedStepKinds:      []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		EvidenceDeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID),
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements("definition-execution-proof", []runtimecontract.RuntimeCoverageItem{item}, []runtimecontract.RuntimeDefinitionFocus{focus})
	if err != nil || len(requirements) != 1 {
		t.Fatalf("compile validation proof requirement: requirements=%+v err=%v", requirements, err)
	}
	requirement := requirements[0]
	const recipeID = "recipe/execution-proof"
	attestations := runtimeTestValidationCheckAttestations(requirement, recipeID)
	ownerBinding, err := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirement, recipeID, attestations...)
	if err != nil {
		t.Fatal(err)
	}
	proof := runtimecontract.CoverageEvidenceProof{
		EvidenceKind: runtimecontract.EvidenceKindValidation, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectEphemeralSession},
		AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: recipeID,
		ValidationProofRequirementHash: requirement.Hash, RequiredCheckRefs: append([]string(nil), requirement.RequiredCheckRefs...),
		ValidationCheckAttestations: runtimecontract.CloneRuntimeValidationCheckAttestationBindings(attestations),
	}
	coverageBinding := runtimecontract.CoverageEvidenceBinding{
		ID: "binding/execution-proof", CoverageWorkItemID: item.ID, FocusID: focus.ID, DefinitionRevision: requirement.DefinitionRevision,
		Proofs: []runtimecontract.CoverageEvidenceProof{proof}, TargetRefs: []string{"validation_target.go"}, DeclarationRef: requirement.DeclarationRef,
	}
	coverageBinding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(coverageBinding)
	if err := coverageBinding.Validate(); err != nil {
		t.Fatalf("coverage binding: %v", err)
	}
	recipe := runtimecontract.RuntimeValidationRecipe{ID: recipeID, Command: "owner-test", Args: []string{"opaque"}, TimeoutMillis: 1000}
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "shell-execution-proof", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationRecipeRefs: []string{recipeID},
		ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{recipe}, ObservedRevision: "shell-execution-proof-v1",
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{
			ID: recipeID, OwnerModuleID: runtimecontract.RuntimeModuleShell, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			Readiness: runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "schema/execution-proof", InputBindingRef: "input/execution-proof",
			CheckRefs: append([]string(nil), requirement.RequiredCheckRefs...), ProofApplicability: runtimecontract.RuntimeValidationProofDirect,
			ProofRequirementBindings: []runtimecontract.RuntimeValidationProofRequirementBinding{ownerBinding},
			EffectScope:              runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		}},
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatalf("owner catalog: %v", err)
	}
	operation := runtimecontract.RuntimeOperation{
		ID: "validate-execution-proof", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		BindingRefs: []string{coverageBinding.ID}, TargetRefs: []string{"validation_target.go"},
	}
	dispatch := SealedDispatch{ID: "dispatch-execution-proof", CoverageBindings: []runtimecontract.CoverageEvidenceBinding{coverageBinding}}
	return dispatch, operation, catalog, ownerBinding
}
