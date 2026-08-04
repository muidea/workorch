package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func baseRunSnapshotFromModels(input RunSnapshotModelInput, stepViews []projectioncontract.StepView, currentStep *projectioncontract.StepView) readmodelcore.RunSnapshotState {
	ret := readmodelcore.RunSnapshotState{
		Workspace:             MapWorkspaceWithRunProjection(input.Workspace, input.Run),
		Run:                   runtimecore.MapRun(input.Run),
		Step:                  currentStep,
		Steps:                 stepViews,
		ToolCalls:             MapToolCalls(input.ToolCalls),
		Patches:               MapPatches(input.Patches),
		LLMContextBudgets:     MapLLMContextBudgetTraces(stepViews),
		LLMUsageTraces:        MapLLMUsageTracesFromEventRecords(MapEventRecords(input.LLMUsageEventRecords), string(input.Run.ID)),
		ValidationResults:     MapValidationResults(input.Validations),
		RuntimeCheckpoints:    MapRuntimeCheckpoints(input.Checkpoints),
		Results:               MapResults(input.Results),
		Artifacts:             MapArtifacts(input.Artifacts),
		ArtifactContinuations: MapArtifactContinuations(input.ArtifactContinuations),
		MemoryRecallTraces:    MapMemoryRecallTraces(input.MemoryRecallTraces),
		EventRecords:          MapProjectedEventRecords(input.EventRecords),
		Decisions:             input.Decisions,
		Approvals:             input.Approvals,
	}
	applyRunSnapshotCurrentStepToRun(&ret, currentStep)
	ret.LLMContextBudgetSummary = SummarizeLLMContextBudgets(ret.LLMContextBudgets)
	ret.LLMUsageSummary = SummarizeLLMUsage(ret.LLMUsageTraces)
	ret.Acceptance = MapAcceptanceProjection(ret.Run)
	ret.Outcome = MapRunOutcome(ret)
	return ret
}

func applyRunSnapshotCurrentStepToRun(snapshot *readmodelcore.RunSnapshotState, currentStep *projectioncontract.StepView) {
	if snapshot == nil || currentStep == nil {
		return
	}
	snapshot.Run.CurrentStepID = currentStep.ID
	snapshot.Run.CurrentStepKind = CanonicalStepKind(currentStep.Kind)
	snapshot.Run.CurrentStepState = currentStep.State
}
