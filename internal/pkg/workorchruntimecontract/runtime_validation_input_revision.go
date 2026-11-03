package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
)

const RuntimeValidationInputRevisionProtocolV1 = "validation/input-revision/v1"
const RuntimeValidationInputRevisionRetirementOutcome = "validation_input_revision_required"

// RuntimeValidationInputRevision retains the original complete rejected review.
// It opens only those observed input resources for a bounded ordinary work
// round. It is neither a consistency verdict nor a validation-failure repair.
// Aggregate admission must additionally reverse-prove the original generation.
type RuntimeValidationInputRevision struct {
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	DefinitionSeedHash     string
	SourceManifestHash     string
	SourceLogicalRequestID string
	SourceDocumentHash     string
	OwnerModuleID          string
	InputTargetRefs        []string
	Binding                RuntimeValidationProofRequirementBinding
	Report                 RuntimeValidationInputReview
}

func (v *RuntimeValidationInputRevision) Clone() *RuntimeValidationInputRevision {
	if v == nil {
		return nil
	}
	c := *v
	c.InputTargetRefs = slices.Clone(v.InputTargetRefs)
	c.Binding = v.Binding
	c.Binding.RequiredCheckRefs = slices.Clone(v.Binding.RequiredCheckRefs)
	c.Binding.CheckAttestations = slices.Clone(v.Binding.CheckAttestations)
	if v.Binding.InputReview != nil {
		b := v.Binding.InputReview.Clone()
		c.Binding.InputReview = &b
	}
	c.Report = v.Report.Clone()
	return &c
}

func (v RuntimeValidationInputRevision) Validate() error {
	for _, ref := range []string{string(v.WorkspaceID), string(v.RunID), v.DefinitionSeedHash, v.SourceManifestHash, v.SourceLogicalRequestID, v.SourceDocumentHash, v.OwnerModuleID} {
		if !implementationWorkExactRef(ref) {
			return fmt.Errorf("input revision requires exact original review identity")
		}
	}
	if err := v.Binding.Validate(); err != nil {
		return err
	}
	if v.Binding.InputReview == nil {
		return fmt.Errorf("input revision requires complete original owner input")
	}
	if err := v.Report.ValidateAgainstBinding(v.Binding); err != nil {
		return err
	}
	if RuntimeValidationInputReviewConclusion(v.Report) != RuntimeValidationInputConflict {
		return fmt.Errorf("input revision requires an actual conflict, not a retry of unchanged accepted/unknown input")
	}
	if len(v.InputTargetRefs) == 0 || !reflect.DeepEqual(v.InputTargetRefs, normalizeStrings(v.InputTargetRefs)) {
		return fmt.Errorf("input revision requires exact conflict input targets")
	}
	for _, target := range v.InputTargetRefs {
		found := false
		for _, f := range v.Binding.InputReview.Files {
			found = found || f.TargetRef == target
		}
		if !found {
			return fmt.Errorf("revision target was not actually reviewed")
		}
	}
	return nil
}

func (v RuntimeValidationInputRevision) TargetRefs() []string { return slices.Clone(v.InputTargetRefs) }

// MatchesReplayProof requires a changed postimage of the exact reviewed bytes.
// Semantic consistency of that new postimage is checked by a new independent
// review, never by this effect proof.
func (v *RuntimeValidationInputRevision) MatchesReplayProof(p *RuntimeOperationReplayProof) bool {
	if v == nil || v.Validate() != nil || p == nil || p.Validate() != nil || p.OwnerModuleID != v.OwnerModuleID || len(p.TargetRefs) != 1 {
		return false
	}
	for _, f := range v.Binding.InputReview.Files {
		if slices.Contains(v.InputTargetRefs, f.TargetRef) && f.TargetRef == p.TargetRefs[0] {
			return p.BeforeHash == f.ContentHash
		}
	}
	return false
}

// CompileRuntimeValidationInputRevision derives a single exact-CWI route from
// the original admitted document and validation owner's input declaration.
// Mixed/ambiguous sources stay closed; no guessed resource owner is permitted.
func CompileRuntimeValidationInputRevision(l RuntimeSemanticPlanningLedger) (*RuntimeValidationInputRevision, error) {
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
						for _, f := range b.InputReview.Files {
							if slices.Contains(surface.InputTargetRefs, f.TargetRef) && slices.Contains(findings, f.TargetRef) {
								targets = append(targets, f.TargetRef)
							}
						}
					}
				}
			}
		}

		v = &RuntimeValidationInputRevision{WorkspaceID: l.Manifest.WorkspaceID, RunID: l.Manifest.RunID, DefinitionSeedHash: l.Manifest.DefinitionSeedHash, SourceManifestHash: l.Manifest.Hash, SourceLogicalRequestID: l.LogicalRequestID, SourceDocumentHash: StableRuntimeSemanticPlanDocumentHash(*doc), OwnerModuleID: owner, InputTargetRefs: normalizeStrings(targets), Binding: b, Report: p.ValidationInputReview.Clone()}
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

func attachRuntimeInputRevision(r RuntimeImplementationWorkScopeRequest, v *RuntimeValidationInputRevision) (RuntimeImplementationWorkScopeRequest, error) {
	r.InputRevision = v.Clone()
	r.RequestID = implementationWorkScopeRequestID(r)
	return r, r.Validate()
}

// Reverse proof is performed at every fresh aggregate/DTO load. Compilers may
// carry this detached product, but cannot create a retired source generation.
func ValidateRuntimeInputRevisionSource(v *RuntimeValidationInputRevision, l RuntimeSemanticPlanningLedger) error {
	if v == nil {
		return nil
	}
	matches := 0
	for _, g := range l.AllHistory() {
		if g.Reason != RuntimeSemanticPlanningRetirementInputRevision {
			continue
		}
		original, err := CompileRuntimeValidationInputRevision(g.Snapshot)
		if err == nil && reflect.DeepEqual(v, original) {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("input revision has no unique original rejected semantic generation")
	}
	return nil
}

func runtimeWorkInputExclusions(owner string, catalogs []RuntimeOperationAdmissibility, v *RuntimeValidationInputRevision) []string {
	excluded := runtimeValidationInputMutationExclusions(owner, RuntimeCandidateManifestOwnerFacts{OperationAdmissibilities: catalogs}, RequiredOutputContract{}, nil)
	if v != nil && v.OwnerModuleID == owner && v.Validate() == nil {
		excluded = runtimeTargetsWithoutExcluded(excluded, v.TargetRefs())
	}
	return excluded
}

func validateRuntimeInputRevisionTargets(v *RuntimeValidationInputRevision, p RuntimeSemanticProofProposal) error {
	if v == nil {
		return nil
	}
	if p.OwnerModuleID != v.OwnerModuleID || p.Target.Policy == RuntimeOperationTargetPolicyProspectiveOwnerTarget {
		return fmt.Errorf("input revision requires the original existing input owner")
	}
	for _, target := range RuntimeSemanticProposalTargetRefs(p) {
		if !slices.Contains(v.TargetRefs(), target) {
			return fmt.Errorf("input revision cannot mutate unrelated targets")
		}
	}
	return nil
}

// PendingRuntimeValidationInputRevision consumes a rejected report exactly once
// through the existing real completed work round. A new report does not reset
// any original-CWI work/provider/history budget.
func PendingRuntimeValidationInputRevision(s RuntimeSemanticPlanningLedger, w RuntimeImplementationWorkLedger, cwi common.CoverageWorkItemID) (*RuntimeValidationInputRevision, error) {
	var pending *RuntimeValidationInputRevision
	for _, g := range s.AllHistory() {
		if g.Reason != RuntimeSemanticPlanningRetirementInputRevision {
			continue
		}
		v, err := CompileRuntimeValidationInputRevision(g.Snapshot)
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

func RuntimeImplementationWorkTerminalPurposeSelected(w RuntimeImplementationWorkLedger, s RuntimeSemanticPlanningLedger, cwi common.CoverageWorkItemID) bool {
	pending, err := PendingRuntimeValidationInputRevision(s, w, cwi)
	if err != nil {
		return true
	} // malformed history cannot open a new work route
	if pending != nil {
		return false
	}
	last := RuntimeImplementationWorkPurpose("")
	for _, p := range w.PurposeChoices {
		if p.CoverageWorkItemID == cwi {
			last = p.Purpose
		}
	}
	return last == RuntimeImplementationWorkPurposeTerminal
}

func validateRuntimeInputRevisionState(def RuntimeDefinitionContext, facts RuntimeFacts, w RuntimeImplementationWorkLedger, s RuntimeSemanticPlanningLedger) error {
	// A history product may only be consumed by a later semantic transaction.
	for _, consumer := range s.AllHistory() {
		if consumer.Snapshot.Manifest == nil {
			continue
		}
		for _, o := range consumer.Snapshot.Manifest.ProofObligations {
			if o.ImplementationWork == nil || o.ImplementationWork.Scope.Request.InputRevision == nil {
				continue
			}
			v := o.ImplementationWork.Scope.Request.InputRevision
			if err := ValidateRuntimeInputRevisionSource(v, s); err != nil {
				return err
			}
			for _, source := range s.AllHistory() {
				if source.Reason == RuntimeSemanticPlanningRetirementInputRevision && source.Snapshot.Manifest != nil && source.Snapshot.Manifest.Hash == v.SourceManifestHash && source.RetiredAtStateRevision >= consumer.RetiredAtStateRevision {
					return fmt.Errorf("input revision borrowed a future conflict report")
				}
			}
		}
	}
	validate := func(r RuntimeImplementationWorkScopeRequest) error {
		if err := ValidateRuntimeInputRevisionSource(r.InputRevision, s); err != nil {
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
		if err := validate(r.ScopeRequest); err != nil {
			return err
		}
	}
	for _, g := range s.AllHistory() {
		if g.Reason != RuntimeSemanticPlanningRetirementInputRevision {
			continue
		}
		v, err := CompileRuntimeValidationInputRevision(g.Snapshot)
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
		pending, err := PendingRuntimeValidationInputRevision(s, w, v.Binding.CoverageWorkItemID)
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

// ValidateRuntimeInputRevisionRequestSources joins every retired conflict to
// the independently persisted SM-3 request. A report alone is not a completed
// provider transaction; retirement must retain the exact immutable expectation.
func ValidateRuntimeInputRevisionRequestSources(semantic RuntimeSemanticPlanningLedger, sessions []RuntimeCandidateRequestSession) error {
	for _, g := range semantic.AllHistory() {
		if g.Reason != RuntimeSemanticPlanningRetirementInputRevision {
			continue
		}
		v, err := CompileRuntimeValidationInputRevision(g.Snapshot)
		if err != nil {
			return err
		}
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
