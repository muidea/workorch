package orchestrator

// Governance request adapter is internal to the Runtime owner Biz.

import (
	"context"
	"fmt"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	magicevent "github.com/muidea/magicCommon/event"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func OpenRuntimeGovernanceRequest(ctx context.Context, hub magicevent.Hub, command decisionevents.OpenRuntimeGovernanceRequestCommand) (decisionevents.OpenRuntimeGovernanceRequestResult, error) {
	if hub == nil || command.TicketID == "" || command.WorkspaceID == "" || command.RunID == "" || command.DispatchID == "" ||
		command.Transaction.AuthoritySnapshotID == "" || command.Dependency.Kind == "" || command.Dependency.Reason == "" {
		return decisionevents.OpenRuntimeGovernanceRequestResult{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(
		decisionevents.TopicOpenRuntimeGovernanceRequest,
		ModuleID,
		runtimecontract.RuntimeModuleDecision,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if result == nil {
		return decisionevents.OpenRuntimeGovernanceRequestResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return decisionevents.OpenRuntimeGovernanceRequestResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return decisionevents.OpenRuntimeGovernanceRequestResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(decisionevents.OpenRuntimeGovernanceRequestResult)
	if !ok || response.TicketID != command.TicketID || response.WorkspaceID != command.WorkspaceID || response.RunID != command.RunID ||
		response.DispatchID != command.DispatchID || response.AuthoritySnapshotID != command.Transaction.AuthoritySnapshotID {
		return decisionevents.OpenRuntimeGovernanceRequestResult{}, fmt.Errorf("decision block returned an invalid runtime governance request result")
	}
	return response, nil
}

func (m *Biz) prepareRuntimeGovernanceRequest(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
	if m == nil || m.runtimeController == nil || m.EventHub() == nil || aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.Candidate.Governance == nil {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	dispatch := aggregate.ActiveDispatch
	for _, request := range aggregate.GovernanceRequests {
		if request.DispatchID == dispatch.ID && request.AuthoritySnapshotID == dispatch.Transaction.AuthoritySnapshotID {
			return aggregate, nil
		}
	}
	dependency := dispatch.Candidate.Governance.ExternalDependency
	ticketID := common.TicketID("runtime-" + dispatch.ID)
	response, err := OpenRuntimeGovernanceRequest(ctx, m.EventHub(), decisionevents.OpenRuntimeGovernanceRequestCommand{
		TicketID: ticketID, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DispatchID: dispatch.ID, Transaction: dispatch.Transaction, Dependency: dependency,
		ContinueResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
		FailResolutionType:     decisioncommon.ResolutionTypeRejectCurrentPath,
	})
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	request := runtimecontract.RuntimeGovernanceRequestRecord{
		TicketID: response.TicketID, AuthoritySnapshotID: response.AuthoritySnapshotID, DispatchID: response.DispatchID,
		DependencyKind: dependency.Kind, SubjectRef: dependency.SubjectRef,
	}
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventGovernanceRequestObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, GovernanceRequest: &request,
	})
	return updated.Aggregate, err
}
