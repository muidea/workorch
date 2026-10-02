package biz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/specifications"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// CompileAuthoringSpecification returns a candidate for user confirmation, not
// an admitted Definition. Each proposal and critic has its own durable permit.
func (b *Biz) CompileAuthoringSpecification(ctx context.Context, id common.WorkspaceID) (workspace.GoalSpecification, error) {
	view, err := b.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: id})
	if err != nil {
		return workspace.GoalSpecification{}, err
	}
	if view.Record == nil || !view.CurrentTemplateAvailable || view.Record.TemplateSnapshotHash != view.CurrentTemplateSnapshotHash {
		return workspace.GoalSpecification{}, common.ErrConflict
	}
	record, err := view.Record.Model()
	if err != nil {
		return workspace.GoalSpecification{}, err
	}
	var derivation *workspace.DefinitionDerivationRequest
	for n := range record.DerivationRequests {
		request := &record.DerivationRequests[n]
		if request.Input.InputHash() == record.InputHash && workspace.WorkspaceTemplateSnapshotHash(&request.Template) == record.TemplateSnapshotHash && request.Proposal != nil {
			derivation = request
		}
	}
	if derivation == nil {
		return workspace.GoalSpecification{}, fmt.Errorf("%w: derived acceptance must be persisted before specification", common.ErrConflict)
	}
	// A complete replay does not consult mutable provider configuration.
	if specification, complete, replayErr := compiledAuthoringSpecification(record, *derivation); replayErr != nil || complete {
		return specification, replayErr
	}
	configuration, err := b.activeDefinitionProviderConfiguration(ctx)
	if err != nil {
		return workspace.GoalSpecification{}, err
	}
	bindingHash := definitionProviderBindingHash(configuration)
	if derivation.ProviderBindingHash != bindingHash {
		return workspace.GoalSpecification{}, fmt.Errorf("%w: specification provider configuration changed after derivation", common.ErrConflict)
	}
	for _, previous := range record.SpecificationRequests {
		if previous.DerivationPermitID == derivation.PermitID && previous.ProviderBindingHash != bindingHash {
			return workspace.GoalSpecification{}, fmt.Errorf("%w: specification provider configuration changed between persisted stages", common.ErrConflict)
		}
	}
	proposalRequest := workspace.DefinitionAuthoringSpecificationRequest{RequestID: "proposal/" + derivation.PermitID, DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationProposal, ProviderBindingHash: bindingHash}
	proposalCommand, err := specifications.AuthoringProposalCommand(*derivation, configuration.DefaultChat)
	if err != nil {
		return workspace.GoalSpecification{}, err
	}
	proposalCommand.ExpectedConfigurationRevision = configuration.ConfigurationRevision
	outcome, err := b.completeAuthoringSpecification(ctx, id, view.Record.InputHash, proposalRequest, proposalCommand, *derivation, nil)
	if err != nil {
		return workspace.GoalSpecification{}, err
	}
	proposal := outcome.Proposal.Clone()
	for _, kind := range specifications.ReviewKinds() {
		request := workspace.DefinitionAuthoringSpecificationRequest{RequestID: "review/" + kind + "/" + derivation.PermitID, DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposal.ContentHash(), ProviderBindingHash: bindingHash}
		unreviewed := proposal.Clone()
		unreviewed.Reviews = nil
		command, err := specifications.AuthoringReviewCommand(*derivation, unreviewed, kind, configuration.DefaultChat)
		if err != nil {
			return workspace.GoalSpecification{}, err
		}
		command.ExpectedConfigurationRevision = configuration.ConfigurationRevision
		outcome, err := b.completeAuthoringSpecification(ctx, id, view.Record.InputHash, request, command, *derivation, &proposal)
		if err != nil {
			return workspace.GoalSpecification{}, err
		}
		proposal.Reviews = append(proposal.Reviews, *outcome.Review)
	}
	if err := workspace.ValidateAuthoringSpecification(derivation.Input, &derivation.Template, *derivation.Proposal, derivation.Resources, proposal); err != nil {
		return workspace.GoalSpecification{}, err
	}
	return proposal, nil
}

func compiledAuthoringSpecification(record workspace.DefinitionAuthoringRecord, derivation workspace.DefinitionDerivationRequest) (workspace.GoalSpecification, bool, error) {
	if err := record.Validate(); err != nil {
		return workspace.GoalSpecification{}, false, err
	}
	var proposal *workspace.GoalSpecification
	for _, request := range record.SpecificationRequests {
		if request.DerivationPermitID != derivation.PermitID {
			continue
		}
		if request.FailureCode != "" {
			return workspace.GoalSpecification{}, true, fmt.Errorf("%w: authoring specification %s failed (%s)", common.ErrConflict, request.Phase, request.FailureCode)
		}
		if request.Proposal == nil && request.Review == nil {
			return workspace.GoalSpecification{}, true, fmt.Errorf("%w: authoring specification outcome unresolved", common.ErrConflict)
		}
		if request.Phase != workspace.DefinitionSpecificationProposal {
			continue
		}
		if proposal != nil {
			return workspace.GoalSpecification{}, true, common.ErrConflict
		}
		copy := request.Proposal.Clone()
		proposal = &copy
	}
	if proposal == nil {
		return workspace.GoalSpecification{}, false, nil
	}
	// Incomplete generations resume only against the same proposal hash.
	proposalHash := proposal.ContentHash()
	for _, kind := range specifications.ReviewKinds() {
		var matched *workspace.GoalSpecificationCompletenessReview
		for _, request := range record.SpecificationRequests {
			if request.DerivationPermitID != derivation.PermitID || request.Phase != workspace.DefinitionSpecificationCritic || request.ReviewKind != kind {
				continue
			}
			if request.ProposalHash != proposalHash || request.FailureCode != "" || request.Review == nil || matched != nil {
				return workspace.GoalSpecification{}, true, fmt.Errorf("%w: authoring critic %s is unresolved or inconsistent", common.ErrConflict, kind)
			}
			copy := *request.Clone().Review
			matched = &copy
		}
		if matched == nil {
			return workspace.GoalSpecification{}, false, nil
		}
		proposal.Reviews = append(proposal.Reviews, *matched)
	}
	if err := workspace.ValidateAuthoringSpecification(derivation.Input, &derivation.Template, *derivation.Proposal, derivation.Resources, *proposal); err != nil {
		return workspace.GoalSpecification{}, true, err
	}
	return *proposal, true, nil
}

func (b *Biz) activeDefinitionProviderConfiguration(ctx context.Context) (llmevents.GetActiveConfigurationResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil {
		return llmevents.GetActiveConfigurationResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return llmevents.GetActiveConfigurationResult{}, err
	}
	ev := magicevent.NewEventWithContext(llmevents.TopicLLMCommandGetActiveConfiguration, ModuleID, llmevents.CommandDestination, magicevent.NewHeader(), ctx, llmevents.GetActiveConfigurationCommand{})
	result := b.SendEvent(ev)
	if result == nil {
		return llmevents.GetActiveConfigurationResult{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return llmevents.GetActiveConfigurationResult{}, err
	}
	value, err := result.Get()
	if err != nil {
		return llmevents.GetActiveConfigurationResult{}, err
	}
	configuration, ok := value.(llmevents.GetActiveConfigurationResult)
	if !ok || configuration.ConfigurationRevision == "" || configuration.DefaultChat.Validate() != nil {
		return llmevents.GetActiveConfigurationResult{}, common.ErrConflict
	}
	return configuration, nil
}

func definitionProviderBindingHash(configuration llmevents.GetActiveConfigurationResult) string {
	h := sha256.New()
	for _, field := range []string{configuration.ConfigurationRevision, configuration.DefaultChat.ProviderID, configuration.DefaultChat.ModelID} {
		_, _ = fmt.Fprintf(h, "%08x%s", len(field), field)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (b *Biz) completeAuthoringSpecification(ctx context.Context, id common.WorkspaceID, inputHash string, request workspace.DefinitionAuthoringSpecificationRequest, command llmevents.CompleteProviderCommand, derivation workspace.DefinitionDerivationRequest, proposal *workspace.GoalSpecification) (workspace.DefinitionAuthoringSpecificationRequest, error) {
	reserve := workspaceevents.ReserveAuthoringSpecificationCommand{WorkspaceID: id, InputHash: inputHash, RequestID: request.RequestID, DerivationPermitID: request.DerivationPermitID, ReviewKind: request.ReviewKind, ProposalHash: request.ProposalHash, ProviderBindingHash: request.ProviderBindingHash}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceAuthoringSpecificationReserve, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, reserve)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceAuthoringSpecificationReserve, eventcontract.LaneScope{WorkspaceID: string(id)}))
	result := b.SendEvent(ev)
	if result == nil {
		return request, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return request, err
	}
	value, err := result.Get()
	if err != nil {
		return request, err
	}
	reserved, ok := value.(workspaceevents.AuthoringSpecificationRequestResult)
	if !ok || reserved.ValidateReserve(reserve) != nil {
		return request, common.ErrConflict
	}
	if reserved.FreshPermitID == "" {
		return authoringSpecificationOutcome(reserved.Request)
	}
	payload, providerErr := b.completeDefinitionDocument(ctx, command)
	completion := workspaceevents.CompleteAuthoringSpecificationCommand{WorkspaceID: id, PermitID: reserved.FreshPermitID}
	if providerErr != nil {
		completion.FailureCode = "provider_failed"
	} else if request.Phase == workspace.DefinitionSpecificationProposal {
		parsed, parseErr := specifications.ParseAuthoringProposal(payload, derivation)
		if parseErr != nil {
			completion.FailureCode = "invalid_proposal"
			diagnostic := workspace.NewDefinitionAuthoringDiagnostic(parseErr.Error(), payload)
			completion.Diagnostic = &diagnostic
		} else {
			completion.Proposal = &parsed
		}
	} else {
		parsed, parseErr := specifications.ParseAuthoringReview(payload, request.ReviewKind, *proposal, derivation)
		if parseErr != nil {
			completion.FailureCode = "invalid_review"
			diagnostic := workspace.NewDefinitionAuthoringDiagnostic(parseErr.Error(), payload)
			completion.Diagnostic = &diagnostic
		} else {
			completion.Review = &parsed
		}
	}
	if err := ctx.Err(); err != nil {
		return request, err
	}
	ev = magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceAuthoringSpecificationComplete, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, completion)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceAuthoringSpecificationComplete, eventcontract.LaneScope{WorkspaceID: string(id)}))
	result = b.SendEvent(ev)
	if result == nil {
		return request, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return request, err
	}
	value, err = result.Get()
	if err != nil {
		return request, err
	}
	completed, ok := value.(workspaceevents.AuthoringSpecificationRequestResult)
	if !ok || completed.ValidateCompletion(completion) != nil {
		return request, common.ErrConflict
	}
	return authoringSpecificationOutcome(completed.Request)
}

func authoringSpecificationOutcome(request workspace.DefinitionAuthoringSpecificationRequest) (workspace.DefinitionAuthoringSpecificationRequest, error) {
	if err := request.Validate(); err != nil {
		return request, err
	}
	if request.FailureCode != "" {
		return request, fmt.Errorf("%w: authoring specification failed (%s)", common.ErrConflict, request.FailureCode)
	}
	if request.Proposal != nil || request.Review != nil {
		return request.Clone(), nil
	}
	return request, fmt.Errorf("%w: authoring specification outcome unresolved; refusing repeated provider call", common.ErrConflict)
}
