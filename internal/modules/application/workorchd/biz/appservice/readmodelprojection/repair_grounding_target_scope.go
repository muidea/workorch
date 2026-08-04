package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func applyRepairGroundingTargetScope(ret *projectioncontract.RepairLoop, snapshot readmodelcore.RunSnapshot, repairStep *readmodelcore.StepView, actionPackage map[string]any) {
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
