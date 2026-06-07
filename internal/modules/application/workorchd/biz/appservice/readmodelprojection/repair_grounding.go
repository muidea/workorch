package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func projectRepairGroundingObservability(ret *readmodelcore.RepairLoop, snapshot readmodelcore.RunSnapshot) {
	if ret == nil || strings.TrimSpace(ret.LastRepairStepID) == "" {
		return
	}
	repairStep := stepByID(snapshot.Steps, ret.LastRepairStepID)
	if repairStep == nil {
		return
	}
	actionPackage := readModelMapValue(repairStep.Input["repairActionPackage"])
	applyRepairGroundingTargetScope(ret, snapshot, repairStep, actionPackage)
	applyRepairGroundingStructuredSignals(ret, snapshot, repairStep, actionPackage)
	applyRepairGroundingInputDetails(ret, repairStep, actionPackage)
	ret.LastRepairInputBytes = readModelJSONSize(repairStep.Input)
}
