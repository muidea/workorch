package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func testValidationInputRepair() *RuntimeValidationInputRepair {
	return &RuntimeValidationInputRepair{ContractRef: "owner/input-case/v1", RequiredCheckRef: "check", ObservationProtocolRef: "owner/v4", OriginalNativeCheckRef: "original", SupplementalNativeCheckRef: "supplement", TargetRef: "checks/input.test", BeforeHash: strings.Repeat("a", 64), AfterHash: strings.Repeat("b", 64), Corrections: []RuntimeValidationInputSymbolCorrection{{Line: 3, Column: 9, Before: "expectedvalue", After: "expectedValue"}}}
}

func TestValidationInputRepairRequiresExactBindingDeclarationAndPostimage(t *testing.T) {
	repair := testValidationInputRepair()
	binding := CoverageEvidenceBinding{Proofs: []CoverageEvidenceProof{{EvidenceKind: EvidenceKindValidation, ValidationCheckAttestations: []RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "check", ObservationProtocolRef: "owner/v4", NativeCheckRef: "original", SupplementalNativeCheckRef: "supplement"}}}}}
	diagnostic := ValidationFailureDiagnostic{InputRepair: repair, TargetRefs: []string{repair.TargetRef}, InputLocations: []RuntimeValidationInputLocation{{TargetRef: repair.TargetRef, Line: 3, Column: 9, Code: "undefined_identifier", Symbol: "expectedvalue"}}}
	route := testValidationFailureRepairConvergence().Routes[0].Clone()
	route.RequireInputRepair, route.RepairTargetPolicy = true, RuntimeOperationTargetPolicyExistingOwnerScope
	route.RepairOperationTargetSelectorRef = "workspace-relative-existing-file-target"
	route.RepairOperationKinds = []string{RuntimeBlockKindPatchApply}
	if repair.Validate() != nil || validateRuntimeValidationInputDiagnostic(diagnostic) != nil || route.Validate() != nil || !route.AcceptsInputDiagnostic(diagnostic, binding) {
		t.Fatalf("exact input repair refused: repair=%v diagnostic=%v route=%v matches=%v", repair.Validate(), validateRuntimeValidationInputDiagnostic(diagnostic), route.Validate(), route.AcceptsInputDiagnostic(diagnostic, binding))
	}
	proof := &RuntimeOperationReplayProof{OwnerModuleID: RuntimeModuleFile, OperationID: "repair", TargetRefs: diagnostic.TargetRefs, BeforeHash: repair.BeforeHash, AfterHash: repair.AfterHash, AdmissibilitySnapshotID: "catalog"}
	if !repair.MatchesReplayProof(proof) {
		t.Fatal("exact pre/postimage refused")
	}
	for _, mutate := range []func(*RuntimeValidationInputRepair){
		func(v *RuntimeValidationInputRepair) { v.RequiredCheckRef = "other" },
		func(v *RuntimeValidationInputRepair) { v.OriginalNativeCheckRef = "other" },
		func(v *RuntimeValidationInputRepair) { v.SupplementalNativeCheckRef = "other" },
		func(v *RuntimeValidationInputRepair) { v.ObservationProtocolRef = "owner/v3" },
	} {
		changed := repair.Clone()
		mutate(changed)
		if changed.MatchesBinding(binding) {
			t.Fatal("different acceptance identity matched")
		}
	}
	for _, field := range []string{"before", "after", "target"} {
		changed := proof.Clone()
		switch field {
		case "before":
			changed.BeforeHash = strings.Repeat("c", 64)
		case "after":
			changed.AfterHash = strings.Repeat("c", 64)
		case "target":
			changed.TargetRefs = []string{"other.test"}
		}
		if repair.MatchesReplayProof(&changed) {
			t.Fatal("stale or widened postimage admitted")
		}
	}
	ordinary := route
	ordinary.RequireInputRepair = false
	if ordinary.AcceptsInputDiagnostic(diagnostic, binding) {
		t.Fatal("input repair borrowed implementation route")
	}
	missing := diagnostic.Clone()
	missing.InputRepair = nil
	if route.AcceptsInputDiagnostic(missing, binding) {
		t.Fatal("location-only diagnostic granted write authority")
	}
	widened := diagnostic.Clone()
	widened.TargetRefs = append(widened.TargetRefs, "other.test")
	if route.AcceptsInputDiagnostic(widened, binding) {
		t.Fatal("input target set widened")
	}
	for _, mutate := range []func(*RuntimeValidationFailureRepairRoute){
		func(v *RuntimeValidationFailureRepairRoute) { v.RequireInputAddition = true },
		func(v *RuntimeValidationFailureRepairRoute) {
			v.RepairTargetPolicy = RuntimeOperationTargetPolicyProspectiveOwnerTarget
		},
		func(v *RuntimeValidationFailureRepairRoute) { v.RepairOperationKinds = []string{StepKindFileEdit} },
	} {
		changed := route.Clone()
		mutate(&changed)
		if changed.Validate() == nil {
			t.Fatal("repair route bypassed exact owner replay proof")
		}
	}
}

func TestValidationInputRepairRejectsUnboundedOrAmbiguousFacts(t *testing.T) {
	for _, mutate := range []func(*RuntimeValidationInputRepair){
		func(v *RuntimeValidationInputRepair) { v.Corrections = nil },
		func(v *RuntimeValidationInputRepair) { v.Corrections = append(v.Corrections, v.Corrections[0]) },
		func(v *RuntimeValidationInputRepair) { v.Corrections[0].Line = 0 },
		func(v *RuntimeValidationInputRepair) { v.Corrections[0].Before = "x\ncode" },
		func(v *RuntimeValidationInputRepair) { v.Corrections[0].After = v.Corrections[0].Before },
		func(v *RuntimeValidationInputRepair) { v.BeforeHash = strings.Repeat("A", 64) },
		func(v *RuntimeValidationInputRepair) { v.AfterHash = v.BeforeHash },
		func(v *RuntimeValidationInputRepair) { v.TargetRef = "../outside" },
		func(v *RuntimeValidationInputRepair) { v.ContractRef = "owner\tcontract" },
	} {
		changed := testValidationInputRepair()
		mutate(changed)
		if changed.Validate() == nil {
			t.Fatalf("malformed input repair accepted: %+v", changed)
		}
	}
	diagnostic := ValidationFailureDiagnostic{InputRepair: testValidationInputRepair()}
	if validateRuntimeValidationInputDiagnostic(diagnostic) == nil {
		t.Fatal("repair without compiler locations accepted")
	}
}

func TestValidationInputRepairHashesCloneAndLegacyEmptyCompatibility(t *testing.T) {
	for _, legacy := range []any{ValidationFailureDiagnostic{}, RuntimeCandidateOperationManifest{}, RuntimeDispatchRepairCommitment{}, RuntimeValidationFailureRepairRoute{}} {
		encoded, err := json.Marshal(legacy)
		if err != nil || strings.Contains(strings.ToLower(string(encoded)), "inputrepair") || strings.Contains(string(encoded), "inputLocations") {
			t.Fatal("empty extension changed historical JSON")
		}
	}
	before, after := sha256.New(), sha256.New()
	writeHashString(before, "legacy")
	writeHashString(after, "legacy")
	writeRuntimeValidationInputDiagnosticHash(after, ValidationFailureDiagnostic{})
	writeRuntimeValidationInputRepairHash(after, nil)
	if !reflect.DeepEqual(before.Sum(nil), after.Sum(nil)) {
		t.Fatal("empty extension changed old hash bytes")
	}
	repair := testValidationInputRepair()
	diagnostic := ValidationFailureDiagnostic{InputRepair: repair, InputLocations: []RuntimeValidationInputLocation{{TargetRef: repair.TargetRef, Line: 3, Column: 9, Code: "undefined_identifier", Symbol: "expectedvalue"}}}
	clone := cloneValidationFailureDiagnosticRaw(diagnostic)
	clone.InputRepair.Corrections[0].After = "changed"
	clone.InputLocations[0].Line++
	if SameValidationFailureDiagnostic(&diagnostic, &clone) || diagnostic.InputRepair.Corrections[0].After != "expectedValue" || diagnostic.InputLocations[0].Line != 3 {
		t.Fatal("clone aliases or ignores original diagnostic")
	}
	for _, mutate := range []func(*RuntimeValidationInputRepair){
		func(v *RuntimeValidationInputRepair) { v.ContractRef += "changed" }, func(v *RuntimeValidationInputRepair) { v.RequiredCheckRef += "changed" },
		func(v *RuntimeValidationInputRepair) { v.ObservationProtocolRef += "changed" }, func(v *RuntimeValidationInputRepair) { v.OriginalNativeCheckRef += "changed" },
		func(v *RuntimeValidationInputRepair) { v.SupplementalNativeCheckRef += "changed" }, func(v *RuntimeValidationInputRepair) { v.TargetRef += "changed" },
		func(v *RuntimeValidationInputRepair) { v.BeforeHash = strings.Repeat("c", 64) }, func(v *RuntimeValidationInputRepair) { v.AfterHash = strings.Repeat("c", 64) },
		func(v *RuntimeValidationInputRepair) { v.Corrections[0].Line++ }, func(v *RuntimeValidationInputRepair) { v.Corrections[0].Column++ },
		func(v *RuntimeValidationInputRepair) { v.Corrections[0].Before += "changed" }, func(v *RuntimeValidationInputRepair) { v.Corrections[0].After += "changed" },
	} {
		changed := repair.Clone()
		mutate(changed)
		originalManifest := RuntimeCandidateContractManifest{Operations: []RuntimeCandidateOperationManifest{{Kind: RuntimeBlockKindPatchApply, InputRepair: repair}}}
		changedManifest := RuntimeCandidateContractManifest{Operations: []RuntimeCandidateOperationManifest{{Kind: RuntimeBlockKindPatchApply, InputRepair: changed}}}
		if StableRuntimeCandidateContractManifestHash(originalManifest) == StableRuntimeCandidateContractManifestHash(changedManifest) ||
			StableRuntimeDispatchRepairCommitmentHash(RuntimeDispatchRepairCommitment{InputRepair: repair}) == StableRuntimeDispatchRepairCommitmentHash(RuntimeDispatchRepairCommitment{InputRepair: changed}) {
			t.Fatal("input repair escaped authority hashes")
		}
		original, drifted := diagnostic.Clone(), diagnostic.Clone()
		drifted.InputRepair = changed
		a, b := sha256.New(), sha256.New()
		writeRuntimeValidationInputDiagnosticHash(a, original)
		writeRuntimeValidationInputDiagnosticHash(b, drifted)
		if reflect.DeepEqual(a.Sum(nil), b.Sum(nil)) {
			t.Fatal("input repair escaped receipt diagnostic hash")
		}
	}
}
