package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPreparationDiagnosticIsOptionalHashBoundAndDetached(t *testing.T) {
	surface := runtimeExactProofClonePreparingSurfaceFixture("recipe").Normalized()
	catalog := RuntimeOperationAdmissibility{PrerequisiteID: "catalog", OwnerModuleID: RuntimeModuleShell,
		ObservedRevision: "shell-policy-v1", OperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationSurfaces: []RuntimeValidationSurface{surface}}
	legacyHash := StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	legacyJSON, err := json.Marshal(catalog)
	if err != nil || strings.Contains(string(legacyJSON), "OriginalDiagnostic") {
		t.Fatal("legacy JSON gained a diagnostic field")
	}
	preparation := &catalog.ValidationSurfaces[0].ProofRequirementPreparations[0]
	preparation.OriginalDiagnostic = &RuntimeOriginalDiagnostic{SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: RuntimeModuleShell,
		OriginalClass: "validation_readiness", OriginalCode: "check_missing", OriginalMessage: "  exact owner message  ", Signature: "check-missing/1",
		ObservedRevision: "shell-policy-v1", TargetRefs: []string{"checks/a.test"}}
	if err := preparation.Validate(); err != nil {
		t.Fatal(err)
	}
	currentHash := StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if currentHash == legacyHash {
		t.Fatal("catalog hash omitted original diagnostic")
	}
	clone := catalog.Normalized()
	clone.ValidationSurfaces[0].ProofRequirementPreparations[0].OriginalDiagnostic.TargetRefs[0] = "other"
	if preparation.OriginalDiagnostic.TargetRefs[0] != "checks/a.test" || StableRuntimeOperationAdmissibilitySnapshotID(clone) == currentHash {
		t.Fatal("diagnostic clone/hash lost exact targets")
	}
	for _, owner := range []string{RuntimeModuleFile, ""} {
		bad := preparation.Clone()
		bad.OriginalDiagnostic.OwnerModuleID = owner
		if bad.Validate() == nil {
			t.Fatal("cross-owner diagnostic accepted")
		}
	}
	bad := preparation.Clone()
	bad.OriginalDiagnostic.ObservedRevision = "stale"
	if bad.Validate() == nil {
		t.Fatal("stale diagnostic accepted")
	}
	var restored RuntimeOperationAdmissibility
	if err := json.Unmarshal(legacyJSON, &restored); err != nil {
		t.Fatal(err)
	}
	if StableRuntimeOperationAdmissibilitySnapshotID(restored) != legacyHash {
		t.Fatal("legacy snapshot changed after round trip")
	}
	terminal := RuntimeDecision{Terminal: &RuntimeTerminalDecision{Code: "failed", OriginalDiagnostic: preparation.OriginalDiagnostic}}
	copy := cloneRuntimeDecision(terminal)
	if !reflect.DeepEqual(copy.Terminal, terminal.Terminal) {
		t.Fatal("decision clone normalized owner text")
	}
	copy.Terminal.OriginalDiagnostic.TargetRefs[0] = "changed"
	if terminal.Terminal.OriginalDiagnostic.TargetRefs[0] == "changed" {
		t.Fatal("decision aliases terminal cause")
	}
}
