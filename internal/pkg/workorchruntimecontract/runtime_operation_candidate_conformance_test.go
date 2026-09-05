package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func TestTextEditConstraintsAreOperationScopedSealedAndDetached(t *testing.T) {
	var authorizations []RuntimeCandidateOperationManifest
	for _, kind := range []string{StepKindFileEdit, RuntimeBlockKindPatchApply, RuntimeBlockKindPatchPreview} {
		contract, _ := RuntimeOperationContractForKind(kind)
		authorization := RuntimeCandidateOperationManifest{Kind: kind, OwnerModuleID: RuntimeModuleFile,
			Parameters: contract.Parameters, Variants: contract.Variants, CandidateConstraints: contract.CandidateConstraints,
			OperationContractRevision: RuntimeOperationContractRevision()}.Normalized()
		authorizations = append(authorizations, authorization)
		old, replacement, patchType := "old", "new", "text_replace"
		operation := RuntimeOperation{ID: "edit", Kind: kind, TargetRefs: []string{"target"},
			Parameters: []OperationParameter{{Name: "oldText", String: &old}, {Name: "newText", String: &replacement}}}
		expectedVariant := ""
		if kind != StepKindFileEdit {
			expectedVariant = patchType
			operation.Parameters = append(operation.Parameters, OperationParameter{Name: "patchType", String: &patchType})
		}
		variant, constraints, err := RuntimeOperationCandidateConstraintsForOperation(operation, authorization)
		if err != nil || variant != expectedVariant || !reflect.DeepEqual(constraints, runtimeTextReplacementCandidateConstraints()) {
			t.Fatalf("%s constraints=%+v variant=%q err=%v", kind, constraints, variant, err)
		}
		_, static, err := RuntimeOperationCandidateConformanceConstraintsForOperation(operation, authorization)
		if err != nil || len(static) != 0 {
			t.Fatalf("%s created a placeholder static probe: %+v err=%v", kind, static, err)
		}
	}
	manifest := RuntimeCandidateContractManifest{Revision: RuntimeCandidateContractManifestRevision, Operations: authorizations}.Normalized()
	for _, kind := range []string{StepKindFileEdit, RuntimeBlockKindPatchApply, RuntimeBlockKindPatchPreview} {
		if constraint, found := RuntimeCandidateConstraintForOperationRef(manifest, kind, runtimeTextReplacementCandidateConstraints()[0].Ref); !found ||
			constraint.RequiredShape != RuntimeOperationCandidateRequiredShapeTextReplaceApplicableToObservedTarget {
			t.Fatalf("%s lookup lost its own constraint: %+v", kind, constraint)
		}
	}
	with := StableRuntimeCandidateContractManifestHash(manifest)
	detached := manifest.Clone()
	for index := range detached.Operations {
		if detached.Operations[index].Kind == StepKindFileEdit {
			detached.Operations[index].CandidateConstraints[0].Ref = "tampered"
		}
	}
	if with == StableRuntimeCandidateContractManifestHash(detached) || with != StableRuntimeCandidateContractManifestHash(manifest) {
		t.Fatal("operation-level constraints are not detached/hash-bound")
	}
	witness := RuntimeCandidateCorrectionWitness{Dimension: RuntimeCandidateCorrectionWitnessOwnerConstraint, OperationKind: StepKindFileEdit,
		Path: "$.operations[0].parameters[name=oldText]", ConstraintRef: runtimeTextReplacementCandidateConstraints()[0].Ref,
		RejectedValue:    string(RuntimeOperationCandidateActualShapeObservedTargetContextMismatch),
		AlternativeValue: string(RuntimeOperationCandidateRequiredShapeTextReplaceApplicableToObservedTarget)}
	if err := witness.Validate(); err != nil {
		t.Fatalf("declared edit constraint cannot form bounded correction: %v", err)
	}
}

func TestDynamicEditConstraintCannotBypassOwnerFeasibilityWithStaticSeal(t *testing.T) {
	for _, kind := range []string{StepKindFileEdit, RuntimeBlockKindPatchApply, RuntimeBlockKindPatchPreview} {
		contract, _ := RuntimeOperationContractForKind(kind)
		old, replacement, patchType := "old", "new", "text_replace"
		operation := RuntimeOperation{ID: "edit", Kind: kind, TargetRefs: []string{"target"},
			Parameters: []OperationParameter{{Name: "oldText", String: &old}, {Name: "newText", String: &replacement}}}
		if kind != StepKindFileEdit {
			operation.Parameters = append(operation.Parameters, OperationParameter{Name: "patchType", String: &patchType})
		}
		manifest := RuntimeCandidateContractManifest{Hash: "manifest", Operations: []RuntimeCandidateOperationManifest{{
			Kind: kind, OwnerModuleID: RuntimeModuleFile, Parameters: contract.Parameters, Variants: contract.Variants, CandidateConstraints: contract.CandidateConstraints,
		}}}.Normalized()
		candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: operation}}}}
		decision := evaluateRuntimeCompleteAdmissionFeasibility(RuntimeFacts{}, manifest, candidate, nil)
		if decision.Admitted() || decision.Code != "owner_catalog_missing" {
			t.Fatalf("%s bypassed dynamic constraint: %+v", kind, decision)
		}
	}
}

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
	if variantID != "unified_diff" || len(constraints) != 3 ||
		constraints[0].Ref != "patch/unified_diff/diff/observed_target" || constraints[0].RequiredShape != RuntimeOperationCandidateRequiredShapeUnifiedDiffApplicableToObservedTarget ||
		constraints[1].Ref != "patch/unified_diff/diff/single_file" || constraints[1].ParameterName != "diff" ||
		constraints[2].Ref != "patch/unified_diff/diff/target_mode" || constraints[2].RequiredShape != RuntimeOperationCandidateRequiredShapeUnifiedDiffAuthorizedTargetMode {
		t.Fatalf("sealed constraints = variant=%q values=%+v", variantID, constraints)
	}
	_, conformanceConstraints, err := RuntimeOperationCandidateConformanceConstraintsForOperation(operation, authorization)
	if err != nil || len(conformanceConstraints) != 2 {
		t.Fatalf("representation constraints = %+v err=%v", conformanceConstraints, err)
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
	violation := &RuntimeOperationCandidateConstraintViolation{
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

func TestRuntimeOperationCandidateApplicabilityViolationIsManifestAndObservationBound(t *testing.T) {
	patch, _ := RuntimeOperationContractForKind(RuntimeBlockKindPatchApply)
	patchType := "unified_diff"
	diff := "--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n"
	inventory, err := NewRuntimeOperationAdmissibility("admissibility-file", RuntimeModuleFile, []string{RuntimeBlockKindPatchApply}, []string{"main.go"}, nil, nil, "tree-v1")
	if err != nil {
		t.Fatal(err)
	}
	request := RuntimeOperationFeasibilityRequest{
		WorkspaceID: "ws-1", RunID: "run-1", Transaction: AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
		WorkspaceDeliverableDirectory: t.TempDir(), CandidateManifestHash: "manifest-1", Admissibility: inventory,
		Operation: RuntimeOperation{ID: "patch-1", Kind: RuntimeBlockKindPatchApply, TargetRefs: []string{"main.go"}, Parameters: []OperationParameter{{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}}},
		OperationAuthorization: RuntimeCandidateOperationManifest{
			Kind: patch.Kind, OwnerModuleID: RuntimeModuleFile, Parameters: patch.Parameters, Variants: patch.Variants,
			Target:                    RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"main.go"}},
			OperationContractRevision: RuntimeOperationContractRevision(),
		}.Normalized(),
	}
	violation := RuntimeOperationCandidateConstraintViolation{
		ConstraintRef: "patch/unified_diff/diff/observed_target", ParameterName: "diff",
		RequiredShape: RuntimeOperationCandidateRequiredShapeUnifiedDiffApplicableToObservedTarget,
		ActualShape:   RuntimeOperationCandidateActualShapeObservedTargetContextMismatch,
		TargetRef:     "main.go", ObservedRevision: inventory.ObservedRevision,
	}
	result, err := NewRuntimeOperationCandidateConstraintViolationObservation(request, violation)
	if err != nil || result.Outcome != RuntimeOwnerFeasibilityCandidateConstraintViolated || result.CandidateConstraintViolation == nil || result.Feedback != nil {
		t.Fatalf("candidate applicability result = %+v err=%v", result, err)
	}
	tampered := violation
	tampered.ObservedRevision = "tree-v2"
	if _, err := NewRuntimeOperationCandidateConstraintViolationObservation(request, tampered); err == nil {
		t.Fatal("candidate applicability violation with a foreign owner revision was accepted")
	}
	tampered = violation
	tampered.ConstraintRef = "patch/unified_diff/diff/single_file"
	if _, err := NewRuntimeOperationCandidateConstraintViolationObservation(request, tampered); err == nil {
		t.Fatal("target applicability fact crossed a representation-only constraint")
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
