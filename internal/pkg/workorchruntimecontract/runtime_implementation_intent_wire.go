package workorchruntimecontract

import "fmt"

// Only semantic judgments cross the provider boundary. Exact current content,
// declaration and previous-round receipt identities come from the one frozen
// work manifest, never from provider echo or a guessed fallback.
type runtimeImplementationIntentPayload struct {
	RemainingWork           string                                   `json:"remainingWork"`
	Reason                  string                                   `json:"reason"`
	RequirementReviews      []RuntimeImplementationRequirementReview `json:"requirementReviews,omitempty"`
	SelectedRequirementRefs []string                                 `json:"selectedRequirementRefs,omitempty"`
	PreviousRoundReview     *runtimeImplementationRoundReviewPayload `json:"previousRoundReview,omitempty"`
}
type runtimeImplementationRoundReviewPayload struct {
	Outcome RuntimeImplementationRoundReviewOutcome `json:"outcome"`
	Reason  string                                  `json:"reason"`
}

func runtimeImplementationIntentToWire(i *RuntimeImplementationIntent) *runtimeImplementationIntentPayload {
	if i == nil {
		return nil
	}
	i = i.Clone()
	w := &runtimeImplementationIntentPayload{RemainingWork: i.RemainingWork, Reason: i.Reason, RequirementReviews: i.RequirementReviews, SelectedRequirementRefs: i.SelectedRequirementRefs}
	if p := i.PreviousRoundReview; p != nil {
		w.PreviousRoundReview = &runtimeImplementationRoundReviewPayload{Outcome: p.Outcome, Reason: p.Reason}
	}
	return w
}
func (w *runtimeImplementationIntentPayload) compile(m RuntimeSemanticPlanManifest) (*RuntimeImplementationIntent, error) {
	if w == nil {
		return nil, nil
	}
	var work *RuntimeSemanticImplementationWork
	declaration := ""
	for _, o := range m.ProofObligations {
		if o.ImplementationWork != nil && o.ImplementationWork.PurposeChoiceOnly {
			if work != nil {
				return nil, fmt.Errorf("implementation intent has ambiguous work source")
			}
			work = o.ImplementationWork
			declaration = o.DeclarationRef
		}
	}
	if work == nil || work.Scope.Content == nil {
		return nil, fmt.Errorf("implementation intent lacks frozen current work content")
	}
	i := &RuntimeImplementationIntent{DeclarationRef: declaration, ContentHash: work.Scope.Content.Hash(), RemainingWork: w.RemainingWork, Reason: w.Reason, RequirementReviews: w.RequirementReviews, SelectedRequirementRefs: w.SelectedRequirementRefs}
	if p := w.PreviousRoundReview; p != nil {
		if work.PreviousRound == nil {
			return nil, fmt.Errorf("previous review has no frozen preceding round")
		}
		i.PreviousRoundReview = &RuntimeImplementationRoundReview{DispatchID: work.PreviousRound.DispatchID, ReceiptSetHash: work.PreviousRound.ReceiptSetHash, Outcome: p.Outcome, Reason: p.Reason}
	}
	return i.Clone(), nil
}
