package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
)

func runSnapshotFromModels(input RunSnapshotModelInput) readmodelcore.RunSnapshotState {
	stepViews := mapSteps(input.Steps)
	currentStep := currentRunSnapshotStepForRun(
		input.Run,
		stepViews,
		decisionStepID(input.SelectedTicket),
		approvalStepID(input.SelectedApproval),
		mapRuntimeCheckpoints(input.Checkpoints),
	)
	observedItems := observed.BuildWorkorchSnapshot(
		input.Workspace,
		input.Run,
		input.SelectedTicket,
		input.SelectedResolution,
		input.SelectedApproval,
		input.PublishedEvents,
	)
	ret := baseRunSnapshotFromModels(input, stepViews, currentStep)
	applyRunSnapshotDerivedReadModels(&ret, input, stepViews, observedItems)
	applyRunSnapshotGovernanceProjection(&ret)
	return ret
}
