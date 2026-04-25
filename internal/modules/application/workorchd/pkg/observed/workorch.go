package observed

import (
	"fmt"
	"time"

	workorchdcommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
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
		subjectID:   string(ws.ID),
		workspaceID: ws.ID,
		runID:       r.ID,
		title:       ws.Title,
	}, published); ok {
		observed = append(observed, workspaceEnvelope)
	} else {
		observed = append(observed, workorchWorkspaceSnapshotContract.fallback(workorchSnapshotRef{
			subjectID:   string(ws.ID),
			workspaceID: ws.ID,
			runID:       r.ID,
			title:       ws.Title,
		}, WorkspacePayload{
			Title:       ws.Title,
			State:       string(ws.State),
			ActiveRunID: string(ws.ActiveRunID),
		}))
	}
	if runEnvelope, ok := latestPublishedSnapshot(workorchRunSnapshotContract, workorchSnapshotRef{
		subjectID:   string(r.ID),
		workspaceID: ws.ID,
		runID:       r.ID,
	}, published); ok {
		observed = append(observed, runEnvelope)
	} else {
		observed = append(observed, workorchRunSnapshotContract.fallback(workorchSnapshotRef{
			subjectID:   string(r.ID),
			workspaceID: ws.ID,
			runID:       r.ID,
		}, RunPayload{
			GoalID:             string(r.GoalID),
			DoDID:              string(r.DoDID),
			State:              string(r.State),
			BlockingTicketID:   string(r.BlockingTicketID),
			BlockingApprovalID: string(r.BlockingApprovalID),
			FailureReason:      r.FailureReason,
		}))
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
		} else {
			observed = append(observed, workorchStepSnapshotContract.fallback(workorchSnapshotRef{
				subjectID:   string(step.ID),
				workspaceID: ws.ID,
				runID:       r.ID,
				stepID:      step.ID,
			}, StepPayload{
				Kind:   step.Kind,
				State:  string(step.State),
				Input:  cloneMap(step.Input),
				Output: cloneMap(step.Output),
			}))
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
		} else {
			ticketPayload := DecisionTicketPayload{
				Reason: ticket.Reason,
				State:  string(ticket.State),
			}
			if resolution != nil {
				ticketPayload.ResolutionID = string(resolution.ID)
				ticketPayload.ResolutionType = resolution.Type
				ticketPayload.ResolutionPayload = cloneMap(resolution.Payload)
			}
			observed = append(observed, workorchTicketSnapshotContract.fallback(workorchSnapshotRef{
				subjectID:   string(ticket.ID),
				workspaceID: ws.ID,
				runID:       r.ID,
				stepID:      ticket.StepID,
			}, ticketPayload))
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
		} else {
			observed = append(observed, workorchApprovalSnapshotContract.fallback(workorchSnapshotRef{
				subjectID:   string(approval.ID),
				workspaceID: ws.ID,
				runID:       r.ID,
				stepID:      approval.StepID,
			}, ApprovalPayload{
				Reason: approval.Reason,
				State:  string(approval.State),
			}))
		}
	}
	return observed
}

func newSnapshotItem(topic, subjectType, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, payload SnapshotPayload) SnapshotItem {
	return SnapshotItem{
		ID:           fmt.Sprintf("%s:%s", topic, subjectID),
		Version:      events.WorkorchSnapshotVersion,
		Topic:        topic,
		OccurredAt:   time.Now().UTC().Format(time.RFC3339),
		SourceModule: workorchdcommon.ModuleID,
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		StepID:       string(stepID),
		Payload:      payload,
	}
}
