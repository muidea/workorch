package readmodelports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
)

func (models runSnapshotModels) projectionInput(item *runmodel.Run) readmodelprojection.RunSnapshotModelInput {
	return readmodelprojection.RunSnapshotModelInput{
		Workspace:             models.Workspace,
		Run:                   item,
		Steps:                 models.Steps,
		ToolCalls:             models.ToolCalls,
		Patches:               models.Patches,
		Validations:           models.Validations,
		Checkpoints:           models.Checkpoints,
		Results:               models.Results,
		Artifacts:             models.Artifacts,
		ArtifactContinuations: models.ArtifactContinuations,
		CoverageWorkItems:     models.CoverageWorkItems,
		EventRecords:          models.EventRecords,
		LLMUsageEventRecords:  models.LLMUsageEventRecords,
		MemoryRecallTraces:    models.MemoryRecallTraces,
		Decisions:             models.Decisions.Items,
		Approvals:             models.Approvals.Items,
		MCPCapabilities:       models.MCPCapabilities,
	}
}
