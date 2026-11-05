package workorchruntimecontract

// Frozen complete pre-change source compiler/state algorithm. This is an
// independent test oracle, never a production compatibility implementation.
import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	"reflect"
	"slices"
)

func beforeCompileRuntimeValidationInputRevision(l RuntimeSemanticPlanningLedger) (*RuntimeValidationInputRevision, error) {
	if l.Phase != RuntimeSemanticPlanningProofBatchDocumentAccepted || l.Manifest == nil {
		return nil, fmt.Errorf("input revision requires original accepted review batch")
	}
	var doc *RuntimeSemanticPlanDocument
	for _, d := range l.BatchDocuments {
		if d.BatchID == l.ActiveBatchID {
			if doc != nil {
				return nil, fmt.Errorf("duplicate input review batch")
			}
			c := d
			doc = &c
		}
	}
	if doc == nil {
		return nil, fmt.Errorf("missing original input review")
	}
	if err := doc.ValidateAgainstManifest(*l.Manifest); err != nil {
		return nil, err
	}
	var v *RuntimeValidationInputRevision
	for _, p := range doc.Proposals {
		if p.ValidationInputReview == nil {
			continue
		}
		if v != nil {
			return nil, fmt.Errorf("input revision requires one unambiguous review")
		}
		b, ok := RuntimeSemanticValidationInputReviewBinding(p, l.Manifest.OwnerCatalogs)
		if !ok || b.InputReview == nil {
			return nil, fmt.Errorf("missing original review binding")
		}
		owner := ""
		var recipe *RuntimeValidationRecipe
		var targets []string
		var findings []string
		for _, check := range p.ValidationInputReview.Checks {
			if check.Conclusion == RuntimeValidationInputConflict {
				findings = append(findings, check.InputTargetRefs...)
			}
		}
		for _, source := range p.ValidationInputReview.DiagnosticSources {
			if source.Conclusion == RuntimeValidationInputConflict {
				findings = append(findings, source.InputTargetRefs...)
			}
		}
		for _, c := range l.Manifest.OwnerCatalogs {
			if c.OwnerModuleID != p.OwnerModuleID {
				continue
			}
			for _, surface := range c.ValidationSurfaces {
				if surface.ID != p.ValidationRecipeID || surface.OperationKind != p.OperationKind {
					continue
				}
				for _, original := range surface.ProofRequirementBindings {
					if reflect.DeepEqual(original.Normalized(), b) {
						if owner != "" {
							return nil, fmt.Errorf("ambiguous validation input resource owner")
						}
						owner = surface.InputTargetOwnerModuleID
						if b.InputReview.Recipe != nil {
							originalRecipe, found := RuntimeValidationRecipeForID(c.ValidationRecipes, b.ValidationRecipeID)
							if !found || !reflect.DeepEqual(*b.InputReview.Recipe, originalRecipe) {
								return nil, fmt.Errorf("input revision declared method differs from original owner catalog")
							}
							recipe = cloneRuntimeInputRecipe(b.InputReview.Recipe)
						}
						for _, f := range b.InputReview.Files {
							if slices.Contains(surface.InputTargetRefs, f.TargetRef) && slices.Contains(findings, f.TargetRef) {
								targets = append(targets, f.TargetRef)
							}
						}
					}
				}
			}
		}

		v = &RuntimeValidationInputRevision{WorkspaceID: l.Manifest.WorkspaceID, RunID: l.Manifest.RunID, DefinitionSeedHash: l.Manifest.DefinitionSeedHash, SourceManifestHash: l.Manifest.Hash, SourceLogicalRequestID: l.LogicalRequestID, SourceDocumentHash: StableRuntimeSemanticPlanDocumentHash(*doc), OwnerModuleID: owner, InputTargetRefs: normalizeStrings(targets), Binding: b, Recipe: cloneRuntimeInputRecipe(recipe), Report: p.ValidationInputReview.Clone()}
	}
	if v == nil {
		return nil, fmt.Errorf("no actual input review conflict")
	}
	for _, p := range doc.Proposals {
		if p.CoverageWorkItemID != v.Binding.CoverageWorkItemID {
			return nil, fmt.Errorf("input revision cannot discard another CWI batch")
		}
	}
	return v, v.Validate()
}

func beforeValidateRuntimeInputRevisionSource(v *RuntimeValidationInputRevision, l RuntimeSemanticPlanningLedger) error {
	if v == nil {
		return nil
	}
	matches := 0
	for _, g := range l.AllHistory() {
		if g.Reason != RuntimeSemanticPlanningRetirementInputRevision {
			continue
		}
		original, err := beforeCompileRuntimeValidationInputRevisionGeneration(g)
		if err == nil && reflect.DeepEqual(v, original) {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("input revision has no unique original rejected semantic generation")
	}
	return nil
}

func beforePendingRuntimeValidationInputRevision(s RuntimeSemanticPlanningLedger, w RuntimeImplementationWorkLedger, cwi common.CoverageWorkItemID) (*RuntimeValidationInputRevision, error) {
	var pending *RuntimeValidationInputRevision
	for _, g := range s.AllHistory() {
		if g.Reason != RuntimeSemanticPlanningRetirementInputRevision {
			continue
		}
		v, err := beforeCompileRuntimeValidationInputRevisionGeneration(g)
		if err != nil {
			return nil, err
		}
		if v.Binding.CoverageWorkItemID != cwi {
			continue
		}
		consumed, matches := false, 0
		for _, r := range w.Rounds {
			if reflect.DeepEqual(r.ScopeRequest.InputRevision, v) {
				matches++
				consumed = r.CompletedReceiptSetHash != "" && r.FailedReceiptHash == ""
			}
		}
		if matches > 1 {
			return nil, fmt.Errorf("input review reused for multiple effect rounds")
		}
		if !consumed {
			if pending != nil {
				return nil, fmt.Errorf("multiple unresolved input revisions")
			}
			pending = v
		}
	}
	return pending, nil
}

func beforeValidateRuntimeInputRevisionState(def RuntimeDefinitionContext, facts RuntimeFacts, w RuntimeImplementationWorkLedger, s RuntimeSemanticPlanningLedger, session *runtimeSemanticValidationSession) error {
	// A history product may only be consumed by a later semantic transaction.
	for _, consumer := range s.AllHistory() {
		if session.committedGeneration(consumer) {
			continue
		}
		if consumer.Snapshot.Manifest == nil {
			continue
		}
		for _, o := range consumer.Snapshot.Manifest.ProofObligations {
			if o.ImplementationWork == nil || o.ImplementationWork.Scope.Request.InputRevision == nil {
				continue
			}
			v := o.ImplementationWork.Scope.Request.InputRevision
			if err := beforeValidateRuntimeInputRevisionSource(v, s); err != nil {
				return err
			}
			for _, source := range s.AllHistory() {
				if source.Reason == RuntimeSemanticPlanningRetirementInputRevision && ((v.OwnerObservation != nil && reflect.DeepEqual(source.OwnerInputRevision, v)) || (source.Snapshot.Manifest != nil && source.Snapshot.Manifest.Hash == v.SourceManifestHash)) && source.RetiredAtStateRevision >= consumer.RetiredAtStateRevision {
					return fmt.Errorf("input revision borrowed a future conflict report")
				}
			}
		}
	}
	validate := func(r RuntimeImplementationWorkScopeRequest) error {
		if err := beforeValidateRuntimeInputRevisionSource(r.InputRevision, s); err != nil {
			return err
		}
		if v := r.InputRevision; v != nil && (v.WorkspaceID != facts.WorkspaceID || v.RunID != facts.RunID || v.DefinitionSeedHash != facts.DefinitionSeed.Hash || v.Binding.DefinitionRevision != def.Revision) {
			return fmt.Errorf("input revision escapes original startup")
		}
		return nil
	}
	if w.ScopePreview != nil {
		if err := validate(w.ScopePreview.Request); err != nil {
			return err
		}
	}
	for _, r := range w.Rounds {
		if session.completedWorkRound(r) {
			continue
		}
		if err := validate(r.ScopeRequest); err != nil {
			return err
		}
	}
	for _, g := range s.AllHistory() {
		if g.Reason != RuntimeSemanticPlanningRetirementInputRevision {
			continue
		}
		v, err := beforeCompileRuntimeValidationInputRevisionGeneration(g)
		if err != nil {
			return err
		}
		declared := false
		for _, f := range facts.DefinitionFocuses {
			if len(f.CoverageWorkItemIDs) == 1 && f.CoverageWorkItemIDs[0] == v.Binding.CoverageWorkItemID && f.ImplementationWorkPolicy != nil && f.ImplementationWorkPolicy.InputRevisionProtocolRef == RuntimeValidationInputRevisionProtocolV1 && f.ImplementationWorkPolicy.OwnerModuleID == v.OwnerModuleID {
				declared = true
			}
		}
		if !declared || v.WorkspaceID != facts.WorkspaceID || v.RunID != facts.RunID || v.DefinitionSeedHash != facts.DefinitionSeed.Hash || v.Binding.DefinitionRevision != def.Revision {
			return fmt.Errorf("retired input revision lacks original Profile/startup contract")
		}
		pending, err := beforePendingRuntimeValidationInputRevision(s, w, v.Binding.CoverageWorkItemID)
		if err != nil {
			return err
		}

		if pending != nil { // It must still be the active CWI until a real effect settles.
			if !slices.Contains(facts.Focus.CoverageWorkItemIDs, pending.Binding.CoverageWorkItemID) {
				return fmt.Errorf("pending input revision lost original CWI focus")
			}
		}
	}
	return nil
}

func beforeValidateRuntimeInputRevisionRequestSources(semantic RuntimeSemanticPlanningLedger, sessions []RuntimeCandidateRequestSession) error {
	for _, g := range semantic.AllHistory() {
		if g.Reason != RuntimeSemanticPlanningRetirementInputRevision {
			continue
		}
		v, err := beforeCompileRuntimeValidationInputRevisionGeneration(g)
		if err != nil {
			return err
		}
		if v.OwnerObservation != nil {
			continue
		} // owner observations are not provider transactions
		var doc *RuntimeSemanticPlanDocument
		for i := range g.Snapshot.BatchDocuments {
			d := &g.Snapshot.BatchDocuments[i]
			if d.BatchID == g.Snapshot.ActiveBatchID {
				doc = d
			}
		}
		matches := 0
		for _, s := range sessions {
			e := s.ExpectedDocument
			if s.LogicalRequestID != v.SourceLogicalRequestID || s.DocumentKind != RuntimeProviderDocumentSemanticPlan || s.State != RuntimeCandidateRequestSessionTerminal || s.ProviderSubmitted || s.ManifestHash != v.SourceManifestHash || s.StablePrefixHash != g.Snapshot.StablePrefixHash || e == nil || s.LastOutcome.Class != RuntimeCandidateRequestOutcomePrerequisiteStale || s.LastOutcome.Code != RuntimeValidationInputRevisionRetirementOutcome || s.LastOutcome.Signature != RuntimeValidationInputRevisionRetirementOutcome+"/"+s.ManifestHash+"/"+s.LogicalRequestID {
				continue
			}
			if e.LogicalRequestID == s.LogicalRequestID && e.ManifestHash == s.ManifestHash && e.DefinitionSeedHash == doc.DefinitionSeedHash && e.PlanStep == doc.PlanStep && e.BatchID == doc.BatchID && e.BatchSequence == doc.BatchSequence && reflect.DeepEqual(e.BatchCoverageWorkItemIDs, doc.BatchCoverageWorkItemIDs) {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("input revision lacks unique original completed-and-retired request source")
		}
	}
	return nil
}

func beforeCompileRuntimeValidationInputRevisionGeneration(g RuntimeSemanticPlanningGeneration) (*RuntimeValidationInputRevision, error) {
	if g.OwnerInputRevision != nil {
		v := g.OwnerInputRevision.Clone()
		return v, v.Validate()
	}
	return beforeCompileRuntimeValidationInputRevision(g.Snapshot)
}
