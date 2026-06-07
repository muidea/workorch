package appservice

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
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
	eventPayloadStringLimit      = readmodelprojection.EventPayloadStringLimit
)

func readModelMapValue(value any) map[string]any {
	return readmodelprojection.MapValue(value)
}

func readModelMapSliceValue(value any) []map[string]any {
	return readmodelprojection.MapSliceValue(value)
}
