package observed

import (
	"fmt"
	"time"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func BuildPhase1Snapshot(
	ws *workspacemodel.Workspace,
	r *runmodel.Run,
	step *runmodel.Step,
	ticket *decisionmodel.Ticket,
	approval *approvalmodel.Approval,
	published []events.Phase1Event,
) []SnapshotItem {
	observed := []SnapshotItem{}
	if workspaceEnvelope, ok := latestPublishedSnapshot(phase1WorkspaceSnapshotContract, phase1SnapshotRef{
		subjectID:   string(ws.ID),
		workspaceID: ws.ID,
		runID:       r.ID,
	}, published); ok {
		observed = append(observed, workspaceEnvelope)
	} else {
		observed = append(observed, mustSnapshotItem(phase1WorkspaceSnapshotContract.fallback(phase1SnapshotRef{
			subjectID:   string(ws.ID),
			workspaceID: ws.ID,
			runID:       r.ID,
		}, events.WorkspacePayload("", ws.State, ws.ActiveRunID))))
	}
	if runEnvelope, ok := latestPublishedSnapshot(phase1RunSnapshotContract, phase1SnapshotRef{
		subjectID:   string(r.ID),
		workspaceID: ws.ID,
		runID:       r.ID,
	}, published); ok {
		observed = append(observed, runEnvelope)
	} else {
		observed = append(observed, mustSnapshotItem(phase1RunSnapshotContract.fallback(phase1SnapshotRef{
			subjectID:   string(r.ID),
			workspaceID: ws.ID,
			runID:       r.ID,
		}, events.RunPayload(r.State, r.BlockingTicketID, r.BlockingApprovalID))))
	}
	if step != nil {
		if stepEnvelope, ok := latestPublishedSnapshot(phase1StepSnapshotContract, phase1SnapshotRef{
			subjectID:   string(step.ID),
			workspaceID: ws.ID,
			runID:       r.ID,
			stepID:      step.ID,
		}, published); ok {
			observed = append(observed, stepEnvelope)
		} else {
			observed = append(observed, mustSnapshotItem(phase1StepSnapshotContract.fallback(phase1SnapshotRef{
				subjectID:   string(step.ID),
				workspaceID: ws.ID,
				runID:       r.ID,
				stepID:      step.ID,
			}, events.StepPayload(step.Kind, step.State, step.Input, step.Output))))
		}
	}
	if ticket != nil {
		if ticketEnvelope, ok := latestPublishedSnapshot(phase1TicketSnapshotContract, phase1SnapshotRef{
			subjectID:   string(ticket.ID),
			workspaceID: ws.ID,
			runID:       r.ID,
			stepID:      ticket.StepID,
		}, published); ok {
			observed = append(observed, ticketEnvelope)
		} else {
			observed = append(observed, mustSnapshotItem(phase1TicketSnapshotContract.fallback(phase1SnapshotRef{
				subjectID:   string(ticket.ID),
				workspaceID: ws.ID,
				runID:       r.ID,
				stepID:      ticket.StepID,
			}, events.DecisionTicketPayload(ticket.Reason, ticket.State))))
		}
	}
	if approval != nil {
		if approvalEnvelope, ok := latestPublishedSnapshot(phase1ApprovalSnapshotContract, phase1SnapshotRef{
			subjectID:   string(approval.ID),
			workspaceID: ws.ID,
			runID:       r.ID,
			stepID:      approval.StepID,
		}, published); ok {
			observed = append(observed, approvalEnvelope)
		} else {
			observed = append(observed, mustSnapshotItem(phase1ApprovalSnapshotContract.fallback(phase1SnapshotRef{
				subjectID:   string(approval.ID),
				workspaceID: ws.ID,
				runID:       r.ID,
				stepID:      approval.StepID,
			}, events.ApprovalPayload(approval.Reason, approval.State))))
		}
	}
	return observed
}

func mustSnapshotItem(envelope events.Envelope) SnapshotItem {
	item, ok := FromEnvelope(envelope)
	if !ok {
		panic(fmt.Sprintf("invalid phase1 snapshot envelope topic=%s", envelope.Topic))
	}
	return item
}

func newEnvelope(topic, subjectType, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, payload map[string]any) events.Envelope {
	return events.Envelope{
		ID:           fmt.Sprintf("%s:%s", topic, subjectID),
		Version:      events.Phase1SnapshotVersion,
		Topic:        topic,
		OccurredAt:   time.Now().UTC().Format(time.RFC3339),
		SourceModule: "application/workorchd",
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		StepID:       string(stepID),
		Payload:      payload,
	}
}
