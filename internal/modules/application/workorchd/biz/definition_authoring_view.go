package biz

import (
	"context"
	"slices"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func (b *Biz) ReadDefinitionAuthoringView(ctx context.Context, id common.WorkspaceID) (workorchmanagementcontract.DefinitionAuthoringView, error) {
	view, err := b.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: id})
	if err != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, err
	}
	result, err := definitionAuthoringView(view)
	if err != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, err
	}
	if view.AdmissionCandidate != nil && view.AdmissionResult != nil && view.AdmissionResult.Disposition == workspace.DefinitionAdmissionClarificationRequired {
		binding := workspaceevents.DefinitionAdmissionClarificationBinding{Candidate: *view.AdmissionCandidate, AdmissionResult: *view.AdmissionResult}
		identity, err := binding.DecisionRequestIdentity()
		if err != nil {
			return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrConflict
		}
		ticket, err := governanceports.ReadDecisionTicket(common.ContextWithRuntimeWorkspaceID(ctx, id), b.EventHub(), identity.TicketID)
		if err != nil {
			return result, nil // Workspace candidate remains queryable while Decision is unavailable.
		}
		if ticket != nil && ticket.ID == identity.TicketID && ticket.WorkspaceID == id && ticket.RunID == "" && ticket.DefinitionClarification != nil && ticket.DefinitionClarification.Class == decisionmodel.DefinitionClarificationWorkspacePreSeal && ticket.DefinitionClarification.OpenBindingHash == identity.BindingHash() && ticket.DefinitionClarification.CandidateID == identity.CandidateID && ticket.DefinitionClarification.CandidateRevision == identity.CandidateRevision && ticket.DefinitionClarification.CandidateHash == identity.CandidateHash && ticket.DefinitionClarification.ViolationOrGapCode == string(identity.ViolationCode) && ticket.DefinitionClarification.CanonicalSignatureVersion == identity.CanonicalSignatureVersion && ticket.DefinitionClarification.CanonicalSignature == identity.CanonicalSignature && ticket.DefinitionClarification.CanonicalSignatureHash == identity.CanonicalSignatureHash && ticket.DefinitionClarification.RequiredActionKind == identity.RequiredActionKind && slices.Equal(ticket.DefinitionClarification.OffendingRefs, identity.OffendingRefs) {
			result.BlockingTicketID = string(identity.TicketID)
		}
	}
	return result, nil
}

func definitionAuthoringView(view workspaceevents.QueryDefinitionAuthoringResult) (workorchmanagementcontract.DefinitionAuthoringView, error) {
	if view.ValidateFor(workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: view.WorkspaceID}) != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrConflict
	}
	result := workorchmanagementcontract.DefinitionAuthoringView{WorkspaceID: string(view.WorkspaceID), WorkspaceRevision: view.WorkspaceRevision, State: "workspace_created", TemplateCurrent: view.CurrentTemplateAvailable}
	if view.Record == nil {
		return result, nil
	}
	record := view.Record
	input := record.Input.Normalized()
	result.Input = &workorchmanagementcontract.DefinitionAuthoringInput{GoalText: input.GoalText, AcceptanceRequirements: input.AcceptanceRequirements, ResourceRefs: input.ResourceRefs}
	result.InputHash = record.InputHash
	result.InputRevision = input.Revision
	result.TemplateID = input.TemplateID
	result.TemplateCurrent = view.CurrentTemplateAvailable && view.CurrentTemplateSnapshotHash == record.TemplateSnapshotHash
	result.State = "draft"
	var derivation *workspace.DefinitionDerivationRequest
	selectedPermit := ""
	if view.Confirmation != nil {
		selectedPermit = view.Confirmation.DerivationPermitID
	}
	for n := range record.DerivationRequests {
		request := &record.DerivationRequests[n]
		if request.Input.InputHash() != record.InputHash || workspace.WorkspaceTemplateSnapshotHash(&request.Template) != record.TemplateSnapshotHash {
			continue
		}
		if selectedPermit != "" {
			if request.PermitID == selectedPermit {
				derivation = request
				break
			}
		} else if request.RequestID == "derive/"+record.InputHash {
			derivation = request
			break
		}
	}
	if derivation != nil {
		result.DerivationPermitID = derivation.PermitID
		result.State = "deriving"
		for _, resource := range derivation.Resources {
			result.Resources = append(result.Resources, workorchmanagementcontract.DefinitionAuthoringResource{Ref: resource.Ref, SnapshotHash: resource.ContentHash})
		}
		switch {
		case derivation.FailureCode != "":
			result.Requests = append(result.Requests, workorchmanagementcontract.DefinitionAuthoringRequestState{Phase: "derivation", State: "failed", FailureCode: derivation.FailureCode})
			result.State = "failed"
		case derivation.Proposal == nil:
			result.Requests = append(result.Requests, workorchmanagementcontract.DefinitionAuthoringRequestState{Phase: "derivation", State: "outcome_unresolved"})
			result.State = "outcome_unresolved"
		default:
			result.State = "derived"
			items, err := derivation.Proposal.CandidateItems(derivation.Input, &derivation.Template)
			if err != nil {
				return workorchmanagementcontract.DefinitionAuthoringView{}, err
			}
			proposal := workorchmanagementcontract.DefinitionAuthoringProposal{GoalText: input.GoalText, DerivationPermitID: derivation.PermitID}
			for _, item := range items {
				proposal.Items = append(proposal.Items, workorchmanagementcontract.DefinitionAuthoringItem{Ref: item.CandidateItemRef, Text: item.Text, SourceKind: authoringItemSourceKind(item.CandidateItemRef)})
			}
			var spec *workspace.GoalSpecification
			for _, request := range record.SpecificationRequests {
				if request.DerivationPermitID != derivation.PermitID {
					continue
				}
				state := "outcome_unresolved"
				if request.FailureCode != "" {
					state = "failed"
				} else if request.Proposal != nil || request.Review != nil {
					state = "completed"
				}
				result.Requests = append(result.Requests, workorchmanagementcontract.DefinitionAuthoringRequestState{Phase: string(request.Phase), ReviewKind: request.ReviewKind, State: state, FailureCode: request.FailureCode})
				if request.Proposal != nil {
					copy := request.Proposal.Clone()
					spec = &copy
				}
			}
			if spec == nil && len(result.Requests) > 0 {
				result.State = "outcome_unresolved"
				for _, request := range result.Requests {
					if request.State == "failed" {
						result.State = "failed"
						break
					}
				}
			}
			if spec != nil {
				proposal.ProposalHash = spec.ContentHash()
				for _, resource := range spec.Resources {
					proposal.Resources = append(proposal.Resources, workorchmanagementcontract.DefinitionAuthoringResource{ID: resource.ID, Ref: resource.Ref, SnapshotHash: resource.SnapshotHash})
				}
				for _, verification := range spec.Verifications {
					proposal.Verifications = append(proposal.Verifications, workorchmanagementcontract.DefinitionAuthoringVerification{ID: verification.ID, ObligationID: verification.ObligationID, Declaration: verification.Declaration, Status: verification.Status, ProfileID: verification.ProfileID, EvidenceRequirementID: verification.EvidenceRequirementID, Scope: verification.Scope})
				}
				result.State = "reviewing"
				reviewBlocked := false
				reviewUnresolved := false
				for _, request := range record.SpecificationRequests {
					if request.DerivationPermitID != derivation.PermitID || request.ProposalHash != proposal.ProposalHash {
						continue
					}
					if request.FailureCode != "" || request.Review == nil {
						reviewUnresolved = true
						continue
					}
					review := request.Review
					if review.Status != workspace.GoalReviewPass {
						reviewBlocked = true
					}
					proposal.Reviews = append(proposal.Reviews, workorchmanagementcontract.DefinitionAuthoringReview{Kind: review.Kind, Status: review.Status, Reason: review.Reason, EvidenceRefs: review.EvidenceRefs, RequirementIDs: review.RequirementIDs, SpecificationHash: review.SpecificationHash})
				}
				if reviewed, complete, reviewErr := compiledAuthoringSpecification(*record, *derivation); reviewErr == nil && complete {
					proposal.SpecificationHash = reviewed.AdmissionHash()
					if _, _, confirmErr := record.ConfirmableSpecification(derivation.PermitID, proposal.ProposalHash); confirmErr == nil {
						result.State = "ready_to_confirm"
					} else {
						result.State = "review_blocked"
					}
				} else if reviewUnresolved {
					result.State = "outcome_unresolved"
				} else if reviewBlocked {
					result.State = "review_blocked"
				} else if reviewErr != nil {
					result.State = "outcome_unresolved"
				}
			}
			result.Proposal = &proposal
		}
	}
	if view.Confirmation != nil {
		result.ConfirmationPhase = string(view.Confirmation.Phase)
		result.State = "confirming"
		if view.Confirmation.Phase == workspace.DefinitionAuthoringConfirmationMaterialized {
			result.State = "admitting"
		}
	}
	if view.AdmissionCandidate != nil && view.AdmissionResult != nil {
		result.CandidateHash = view.AdmissionCandidate.CandidateHash
		result.AdmissionDisposition = string(view.AdmissionResult.Disposition)
		result.AdmissionViolationCode = string(view.AdmissionResult.ViolationCode)
		result.AdmissionOffendingRefs = append([]string(nil), view.AdmissionResult.OffendingRefs...)
		result.State = "clarification_required"
		if view.AdmissionResult.Disposition == workspace.DefinitionAdmissionAdmittedAllOf {
			result.State = "admitting"
		}
	}
	if view.ActivationAdmission != nil {
		result.State = "admitting"
		result.ActivationState = string(view.ActivationAdmission.State)
		if view.ActivationAdmission.State == workspace.WorkspaceActivationEligible {
			result.State = "admitted"
			result.StartupInputHash = view.ActivationAdmission.StartupInputHash
		}
	}
	return result, nil
}

func authoringItemSourceKind(ref string) string {
	switch {
	case len(ref) >= len("derived-dod:user:") && ref[:len("derived-dod:user:")] == "derived-dod:user:":
		return workspace.GoalRequirementUserExplicit
	case len(ref) >= len("derived-dod:template:") && ref[:len("derived-dod:template:")] == "derived-dod:template:":
		return workspace.GoalRequirementTemplateRequired
	default:
		return workspace.GoalRequirementDerived
	}
}
