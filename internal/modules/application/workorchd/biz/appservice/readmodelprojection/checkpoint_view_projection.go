package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ProjectRuntimeCheckpointView(item readmodelcore.RuntimeCheckpoint) readmodelcore.RuntimeCheckpoint {
	return readmodelcore.RuntimeCheckpoint{
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
