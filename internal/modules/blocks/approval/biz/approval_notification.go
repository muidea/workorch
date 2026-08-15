package approval

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

func (m *Biz) publishApprovalChanged(ctx context.Context, topic string, item *approvalmodel.Approval) {
	if m == nil || m.EventHub() == nil || item == nil {
		return
	}
	event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, approvalevents.NotificationDestination, magicevent.NewHeader(), ctx, approvalevents.ApprovalChangedData{
		ApprovalID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, StepID: item.StepID,
		Reason: item.Reason, Action: item.Action, Context: item.Context.Clone().Fields, State: item.State,
	})
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(approvalevents.Contracts(), topic, eventcontract.LaneScope{SubjectType: "approval", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID), StepID: string(item.StepID)}))
	m.PostEvent(event)
}
