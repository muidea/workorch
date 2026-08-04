package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func applyRunSnapshotDerivedReadModels(ret *readmodelcore.RunSnapshotState, input RunSnapshotModelInput, stepViews []projectioncontract.StepView, observedItems []observed.SnapshotItem) {
	ret.GitOperations = MapGitOperationTraces(stepViews)
	ret.WebSources = MapWebSourceTraces(stepViews)
	ret.CoverageWorkItems = MapCoverageWorkItems(input.CoverageWorkItems)
	ret.GapBaseline = GapBaselineFromRunSnapshot(*ret)
	ret.AuthorityDiagnostics = AuthorityDiagnosticsFromRunSnapshot(*ret)
	ret.Observed = MapObserved(observedItems)
	capabilityCatalog := RuntimeCapabilityCatalogFromItems(input.MCPCapabilities)
	ret.SelectedCapabilities = SelectedRuntimeCapabilitiesFromRun(input.Run, capabilityCatalog)
	ret.UsedCapabilities = UsedRuntimeCapabilities(*ret, input.Run, capabilityCatalog)
	ret.TaskProgress = MapLocalizedTaskProgress(*ret)
	applyRuntimeProgress(&ret.TaskProgress, input.Run)
	ret.RepairLoop = ret.TaskProgress.RepairLoop
	ret.TaskDecompositions = MapTaskDecompositions(*ret)
	ret.TaskPlans = MapTaskPlansFromSnapshot(*ret)
	ret.ClosureTraces = MapClosureTraces(*ret)
	ret.DoDEvidence = MapDoDEvidence(*ret)
	ret.ResultQuality = MapResultQuality(*ret)
	ret.ExecutionSections = MapExecutionSections(*ret)
	ret.ConvergenceStatus = MapConvergenceStatus(*ret)
}
