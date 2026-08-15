package appservice

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

const (
	readModelEventRecordLimit    = readmodelcore.DefaultEventRecordLimit
	readModelStepPageLimit       = 100
	readModelToolCallPageLimit   = 100
	readModelCheckpointPageLimit = 100
	readModelResultPageLimit     = 100
	readModelPatchPageLimit      = 100
	readModelValidationPageLimit = 100
	readModelArtifactPageLimit   = 100
	readModelDecisionPageLimit   = 100
	readModelApprovalPageLimit   = 100
	eventPayloadStringLimit      = 512
)

func readModelMapValue(value any) map[string]any {
	ret, _ := value.(map[string]any)
	return ret
}

func readModelMapSliceValue(value any) []map[string]any {
	if typed, ok := value.([]map[string]any); ok {
		return typed
	}
	items, _ := value.([]any)
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if typed, ok := item.(map[string]any); ok {
			ret = append(ret, typed)
		}
	}
	return ret
}
