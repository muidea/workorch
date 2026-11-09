package runtime

import (
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// implementationInputReview projects formal work and its original input source.
// It is not a second pending queue or a write permission.
type implementationInputReview struct {
	coverageID common.CoverageWorkItemID
}

// currentImplementationInputReview keeps transaction consumption separate from
// the current postimage's semantic obligations. Only intersecting work after an
// input-maintenance source needs the ordinary independent input/proof path.
// In-flight work, failures and exact repair retain their original responsibility.
func currentImplementationInputReview(a RuntimeAggregate) (*implementationInputReview, error) {
	if a.Termination != nil || a.Facts.Failure != nil || a.Facts.RepairDirective != nil || a.ActiveDispatch != nil || a.SemanticPlanning.FailureInputReview != nil || len(a.Facts.Focus.CoverageWorkItemIDs) != 1 {
		return nil, nil
	}
	cwi := a.Facts.Focus.CoverageWorkItemIDs[0]
	type inputTarget struct{ owner, target string }
	targets := map[inputTarget]struct{}{}
	changed := map[inputTarget]string{}
	for i := range a.ImplementationWork.Rounds {
		r := &a.ImplementationWork.Rounds[i]
		if r.CoverageWorkItemID != cwi {
			continue
		}
		if r.CompletedReceiptSetHash == "" || r.FailedReceiptHash != "" {
			return nil, nil
		}
		if v := r.ScopeRequest.InputRevision; v != nil {
			for _, input := range v.InputFiles() {
				targets[inputTarget{v.OwnerModuleID, input.TargetRef}] = struct{}{}
			}
			for _, check := range v.Report.Checks {
				for _, target := range check.DependencyTargetRefs {
					targets[inputTarget{v.OwnerModuleID, target}] = struct{}{}
				}
			}
		}
		// Receipt settlement persists these exact owner-observed edges in the
		// same CAS as completion and catalog invalidation. Proposed scope alone
		// does not establish that a protected input actually changed.
		for _, invalidation := range a.PrerequisiteInvalidations {
			if r.DispatchID == "" || invalidation.DispatchID != r.DispatchID {
				continue
			}
			for _, change := range invalidation.ResourceChanges {
				if _, intersects := targets[inputTarget{change.OwnerModuleID, change.SubjectRef}]; intersects && change.BeforeRevision != change.AfterRevision {
					changed[inputTarget{change.OwnerModuleID, change.SubjectRef}] = change.AfterRevision
				}
			}
		}
	}
	if len(changed) == 0 {
		return nil, nil
	}
	// A retired ordinary generation may already have settled these exact
	// postimages. Consume its formal consistent report within that scope;
	// never reopen historical work merely because an old edge still exists.
	for _, generation := range a.SemanticPlanning.AllHistory() {
		l := generation.Snapshot
		if l.Phase != rc.RuntimeSemanticPlanningSealed || l.Manifest == nil {
			continue
		}
		for _, document := range l.BatchDocuments {
			for _, proposal := range document.Proposals {
				r := proposal.ValidationInputReview
				if proposal.CoverageWorkItemID != cwi || r == nil || rc.RuntimeValidationInputReviewConclusion(*r) != rc.RuntimeValidationInputConsistent {
					continue
				}
				binding, found := rc.RuntimeSemanticValidationInputReviewBinding(proposal, l.Manifest.OwnerCatalogs)
				if !found || binding.InputReview == nil || binding.InputReview.Hash != r.SubjectHash {
					continue
				}
				inputOwner := ""
				for _, catalog := range l.Manifest.OwnerCatalogs {
					if catalog.OwnerModuleID != proposal.OwnerModuleID {
						continue
					}
					for _, surface := range catalog.ValidationSurfaces {
						if surface.ID == proposal.ValidationRecipeID && surface.OperationKind == proposal.OperationKind {
							inputOwner = surface.InputTargetOwnerModuleID
						}
					}
				}
				covered := 0
				for target, revision := range changed {
					if target.owner != inputOwner {
						continue
					}
					for _, file := range binding.InputReview.Files {
						if file.TargetRef == target.target && file.ContentHash == revision {
							covered++
							break
						}
					}
				}
				if covered == len(changed) {
					return nil, nil
				}
			}
		}
	}
	pending, err := rc.PendingRuntimeValidationInputRevision(a.SemanticPlanning, a.ImplementationWork, cwi)
	if err != nil || pending != nil {
		return nil, err
	}
	// The ordinary semantic manifest owns the new subject, independent input
	// report, feasibility and proof binding. A consistent report proceeds along
	// that same transaction to fresh all-of; it is not retired to ask another
	// purpose request to judge the identical input a second time.
	return &implementationInputReview{coverageID: cwi}, nil
}
