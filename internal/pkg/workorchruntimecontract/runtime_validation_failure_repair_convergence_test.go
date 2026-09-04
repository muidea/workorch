package workorchruntimecontract

import (
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func testValidationFailureRepairConvergence() RuntimeValidationFailureRepairConvergence {
	return RuntimeValidationFailureRepairConvergence{
		ID: "dod-validation-repair",
		Routes: []RuntimeValidationFailureRepairRoute{
			testValidationFailureRepairRoute("implementation-gap-to-file", ValidationDiagnosticImplementationGap, ValidationEvidenceTargetFailureSurface),
			testValidationFailureRepairRoute("build-failure-to-file", ValidationDiagnosticBuildFailed, ValidationEvidenceTargetFailureSurface),
			testValidationFailureRepairRoute("test-failure-to-file", ValidationDiagnosticTestFailed, ValidationEvidenceTargetBehaviorEvidence),
		},
		RequireFreshValidation: true,
		MaxAttempts:            2,
	}
}

func testValidationFailureRepairRoute(id, diagnosticClass, targetType string) RuntimeValidationFailureRepairRoute {
	return RuntimeValidationFailureRepairRoute{
		ID:                                        id,
		TriggerOwnerModuleID:                      RuntimeModuleShell,
		TriggerOperationKind:                      RuntimeBlockKindManagedValidationSession,
		TriggerDiagnosticClass:                    diagnosticClass,
		TriggerEvidenceTargetType:                 targetType,
		TriggerOwnerDiagnosticCode:                "shell_exit_nonzero",
		RepairEvidenceKind:                        EvidenceKindStateChange,
		RepairOwnerModuleID:                       RuntimeModuleFile,
		RepairOperationKinds:                      []string{StepKindFileWrite, RuntimeBlockKindPatchApply},
		RepairTargetPolicy:                        RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		RepairTargetRole:                          "deliverable_implementation",
		RepairDeclarationTargetSelectorRef:        "file/workspace_deliverable",
		RepairDeclarationTargetSelectorOwnerRef:   "software-dev",
		RepairDeclarationTargetSelectorSubjectRef: "dodItem_repair",
		RepairOperationTargetSelectorRef:          "workspace-relative-new-file-target",
	}
}

func TestRuntimeValidationFailureRepairConvergenceIsOrderedAllOfAndStable(t *testing.T) {
	value := testValidationFailureRepairConvergence()
	value.Routes = []RuntimeValidationFailureRepairRoute{value.Routes[2], value.Routes[0], value.Routes[1]}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	normalized := value.Normalized()
	route, found := normalized.RouteForTrigger(RuntimeModuleShell, RuntimeBlockKindManagedValidationSession, ValidationDiagnosticBuildFailed, ValidationEvidenceTargetFailureSurface, "shell_exit_nonzero")
	if len(normalized.Routes) != 3 || !found || route.ID != "build-failure-to-file" {
		t.Fatalf("normalized convergence = %+v", normalized)
	}
	clone := normalized.Clone()
	clone.Routes[0].TriggerOwnerDiagnosticCode = "different-owner-code"
	if normalized.Routes[0].TriggerOwnerDiagnosticCode == clone.Routes[0].TriggerOwnerDiagnosticCode {
		t.Fatal("convergence clone shares route storage")
	}
	if StableRuntimeValidationFailureRepairConvergenceHash(normalized) == StableRuntimeValidationFailureRepairConvergenceHash(clone) {
		t.Fatal("convergence hash ignored exact route drift")
	}
	if _, found := normalized.RouteForTrigger(RuntimeModuleShell, RuntimeBlockKindManagedValidationSession, ValidationDiagnosticBuildFailed, ValidationEvidenceTargetFailureSurface, "another-code"); found {
		t.Fatal("convergence accepted a non-exact owner diagnostic code")
	}
}

func TestRuntimeValidationFailureRepairRouteRequiresObservationOnlyForExistingScope(t *testing.T) {
	prospective := testValidationFailureRepairRoute("prospective", ValidationDiagnosticBuildFailed, ValidationEvidenceTargetFailureSurface)
	if prospective.RequiresStructuredObservation() {
		t.Fatal("prospective owner target unexpectedly requires a preexisting observation")
	}
	existing := prospective.Clone()
	existing.RepairTargetPolicy = RuntimeOperationTargetPolicyExistingOwnerScope
	if !existing.RequiresStructuredObservation() {
		t.Fatal("existing owner scope did not require a structured owner observation")
	}
}

func TestRuntimeValidationFailureRepairConvergenceRejectsIncompletePresealContract(t *testing.T) {
	valid := testValidationFailureRepairConvergence()
	for _, tc := range []struct {
		name string
		want string
		edit func(*RuntimeValidationFailureRepairConvergence)
	}{
		{name: "missing routes", want: "unique exact routes", edit: func(v *RuntimeValidationFailureRepairConvergence) { v.Routes = nil }},
		{name: "missing diagnostic", want: "exact typed trigger", edit: func(v *RuntimeValidationFailureRepairConvergence) { v.Routes[0].TriggerDiagnosticClass = "" }},
		{name: "unknown diagnostic", want: "unknown diagnostic", edit: func(v *RuntimeValidationFailureRepairConvergence) {
			v.Routes[0].TriggerDiagnosticClass = "free_form_failure"
		}},
		{name: "non validation trigger", want: "not validation proof", edit: func(v *RuntimeValidationFailureRepairConvergence) {
			v.Routes[0].TriggerOwnerModuleID = RuntimeModuleFile
			v.Routes[0].TriggerOperationKind = StepKindFileWrite
		}},
		{name: "non state repair", want: "state_change", edit: func(v *RuntimeValidationFailureRepairConvergence) {
			v.Routes[0].RepairEvidenceKind = EvidenceKindValidation
		}},
		{name: "cross owner repair", want: "not state-change proof", edit: func(v *RuntimeValidationFailureRepairConvergence) {
			v.Routes[0].RepairOperationKinds = []string{StepKindShellCommand}
		}},
		{name: "exact policy", want: "selector-backed", edit: func(v *RuntimeValidationFailureRepairConvergence) {
			v.Routes[0].RepairTargetPolicy = RuntimeOperationTargetPolicyExactTarget
		}},
		{name: "missing operation selector", want: "complete selector", edit: func(v *RuntimeValidationFailureRepairConvergence) { v.Routes[0].RepairOperationTargetSelectorRef = "" }},
		{name: "aliased declaration and operation selectors", want: "distinct declaration and operation selector", edit: func(v *RuntimeValidationFailureRepairConvergence) {
			v.Routes[0].RepairOperationTargetSelectorRef = v.Routes[0].RepairDeclarationTargetSelectorRef
		}},
		{name: "duplicate exact trigger", want: "duplicate exact trigger", edit: func(v *RuntimeValidationFailureRepairConvergence) {
			duplicate := v.Routes[0].Clone()
			duplicate.ID = "duplicate-trigger"
			v.Routes = append(v.Routes, duplicate)
		}},
		{name: "duplicate route identity", want: "duplicate route identity", edit: func(v *RuntimeValidationFailureRepairConvergence) {
			duplicate := v.Routes[0].Clone()
			duplicate.TriggerDiagnosticClass = ValidationDiagnosticEnvironmentMissing
			duplicate.TriggerEvidenceTargetType = ValidationEvidenceTargetEnvironment
			duplicate.TriggerOwnerDiagnosticCode = "command_not_found"
			v.Routes = append(v.Routes, duplicate)
		}},
		{name: "empty route entry", want: "non-empty unique route identities", edit: func(v *RuntimeValidationFailureRepairConvergence) {
			v.Routes = append(v.Routes, RuntimeValidationFailureRepairRoute{})
		}},
		{name: "owner without confirmation protocol", want: "confirmation protocol", edit: func(v *RuntimeValidationFailureRepairConvergence) {
			v.Routes[0].RepairOwnerModuleID = RuntimeModuleGit
			v.Routes[0].RepairOperationKinds = []string{StepKindGitWrite}
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
