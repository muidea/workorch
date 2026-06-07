package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func applyRecoveryCheckpoint(recovery *readmodelcore.RecoveryPlan, latestCheckpoint *readmodelcore.RuntimeCheckpoint) {
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
