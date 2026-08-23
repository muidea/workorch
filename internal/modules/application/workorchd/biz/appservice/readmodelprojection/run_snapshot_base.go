package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func baseRunSnapshotFromModels(input RunSnapshotModelInput, stepViews []projectioncontract.StepView, currentStep *projectioncontract.StepView) readmodelcore.RunSnapshotState {
	ret := readmodelcore.RunSnapshotState{
		Workspace:             mapWorkspaceWithRunProjection(input.Workspace, input.Run),
		Run:                   runtimecore.MapRun(input.Run),
		Step:                  currentStep,
		Steps:                 stepViews,
		ToolCalls:             mapToolCalls(input.ToolCalls),
		Patches:               mapPatches(input.Patches),
		LLMContextBudgets:     mapLLMContextBudgetTraces(stepViews),
		LLMUsageTraces:        mapLLMUsageTracesFromEventRecords(mapEventRecords(input.LLMUsageEventRecords), string(input.Run.ID)),
		ValidationResults:     mapValidationResults(input.Validations),
		RuntimeCheckpoints:    mapRuntimeCheckpoints(input.Checkpoints),
		Results:               mapResults(input.Results),
		Artifacts:             mapArtifacts(input.Artifacts),
		ArtifactContinuations: mapArtifactContinuations(input.ArtifactContinuations),
		MemoryRecallTraces:    mapMemoryRecallTraces(input.MemoryRecallTraces),
		EventRecords:          mapProjectedEventRecords(input.EventRecords),
		Decisions:             input.Decisions,
		Approvals:             input.Approvals,
	}
	applyRunSnapshotCurrentStepToRun(&ret, currentStep)
	ret.LLMContextBudgetSummary = summarizeLLMContextBudgets(ret.LLMContextBudgets)
	ret.LLMUsageSummary = summarizeLLMUsage(ret.LLMUsageTraces)
	ret.Acceptance = mapAcceptanceProjection(input.Run)
	ret.DisplayOutcome = mapRunOutcome(ret)
	return ret
}

func applyRunSnapshotCurrentStepToRun(snapshot *readmodelcore.RunSnapshotState, currentStep *projectioncontract.StepView) {
	if snapshot == nil || currentStep == nil {
		return
	}
	snapshot.Run.CurrentStepID = currentStep.ID
	snapshot.Run.CurrentStepKind = canonicalStepKind(currentStep.Kind)
	snapshot.Run.CurrentStepState = currentStep.State
}
