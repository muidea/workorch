package workspace

import "fmt"

// ConfirmableSpecification reconstructs the reviewed result exclusively from
// persisted requests. A caller's copy of a proposal or review is never an
// authority for confirmation.
func (r DefinitionAuthoringRecord) ConfirmableSpecification(derivationPermitID, proposalHash string) (DefinitionDerivationRequest, GoalSpecification, error) {
	if err := r.Validate(); err != nil {
		return DefinitionDerivationRequest{}, GoalSpecification{}, err
	}
	return r.confirmableSpecification(derivationPermitID, proposalHash)
}

// 仅由本次调用内已完整校验 ledger 的入口使用，不持久缓存或接受外部校验标记。
func (r DefinitionAuthoringRecord) confirmableSpecification(derivationPermitID, proposalHash string) (DefinitionDerivationRequest, GoalSpecification, error) {
	derivation, err := r.derivationForSpecification(derivationPermitID)
	if err != nil || derivation.Input.InputHash() != r.InputHash || WorkspaceTemplateSnapshotHash(&derivation.Template) != r.TemplateSnapshotHash {
		return DefinitionDerivationRequest{}, GoalSpecification{}, fmt.Errorf("authoring confirmation requires current completed derivation")
	}
	if proposalHash == "" {
		return DefinitionDerivationRequest{}, GoalSpecification{}, fmt.Errorf("authoring confirmation requires exact proposal hash")
	}
	var proposal *GoalSpecification
	for _, request := range r.SpecificationRequests {
		if request.DerivationPermitID != derivationPermitID || request.Phase != DefinitionSpecificationProposal {
			continue
		}
		if request.FailureCode != "" || request.Proposal == nil || request.Proposal.ContentHash() != proposalHash || proposal != nil {
			return DefinitionDerivationRequest{}, GoalSpecification{}, fmt.Errorf("authoring confirmation proposal history is unresolved or ambiguous")
		}
		value := request.Proposal.Clone()
		proposal = &value
	}
	if proposal == nil {
		return DefinitionDerivationRequest{}, GoalSpecification{}, fmt.Errorf("authoring confirmation has no completed proposal")
	}
	for _, kind := range goalReviewKinds {
		var review *GoalSpecificationCompletenessReview
		for _, request := range r.SpecificationRequests {
			if request.DerivationPermitID != derivationPermitID || request.Phase != DefinitionSpecificationCritic || request.ReviewKind != kind || request.ProposalHash != proposalHash {
				continue
			}
			if review != nil || request.Review == nil || request.FailureCode != "" || request.Review.Status != GoalReviewPass {
				return DefinitionDerivationRequest{}, GoalSpecification{}, fmt.Errorf("authoring confirmation review %q is not uniquely PASS", kind)
			}
			value := *request.Clone().Review
			review = &value
		}
		if review == nil {
			return DefinitionDerivationRequest{}, GoalSpecification{}, fmt.Errorf("authoring confirmation lacks review %q", kind)
		}
		proposal.Reviews = append(proposal.Reviews, *review)
	}
	if err := ValidateAuthoringSpecification(derivation.Input, &derivation.Template, *derivation.Proposal, derivation.Resources, *proposal); err != nil {
		return DefinitionDerivationRequest{}, GoalSpecification{}, err
	}
	return derivation.Clone(), proposal.Clone(), nil
}
