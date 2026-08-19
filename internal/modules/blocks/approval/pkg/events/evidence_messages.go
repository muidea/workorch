package events

import (
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const EvidenceCommandDestination = "kernel/orchestrator"

type RecordApprovalEvidenceCommand struct {
	EvidenceRecordID  common.EvidenceRecordID
	WorkspaceID       common.WorkspaceID
	RunID             common.RunID
	StepID            common.StepID
	ApprovalID        common.ApprovalID
	ApprovalState     common.ApprovalState
	ResolutionID      common.ResolutionID
	Principal         approvalmodel.ApprovalResolutionPrincipal
	Reason            string
	Action            string
	Context           []workorcheventpayload.Field
	ResolutionContext []workorcheventpayload.Field
	Timestamp         string
}

type RecordApprovalEvidenceResult struct {
	EvidenceRecordID common.EvidenceRecordID
	WorkspaceID      common.WorkspaceID
	RunID            common.RunID
	ApprovalID       common.ApprovalID
	ResolutionID     common.ResolutionID
	ApprovalState    common.ApprovalState
}
