package approval

import (
	"context"
	"fmt"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) recordApprovalGovernanceEvidence(ctx context.Context, item *approvalmodel.Approval) error {
	if m == nil || m.EventHub() == nil || item == nil || item.ID == "" {
		return nil
	}
	if item.State != common.ApprovalStateApproved && item.State != common.ApprovalStateRejected {
		return nil
	}
	timestamp := strings.TrimSpace(item.UpdatedAt)
	if timestamp == "" {
		timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	command := approvalevents.RecordApprovalEvidenceCommand{
		EvidenceRecordID: orchestratormodel.StableEvidenceRecordID(string(item.ID), "approval", "approval"),
		WorkspaceID:      item.WorkspaceID,
		RunID:            item.RunID,
		StepID:           item.StepID,
		ApprovalID:       item.ID,
		ApprovalState:    item.State,
		Reason:           strings.TrimSpace(item.Reason),
		Action:           strings.TrimSpace(item.Action),
		Context:          item.Context.Clone().Fields,
		Timestamp:        timestamp,
	}
	ev := magicevent.NewEventWithContext(
		approvalevents.TopicApprovalCommandRecordEvidence,
		modulecommon.ModuleID,
		approvalevents.EvidenceCommandDestination,
		magicevent.NewHeader(),
		ctx,
		command,
	)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(ev.ID(), workorchevents.EventHubLaneScope{
		SubjectType: "approval",
		SubjectID:   string(item.ID),
		WorkspaceID: string(item.WorkspaceID),
		RunID:       string(item.RunID),
		StepID:      string(item.StepID),
	}))
	result := m.SendEvent(ev)
	if result == nil {
		return fmt.Errorf("approval evidence command returned no result")
	}
	if resultErr := result.Error(); resultErr != nil {
		if resultErr.Cause != nil {
			return resultErr.Cause
		}
		return resultErr
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		if resultErr.Cause != nil {
			return resultErr.Cause
		}
		return resultErr
	}
	response, ok := data.(approvalevents.RecordApprovalEvidenceResult)
	if !ok || response.EvidenceRecordID != command.EvidenceRecordID {
		return fmt.Errorf("%w: approval evidence result type %T", common.ErrInvalidArgument, data)
	}
	return nil
}
