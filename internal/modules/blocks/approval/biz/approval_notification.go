package approval

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func (m *Biz) deliverResolvedApproval(ctx context.Context, item *approvalmodel.Approval) error {
	if m == nil || m.service == nil || m.EventHub() == nil || item == nil ||
		(item.State != common.ApprovalStateApproved && item.State != common.ApprovalStateRejected) {
		return common.ErrInvalidArgument
	}
	if item.EvidenceRecordedAt == "" {
		if err := m.recordApprovalGovernanceEvidence(ctx, item); err != nil {
			return err
		}
		updated, err := m.service.MarkEvidenceRecorded(ctx, item)
		if err != nil {
			return err
		}
		item = updated
	}
	if item.ResolutionNotificationPublishedAt != "" {
		return nil
	}
	data, err := approvalResolutionNotification(item)
	if err != nil {
		return err
	}
	if err := m.recordApprovalResolution(ctx, data); err != nil {
		return err
	}
	if err := m.publishApprovalChanged(ctx, approvalevents.TopicApprovalResolved, data); err != nil {
		return err
	}
	_, err = m.service.MarkNotificationPublished(ctx, item)
	return err
}

func approvalResolutionNotification(item *approvalmodel.Approval) (approvalevents.ApprovalChangedData, error) {
	if item == nil {
		return approvalevents.ApprovalChangedData{}, common.ErrInvalidArgument
	}
	data := approvalevents.ApprovalChangedData{
		NotificationID: approvalResolvedNotificationID(item), ResolutionID: item.ResolutionID,
		ApprovalID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, StepID: item.StepID,
		Reason: item.Reason, Action: item.Action, Context: item.Context.Clone().Fields,
		ResolutionContext: item.ResolutionContext.Clone().Fields, Principal: item.ResolutionPrincipal.Normalized(), State: item.State,
	}
	if err := data.Validate(); err != nil {
		return approvalevents.ApprovalChangedData{}, err
	}
	return data, nil
}

func (m *Biz) recordApprovalResolution(ctx context.Context, data approvalevents.ApprovalChangedData) error {
	if m == nil || m.EventHub() == nil || data.Validate() != nil {
		return common.ErrInvalidArgument
	}
	return m.withWorkspaceActiveDelivery(ctx, data.WorkspaceID, func(guarded context.Context) error {
		topic := eventstreamevents.TopicEventStreamCommandRecordApprovalResolution
		command := eventstreamevents.RecordApprovalResolutionCommand{Data: eventstreamevents.ApprovalResolutionFact{
			NotificationID: data.NotificationID, ResolutionID: data.ResolutionID, ApprovalID: data.ApprovalID,
			WorkspaceID: data.WorkspaceID, RunID: data.RunID, StepID: data.StepID, Reason: data.Reason, Action: data.Action,
			Context: append([]workorcheventpayload.Field(nil), data.Context...), ResolutionContext: append([]workorcheventpayload.Field(nil), data.ResolutionContext...),
			PrincipalKind: string(data.Principal.Kind), PrincipalSubjectID: data.Principal.SubjectID, State: data.State,
		}}
		event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, eventstreamevents.CommandDestination, magicevent.NewHeader(), guarded, command)
		event.BindLaneKey(workorchevents.EventHubLaneKeyFor(eventstreamevents.Contracts(), topic, eventcontract.LaneScope{
			SubjectType: "approval", SubjectID: string(data.ApprovalID), WorkspaceID: string(data.WorkspaceID), RunID: string(data.RunID), StepID: string(data.StepID),
		}))
		result := m.SendEvent(event)
		if result == nil {
			return common.ErrInvalidArgument
		}
		if resultErr := result.Error(); resultErr != nil {
			if resultErr.Cause != nil {
				return resultErr.Cause
			}
			return resultErr
		}
		value, resultErr := result.Get()
		if resultErr != nil {
			if resultErr.Cause != nil {
				return resultErr.Cause
			}
			return resultErr
		}
		response, ok := value.(eventstreamevents.RecordApprovalResolutionResult)
		if !ok || response.NotificationID != data.NotificationID || response.ResolutionID != data.ResolutionID || response.ApprovalID != data.ApprovalID ||
			response.WorkspaceID != data.WorkspaceID || response.RunID != data.RunID {
			return common.ErrInvalidArgument
		}
		return nil
	})
}

func (m *Biz) publishApprovalChanged(ctx context.Context, topic string, data approvalevents.ApprovalChangedData) error {
	if m == nil || m.EventHub() == nil || data.Validate() != nil {
		return common.ErrInvalidArgument
	}
	return m.withWorkspaceActiveDelivery(ctx, data.WorkspaceID, func(guarded context.Context) error {
		event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, approvalevents.NotificationDestination, magicevent.NewHeader(), guarded, data)
		event.BindLaneKey(workorchevents.EventHubLaneKeyFor(approvalevents.Contracts(), topic, eventcontract.LaneScope{
			SubjectType: "approval", SubjectID: string(data.ApprovalID), WorkspaceID: string(data.WorkspaceID), RunID: string(data.RunID), StepID: string(data.StepID),
		}))
		m.PostEvent(event)
		return nil
	})
}

func approvalResolvedNotificationID(item *approvalmodel.Approval) string {
	if item == nil || item.WorkspaceID == "" || item.RunID == "" || item.ID == "" || item.ResolutionID == "" {
		return ""
	}
	return fmt.Sprintf("approval-notification/resolved/%s/%s/%s/%s", item.WorkspaceID, item.RunID, item.ID, item.ResolutionID)
}
