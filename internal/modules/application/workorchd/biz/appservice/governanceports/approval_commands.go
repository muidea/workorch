package governanceports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	approvalcontracts "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/contracts"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ApproveApproval(ctx context.Context, hub events.Hub, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	return resolveApproval(ctx, hub, events.TopicApprovalCommandEnsureApproved, approvalID)
}

func RejectApproval(ctx context.Context, hub events.Hub, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	return resolveApproval(ctx, hub, events.TopicApprovalCommandEnsureRejected, approvalID)
}

func resolveApproval(ctx context.Context, hub events.Hub, topic string, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	response, err := ownercommandports.PublishWithResponse(ctx, hub, topic, "approval", string(approvalID), "", "", map[string]any{
		events.PayloadKeyCommand: approvalcontracts.ResolveApprovalCommand{ApprovalID: approvalID},
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyApproval].(*approvalmodel.Approval)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}
