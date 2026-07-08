package readmodelports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
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
	var currentRunStep *runmodel.Step
	if currentStep := readmodelprojection.CurrentRunStatusStep(item, related.steps, related.checkpoints, related.decisions, related.approvals); currentStep != nil {
		currentRunStep = currentStep
		ret.CurrentStepID = string(currentStep.ID)
		ret.CurrentStepKind = readmodelprojection.CanonicalStepKind(currentStep.Kind)
		ret.CurrentStepState = string(currentStep.State)
	}
	stepViews := presenter.MapSteps(related.steps)
	var currentStep *presenter.StepView
	if currentRunStep != nil {
		if mapped := presenter.MapStep(currentRunStep); mapped != nil {
			currentStep = mapped
			currentStep.Kind = ret.CurrentStepKind
			currentStep.State = ret.CurrentStepState
		}
	}
	ret.RuntimeExecution = readmodelprojection.MapRuntimeExecutionState(stepViews, readmodelprojection.MapRuntimeCheckpoints(related.checkpoints), currentStep)
	runView := runtimecore.MapRun(item)
	ret.ConvergenceStatus = readmodelprojection.MapConvergenceStatus(readmodelcore.RunSnapshot{
		Run:              runView,
		Steps:            readmodelprojection.MapSteps(related.steps),
		RuntimeExecution: ret.RuntimeExecution,
		Acceptance:       readmodelprojection.MapAcceptanceProjection(runView),
	})
	if ret.ConvergenceStatus == nil {
		ret.ConvergenceStatus = &readmodelcore.ConvergenceStatus{
			WorkspaceID: string(item.WorkspaceID),
			RunID:       string(item.ID),
			RunState:    string(item.State),
		}
	}
	if ret.RuntimeExecution != nil && ret.RuntimeExecution.ContractViolation != nil {
		ret.ConvergenceStatus.LatestContractViolationRule = ret.RuntimeExecution.ContractViolation.ViolatedRule
		ret.ConvergenceStatus.LatestRecovery = ret.RuntimeExecution.ContractViolation.Recoverability
	}
	return ret
}
