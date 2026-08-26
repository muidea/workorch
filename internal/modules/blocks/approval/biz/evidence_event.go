package approval

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) recordApprovalGovernanceEvidence(ctx context.Context, item *approvalmodel.Approval) error {
	if m == nil || m.EventHub() == nil || item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if item.State != common.ApprovalStateApproved && item.State != common.ApprovalStateRejected {
		return common.ErrConflict
	}
	// Evidence replay is content-addressed by the owner resolution. UpdatedAt
	// changes as delivery acknowledgements advance and therefore cannot be part
	// of the immutable evidence payload.
	timestamp := strings.TrimSpace(item.ResolvedAt)
	if timestamp == "" {
		return common.ErrConflict
	}
	command := orchestratorevents.RecordApprovalEvidenceCommand{
		EvidenceRecordID: orchestratormodel.StableEvidenceRecordID(fmt.Sprintf("%s/%s/%s", item.WorkspaceID, item.RunID, item.ID), "approval", "approval"),
		WorkspaceID:      item.WorkspaceID,
		RunID:            item.RunID,
		StepID:           item.StepID,
		ApprovalID:       item.ID,
		ApprovalState:    item.State,
		ResolutionID:     item.ResolutionID,
		Principal: orchestratorevents.GovernanceEvidencePrincipal{
			Kind: string(item.ResolutionPrincipal.Kind), SubjectID: item.ResolutionPrincipal.SubjectID,
		}.Normalized(),
		Reason:            strings.TrimSpace(item.Reason),
		Action:            strings.TrimSpace(item.Action),
		Context:           item.Context.Clone().Fields,
		ResolutionContext: item.ResolutionContext.Clone().Fields,
		Timestamp:         timestamp,
	}
	return m.withWorkspaceActiveDelivery(ctx, item.WorkspaceID, func(guarded context.Context) error {
		ev := magicevent.NewEventWithContext(
			orchestratorevents.TopicOrchestratorCommandRecordApprovalEvidence,
			modulecommon.ModuleID,
			orchestratorevents.CommandDestination,
			magicevent.NewHeader(),
			guarded,
			command,
		)
		ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(orchestratorevents.Contracts(), ev.ID(), eventcontract.LaneScope{
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
		response, ok := data.(orchestratorevents.RecordApprovalEvidenceResult)
		if !ok || response.EvidenceRecordID != command.EvidenceRecordID || response.WorkspaceID != command.WorkspaceID || response.RunID != command.RunID ||
			response.ApprovalID != command.ApprovalID || response.ResolutionID != command.ResolutionID || response.ApprovalState != command.ApprovalState {
			return fmt.Errorf("%w: approval evidence result type %T", common.ErrInvalidArgument, data)
		}
		return nil
	})
}
