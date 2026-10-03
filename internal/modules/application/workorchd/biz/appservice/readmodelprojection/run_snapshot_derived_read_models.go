package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func applyRunSnapshotDerivedReadModels(ret *readmodelcore.RunSnapshotState, input RunSnapshotModelInput, stepViews []projectioncontract.StepView, observedItems []observed.SnapshotItem) {
	ret.GitOperations = mapGitOperationTraces(stepViews)
	ret.WebSources = mapWebSourceTraces(stepViews)
	ret.CoverageWorkItems = mapCoverageWorkItems(input.CoverageWorkItems)
	ret.GapBaseline = gapBaselineFromRunSnapshot(*ret)
	ret.AuthorityDiagnostics = authorityDiagnosticsFromRunSnapshot(*ret)
	ret.Observed = mapObserved(observedItems)
	capabilityCatalog := runtimeCapabilityCatalogFromItems(input.MCPCapabilities)
	ret.SelectedCapabilities = selectedRuntimeCapabilitiesFromRun(input.Run, capabilityCatalog)
	ret.UsedCapabilities = usedRuntimeCapabilities(*ret, input.Run, capabilityCatalog)
	ret.TaskProgress = mapLocalizedTaskProgress(*ret)
	applyRuntimeProgress(&ret.TaskProgress, input.Run)
	ret.TaskProgress = finalizeRunFormalProgress(ret.TaskProgress, *ret)
	ret.RepairLoop = ret.TaskProgress.RepairLoop
	ret.TaskDecompositions = mapTaskDecompositions(*ret)
	ret.TaskPlans = mapTaskPlansFromSnapshot(*ret)
	ret.ClosureTraces = mapClosureTraces(*ret)
	ret.DoDEvidence = mapDoDEvidence(*ret)
	ret.ResultQuality = mapResultQuality(*ret)
	ret.ExecutionSections = mapExecutionSections(*ret)
	ret.ConvergenceStatus = mapConvergenceStatus(*ret)
}
