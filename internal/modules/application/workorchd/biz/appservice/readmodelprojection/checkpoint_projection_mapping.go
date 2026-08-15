package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: checkpoint_activity.go

func mapRuntimeCheckpoints(items []*runmodel.RuntimeCheckpoint) []projectioncontract.RuntimeCheckpoint {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.RuntimeCheckpoint, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, projectRuntimeCheckpointWithData(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: checkpoint_data_projection.go

func projectRuntimeCheckpointWithData(item *runmodel.RuntimeCheckpoint) projectioncontract.RuntimeCheckpoint {
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
		Data:               cloneReadModelMap(item.Data.Map()),
	}
}

// source: checkpoint_projection.go

func projectRuntimeCheckpoint(item *runmodel.RuntimeCheckpoint) projectioncontract.RuntimeCheckpoint {
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
