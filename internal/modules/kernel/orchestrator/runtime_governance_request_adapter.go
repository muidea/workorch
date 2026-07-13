package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func OpenRuntimeGovernanceRequest(ctx context.Context, hub magicevent.Hub, command orchestratorevents.OpenRuntimeGovernanceRequestCommand) (orchestratorevents.OpenRuntimeGovernanceRequestResult, error) {
	if hub == nil || command.TicketID == "" || command.WorkspaceID == "" || command.RunID == "" || command.DispatchID == "" ||
		command.Transaction.AuthoritySnapshotID == "" || command.Dependency.Kind == "" || command.Dependency.Reason == "" {
		return orchestratorevents.OpenRuntimeGovernanceRequestResult{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorOpenRuntimeGovernanceRequest,
		ModuleID,
		runtimecontract.RuntimeModuleDecision,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if result == nil {
		return orchestratorevents.OpenRuntimeGovernanceRequestResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return orchestratorevents.OpenRuntimeGovernanceRequestResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return orchestratorevents.OpenRuntimeGovernanceRequestResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(orchestratorevents.OpenRuntimeGovernanceRequestResult)
	if !ok || response.TicketID != command.TicketID || response.WorkspaceID != command.WorkspaceID || response.RunID != command.RunID ||
		response.DispatchID != command.DispatchID || response.AuthoritySnapshotID != command.Transaction.AuthoritySnapshotID {
		return orchestratorevents.OpenRuntimeGovernanceRequestResult{}, fmt.Errorf("decision block returned an invalid runtime governance request result")
	}
	return response, nil
}

func (m *Module) prepareRuntimeV2GovernanceRequest(ctx context.Context, aggregate runtimev2.RuntimeAggregate) (runtimev2.RuntimeAggregate, error) {
	if m == nil || m.bizPtr == nil || m.rawEventHub == nil || aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.Candidate.Governance == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	dispatch := aggregate.ActiveDispatch
	for _, request := range aggregate.GovernanceRequests {
		if request.DispatchID == dispatch.ID && request.AuthoritySnapshotID == dispatch.Transaction.AuthoritySnapshotID {
			return aggregate, nil
		}
	}
	dependency := dispatch.Candidate.Governance.ExternalDependency
	ticketID := common.TicketID("runtime-v2-" + dispatch.ID)
	response, err := OpenRuntimeGovernanceRequest(ctx, m.rawEventHub, orchestratorevents.OpenRuntimeGovernanceRequestCommand{
		TicketID: ticketID, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DispatchID: dispatch.ID, Transaction: dispatch.Transaction, Dependency: dependency,
		ContinueResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
		FailResolutionType:     decisioncommon.ResolutionTypeRejectCurrentPath,
	})
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	return m.bizPtr.BindRuntimeV2GovernanceRequest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeGovernanceRequestRecord{
		TicketID: response.TicketID, AuthoritySnapshotID: response.AuthoritySnapshotID, DispatchID: response.DispatchID,
		DependencyKind: dependency.Kind, SubjectRef: dependency.SubjectRef,
	})
}
