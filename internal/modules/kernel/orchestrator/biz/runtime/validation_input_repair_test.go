package runtime

import (
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestInputRepairFeasibilityCannotLoseOrWidenOwnerPostimage(t *testing.T) {
	repair := &runtimecontract.RuntimeValidationInputRepair{ContractRef: "owner/input-case/v1", RequiredCheckRef: "check", ObservationProtocolRef: "owner/v4", OriginalNativeCheckRef: "original", SupplementalNativeCheckRef: "supplement", TargetRef: "checks/input.test", BeforeHash: strings.Repeat("a", 64), AfterHash: strings.Repeat("b", 64), Corrections: []runtimecontract.RuntimeValidationInputSymbolCorrection{{Line: 3, Column: 9, Before: "expectedvalue", After: "expectedValue"}}}
	manifest := runtimecontract.RuntimeCandidateContractManifest{Operations: []runtimecontract.RuntimeCandidateOperationManifest{{Kind: runtimecontract.RuntimeBlockKindPatchApply, InputRepair: repair}}}
	operation := runtimecontract.RuntimeOperation{ID: "repair", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{repair.TargetRef}}
	proof := runtimecontract.RuntimeOperationReplayProof{OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationID: operation.ID, TargetRefs: operation.TargetRefs, BeforeHash: repair.BeforeHash, AfterHash: repair.AfterHash, AdmissibilitySnapshotID: "catalog"}
	for _, scenario := range []string{"exact", "absent", "changed_assertion", "old_preimage", "other_target"} {
		t.Run(scenario, func(t *testing.T) {
			value := proof.Clone()
			record := runtimecontract.RuntimeCandidateFeasibilityRecord{ReplayProof: &value, Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible}}
			switch scenario {
			case "absent":
				record.ReplayProof = nil
			case "changed_assertion":
				value.AfterHash = strings.Repeat("c", 64)
			case "old_preimage":
				value.BeforeHash = strings.Repeat("c", 64)
			case "other_target":
				value.TargetRefs = []string{"other.test"}
			}
			err := validateCandidateFeasibilityObservationBinding(nil, operation, record, manifest)
			if (err == nil) != (scenario == "exact") {
				t.Fatalf("SM-3 input proof gate: %v", err)
			}
		})
	}
}
