package biz

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/specifications"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// DeriveDefinitionAcceptance 仅处理派生阶段；结果仍需规格审查与用户确认。
func (b *Biz) DeriveDefinitionAcceptance(ctx context.Context, id common.WorkspaceID) (workspace.DefinitionDerivedAcceptance, error) {
	view, err := b.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: id})
	if err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	if view.Record == nil {
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
	// 历史请求只按封存身份重放，不依赖已变化或删除的现行模板。
	for _, previous := range record.DerivationRequests {
		if previous.RequestID == requestID {
			return derivationOutcome(previous)
		}
	}
	if !view.CurrentTemplateAvailable || record.TemplateSnapshotHash != view.CurrentTemplateSnapshotHash {
		return workspace.DefinitionDerivedAcceptance{}, common.ErrConflict
	}
	resources := []workspace.DefinitionAuthoringResourceSnapshot(nil)
	if len(record.Input.ResourceRefs) != 0 {
		facts, err := b.AcquireDefinitionAuthoringResources(ctx, id)
		if err != nil {
			return workspace.DefinitionDerivedAcceptance{}, err
		}
		resources, err = authoringResourceSnapshotsFor(record.Input, facts)
		if err != nil {
			return workspace.DefinitionDerivedAcceptance{}, err
		}
		if len(facts) == 0 || !sameAuthoringResourceRevisions(facts, view.WorkspaceRevision) {
			return workspace.DefinitionDerivedAcceptance{}, common.ErrConflict
		}
	}
	configuration, err := b.activeDefinitionProviderConfiguration(ctx)
	if err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	providerBindingHash := definitionProviderBindingHash(configuration)
	pending := workspace.DefinitionDerivationRequest{RequestID: requestID, Input: record.Input.Clone(), Template: record.TemplateSnapshot.Clone(), Resources: resources, ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(resources), ProviderBindingHash: providerBindingHash}
	pending.PermitID = pending.ExpectedPermitID()
	command, err := specifications.DerivationCommand(pending, configuration.DefaultChat)
	if err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	command.ExpectedConfigurationRevision = configuration.ConfigurationRevision
	reserve := workspaceevents.ReserveDefinitionDerivationCommand{WorkspaceID: id, WorkspaceRevision: view.WorkspaceRevision, InputHash: record.InputHash, RequestID: requestID, Resources: resources, ResourceSnapshotHash: pending.ResourceSnapshotHash, ProviderBindingHash: providerBindingHash}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDerivationReserve, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, reserve)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceDerivationReserve, eventcontract.LaneScope{WorkspaceID: string(id)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workspace.DefinitionDerivedAcceptance{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
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
	payload, providerErr := b.completeDefinitionDocument(ctx, command)
	completion := workspaceevents.CompleteDefinitionDerivationCommand{WorkspaceID: id, PermitID: reserved.FreshPermitID}
	if providerErr != nil {
		completion.FailureCode = "provider_failed"
	} else {
		proposal, parseErr := specifications.ParseDerivedAcceptance(payload, reservedRequest.Input, &reservedRequest.Template)
		if parseErr != nil {
			completion.FailureCode = "invalid_proposal"
		} else {
			completion.Proposal = &proposal
		}
	}
	if err := ctx.Err(); err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	completeEvent := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDerivationComplete, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, completion)
	completeEvent.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceDerivationComplete, eventcontract.LaneScope{WorkspaceID: string(id)}))
	result = b.SendEvent(completeEvent)
	if result == nil {
		return workspace.DefinitionDerivedAcceptance{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	value, getErr = result.Get()
	if getErr != nil {
		return workspace.DefinitionDerivedAcceptance{}, getErr
	}
	completed, ok := value.(workspaceevents.DefinitionDerivationRequestResult)
	if !ok {
		return workspace.DefinitionDerivedAcceptance{}, common.ErrConflict
	}
	if err := completed.ValidateCompletion(completion); err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
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
	if err := request.Validate(); err != nil {
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
