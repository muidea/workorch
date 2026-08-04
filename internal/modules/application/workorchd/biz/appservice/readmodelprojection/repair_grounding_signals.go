package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func applyRepairGroundingStructuredSignals(ret *projectioncontract.RepairLoop, snapshot readmodelcore.RunSnapshotState, repairStep *projectioncontract.StepView, actionPackage map[string]any) {
	if ret == nil || repairStep == nil {
		return
	}
	if failedStep := stepByID(snapshot.Steps, ret.LastFailureStepID); failedStep != nil {
		ret.StructuredFailureSignals = readModelStructuredFailureSignalsFromStep(failedStep, ret.LastFailureClass, ret.LastFailureSignature)
	}
	if len(ret.StructuredFailureSignals) > 0 {
		// Current failed step wins. Repair-step inputs may still contain inherited stale signals.
		return
	}
	if signals := readModelMapValue(repairStep.Input["structuredFailureSignals"]); len(signals) > 0 {
		ret.StructuredFailureSignals = signals
		return
	}
	if signals := readModelMapValue(actionPackage["structuredFailureSignals"]); len(signals) > 0 {
		ret.StructuredFailureSignals = signals
	}
}
