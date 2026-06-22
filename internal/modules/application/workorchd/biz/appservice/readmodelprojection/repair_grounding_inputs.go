package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func applyRepairGroundingInputDetails(ret *readmodelcore.RepairLoop, repairStep *StepView, actionPackage map[string]any) {
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
