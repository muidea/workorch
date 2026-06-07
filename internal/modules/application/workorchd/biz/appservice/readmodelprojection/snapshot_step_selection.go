package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
)

func CurrentRunSnapshotStep(steps []presenter.StepView, decisionStep string, approvalStep string, checkpoints []readmodelcore.RuntimeCheckpoint) *presenter.StepView {
	if decisionStep != "" {
		if step := stepByID(steps, decisionStep); step != nil {
			return step
		}
	}
	if approvalStep != "" {
		if step := stepByID(steps, approvalStep); step != nil {
			return step
		}
	}
	if checkpoint := latestRuntimeCheckpoint(checkpoints); checkpoint != nil {
		if step := stepByID(steps, checkpoint.CurrentStepID); step != nil {
			return step
		}
	}
	if len(steps) == 0 {
		return nil
	}
	if step := LatestNonTerminalSnapshotStep(steps); step != nil {
		return step
	}
	if step := TerminalSnapshotStep(steps); step != nil {
		return step
	}
	return &steps[len(steps)-1]
}
