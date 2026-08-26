package decision

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) recordDecisionResolutionEvidence(ctx context.Context, ticket *decisionmodel.Ticket, resolution *decisionmodel.Resolution) error {
	if m == nil || m.EventHub() == nil || ticket == nil || resolution == nil || resolution.ID == "" {
		return nil
	}
	// Evidence identity is immutable, so replay must use the immutable owner
	// resolution timestamp rather than UpdatedAt (which advances when delivery
	// acknowledgements are persisted).
	timestamp := strings.TrimSpace(resolution.ResolvedAt)
	if timestamp == "" || timestamp != strings.TrimSpace(ticket.ResolvedAt) {
		return common.ErrConflict
	}
	command := orchestratorevents.RecordDecisionEvidenceCommand{
		EvidenceRecordID:   orchestratormodel.StableEvidenceRecordID(fmt.Sprintf("%s/%s/%s/%s", ticket.WorkspaceID, ticket.RunID, ticket.ID, resolution.ID), "decision-resolution", "decision"),
		WorkspaceID:        ticket.WorkspaceID,
		RunID:              ticket.RunID,
		StepID:             ticket.StepID,
		TicketID:           ticket.ID,
		ResolutionID:       resolution.ID,
		ResolutionType:     strings.TrimSpace(resolution.Type),
		DecisionKind:       strings.TrimSpace(ticket.DecisionKind),
		Reason:             strings.TrimSpace(ticket.Reason),
		Question:           strings.TrimSpace(ticket.Question),
		ResolutionPayload:  resolution.Payload.Clone().Fields,
		ContinuationAction: strings.TrimSpace(resolution.ContinuationAction),
		ResumeRunID:        resolution.ResumeRunID,
		SuccessorRunID:     resolution.SuccessorRunID,
		Timestamp:          timestamp,
	}
	return m.withWorkspaceActiveDelivery(ctx, ticket.WorkspaceID, func(guarded context.Context) error {
		ev := magicevent.NewEventWithContext(
			orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence,
			modulecommon.ModuleID,
			orchestratorevents.CommandDestination,
			magicevent.NewHeader(),
			guarded,
			command,
		)
		ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(orchestratorevents.Contracts(), ev.ID(), eventcontract.LaneScope{
			SubjectType: "decision_ticket",
			SubjectID:   string(ticket.ID),
			WorkspaceID: string(ticket.WorkspaceID),
			RunID:       string(ticket.RunID),
			StepID:      string(ticket.StepID),
		}))
		result := m.SendEvent(ev)
		if result == nil {
			return fmt.Errorf("decision evidence command returned no result")
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
		response, ok := data.(orchestratorevents.RecordDecisionEvidenceResult)
		if !ok || response.EvidenceRecordID != command.EvidenceRecordID {
			return fmt.Errorf("%w: decision evidence result type %T", common.ErrInvalidArgument, data)
		}
		return nil
	})
}
