package run

import "github.com/muidea/workorch/internal/pkg/common"
import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

type RuntimeCheckpoint struct {
	ID                 common.CheckpointID
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	CheckpointType     common.CheckpointType
	CurrentStepID      common.StepID
	RunStateSnapshot   common.RunState
	BlockingTicketID   common.TicketID
	BlockingApprovalID common.ApprovalID
	ThreadRef          string
	OccurredAt         string
	RestorableUntil    string
	Data               workorcheventpayload.Payload
}

func CloneRuntimeCheckpoint(item *RuntimeCheckpoint) *RuntimeCheckpoint {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.Data = item.Data.Clone()
	return &cloned
}

func CloneRuntimeCheckpoints(items []*RuntimeCheckpoint) []*RuntimeCheckpoint {
	if items == nil {
		return nil
	}
	ret := make([]*RuntimeCheckpoint, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneRuntimeCheckpoint(item))
	}
	return ret
}
