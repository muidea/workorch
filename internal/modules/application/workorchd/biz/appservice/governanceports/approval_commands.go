package governanceports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func ApproveApprovalItem(ctx context.Context, hub magicevent.Hub, item *approvalmodel.Approval) (*approvalmodel.Approval, error) {
	return resolveApprovalItem(ctx, hub, approvalevents.TopicApprovalCommandEnsureApproved, item, workorcheventpayload.Payload{})
}

func RejectApprovalItem(ctx context.Context, hub magicevent.Hub, item *approvalmodel.Approval) (*approvalmodel.Approval, error) {
	return resolveApprovalItem(ctx, hub, approvalevents.TopicApprovalCommandEnsureRejected, item, workorcheventpayload.Payload{})
}

func ResolveApprovalItemWithContext(ctx context.Context, hub magicevent.Hub, item *approvalmodel.Approval, action string, resolutionContext workorcheventpayload.Payload) (*approvalmodel.Approval, error) {
	if action == "approve" {
		return resolveApprovalItem(ctx, hub, approvalevents.TopicApprovalCommandEnsureApproved, item, resolutionContext)
	}
	return resolveApprovalItem(ctx, hub, approvalevents.TopicApprovalCommandEnsureRejected, item, resolutionContext)
}

func resolveApprovalItem(ctx context.Context, hub magicevent.Hub, topic string, item *approvalmodel.Approval, resolutionContext workorcheventpayload.Payload) (*approvalmodel.Approval, error) {
	if item == nil || item.ID == "" {
		return nil, common.ErrInvalidArgument
	}
	if topic == approvalevents.TopicApprovalCommandEnsureApproved {
		return resolveApproved(ctx, hub, approvalevents.EnsureApprovedCommand{
			ApprovalID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID,
			Principal:         approvalmodel.ApprovalResolutionPrincipal{Kind: approvalmodel.ApprovalResolutionPrincipalHumanOperator, SubjectID: "application/workorchd/appservice"},
			ResolutionContext: resolutionContext.Clone().Fields,
		})
	}
	return resolveRejected(ctx, hub, approvalevents.EnsureRejectedCommand{
		ApprovalID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID,
		Principal:         approvalmodel.ApprovalResolutionPrincipal{Kind: approvalmodel.ApprovalResolutionPrincipalHumanOperator, SubjectID: "application/workorchd/appservice"},
		ResolutionContext: resolutionContext.Clone().Fields,
	})
}

func resolveApproved(ctx context.Context, hub magicevent.Hub, cmd approvalevents.EnsureApprovedCommand) (*approvalmodel.Approval, error) {
	topic := approvalevents.TopicApprovalCommandEnsureApproved
	event := magicevent.NewEventWithContext(topic, "application/workorchd/appservice", approvalevents.CommandDestination, magicevent.NewHeader(), ctx, cmd)
	return resolveApprovalEvent[approvalevents.EnsureApprovedResult](hub, topic, event, cmd.ApprovalID, cmd.WorkspaceID, cmd.RunID)
}

func resolveRejected(ctx context.Context, hub magicevent.Hub, cmd approvalevents.EnsureRejectedCommand) (*approvalmodel.Approval, error) {
	topic := approvalevents.TopicApprovalCommandEnsureRejected
	event := magicevent.NewEventWithContext(topic, "application/workorchd/appservice", approvalevents.CommandDestination, magicevent.NewHeader(), ctx, cmd)
	return resolveApprovalEvent[approvalevents.EnsureRejectedResult](hub, topic, event, cmd.ApprovalID, cmd.WorkspaceID, cmd.RunID)
}

type approvalResolutionResult interface {
	approvalevents.EnsureApprovedResult | approvalevents.EnsureRejectedResult
}

func resolveApprovalEvent[Response approvalResolutionResult](hub magicevent.Hub, topic string, event magicevent.Event, approvalID common.ApprovalID, workspaceID common.WorkspaceID, runID common.RunID) (*approvalmodel.Approval, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	ownerevent.BindLane(event, approvalevents.Contracts(), topic, "approval", string(approvalID), workspaceID, runID, "")
	response, err := ownerevent.Approval[Response](hub.Send(event), topic)
	if err != nil {
		return nil, err
	}
	var item *approvalmodel.Approval
	switch value := any(response).(type) {
	case approvalevents.EnsureApprovedResult:
		item = value.Approval
	case approvalevents.EnsureRejectedResult:
		item = value.Approval
	}
	if item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}
