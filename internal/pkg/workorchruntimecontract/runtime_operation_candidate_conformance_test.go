package workorchruntimecontract

import "testing"

func TestRuntimeOperationCandidateConstraintsAreSealedAndOwnerNeutral(t *testing.T) {
	patch, found := RuntimeOperationContractForKind(RuntimeBlockKindPatchApply)
	if !found {
		t.Fatal("patch/apply contract is missing")
	}
	patchType, diff := "unified_diff", "not-a-diff"
	operation := RuntimeOperation{
		ID: "patch-1", Kind: RuntimeBlockKindPatchApply, TargetRefs: []string{"main.go"},
		Parameters: []OperationParameter{{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}},
	}
	authorization := RuntimeCandidateOperationManifest{
		Kind: patch.Kind, OwnerModuleID: RuntimeModuleFile, Parameters: patch.Parameters, Variants: patch.Variants,
		OperationContractRevision: RuntimeOperationContractRevision(),
	}.Normalized()
	variantID, constraints, err := RuntimeOperationCandidateConstraintsForOperation(operation, authorization)
	if err != nil {
		t.Fatal(err)
	}
	if variantID != "unified_diff" || len(constraints) != 1 || constraints[0].Ref != "patch/unified_diff/diff/single_file" || constraints[0].ParameterName != "diff" {
		t.Fatalf("sealed constraints = variant=%q values=%+v", variantID, constraints)
	}
	for _, kind := range []string{StepKindGitRead, StepKindGitWrite, StepKindShellCommand, StepKindSSHCommand, StepKindWebFetch, StepKindMCPToolCall} {
		contract, ok := RuntimeOperationContractForKind(kind)
		if !ok {
			t.Fatalf("operation contract %q is missing", kind)
		}
		for _, variant := range contract.Variants {
			if len(variant.CandidateConstraints) != 0 {
				t.Fatalf("unrelated owner %q gained placeholder candidate constraints: %+v", kind, variant.CandidateConstraints)
			}
		}
	}
}

func TestRuntimeOperationCandidateConformanceObservationRejectsTamperedConstraint(t *testing.T) {
	patch, _ := RuntimeOperationContractForKind(RuntimeBlockKindPatchApply)
	patchType, diff := "unified_diff", "not-a-diff"
	request := RuntimeOperationCandidateConformanceRequest{
		WorkspaceID: "ws-1", RunID: "run-1", Transaction: AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
		CandidateManifestHash:  "manifest-1",
		Operation:              RuntimeOperation{ID: "patch-1", Kind: RuntimeBlockKindPatchApply, TargetRefs: []string{"main.go"}, Parameters: []OperationParameter{{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}}},
		OperationAuthorization: RuntimeCandidateOperationManifest{Kind: patch.Kind, OwnerModuleID: RuntimeModuleFile, Parameters: patch.Parameters, Variants: patch.Variants, OperationContractRevision: RuntimeOperationContractRevision()}.Normalized(),
	}
	violation := &RuntimeOperationCandidateConformanceViolation{
		ConstraintRef: "patch/unified_diff/diff/single_file", ParameterName: "diff",
		RequiredShape: RuntimeOperationCandidateRequiredShapeUnifiedDiffSingleFile, ActualShape: RuntimeOperationCandidateActualShapeInvalidSyntax,
	}
	observation := RuntimeOperationCandidateConformanceObservation{
		OperationID: "patch-1", OperationKind: RuntimeBlockKindPatchApply, VariantID: "unified_diff", OwnerModuleID: RuntimeModuleFile,
		CandidateManifestHash: "manifest-1", Outcome: RuntimeOperationCandidateNonconformant, Violation: violation,
	}
	if err := observation.ValidateForRequest(request, RuntimeModuleFile); err != nil {
		t.Fatal(err)
	}
	tampered := observation.Clone()
	tampered.Violation.ConstraintRef = "patch/unified_diff/diff/other"
	if err := tampered.ValidateForRequest(request, RuntimeModuleFile); err == nil {
		t.Fatal("tampered constraint ref crossed the owner boundary")
	}
	tampered = observation.Clone()
	tampered.CandidateManifestHash = "manifest-2"
	if err := tampered.ValidateForRequest(request, RuntimeModuleFile); err == nil {
		t.Fatal("tampered manifest identity crossed the owner boundary")
	}
}

func TestRuntimeCandidateManifestHashIncludesOwnerCandidateConstraint(t *testing.T) {
	patch, _ := RuntimeOperationContractForKind(RuntimeBlockKindPatchApply)
	base := RuntimeCandidateContractManifest{
		Revision: RuntimeCandidateContractManifestRevision, State: ConvergenceStateNeedStateChange,
		OperationContractRevision: RuntimeOperationContractRevision(),
		Operations: []RuntimeCandidateOperationManifest{{
			Kind: patch.Kind, OwnerModuleID: RuntimeModuleFile, Parameters: patch.Parameters, Variants: patch.Variants,
			OperationContractRevision: RuntimeOperationContractRevision(),
		}},
	}
	withConstraint := StableRuntimeCandidateContractManifestHash(base)
	without := base.Clone()
	for variantIndex := range without.Operations[0].Variants {
		without.Operations[0].Variants[variantIndex].CandidateConstraints = nil
	}
	if withConstraint == StableRuntimeCandidateContractManifestHash(without) {
		t.Fatal("candidate manifest hash ignored owner candidate constraints")
	}
}

func TestRuntimeCandidateConstraintLookupIsOperationScopedWhenRefIsShared(t *testing.T) {
	apply, found := RuntimeOperationContractForKind(RuntimeBlockKindPatchApply)
	if !found {
		t.Fatal("patch/apply contract is missing")
	}
	preview, found := RuntimeOperationContractForKind(RuntimeBlockKindPatchPreview)
	if !found {
		t.Fatal("patch/preview contract is missing")
	}
	manifest := RuntimeCandidateContractManifest{
		Revision:                  RuntimeCandidateContractManifestRevision,
		State:                     ConvergenceStateNeedStateChange,
		OperationContractRevision: RuntimeOperationContractRevision(),
		Operations: []RuntimeCandidateOperationManifest{
			{Kind: apply.Kind, OwnerModuleID: RuntimeModuleFile, Parameters: apply.Parameters, Variants: apply.Variants, OperationContractRevision: RuntimeOperationContractRevision()},
			{Kind: preview.Kind, OwnerModuleID: RuntimeModuleFile, Parameters: preview.Parameters, Variants: preview.Variants, OperationContractRevision: RuntimeOperationContractRevision()},
		},
	}.Normalized()
	manifest.Hash = StableRuntimeCandidateContractManifestHash(manifest)

	const ref = "patch/unified_diff/diff/single_file"
	applyConstraint, applyFound := RuntimeCandidateConstraintForOperationRef(manifest, RuntimeBlockKindPatchApply, ref)
	previewConstraint, previewFound := RuntimeCandidateConstraintForOperationRef(manifest, RuntimeBlockKindPatchPreview, ref)
	if !applyFound || !previewFound || applyConstraint != previewConstraint {
		t.Fatalf("shared constraint lookup apply=(%+v,%t) preview=(%+v,%t)", applyConstraint, applyFound, previewConstraint, previewFound)
	}
	if _, found := RuntimeCandidateConstraintForOperationRef(manifest, StepKindShellCommand, ref); found {
		t.Fatal("shared File constraint resolved for an unrelated operation kind")
	}
}
