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
		if sourceKey == "rejectedResultContinuation" && convergenceAssessmentOnly(input, failedStep) {
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
		if sourceKey == "rejectedResultContinuation" {
			inheritRejectedResultCoverageContract(input, source)
		}
	}
	enrichRepairInputWithProgressFailureSignals(input)
	hydrateConvergenceTargetEvidence(input)
	refreshConvergenceCoverageTargets(input)
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
	if pkg := stateChangingEvidenceRequirementRepairActionPackage(run, input); len(pkg) > 0 && convergenceFailedAcceptedStateChangingEvidenceStep(input, failedStep, pkg) {
		populateProfileStateChangingEvidenceConvergenceContract(input, &contract, pkg)
		return contract
	}
	if convergenceMustPreserveAssessmentSchemaRepair(input, failedStep) {
		populateAssessmentSchemaConvergenceContract(input, &contract)
		return contract
	}
	if convergenceMustPreserveConcreteValidationSchemaRepair(input, failedStep) {
		populateConcreteValidationConvergenceContract(input, &contract)
		return contract
	}
	if boolValue(input["validationRepairEscalatedToImplementation"]) {
		pkg := repeatedValidationRepairEscalationPackage(run, input, failedStep)
		if repairActionPackageRequiresEvidenceAcquisition(pkg) {
			contract.State = convergenceStateNeedEvidence
			contract.RuntimeStage = runtimeStageRepair
			contract.RuntimeStageMode = "strict_convergence"
			contract.RepairObjective = repairObjectiveAcquireFailureEvidence
			contract.ConcreteFailureEvidenceRequired = true
			contract.RepairActionPackage = pkg
			contract.TargetEvidence = convergenceTargetEvidence(input, pkg)
			return contract
		}
		contract.State = convergenceStateNeedStateChange
		contract.RuntimeStage = runtimeStageRepair
		contract.RuntimeStageMode = "strict_convergence"
		contract.RepairObjective = repairObjectiveStateChangingOutput
		contract.StateChangingOutputRequired = true
		if len(pkg) == 0 {
			pkg = stateChangingRepairActionPackageForConvergence(input, failedStep)
		}
		contract.RepairActionPackage = pkg
		contract.TargetEvidence = convergenceTargetEvidence(input, pkg)
		return contract
	}
	if populateCoverageWorkItemConvergenceContract(run, input, &contract) {
		return contract
	}
	if convergenceTargetEvidenceReadyForStateChange(run, input, failedStep) {
		contract.State = convergenceStateNeedStateChange
		contract.RuntimeStage = runtimeStageRepair
		contract.RuntimeStageMode = "strict_convergence"
		contract.RepairObjective = repairObjectiveStateChangingOutput
		contract.StateChangingOutputRequired = true
		if pkg := stateChangingEvidenceRequirementRepairActionPackage(run, input); len(pkg) > 0 {
			contract.RepairActionPackage = pkg
		} else {
			contract.RepairActionPackage = stateChangingRepairActionPackageForConvergence(input, failedStep)
		}
		contract.TargetEvidence = convergenceTargetEvidence(input, contract.RepairActionPackage)
		return contract
	}
	if convergenceNeedsValidation(input, failedStep) {
		if pkg := stateChangingEvidenceRequirementRepairActionPackage(run, input); len(pkg) > 0 {
			populateProfileStateChangingEvidenceConvergenceContract(input, &contract, pkg)
			return contract
		}
		populateConcreteValidationConvergenceContract(input, &contract)
		return contract
	}
	if pkg := stateChangingEvidenceRequirementRepairActionPackage(run, input); len(pkg) > 0 {
		populateProfileStateChangingEvidenceConvergenceContract(input, &contract, pkg)
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
	if convergenceNeedsStateChange(run, input, failedStep) {
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

func populateProfileStateChangingEvidenceConvergenceContract(input map[string]any, contract *ConvergenceContract, pkg map[string]any) {
	if contract == nil {
		return
	}
	if repairActionPackageRequiresEvidenceAcquisition(pkg) {
		contract.State = convergenceStateNeedEvidence
		contract.RuntimeStage = runtimeStageRepair
		contract.RuntimeStageMode = "strict_convergence"
		contract.RepairObjective = repairObjectiveAcquireFailureEvidence
		contract.ConcreteFailureEvidenceRequired = true
		contract.RepairActionPackage = pkg
		contract.TargetEvidence = convergenceTargetEvidence(input, pkg)
		return
	}
	contract.State = convergenceStateNeedStateChange
	contract.RuntimeStage = runtimeStageRepair
	contract.RuntimeStageMode = "strict_convergence"
	contract.RepairObjective = repairObjectiveStateChangingOutput
	contract.StateChangingOutputRequired = true
	contract.RepairActionPackage = pkg
	contract.TargetEvidence = convergenceTargetEvidence(input, pkg)
}

func populateCoverageWorkItemConvergenceContract(run *runModel.Run, input map[string]any, contract *ConvergenceContract) bool {
	if contract == nil {
		return false
	}
	if boolValue(input[stepInputKeyAssessmentOnly]) ||
		boolValue(input["assessmentOnlyContractCorrection"]) ||
		strings.TrimSpace(stringFromMap(input, "repairObjective")) == "complete_dod_goal_coverage_assessment" {
		return false
	}
	if !coverageWorkItemConvergenceEligible(input) {
		return false
	}
	context := coverageWorkItemContextFromInput(input)
	if len(context) == 0 {
		return false
	}
	focus := coverageWorkItemPrimaryFocus(context)
	if len(focus) == 0 {
		return false
	}
	status := strings.TrimSpace(stringFromMap(focus, "status"))
	if status == "satisfied" {
		return false
	}
	missing := stringSet(stringSliceFromValue(focus["missingEvidenceKinds"]))
	if len(missing) == 0 {
		missing = stringSet(coverageMissingEvidenceKinds(focus))
	}
	contract.RepairActionPackage = cloneStepPayload(mapFromValue(input["repairActionPackage"]))
	if len(contract.RepairActionPackage) == 0 {
		contract.RepairActionPackage = map[string]any{}
	}
	contract.RepairActionPackage[coverageWorkItemContextKey] = context
	if len(mapSliceFromValue(contract.RepairActionPackage["coverageTargetSet"])) == 0 {
		if targets := coverageTargetSetFromWorkItemContext(context); len(targets) > 0 {
			contract.RepairActionPackage["coverageRepairRequired"] = true
			contract.RepairActionPackage["coverageTargetSet"] = targets
		}
	}
	if missing["decision"] || missing["approval"] || missing["external_dependency"] || status == "blocked" {
		contract.State = convergenceStateNeedGovernance
		contract.RuntimeStage = runtimeStageDecision
		contract.RuntimeStageMode = "structured_governance"
		contract.GovernanceReason = firstNonEmptyString(stringFromMap(focus, "currentGap"), "coverage work item requires governance evidence")
		return true
	}
	if coverageWorkItemContextHasMissingEvidenceKind(context, "state_change") {
		if pkg := stateChangingEvidenceRequirementRepairActionPackage(run, input); len(pkg) > 0 {
			populateProfileStateChangingEvidenceConvergenceContract(input, contract, pkg)
			contract.RepairActionPackage = pkg
			contract.RepairActionPackage[coverageWorkItemContextKey] = context
			contract.TargetEvidence = convergenceTargetEvidence(input, pkg)
			return true
		}
	}
	if missing["state_change"] {
		if coverageFocusCanStartPrimaryStateChangeWithoutTargetEvidence(focus) {
			contract.State = convergenceStateNeedStateChange
			contract.RuntimeStage = runtimeStageRepair
			contract.RuntimeStageMode = "strict_convergence"
			contract.RepairObjective = repairObjectiveStateChangingOutput
			contract.StateChangingOutputRequired = true
			required := enforceStateChangingRequiredOutputForConvergence(contract.RepairActionPackage)
			required["mustReferenceCoverageWorkItem"] = true
			required["mustIncludePrimaryStateChangingNode"] = true
			required["primaryAcceptedKinds"] = stateChangingRepairAcceptedKinds()
			required["primaryStateChangingRule"] = "when the current focus is a primary deliverable state change, produce the smallest state-changing artifact that directly maps to the focused coverage work item before validation"
			contract.RepairActionPackage["requiredOutput"] = required
			contract.TargetEvidence = convergenceTargetEvidence(input, contract.RepairActionPackage)
			return true
		}
		if repairHasImplementationTargetEvidence(input, contract.RepairActionPackage) ||
			len(mapSliceFromValue(contract.RepairActionPackage["targetSnapshots"])) > 0 ||
			len(filterAdmissibleRuntimeRepairTargetRefs(stringSliceFromValue(contract.RepairActionPackage["repairTargetSet"]))) > 0 {
			contract.State = convergenceStateNeedStateChange
			contract.RuntimeStage = runtimeStageRepair
			contract.RuntimeStageMode = "strict_convergence"
			contract.RepairObjective = repairObjectiveStateChangingOutput
			contract.StateChangingOutputRequired = true
			required := enforceStateChangingRequiredOutputForConvergence(contract.RepairActionPackage)
			required["mustReferenceCoverageWorkItem"] = true
			contract.RepairActionPackage["requiredOutput"] = required
			contract.TargetEvidence = convergenceTargetEvidence(input, contract.RepairActionPackage)
			return true
		}
		contract.State = convergenceStateNeedEvidence
		contract.RuntimeStage = runtimeStageRepair
		contract.RuntimeStageMode = "strict_convergence"
		contract.RepairObjective = repairObjectiveAcquireFailureEvidence
		contract.ConcreteFailureEvidenceRequired = true
		required := evidenceAcquisitionRequiredOutput(mapFromValue(contract.RepairActionPackage["requiredOutput"]))
		required["mustReferenceCoverageWorkItem"] = true
		contract.RepairActionPackage["requiredOutput"] = required
		return true
	}
	if missing["validation"] || missing["artifact"] {
		if concreteValidationFailureCanDriveStateChange(input, contract.RepairActionPackage) {
			contract.State = convergenceStateNeedStateChange
			contract.RuntimeStage = runtimeStageRepair
			contract.RuntimeStageMode = "strict_convergence"
			contract.RepairObjective = repairObjectiveStateChangingOutput
			contract.StateChangingOutputRequired = true
			required := enforceStateChangingRequiredOutputForConvergence(contract.RepairActionPackage)
			required["mustReferenceCoverageWorkItem"] = true
			contract.RepairActionPackage["requiredOutput"] = required
			contract.TargetEvidence = convergenceTargetEvidence(input, contract.RepairActionPackage)
			return true
		}
		contract.State = convergenceStateNeedValidation
		contract.RuntimeStage = runtimeStageVerification
		contract.RuntimeStageMode = "strict_convergence"
		contract.RepairObjective = repairObjectiveConcreteValidationEvidence
		required := concreteValidationEvidenceRequiredOutput(mapFromValue(contract.RepairActionPackage["requiredOutput"]))
		required["mustReferenceCoverageWorkItem"] = true
		contract.RepairActionPackage["requiredOutput"] = required
		return true
	}
	contract.State = convergenceStateNeedAssessment
	contract.RuntimeStage = runtimeStageVerification
	contract.RuntimeStageMode = "strict_convergence"
	contract.RepairObjective = "complete_dod_goal_coverage_assessment"
	return true
}

func coverageFocusCanStartPrimaryStateChangeWithoutTargetEvidence(focus map[string]any) bool {
	if len(focus) == 0 {
		return false
	}
	missing := stringSet(stringSliceFromValue(focus["missingEvidenceKinds"]))
	if len(missing) == 0 {
		missing = stringSet(coverageMissingEvidenceKinds(focus))
	}
	return missing["state_change"] && strings.TrimSpace(stringFromMap(focus, "sourceScope")) != ""
}

func coverageWorkItemContextHasMissingEvidenceKind(context map[string]any, evidenceKind string) bool {
	evidenceKind = strings.TrimSpace(evidenceKind)
	if len(context) == 0 || evidenceKind == "" {
		return false
	}
	for _, key := range []string{"currentFocus", "currentFocusItems", "unresolvedItems", "ledgerSummary"} {
		if item := mapFromValue(context[key]); len(item) > 0 && stringSet(stringSliceFromValue(item["missingEvidenceKinds"]))[evidenceKind] {
			return true
		}
		for _, item := range mapSliceFromValue(context[key]) {
			if stringSet(stringSliceFromValue(item["missingEvidenceKinds"]))[evidenceKind] {
				return true
			}
		}
	}
	return false
}

func convergenceFailedAcceptedStateChangingEvidenceStep(input map[string]any, failedStep *runModel.Step, pkg map[string]any) bool {
	if failedStep == nil || len(input) == 0 || len(pkg) == 0 {
		return false
	}
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) {
		return false
	}
	acceptedKinds := mapStringSet(stringSliceFromValue(required["acceptedKinds"]))
	if !acceptedKinds[strings.TrimSpace(failedStep.Kind)] {
		return false
	}
	if len(stringSliceFromValue(required["acceptedOperations"])) > 0 {
		operation := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
			stringFromMap(failedStep.Input, "operation"),
			stringFromMap(failedStep.Output, "operation"),
		)))
		if !mapStringSet(lowerStringSlice(stringSliceFromValue(required["acceptedOperations"])))[operation] {
			return false
		}
	}
	return true
}

func concreteValidationFailureCanDriveStateChange(input map[string]any, pkg map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if len(mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"])) > 0 ||
		boolValue(input["stateChangeAlreadySatisfiedByCurrentTarget"]) {
		return false
	}
	if len(pkg) > 0 {
		if len(mapSliceFromValue(pkg["currentTargetAlreadySatisfiesFailedPatch"])) > 0 ||
			boolValue(pkg["stateChangeAlreadySatisfiedByCurrentTarget"]) {
			return false
		}
	}
	if !inputHasConcreteValidationFailureDiagnostic(input, pkg) {
		return false
	}
	return repairHasImplementationTargetEvidence(input, pkg)
}

func inputHasConcreteValidationFailureDiagnostic(input map[string]any, pkg map[string]any) bool {
	if len(input) == 0 && len(pkg) == 0 {
		return false
	}
	if diagnosticsIndicateConcreteFailure(mapFromValue(input["lastFailureDiagnostics"])) ||
		diagnosticsIndicateConcreteFailure(mapFromValue(pkg["lastFailureDiagnostics"])) {
		return true
	}
	if signals := mapFromValue(input["structuredFailureSignals"]); structuredFailureSignalsHaveConcreteTargetEvidence(signals) {
		return true
	}
	if signals := mapFromValue(pkg["structuredFailureSignals"]); structuredFailureSignalsHaveConcreteTargetEvidence(signals) {
		return true
	}
	return progressHasConcreteTargetEvidence(mapFromValue(input["currentExecutionProgress"])) ||
		progressHasConcreteTargetEvidence(mapFromValue(pkg["currentExecutionProgress"]))
}

func diagnosticsIndicateConcreteFailure(diagnostics map[string]any) bool {
	if len(diagnostics) == 0 {
		return false
	}
	if len(filterAdmissibleRuntimeRepairTargetRefs(stringSliceFromValue(diagnostics["failureTargets"]))) > 0 {
		return true
	}
	return structuredFailureSignalsHaveConcreteTargetEvidence(structuredFailureSignalsFromFailureOutput(diagnostics, nil, "", ""))
}

func coverageWorkItemConvergenceEligible(input map[string]any) bool {
	if len(input) == 0 || boolValue(input["replanStateChangingContinuation"]) {
		return false
	}
	stage := strings.TrimSpace(firstNonEmptyString(stringFromMap(input, stepInputKeyRuntimeStage), stringFromMap(input, "currentRuntimeStage")))
	mode := strings.TrimSpace(firstNonEmptyString(stringFromMap(input, stepInputKeyRuntimeStageMode), stringFromMap(input, "currentRuntimeStageMode")))
	if mode != "strict_convergence" && !boolValue(input[stepInputKeyRuntimeRepairFollowUp]) {
		return false
	}
	if boolValue(input[stepInputKeyRuntimeRepairFollowUp]) ||
		boolValue(input["coverageRepairRequired"]) ||
		strings.TrimSpace(stringFromMap(input, "repairObjective")) != "" ||
		len(mapFromValue(input["repairActionPackage"])) > 0 {
		switch stage {
		case runtimeStageRepair, runtimeStageVerification:
			return true
		}
	}
	return false
}

func convergenceMustPreserveConcreteValidationSchemaRepair(input map[string]any, failedStep *runModel.Step) bool {
	if len(input) == 0 || failedStep == nil || !stateChangingSchemaInvalidRepair(failedStep) {
		return false
	}
	if inputRequiresConcreteValidationEvidence(input) || inputRequiresConcreteValidationEvidence(failedStep.Input) {
		return true
	}
	if strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveConcreteValidationEvidence ||
		strings.TrimSpace(stringFromMap(failedStep.Input, "repairObjective")) == repairObjectiveConcreteValidationEvidence {
		return true
	}
	return false
}

func convergenceMustPreserveAssessmentSchemaRepair(input map[string]any, failedStep *runModel.Step) bool {
	if len(input) == 0 || failedStep == nil {
		return false
	}
	if len(mapFromValue(input["assessmentSchemaInvalidRecovery"])) > 0 {
		return true
	}
	if strings.TrimSpace(stringFromMap(input, "schemaInvalidRepairMode")) == "single_valid_json_decision" &&
		strings.TrimSpace(stringFromMap(input, "assessmentOnlyReason")) == "assessment_schema_invalid_repair" {
		return true
	}
	return assessmentOnlySchemaInvalidRepairRequired(failedStep)
}

func populateConcreteValidationConvergenceContract(input map[string]any, contract *ConvergenceContract) {
	if contract == nil {
		return
	}
	contract.State = convergenceStateNeedValidation
	contract.RuntimeStage = runtimeStageVerification
	contract.RuntimeStageMode = "strict_convergence"
	contract.RepairObjective = repairObjectiveConcreteValidationEvidence
	ensureCoverageRepairTargetSet(input)
	contract.RepairActionPackage = cloneStepPayload(mapFromValue(input["repairActionPackage"]))
	if len(contract.RepairActionPackage) == 0 {
		contract.RepairActionPackage = map[string]any{}
	}
	if targets := coverageTargetSetFromRepairInput(input); len(targets) > 0 {
		contract.RepairActionPackage["coverageRepairRequired"] = true
		contract.RepairActionPackage["coverageTargetSet"] = targets
	}
	required := concreteValidationEvidenceRequiredOutput(mapFromValue(contract.RepairActionPackage["requiredOutput"]))
	if len(mapSliceFromValue(contract.RepairActionPackage["coverageTargetSet"])) > 0 {
		required["mustDeclareCoverageTargets"] = true
		required["coverageTargetMatchRule"] = "validation evidence must map to one unresolved DoD item text, goalRequirementId, goal item, or deliverable item from coverageTargetSet"
	}
	contract.RepairActionPackage["requiredOutput"] = required
	stripRepairActionPackageImplementationTargetEvidence(contract.RepairActionPackage)
	contract.TargetEvidence = convergenceTargetEvidence(input, contract.RepairActionPackage)
}

func populateAssessmentSchemaConvergenceContract(input map[string]any, contract *ConvergenceContract) {
	if contract == nil {
		return
	}
	contract.State = convergenceStateNeedAssessment
	contract.RuntimeStage = runtimeStageVerification
	contract.RuntimeStageMode = "strict_convergence"
	contract.RepairObjective = "complete_dod_goal_coverage_assessment"
}

func repeatedValidationRepairEscalationPackage(run *runModel.Run, input map[string]any, failedStep *runModel.Step) map[string]any {
	if pkg := stateChangingEvidenceRequirementRepairActionPackage(run, input); len(pkg) > 0 {
		return pkg
	}
	var rawFailedOutput []map[string]any
	if failedStep != nil && len(failedStep.Output) > 0 {
		rawFailedOutput = append(rawFailedOutput, failedStep.Output)
	}
	pkg := repairActionPackageFromRepairInput(input, rawFailedOutput...)
	if len(pkg) == 0 {
		pkg = cloneStepPayload(mapFromValue(input["repairActionPackage"]))
	}
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	if repairActionPackageRequiresEvidenceAcquisition(pkg) {
		return pkg
	}
	required := enforceStateChangingRequiredOutputForConvergence(pkg)
	addStrategyReassessmentRequiredOutputContract(pkg, required)
	return pkg
}

func hydrateConvergenceTargetEvidence(input map[string]any) {
	if len(input) == 0 {
		return
	}
	contract := mapFromValue(input["convergenceContract"])
	targetEvidence := mapFromValue(contract["targetEvidence"])
	if len(targetEvidence) == 0 {
		return
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	pkgChanged := false
	inherit := func(key string) {
		value, ok := targetEvidence[key]
		if !ok || value == nil {
			return
		}
		if input[key] == nil {
			input[key] = cloneStepInputValue(value)
		}
		if pkg[key] == nil {
			pkg[key] = cloneStepInputValue(value)
			pkgChanged = true
		}
	}
	for _, key := range []string{
		"repairTargetSet",
		"structuredFailureSignals",
		"failureTargetRepairContext",
		"patchRepairContext",
		"coverageTargetSet",
		"targetSnapshots",
		"primaryDiagnosticRepair",
	} {
		inherit(key)
	}
	if len(mapSliceFromValue(pkg["currentTargetSnapshots"])) == 0 {
		if snapshots := mapSliceFromValue(pkg["targetSnapshots"]); len(snapshots) > 0 {
			pkg["currentTargetSnapshots"] = repairActionTargetSnapshotMetadata(snapshots)
			pkgChanged = true
		}
	}
	if pkgChanged || len(pkg) > 0 {
		input["repairActionPackage"] = pkg
	}
}

func refreshConvergenceCoverageTargets(input map[string]any) {
	if len(input) == 0 {
		return
	}
	targets := coverageTargetSetFromRepairInput(input)
	if len(targets) == 0 {
		return
	}
	input["coverageRepairRequired"] = true
	input["coverageTargetSet"] = targets
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		return
	}
	pkg["coverageRepairRequired"] = true
	pkg["coverageTargetSet"] = targets
	if boolValue(pkg["capabilityPolicyRepairRequired"]) {
		input["repairActionPackage"] = pkg
		return
	}
	if promoteRepairActionPackagePrerequisiteRootCauseCandidates(pkg) &&
		stateChangingRepairHasConcreteTargetEvidence(input, pkg) {
		input["repairActionPackage"] = pkg
		return
	}
	if profileStateChangingPackageMatchesCoverageTargets(pkg, targets) {
		input["repairActionPackage"] = pkg
		return
	}
	delete(pkg, "evidenceKindRequirements")
	delete(pkg, "targetEvidence")
	delete(pkg, "targetSnapshots")
	delete(pkg, "currentTargetSnapshots")
	delete(pkg, "repairTargetSet")
	required := mapFromValue(pkg["requiredOutput"])
	if boolValue(required["mustIncludeStateChangingNode"]) {
		delete(pkg, "requiredOutput")
	}
	input["repairActionPackage"] = pkg
}

func profileStateChangingPackageMatchesCoverageTargets(pkg map[string]any, targets []map[string]any) bool {
	if len(pkg) == 0 || len(targets) == 0 {
		return false
	}
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) {
		return true
	}
	requirements := make([]profileEvidenceKindRequirement, 0)
	for _, item := range mapSliceFromValue(pkg["evidenceKindRequirements"]) {
		requirement := profileEvidenceKindRequirement{
			Scope:              stringFromMap(item, "scope"),
			ID:                 stringFromMap(item, "id"),
			AcceptedStepKinds:  stringSliceFromValue(item["acceptedStepKinds"]),
			AcceptedOperations: stringSliceFromValue(item["acceptedOperations"]),
		}
		if requirement.ID != "" && requirement.Scope != "" {
			requirements = append(requirements, requirement)
		}
	}
	if len(requirements) == 0 {
		return false
	}
	for _, requirement := range requirements {
		for _, target := range targets {
			if evidenceKindRequirementMatchesCoverageTarget(requirement, target) {
				return true
			}
		}
	}
	return false
}

func convergenceTargetEvidenceReadyForStateChange(run *runModel.Run, input map[string]any, failedStep *runModel.Step) bool {
	if len(input) == 0 {
		return false
	}
	if len(mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"])) > 0 ||
		boolValue(input["stateChangeAlreadySatisfiedByCurrentTarget"]) {
		return false
	}
	if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
		if len(mapSliceFromValue(pkg["currentTargetAlreadySatisfiesFailedPatch"])) > 0 ||
			boolValue(pkg["stateChangeAlreadySatisfiedByCurrentTarget"]) {
			return false
		}
	}
	if convertedReadableDecisionRepairRequiresAssessmentOnly(run, input, failedStep) {
		return false
	}
	if !coverageVerificationGapPresent(input, failedStep) {
		return false
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveConcreteValidationEvidence &&
		len(coverageTargetSetFromRepairInput(input)) > 0 &&
		!repairHasDirectImplementationTargetEvidence(input, pkg) {
		return false
	}
	if !repairHasImplementationTargetEvidence(input, pkg) {
		return false
	}
	return inputRequiresConcreteValidationEvidence(input) ||
		strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveConcreteValidationEvidence ||
		stringFromMap(mapFromValue(input["convergenceContract"]), "state") == convergenceStateNeedValidation
}

func convertedReadableDecisionRepairRequiresAssessmentOnly(run *runModel.Run, input map[string]any, failedStep *runModel.Step) bool {
	if len(input) == 0 || failedStep == nil {
		return false
	}
	if !convertedDecisionGateFailure(failedStep.Output) {
		return false
	}
	if len(missingGoalCoverageFromInput(input)) > 0 ||
		len(mapSliceFromValue(input["missingDeliverableCoverage"])) > 0 {
		return false
	}
	if (len(mapSliceFromValue(input["missingDoDItems"])) > 0 ||
		len(mapSliceFromValue(input["coverageTargetSet"])) > 0) &&
		!convertedReadableDecisionHasOnlySyntheticFallbackCoverage(run, input) {
		return false
	}
	return len(mapSliceFromValue(input["readableResourceRepairContext"])) > 0
}

func convertedReadableDecisionHasOnlySyntheticFallbackCoverage(run *runModel.Run, input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if len(effectiveDoDItemTexts(run)) > 0 ||
		len(normalizedRunGoalCoverageItems(run)) > 0 ||
		len(requiredProfileDeliverableExpectations(run)) > 0 {
		return false
	}
	for _, item := range mapSliceFromValue(input["missingDoDItems"]) {
		if !syntheticFallbackDoDCoverageItem(item) {
			return false
		}
	}
	for _, item := range mapSliceFromValue(input["coverageTargetSet"]) {
		if !syntheticFallbackDoDCoverageItem(item) {
			return false
		}
	}
	return len(mapSliceFromValue(input["missingDoDItems"])) > 0 ||
		len(mapSliceFromValue(input["coverageTargetSet"])) > 0
}

func syntheticFallbackDoDCoverageItem(item map[string]any) bool {
	if len(item) == 0 {
		return false
	}
	for _, key := range []string{"item", "targetText", "coverageTargetToken"} {
		if text := strings.TrimSpace(stringFromMap(item, key)); text != "" && text != "effective DoD" {
			return false
		}
	}
	if kind := strings.TrimSpace(stringFromMap(item, "kind")); kind != "" && kind != "dod" && kind != "leaf" {
		return false
	}
	return true
}

func repairHasDirectImplementationTargetEvidence(input map[string]any, pkg map[string]any) bool {
	if len(input) > 0 {
		for _, key := range []string{"failureTargetRepairContext", "patchRepairContext", "rootCauseCandidateContext"} {
			if repairContextHasAdmissibleTargetEvidence(input[key]) {
				return true
			}
		}
		if len(repairActionTargetSnapshots(input)) > 0 {
			return true
		}
		if signals := mapFromValue(input["structuredFailureSignals"]); len(signals) > 0 && structuredFailureSignalsHaveConcreteTargetEvidence(signals) {
			return true
		}
	}
	if len(pkg) > 0 {
		if repairContextHasAdmissibleTargetEvidence(pkg["targetSnapshots"]) {
			return true
		}
		if len(filterAdmissibleRuntimeRepairTargetRefs(stringSliceFromValue(pkg["repairTargetSet"]))) > 0 {
			return true
		}
		if primary := mapFromValue(pkg["primaryDiagnosticRepair"]); len(primary) > 0 && structuredFailureSignalsHaveConcreteTargetEvidence(primary) {
			return true
		}
		if signals := mapFromValue(pkg["structuredFailureSignals"]); len(signals) > 0 && structuredFailureSignalsHaveConcreteTargetEvidence(signals) {
			return true
		}
	}
	return false
}

func stateChangingRepairActionPackageForConvergence(input map[string]any, failedStep *runModel.Step) map[string]any {
	existing := cloneStepPayload(mapFromValue(input["repairActionPackage"]))
	if len(existing) > 0 {
		required := mapFromValue(existing["requiredOutput"])
		if boolValue(required["mustIncludeStateChangingNode"]) {
			applyConcreteRepairEvidenceOutputContract(input, existing, required)
			addStrategyReassessmentRequiredOutputContract(existing, required)
			return existing
		}
	}
	var rawFailedOutput []map[string]any
	if failedStep != nil && len(failedStep.Output) > 0 {
		rawFailedOutput = append(rawFailedOutput, failedStep.Output)
	}
	pkg := repairActionPackageFromRepairInput(input, rawFailedOutput...)
	if len(pkg) == 0 {
		pkg = existing
	}
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	required := enforceStateChangingRequiredOutputForConvergence(pkg)
	addStrategyReassessmentRequiredOutputContract(pkg, required)
	return pkg
}

func enforceStateChangingRequiredOutputForConvergence(pkg map[string]any) map[string]any {
	required := stateChangingRepairRequiredOutput()
	preserveStateChangingRequiredOutputSignals(required, mapFromValue(pkg["requiredOutput"]))
	if len(mapSliceFromValue(pkg["targetSnapshots"])) > 0 || len(mapSliceFromValue(pkg["currentTargetSnapshots"])) > 0 {
		required["mustTargetFailureScope"] = true
		required["targetRelation"] = "failure_target_repair_snapshot_or_declared_related_failure_target"
		applyCurrentSnapshotDirectPatchContract(required)
	}
	if len(stringSliceFromValue(pkg["repairTargetSet"])) > 0 {
		required["mustTargetRepairTargetSet"] = true
	}
	if len(mapSliceFromValue(pkg["coverageTargetSet"])) > 0 {
		required["mustDeclareCoverageTargets"] = true
		required["coverageTargetMatchRule"] = "patch/apply coverageTargets must match one unresolved DoD item text, goalRequirementId, goal item, or deliverable item from coverageTargetSet"
	}
	pkg["requiredOutput"] = required
	return required
}

func preserveStateChangingRequiredOutputSignals(required map[string]any, existing map[string]any) {
	if required == nil || len(existing) == 0 {
		return
	}
	for key, value := range existing {
		if value == nil {
			continue
		}
		switch key {
		case "mustReturnValidJSON",
			"mustNotReuseRejectedJSON",
			"forbiddenStalePatchReuse",
			"mustPatchDirectlyFromProvidedSnapshot",
			"sameGraphReadPatchRule",
			"patchApplyFieldRule",
			"readBeforePatchRecoveryRule",
			"mustDeclareStrategyReassessmentFields",
			"strategyReassessmentRequiredFields",
			"strategyReassessmentMatchRule",
			"mustReassessRepeatedFailedSurface",
			"estimatedPatchContentBytes",
			"estimatedGraphBytes",
			"expectedStateChangingArtifactCount",
			"maxArtifactChunkBytes",
			"previousChunkTailBytes":
			required[key] = value
		}
	}
}

func inheritRejectedResultCoverageContract(input map[string]any, source map[string]any) bool {
	if input == nil || len(source) == 0 {
		return false
	}
	coverageGaps := mapFromValue(source["coverageGaps"])
	inherited := false
	inheritSlice := func(targetKey string, value any) {
		if len(mapSliceFromValue(input[targetKey])) > 0 {
			return
		}
		items := mapSliceFromValue(value)
		if len(items) == 0 {
			return
		}
		input[targetKey] = cloneMapSlice(items)
		inherited = true
	}
	inheritSlice("missingDoDItems", source["unsatisfiedDoDItems"])
	inheritSlice("missingGoalCoverage", coverageGaps["missingGoalCoverage"])
	inheritSlice("missingDeliverableCoverage", coverageGaps["missingDeliverableCoverage"])
	if !inherited {
		return false
	}
	input["coverageRepairRequired"] = true
	if inputRequiresStateChangingOutput(input) ||
		coverageWorkItemContextHasMissingEvidenceKind(coverageWorkItemContextFromInput(input), "state_change") {
		ensureCoverageRepairTargetSet(input)
		return true
	}
	input["repairObjective"] = repairObjectiveConcreteValidationEvidence
	input["boundedValidationRequired"] = true
	input["evidenceRepairRequired"] = true
	delete(input, "stateChangingOutputRequired")
	delete(input, "concreteFailureEvidenceRequired")
	ensureCoverageRepairTargetSet(input)
	return true
}

func ApplyConvergenceContract(input map[string]any, contract ConvergenceContract) map[string]any {
	if input == nil {
		input = map[string]any{}
	}
	if contract.State == "" || contract.State == convergenceStateBlocked {
		return input
	}
	clearConvergenceDerivedOutputModeFields(input)
	input["convergenceContract"] = compactConvergenceContract(contract)
	switch contract.State {
	case convergenceStateNeedEvidence:
		setRuntimeStage(input, runtimeStageRepair)
		input[stepInputKeySoulStage] = soulStageExecution
		input["repairObjective"] = repairObjectiveAcquireFailureEvidence
		input["concreteFailureEvidenceRequired"] = true
		delete(input, "stateChangingOutputRequired")
		delete(input, "boundedValidationRequired")
		delete(input, "evidenceRepairRequired")
		pkg := cloneStepPayload(contract.RepairActionPackage)
		if len(pkg) == 0 {
			pkg = map[string]any{}
		}
		required := evidenceAcquisitionRequiredOutput(mapFromValue(pkg["requiredOutput"]))
		pkg["requiredOutput"] = required
		pkg["evidenceAcquisitionRequired"] = true
		sanitizeRepairActionPackageTargetEvidence(pkg)
		input["repairActionPackage"] = pkg
	case convergenceStateNeedStateChange:
		setRuntimeStage(input, runtimeStageRepair)
		input[stepInputKeySoulStage] = soulStageExecution
		input["repairObjective"] = repairObjectiveStateChangingOutput
		input["stateChangingOutputRequired"] = true
		delete(input, "concreteFailureEvidenceRequired")
		delete(input, "boundedValidationRequired")
		delete(input, "evidenceRepairRequired")
		delete(input, stepInputKeyAssessmentOnly)
		delete(input, "assessmentOnlyReason")
		input["repairGuidance"] = stateChangingConvergenceGuidance(input["repairGuidance"])
		if len(contract.RepairActionPackage) > 0 {
			input["repairActionPackage"] = contract.RepairActionPackage
		}
	case convergenceStateNeedValidation:
		setRuntimeStage(input, runtimeStageVerification)
		input[stepInputKeySoulStage] = soulStageValidation
		input["repairObjective"] = repairObjectiveConcreteValidationEvidence
		input["boundedValidationRequired"] = true
		delete(input, stepInputKeyAssessmentOnly)
		delete(input, "assessmentOnlyReason")
		delete(input, "stateChangingOutputRequired")
		delete(input, "concreteFailureEvidenceRequired")
		pkg := cloneStepPayload(contract.RepairActionPackage)
		if len(pkg) == 0 {
			pkg = cloneStepPayload(mapFromValue(input["repairActionPackage"]))
		}
		if len(pkg) == 0 {
			pkg = map[string]any{}
		}
		required := concreteValidationEvidenceRequiredOutput(mapFromValue(pkg["requiredOutput"]))
		if targets := coverageTargetSetFromRepairInput(input); len(targets) > 0 {
			input["coverageRepairRequired"] = true
			input["coverageTargetSet"] = targets
			pkg["coverageRepairRequired"] = true
			pkg["coverageTargetSet"] = targets
			required["mustDeclareCoverageTargets"] = true
			required["coverageTargetMatchRule"] = "validation evidence must map to one unresolved DoD item text, goalRequirementId, goal item, or deliverable item from coverageTargetSet"
		} else if targets := mapSliceFromValue(pkg["coverageTargetSet"]); len(targets) > 0 {
			input["coverageRepairRequired"] = true
			input["coverageTargetSet"] = targets
			pkg["coverageRepairRequired"] = true
			pkg["coverageTargetSet"] = targets
			required["mustDeclareCoverageTargets"] = true
			required["coverageTargetMatchRule"] = "validation evidence must map to one unresolved DoD item text, goalRequirementId, goal item, or deliverable item from coverageTargetSet"
		}
		if progress := mapFromValue(input["currentExecutionProgress"]); len(progress) > 0 {
			pkg["currentExecutionProgress"] = compactLLMContextMap(progress, 0)
		}
		pkg["requiredOutput"] = required
		stripRepairActionPackageImplementationTargetEvidence(pkg)
		input["repairActionPackage"] = pkg
	case convergenceStateNeedAssessment:
		setRuntimeStage(input, runtimeStageVerification)
		input[stepInputKeySoulStage] = soulStageValidation
		input[stepInputKeyAssessmentOnly] = true
		if boolValue(input["assessmentOnlyContractCorrection"]) {
			delete(input, "repairObjective")
		} else {
			input["repairObjective"] = firstNonEmptyString(contract.RepairObjective, "complete_dod_goal_coverage_assessment")
		}
		delete(input, "stateChangingOutputRequired")
		delete(input, "concreteFailureEvidenceRequired")
		delete(input, "boundedValidationRequired")
		delete(input, "evidenceRepairRequired")
		stripRepairActionPackageExecutableOutputContract(input)
	case convergenceStateNeedGovernance:
		setRuntimeStage(input, runtimeStageDecision)
		input[stepInputKeySoulStage] = soulStageDecision
		delete(input, stepInputKeyAssessmentOnly)
		delete(input, "assessmentOnlyReason")
		delete(input, "repairObjective")
		delete(input, "stateChangingOutputRequired")
		delete(input, "concreteFailureEvidenceRequired")
		delete(input, "boundedValidationRequired")
		delete(input, "evidenceRepairRequired")
		delete(input, "repairGuidance")
		stripRepairActionPackageExecutableOutputContract(input)
		input["governanceRequired"] = true
		if contract.GovernanceReason != "" {
			input["governanceReason"] = contract.GovernanceReason
		}
	}
	return input
}

func stripRepairActionPackageImplementationTargetEvidence(pkg map[string]any) {
	if len(pkg) == 0 {
		return
	}
	for _, key := range []string{
		"targetEvidence",
		"targetSnapshots",
		"currentTargetSnapshots",
		"repairTargetSet",
		"failureTargetRepairContext",
		"patchRepairContext",
		"primaryDiagnosticRepair",
	} {
		delete(pkg, key)
	}
	required := mapFromValue(pkg["requiredOutput"])
	for _, key := range []string{
		"mustTargetFailureScope",
		"mustTargetRepairTargetSet",
		"targetRelation",
		"mustUseCurrentTargetSnapshot",
		"mustPatchDirectlyFromProvidedSnapshot",
		"sameGraphReadPatchRule",
		"fileWriteExistingTargetRule",
		"primaryDiagnosticMatchRule",
		"mustAddressPrimaryDiagnostics",
	} {
		delete(required, key)
	}
	if len(required) > 0 {
		pkg["requiredOutput"] = required
	}
}

func clearConvergenceDerivedOutputModeFields(input map[string]any) {
	if len(input) == 0 {
		return
	}
	delete(input, "outputMode")
	delete(input, "outputModeSelector")
	delete(input, "artifactOutputContract")
	delete(input, "artifactContinuationContext")
	delete(input, "artifactContinuationId")
}

func stripRepairActionPackageExecutableOutputContract(input map[string]any) {
	if len(input) == 0 {
		return
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		return
	}
	for _, key := range []string{"requiredOutput", "forbiddenActions", "exitCriteria", "evidenceAcquisitionRequired"} {
		delete(pkg, key)
	}
	if len(pkg) == 0 {
		delete(input, "repairActionPackage")
		return
	}
	input["repairActionPackage"] = pkg
}

func stateChangingConvergenceGuidance(existing any) []string {
	return appendRepairGuidanceWithout(existing, []string{
		"return only completed or failed dodassessment",
		"do not return executiongraph",
		"do not return execution graph",
		"do not return shellcommands",
		"do not return mcp/toolcalls",
		"do not return patchrequests",
		"do not return other executable work",
	}, append(stateChangingRepairGuidance(),
		"When stateChangingOutputRequired is true, dodAssessment alone cannot satisfy the repair contract; first produce an accepted executable state-changing operation or return a real governance blocker.",
	)...)
}

func appendRepairGuidanceWithout(existing any, blockedSubstrings []string, items ...string) []string {
	ret := []string{}
	seen := map[string]bool{}
	blocked := make([]string, 0, len(blockedSubstrings))
	for _, item := range blockedSubstrings {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			blocked = append(blocked, item)
		}
	}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		lower := strings.ToLower(value)
		for _, marker := range blocked {
			if strings.Contains(lower, marker) {
				return
			}
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, item := range stringListFromValue(existing) {
		add(item)
	}
	for _, item := range items {
		add(item)
	}
	return ret
}

func ShouldCreateDecisionTicket(input map[string]any, llmOutput map[string]any, contract ConvergenceContract) bool {
	spec := decisionBlockSpec(llmOutput)
	if spec.Reason == "" {
		return true
	}
	if decisionBlockDeclaresExternalOrRuntimeDependency(spec.Context) {
		return true
	}
	switch contract.State {
	case convergenceStateNeedEvidence:
		return !mechanicalEvidenceAcquisitionContractCanContinue(input, contract)
	case convergenceStateNeedStateChange:
		return !mechanicalStateChangingContractCanContinue(contract)
	case convergenceStateNeedValidation:
		return !mechanicalConcreteValidationContractCanContinue(input, contract)
	default:
		return true
	}
}

func mechanicalStateChangingContractCanContinue(contract ConvergenceContract) bool {
	if !contract.StateChangingOutputRequired {
		return false
	}
	if len(contract.TargetEvidence) > 0 {
		return true
	}
	pkg := mapFromValue(contract.RepairActionPackage)
	if boolValue(pkg["capabilityPolicyRepairRequired"]) {
		return true
	}
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) || len(mapSliceFromValue(pkg["coverageTargetSet"])) == 0 || len(mapSliceFromValue(pkg["evidenceKindRequirements"])) == 0 {
		return false
	}
	for _, kind := range stringSliceFromValue(required["acceptedKinds"]) {
		if stepKindCanProduceStateChangingEvidence(kind) {
			return true
		}
	}
	return false
}

func mechanicalEvidenceAcquisitionContractCanContinue(input map[string]any, contract ConvergenceContract) bool {
	if !contract.ConcreteFailureEvidenceRequired && !boolValue(input["concreteFailureEvidenceRequired"]) {
		return false
	}
	pkg := mapFromValue(contract.RepairActionPackage)
	if len(pkg) == 0 {
		pkg = mapFromValue(input["repairActionPackage"])
	}
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustProduceConcreteFailureEvidence"]) {
		return false
	}
	accepted := stringSliceFromValue(required["acceptedKinds"])
	for _, kind := range evidenceAcquisitionAcceptedKinds() {
		if stringSliceContains(accepted, kind) {
			return true
		}
	}
	return false
}

func mechanicalConcreteValidationContractCanContinue(input map[string]any, contract ConvergenceContract) bool {
	pkg := mapFromValue(contract.RepairActionPackage)
	if len(pkg) == 0 {
		pkg = mapFromValue(input["repairActionPackage"])
	}
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustProduceConcreteValidationEvidence"]) {
		return false
	}
	accepted := stringSliceFromValue(required["acceptedKinds"])
	return stringSliceContains(accepted, "shell/command") || stringSliceContains(accepted, "mcp/tool_call")
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
	if convergenceConcreteValidationCannotAdvanceWithoutTarget(input, failedStep) {
		return false
	}
	if strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveConcreteValidationEvidence {
		return true
	}
	if convergenceCoverageVerificationEvidenceGap(input, failedStep) {
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

func stateChangingEvidenceRequirementRepairActionPackage(run *runModel.Run, input map[string]any) map[string]any {
	if run == nil || len(input) == 0 {
		return nil
	}
	if boolValue(mapFromValue(input["repairActionPackage"])["capabilityPolicyRepairRequired"]) {
		return nil
	}
	refreshRepairStrategyReassessment(input)
	targets := coverageTargetSetFromRepairInput(input)
	if len(targets) == 0 {
		targets = mapSliceFromValue(input["coverageTargetSet"])
	}
	if len(targets) == 0 {
		return nil
	}
	requirements := coverageTargetEvidenceKindRequirements(run, targets)
	if len(requirements) == 0 {
		return nil
	}
	acceptedKinds := make([]string, 0)
	acceptedOperations := make([]string, 0)
	requirementSummaries := make([]map[string]any, 0, len(requirements))
	requiresStateChange := false
	matchedRequirements := make([]profileEvidenceKindRequirement, 0, len(requirements))
	for _, requirement := range requirements {
		if len(requirement.AcceptedStepKinds) == 0 {
			continue
		}
		if !profileRequirementRequiresStateChangingEvidence(requirement) {
			continue
		}
		requiresStateChange = true
		matchedRequirements = append(matchedRequirements, requirement)
		acceptedKinds = append(acceptedKinds, requirement.AcceptedStepKinds...)
		acceptedOperations = append(acceptedOperations, requirement.AcceptedOperations...)
		requirementSummaries = append(requirementSummaries, map[string]any{
			"scope":              normalizeEvidenceKindRequirementScope(requirement.Scope),
			"id":                 requirement.ID,
			"acceptedStepKinds":  compactUniqueStrings(requirement.AcceptedStepKinds),
			"acceptedOperations": compactUniqueStrings(requirement.AcceptedOperations),
			"requireDirect":      requirement.RequireDirect,
			"description":        requirement.Description,
		})
	}
	if !requiresStateChange {
		return nil
	}
	primaryStateChangeRequired := coverageTargetsRequirePrimaryStateChangingOutput(targets, matchedRequirements)
	pkg := cloneStepPayload(mapFromValue(input["repairActionPackage"]))
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	pkg["coverageRepairRequired"] = true
	pkg["coverageTargetSet"] = targets
	pkg["evidenceKindRequirements"] = requirementSummaries
	if progress := mapFromValue(input["currentExecutionProgress"]); len(progress) > 0 {
		pkg["currentExecutionProgress"] = compactLLMContextMap(progress, 0)
	}
	required := stateChangingRepairRequiredOutput()
	for key, value := range mapFromValue(pkg["requiredOutput"]) {
		switch key {
		case "mustReturnValidJSON", "mustNotReuseRejectedJSON", "forbiddenStalePatchReuse", "mustDeclareStrategyReassessmentFields", "strategyReassessmentRequiredFields", "strategyReassessmentMatchRule", "mustReassessRepeatedFailedSurface":
			required[key] = value
		}
	}
	if primaryStateChangeRequired {
		acceptedKinds = append(acceptedKinds, stateChangingRepairAcceptedKinds()...)
	}
	required["acceptedKinds"] = compactUniqueStrings(acceptedKinds)
	if !primaryStateChangeRequired && len(acceptedOperations) > 0 {
		required["acceptedOperations"] = compactUniqueStrings(lowerStringSlice(acceptedOperations))
	} else {
		delete(required, "acceptedOperations")
	}
	required["mustDeclareCoverageTargets"] = true
	required["coverageTargetMatchRule"] = "state-changing evidence must map to one unresolved DoD item text, goalRequirementId, goal item, or deliverable item from coverageTargetSet"
	required["evidenceKindRequirementRule"] = "when a coverage target has profile evidenceRequirements, include at least one accepted state-changing evidence node with the required kind and operation before claiming the target satisfied; acceptedOperations constrains the required evidence node, not necessary supporting capability nodes"
	required["supportingEvidenceNodeRule"] = "supporting nodes from currently allowed runtime capabilities may prepare or verify the required evidence, but they do not satisfy the requirement unless their kind and operation match acceptedKinds and acceptedOperations"
	mergeCurrentRepairTargetEvidenceForProfileStateChange(input, pkg, required)
	if primaryStateChangeRequired {
		if !repairHasImplementationTargetEvidence(input, pkg) {
			return profileStateChangingEvidenceAcquisitionRepairActionPackage(pkg, required)
		}
		required["mustIncludePrimaryStateChangingNode"] = true
		required["primaryAcceptedKinds"] = stateChangingRepairAcceptedKinds()
		required["primaryStateChangingRule"] = "when unresolved coverage targets include general deliverable, goal, or DoD state change beyond profile-specific write evidence, include a primary state-changing artifact before any supplementary evidence-only write such as snapshot/commit"
	}
	pkg["requiredOutput"] = required
	pkg["exitCriteria"] = []string{
		"profile_required_state_changing_evidence_produced_then_verify",
		"failed_assessment_or_blocker_when_required_evidence_cannot_be_produced",
	}
	return pkg
}

func profileStateChangingEvidenceAcquisitionRepairActionPackage(pkg map[string]any, previousRequired map[string]any) map[string]any {
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	required := evidenceAcquisitionRequiredOutput(mapFromValue(previousRequired))
	required["mustDeclareCoverageTargets"] = true
	required["coverageTargetMatchRule"] = "evidence acquisition must stay bounded to one unresolved DoD item text, goalRequirementId, goal item, or deliverable item from coverageTargetSet"
	required["profileEvidenceRequirementDeferred"] = true
	required["profileEvidenceRequirementRule"] = "profile state-changing evidence remains required after concrete implementation target evidence is acquired; do not emit primary state-changing output until a current target snapshot, targetRef, lineRef, or equivalent implementation target evidence is available"
	delete(required, "mustIncludePrimaryStateChangingNode")
	delete(required, "primaryAcceptedKinds")
	delete(required, "primaryStateChangingRule")
	delete(required, "acceptedOperations")
	pkg["requiredOutput"] = required
	pkg["evidenceAcquisitionRequired"] = true
	pkg["concreteFailureEvidenceRequired"] = true
	pkg["repairObjective"] = repairObjectiveAcquireFailureEvidence
	pkg["exitCriteria"] = []string{
		"concrete_implementation_target_evidence_acquired_then_profile_state_change",
		"failed_assessment_or_blocker_when_required_evidence_cannot_be_acquired",
	}
	stripRepairActionPackageImplementationTargetEvidence(pkg)
	return pkg
}

func mergeCurrentRepairTargetEvidenceForProfileStateChange(input map[string]any, pkg map[string]any, required map[string]any) {
	if len(input) == 0 || len(pkg) == 0 || len(required) == 0 {
		return
	}
	if targets := repairActionTargetSnapshots(input); len(targets) > 0 {
		existing := mapSliceFromValue(pkg["targetSnapshots"])
		merged := mergeRepairTargetSnapshots(existing, targets)
		if len(merged) > 0 {
			pkg["targetSnapshots"] = merged
			pkg["currentTargetSnapshots"] = repairActionTargetSnapshotMetadata(merged)
			required["mustTargetFailureScope"] = true
			required["targetRelation"] = "failure_target_repair_snapshot_or_declared_related_failure_target"
			applyCurrentSnapshotDirectPatchContract(required)
			required["fileWriteExistingTargetRule"] = "only allowed when the target snapshot is complete/safe and patch/apply file_write includes baseHash equal to that snapshot contentHash; otherwise use patch/apply text_replace with exact oldText/newText/baseHash, bounded unified_diff, or file/edit"
		}
	}
	if targetSet := repairActionTargetSet(input, pkg); len(targetSet) > 0 {
		pkg["repairTargetSet"] = targetSet
		required["mustTargetRepairTargetSet"] = true
	}
	if reassessment := mapFromValue(input["strategyReassessment"]); len(reassessment) > 0 {
		pkg["strategyReassessment"] = compactLLMContextMap(reassessment, 1)
	}
}

func coverageTargetsRequirePrimaryStateChangingOutput(targets []map[string]any, requirements []profileEvidenceKindRequirement) bool {
	if len(targets) == 0 {
		return false
	}
	filtered := make([]profileEvidenceKindRequirement, 0, len(requirements))
	for _, requirement := range requirements {
		if profileRequirementRequiresStateChangingEvidence(requirement) {
			filtered = append(filtered, requirement)
		}
	}
	if len(filtered) == 0 {
		return false
	}
	for _, target := range targets {
		matched := false
		for _, requirement := range filtered {
			if evidenceKindRequirementMatchesCoverageTarget(requirement, target) {
				matched = true
				break
			}
		}
		if !matched {
			return true
		}
	}
	return false
}

func coverageTargetEvidenceKindRequirements(run *runModel.Run, targets []map[string]any) []profileEvidenceKindRequirement {
	requirements := profileEvidenceKindRequirements(run)
	if len(requirements) == 0 || len(targets) == 0 {
		return nil
	}
	ret := make([]profileEvidenceKindRequirement, 0)
	seen := map[string]bool{}
	for _, requirement := range requirements {
		for _, target := range targets {
			if !evidenceKindRequirementMatchesCoverageTarget(requirement, target) {
				continue
			}
			key := normalizeEvidenceKindRequirementScope(requirement.Scope) + ":" + requirement.ID
			if seen[key] {
				continue
			}
			seen[key] = true
			ret = append(ret, requirement)
			break
		}
	}
	return ret
}

func evidenceKindRequirementMatchesCoverageTarget(requirement profileEvidenceKindRequirement, target map[string]any) bool {
	if len(target) == 0 {
		return false
	}
	scope := normalizeEvidenceKindRequirementScope(requirement.Scope)
	if scope != "" {
		if annotatedScopes := coverageTargetAnnotatedScopes(target); len(annotatedScopes) > 0 {
			if !annotatedScopes[scope] {
				return false
			}
		} else if targetKind := coverageTargetExplicitScope(target); targetKind != "" && scope != targetKind {
			return false
		}
	}
	return evidenceKindRequirementMatchesItem(requirement, target)
}

func coverageTargetExplicitScope(target map[string]any) string {
	if len(target) == 0 {
		return ""
	}
	for _, key := range []string{"kind", "source"} {
		scope := normalizeEvidenceKindRequirementScope(stringFromMap(target, key))
		if knownEvidenceKindRequirementScope(scope) {
			return scope
		}
	}
	return ""
}

func coverageTargetAnnotatedScopes(target map[string]any) map[string]bool {
	if len(target) == 0 {
		return nil
	}
	ret := map[string]bool{}
	for _, key := range []string{
		"id",
		"dimension",
		"key",
		"name",
		"deliverable",
		"goal",
		"goalItem",
		"item",
		"raw",
		"requirement",
		"targetText",
		"coverageTargetToken",
	} {
		if _, scope := coverageTargetScopeAnnotatedText(stringFromMap(target, key)); scope != "" {
			ret[scope] = true
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func profileRequirementRequiresStateChangingEvidence(requirement profileEvidenceKindRequirement) bool {
	hasStateChangingKind := false
	for _, kind := range requirement.AcceptedStepKinds {
		if stepKindCanProduceStateChangingEvidence(kind) {
			hasStateChangingKind = true
			continue
		}
		return false
	}
	return hasStateChangingKind
}

func convergenceCoverageVerificationEvidenceGap(input map[string]any, failedStep *runModel.Step) bool {
	if boolValue(input[stepInputKeyAssessmentOnly]) ||
		strings.TrimSpace(stringFromMap(input, "repairObjective")) == "complete_dod_goal_coverage_assessment" {
		return false
	}
	if !coverageVerificationGapPresent(input, failedStep) {
		return false
	}
	if !coverageValidationEvidenceRepairEligible(failedStep) {
		return false
	}
	if repairHasImplementationTargetEvidence(input, mapFromValue(input["repairActionPackage"])) {
		return false
	}
	if failedStep != nil && repairHasImplementationTargetEvidence(failedStep.Input, mapFromValue(failedStep.Input["repairActionPackage"])) {
		return false
	}
	return true
}

func convergenceNeedsEvidence(input map[string]any, failedStep *runModel.Step) bool {
	if convergenceConcreteValidationCannotAdvanceWithoutTarget(input, failedStep) {
		return true
	}
	if convergenceStateChangeCannotAdvanceWithoutTarget(input, failedStep) {
		return true
	}
	if boolValue(input["concreteFailureEvidenceRequired"]) ||
		strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveAcquireFailureEvidence {
		return true
	}
	return repairActionPackageRequiresEvidenceAcquisition(mapFromValue(input["repairActionPackage"]))
}

func convergenceStateChangeCannotAdvanceWithoutTarget(input map[string]any, failedStep *runModel.Step) bool {
	if !convergenceNeedsStateChange(nil, input, failedStep) {
		return false
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if boolValue(pkg["capabilityPolicyRepairRequired"]) {
		return false
	}
	if !coverageVerificationGapPresent(input, failedStep) {
		return false
	}
	if repairHasImplementationTargetEvidence(input, pkg) {
		return false
	}
	if failedStep != nil && repairHasImplementationTargetEvidence(failedStep.Input, mapFromValue(failedStep.Input["repairActionPackage"])) {
		return false
	}
	return true
}

func convergenceConcreteValidationCannotAdvanceWithoutTarget(input map[string]any, failedStep *runModel.Step) bool {
	if failedStep == nil || !inputRequiresConcreteValidationEvidence(failedStep.Input) {
		return false
	}
	diagnostics := mapFromValue(failedStep.Output["runtimeDiagnostics"])
	if strings.TrimSpace(stringFromMap(diagnostics, "kind")) == "dod_evidence_quality_gap" ||
		boolValue(diagnostics["claimAlignedEvidenceNeeded"]) {
		return false
	}
	if !coverageVerificationGapPresent(input, failedStep) {
		return false
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if repairHasImplementationTargetEvidence(input, pkg) ||
		repairHasImplementationTargetEvidence(failedStep.Input, mapFromValue(failedStep.Input["repairActionPackage"])) {
		return false
	}
	if failedConcreteValidationAssessmentOutput(failedStep.Output) {
		return true
	}
	return false
}

func failedConcreteValidationAssessmentOutput(output map[string]any) bool {
	if len(output) == 0 {
		return false
	}
	if assessment := doDAssessmentFromOutput(output); len(assessment) > 0 {
		if satisfied, ok := assessment["satisfied"].(bool); ok && !satisfied {
			return true
		}
	}
	if len(missingDoDItemsFromFailureOutput(output)) > 0 ||
		len(missingGoalCoverageFromFailureOutput(output)) > 0 ||
		len(missingDeliverableCoverageFromFailureOutput(output)) > 0 {
		return true
	}
	text := strings.ToLower(strings.Join([]string{
		stringFromMap(output, "error"),
		stringFromMap(output, "reason"),
		stringFromMap(output, failureOutputKeySignature),
	}, "\n"))
	return strings.Contains(text, "plain failed dodassessment") ||
		strings.Contains(text, "dodassessment is not satisfied")
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

func convergenceNeedsStateChange(run *runModel.Run, input map[string]any, failedStep *runModel.Step) bool {
	if convertedReadableDecisionRepairRequiresAssessmentOnly(run, input, failedStep) {
		return false
	}
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
