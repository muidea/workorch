package observed

import (
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/events"
)

func BuildWorkorchSnapshot(
	ws *workspacemodel.Workspace,
	r *runmodel.Run,
	steps []*runmodel.Step,
	ticket *decisionmodel.Ticket,
	resolution *decisionmodel.Resolution,
	approval *approvalmodel.Approval,
	published []events.WorkorchEvent,
) []SnapshotItem {
	observed := []SnapshotItem{}
	if workspaceEnvelope, ok := latestPublishedSnapshot(workorchWorkspaceSnapshotContract, workorchSnapshotRef{
		subjectID:        string(ws.ID),
		workspaceID:      ws.ID,
		runID:            r.ID,
		title:            ws.Title,
		description:      ws.Description,
		workingDirectory: ws.WorkingDirectory,
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
	for _, step := range steps {
		if step == nil {
			continue
		}
		if stepEnvelope, ok := latestPublishedSnapshot(workorchStepSnapshotContract, workorchSnapshotRef{
			subjectID:   string(step.ID),
			workspaceID: ws.ID,
			runID:       r.ID,
			stepID:      step.ID,
		}, published); ok {
			observed = append(observed, stepEnvelope)
		}
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
