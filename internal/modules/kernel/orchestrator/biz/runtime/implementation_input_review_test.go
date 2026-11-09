package runtime

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func implementationInputPostimageFixture(t *testing.T) RuntimeAggregate {
	t.Helper()
	a, retained, err := retainRuntimeOwnerInputRevision(ownerInputMaintenanceGuardFixture(t))
	if err != nil || !retained {
		t.Fatal("original structural source not admitted", err)
	}
	v := a.SemanticPlanning.History[0].OwnerInputRevision
	a.ImplementationWork.Rounds = []rc.RuntimeImplementationWorkRound{{
		CoverageWorkItemID: v.Binding.CoverageWorkItemID,
		ScopeRequest:       rc.RuntimeImplementationWorkScopeRequest{InputRevision: v.Clone()},
		DispatchID:         "completed-input-work", CompletedReceiptSetHash: "original-complete-receipts",
	}}
	a.PrerequisiteInvalidations = []rc.RuntimePrerequisiteInvalidationRecord{{DispatchID: "completed-input-work", ResourceChanges: []rc.RuntimeResourceChange{{OwnerModuleID: v.OwnerModuleID, SubjectRef: v.InputFiles()[0].TargetRef, BeforeRevision: v.InputFiles()[0].ContentHash, AfterRevision: "actual-new-postimage"}}}}
	return a
}

// This test isolates current demand projection, not source admission. The
// real-owner incremental mainline supplies the full effect/report/CAS proof.
func TestImplementationInputPostimageReviewDoesNotReopenConsumedAuthority(t *testing.T) {
	a := implementationInputPostimageFixture(t)
	before := a.CloneRaw()
	for range 2 {
		pending, err := rc.PendingRuntimeValidationInputRevision(a.SemanticPlanning, a.ImplementationWork, a.Facts.Focus.CoverageWorkItemIDs[0])
		if err != nil || pending != nil {
			t.Fatal("completed source was made pending again", err)
		}
		need, err := currentImplementationInputReview(a)
		if err != nil || need == nil || need.coverageID != a.Facts.Focus.CoverageWorkItemIDs[0] {
			t.Fatal("new postimage lost its independent review responsibility", err)
		}
		if RuntimeImplementationWorkPurposePending(a) {
			t.Fatal("new purpose preempted current input review")
		}
		if request, err := NextRuntimeImplementationWorkScopeRequest(a); err != nil || request != nil {
			t.Fatal("ordinary scope excluded the input before its fresh review", err)
		}
	}
	if !reflect.DeepEqual(before, a) {
		t.Fatal("read projection changed receipts, source, budget or authority")
	}
}

func TestImplementationInputPostimageReviewUsesActualAffectedOwnerEdges(t *testing.T) {
	for name, mutate := range map[string]func(*RuntimeAggregate){
		"no change":       func(a *RuntimeAggregate) { a.PrerequisiteInvalidations = nil },
		"unrelated input": func(a *RuntimeAggregate) { a.PrerequisiteInvalidations[0].ResourceChanges[0].SubjectRef = "unrelated" },
		"other owner": func(a *RuntimeAggregate) {
			a.PrerequisiteInvalidations[0].ResourceChanges[0].OwnerModuleID = "other/owner"
		},
		"other dispatch": func(a *RuntimeAggregate) { a.PrerequisiteInvalidations[0].DispatchID = "unrelated-dispatch" },
		"unchanged edge": func(a *RuntimeAggregate) {
			c := &a.PrerequisiteInvalidations[0].ResourceChanges[0]
			c.AfterRevision = c.BeforeRevision
		},
		"other CWI": func(a *RuntimeAggregate) {
			a.Facts.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"other-cwi"}
		},
		"unfinished work": func(a *RuntimeAggregate) { a.ImplementationWork.Rounds[0].CompletedReceiptSetHash = "" },
		"failed work":     func(a *RuntimeAggregate) { a.ImplementationWork.Rounds[0].FailedReceiptHash = "original-failure" },
		"active dispatch": func(a *RuntimeAggregate) { a.ActiveDispatch = &rc.RuntimeDispatchRecord{} },
		"formal failure":  func(a *RuntimeAggregate) { a.Facts.Failure = &rc.FailureFact{} },
		"exact repair":    func(a *RuntimeAggregate) { a.Facts.RepairDirective = &rc.RuntimeRepairDirective{} },
		"failure review": func(a *RuntimeAggregate) {
			a.SemanticPlanning.FailureInputReview = &rc.RuntimeValidationFailureInputReview{}
		},
		"termination": func(a *RuntimeAggregate) { a.Termination = &rc.RuntimeTerminationRecord{} },
	} {
		t.Run(name, func(t *testing.T) {
			a := implementationInputPostimageFixture(t)
			mutate(&a)
			before := a.CloneRaw()
			need, err := currentImplementationInputReview(a)
			if err != nil || need != nil || !reflect.DeepEqual(before, a) {
				t.Fatalf("unaffected or already-owned transaction became new review: need=%v err=%v", need, err)
			}
		})
	}
}

func TestImplementationInputPostimageReusesFormalCoveredStateOnly(t *testing.T) {
	for _, kind := range []string{"covered", "new revision", "other CWI", "unsealed", "wrong subject", "other input owner"} {
		t.Run(kind, func(t *testing.T) {
			a := implementationInputPostimageFixture(t)
			v := a.ImplementationWork.Rounds[0].ScopeRequest.InputRevision
			binding := v.Binding.Clone()
			binding.InputReview = &rc.RuntimeValidationInputReviewSubject{Hash: "formal-current-subject", Files: append([]rc.RuntimeValidationInputReviewFile(nil), v.InputFiles()...)}
			binding.InputReview.Files[0].ContentHash = "actual-new-postimage"
			report := rc.RuntimeValidationInputReview{SubjectHash: binding.InputReview.Hash, Checks: []rc.RuntimeValidationInputCheckReview{{Conclusion: rc.RuntimeValidationInputConsistent}}}
			for i := range report.Checks {
				report.Checks[i].Conclusion = rc.RuntimeValidationInputConsistent
			}
			proposal := rc.RuntimeSemanticProofProposal{CoverageWorkItemID: binding.CoverageWorkItemID, DeclarationRef: binding.DeclarationRef, OwnerModuleID: rc.RuntimeModuleShell, OperationKind: rc.RuntimeBlockKindManagedValidationSession, ValidationRecipeID: binding.ValidationRecipeID, RequiredEvidenceKinds: []string{rc.EvidenceKindValidation}, ValidationInputReview: &report}
			catalog := rc.RuntimeOperationAdmissibility{OwnerModuleID: proposal.OwnerModuleID, ValidationSurfaces: []rc.RuntimeValidationSurface{{ID: proposal.ValidationRecipeID, OperationKind: proposal.OperationKind, InputTargetOwnerModuleID: v.OwnerModuleID, ProofRequirementBindings: []rc.RuntimeValidationProofRequirementBinding{binding}}}}
			snapshot := rc.RuntimeSemanticPlanningLedger{Phase: rc.RuntimeSemanticPlanningSealed, Manifest: &rc.RuntimeSemanticPlanManifest{OwnerCatalogs: []rc.RuntimeOperationAdmissibility{catalog}}, BatchDocuments: []rc.RuntimeSemanticPlanDocument{{Proposals: []rc.RuntimeSemanticProofProposal{proposal}}}}
			switch kind {
			case "new revision":
				a.PrerequisiteInvalidations[0].ResourceChanges[0].AfterRevision = "newer-postimage"
			case "other CWI":
				snapshot.BatchDocuments[0].Proposals[0].CoverageWorkItemID = "other"
			case "unsealed":
				snapshot.Phase = rc.RuntimeSemanticPlanningProofBatchDocumentAccepted
			case "wrong subject":
				snapshot.BatchDocuments[0].Proposals[0].ValidationInputReview.SubjectHash = "other-subject"
			case "other input owner":
				snapshot.Manifest.OwnerCatalogs[0].ValidationSurfaces[0].InputTargetOwnerModuleID = "other-owner"
			}
			a.SemanticPlanning.History = append(a.SemanticPlanning.History, rc.RuntimeSemanticPlanningGeneration{Snapshot: snapshot})
			before := a.CloneRaw()
			for range 2 {
				need, err := currentImplementationInputReview(a)
				if err != nil || (need == nil) != (kind == "covered") {
					t.Fatal("formal applicability changed", kind, need, err)
				}
			}
			if !reflect.DeepEqual(a, before) {
				t.Fatal("consuming a formal report rewrote history")
			}
		})
	}
}
