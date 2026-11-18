package runtime

import (
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCommittedSemanticBindingConsumerKeepsExactProjectionBoundary(t *testing.T) {
	before := runtimeTestSemanticPreimageStateChangeAggregate(t)
	if err := before.Validate(); err != nil {
		t.Fatal(err)
	}
	after := before.CloneRaw()
	after.StateRevision++
	after.Facts.Failure = &rc.FailureFact{Class: rc.FailureClassRuntimeContractMisalignment, Code: "runtime_drive_failed", Signature: "test", Message: "unrelated new failure"}
	after.SemanticPlanning.Phase = rc.RuntimeSemanticPlanningRequired
	if !runtimeSemanticBindingProjectionsUnchanged(&before, after) {
		t.Fatal("unrelated facts/request progress caused a new binding task")
	}
	for _, arm := range []string{"focus", "coverage", "reconciliation", "new_session", "source"} {
		t.Run(arm, func(t *testing.T) {
			next := before.CloneRaw()
			next.StateRevision++
			switch arm {
			case "focus":
				next = runtimeTestRewriteSemanticBindingProjections(t, before, true, false, false, false)
			case "coverage":
				next.Facts.Coverage[0].CurrentBindingRef = nil
			case "reconciliation":
				next.Facts.ReconciliationContract = rc.RuntimeCWIReconciliationContract{}
			case "new_session":
				next.CandidateRequestSessions = append(next.CandidateRequestSessions, rc.RuntimeCandidateRequestSession{LogicalRequestID: "new", CandidateManifest: &rc.RuntimeCandidateContractManifest{State: rc.ConvergenceStateNeedStateChange, Bindings: []rc.CoverageEvidenceBinding{{ID: "forged", Hash: "self"}}}})
			case "source":
				next.SemanticPlanning.Batches[0].RequestLogicalID = "coordinated-rewrite"
				next.SemanticPlanning.Batches[0].Hash = rc.StableRuntimeSemanticPlanBatchHash(next.SemanticPlanning.Batches[0])
			}
			if arm == "source" {
				if err := rc.ValidateRuntimeCommittedHistoryTransition(ownerStateRecord(before), ownerStateRecord(next)); err == nil {
					t.Fatal("original accepted source was replaceable")
				}
			} else if runtimeSemanticBindingProjectionsUnchanged(&before, next) {
				t.Fatal("new or changed authority consumed old result")
			}
		})
	}
	if runtimeSemanticBindingProjectionsUnchanged(nil, after) {
		t.Fatal("cold input claimed committed authority")
	}

	advanced := before.CloneRaw()
	advanced.StateRevision++
	nextFocus := advanced.Facts.Focus
	nextFocus.ID = "focus-next"
	advanced.Facts.CompletedFocuses = append(advanced.Facts.CompletedFocuses, advanced.Facts.Focus)
	advanced.Facts.Focus = rc.CoverageFocus{
		ID:                       nextFocus.ID,
		CoverageEvidenceBindings: []rc.CoverageEvidenceBinding{},
	}
	if !runtimeSemanticBindingProjectionsUnchanged(&before, advanced) {
		t.Fatal("pure focus-queue advancement with unchanged bindings should reuse committed bindings")
	}
	advanced.Facts.CompletedFocuses[len(advanced.Facts.CompletedFocuses)-1].CoverageEvidenceBindings = append(
		[]rc.CoverageEvidenceBinding(nil),
		before.Facts.Focus.CoverageEvidenceBindings...,
	)
	advanced.Facts.CompletedFocuses[len(advanced.Facts.CompletedFocuses)-1].CoverageEvidenceBindings[0].Hash = "tampered"
	if runtimeSemanticBindingProjectionsUnchanged(&before, advanced) {
		t.Fatal("modified focus binding in CompletedFocuses must trigger full semantic preimage check")
	}
}
