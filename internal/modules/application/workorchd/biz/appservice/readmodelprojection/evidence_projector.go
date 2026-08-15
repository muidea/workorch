package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

// evidenceProjector is a typed, stateless read-only facade table.
type evidenceProjector struct {
	PageResults           func([]*resultmodel.Result, workorchquerycontract.RunResourcePageQuery) readmodelcore.ResultPage
	PagePatches           func([]*runtimeevidencemodel.Patch, workorchquerycontract.RunResourcePageQuery) readmodelcore.PatchPage
	PageValidationResults func([]*runtimeevidencemodel.ValidationResult, workorchquerycontract.RunResourcePageQuery) readmodelcore.ValidationResultPage
	PageArtifacts         func([]*resultmodel.Artifact, workorchquerycontract.RunResourcePageQuery) readmodelcore.ArtifactPage
	PageToolCalls         func([]*runtimeevidencemodel.ToolCall, workorchquerycontract.RunResourcePageQuery) readmodelcore.ToolCallPage
	PageSteps             func([]*runmodel.Step, workorchquerycontract.RunResourcePageQuery) readmodelcore.StepPage
	PageCheckpoints       func([]*runmodel.RuntimeCheckpoint, workorchquerycontract.RunResourcePageQuery) readmodelcore.RuntimeCheckpointPage
}

// EvidenceProjection returns the one typed application-facing evidence page
// facade. It only maps already-read owner facts and never issues a command.
func EvidenceProjection() evidenceProjector {
	return evidenceProjector{
		PageResults:           pageRunResults,
		PagePatches:           pageRunPatches,
		PageValidationResults: pageRunValidationResults,
		PageArtifacts:         pageRunArtifacts,
		PageToolCalls:         pageRunToolCalls,
		PageSteps:             pageRunSteps,
		PageCheckpoints:       pageRunRuntimeCheckpoints,
	}
}
