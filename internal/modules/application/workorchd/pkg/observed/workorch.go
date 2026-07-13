package observed

import (
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func BuildWorkorchSnapshot(
	ws *workspacemodel.Workspace,
	r *runmodel.Run,
	ticket *decisionmodel.Ticket,
	resolution *decisionmodel.Resolution,
	approval *approvalmodel.Approval,
	published []eventstreamevents.EventRecordData,
) []SnapshotItem {
	observed := []SnapshotItem{}
	if workspaceEnvelope, ok := latestPublishedSnapshot(workorchWorkspaceSnapshotContract, workorchSnapshotRef{
		subjectID:            string(ws.ID),
		workspaceID:          ws.ID,
		runID:                r.ID,
		title:                ws.Title,
		description:          ws.Description,
		deliverableDirectory: ws.DeliverableDirectory,
	}, published); ok {
		observed = append(observed, workspaceEnvelope)
	}
	if runEnvelope, ok := latestPublishedSnapshot(workorchRunSnapshotContract, workorchSnapshotRef{
		subjectID:   string(r.ID),
		workspaceID: ws.ID,
		runID:       r.ID,
	}, published); ok {
		observed = append(observed, runEnvelope)
	}
	if ticket != nil {
		if ticketEnvelope, ok := latestPublishedSnapshot(workorchTicketSnapshotContract, workorchSnapshotRef{
			subjectID:   string(ticket.ID),
			workspaceID: ws.ID,
			runID:       r.ID,
			stepID:      ticket.StepID,
		}, published); ok {
			observed = append(observed, ticketEnvelope)
		}
	}
	if approval != nil {
		if approvalEnvelope, ok := latestPublishedSnapshot(workorchApprovalSnapshotContract, workorchSnapshotRef{
			subjectID:   string(approval.ID),
			workspaceID: ws.ID,
			runID:       r.ID,
			stepID:      approval.StepID,
		}, published); ok {
			observed = append(observed, approvalEnvelope)
		}
	}
	return observed
}
