package biz

import (
	"context"
	"slices"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/specifications"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/definitionconfiguration"
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
		if authoringTicketDeliveryComplete(ticket) && ticket.ID == identity.TicketID && ticket.WorkspaceID == id && ticket.RunID == "" && ticket.DefinitionClarification.Class == decisionmodel.DefinitionClarificationWorkspacePreSeal && ticket.DefinitionClarification.OpenBindingHash == identity.BindingHash() && ticket.DefinitionClarification.CandidateID == identity.CandidateID && ticket.DefinitionClarification.CandidateRevision == identity.CandidateRevision && ticket.DefinitionClarification.CandidateHash == identity.CandidateHash && ticket.DefinitionClarification.ViolationOrGapCode == string(identity.ViolationCode) && ticket.DefinitionClarification.CanonicalSignatureVersion == identity.CanonicalSignatureVersion && ticket.DefinitionClarification.CanonicalSignature == identity.CanonicalSignature && ticket.DefinitionClarification.CanonicalSignatureHash == identity.CanonicalSignatureHash && ticket.DefinitionClarification.RequiredActionKind == identity.RequiredActionKind && slices.Equal(ticket.DefinitionClarification.OffendingRefs, identity.OffendingRefs) {
			result.BlockingTicketID = string(identity.TicketID)
		}
	}
	return result, nil
}

// A persisted ticket without its owner delivery marker is still a recoverable
// cut, not completed delivery. GET never repairs that cut. Retired or partially
// resolved tickets cannot be advertised as the current open blocker either.
func authoringTicketDeliveryComplete(ticket *decisionmodel.Ticket) bool {
	if ticket == nil || ticket.DefinitionClarification == nil || ticket.CreatedNotificationPublishedAt == "" || ticket.State != common.DecisionTicketStateOpen || ticket.StepID != "" || ticket.ResolvedAt != "" || ticket.ResolvedBy != "" || ticket.ResolutionNotificationPublishedAt != "" {
		return false
	}
	binding := ticket.DefinitionClarification
	return binding.ResolutionID == "" && binding.ResolutionBindingHash == "" && binding.ResolutionCandidateID == "" && binding.ResolutionCandidateRevision == 0 && binding.ResolutionCandidateHash == "" && binding.AdmissionResultHash == "" && binding.DefinitionRevision == "" && binding.StartupInputHash == "" && binding.ReplacementIntentID == "" && binding.ReplacementIntentHash == "" && binding.SuccessorDefinitionRevision == "" && binding.SuccessorStartupInputHash == ""
}

// 展示只复用纯声明检查，不生成确认身份、binding 或 credit；owner 仍独立校验。
func authoringViewVerificationApplicable(derivation workspace.DefinitionDerivationRequest, specification workspace.GoalSpecification, items []workspace.DefinitionCandidateItem) bool {
	bindings, err := definitionconfiguration.CompileAuthoring(derivation)
	return err == nil && definitionadmission.ValidateAuthoringVerificationApplicability(specification, items, bindings.ProfileContracts, workspace.DefinitionCandidateItem{CandidateItemRef: specification.GoalItemRef, Text: derivation.Input.GoalText}) == nil
}

func authoringDiagnosticMetadata(d *workspace.DefinitionAuthoringDiagnostic) *workorchmanagementcontract.DefinitionAuthoringDiagnosticMetadata {
	if d == nil {
		return nil
	}
	return &workorchmanagementcontract.DefinitionAuthoringDiagnosticMetadata{ParseError: d.ParseError, ParseErrorSHA256: d.ParseErrorSHA256, ParseErrorBytes: d.ParseErrorBytes, ParseErrorSaved: d.ParseErrorSaved, PayloadSHA256: d.PayloadSHA256, PayloadBytes: d.PayloadBytes, PayloadSaved: d.PayloadSaved, NotSavedReason: d.NotSavedReason, ProviderCode: d.ProviderCode, ProviderDocument: authoringProviderObservationMetadata(d.ProviderDocument), ProviderWire: authoringProviderObservationMetadata(d.ProviderWire), ProviderCause: authoringProviderObservationMetadata(d.ProviderCause)}
}

func definitionAuthoringView(view workspaceevents.QueryDefinitionAuthoringResult) (workorchmanagementcontract.DefinitionAuthoringView, error) {
	if view.ValidateFor(workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: view.WorkspaceID}) != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrConflict
	}
	result := workorchmanagementcontract.DefinitionAuthoringView{WorkspaceID: string(view.WorkspaceID), WorkspaceRevision: view.WorkspaceRevision, State: "workspace_created", TemplateCurrent: view.CurrentTemplateAvailable}
	result.Title, result.Description, result.DeliverableDirectory = view.Title, view.Description, view.DeliverableDirectory
	if c := view.Creation; c != nil {
		result.Creation = &workorchmanagementcontract.DefinitionAuthoringCreationStatus{RequestID: c.RequestID, InputHash: c.InputHash, DirectoryPrepared: c.DirectoryPrepared}
		if r := c.Replay; r != nil {
			result.Creation.Replay = &workorchmanagementcontract.DefinitionAuthoringCreate{RequestID: r.RequestID, Title: r.Title, Description: r.Description, TemplateID: r.Input.TemplateID, DeliverableDirectory: r.DeliverableDirectory, Input: workorchmanagementcontract.DefinitionAuthoringInput{GoalText: r.Input.GoalText, AcceptanceRequirements: slices.Clone(r.Input.AcceptanceRequirements), ResourceRefs: slices.Clone(r.Input.ResourceRefs)}}
		}
	}
	if view.Record == nil {
		return result, nil
	}
	recordModel, err := view.Record.Model()
	if err != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, err
	}
	record := &recordModel
	input := record.Input.Normalized()
	result.Input = &workorchmanagementcontract.DefinitionAuthoringInput{GoalText: input.GoalText, AcceptanceRequirements: input.AcceptanceRequirements, ResourceRefs: input.ResourceRefs}
	result.InputHash = record.InputHash
	result.InputRevision = input.Revision
	result.TemplateID = input.TemplateID
	result.TemplateSnapshotHash = record.TemplateSnapshotHash
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
		result.DerivationRequestID, result.ProviderBindingHash, result.ResourceSnapshotHash = derivation.RequestID, derivation.ProviderBindingHash, derivation.ResourceSnapshotHash
		result.State = "deriving"
		for _, resource := range derivation.Resources {
			result.Resources = append(result.Resources, workorchmanagementcontract.DefinitionAuthoringResource{Ref: resource.Ref, SnapshotHash: resource.ContentHash})
		}
		switch {
		case derivation.FailureCode != "":
			result.Requests = append(result.Requests, workorchmanagementcontract.DefinitionAuthoringRequestState{PermitID: derivation.PermitID, Phase: "derivation", State: "failed", FailureCode: derivation.FailureCode, Diagnostic: authoringDiagnosticMetadata(derivation.Diagnostic)})
			result.State = "failed"
		case derivation.Proposal == nil:
			result.Requests = append(result.Requests, workorchmanagementcontract.DefinitionAuthoringRequestState{PermitID: derivation.PermitID, Phase: "derivation", State: "outcome_unresolved"})
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
				requestState := workorchmanagementcontract.DefinitionAuthoringRequestState{PermitID: request.PermitID, Phase: string(request.Phase), ReviewKind: request.ReviewKind, State: state, FailureCode: request.FailureCode}
				requestState.Diagnostic = authoringDiagnosticMetadata(request.Diagnostic)
				requestState.RequestID, requestState.ProviderBindingHash, requestState.ProposalHash = request.RequestID, request.ProviderBindingHash, request.ProposalHash
				result.Requests = append(result.Requests, requestState)
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
				proposal.Version, proposal.GoalItemRef, proposal.GoalTextHash = spec.Version, spec.GoalItemRef, spec.GoalTextHash
				for _, dimension := range spec.Dimensions {
					proposal.Dimensions = append(proposal.Dimensions, workorchmanagementcontract.DefinitionAuthoringDimension{ID: dimension.ID, Required: dimension.Required, Activated: dimension.Activated, ResourceIDs: slices.Clone(dimension.ResourceIDs)})
				}
				for _, requirement := range spec.Requirements {
					proposal.Requirements = append(proposal.Requirements, workorchmanagementcontract.DefinitionAuthoringRequirement{ID: requirement.ID, DimensionID: requirement.DimensionID, Required: requirement.Required, SourceKind: requirement.SourceKind, SourceItemRefs: slices.Clone(requirement.SourceItemRefs), ResourceIDs: slices.Clone(requirement.ResourceIDs), Text: requirement.Text})
				}
				for _, obligation := range spec.Obligations {
					proposal.Obligations = append(proposal.Obligations, workorchmanagementcontract.DefinitionAuthoringObligation{ID: obligation.ID, RequirementID: obligation.RequirementID, DoDItemRefs: slices.Clone(obligation.DoDItemRefs), Text: obligation.Text})
				}
				proposal.ProposalHash = spec.ContentHash()
				for _, mapping := range spec.EvidencePurposeMappings {
					proposal.EvidencePurposeMappings = append(proposal.EvidencePurposeMappings, workorchmanagementcontract.DefinitionAuthoringEvidencePurposeMapping{ContractRef: mapping.ContractRef, HistoricalItemRef: mapping.HistoricalItemRef, AcceptanceItemRef: mapping.AcceptanceItemRef, HistoricalProfileID: mapping.HistoricalProfileID, HistoricalDeliverableID: mapping.HistoricalDeliverableID, HistoricalProfileHash: mapping.HistoricalProfileHash, HistoricalText: mapping.HistoricalText})
				}
				for _, mapping := range spec.ExecutionProofMappings {
					proposal.ExecutionProofMappings = append(proposal.ExecutionProofMappings, workorchmanagementcontract.DefinitionAuthoringExecutionProofMapping{ContractRef: mapping.ContractRef, ConsumerItemRef: mapping.ConsumerItemRef, RuleRef: mapping.RuleRef, SourceItemRef: mapping.SourceItemRef, ConsumerGoalText: mapping.ConsumerGoalText})
				}
				for _, resource := range spec.Resources {
					proposal.Resources = append(proposal.Resources, workorchmanagementcontract.DefinitionAuthoringResource{ID: resource.ID, Ref: resource.Ref, SnapshotHash: resource.SnapshotHash, Required: resource.Required, Available: resource.Available})
				}
				for _, verification := range spec.Verifications {
					proposal.Verifications = append(proposal.Verifications, workorchmanagementcontract.DefinitionAuthoringVerification{ID: verification.ID, ObligationID: verification.ObligationID, Declaration: verification.Declaration, Status: verification.Status, ProfileID: verification.ProfileID, EvidenceRequirementID: verification.EvidenceRequirementID, Scope: verification.Scope})
				}
				result.State = "reviewing"
				reviewBlocked := false
				reviewFailed := false
				reviewUnresolved := false
				for _, request := range record.SpecificationRequests {
					if request.DerivationPermitID != derivation.PermitID || request.ProposalHash != proposal.ProposalHash {
						continue
					}
					if request.FailureCode != "" {
						reviewFailed = true
						continue
					}
					if request.Review == nil {
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
					if _, _, confirmErr := record.ConfirmableSpecificationView(derivation.PermitID, proposal.ProposalHash); confirmErr == nil && authoringViewVerificationApplicable(*derivation, reviewed, items) {
						result.State = "ready_to_confirm"
					} else {
						result.State = "review_blocked"
					}
				} else if reviewFailed {
					result.State = "failed"
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
	// 已 reserve 的未决请求优先于已知失败和准备诊断。
	// 历史准备诊断不能覆盖确认、准入或完整 ready ledger。
	for _, request := range record.DerivationRequests {
		if request.Input.InputHash() == record.InputHash && workspace.WorkspaceTemplateSnapshotHash(&request.Template) == record.TemplateSnapshotHash && request.FailureCode == "" && request.Proposal == nil {
			result.State = "outcome_unresolved"
			break
		}
	}
	if derivation != nil && derivation.Proposal != nil {
		for _, request := range record.SpecificationRequests {
			if request.DerivationPermitID == derivation.PermitID && request.FailureCode == "" && request.Proposal == nil && request.Review == nil {
				result.State = "outcome_unresolved"
				break
			}
		}
	}
	for _, diagnostic := range record.PreparationDiagnostics {
		current := view.Confirmation == nil && view.AdmissionCandidate == nil && view.ActivationAdmission == nil && authoringPreparationDiagnosticCurrent(*record, derivation, diagnostic)
		result.PreparationDiagnostics = append(result.PreparationDiagnostics, authoringPreparationDiagnosticMetadata(diagnostic, current))
		if current && (result.State == "draft" || result.State == "derived" || result.State == "reviewing" || result.State == "review_blocked") {
			result.State = "preparation_failed"
		}
	}
	if view.Confirmation != nil {
		result.ConfirmationPhase = string(view.Confirmation.Phase)
		// 只投影 owner 已校验的确认身份；revision 仅约束首次准备，重放由持久 journal 校验。
		result.ConfirmationRecovery = &workorchmanagementcontract.DefinitionAuthoringConfirm{
			InputHash: view.Confirmation.InputHash, DerivationPermitID: view.Confirmation.DerivationPermitID,
			ProposalHash: view.Confirmation.ProposalHash, SpecificationHash: view.Confirmation.SpecificationHash,
			ExpectedWorkspaceRevision: view.WorkspaceRevision,
		}
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

func authoringPreparationDiagnosticCurrent(record workspace.DefinitionAuthoringRecord, derivation *workspace.DefinitionDerivationRequest, diagnostic workspace.DefinitionAuthoringPreparationDiagnostic) bool {
	if diagnostic.InputHash != record.InputHash || diagnostic.TemplateSnapshotHash != record.TemplateSnapshotHash {
		return false
	}
	for _, request := range record.DerivationRequests {
		if request.Input.InputHash() == record.InputHash && workspace.WorkspaceTemplateSnapshotHash(&request.Template) == record.TemplateSnapshotHash && request.Proposal == nil && request.FailureCode == "" {
			return false
		}
	}
	if derivation == nil {
		return diagnostic.Stage == workspace.DefinitionAuthoringPreparationDerivation && diagnostic.TargetRequestID == "derive/"+record.InputHash
	}
	if derivation.Proposal == nil || derivation.FailureCode != "" || diagnostic.DerivationPermitID != derivation.PermitID {
		return false
	}
	var proposal *workspace.GoalSpecification
	for _, request := range record.SpecificationRequests {
		if request.DerivationPermitID != derivation.PermitID {
			continue
		}
		if request.FailureCode != "" || request.Proposal == nil && request.Review == nil {
			return false
		}
		if request.Proposal != nil {
			proposal = request.Proposal
		}
	}
	if proposal == nil {
		return diagnostic.Stage == workspace.DefinitionAuthoringPreparationProposal && diagnostic.TargetRequestID == "proposal/"+derivation.PermitID
	}
	for _, kind := range specifications.ReviewKinds() {
		completed := false
		for _, request := range record.SpecificationRequests {
			if request.DerivationPermitID == derivation.PermitID && request.ReviewKind == kind && request.ProposalHash == proposal.ContentHash() && request.Review != nil {
				completed = true
				break
			}
		}
		if !completed {
			return diagnostic.Stage == workspace.DefinitionAuthoringPreparationReview && diagnostic.ReviewKind == kind && diagnostic.ProposalHash == proposal.ContentHash() && diagnostic.TargetRequestID == "review/"+kind+"/"+derivation.PermitID
		}
	}
	return false
}

func authoringPreparationDiagnosticMetadata(d workspace.DefinitionAuthoringPreparationDiagnostic, current bool) workorchmanagementcontract.DefinitionAuthoringPreparationDiagnosticMetadata {
	return workorchmanagementcontract.DefinitionAuthoringPreparationDiagnosticMetadata{
		DiagnosticID: d.DiagnosticID, WorkspaceID: string(d.WorkspaceID), InputHash: d.InputHash, TemplateSnapshotHash: d.TemplateSnapshotHash,
		ObservedWorkspaceRevision: d.ObservedWorkspaceRevision, Stage: string(d.Stage), TargetRequestID: d.TargetRequestID,
		DerivationPermitID: d.DerivationPermitID, ReviewKind: d.ReviewKind, ProposalHash: d.ProposalHash,
		PhaseCode: d.PhaseCode, ResourceRef: d.ResourceRef, ProviderBindingHash: d.ProviderBindingHash,
		CauseSHA256: d.CauseSHA256, CauseBytes: d.CauseBytes, CauseSaved: d.CauseSaved, NotSavedReason: d.NotSavedReason, Current: current,
	}
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
