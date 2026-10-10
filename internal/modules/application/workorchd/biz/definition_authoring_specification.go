package biz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/specifications"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// CompileAuthoringSpecification returns a candidate for user confirmation, not
// an admitted Definition. Each proposal and critic has its own durable permit.
func (b *Biz) CompileAuthoringSpecification(ctx context.Context, id common.WorkspaceID) (workspace.GoalSpecification, error) {
	ctx, release, err := b.authoringTasks.accept(ctx)
	if err != nil {
		return workspace.GoalSpecification{}, err
	}
	defer release()
	view, err := b.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: id})
	if err != nil {
		return workspace.GoalSpecification{}, err
	}
	if view.Record == nil {
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
	proposalRequest := workspace.DefinitionAuthoringSpecificationRequest{RequestID: "proposal/" + derivation.PermitID, DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationProposal}
	outcome, err := b.prepareAuthoringSpecification(ctx, id, record, proposalRequest, *derivation, nil)
	if err != nil {
		return workspace.GoalSpecification{}, err
	}
	proposal := outcome.Proposal.Clone()
	for _, kind := range specifications.ReviewKinds() {
		request := workspace.DefinitionAuthoringSpecificationRequest{RequestID: "review/" + kind + "/" + derivation.PermitID, DerivationPermitID: derivation.PermitID, Phase: workspace.DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposal.ContentHash()}
		outcome, err := b.prepareAuthoringSpecification(ctx, id, record, request, *derivation, &proposal)
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
	if err := record.ValidateView(); err != nil {
		return workspace.GoalSpecification{}, false, err
	}
	// 任意已 reserve 未决槽位优先于已知失败和准备观察，
	// 不能因此授权另一次 provider 调用。
	for _, request := range record.SpecificationRequests {
		if request.DerivationPermitID == derivation.PermitID && request.FailureCode == "" && request.Proposal == nil && request.Review == nil {
			return workspace.GoalSpecification{}, true, fmt.Errorf("%w: authoring specification outcome unresolved", common.ErrConflict)
		}
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

// prepareAuthoringSpecification 先重放 current ledger，再读取可变配置或渲染。
// 每个新 review 都重新观察 owner revision，包括前序 permit/completion 推进。
func (b *Biz) prepareAuthoringSpecification(ctx context.Context, id common.WorkspaceID, expected workspace.DefinitionAuthoringRecord, request workspace.DefinitionAuthoringSpecificationRequest, derivation workspace.DefinitionDerivationRequest, proposal *workspace.GoalSpecification) (workspace.DefinitionAuthoringSpecificationRequest, error) {
	view, err := b.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: id})
	if err != nil {
		return request, err
	}
	if view.Record == nil || view.Record.InputHash != expected.InputHash || view.Record.TemplateSnapshotHash != expected.TemplateSnapshotHash {
		return request, common.ErrConflict
	}
	record, err := view.Record.Model()
	if err != nil {
		return request, err
	}
	if _, _, replayErr := compiledAuthoringSpecification(record, derivation); replayErr != nil {
		return request, replayErr
	}
	for _, previous := range record.SpecificationRequests {
		if previous.DerivationPermitID == request.DerivationPermitID && previous.Phase == request.Phase && previous.ReviewKind == request.ReviewKind && previous.ProposalHash == request.ProposalHash {
			return authoringSpecificationOutcome(previous)
		}
	}
	if !view.CurrentTemplateAvailable || view.CurrentTemplateSnapshotHash != expected.TemplateSnapshotHash {
		return request, common.ErrConflict
	}
	stage := workspace.DefinitionAuthoringPreparationProposal
	if request.Phase == workspace.DefinitionSpecificationCritic {
		stage = workspace.DefinitionAuthoringPreparationReview
	}
	identity := authoringPreparationIdentity(view, stage, request.RequestID)
	identity.DerivationPermitID, identity.ReviewKind, identity.ProposalHash = request.DerivationPermitID, request.ReviewKind, request.ProposalHash
	configuration, err := b.activeDefinitionProviderConfiguration(ctx)
	if err != nil {
		return request, b.captureAuthoringPreparationFailure(ctx, identity, "config", err)
	}
	bindingHash := definitionProviderBindingHash(configuration)
	identity.ProviderBindingHash = bindingHash
	if derivation.ProviderBindingHash != bindingHash {
		err := fmt.Errorf("%w: specification provider configuration changed after derivation", common.ErrConflict)
		return request, b.captureAuthoringPreparationFailure(ctx, identity, "config", err)
	}
	for _, previous := range record.SpecificationRequests {
		if previous.DerivationPermitID == derivation.PermitID && previous.ProviderBindingHash != bindingHash {
			err := fmt.Errorf("%w: specification provider configuration changed between persisted stages", common.ErrConflict)
			return request, b.captureAuthoringPreparationFailure(ctx, identity, "config", err)
		}
	}
	request.ProviderBindingHash = bindingHash
	var command llmevents.CompleteProviderCommand
	if proposal == nil {
		command, err = specifications.AuthoringProposalCommand(derivation, configuration.DefaultChat)
	} else {
		unreviewed := proposal.Clone()
		unreviewed.Reviews = nil
		command, err = specifications.AuthoringReviewCommand(derivation, unreviewed, request.ReviewKind, configuration.DefaultChat)
	}
	if err != nil {
		return request, b.captureAuthoringPreparationFailure(ctx, identity, "render", err)
	}
	command.ExpectedConfigurationRevision = configuration.ConfigurationRevision
	return b.completeAuthoringSpecification(ctx, id, expected.InputHash, request, command, derivation, proposal, identity)
}

func (b *Biz) completeAuthoringSpecification(ctx context.Context, id common.WorkspaceID, inputHash string, request workspace.DefinitionAuthoringSpecificationRequest, command llmevents.CompleteProviderCommand, derivation workspace.DefinitionDerivationRequest, proposal *workspace.GoalSpecification, identity workspace.DefinitionAuthoringPreparationDiagnostic) (workspace.DefinitionAuthoringSpecificationRequest, error) {
	if err := ctx.Err(); err != nil {
		return request, err
	}
	reserve := workspaceevents.ReserveAuthoringSpecificationCommand{WorkspaceID: id, InputHash: inputHash, RequestID: request.RequestID, DerivationPermitID: request.DerivationPermitID, ReviewKind: request.ReviewKind, ProposalHash: request.ProposalHash, ProviderBindingHash: request.ProviderBindingHash}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceAuthoringSpecificationReserve, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, reserve)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceAuthoringSpecificationReserve, eventcontract.LaneScope{WorkspaceID: string(id)}))
	result := b.SendEvent(ev)
	if result == nil {
		return request, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return request, b.captureAuthoringBudgetRefusal(ctx, identity, err)
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
	response, deliveryErr := b.completeAuthoringDocument(ctx, reserved.FreshPermitID, command)
	if deliveryErr != nil {
		return request, deliveryErr
	}
	settlement, cancelSettlement := authoringSettlementContext(ctx)
	defer cancelSettlement()
	ctx = context.WithValue(settlement, authoringSettlementKey{}, settlement)
	completion := workspaceevents.CompleteAuthoringSpecificationCommand{WorkspaceID: id, PermitID: reserved.FreshPermitID}
	completion.Diagnostic = authoringProviderDiagnostic(response)
	var payload []byte
	if completion.Diagnostic == nil {
		var payloadErr error
		payload, payloadErr = authoringCompletePayload(response)
		if payloadErr != nil {
			return request, payloadErr
		}
	}
	if completion.Diagnostic != nil {
		completion.FailureCode = "provider_failed"
	} else if request.Phase == workspace.DefinitionSpecificationProposal {
		parsed, parseErr := specifications.ParseAuthoringMapping(payload)
		if parseErr != nil {
			completion.FailureCode = "invalid_proposal"
			diagnostic := authoringParserDiagnostic(parseErr, payload, response)
			completion.Diagnostic = &diagnostic
		} else {
			completion.Mapping = &parsed
			completion.SuccessAudit = &workspace.DefinitionAuthoringSuccessByteAudit{Document: authoringProviderObservation(response.ObservedDocument), Wire: authoringProviderObservation(response.Wire)}
		}
	} else {
		parsed, parseErr := specifications.ParseAuthoringReview(payload, request.ReviewKind, *proposal, derivation)
		if parseErr != nil {
			completion.FailureCode = "invalid_review"
			diagnostic := authoringParserDiagnostic(parseErr, payload, response)
			completion.Diagnostic = &diagnostic
		} else {
			completion.Review = &parsed
		}
	}
	completed, settlementErr := b.settleAuthoringSpecification(ctx, completion)
	if settlementErr != nil {
		return request, settlementErr
	}
	return authoringSpecificationOutcome(completed.Request)
}

func authoringSpecificationOutcome(request workspace.DefinitionAuthoringSpecificationRequest) (workspace.DefinitionAuthoringSpecificationRequest, error) {
	if err := request.ValidateView(); err != nil {
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
