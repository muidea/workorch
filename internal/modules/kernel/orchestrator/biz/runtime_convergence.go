package orchestrator

import (
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

const (
	convergenceStateNeedEvidence    = "need_evidence"
	convergenceStateNeedStateChange = "need_state_change"
	convergenceStateNeedValidation  = "need_validation"
	convergenceStateNeedAssessment  = "need_assessment"
	convergenceStateNeedGovernance  = "need_governance"
	convergenceStateBlocked         = "blocked"
)

type ConvergenceContract struct {
	State                           string
	RuntimeStage                    string
	RuntimeStageMode                string
	RepairObjective                 string
	StateChangingOutputRequired     bool
	ConcreteFailureEvidenceRequired bool
	RepairActionPackage             map[string]any
	GovernanceReason                string
	SourceRefs                      []map[string]any
	TargetEvidence                  map[string]any
	HistoricalSignals               map[string]any
}

func BuildConvergenceContract(
	run *runModel.Run,
	currentStep *runModel.Step,
	failedStep *runModel.Step,
	lineage map[string]any,
	evidence map[string]any,
) ConvergenceContract {
	input := mapFromValue(lineage["input"])
	if len(input) == 0 && currentStep != nil {
		input = currentStep.Input
	}
	if len(input) == 0 && failedStep != nil {
		input = failedStep.Input
	}
	for _, sourceKey := range []string{"ticketContext", "replanContinuation", "rejectedResultContinuation"} {
		source := mapFromValue(input[sourceKey])
		if len(source) == 0 && run != nil {
			source = mapFromValue(run.ExecutionContext[sourceKey])
		}
		if len(source) == 0 {
			source = mapFromValue(mapFromValue(input["executionContext"])[sourceKey])
		}
		if len(source) == 0 {
			continue
		}
		if sourceKey == "replanContinuation" &&
			(!replanContinuationCanDriveCurrentProgress(input, source) || convergenceRunHasRejectedContinuation(run)) {
			continue
		}
		if len(mapFromValue(input["repairActionPackage"])) == 0 {
			if pkg := mapFromValue(source["repairActionPackage"]); len(pkg) > 0 {
				input["repairActionPackage"] = pkg
			}
		}
		for _, key := range []string{
			"stateChangingOutputRequired",
			"concreteFailureEvidenceRequired",
			"repairObjective",
			"failureTargetRepairContext",
			"patchRepairContext",
			"structuredFailureSignals",
			"currentTargetAlreadySatisfiesFailedPatch",
			"coverageTargetSet",
			"missingDoDItems",
			"missingGoalCoverage",
			"missingDeliverableCoverage",
		} {
			if _, ok := input[key]; !ok && source[key] != nil {
				input[key] = source[key]
			}
		}
	}
	enrichRepairInputWithProgressFailureSignals(input)
	contract := ConvergenceContract{
		State:            convergenceStateBlocked,
		RuntimeStage:     runtimeStageRepair,
		RuntimeStageMode: "strict_convergence",
		SourceRefs:       convergenceSourceRefs(currentStep, failedStep, lineage, evidence),
	}
	if governanceReason := convergenceGovernanceReason(input, failedStep); governanceReason != "" {
		contract.State = convergenceStateNeedGovernance
		contract.RuntimeStage = runtimeStageDecision
		contract.RuntimeStageMode = "structured_governance"
		contract.GovernanceReason = governanceReason
		return contract
	}
	if convergenceNeedsValidation(input, failedStep) {
		contract.State = convergenceStateNeedValidation
		contract.RuntimeStage = runtimeStageVerification
		contract.RuntimeStageMode = "strict_convergence"
		contract.RepairObjective = repairObjectiveConcreteValidationEvidence
		contract.RepairActionPackage = cloneStepPayload(mapFromValue(input["repairActionPackage"]))
		contract.TargetEvidence = convergenceTargetEvidence(input, contract.RepairActionPackage)
		return contract
	}
	if convergenceAssessmentOnly(input, failedStep) {
		contract.State = convergenceStateNeedAssessment
		contract.RuntimeStage = runtimeStageVerification
		contract.RuntimeStageMode = "strict_convergence"
		contract.RepairObjective = strings.TrimSpace(stringFromMap(input, "repairObjective"))
		return contract
	}
	if convergenceNeedsEvidence(input, failedStep) {
		contract.State = convergenceStateNeedEvidence
		contract.RuntimeStage = runtimeStageRepair
		contract.RuntimeStageMode = "strict_convergence"
		contract.RepairObjective = repairObjectiveAcquireFailureEvidence
		contract.ConcreteFailureEvidenceRequired = true
		contract.RepairActionPackage = cloneStepPayload(mapFromValue(input["repairActionPackage"]))
		if len(contract.RepairActionPackage) == 0 {
			contract.RepairActionPackage = map[string]any{"requiredOutput": evidenceAcquisitionRequiredOutput(stateChangingRepairRequiredOutput())}
		}
		return contract
	}
	if convergenceNeedsStateChange(input, failedStep) {
		contract.State = convergenceStateNeedStateChange
		contract.RuntimeStage = runtimeStageRepair
		contract.RuntimeStageMode = "strict_convergence"
		contract.RepairObjective = repairObjectiveStateChangingOutput
		contract.StateChangingOutputRequired = true
		contract.RepairActionPackage = cloneStepPayload(mapFromValue(input["repairActionPackage"]))
		contract.TargetEvidence = convergenceTargetEvidence(input, contract.RepairActionPackage)
		return contract
	}
	if convergenceNeedsEvidenceFromRejectedContinuation(run) {
		contract.State = convergenceStateNeedEvidence
		contract.RuntimeStage = runtimeStageRepair
		contract.RuntimeStageMode = "strict_convergence"
		contract.RepairObjective = repairObjectiveAcquireFailureEvidence
		contract.ConcreteFailureEvidenceRequired = true
		contract.RepairActionPackage = map[string]any{"requiredOutput": evidenceAcquisitionRequiredOutput(stateChangingRepairRequiredOutput())}
		return contract
	}
	return contract
}

func ApplyConvergenceContract(input map[string]any, contract ConvergenceContract) map[string]any {
	if input == nil {
		input = map[string]any{}
	}
	if contract.State == "" || contract.State == convergenceStateBlocked {
		return input
	}
	input["convergenceContract"] = compactConvergenceContract(contract)
	switch contract.State {
	case convergenceStateNeedEvidence:
		setRuntimeStage(input, runtimeStageRepair)
		input["repairObjective"] = repairObjectiveAcquireFailureEvidence
		input["concreteFailureEvidenceRequired"] = true
		delete(input, "stateChangingOutputRequired")
		pkg := cloneStepPayload(contract.RepairActionPackage)
		if len(pkg) == 0 {
			pkg = map[string]any{}
		}
		required := evidenceAcquisitionRequiredOutput(mapFromValue(pkg["requiredOutput"]))
		pkg["requiredOutput"] = required
		pkg["evidenceAcquisitionRequired"] = true
		input["repairActionPackage"] = pkg
	case convergenceStateNeedStateChange:
		setRuntimeStage(input, runtimeStageRepair)
		input["repairObjective"] = repairObjectiveStateChangingOutput
		input["stateChangingOutputRequired"] = true
		delete(input, "concreteFailureEvidenceRequired")
		if len(contract.RepairActionPackage) > 0 {
			input["repairActionPackage"] = contract.RepairActionPackage
		}
	case convergenceStateNeedValidation:
		setRuntimeStage(input, runtimeStageVerification)
		input["repairObjective"] = repairObjectiveConcreteValidationEvidence
		input["boundedValidationRequired"] = true
		delete(input, stepInputKeyAssessmentOnly)
		delete(input, "assessmentOnlyReason")
		delete(input, "stateChangingOutputRequired")
		delete(input, "concreteFailureEvidenceRequired")
		if len(contract.RepairActionPackage) > 0 {
			input["repairActionPackage"] = contract.RepairActionPackage
		}
	case convergenceStateNeedAssessment:
		setRuntimeStage(input, runtimeStageVerification)
		input[stepInputKeyAssessmentOnly] = true
		delete(input, "stateChangingOutputRequired")
		delete(input, "concreteFailureEvidenceRequired")
	case convergenceStateNeedGovernance:
		setRuntimeStage(input, runtimeStageDecision)
		input["governanceRequired"] = true
		if contract.GovernanceReason != "" {
			input["governanceReason"] = contract.GovernanceReason
		}
	}
	return input
}

func ShouldCreateDecisionTicket(input map[string]any, llmOutput map[string]any, contract ConvergenceContract) bool {
	spec := decisionBlockSpec(llmOutput)
	if spec.Reason == "" {
		return true
	}
	if contract.State != convergenceStateNeedStateChange {
		return true
	}
	if !contract.StateChangingOutputRequired {
		return true
	}
	if len(contract.TargetEvidence) == 0 {
		return true
	}
	if decisionBlockDeclaresExternalOrRuntimeDependency(spec.Context) {
		return true
	}
	return false
}

func convergenceAssessmentOnly(input map[string]any, failedStep *runModel.Step) bool {
	if boolValue(input[stepInputKeyAssessmentOnly]) ||
		boolValue(input["assessmentOnlyContractCorrection"]) ||
		boolValue(input[stepInputKeyGraphLimitFollowUp]) {
		return true
	}
	return doDCoverageContractCorrectionRequired(failedStep) && !coverageCorrectionNeedsConcreteEvidence(failedStep)
}

func convergenceNeedsValidation(input map[string]any, failedStep *runModel.Step) bool {
	if strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveConcreteValidationEvidence {
		return true
	}
	if len(mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"])) > 0 ||
		boolValue(input["stateChangeAlreadySatisfiedByCurrentTarget"]) {
		return true
	}
	if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
		if len(mapSliceFromValue(pkg["currentTargetAlreadySatisfiesFailedPatch"])) > 0 ||
			boolValue(pkg["stateChangeAlreadySatisfiedByCurrentTarget"]) {
			return true
		}
		required := mapFromValue(pkg["requiredOutput"])
		return boolValue(required["mustProduceConcreteValidationEvidence"])
	}
	return false
}

func convergenceNeedsEvidence(input map[string]any, failedStep *runModel.Step) bool {
	if boolValue(input["concreteFailureEvidenceRequired"]) ||
		strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveAcquireFailureEvidence {
		return true
	}
	return repairActionPackageRequiresEvidenceAcquisition(mapFromValue(input["repairActionPackage"]))
}

func convergenceNeedsEvidenceFromRejectedContinuation(run *runModel.Run) bool {
	return convergenceRunHasRejectedContinuation(run)
}

func convergenceRunHasRejectedContinuation(run *runModel.Run) bool {
	if run == nil {
		return false
	}
	return len(mapFromValue(run.ExecutionContext["rejectedResultContinuation"])) > 0
}

func convergenceNeedsStateChange(input map[string]any, failedStep *runModel.Step) bool {
	if boolValue(input["stateChangingOutputRequired"]) ||
		strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveStateChangingOutput {
		return true
	}
	return repairActionPackageRequiresStateChangingOutput(mapFromValue(input["repairActionPackage"]))
}

func convergenceGovernanceReason(input map[string]any, failedStep *runModel.Step) string {
	context := mapFromValue(input[stepOutputKeyBlockDecisionContext])
	if len(context) == 0 && failedStep != nil {
		context = mapFromValue(failedStep.Output[stepOutputKeyBlockDecisionContext])
	}
	if decisionBlockDeclaresExternalOrRuntimeDependency(context) {
		return firstNonEmptyString(
			stringFromMap(context, "reason"),
			stringFromMap(context, decisionContextKeyReason),
			stringFromMap(input, "governanceReason"),
		)
	}
	return ""
}

func convergenceTargetEvidence(input map[string]any, pkg map[string]any) map[string]any {
	ret := map[string]any{}
	for _, key := range []string{
		"repairTargetSet",
		"structuredFailureSignals",
		"failureTargetRepairContext",
		"patchRepairContext",
		"coverageTargetSet",
		"targetSnapshots",
	} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	for _, key := range []string{
		"repairTargetSet",
		"structuredFailureSignals",
		"coverageTargetSet",
		"targetSnapshots",
		"primaryDiagnosticRepair",
	} {
		if value, ok := pkg[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	if len(ret) == 0 || !stateChangingRepairHasConcreteTargetEvidence(input, pkg) {
		return nil
	}
	return ret
}

func convergenceSourceRefs(currentStep *runModel.Step, failedStep *runModel.Step, lineage map[string]any, evidence map[string]any) []map[string]any {
	var ret []map[string]any
	if currentStep != nil && currentStep.ID != "" {
		ret = append(ret, map[string]any{"type": "current_step", "stepId": string(currentStep.ID), "kind": currentStep.Kind})
	}
	if failedStep != nil && failedStep.ID != "" && (currentStep == nil || failedStep.ID != currentStep.ID) {
		ret = append(ret, map[string]any{"type": "failed_step", "stepId": string(failedStep.ID), "kind": failedStep.Kind})
	}
	if sourceRef := strings.TrimSpace(stringFromMap(lineage, "sourceRef")); sourceRef != "" {
		ret = append(ret, map[string]any{"type": "lineage", "sourceRef": sourceRef})
	}
	if sourceRef := strings.TrimSpace(stringFromMap(evidence, "sourceRef")); sourceRef != "" {
		ret = append(ret, map[string]any{"type": "evidence", "sourceRef": sourceRef})
	}
	return ret
}

func compactConvergenceContract(contract ConvergenceContract) map[string]any {
	ret := map[string]any{
		"state":        contract.State,
		"runtimeStage": contract.RuntimeStage,
	}
	if contract.RuntimeStageMode != "" {
		ret["runtimeStageMode"] = contract.RuntimeStageMode
	}
	if contract.RepairObjective != "" {
		ret["repairObjective"] = contract.RepairObjective
	}
	if contract.StateChangingOutputRequired {
		ret["stateChangingOutputRequired"] = true
	}
	if contract.ConcreteFailureEvidenceRequired {
		ret["concreteFailureEvidenceRequired"] = true
	}
	if len(contract.TargetEvidence) > 0 {
		ret["targetEvidence"] = contract.TargetEvidence
	}
	if len(contract.SourceRefs) > 0 {
		ret["sourceRefs"] = contract.SourceRefs
	}
	if contract.GovernanceReason != "" {
		ret["governanceReason"] = contract.GovernanceReason
	}
	return ret
}
