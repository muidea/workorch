package orchestrator

// Governance request adapter is internal to the Runtime owner Biz.

import (
	"context"
	"fmt"

	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	magicevent "github.com/muidea/magicCommon/event"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func OpenRuntimeApprovalRequest(ctx context.Context, hub magicevent.Hub, command approvalevents.OpenRuntimeApprovalRequestCommand) (approvalevents.OpenRuntimeApprovalRequestResult, error) {
	if hub == nil || command.ApprovalID == "" || command.WorkspaceID == "" || command.RunID == "" || command.DispatchID == "" ||
		command.AuthoritySnapshotID == "" || command.DependencyKind != string(runtimecontract.GovernanceApproval) || command.SubjectRef == "" || command.Reason == "" ||
		command.RequestedAction == "" || len(command.AuthorizationContext) == 0 {
		return approvalevents.OpenRuntimeApprovalRequestResult{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(
		approvalevents.TopicOpenRuntimeApprovalRequest, ModuleID, approvalevents.CommandDestination,
		magicevent.NewHeader(), ctx, command,
	))
	if result == nil {
		return approvalevents.OpenRuntimeApprovalRequestResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return approvalevents.OpenRuntimeApprovalRequestResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return approvalevents.OpenRuntimeApprovalRequestResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(approvalevents.OpenRuntimeApprovalRequestResult)
	if !ok || response.ApprovalID != command.ApprovalID || response.WorkspaceID != command.WorkspaceID || response.RunID != command.RunID ||
		response.DispatchID != command.DispatchID || response.AuthoritySnapshotID != command.AuthoritySnapshotID {
		return approvalevents.OpenRuntimeApprovalRequestResult{}, fmt.Errorf("approval block returned an invalid runtime governance request result")
	}
	return response, nil
}

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
	if _, found := governancecheck.RequestForDispatch(aggregate.GovernanceRequests, dispatch.ID, dispatch.Transaction.AuthoritySnapshotID); found {
		return aggregate, nil
	}
	dependency := dispatch.Candidate.Governance.ExternalDependency
	request := runtimecontract.RuntimeGovernanceRequestRecord{
		AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID, DispatchID: dispatch.ID,
		DependencyKind: dependency.Kind, SubjectRef: dependency.SubjectRef,
		RequestedAction: dependency.RequestedAction, RiskLevel: dependency.RiskLevel,
		AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(dependency.AuthorizationContext),
	}
	if dependency.Kind == string(runtimecontract.GovernanceApproval) {
		approvalID := common.ApprovalID("runtime-" + dispatch.ID)
		response, err := OpenRuntimeApprovalRequest(ctx, m.EventHub(), approvalevents.OpenRuntimeApprovalRequestCommand{
			ApprovalID: approvalID, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			DispatchID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID,
			DependencyKind: dependency.Kind, SubjectRef: dependency.SubjectRef, Reason: dependency.Reason,
			RequestedAction: dependency.RequestedAction, RiskLevel: dependency.RiskLevel,
			AuthorizationContext: dependency.AuthorizationContext.Clone().Fields,
		})
		if err != nil {
			return runtime.RuntimeAggregate{}, err
		}
		request.ApprovalID = response.ApprovalID
		request.AuthoritySnapshotID = response.AuthoritySnapshotID
		request.DispatchID = response.DispatchID
	} else {
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
		request.TicketID = response.TicketID
		request.AuthoritySnapshotID = response.AuthoritySnapshotID
		request.DispatchID = response.DispatchID
	}
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventGovernanceRequestObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{GovernanceRequest: &request},
	})
	return updated.Aggregate, err
}
