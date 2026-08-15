package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: repair_grounding.go

func projectRepairGroundingObservability(ret *projectioncontract.RepairLoop, snapshot readmodelcore.RunSnapshotState) {
	if ret == nil || strings.TrimSpace(ret.LastRepairStepID) == "" {
		return
	}
	repairStep := stepByID(snapshot.Steps, ret.LastRepairStepID)
	if repairStep == nil {
		return
	}
	actionPackage := readModelMapValue(repairStep.Input["repairActionPackage"])
	applyRepairGroundingTargetScope(ret, snapshot, repairStep, actionPackage)
	applyRepairGroundingStructuredSignals(ret, snapshot, repairStep, actionPackage)
	applyRepairGroundingInputDetails(ret, repairStep, actionPackage)
	ret.LastRepairInputBytes = readModelJSONSize(repairStep.Input)
}

// source: repair_grounding_target_scope.go

func applyRepairGroundingTargetScope(ret *projectioncontract.RepairLoop, snapshot readmodelcore.RunSnapshotState, repairStep *projectioncontract.StepView, actionPackage map[string]any) {
	if ret == nil || repairStep == nil {
		return
	}
	targetDiagnostics := readModelMapValue(actionPackage["targetRelevanceDiagnostics"])
	ret.LastFailureTargetScope = strings.TrimSpace(readModelFirstNonEmptyString(
		readModelFailureTargetScopeFromStep(stepByID(snapshot.Steps, ret.LastFailureStepID)),
		readModelStringValue(repairStep.Input["failureTargetScope"]),
		readModelStringValue(actionPackage["failureTargetScope"]),
		readModelStringValue(targetDiagnostics["failureTargetScope"]),
	))
}

// source: task_plan_projection.go

func mapTaskPlans(snapshot readmodelcore.RunSnapshotState, decompositions []projectioncontract.TaskDecomposition) []projectioncontract.TaskPlan {
	if len(decompositions) == 0 {
		return nil
	}
	stepByID := map[string]projectioncontract.StepView{}
	for _, step := range snapshot.Steps {
		stepByID[step.ID] = step
	}
	occurrences := timelineStepOccurrences(snapshot)
	grouped := map[string][]projectioncontract.TaskPlan{}
	knownAnchors := map[string]bool{}
	for _, decomposition := range decompositions {
		if strings.TrimSpace(decomposition.RootStepID) == "" {
			continue
		}
		step, hasStep := stepByID[decomposition.RootStepID]
		plan := projectTaskPlan(snapshot, decomposition, step, hasStep, occurrences)
		grouped[plan.AnchorStepID] = append(grouped[plan.AnchorStepID], plan)
		knownAnchors[plan.AnchorStepID] = true
		knownAnchors[decomposition.RootStepID] = true
	}
	for _, step := range snapshot.Steps {
		plan, ok := taskRetryPlanFromStep(snapshot, step, occurrences, knownAnchors)
		if !ok {
			continue
		}
		grouped[plan.AnchorStepID] = append(grouped[plan.AnchorStepID], plan)
	}
	if len(grouped) == 0 {
		return nil
	}
	ret := taskPlanRevisions(grouped)
	if len(ret) == 0 {
		return nil
	}
	return ret
}
