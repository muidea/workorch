package runtime

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type implementationInputTarget struct {
	owner, target string
}

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
	targets := map[implementationInputTarget]struct{}{}
	ownerMaintenanceTargets := map[implementationInputTarget]struct{}{}
	semanticTargets := map[implementationInputTarget]struct{}{}
	changed := map[implementationInputTarget]string{}
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
				key := implementationInputTarget{v.OwnerModuleID, input.TargetRef}
				targets[key] = struct{}{}
				if v.OwnerObservation != nil {
					ownerMaintenanceTargets[key] = struct{}{}
				} else {
					semanticTargets[key] = struct{}{}
				}
			}
			for _, check := range v.Report.Checks {
				for _, target := range check.DependencyTargetRefs {
					key := implementationInputTarget{v.OwnerModuleID, target}
					targets[key] = struct{}{}
					semanticTargets[key] = struct{}{}
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
				if _, intersects := targets[implementationInputTarget{change.OwnerModuleID, change.SubjectRef}]; intersects && change.BeforeRevision != change.AfterRevision {
					changed[implementationInputTarget{change.OwnerModuleID, change.SubjectRef}] = change.AfterRevision
				}
			}
		}
	}
	if len(changed) == 0 {
		return nil, nil
	}
	// Structural maintenance may repair an already-observed input outside the
	// current CWI's independent review subject (for example before this CWI has
	// a ready binding or created package). Once fresh owner catalogs resolve the
	// CWI cleanly with zero unavailability, ignore changes that exist solely on
	// such non-subject owner-maintenance files while failing closed otherwise.
	if subjectTargets, resolved := currentValidationInputReviewSubjectTargets(a.Facts, cwi); resolved {
		for target := range changed {
			_, isMaint := ownerMaintenanceTargets[target]
			_, isSem := semanticTargets[target]
			if isMaint && !isSem {
				if _, inSubject := subjectTargets[target]; !inSubject {
					delete(changed, target)
				}
			}
		}
		if len(changed) == 0 {
			return nil, nil
		}
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

// currentValidationInputReviewSubjectTargets resolves the current CWI's active
// validation review subject files from quiescent fresh owner catalogs. It
// fails closed when prerequisites are pending, any validation requirement is
// still structurally unavailable, or the catalog neither binds nor prepares the
// requirement.
func currentValidationInputReviewSubjectTargets(f rc.RuntimeFacts, cwi common.CoverageWorkItemID) (map[implementationInputTarget]struct{}, bool) {
	if len(f.Prerequisites.Pending) != 0 {
		return nil, false
	}
	var validationReqs []rc.RuntimeOwnerFactRequirement
	for _, raw := range currentFocusOwnerFactRequirements(f) {
		req := raw.Normalized()
		if req.FactKind == rc.RuntimeOwnerFactSelectableValidationSurface && req.ValidationProofRequirementHash != "" {
			validationReqs = append(validationReqs, req)
		}
	}
	if len(validationReqs) == 0 {
		return nil, false
	}
	subjectTargets := map[implementationInputTarget]struct{}{}
	for _, req := range validationReqs {
		if rc.RuntimeOwnerFactHasUnavailableValidationProof(req, f.Resources.OperationAdmissibilities) {
			return nil, false
		}
		resolvedReq := false
		for _, catalog := range f.Resources.OperationAdmissibilities {
			if strings.TrimSpace(catalog.OwnerModuleID) != req.OwnerModuleID {
				continue
			}
			for _, surface := range catalog.ValidationSurfaces {
				if strings.TrimSpace(surface.OwnerModuleID) != req.OwnerModuleID || rc.CanonicalRuntimeBlockKind(surface.OperationKind) != req.OperationKind {
					continue
				}
				for _, binding := range surface.ProofRequirementBindings {
					if binding.RequirementHash == req.ValidationProofRequirementHash && binding.CoverageWorkItemID == cwi {
						resolvedReq = true
						if binding.InputReview != nil {
							for _, file := range binding.InputReview.Files {
								subjectTargets[implementationInputTarget{surface.InputTargetOwnerModuleID, file.TargetRef}] = struct{}{}
							}
						}
					}
				}
				for _, prep := range surface.ProofRequirementPreparations {
					if prep.RequirementHash == req.ValidationProofRequirementHash && prep.CoverageWorkItemID == cwi {
						resolvedReq = true
					}
				}
			}
		}
		if !resolvedReq {
			return nil, false
		}
	}
	return subjectTargets, true
}
