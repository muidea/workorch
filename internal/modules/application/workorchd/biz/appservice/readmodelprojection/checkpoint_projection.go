package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func ProjectRuntimeCheckpoint(item *runmodel.RuntimeCheckpoint) readmodelcore.RuntimeCheckpoint {
	if item == nil {
		return readmodelcore.RuntimeCheckpoint{}
	}
	return readmodelcore.RuntimeCheckpoint{
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
