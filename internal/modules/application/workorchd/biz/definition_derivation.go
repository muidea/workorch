package biz

import (
	"context"
	"errors"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/specifications"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// DeriveDefinitionAcceptance 仅处理派生阶段；结果仍需规格审查与用户确认。
func (b *Biz) DeriveDefinitionAcceptance(ctx context.Context, id common.WorkspaceID) (workspace.DefinitionDerivedAcceptance, error) {
	return b.deriveDefinitionAcceptance(ctx, id, "")
}

func (b *Biz) deriveDefinitionAcceptance(ctx context.Context, id common.WorkspaceID, inputHash string) (workspace.DefinitionDerivedAcceptance, error) {
	return b.deriveDefinitionAcceptanceBound(ctx, id, inputHash, nil)
}

func (b *Biz) deriveDefinitionAcceptanceBound(ctx context.Context, id common.WorkspaceID, inputHash string, expected *workorchmanagementcontract.DefinitionAuthoringCompile) (workspace.DefinitionDerivedAcceptance, error) {
	ctx, release, err := b.authoringTasks.accept(ctx)
	if err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	defer release()
	view, err := b.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: id, InputHash: inputHash})
	if err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	if view.Record == nil {
		return workspace.DefinitionDerivedAcceptance{}, common.ErrConflict
	}
	if expected != nil && (view.Record.InputHash != expected.ExpectedInputHash || view.Record.TemplateSnapshotHash != expected.ExpectedTemplateSnapshotHash || !view.CurrentTemplateAvailable || view.CurrentTemplateSnapshotHash != expected.ExpectedTemplateSnapshotHash) {
		return workspace.DefinitionDerivedAcceptance{}, common.ErrConflict
	}
	if err := view.Record.Validate(); err != nil {
		return workspace.DefinitionDerivedAcceptance{}, common.ErrConflict
	}
	recordModel, err := view.Record.Model()
	if err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	record := &recordModel
	requestID := "derive/" + record.InputHash
	for _, previous := range record.DerivationRequests {
		if previous.Input.InputHash() == record.InputHash && workspace.WorkspaceTemplateSnapshotHash(&previous.Template) == record.TemplateSnapshotHash && previous.Proposal == nil && previous.FailureCode == "" {
			return derivationOutcome(previous)
		}
	}
	// 历史请求只按封存身份重放，不依赖已变化或删除的现行模板。
	for _, previous := range record.DerivationRequests {
		if previous.RequestID == requestID {
			return derivationOutcome(previous)
		}
	}
	if !view.CurrentTemplateAvailable || record.TemplateSnapshotHash != view.CurrentTemplateSnapshotHash {
		return workspace.DefinitionDerivedAcceptance{}, common.ErrConflict
	}
	identity := authoringPreparationIdentity(view, workspace.DefinitionAuthoringPreparationDerivation, requestID)
	resources := []workspace.DefinitionAuthoringResourceSnapshot(nil)
	if len(record.Input.ResourceRefs) != 0 {
		facts, err := b.acquireDefinitionAuthoringResources(ctx, id, inputHash)
		if err != nil {
			var resourceFailure *authoringResourcePreparationError
			if errors.As(err, &resourceFailure) {
				identity.ResourceRef = resourceFailure.ref
			}
			return workspace.DefinitionDerivedAcceptance{}, b.captureAuthoringPreparationFailure(ctx, identity, "resources", err)
		}
		resources, err = authoringResourceSnapshotsFor(record.Input, facts)
		if err != nil {
			return workspace.DefinitionDerivedAcceptance{}, b.captureAuthoringPreparationFailure(ctx, identity, "resources", err)
		}
		if len(facts) == 0 || !sameAuthoringResourceRevisions(facts, view.WorkspaceRevision) {
			return workspace.DefinitionDerivedAcceptance{}, b.captureAuthoringPreparationFailure(ctx, identity, "resources", common.ErrConflict)
		}
	}
	configuration, err := b.activeDefinitionProviderConfiguration(ctx)
	if err != nil {
		return workspace.DefinitionDerivedAcceptance{}, b.captureAuthoringPreparationFailure(ctx, identity, "config", err)
	}
	providerBindingHash := definitionProviderBindingHash(configuration)
	identity.ProviderBindingHash = providerBindingHash
	pending := workspace.DefinitionDerivationRequest{WireContractRef: workspace.DefinitionAuthoringOwnerBoundV2, WireContractHash: workspace.AuthoringWireHash(workspace.DefinitionAuthoringOwnerBoundV2, "derive"), RequestID: requestID, Input: record.Input.Clone(), Template: record.TemplateSnapshot.Clone(), Resources: resources, ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(resources), ProviderBindingHash: providerBindingHash}
	pending.PermitID = pending.ExpectedPermitID()
	command, err := specifications.DerivationCommand(pending, configuration.DefaultChat)
	if err != nil {
		return workspace.DefinitionDerivedAcceptance{}, b.captureAuthoringPreparationFailure(ctx, identity, "render", err)
	}
	command.ExpectedConfigurationRevision = configuration.ConfigurationRevision
	if err := ctx.Err(); err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	reserve := workspaceevents.ReserveDefinitionDerivationCommand{WireContractRef: pending.WireContractRef, WireContractHash: pending.WireContractHash, WorkspaceID: id, WorkspaceRevision: view.WorkspaceRevision, InputHash: record.InputHash, RequestID: requestID, Resources: resources, ResourceSnapshotHash: pending.ResourceSnapshotHash, ProviderBindingHash: providerBindingHash}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDerivationReserve, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, reserve)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceDerivationReserve, eventcontract.LaneScope{WorkspaceID: string(id)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workspace.DefinitionDerivedAcceptance{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return workspace.DefinitionDerivedAcceptance{}, b.captureAuthoringBudgetRefusal(ctx, identity, err)
	}
	value, getErr := result.Get()
	if getErr != nil {
		return workspace.DefinitionDerivedAcceptance{}, getErr
	}
	reserved, ok := value.(workspaceevents.DefinitionDerivationRequestResult)
	if !ok {
		return workspace.DefinitionDerivedAcceptance{}, common.ErrConflict
	}
	if err := reserved.ValidateReserve(reserve); err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	reservedRequest, err := reserved.Request.Model()
	if err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	if reserved.FreshPermitID == "" {
		return derivationOutcome(reservedRequest)
	}
	response, deliveryErr := b.completeAuthoringDocument(ctx, reserved.FreshPermitID, command)
	if deliveryErr != nil {
		return workspace.DefinitionDerivedAcceptance{}, deliveryErr
	}
	settlement, cancelSettlement := authoringSettlementContext(ctx)
	defer cancelSettlement()
	ctx = context.WithValue(settlement, authoringSettlementKey{}, settlement)
	completion := workspaceevents.CompleteDefinitionDerivationCommand{WorkspaceID: id, PermitID: reserved.FreshPermitID}
	completion.Diagnostic = authoringProviderDiagnostic(response)
	if completion.Diagnostic != nil {
		completion.FailureCode = "provider_failed"
	} else {
		payload, payloadErr := authoringCompletePayload(response)
		if payloadErr != nil {
			return workspace.DefinitionDerivedAcceptance{}, payloadErr
		}
		proposal, parseErr := specifications.ParseDerivedAcceptanceFor(payload, reservedRequest)
		if parseErr != nil {
			completion.FailureCode = "invalid_proposal"
			diagnostic := authoringParserDiagnostic(parseErr, payload, response)
			completion.Diagnostic = &diagnostic
		} else {
			completion.Proposal = &proposal
			if pending.WireContractRef != "" {
				completion.SuccessAudit = &workspace.DefinitionAuthoringSuccessByteAudit{Document: authoringProviderObservation(response.ObservedDocument), Wire: authoringProviderObservation(response.Wire)}
			}
		}
	}
	completed, settlementErr := b.settleDerivation(ctx, completion)
	if settlementErr != nil {
		return workspace.DefinitionDerivedAcceptance{}, settlementErr
	}
	completedRequest, err := completed.Request.Model()
	if err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	return derivationOutcome(completedRequest)
}

func sameAuthoringResourceRevisions(facts []fileevents.AcquireAuthoringResourceResult, revision uint64) bool {
	if revision == 0 {
		return false
	}
	for _, fact := range facts {
		if fact.WorkspaceRevision != revision {
			return false
		}
	}
	return true
}

func derivationOutcome(request workspace.DefinitionDerivationRequest) (workspace.DefinitionDerivedAcceptance, error) {
	if err := request.ValidateView(); err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	if request.Proposal != nil {
		return *request.Clone().Proposal, nil
	}
	if request.FailureCode != "" {
		return workspace.DefinitionDerivedAcceptance{}, fmt.Errorf("%w: derivation failed: %s", common.ErrConflict, request.FailureCode)
	}
	return workspace.DefinitionDerivedAcceptance{}, fmt.Errorf("%w: derivation outcome unresolved; physical call will not be repeated", common.ErrConflict)
}
