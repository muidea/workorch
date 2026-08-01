package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func RunRecoverySnapshotFromModels(input RunSnapshotModelInput) readmodelcore.RunSnapshot {
	stepViews := MapSteps(input.Steps)
	currentStep := CurrentRunSnapshotStep(stepViews, "", "", MapRuntimeCheckpoints(input.Checkpoints))
	ret := baseRunSnapshotFromModels(input, stepViews, currentStep)
	ret.TaskProgress = MapLocalizedTaskProgress(ret)
	applyRuntimeProgress(&ret.TaskProgress, input.Run)
	ret.RepairLoop = ret.TaskProgress.RepairLoop
	ret.TaskPlans = MapTaskPlansFromSnapshot(ret)
	ret.DoDEvidence = MapDoDEvidence(ret)
	ret.ResultQuality = MapResultQuality(ret)
	return ret
}
