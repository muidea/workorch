package readmodelports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func projectRunStatus(item *runmodel.Run, related runStatusRelatedModels) readmodelcore.RunStatus {
	ret := readmodelcore.RunStatus{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		State:              string(item.State),
		StepCount:          len(related.steps),
		DecisionCount:      len(related.decisions),
		ApprovalCount:      len(related.approvals),
		BlockingTicketID:   string(item.BlockingTicketID),
		BlockingApprovalID: string(item.BlockingApprovalID),
		FailureReason:      item.FailureReason,
	}
	if currentStep := readmodelprojection.CurrentRunStatusStep(item, related.steps, related.checkpoints, related.decisions, related.approvals); currentStep != nil {
		ret.CurrentStepID = string(currentStep.ID)
		ret.CurrentStepKind = readmodelprojection.CanonicalStepKind(currentStep.Kind)
		ret.CurrentStepState = string(currentStep.State)
	}
	return ret
}
