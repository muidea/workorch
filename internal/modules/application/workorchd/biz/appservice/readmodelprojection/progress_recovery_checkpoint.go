package readmodelprojection

import (
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func applyRecoveryCheckpoint(recovery *projectioncontract.RecoveryPlan, latestCheckpoint *projectioncontract.RuntimeCheckpoint) {
	if recovery == nil || latestCheckpoint == nil {
		return
	}
	recovery.CheckpointID = latestCheckpoint.ID
	if recovery.BlockingTicketID == "" && recovery.BlockingApprovalID == "" && latestCheckpoint.CurrentStepID != "" {
		recovery.StepID = latestCheckpoint.CurrentStepID
	} else if recovery.StepID == "" {
		recovery.StepID = latestCheckpoint.CurrentStepID
	}
	if recovery.BlockingTicketID == "" {
		recovery.BlockingTicketID = latestCheckpoint.BlockingTicketID
	}
	if recovery.BlockingApprovalID == "" {
		recovery.BlockingApprovalID = latestCheckpoint.BlockingApprovalID
	}
	if recovery.FailureClass == "" {
		recovery.FailureClass = strings.TrimSpace(readModelStringValue(latestCheckpoint.Data["failureClass"]))
	}
	if recovery.FailureSignature == "" {
		recovery.FailureSignature = strings.TrimSpace(readModelStringValue(latestCheckpoint.Data["failureSignature"]))
	}
}
