package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const EvidenceCommandDestination = "kernel/orchestrator"

type RecordApprovalEvidenceCommand struct {
	EvidenceRecordID common.EvidenceRecordID
	WorkspaceID      common.WorkspaceID
	RunID            common.RunID
	StepID           common.StepID
	ApprovalID       common.ApprovalID
	ApprovalState    common.ApprovalState
	Reason           string
	Action           string
	Context          []workorcheventpayload.Field
	Timestamp        string
}

type RecordApprovalEvidenceResult struct {
	EvidenceRecordID common.EvidenceRecordID
}
