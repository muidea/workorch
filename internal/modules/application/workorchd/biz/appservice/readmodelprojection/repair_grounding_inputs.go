package readmodelprojection

import (
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func applyRepairGroundingInputDetails(ret *projectioncontract.RepairLoop, repairStep *projectioncontract.StepView, actionPackage map[string]any) {
	if ret == nil || repairStep == nil {
		return
	}
	if compaction := readModelMapValue(repairStep.Input["repairInputCompaction"]); len(compaction) > 0 {
		ret.RepairInputCompaction = compaction
	}
	if repeated := readModelMapValue(actionPackage["repeatedTargetRepairMode"]); len(repeated) > 0 {
		ret.RepeatedFailureReason = strings.TrimSpace(readModelStringValue(repeated["reason"]))
	}
}
