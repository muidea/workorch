package readmodelports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
)

func projectRunStatus(item *runmodel.Run, related runStatusRelatedModels) readmodelcore.RunStatus {
	// StepCount uses dispatch/operation units only. Candidate attempts are not steps.
	stepCount := len(related.steps)
	if stepCount == 0 && item.RuntimeState != nil {
		stepCount = len(item.RuntimeState.CompletedDispatches)
		if item.RuntimeState.ActiveDispatch != nil {
			stepCount++
		}
	}
	ret := readmodelcore.RunStatus{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		State:              string(item.State),
		StepCount:          stepCount,
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
	runView := runtimecore.MapRun(item)
	ret.ConvergenceStatus = readmodelprojection.MapConvergenceStatus(readmodelcore.RunSnapshot{
		Run:        runView,
		Steps:      readmodelprojection.MapSteps(related.steps),
		Acceptance: readmodelprojection.MapAcceptanceProjection(runView),
	})
	if ret.ConvergenceStatus == nil {
		ret.ConvergenceStatus = &readmodelcore.ConvergenceStatus{
			WorkspaceID: string(item.WorkspaceID),
			RunID:       string(item.ID),
			RunState:    string(item.State),
		}
	}
	return ret
}
