package decision

import (
	"context"
	"fmt"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) recordDecisionResolutionEvidence(ctx context.Context, ticket *decisionmodel.Ticket, resolution *decisionmodel.Resolution) error {
	if m == nil || m.EventHub() == nil || ticket == nil || resolution == nil || resolution.ID == "" {
		return nil
	}
	timestamp := strings.TrimSpace(ticket.UpdatedAt)
	if timestamp == "" {
		timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	command := decisionevents.RecordDecisionEvidenceCommand{
		EvidenceRecordID:   orchestratormodel.StableEvidenceRecordID(string(resolution.ID), "decision-resolution", "decision"),
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
			decisionevents.TopicDecisionCommandRecordEvidence,
			modulecommon.ModuleID,
			decisionevents.EvidenceCommandDestination,
			magicevent.NewHeader(),
			guarded,
			command,
		)
		ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(decisionevents.Contracts(), ev.ID(), eventcontract.LaneScope{
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
		response, ok := data.(decisionevents.RecordDecisionEvidenceResult)
		if !ok || response.EvidenceRecordID != command.EvidenceRecordID {
			return fmt.Errorf("%w: decision evidence result type %T", common.ErrInvalidArgument, data)
		}
		return nil
	})
}
