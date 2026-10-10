package orchestrator

import (
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 真实 Shell producer 的既有 mainline；仅检查选中 proof 来源，不把复制集合称为全部 assessment 依赖。
func assertActualShellAssessmentProofDependencies(t *testing.T, record runtimecontract.RuntimeStateRecord) {
	t.Helper()
	members := 0
	for _, dispatch := range record.CompletedDispatches {
		if dispatch.AssessmentAdjudication == nil {
			continue
		}
		for _, proof := range dispatch.AssessmentAdjudication.Proofs {
			if proof.EvidenceKind != runtimecontract.EvidenceKindValidation {
				continue
			}
			var selected *runtimecontract.EvidenceFact
			for i := range record.Facts.Evidence.Accepted {
				if record.Facts.Evidence.Accepted[i].ID == proof.EvidenceFactID {
					if selected != nil {
						t.Fatal("actual assessment proof has duplicate evidence source")
					}
					selected = &record.Facts.Evidence.Accepted[i]
				}
			}
			if selected == nil || selected.OwnerRevisionSet == nil || len(selected.OwnerRevisionSet.Entries) == 0 ||
				proof.EvidenceFactHash != runtimecontract.StableRuntimeEvidenceFactHash(*selected) ||
				!reflect.DeepEqual(proof.OwnerRevisionSet, selected.OwnerRevisionSet) {
				t.Fatal("actual Shell assessment lost selected fact dependencies")
			}
			var receipt *runtimecontract.RuntimeOperationReceipt
			for _, source := range record.CompletedDispatches {
				if source.ID != selected.DispatchID {
					continue
				}
				for _, execution := range source.OperationExecutions {
					if execution.OperationID != selected.OperationID {
						continue
					}
					shellOwned := false
					for _, catalog := range source.OperationAdmissibilities {
						if catalog.SnapshotID == execution.AdmissibilitySnapshotID && catalog.OwnerModuleID == runtimecontract.RuntimeModuleShell {
							shellOwned = true
						}
					}
					if !shellOwned {
						continue
					}
					for i := range record.OperationReceipts {
						value := &record.OperationReceipts[i]
						if value.ExecutionKey == execution.ExecutionKey && value.Fence == execution.Fence {
							if receipt != nil {
								t.Fatal("actual assessment proof has duplicate owner receipt")
							}
							receipt = value
							if execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(*value) {
								t.Fatal("actual assessment proof receipt hash mismatch")
							}
						}
					}
				}
			}
			if receipt == nil || !reflect.DeepEqual(receipt.Evidence.OwnerRevisionSet, proof.OwnerRevisionSet) || proof.OwnerRevisionSet.DefinitionRevision != record.Facts.DefinitionRevision {
				t.Fatal("actual assessment proof differs from original Shell receipt")
			}
			members++
		}
	}
	if members == 0 {
		t.Fatal("actual Shell mainline did not exercise assessment dependency projection")
	}
}
