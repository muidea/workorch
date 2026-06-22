package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
)

func RunSnapshotFromModels(input RunSnapshotModelInput) readmodelcore.RunSnapshot {
	stepViews := MapSteps(input.Steps)
	currentStep := CurrentRunSnapshotStep(
		stepViews,
		DecisionStepID(input.SelectedTicket),
		ApprovalStepID(input.SelectedApproval),
		MapRuntimeCheckpoints(input.Checkpoints),
	)
	observedItems := observed.BuildWorkorchSnapshot(
		input.Workspace,
		input.Run,
		input.Steps,
		input.SelectedTicket,
		input.SelectedResolution,
		input.SelectedApproval,
		input.PublishedEvents,
	)
	ret := baseRunSnapshotFromModels(input, stepViews, currentStep)
	applyRunSnapshotDerivedReadModels(&ret, input, stepViews, observedItems)
	return ret
}
