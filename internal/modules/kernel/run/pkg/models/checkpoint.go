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
