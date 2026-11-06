package runtime

import (
	"fmt"
	"slices"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ChangeReviewContext consumes committed owner records, not projections, and
// selects every mutation of the exact supplied owner corpus. No recent-N tail,
// repeat effect, historical business re-admission or model judgment is involved.
func ChangeReviewContext(a RuntimeAggregate, files []rc.RuntimeValidationInputReviewFile) (*rc.RuntimeChangeReviewContext, error) {
	if len(files) == 0 {
		return nil, nil
	}
	c := &rc.RuntimeChangeReviewContext{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, DefinitionRevision: a.Definition.Revision}
	byRef := map[string]rc.RuntimeValidationInputReviewFile{}
	for _, f := range files {
		if old, ok := byRef[f.TargetRef]; ok && old != f {
			return nil, fmt.Errorf("change review has ambiguous current owner content")
		}
		byRef[f.TargetRef] = f
	}
	for _, f := range byRef {
		c.Current = append(c.Current, f)
	}
	slices.SortFunc(c.Current, func(a, b rc.RuntimeValidationInputReviewFile) int {
		if a.TargetRef < b.TargetRef {
			return -1
		}
		if a.TargetRef > b.TargetRef {
			return 1
		}
		return 0
	})
	type source struct {
		dispatch  rc.RuntimeDispatchRecord
		execution rc.RuntimeOperationExecutionRecord
	}
	selectedKeys := map[string]bool{}
	for _, r := range a.OperationReceipts {
		if r.Changed && r.Outcome == rc.RuntimeOperationOutcomeAccepted && r.ObservedEffectScope == rc.RuntimeEffectWorkspaceDurable {
			for _, ref := range r.TargetRefs {
				if _, ok := byRef[ref]; ok {
					selectedKeys[r.ExecutionKey] = true
				}
			}
		}
	}
	sources := map[string]source{}
	for _, d := range a.CompletedDispatches {
		for _, e := range d.OperationExecutions {
			if !selectedKeys[e.ExecutionKey] {
				continue
			}
			if _, ok := sources[e.ExecutionKey]; ok {
				return nil, fmt.Errorf("change review duplicate execution source")
			}
			sources[e.ExecutionKey] = source{d, e}
		}
	}
	last := map[string]string{}
	for _, r := range a.OperationReceipts {
		if !r.Changed || r.Outcome != rc.RuntimeOperationOutcomeAccepted || r.ObservedEffectScope != rc.RuntimeEffectWorkspaceDurable {
			continue
		}
		selected := false
		for _, ref := range r.TargetRefs {
			if _, ok := byRef[ref]; ok {
				selected = true
			}
		}
		if !selected {
			continue
		}
		if len(r.TargetRefs) != 1 {
			return nil, fmt.Errorf("change review requires an exact single-target content version edge")
		}
		s, ok := sources[r.ExecutionKey]
		if !ok || s.dispatch.WorkspaceID != a.Facts.WorkspaceID || s.dispatch.RunID != a.Facts.RunID || s.execution.OperationID != r.OperationID || s.execution.Fence != r.Fence || s.execution.State != rc.OperationExecutionEvidenceCommitted || s.execution.OwnerReceiptHash != rc.StableRuntimeOperationReceiptHash(r) {
			return nil, fmt.Errorf("change review receipt has no exact committed source")
		}
		op, ok := candidateOperationByID(s.dispatch.Candidate, r.OperationID)
		if !ok || s.execution.OperationHash != rc.StableRuntimeOperationHash(op) || !slices.Contains(op.TargetRefs, r.TargetRefs[0]) {
			return nil, fmt.Errorf("change review source operation missing or target differs")
		}
		descriptor, ok := rc.RuntimeOperationDescriptorForKind(op.Kind)
		if !ok {
			return nil, fmt.Errorf("change review owner missing")
		}
		ref := r.TargetRefs[0]
		v := rc.RuntimeChangeReviewOperation{Operation: op.Clone(), OperationHash: s.execution.OperationHash, ExecutionKey: r.ExecutionKey, Fence: r.Fence, DispatchID: s.dispatch.ID, OperationID: r.OperationID, OwnerModuleID: descriptor.OwnerModuleID, ReceiptRef: s.execution.OwnerReceiptRef, ReceiptHash: s.execution.OwnerReceiptHash, TargetRef: ref, BeforeHash: r.BeforeHash, AfterHash: r.AfterHash}
		if last[ref] != r.BeforeHash && r.BeforeHash != "" {
			round := s.dispatch.ImplementationWorkCommitment
			if round != nil && round.ScopeObservation != nil && round.ScopeObservation.Content != nil {
				for _, f := range round.ScopeObservation.Content.Snapshots {
					if f.TargetRef == ref && f.ContentHash == r.BeforeHash {
						v.Baseline = &rc.RuntimeValidationInputReviewFile{TargetRef: ref, ContentHash: f.ContentHash, Content: f.Content}
					}
				}
			}
		}
		last[ref] = r.AfterHash
		c.Changes = append(c.Changes, v)
	}
	return c, c.Validate(a.Facts.WorkspaceID, a.Facts.RunID, a.Definition.Revision)
}

func SemanticChangeReviewContext(a RuntimeAggregate, c rc.RuntimeSemanticPlanRequestContext, m rc.RuntimeSemanticPlanManifest) (*rc.RuntimeChangeReviewContext, error) {
	if c.PlanStep == rc.RuntimeSemanticPlanStepCoverageOrder && !rc.RuntimeSemanticPurposeChoice(m, c.PlanStep) {
		return nil, nil
	}
	var files []rc.RuntimeValidationInputReviewFile
	if c.WorkContext != nil {
		if c.WorkContext.Content == nil {
			return nil, fmt.Errorf("change review work source content missing")
		}
		for _, f := range c.WorkContext.Content.Snapshots {
			files = append(files, rc.RuntimeValidationInputReviewFile{TargetRef: f.TargetRef, ContentHash: f.ContentHash, Content: f.Content})
		}
	} else {
		for _, o := range m.ProofObligations {
			if len(c.BatchCoverageWorkItemIDs) != 0 && !slices.Contains(c.BatchCoverageWorkItemIDs, o.CoverageWorkItemID) {
				continue
			}
			for _, catalog := range append(append([]rc.RuntimeOperationAdmissibility(nil), m.OwnerCatalogs...), m.OwnerFactDependencyCatalogs...) {
				for _, s := range catalog.ValidationSurfaces {
					if s.GenerationContext != nil {
						selected := false
						for _, binding := range s.ProofRequirementBindings {
							selected = selected || binding.CoverageWorkItemID == o.CoverageWorkItemID
						}
						for _, preparation := range s.ProofRequirementPreparations {
							selected = selected || preparation.CoverageWorkItemID == o.CoverageWorkItemID
						}
						if selected {
							files = append(files, s.GenerationContext.Files...)
						}
					}
				}
			}
		}
	}
	return ChangeReviewContext(a, files)
}

func CandidateChangeReviewContext(a RuntimeAggregate, c rc.RuntimeCandidateContext) (*rc.RuntimeChangeReviewContext, error) {
	if c.RequiredState != rc.ConvergenceStateNeedAssessment && c.RequiredState != rc.ConvergenceStateNeedStateChange {
		return nil, nil
	}
	var files []rc.RuntimeValidationInputReviewFile
	if c.ImplementationWorkContent != nil {
		for _, f := range c.ImplementationWorkContent.Snapshots {
			files = append(files, rc.RuntimeValidationInputReviewFile{TargetRef: f.TargetRef, ContentHash: f.ContentHash, Content: f.Content})
		}
	} else {
		for _, catalog := range c.Resources.OperationAdmissibilities {
			for _, s := range catalog.ValidationSurfaces {
				if s.GenerationContext != nil {
					selected := false
					for _, b := range s.ProofRequirementBindings {
						selected = selected || slices.Contains(c.Focus.CoverageWorkItemIDs, b.CoverageWorkItemID)
					}
					for _, p := range s.ProofRequirementPreparations {
						selected = selected || slices.Contains(c.Focus.CoverageWorkItemIDs, p.CoverageWorkItemID)
					}
					if selected {
						files = append(files, s.GenerationContext.Files...)
					}
				}
			}
		}
	}
	return ChangeReviewContext(a, files)
}
