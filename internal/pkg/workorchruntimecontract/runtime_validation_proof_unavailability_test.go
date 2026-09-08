package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestValidationProofUnavailabilityIsExactDetachedAndNeverPreparation(t *testing.T) {
	requirement := runtimeValidationProofRequirementForTest(t, "definition", RuntimeCoverageItem{}, "", "recipe", nil)
	surface := runtimeExactProofCloneReadySurfaceFixture("recipe")
	surface = bindRuntimeValidationSurfaceForTest(surface, requirement)
	catalog := RuntimeOperationAdmissibility{PrerequisiteID: "catalog", OwnerModuleID: RuntimeModuleShell, OperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ObservedRevision: "owner-revision", ValidationSurfaces: []RuntimeValidationSurface{surface}, ValidationRecipeRefs: []string{"recipe"}}
	legacyJSON, _ := json.Marshal(catalog)
	legacyHash := StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if strings.Contains(string(legacyJSON), "ProofRequirementUnavailabilities") {
		t.Fatal("empty extension changed archive JSON")
	}
	var restored RuntimeOperationAdmissibility
	if err := json.Unmarshal(legacyJSON, &restored); err != nil || StableRuntimeOperationAdmissibilitySnapshotID(restored) != legacyHash {
		t.Fatal("legacy catalog round trip changed identity")
	}
	unavailable := RuntimeValidationProofUnavailability{Requirement: requirement, ValidationRecipeID: "recipe", OriginalDiagnostic: &RuntimeOriginalDiagnostic{SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: RuntimeModuleShell, ObservedRevision: "owner-revision", OriginalClass: "validation_readiness", OriginalCode: "owner_input_invalid", OriginalMessage: "  exact owner cause  ", Signature: "owner-input/cwi", TargetRefs: []string{"checks/input"}}}
	surface.ProofRequirementBindings, surface.CheckRefs = nil, nil
	surface.ProofRequirementUnavailabilities = []RuntimeValidationProofUnavailability{unavailable}
	catalog.ValidationSurfaces, catalog.ValidationRecipeRefs = []RuntimeValidationSurface{surface}, nil
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeValidationProofRequirementCatalogApplicability([]RuntimeValidationProofRequirement{requirement}, []RuntimeOperationAdmissibility{catalog}); err != nil {
		t.Fatal(err)
	}
	ownerRequirement := RuntimeOwnerFactRequirement{OwnerModuleID: RuntimeModuleShell, FactKind: RuntimeOwnerFactSelectableValidationSurface, OperationKind: RuntimeBlockKindManagedValidationSession, ValidationProofRequirementHash: requirement.Hash}
	convergence := RuntimeOwnerFactConvergence{
		Requirement: ownerRequirement, MissingTrigger: RuntimeOwnerFactMissingTrigger,
		ProspectiveSatisfactionContractRef: "owner/prospective/v1",
		AcquisitionOwnerModuleID:           RuntimeModuleFile, AcquisitionOperationKind: StepKindFileWrite,
		AcquisitionTargetPolicy: RuntimeOperationTargetPolicyProspectiveOwnerTarget, AcquisitionTargetSelectorRef: "workspace-relative-new-file-target",
		TargetSelectorRef: "file/workspace_deliverable", TargetSelectorOwnerRef: "profile/test", TargetSelectorSubjectRef: "validation_preparation",
		ReobserveOwnerModuleID: RuntimeModuleShell, ReobserveFactKind: RuntimeOwnerFactSelectableValidationSurface, MaxAcquisitionAttempts: 1,
	}
	if _, found := ResolveRuntimeOwnerFactAcquisition([]RuntimeOwnerFactRequirement{ownerRequirement}, []RuntimeOwnerFactConvergence{convergence}, nil); !found {
		t.Fatal("fixture has no declared acquisition")
	}
	if _, found := ResolveRuntimeOwnerFactAcquisition([]RuntimeOwnerFactRequirement{ownerRequirement}, []RuntimeOwnerFactConvergence{convergence}, []RuntimeOperationAdmissibility{catalog}); found {
		t.Fatal("unavailability was compiled into another owner-fact acquisition")
	}
	copy := cloneRuntimeValidationSurfaceRaw(surface)
	if !reflect.DeepEqual(copy, surface) {
		t.Fatal("raw clone changed unavailable record")
	}
	copy.ProofRequirementUnavailabilities[0].OriginalDiagnostic.TargetRefs[0] = "other"
	copy.ProofRequirementUnavailabilities[0].Requirement.RequiredCheckRefs[0] = "other"
	if surface.ProofRequirementUnavailabilities[0].OriginalDiagnostic.TargetRefs[0] != "checks/input" || surface.ProofRequirementUnavailabilities[0].Requirement.RequiredCheckRefs[0] != requirement.RequiredCheckRefs[0] {
		t.Fatal("raw clone aliases owner fact")
	}
	for name, mutate := range map[string]func(*RuntimeValidationSurface){
		"duplicate": func(s *RuntimeValidationSurface) {
			s.ProofRequirementUnavailabilities = append(s.ProofRequirementUnavailabilities, unavailable)
		},
		"ready and unavailable": func(s *RuntimeValidationSurface) {
			s.ProofRequirementBindings = bindRuntimeValidationSurfaceForTest(*s, requirement).ProofRequirementBindings
		},
		"foreign owner": func(s *RuntimeValidationSurface) {
			s.ProofRequirementUnavailabilities[0].OriginalDiagnostic.OwnerModuleID = RuntimeModuleFile
		},
		"stale revision": func(s *RuntimeValidationSurface) {
			s.ProofRequirementUnavailabilities[0].OriginalDiagnostic.ObservedRevision = "stale"
		},
		"requirement drift": func(s *RuntimeValidationSurface) {
			s.ProofRequirementUnavailabilities[0].Requirement.DefinitionRevision = "stale"
		},
		"missing cause": func(s *RuntimeValidationSurface) { s.ProofRequirementUnavailabilities[0].OriginalDiagnostic = nil },
	} {
		t.Run(name, func(t *testing.T) {
			changed := catalog.Normalized()
			mutate(&changed.ValidationSurfaces[0])
			changed.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(changed)
			if changed.Validate() == nil {
				t.Fatal("invalid unavailability accepted")
			}
		})
	}
	changed := catalog.Normalized()
	changed.ValidationSurfaces[0].ProofRequirementUnavailabilities[0].OriginalDiagnostic.OriginalCode = "different"
	if StableRuntimeOperationAdmissibilitySnapshotID(changed) == catalog.SnapshotID {
		t.Fatal("catalog identity omitted owner cause")
	}
}
