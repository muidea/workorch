package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// RuntimeImplementationWorkIntentBinding retains the accepted purpose preimage.
// It narrows an existing working transaction; it is not a new plan or credit.
type RuntimeImplementationWorkIntentBinding struct {
	Source RuntimeImplementationWorkPurposeChoice `json:"source"`
	Intent *RuntimeImplementationIntent           `json:"intent"`
}

func (b *RuntimeImplementationWorkIntentBinding) Clone() *RuntimeImplementationWorkIntentBinding {
	if b == nil {
		return nil
	}
	v := *b
	v.Intent = b.Intent.Clone()
	return &v
}

func (b *RuntimeImplementationWorkIntentBinding) ValidateForRound(r RuntimeImplementationWorkRound) error {
	if b == nil || b.Intent == nil || r.ScopeObservation == nil || r.ScopeObservation.Content == nil || b.Source.Purpose != RuntimeImplementationWorkPurposeEffect || b.Source.CoverageWorkItemID != r.CoverageWorkItemID || b.Source.ScopeRequestID != r.ScopeRequest.RequestID || b.Intent.ContentHash != r.ScopeObservation.Content.Hash() || len(b.Intent.SelectedRequirementRefs) == 0 {
		return fmt.Errorf("implementation working intent requires exact accepted purpose and content")
	}
	for _, ref := range []string{b.Source.SemanticManifestHash, b.Source.LogicalRequestID, b.Source.DocumentHash, b.Intent.DeclarationRef} {
		if !implementationWorkExactRef(ref) {
			return fmt.Errorf("implementation working intent lacks original identity")
		}
	}
	return nil
}

// ResolveRuntimeImplementationWorkIntent rejects ambiguous original sources.
// Hash equality alone cannot substitute for the complete accepted document.
func ResolveRuntimeImplementationWorkIntent(r RuntimeImplementationWorkRound, semantic RuntimeSemanticPlanningLedger) (*RuntimeImplementationWorkIntentBinding, error) {
	return resolveRuntimeImplementationWorkIntentInSession(r, semantic, newRuntimeSemanticValidationSession())
}

func resolveRuntimeImplementationWorkIntentInSession(r RuntimeImplementationWorkRound, semantic RuntimeSemanticPlanningLedger, session *runtimeSemanticValidationSession) (*RuntimeImplementationWorkIntentBinding, error) {
	if r.ScopeObservation == nil {
		return nil, fmt.Errorf("work intent requires original scope observation")
	}
	var result *RuntimeImplementationWorkIntentBinding
	for _, g := range semantic.AllHistory() {
		s := g.Snapshot
		if g.Reason != RuntimeSemanticPlanningRetirementImplementationPurpose || s.Manifest == nil || s.CoverageOrderDocument == nil || len(s.Manifest.ProofObligations) != 1 {
			continue
		}
		w := s.Manifest.ProofObligations[0].ImplementationWork
		if w == nil || !w.PurposeChoiceOnly || w.Scope.Request.RequestID != r.ScopeRequest.RequestID {
			continue
		}
		if result != nil {
			return nil, fmt.Errorf("implementation intent has multiple original purpose documents")
		}
		if s.Manifest.WorkspaceID != r.WorkspaceID || s.Manifest.RunID != r.RunID || s.Manifest.DefinitionSeedHash != r.DefinitionSeedHash || s.Phase != RuntimeSemanticPlanningOrderAccepted || w.Scope.Request.CoverageWorkItemID != r.CoverageWorkItemID || !runtimeRawEqual(w.Scope, *r.ScopeObservation) || s.CoverageOrderDocument.ImplementationPurpose != RuntimeImplementationWorkPurposeEffect || s.DocumentHash != StableRuntimeSemanticPlanDocumentHash(*s.CoverageOrderDocument) {
			return nil, fmt.Errorf("implementation intent original purpose differs from round")
		}
		validated, err := validateRuntimeSemanticPlanManifestInSession(*s.Manifest, session)
		if err != nil {
			return nil, err
		}
		if err := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(*s.CoverageOrderDocument, validated); err != nil {
			return nil, err
		}
		result = &RuntimeImplementationWorkIntentBinding{Source: RuntimeImplementationWorkPurposeChoice{CoverageWorkItemID: r.CoverageWorkItemID, ScopeRequestID: r.ScopeRequest.RequestID, SemanticManifestHash: s.Manifest.Hash, LogicalRequestID: s.LogicalRequestID, DocumentHash: s.DocumentHash, Purpose: s.CoverageOrderDocument.ImplementationPurpose}, Intent: s.CoverageOrderDocument.ImplementationIntent.Clone()}
	}
	if err := result.ValidateForRound(r); err != nil {
		return nil, err
	}
	return result, nil
}

// RuntimeImplementationRoundReviewOutcome is an immutable semantic proposal,
// not a requested owner state or validation credit.
type RuntimeImplementationRoundReviewOutcome string

const (
	RuntimeImplementationRoundAddressed    RuntimeImplementationRoundReviewOutcome = "addressed"
	RuntimeImplementationRoundPartial      RuntimeImplementationRoundReviewOutcome = "partial"
	RuntimeImplementationRoundDiverged     RuntimeImplementationRoundReviewOutcome = "diverged"
	RuntimeImplementationRoundInconclusive RuntimeImplementationRoundReviewOutcome = "inconclusive"
)

type RuntimeImplementationRoundReview struct {
	DispatchID     string                                  `json:"dispatchId"`
	ReceiptSetHash string                                  `json:"receiptSetHash"`
	Outcome        RuntimeImplementationRoundReviewOutcome `json:"outcome"`
	Reason         string                                  `json:"reason"`
}

// Before snapshots contain only the preceding round's affected existing
// targets. Absent targets were prospective in its original owner scope.
// Current complete after-content is already carried by the current scope.
type RuntimeImplementationRoundReviewSource struct {
	Ordinal        int                                     `json:"ordinal"`
	DispatchID     string                                  `json:"dispatchId"`
	ReceiptSetHash string                                  `json:"receiptSetHash"`
	IntentBinding  *RuntimeImplementationWorkIntentBinding `json:"intentBinding"`
	Before         []TargetSnapshot                        `json:"before"`
	AbsentTargets  []string                                `json:"absentTargets,omitempty"`
}

func (s *RuntimeImplementationRoundReviewSource) Clone() *RuntimeImplementationRoundReviewSource {
	if s == nil {
		return nil
	}
	v := *s
	v.IntentBinding = s.IntentBinding.Clone()
	v.Before = slices.Clone(s.Before)
	v.AbsentTargets = slices.Clone(s.AbsentTargets)
	return &v
}

func (s *RuntimeImplementationRoundReviewSource) validate() error {
	if s == nil || s.IntentBinding == nil || s.IntentBinding.Intent == nil || s.Ordinal < 1 || !implementationWorkExactRef(s.DispatchID) || !implementationWorkExactRef(s.ReceiptSetHash) || len(s.Before)+len(s.AbsentTargets) == 0 || len(s.Before)+len(s.AbsentTargets) > MaxRuntimeImplementationWorkTargets {
		return fmt.Errorf("implementation review requires bounded exact preceding sources")
	}
	refs := make([]string, 0, len(s.Before))
	seen := map[string]bool{}
	for _, snapshot := range s.Before {
		if !implementationWorkExactRef(snapshot.TargetRef) || seen[snapshot.TargetRef] {
			return fmt.Errorf("duplicate or invalid review target")
		}
		seen[snapshot.TargetRef] = true
		refs = append(refs, snapshot.TargetRef)
	}
	for _, target := range s.AbsentTargets {
		if !implementationWorkExactRef(target) || seen[target] {
			return fmt.Errorf("duplicate or invalid prospective review target")
		}
		seen[target] = true
	}
	content := RuntimeImplementationWorkContent{ProtocolRef: RuntimeImplementationWorkContentV1, ManifestRevision: "before", Snapshots: s.Before}
	return content.Validate(RuntimeImplementationWorkScopeObservation{Selection: RuntimeTargetSelection{ObservedRevision: "before", TargetRefs: refs}})
}

func implementationRoundReviewSource(r RuntimeImplementationWorkRound) (*RuntimeImplementationRoundReviewSource, error) {
	if r.WorkingBinding == nil || r.ScopeObservation == nil || r.ScopeObservation.Content == nil || r.CompletedReceiptSetHash == "" || r.DispatchID == "" {
		return nil, fmt.Errorf("previous implementation review requires complete original round")
	}
	if err := r.WorkingBinding.IntentBinding.ValidateForRound(r); err != nil {
		return nil, err
	}
	s := &RuntimeImplementationRoundReviewSource{Ordinal: r.Ordinal, DispatchID: r.DispatchID, ReceiptSetHash: r.CompletedReceiptSetHash, IntentBinding: r.WorkingBinding.IntentBinding.Clone()}
	seen := map[string]bool{}
	for _, p := range r.WorkingBinding.Proposals {
		for _, target := range RuntimeSemanticProposalTargetRefs(p) {
			if seen[target] {
				continue
			}
			seen[target] = true
			found := false
			for _, snapshot := range r.ScopeObservation.Content.Snapshots {
				if snapshot.TargetRef == target {
					s.Before = append(s.Before, snapshot)
					found = true
				}
			}
			if !found {
				s.AbsentTargets = append(s.AbsentTargets, target)
			}
		}
	}
	return s, nil
}

func validateImplementationRoundReview(i *RuntimeImplementationIntent, w *RuntimeSemanticImplementationWork) error {
	if i == nil {
		return nil
	}
	if w == nil || w.PreviousRound == nil {
		if i.PreviousRoundReview != nil {
			return fmt.Errorf("unexpected previous implementation review")
		}
		return nil
	}
	r, source := i.PreviousRoundReview, w.PreviousRound
	if r == nil || r.DispatchID != source.DispatchID || r.ReceiptSetHash != source.ReceiptSetHash || strings.TrimSpace(r.Reason) == "" || len(r.Reason) > 2048 {
		return fmt.Errorf("implementation purpose requires review of exact preceding intent and receipts")
	}
	if !slices.Contains([]RuntimeImplementationRoundReviewOutcome{RuntimeImplementationRoundAddressed, RuntimeImplementationRoundPartial, RuntimeImplementationRoundDiverged, RuntimeImplementationRoundInconclusive}, r.Outcome) {
		return fmt.Errorf("unknown implementation intent review outcome")
	}
	if r.Outcome == "partial" || r.Outcome == "diverged" {
		for _, ref := range source.IntentBinding.Intent.SelectedRequirementRefs {
			found := false
			for _, review := range i.RequirementReviews {
				if review.RequirementRef == ref && review.WorkRequired {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("unfulfilled previous intent cannot disappear from original requirement gaps")
			}
			if len(i.SelectedRequirementRefs) != 0 && !slices.Contains(i.SelectedRequirementRefs, ref) {
				return fmt.Errorf("new work must retain unfulfilled preceding intent requirement")
			}
		}
	}
	return nil
}

func sameImplementationIntentBinding(a, b *RuntimeImplementationWorkIntentBinding) bool {
	return reflect.DeepEqual(a, b)
}

// RuntimeImplementationWorkObservationFailure compiles an honest factual
// observation into the existing failure surface; it never reserves work.
func RuntimeImplementationWorkObservationFailure(choice RuntimeImplementationWorkPurposeChoice, intent *RuntimeImplementationIntent) *FailureFact {
	code := ""
	switch choice.Purpose {
	case RuntimeImplementationWorkPurposeIncomplete:
		code = "implementation_work_budget_exhausted"
	case RuntimeImplementationWorkPurposeInconclusive:
		code = "implementation_work_review_inconclusive"
	default:
		return nil
	}
	if intent == nil {
		return nil
	}
	message := "implementation work budget exhausted with unresolved original requirements"
	if choice.Purpose == RuntimeImplementationWorkPurposeInconclusive {
		message = "implementation intent review could not establish a conclusive result"
	}
	return &FailureFact{Class: FailureClassNoProgressExhausted, Code: code, Signature: code + "/" + choice.SemanticManifestHash, Message: message,
		OriginalDiagnostic: &RuntimeOriginalDiagnostic{SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: RuntimeModuleLLM, OriginalClass: "implementation_work_observation", OriginalCode: string(choice.Purpose), OriginalMessage: intent.Reason, Signature: choice.DocumentHash}}
}
