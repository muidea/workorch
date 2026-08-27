package workorchruntimecontract

import (
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func testValidationFailureRepairConvergence() RuntimeValidationFailureRepairConvergence {
	return RuntimeValidationFailureRepairConvergence{
		ID:                                        "dod-validation-repair",
		TriggerOwnerModuleID:                      RuntimeModuleShell,
		TriggerOperationKind:                      RuntimeBlockKindManagedValidationSession,
		TriggerDiagnosticClasses:                  []string{ValidationDiagnosticImplementationGap, ValidationDiagnosticBuildFailed, ValidationDiagnosticTestFailed},
		RepairEvidenceKind:                        EvidenceKindStateChange,
		RepairOwnerModuleID:                       RuntimeModuleFile,
		RepairOperationKinds:                      []string{StepKindFileWrite, RuntimeBlockKindPatchApply},
		RepairTargetPolicy:                        RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		RepairTargetRole:                          "deliverable_implementation",
		RepairDeclarationTargetSelectorRef:        "file/workspace_deliverable",
		RepairDeclarationTargetSelectorOwnerRef:   "software-dev",
		RepairDeclarationTargetSelectorSubjectRef: "dodItem_repair",
		RepairOperationTargetSelectorRef:          "workspace-relative-new-file-target",
		RequireFreshValidation:                    true,
		MaxAttempts:                               2,
	}
}

func TestRuntimeValidationFailureRepairConvergenceIsOrderedAllOfAndStable(t *testing.T) {
	value := testValidationFailureRepairConvergence()
	value.TriggerDiagnosticClasses = append(value.TriggerDiagnosticClasses, ValidationDiagnosticBuildFailed)
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	normalized := value.Normalized()
	if len(normalized.TriggerDiagnosticClasses) != 3 || !normalized.MatchesTrigger(RuntimeModuleShell, RuntimeBlockKindManagedValidationSession, ValidationDiagnosticBuildFailed) {
		t.Fatalf("normalized convergence = %+v", normalized)
	}
	clone := normalized.Clone()
	clone.TriggerDiagnosticClasses[0] = ValidationDiagnosticCommandInvalid
	if normalized.TriggerDiagnosticClasses[0] == clone.TriggerDiagnosticClasses[0] {
		t.Fatal("convergence clone shares diagnostic storage")
	}
	if StableRuntimeValidationFailureRepairConvergenceHash(normalized) == StableRuntimeValidationFailureRepairConvergenceHash(clone) {
		t.Fatal("convergence hash ignored diagnostic drift")
	}
}

func TestRuntimeValidationFailureRepairConvergenceRejectsIncompletePresealContract(t *testing.T) {
	valid := testValidationFailureRepairConvergence()
	for _, tc := range []struct {
		name string
		want string
		edit func(*RuntimeValidationFailureRepairConvergence)
	}{
		{name: "missing diagnostic", want: "typed trigger", edit: func(v *RuntimeValidationFailureRepairConvergence) { v.TriggerDiagnosticClasses = nil }},
		{name: "unknown diagnostic", want: "unknown diagnostic", edit: func(v *RuntimeValidationFailureRepairConvergence) {
			v.TriggerDiagnosticClasses = []string{"free_form_failure"}
		}},
		{name: "non validation trigger", want: "not validation proof", edit: func(v *RuntimeValidationFailureRepairConvergence) {
			v.TriggerOwnerModuleID = RuntimeModuleFile
			v.TriggerOperationKind = StepKindFileWrite
		}},
		{name: "non state repair", want: "state_change", edit: func(v *RuntimeValidationFailureRepairConvergence) { v.RepairEvidenceKind = EvidenceKindValidation }},
		{name: "cross owner repair", want: "not state-change proof", edit: func(v *RuntimeValidationFailureRepairConvergence) {
			v.RepairOperationKinds = []string{StepKindShellCommand}
		}},
		{name: "exact policy", want: "selector-backed", edit: func(v *RuntimeValidationFailureRepairConvergence) {
			v.RepairTargetPolicy = RuntimeOperationTargetPolicyExactTarget
		}},
		{name: "missing operation selector", want: "complete selector", edit: func(v *RuntimeValidationFailureRepairConvergence) { v.RepairOperationTargetSelectorRef = "" }},
		{name: "aliased declaration and operation selectors", want: "distinct declaration and operation selector", edit: func(v *RuntimeValidationFailureRepairConvergence) {
			v.RepairOperationTargetSelectorRef = v.RepairDeclarationTargetSelectorRef
		}},
		{name: "no fresh validation", want: "fresh validation", edit: func(v *RuntimeValidationFailureRepairConvergence) { v.RequireFreshValidation = false }},
		{name: "zero attempts", want: "max attempts", edit: func(v *RuntimeValidationFailureRepairConvergence) { v.MaxAttempts = 0 }},
		{name: "excess attempts", want: "max attempts", edit: func(v *RuntimeValidationFailureRepairConvergence) { v.MaxAttempts = MaxRepairAttemptsPerFocusLimit + 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := valid.Clone()
			tc.edit(&value)
			err := value.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() err=%v, want %q", err, tc.want)
			}
		})
	}
}

func TestRuntimeProfileValidationRepairRequiresDoDValidationTerminal(t *testing.T) {
	repair := testValidationFailureRepairConvergence()
	valid := RuntimeProfileContract{ID: "software-dev", EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{
		ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{EvidenceKindValidation},
		AcceptedStepKinds:                  []string{RuntimeBlockKindManagedValidationSession},
		ValidationFailureRepairConvergence: &repair,
	}}}
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{valid}); err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(*RuntimeProfileEvidenceRequirement){
		func(v *RuntimeProfileEvidenceRequirement) { v.Scope = "goalCoverage" },
		func(v *RuntimeProfileEvidenceRequirement) {
			v.RequiredEvidenceKinds = []string{EvidenceKindStateChange, EvidenceKindValidation}
		},
	} {
		invalid := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{valid})[0]
		edit(&invalid.EvidenceRequirements[0])
		if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{invalid}); err == nil {
			t.Fatalf("invalid repair terminal accepted: %+v", invalid.EvidenceRequirements[0])
		}
	}
	same := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{valid})[0]
	exact := same.EvidenceRequirements[0]
	exact.ID = "dod-item-1"
	same.EvidenceRequirements = append(same.EvidenceRequirements, exact)
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{same}); err != nil {
		t.Fatalf("identical exact and wildcard repair declarations rejected: %v", err)
	}
	conflict := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{same})[0]
	for index := range conflict.EvidenceRequirements {
		if conflict.EvidenceRequirements[index].ID != "dod-item-1" {
			continue
		}
		changed := conflict.EvidenceRequirements[index].ValidationFailureRepairConvergence.Clone()
		changed.MaxAttempts++
		conflict.EvidenceRequirements[index].ValidationFailureRepairConvergence = &changed
	}
	if err := ValidateRuntimeProfileContracts([]RuntimeProfileContract{conflict}); err == nil || !strings.Contains(err.Error(), "conflicting validation failure repair") {
		t.Fatalf("conflicting exact and wildcard repair declarations err=%v", err)
	}
}

func TestRuntimeValidationFailureRepairConvergenceIsSealedAcrossProfileAndDefinitionSurfaces(t *testing.T) {
	repair := testValidationFailureRepairConvergence()
	profiles := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{{
		ID: "software-dev",
		EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{
			ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{EvidenceKindValidation},
			AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationFailureRepairConvergence: &repair,
		}},
	}})
	profileHash := StableRuntimeProfileContractsHash(profiles)
	profileClone := NormalizeRuntimeProfileContracts(profiles)
	profileClone[0].EvidenceRequirements[0].ValidationFailureRepairConvergence.MaxAttempts = 1
	if profiles[0].EvidenceRequirements[0].ValidationFailureRepairConvergence.MaxAttempts != 2 {
		t.Fatal("profile normalization shared validation repair storage")
	}
	if SameRuntimeProfileContracts(profiles, profileClone) || profileHash == StableRuntimeProfileContractsHash(profileClone) {
		t.Fatal("profile equality/hash ignored validation repair drift")
	}

	coverage := CoverageFocus{
		ID: "focus-1", SliceID: "slice-1", AssessmentScope: "dodItem", AssessmentID: "dod-001",
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindValidation},
		RequireAssessment: true,
	}.Normalized()
	definition := CompileRuntimeDefinitionFocuses([]CoverageFocus{coverage})[0]
	definition.ValidationFailureRepairConvergence = &repair
	definition = definition.Normalized()
	definitionClone := definition.Clone()
	definitionClone.ValidationFailureRepairConvergence.MaxAttempts = 1
	if definition.ValidationFailureRepairConvergence.MaxAttempts != 2 {
		t.Fatal("definition focus clone shared validation repair storage")
	}
	if sameRuntimeDefinitionFocus(definition, definitionClone) {
		t.Fatal("definition focus equality ignored validation repair drift")
	}
	coverageClone := definition.CoverageFocusDeclaration()
	if !sameCoverageFocus(coverage, coverageClone) {
		t.Fatalf("Definition projection changed mutable focus cursor: got=%+v want=%+v", coverageClone, coverage)
	}
	if _, found := reflect.TypeOf(CoverageFocus{}).FieldByName("ValidationFailureRepairConvergence"); found {
		t.Fatal("mutable CoverageFocus must not carry Profile validation repair convergence")
	}
}
