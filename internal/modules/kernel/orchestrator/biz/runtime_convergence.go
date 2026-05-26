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
		if sourceKey == "rejectedResultContinuation" {
			inheritRejectedResultCoverageContract(input, source)
		}
	}
	enrichRepairInputWithProgressFailureSignals(input)
	hydrateConvergenceTargetEvidence(input)
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
	if convergenceTargetEvidenceReadyForStateChange(input, failedStep) {
		contract.State = convergenceStateNeedStateChange
		contract.RuntimeStage = runtimeStageRepair
		contract.RuntimeStageMode = "strict_convergence"
		contract.RepairObjective = repairObjectiveStateChangingOutput
		contract.StateChangingOutputRequired = true
		contract.RepairActionPackage = stateChangingRepairActionPackageForConvergence(input, failedStep)
		contract.TargetEvidence = convergenceTargetEvidence(input, contract.RepairActionPackage)
		return contract
	}
	if convergenceNeedsValidation(input, failedStep) {
		if pkg := stateChangingEvidenceRequirementRepairActionPackage(run, input); len(pkg) > 0 {
			contract.State = convergenceStateNeedStateChange
			contract.RuntimeStage = runtimeStageRepair
			contract.RuntimeStageMode = "strict_convergence"
			contract.RepairObjective = repairObjectiveStateChangingOutput
			contract.StateChangingOutputRequired = true
			contract.RepairActionPackage = pkg
			contract.TargetEvidence = convergenceTargetEvidence(input, pkg)
			return contract
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

func convergenceTargetEvidenceReadyForStateChange(input map[string]any, failedStep *runModel.Step) bool {
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
	if !coverageVerificationGapPresent(input, failedStep) {
		return false
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if !repairHasImplementationTargetEvidence(input, pkg) {
		return false
	}
	return inputRequiresConcreteValidationEvidence(input) ||
		strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveConcreteValidationEvidence ||
		stringFromMap(mapFromValue(input["convergenceContract"]), "state") == convergenceStateNeedValidation
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
	for key, value := range mapFromValue(pkg["requiredOutput"]) {
		switch key {
		case "mustReturnValidJSON", "mustNotReuseRejectedJSON", "forbiddenStalePatchReuse", "mustDeclareStrategyReassessmentFields", "strategyReassessmentRequiredFields", "strategyReassessmentMatchRule", "mustReassessRepeatedFailedSurface":
			required[key] = value
		}
	}
	if len(mapSliceFromValue(pkg["targetSnapshots"])) > 0 || len(mapSliceFromValue(pkg["currentTargetSnapshots"])) > 0 {
		required["mustTargetFailureScope"] = true
		required["targetRelation"] = "failure_target_repair_snapshot_or_declared_related_failure_target"
		required["mustUseCurrentTargetSnapshot"] = true
		required["forbiddenStalePatchReuse"] = true
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
		input["repairActionPackage"] = pkg
	case convergenceStateNeedAssessment:
		setRuntimeStage(input, runtimeStageVerification)
		input[stepInputKeySoulStage] = soulStageValidation
		input[stepInputKeyAssessmentOnly] = true
		delete(input, "stateChangingOutputRequired")
		delete(input, "concreteFailureEvidenceRequired")
	case convergenceStateNeedGovernance:
		setRuntimeStage(input, runtimeStageDecision)
		input[stepInputKeySoulStage] = soulStageDecision
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
	return contract.StateChangingOutputRequired && len(contract.TargetEvidence) > 0
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
	return stringSliceContains(accepted, "file/list") ||
		stringSliceContains(accepted, "file/read") ||
		stringSliceContains(accepted, "shell/command") ||
		stringSliceContains(accepted, "mcp/tool_call")
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
	for _, requirement := range requirements {
		if len(requirement.AcceptedStepKinds) == 0 {
			continue
		}
		acceptedKinds = append(acceptedKinds, requirement.AcceptedStepKinds...)
		acceptedOperations = append(acceptedOperations, requirement.AcceptedOperations...)
		if profileRequirementAcceptsStateChangingEvidence(requirement) {
			requiresStateChange = true
		}
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
	required["acceptedKinds"] = compactUniqueStrings(acceptedKinds)
	if len(acceptedOperations) > 0 {
		required["acceptedOperations"] = compactUniqueStrings(lowerStringSlice(acceptedOperations))
	}
	required["mustDeclareCoverageTargets"] = true
	required["coverageTargetMatchRule"] = "state-changing evidence must map to one unresolved DoD item text, goalRequirementId, goal item, or deliverable item from coverageTargetSet"
	required["evidenceKindRequirementRule"] = "when a coverage target has profile evidenceRequirements, include at least one accepted state-changing evidence node with the required kind and operation before claiming the target satisfied"
	pkg["requiredOutput"] = required
	pkg["exitCriteria"] = []string{
		"profile_required_state_changing_evidence_produced_then_verify",
		"failed_assessment_or_blocker_when_required_evidence_cannot_be_produced",
	}
	return pkg
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
	targetKind := normalizeEvidenceKindRequirementScope(firstNonEmptyString(
		stringFromMap(target, "kind"),
		stringFromMap(target, "source"),
	))
	if scope != "" && targetKind != "" && scope != targetKind {
		return false
	}
	return evidenceKindRequirementMatchesItem(requirement, target)
}

func profileRequirementAcceptsStateChangingEvidence(requirement profileEvidenceKindRequirement) bool {
	for _, kind := range requirement.AcceptedStepKinds {
		if stepKindCanProduceStateChangingEvidence(kind) {
			return true
		}
	}
	return false
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
	if !convergenceNeedsStateChange(input, failedStep) {
		return false
	}
	if !coverageVerificationGapPresent(input, failedStep) {
		return false
	}
	pkg := mapFromValue(input["repairActionPackage"])
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
