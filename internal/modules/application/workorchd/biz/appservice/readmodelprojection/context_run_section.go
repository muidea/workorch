package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ContextRunSection(snapshot readmodelcore.RunSnapshotState) projectioncontract.ContextSection {
	return projectioncontract.ContextSection{
		ID:       "run",
		Kind:     "run",
		Title:    "Run",
		Priority: 100,
		State:    snapshot.Run.State,
		Summary:  snapshot.Run.FailureReason,
		EvidenceRefs: compactUniqueStrings([]string{
			snapshot.Run.ID,
			snapshot.Run.BlockingTicketID,
			snapshot.Run.BlockingApprovalID,
		}),
		Data: map[string]any{
			"id":                 snapshot.Run.ID,
			"workspaceId":        snapshot.Run.WorkspaceID,
			"goalId":             snapshot.Run.GoalID,
			"doDId":              snapshot.Run.DoDID,
			"state":              snapshot.Run.State,
			"currentStepId":      CurrentStepID(snapshot),
			"blockingTicketId":   snapshot.Run.BlockingTicketID,
			"blockingApprovalId": snapshot.Run.BlockingApprovalID,
			"failureReason":      snapshot.Run.FailureReason,
		},
	}
}
