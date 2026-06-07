package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
)

func applyRunSnapshotDerivedReadModels(ret *readmodelcore.RunSnapshot, input RunSnapshotModelInput, stepViews []presenter.StepView, observedItems []observed.SnapshotItem) {
	ret.GitOperations = MapGitOperationTraces(stepViews)
	ret.WebSources = MapWebSourceTraces(stepViews)
	ret.CoverageWorkItems = MapSliceValue(input.Run.ExecutionContext["coverageWorkItems"])
	ret.CoverageWorkItemContext = CloneMap(MapValue(input.Run.ExecutionContext["coverageWorkItemContext"]))
	ret.Observed = presenter.MapObserved(observedItems)
	capabilityCatalog := RuntimeCapabilityCatalogFromItems(ret.Run.ExecutionContext, ret.ToolCalls, input.MCPCapabilities)
	ret.SelectedCapabilities = SelectedRuntimeCapabilities(ret.Run.ExecutionContext, capabilityCatalog)
	ret.UsedCapabilities = UsedRuntimeCapabilities(*ret, capabilityCatalog)
	ret.TaskProgress = MapLocalizedTaskProgress(*ret)
	ret.RepairLoop = ret.TaskProgress.RepairLoop
	ret.TaskDecompositions = MapTaskDecompositions(*ret)
	ret.TaskPlans = MapTaskPlansFromSnapshot(*ret)
	ret.ClosureTraces = MapClosureTraces(*ret)
	ret.DoDEvidence = MapDoDEvidence(*ret)
	ret.ResultQuality = MapResultQuality(*ret)
	ret.ExecutionSections = MapExecutionSections(*ret)
}
