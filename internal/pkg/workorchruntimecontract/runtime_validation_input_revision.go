package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
)

const RuntimeValidationInputRevisionProtocolV1 = "validation/input-revision/v1"
const RuntimeValidationInputRevisionRetirementOutcome = "validation_input_revision_required"

func cloneRuntimeInputRecipe(r *RuntimeValidationRecipe) *RuntimeValidationRecipe {
	if r == nil {
		return nil
	}
	c := cloneRuntimeValidationRecipeRaw(*r)
	return &c
}

// RuntimeValidationInputRevision retains the original complete rejected review
// or typed owner structural observation, with their exact original input bytes.
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
	// Recipe is the original owner declaration, including its declared deadline.
	// It describes the proof being maintained; it grants no execution authority.
	// Owners with opaque recipe identities need not expose transport metadata.
	Recipe           *RuntimeValidationRecipe `json:",omitempty"`
	Report           RuntimeValidationInputReview
	OwnerObservation *RuntimeValidationInputRevisionObservation `json:",omitempty"`
}

func (v *RuntimeValidationInputRevision) Clone() *RuntimeValidationInputRevision {
	if v == nil {
		return nil
	}
	c := *v
	c.InputTargetRefs = slices.Clone(v.InputTargetRefs)
	c.Binding = v.Binding
	c.Recipe = cloneRuntimeInputRecipe(v.Recipe)
	c.Binding.RequiredCheckRefs = slices.Clone(v.Binding.RequiredCheckRefs)
	c.Binding.CheckAttestations = slices.Clone(v.Binding.CheckAttestations)
	if v.Binding.InputReview != nil {
		b := v.Binding.InputReview.Clone()
		c.Binding.InputReview = &b
	}
	c.Report = v.Report.Clone()
	c.OwnerObservation = v.OwnerObservation.Clone()
	return &c
}

func (v RuntimeValidationInputRevision) Validate() error {
	if v.Recipe != nil && (v.Recipe.ValidateDeclaration() != nil || v.Recipe.ID != v.Binding.ValidationRecipeID) {
		return fmt.Errorf("input revision requires the exact original validation recipe declaration")
	}
	if v.OwnerObservation != nil {
		return v.validateOwnerObservation()
	}
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
	if !reflect.DeepEqual(v.Recipe, v.Binding.InputReview.Recipe) {
		return fmt.Errorf("input revision method differs from original review subject")
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
		for _, f := range v.InputFiles() {
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
	for _, f := range v.InputFiles() {
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
	v, err := runtimeValidationInputRevision(l)
	if err != nil {
		return nil, err
	}
	return v, v.Validate()
}

// Project the original accepted document, not a new semantic verdict. Raw
// documents enter the semantic-ledger admission; this function is also used to
// consume the exact immutable generation supplied by an owner transition.
func runtimeValidationInputRevision(l RuntimeSemanticPlanningLedger) (*RuntimeValidationInputRevision, error) {
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
	return v, nil
}

func attachRuntimeInputRevision(r RuntimeImplementationWorkScopeRequest, v *RuntimeValidationInputRevision) (RuntimeImplementationWorkScopeRequest, error) {
	r.InputRevision = v.Clone()
	r.RequestID = implementationWorkScopeRequestID(r)
	return r, r.Validate()
}

// New input consumers must match exactly one original retired source. Formal
// immutable sources are protected by the owner transition; querying them does
// not re-admit every unrelated historical report.
func ValidateRuntimeInputRevisionSource(v *RuntimeValidationInputRevision, l RuntimeSemanticPlanningLedger) error {
	if v == nil {
		return nil
	}
	matches := 0
	for _, g := range l.AllHistory() {
		if g.Reason != RuntimeSemanticPlanningRetirementInputRevision {
			continue
		}
		if g.OwnerInputRevision != nil {
			if v.OwnerObservation == nil || !reflect.DeepEqual(g.OwnerInputRevision, v) {
				continue
			}
		} else if v.OwnerObservation != nil || g.Snapshot.Manifest == nil || g.Snapshot.Manifest.Hash != v.SourceManifestHash || g.Snapshot.LogicalRequestID != v.SourceLogicalRequestID {
			continue
		}
		original, err := runtimeValidationInputRevisionGeneration(g)
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

// This local source projection lives only in the current owner call. It holds
// original complete products, never a validation credential or retained PASS.
type runtimeInputRevisionSource struct {
	generation RuntimeSemanticPlanningGeneration
	revision   *RuntimeValidationInputRevision
}

func runtimeInputRevisionSources(s RuntimeSemanticPlanningLedger) ([]runtimeInputRevisionSource, error) {
	var result []runtimeInputRevisionSource
	for _, g := range s.AllHistory() {
		if g.Reason != RuntimeSemanticPlanningRetirementInputRevision {
			continue
		}
		v, err := runtimeValidationInputRevisionGeneration(g)
		if err != nil {
			return nil, err
		}
		result = append(result, runtimeInputRevisionSource{generation: g, revision: v})
	}
	return result, nil
}

func validateRuntimeInputRevisionSource(v *RuntimeValidationInputRevision, sources []runtimeInputRevisionSource) error {
	if v == nil {
		return nil
	}
	matches := 0
	for _, source := range sources {
		if reflect.DeepEqual(v, source.revision) {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf("input revision has no unique original rejected semantic generation")
	}
	return nil
}

// PendingRuntimeValidationInputRevision consumes formal original reports and
// current completed work. It is not raw semantic-document admission and grants
// no effect, validation credit or additional budget.
func PendingRuntimeValidationInputRevision(s RuntimeSemanticPlanningLedger, w RuntimeImplementationWorkLedger, cwi common.CoverageWorkItemID) (*RuntimeValidationInputRevision, error) {
	sources, err := runtimeInputRevisionSources(s)
	if err != nil {
		return nil, err
	}
	return pendingRuntimeValidationInputRevision(sources, w, cwi)
}

func pendingRuntimeValidationInputRevision(sources []runtimeInputRevisionSource, w RuntimeImplementationWorkLedger, cwi common.CoverageWorkItemID) (*RuntimeValidationInputRevision, error) {
	var pending *RuntimeValidationInputRevision
	for _, source := range sources {
		v := source.revision
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

func validateRuntimeInputRevisionState(def RuntimeDefinitionContext, facts RuntimeFacts, w RuntimeImplementationWorkLedger, s RuntimeSemanticPlanningLedger, session *runtimeSemanticValidationSession) error {
	sources, err := runtimeInputRevisionSources(s)
	if err != nil {
		return err
	}

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
			if err := validateRuntimeInputRevisionSource(v, sources); err != nil {
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
		if err := validateRuntimeInputRevisionSource(r.InputRevision, sources); err != nil {
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
	checked := make(map[common.CoverageWorkItemID]bool)
	for _, source := range sources {
		g, v := source.generation, source.revision
		// Immutable startup/Profile joins were judged with this exact formal
		// generation. Current pending/focus applicability is checked below.
		if !session.committedGeneration(g) {
			if err := v.Validate(); err != nil {
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
		}
		if checked[v.Binding.CoverageWorkItemID] {
			continue
		}
		checked[v.Binding.CoverageWorkItemID] = true
		pending, err := pendingRuntimeValidationInputRevision(sources, w, v.Binding.CoverageWorkItemID)
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
	return validateRuntimeInputRevisionRequestSourcesInSession(semantic, sessions, nil)
}

func validateRuntimeInputRevisionRequestSourcesInSession(semantic RuntimeSemanticPlanningLedger, sessions []RuntimeCandidateRequestSession, session *runtimeSemanticValidationSession) error {
	for _, g := range semantic.AllHistory() {
		if g.Reason != RuntimeSemanticPlanningRetirementInputRevision {
			continue
		}
		if committedRuntimeInputRevisionRequestSource(g, sessions, session) {
			continue
		}
		v, err := CompileRuntimeValidationInputRevisionGeneration(g)
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

// Both the immutable generation and its independently committed request must
// be the exact formal preimage. A changed/deleted/duplicate source is admitted
// by the original rejecting path; generation identity alone grants nothing.
func committedRuntimeInputRevisionRequestSource(g RuntimeSemanticPlanningGeneration, sessions []RuntimeCandidateRequestSession, session *runtimeSemanticValidationSession) bool {
	if !session.committedGeneration(g) {
		return false
	}
	if g.OwnerInputRevision != nil {
		return true
	}
	var before, after *RuntimeCandidateRequestSession
	id := g.Snapshot.LogicalRequestID
	for i := range session.committedPreimage.CandidateRequestSessions {
		value := &session.committedPreimage.CandidateRequestSessions[i]
		if value.LogicalRequestID == id {
			if before != nil {
				return false
			}
			before = value
		}
	}
	for i := range sessions {
		if sessions[i].LogicalRequestID == id {
			if after != nil {
				return false
			}
			after = &sessions[i]
		}
	}
	return before != nil && after != nil && runtimeRawEqual(*before, *after)
}
