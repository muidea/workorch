package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
)

func ProjectRuntimeCheckpoint(item *runmodel.RuntimeCheckpoint) projectioncontract.RuntimeCheckpoint {
	if item == nil {
		return projectioncontract.RuntimeCheckpoint{}
	}
	return projectioncontract.RuntimeCheckpoint{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		RunID:              string(item.RunID),
		CheckpointType:     string(item.CheckpointType),
		CurrentStepID:      string(item.CurrentStepID),
		RunStateSnapshot:   string(item.RunStateSnapshot),
		BlockingTicketID:   string(item.BlockingTicketID),
		BlockingApprovalID: string(item.BlockingApprovalID),
		ThreadRef:          item.ThreadRef,
		OccurredAt:         item.OccurredAt,
		RestorableUntil:    item.RestorableUntil,
	}
}
