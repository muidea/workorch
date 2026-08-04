package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func ProjectRuntimeCheckpointView(item projectioncontract.RuntimeCheckpoint) projectioncontract.RuntimeCheckpoint {
	return projectioncontract.RuntimeCheckpoint{
		ID:                 item.ID,
		WorkspaceID:        item.WorkspaceID,
		RunID:              item.RunID,
		CheckpointType:     item.CheckpointType,
		CurrentStepID:      item.CurrentStepID,
		RunStateSnapshot:   item.RunStateSnapshot,
		BlockingTicketID:   item.BlockingTicketID,
		BlockingApprovalID: item.BlockingApprovalID,
		ThreadRef:          item.ThreadRef,
		OccurredAt:         item.OccurredAt,
		RestorableUntil:    item.RestorableUntil,
	}
}
