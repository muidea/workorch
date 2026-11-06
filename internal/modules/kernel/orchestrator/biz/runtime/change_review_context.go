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
			var err error
			v.Baseline, err = changeReviewSealedPreimage(a, s.dispatch, ref, r.BeforeHash)
			if err != nil {
				return nil, err
			}
		}
		last[ref] = r.AfterHash
		c.Changes = append(c.Changes, v)
	}
	return c, c.Validate(a.Facts.WorkspaceID, a.Facts.RunID, a.Definition.Revision)
}

// Auxiliary dispatches have no implementation round. Their exact before-bytes
// remain on the original pre-seal candidate request, reached through the
// accepted attempt, never a current catalog or an unrelated matching hash.
// These are committed source joins, not a second candidate/business admission.
func changeReviewSealedPreimage(a RuntimeAggregate, d rc.RuntimeDispatchRecord, ref, hash string) (*rc.RuntimeValidationInputReviewFile, error) {
	var result *rc.RuntimeValidationInputReviewFile
	add := func(f rc.RuntimeValidationInputReviewFile) error {
		if f.TargetRef != ref || f.ContentHash != hash {
			return nil
		}
		if result != nil && *result != f {
			return fmt.Errorf("change review has ambiguous sealed preimage")
		}
		value := f
		result = &value
		return nil
	}
	if round := d.ImplementationWorkCommitment; round != nil {
		if round.ScopeObservation != nil && round.ScopeObservation.Content != nil {
			for _, f := range round.ScopeObservation.Content.Snapshots {
				if err := add(rc.RuntimeValidationInputReviewFile{TargetRef: f.TargetRef, ContentHash: f.ContentHash, Content: f.Content}); err != nil {
					return nil, err
				}
			}
		}
		return result, nil
	}
	// A governance continuation retains the original candidate authority but
	// has its own dispatch transaction. Follow only its formally committed
	// handoff; an equal manifest hash alone cannot select another request.
	seen := map[string]bool{}
	for {
		if seen[d.ID] {
			return nil, fmt.Errorf("change review cyclic candidate admission source")
		}
		seen[d.ID] = true
		sourceID := ""
		for _, binding := range a.OperationGovernanceBindings {
			if binding.State != rc.RuntimeOperationGovernanceBindingGranted || binding.Grant == nil || binding.Grant.ResumeDispatchID != d.ID {
				continue
			}
			if sourceID != "" || binding.OperationDispatchID == "" {
				return nil, fmt.Errorf("change review ambiguous governance admission source")
			}
			sourceID = binding.OperationDispatchID
		}
		if sourceID == "" {
			break
		}
		var source *rc.RuntimeDispatchRecord
		for i := range a.CompletedDispatches {
			x := &a.CompletedDispatches[i]
			if x.ID != sourceID {
				continue
			}
			if source != nil || x.WorkspaceID != d.WorkspaceID || x.RunID != d.RunID || x.State != rc.RuntimeDispatchStateCommitted || x.CandidateHash != d.CandidateHash || x.CandidateManifestHash != d.CandidateManifestHash {
				return nil, fmt.Errorf("change review governance source differs from original candidate")
			}
			source = x
		}
		if source == nil {
			return nil, fmt.Errorf("change review missing governance admission source")
		}
		d = *source
	}
	var attempt *rc.RuntimeCandidateAttemptRecord
	for i := range a.CandidateAttempts {
		x := &a.CandidateAttempts[i]
		if x.DispatchID != d.ID {
			continue
		}
		if attempt != nil || x.State != rc.RuntimeCandidateAttemptStateAccepted || x.Transaction != d.Transaction || x.CandidateHash != d.CandidateHash || x.ManifestHash != d.CandidateManifestHash || x.LogicalRequestID == "" {
			return nil, fmt.Errorf("change review requires original accepted candidate source")
		}
		attempt = x
	}
	if attempt == nil || d.CandidateManifestHash == "" {
		return nil, fmt.Errorf("change review missing original accepted candidate source")
	}
	var manifest *rc.RuntimeCandidateContractManifest
	for _, session := range a.CandidateRequestSessions {
		if session.LogicalRequestID != attempt.LogicalRequestID {
			continue
		}
		if manifest != nil || session.DocumentKind != rc.RuntimeProviderDocumentOperations || session.Transaction != d.Transaction || session.StablePrefixHash != attempt.StablePrefixHash || session.ManifestHash != d.CandidateManifestHash || session.CandidateManifest == nil || session.CandidateManifest.Hash != d.CandidateManifestHash {
			return nil, fmt.Errorf("change review requires exact pre-seal candidate request")
		}
		manifest = session.CandidateManifest
	}
	if manifest == nil {
		return nil, fmt.Errorf("change review missing pre-seal candidate request")
	}
	for _, catalog := range manifest.MutationInputCatalogs {
		for _, surface := range catalog.ValidationSurfaces {
			if surface.GenerationContext != nil {
				for _, f := range surface.GenerationContext.Files {
					if err := add(f); err != nil {
						return nil, err
					}
				}
			}
			for _, binding := range surface.ProofRequirementBindings {
				if binding.InputReview != nil {
					for _, f := range binding.InputReview.Files {
						if err := add(f); err != nil {
							return nil, err
						}
					}
				}
			}
		}
	}
	return result, nil
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
