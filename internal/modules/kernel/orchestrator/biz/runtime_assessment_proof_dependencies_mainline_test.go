package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 真实 Shell producer 的既有 mainline；仅检查选中 proof 来源，不把复制集合称为全部 assessment 依赖。
func assertActualShellAssessmentProofDependencies(t *testing.T, record runtimecontract.RuntimeStateRecord) {
	t.Helper()
	material, err := runtimecontract.DeriveRuntimeTerminalResultMaterialFromState(record)
	if err != nil {
		t.Logf("terminal currentness fence: evidenceRevision=%s generation=%s observations=%+v", record.Facts.EvidenceRevision, record.Reconciliation.RequestHash, record.Facts.EvidenceApplicabilityObservations)
		t.Fatalf("actual owner terminal material: %v", err)
	}
	terminalMembers := 0
	for _, outcome := range material.RuntimeCloseout.RequirementOutcomes {
		for _, item := range outcome.EvidenceItems {
			for _, ledger := range [][]runtimecontract.EvidenceFact{record.Facts.Evidence.Accepted, record.Facts.Evidence.Rejected} {
				for _, fact := range ledger {
					if string(fact.ID) != item.EvidenceRef {
						continue
					}
					if !reflect.DeepEqual(item.OwnerRevisionSet, fact.OwnerRevisionSet) {
						t.Fatal("terminal material changed actual owner receipt/fact read snapshot")
					}
					if fact.OwnerRevisionSet != nil {
						terminalMembers++
					}
				}
			}
		}
	}
	if terminalMembers == 0 {
		t.Fatal("terminal material has no actual owner read dependency")
	}
	if record.TerminalMaterialSeal != nil &&
		!reflect.DeepEqual(record.TerminalMaterialSeal.Material.RuntimeCloseout.RequirementOutcomes, material.RuntimeCloseout.RequirementOutcomes) {
		t.Fatal("terminal seal changed original per-evidence read snapshots")
	}
	fileMembers := 0
	originalFile := sha256.Sum256([]byte("package fixture\nfunc DeliveredValue() int { return 2 }\n"))
	for _, fact := range record.Facts.Evidence.Accepted {
		if fact.Kind != runtimecontract.EvidenceKindStateChange || !reflect.DeepEqual(fact.TargetRefs, []string{"subject.go"}) {
			continue
		}
		if fact.OwnerRevisionSet == nil || fact.OwnerRevisionSet.Validate() != nil || fact.OwnerRevisionSet.DefinitionRevision != record.Facts.DefinitionRevision || len(fact.OwnerRevisionSet.Entries) != 1 {
			t.Fatal("actual File edit fact lost exact owner dependencies")
		}
		entry := fact.OwnerRevisionSet.Entries[0]
		if entry.OwnerKind != "capability" || entry.OwnerInstanceID != runtimecontract.RuntimeModuleFile+"/"+string(record.Facts.WorkspaceID) || entry.ResourceKind != string(runtimecontract.RuntimeResourceWorkspaceTarget) || entry.ResourceID != "subject.go" || entry.Revision != hex.EncodeToString(originalFile[:]) {
			t.Fatal("actual File edit fact lost independently known original input bytes")
		}
		matches := 0
		for _, source := range record.CompletedDispatches {
			if source.ID != fact.DispatchID {
				continue
			}
			for _, execution := range source.OperationExecutions {
				if execution.OperationID != fact.OperationID {
					continue
				}
				fileOwned := false
				for _, catalog := range source.OperationAdmissibilities {
					if catalog.SnapshotID == execution.AdmissibilitySnapshotID && catalog.OwnerModuleID == runtimecontract.RuntimeModuleFile {
						fileOwned = true
					}
				}
				if !fileOwned {
					continue
				}
				for _, receipt := range record.OperationReceipts {
					if receipt.ExecutionKey == execution.ExecutionKey && receipt.Fence == execution.Fence {
						if receipt.OperationID != fact.OperationID || execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) || !reflect.DeepEqual(receipt.Evidence.OwnerRevisionSet, fact.OwnerRevisionSet) {
							t.Fatal("actual File mutation dependency differs from sealed owner receipt")
						}
						matches++
					}
				}
			}
		}
		if matches != 1 {
			t.Fatalf("actual File mutation has no unique sealed owner receipt: matches=%d", matches)
		}
		fileMembers++
	}
	if fileMembers != 1 {
		t.Fatalf("actual mainline did not exercise unique File mutation dependency: members=%d", fileMembers)
	}
	members, ownerFactMembers := 0, 0
	for _, dispatch := range record.CompletedDispatches {
		if dispatch.AssessmentAdjudication == nil {
			continue
		}
		if dispatch.AssessmentAuthorityInput == nil || len(dispatch.AssessmentAdjudication.OwnerFactSources) != len(dispatch.AssessmentAuthorityInput.Focus.RequiredOwnerFacts) {
			t.Fatal("actual assessment lost required owner postcondition sources")
		}
		for _, source := range dispatch.AssessmentAdjudication.OwnerFactSources {
			matches := 0
			for _, catalog := range dispatch.AssessmentAuthorityInput.OperationAdmissibilities {
				if catalog.SnapshotID == source.CatalogSnapshotID && catalog.OwnerModuleID == source.Requirement.OwnerModuleID {
					matches++
				}
			}
			if matches != 1 || source.OwnerRevisionSet == nil || source.OwnerRevisionSet.Validate() != nil || source.OwnerRevisionSet.DefinitionRevision != record.Facts.DefinitionRevision || len(source.OwnerRevisionSet.Entries) != 1 || source.OwnerRevisionSet.Entries[0].OwnerInstanceID != source.Requirement.OwnerModuleID+"/"+string(record.Facts.WorkspaceID) {
				t.Fatal("actual owner postcondition source differs from sealed catalog")
			}
			ownerFactMembers++
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
	if members == 0 || ownerFactMembers == 0 {
		t.Fatal("actual Shell mainline did not exercise proof and owner postcondition dependency projection")
	}
}
