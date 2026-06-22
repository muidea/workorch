package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func applyRepairGroundingTargetScope(ret *readmodelcore.RepairLoop, snapshot readmodelcore.RunSnapshot, repairStep *StepView, actionPackage map[string]any) {
	if ret == nil || repairStep == nil {
		return
	}
	targetDiagnostics := readModelMapValue(actionPackage["targetRelevanceDiagnostics"])
	ret.LastFailureTargetScope = strings.TrimSpace(readModelFirstNonEmptyString(
		readModelFailureTargetScopeFromStep(stepByID(snapshot.Steps, ret.LastFailureStepID)),
		readModelStringValue(repairStep.Input["failureTargetScope"]),
		readModelStringValue(actionPackage["failureTargetScope"]),
		readModelStringValue(targetDiagnostics["failureTargetScope"]),
	))
}
