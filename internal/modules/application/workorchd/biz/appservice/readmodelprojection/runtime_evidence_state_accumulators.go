package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func accumulateToolCallEvidenceStates(ret *readmodelcore.RuntimeEvidenceSummary, items []projectioncontract.ToolCall) {
	for _, item := range items {
		switch item.State {
		case string(common.ToolCallStateRunning):
			ret.ToolCallRunning++
		case string(common.ToolCallStateCompleted):
			ret.ToolCallCompleted++
		case string(common.ToolCallStateFailed):
			ret.ToolCallFailed++
		}
	}
}

func accumulatePatchEvidenceStates(ret *readmodelcore.RuntimeEvidenceSummary, items []projectioncontract.Patch) {
	for _, item := range items {
		switch item.State {
		case string(common.PatchStateApplied):
			ret.PatchApplied++
		case string(common.PatchStateFailed):
			ret.PatchFailed++
		}
	}
}

func accumulateValidationEvidenceStates(ret *readmodelcore.RuntimeEvidenceSummary, items []projectioncontract.ValidationResult) {
	for _, item := range items {
		switch item.State {
		case string(common.ValidationResultStatePassed):
			ret.ValidationPassed++
		case string(common.ValidationResultStateFailed):
			ret.ValidationFailed++
		case string(common.ValidationResultStateWarning):
			ret.ValidationWarning++
		}
	}
}
