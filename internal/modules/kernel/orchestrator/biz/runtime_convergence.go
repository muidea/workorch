package orchestrator

import (
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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
	Prerequisite                    map[string]any
	RepairActionPackage             map[string]any
	GovernanceReason                string
	SourceRefs                      []map[string]any
	TargetEvidence                  map[string]any
	HistoricalSignals               map[string]any
	DeterministicComplete           bool
	CompletionReason                string
}

type convergenceSubsystem struct{}

func newConvergenceSubsystem() *convergenceSubsystem {
	return &convergenceSubsystem{}
}

func (c *convergenceSubsystem) build(
	run *runModel.Run,
	currentStep *runModel.Step,
	failedStep *runModel.Step,
	lineage map[string]any,
	evidence map[string]any,
) ConvergenceContract {
	return BuildConvergenceContract(run, currentStep, failedStep, lineage, evidence)
}

func (c *convergenceSubsystem) apply(input map[string]any, contract ConvergenceContract) map[string]any {
	return ApplyConvergenceContract(input, contract)
}

func (c *convergenceSubsystem) shouldCreateDecisionTicket(input map[string]any, llmOutput map[string]any, contract ConvergenceContract) bool {
	return ShouldCreateDecisionTicket(input, llmOutput, contract)
}

func (c *convergenceSubsystem) compact(contract ConvergenceContract) map[string]any {
	return compactConvergenceContract(contract)
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
	contract := ConvergenceContract{
		State:            convergenceStateBlocked,
		RuntimeStage:     runtimeStageRepair,
		RuntimeStageMode: "strict_convergence",
		SourceRefs:       convergenceSourceRefs(currentStep, failedStep, lineage, evidence),
	}
	if populateConvergenceContractFromNextAction(run, input, failedStep, &contract) {
		return contract
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
		if len(mapFromValue(input["repairActionPackage"])) == 0 && !failedStepRequiresAuthorityRecompute(failedStep) {
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
	if governanceReason := convergenceGovernanceReason(input, failedStep); governanceReason != "" {
		contract.State = convergenceStateNeedGovernance
		contract.RuntimeStage = runtimeStageDecision
		contract.RuntimeStageMode = "structured_governance"
		contract.GovernanceReason = governanceReason
		return contract
	}
	if recoveryContract, ok := contractViolationRecoveryStateChangingContract(input, failedStep, contract.SourceRefs); ok {
		return recoveryContract
	}
	if validationRequiredOutputCapabilityMismatch(input, mapFromValue(input["repairActionPackage"]), failedStep) {
		populateCoverageWorkItemValidationConvergenceContract(input, &contract, false)
		return contract
	}
	if convergenceCapabilityInputLocatorNeedsEvidence(input, failedStep) {
		contract.State = convergenceStateNeedEvidence
		contract.RuntimeStage = runtimeStageRepair
		contract.RuntimeStageMode = "strict_convergence"
		contract.RepairObjective = repairObjectiveAcquireFailureEvidence
		contract.ConcreteFailureEvidenceRequired = true
		contract.RepairActionPackage = capabilityInputLocatorEvidencePackage(input, failedStep)
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
	if boolValue(input["emptyWorkspaceEvidenceEscalatedToImplementation"]) {
		contract.State = convergenceStateNeedStateChange
		contract.RuntimeStage = runtimeStageRepair
		contract.RuntimeStageMode = "strict_convergence"
		contract.RepairObjective = repairObjectiveStateChangingOutput
		contract.StateChangingOutputRequired = true
		contract.RepairActionPackage = convergenceStateChangingRepairActionPackage(input, failedStep)
		contract.TargetEvidence = convergenceTargetEvidence(input, contract.RepairActionPackage)
		return contract
	}
	if boolValue(input["validationRepairEscalatedToImplementation"]) {
		populateValidationRepairEscalationConvergenceContract(run, input, failedStep, &contract)
		return contract
	}
	if populateCoverageWorkItemConvergenceContract(run, input, failedStep, &contract) {
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
			contract.RepairActionPackage = convergenceStateChangingRepairActionPackage(input, failedStep)
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
		if convergenceCurrentFocusCanCloseDeterministically(input) {
			populateCompleteConvergenceContract(&contract, "current coverage focus is deterministically satisfied")
			return contract
		}
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
			contract.RepairActionPackage = map[string]any{"requiredOutput": stateChangingRepairRequiredOutput()}
			contract.RepairActionPackage["requiredOutput"] = evidenceAcquisitionRequiredOutputWithScratch(input, contract.RepairActionPackage, mapFromValue(contract.RepairActionPackage["requiredOutput"]))
		}
		return contract
	}
	if convergenceNeedsStateChange(run, input, failedStep) {
		contract.State = convergenceStateNeedStateChange
		contract.RuntimeStage = runtimeStageRepair
		contract.RuntimeStageMode = "strict_convergence"
		contract.RepairObjective = repairObjectiveStateChangingOutput
		contract.StateChangingOutputRequired = true
		contract.RepairActionPackage = convergenceStateChangingRepairActionPackage(input, failedStep)
		contract.TargetEvidence = convergenceTargetEvidence(input, contract.RepairActionPackage)
		return contract
	}
	if convergenceNeedsEvidenceFromRejectedContinuation(run) {
		contract.State = convergenceStateNeedEvidence
		contract.RuntimeStage = runtimeStageRepair
		contract.RuntimeStageMode = "strict_convergence"
		contract.RepairObjective = repairObjectiveAcquireFailureEvidence
		contract.ConcreteFailureEvidenceRequired = true
		contract.RepairActionPackage = map[string]any{"requiredOutput": stateChangingRepairRequiredOutput()}
		contract.RepairActionPackage["requiredOutput"] = evidenceAcquisitionRequiredOutputWithScratch(input, contract.RepairActionPackage, mapFromValue(contract.RepairActionPackage["requiredOutput"]))
		return contract
	}
	return contract
}

func populateConvergenceContractFromNextAction(run *runModel.Run, input map[string]any, failedStep *runModel.Step, contract *ConvergenceContract) bool {
	if contract == nil {
		return false
	}
	if !convergenceNextActionAuthorityAllowed(input) {
		return false
	}
	if validationCommandContractFailure(input, mapFromValue(input["repairActionPackage"]), failedStep) {
		if contract.RepairActionPackage == nil {
			contract.RepairActionPackage = cloneStepPayload(mapFromValue(input["repairActionPackage"]))
		}
		convergenceContractAttachCoverageContext(input, contract)
		populateCoverageWorkItemValidationConvergenceContract(input, contract, false)
		return true
	}
	if validationRequiredOutputCapabilityMismatch(input, mapFromValue(input["repairActionPackage"]), failedStep) {
		if contract.RepairActionPackage == nil {
			contract.RepairActionPackage = cloneStepPayload(mapFromValue(input["repairActionPackage"]))
		}
		convergenceContractAttachCoverageContext(input, contract)
		populateCoverageWorkItemValidationConvergenceContract(input, contract, false)
		return true
	}
	if convergenceValidationRepairEscalationBlocksStaleNextAction(input, run) {
		return false
	}
	if contractViolationRecoveryRequiresAuthorityRecompute(input) {
		recomputed := ConvergenceContract{
			State:            convergenceStateBlocked,
			RuntimeStage:     runtimeStageRepair,
			RuntimeStageMode: "strict_convergence",
			SourceRefs:       contract.SourceRefs,
		}
		if populateCoverageWorkItemConvergenceContract(run, input, failedStep, &recomputed) &&
			recomputed.State != convergenceStateNeedAssessment {
			*contract = recomputed
			return true
		}
		switch convergenceNextActionKind(run, input) {
		case "assessment", "complete_coverage_assessment":
			populateAuthorityRecomputeBlockedConvergenceContract(contract, recomputed.State)
			return true
		case "":
			return false
		}
	}
	switch convergenceNextActionKind(run, input) {
	case "request_decision", "wait_decision", "request_governance":
		contract.State = convergenceStateNeedGovernance
		contract.RuntimeStage = runtimeStageDecision
		contract.RuntimeStageMode = "structured_governance"
		contract.GovernanceReason = "current next action requires decision"
		return true
	case "request_approval", "wait_approval":
		contract.State = convergenceStateNeedGovernance
		contract.RuntimeStage = runtimeStageDecision
		contract.RuntimeStageMode = "structured_governance"
		contract.GovernanceReason = "current next action requires approval"
		return true
	case "assessment", "complete_coverage_assessment":
		if convergenceCurrentFocusCanCloseDeterministically(input) {
			populateCompleteConvergenceContract(contract, "current coverage focus is deterministically satisfied")
			return true
		}
		convergenceContractAttachCoverageContext(input, contract)
		populateCoverageWorkItemAssessmentConvergenceContract(contract)
		return true
	case "complete":
		populateCompleteConvergenceContract(contract, "current next action is complete")
		return true
	case "state_change", "produce_state_change":
		if contract.RepairActionPackage == nil {
			contract.RepairActionPackage = cloneStepPayload(mapFromValue(input["repairActionPackage"]))
		}
		convergenceContractAttachCoverageContext(input, contract)
		if validationContractFailureMustRemainValidation(input, contract.RepairActionPackage, failedStep) {
			populateCoverageWorkItemValidationConvergenceContract(input, contract, false)
			return true
		}
		if pkg := stateChangingEvidenceRequirementRepairActionPackage(run, input); len(pkg) > 0 {
			populateProfileStateChangingEvidenceConvergenceContract(input, contract, pkg)
			return true
		}
		if stateChangeRequiresTargetSnapshotAcquisition(input, contract.RepairActionPackage) {
			populateTargetSnapshotAcquisitionConvergenceContract(input, contract)
			return true
		}
		populateCoverageWorkItemStateChangeConvergenceContract(input, contract)
		return true
	case "acquire_target_snapshot":
		if contract.RepairActionPackage == nil {
			contract.RepairActionPackage = cloneStepPayload(mapFromValue(input["repairActionPackage"]))
		}
		convergenceContractAttachCoverageContext(input, contract)
		populateTargetSnapshotAcquisitionConvergenceContract(input, contract)
		return true
	case "validate", "validation", "produce_validation_evidence", "artifact", "produce_artifact_evidence":
		convergenceContractAttachCoverageContext(input, contract)
		populateCoverageWorkItemValidationConvergenceContract(input, contract, false)
		return true
	case "repair", "acquire_evidence":
		convergenceContractAttachCoverageContext(input, contract)
		if convergenceCurrentFocusNeedsStateChange(input) {
			if validationContractFailureMustRemainValidation(input, contract.RepairActionPackage, failedStep) {
				populateCoverageWorkItemValidationConvergenceContract(input, contract, false)
				return true
			}
			if pkg := stateChangingEvidenceRequirementRepairActionPackage(run, input); len(pkg) > 0 {
				populateProfileStateChangingEvidenceConvergenceContract(input, contract, pkg)
				return true
			}
			if stateChangeRequiresTargetSnapshotAcquisition(input, contract.RepairActionPackage) {
				populateTargetSnapshotAcquisitionConvergenceContract(input, contract)
				return true
			}
			populateCoverageWorkItemStateChangeConvergenceContract(input, contract)
			return true
		}
		if convergenceCurrentFocusNeedsValidationWithoutStateChange(input) {
			populateCoverageWorkItemValidationConvergenceContract(input, contract, false)
			return true
		}
		populateCoverageWorkItemEvidenceConvergenceContract(input, contract)
		return true
	default:
		if kind := convergenceNextActionKind(run, input); kind != "" {
			contract.State = convergenceStateNeedGovernance
			contract.RuntimeStage = runtimeStageDecision
			contract.RuntimeStageMode = "contract_violation"
			contract.GovernanceReason = "unsupported next action kind: " + kind
			if contract.RepairActionPackage == nil {
				contract.RepairActionPackage = map[string]any{}
			}
			contract.RepairActionPackage["unsupportedNextActionKind"] = kind
			contract.RepairActionPackage["requiredOutput"] = map[string]any{
				"mustResolveUnsupportedNextActionKind": true,
				"unsupportedNextActionKind":            kind,
			}
			return true
		}
		return false
	}
}

func populateAuthorityRecomputeBlockedConvergenceContract(contract *ConvergenceContract, recomputedState string) {
	if contract == nil {
		return
	}
	contract.State = convergenceStateNeedGovernance
	contract.RuntimeStage = runtimeStageDecision
	contract.RuntimeStageMode = "contract_violation"
	contract.GovernanceReason = "runtime authority recompute did not produce an executable convergence action"
	if contract.RepairActionPackage == nil {
		contract.RepairActionPackage = map[string]any{}
	}
	contract.RepairActionPackage["authorityRecomputeBlocked"] = true
	if strings.TrimSpace(recomputedState) != "" {
		contract.RepairActionPackage["recomputedState"] = strings.TrimSpace(recomputedState)
	}
	contract.RepairActionPackage["requiredOutput"] = map[string]any{
		"mustResolveRuntimeAuthorityRecompute": true,
		"recomputedState":                      strings.TrimSpace(recomputedState),
	}
}

func contractViolationRecoveryRequiresAuthorityRecompute(input map[string]any) bool {
	recovery := mapFromValue(input[stepInputKeyContractViolationRecovery])
	if len(recovery) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromMap(recovery, "violatedRule")) == contractViolationRuleCoverageAssessmentIncomplete {
		return true
	}
	if strings.TrimSpace(stringFromMap(recovery, "recoverability")) == "recompute_authority" {
		return true
	}
	correction := mapFromValue(recovery["correctionContract"])
	return len(mapFromValue(correction["runtimeRecomputedAuthority"])) > 0
}

func convergenceNextActionAuthorityAllowed(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if kind := convergenceNextActionKind(nil, input); kind != "" {
		return true
	}
	if boolValue(input["replanBudgetResetPending"]) ||
		boolValue(input[stepInputKeyRuntimeRepairFollowUp]) ||
		boolValue(input[stepInputKeyProviderRetryFollowUp]) {
		return true
	}
	if len(mapFromValue(input["replanContinuation"])) > 0 {
		return true
	}
	if executionContext := mapFromValue(input["executionContext"]); len(executionContext) > 0 {
		if boolValue(executionContext["replanBudgetResetPending"]) ||
			boolValue(executionContext[stepInputKeyRuntimeRepairFollowUp]) ||
			boolValue(executionContext[stepInputKeyProviderRetryFollowUp]) ||
			len(mapFromValue(executionContext["replanContinuation"])) > 0 {
			return true
		}
	}
	return false
}

func convergenceValidationRepairEscalationBlocksStaleNextAction(input map[string]any, run *runModel.Run) bool {
	if !boolValue(input["validationRepairEscalatedToImplementation"]) {
		return false
	}
	switch convergenceNextActionKind(run, input) {
	case "validate", "validation", "produce_validation_evidence", "artifact", "produce_artifact_evidence",
		"assessment", "complete_coverage_assessment":
		return true
	default:
		return false
	}
}

func populateValidationRepairEscalationConvergenceContract(run *runModel.Run, input map[string]any, failedStep *runModel.Step, contract *ConvergenceContract) {
	if contract == nil {
		return
	}
	pkg := repeatedValidationRepairEscalationPackage(run, input, failedStep)
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
	if len(pkg) == 0 {
		pkg = convergenceStateChangingRepairActionPackage(input, failedStep)
	} else if !boolValue(mapFromValue(pkg["requiredOutput"])["mustIncludeStateChangingNode"]) {
		required := enforceStateChangingRequiredOutputForConvergence(pkg, input)
		addStrategyReassessmentRequiredOutputContract(pkg, required)
	}
	contract.RepairActionPackage = pkg
	contract.TargetEvidence = convergenceTargetEvidence(input, pkg)
}

func convergenceInputHasActionableReplanContinuation(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if len(mapFromValue(input["replanContinuation"])) > 0 {
		return true
	}
	if replanContinuationCarriesActionableFailure(input) {
		return true
	}
	if executionContext := mapFromValue(input["executionContext"]); len(executionContext) > 0 {
		if len(mapFromValue(executionContext["replanContinuation"])) > 0 {
			return true
		}
		return replanContinuationCarriesActionableFailure(executionContext)
	}
	return false
}

func convergenceNextActionKind(run *runModel.Run, input map[string]any) string {
	for _, source := range []map[string]any{
		convergenceAuthorityMapFromValue(input[convergenceAuthorityKey]),
		convergenceAuthorityMapFromValue(mapFromValue(input["executionContext"])[convergenceAuthorityKey]),
		gapBaselineMapFromValue(input[gapBaselineKey]),
		gapBaselineMapFromValue(mapFromValue(input["executionContext"])[gapBaselineKey]),
	} {
		if kind := strings.TrimSpace(stringFromMap(mapFromValue(source["nextAction"]), "kind")); kind != "" {
			return kind
		}
	}
	if run != nil {
		if kind := strings.TrimSpace(stringFromMap(mapFromValue(convergenceAuthorityMapFromValue(run.ExecutionContext[convergenceAuthorityKey])["nextAction"]), "kind")); kind != "" {
			return kind
		}
		if kind := strings.TrimSpace(stringFromMap(mapFromValue(gapBaselineMapFromValue(run.ExecutionContext[gapBaselineKey])["nextAction"]), "kind")); kind != "" {
			return kind
		}
	}
	return ""
}

func convergenceContractAttachCoverageContext(input map[string]any, contract *ConvergenceContract) {
	if contract == nil {
		return
	}
	context := coverageWorkItemContextFromInput(input)
	if len(context) == 0 {
		return
	}
	if contract.RepairActionPackage == nil {
		contract.RepairActionPackage = map[string]any{}
	}
	contract.RepairActionPackage[coverageWorkItemContextKey] = context
	if len(mapSliceFromValue(contract.RepairActionPackage["coverageTargetSet"])) == 0 {
		if targets := coverageTargetSetFromWorkItemContext(context); len(targets) > 0 {
			contract.RepairActionPackage["coverageRepairRequired"] = true
			contract.RepairActionPackage["coverageTargetSet"] = targets
		}
	}
}

func contractViolationRecoveryStateChangingContract(input map[string]any, failedStep *runModel.Step, sourceRefs []map[string]any) (ConvergenceContract, bool) {
	if len(input) == 0 || !boolValue(input["contractViolationRetry"]) {
		return ConvergenceContract{}, false
	}
	recovery := mapFromValue(input[stepInputKeyContractViolationRecovery])
	if len(recovery) == 0 {
		return ConvergenceContract{}, false
	}
	if expected := mapFromValue(recovery["expectedContract"]); len(expected) > 0 {
		appendAuthorityDiagnostic(input, "contract_violation_expected_contract_diagnostic_only", "info", "diagnostic_lineage", "contractViolationRecovery.expectedContract", "contract violation expectedContract is diagnostic-only and cannot drive convergence contract authority")
	}
	correction := mapFromValue(recovery["correctionContract"])
	switch strings.TrimSpace(stringFromMap(correction, "requiredOutputShape")) {
	case "valid_patch_apply", "state_changing_output", "state_changing_output_from_available_snapshot":
	default:
		return ConvergenceContract{}, false
	}
	if convergenceCoverageFocusRequiresValidation(input) &&
		validationContractFailureMustRemainValidation(input, mapFromValue(input["repairActionPackage"]), failedStep) {
		return ConvergenceContract{}, false
	}
	if contractViolationRecoveryCurrentContractPreemptsExpectedStateChange(input, recovery) {
		return ConvergenceContract{}, false
	}
	pkg := cloneStepPayload(mapFromValue(input["repairActionPackage"]))
	if len(pkg) == 0 && failedStep != nil && !failedStepRequiresAuthorityRecompute(failedStep) {
		pkg = cloneStepPayload(mapFromValue(failedStep.Input["repairActionPackage"]))
	}
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) {
		required = enforceStateChangingRequiredOutputForConvergence(pkg, input)
		addStrategyReassessmentRequiredOutputContract(pkg, required)
	}
	pkg["requiredOutput"] = required
	return ConvergenceContract{
		State:                       convergenceStateNeedStateChange,
		RuntimeStage:                runtimeStageRepair,
		RuntimeStageMode:            "strict_convergence",
		RepairObjective:             repairObjectiveStateChangingOutput,
		StateChangingOutputRequired: true,
		RepairActionPackage:         pkg,
		TargetEvidence:              convergenceTargetEvidence(input, pkg),
		SourceRefs:                  sourceRefs,
	}, true
}

func contractViolationRecoveryCurrentContractPreemptsExpectedStateChange(input map[string]any, recovery map[string]any) bool {
	correction := mapFromValue(recovery["correctionContract"])
	switch strings.TrimSpace(stringFromMap(correction, "requiredOutputShape")) {
	case "bounded_validation_evidence",
		"bounded_failure_evidence",
		"traceable_coverage_assessment",
		"single_valid_json_decision":
		return true
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if len(required) == 0 {
		required = mapFromValue(input["requiredOutput"])
	}
	if boolValue(required["mustIncludeStateChangingNode"]) {
		return false
	}
	return boolValue(required["mustProduceConcreteValidationEvidence"]) ||
		boolValue(required["mustProduceConcreteFailureEvidence"])
}

func convergenceStateChangingRepairActionPackage(input map[string]any, failedStep *runModel.Step) map[string]any {
	pkg := cloneStepPayload(mapFromValue(input["repairActionPackage"]))
	if len(pkg) == 0 {
		return stateChangingRepairActionPackageForConvergence(input, failedStep)
	}
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) {
		required = enforceStateChangingRequiredOutputForConvergence(pkg, input)
		addStrategyReassessmentRequiredOutputContract(pkg, required)
	}
	return pkg
}

func populateProfileStateChangingEvidenceConvergenceContract(input map[string]any, contract *ConvergenceContract, pkg map[string]any) {
	if contract == nil {
		return
	}
	if gap := mapFromValue(pkg["runtimeCapabilityGap"]); len(gap) > 0 {
		contract.State = convergenceStateNeedGovernance
		contract.RuntimeStage = runtimeStageDecision
		contract.RuntimeStageMode = "structured_governance"
		contract.GovernanceReason = firstNonEmptyString(
			stringFromMap(gap, "reason"),
			"profile evidence requirement cannot be satisfied by currently declared runtime capabilities",
		)
		contract.RepairActionPackage = pkg
		contract.TargetEvidence = convergenceTargetEvidence(input, pkg)
		return
	}
	if convergenceInputAuthorityRequiresStateChange(input) && repairActionPackageRequiresEvidenceAcquisition(pkg) {
		required := enforceStateChangingRequiredOutputForConvergence(pkg, input)
		addStrategyReassessmentRequiredOutputContract(pkg, required)
		pkg["requiredOutput"] = required
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

func convergenceInputAuthorityRequiresStateChange(input map[string]any) bool {
	switch strings.TrimSpace(convergenceNextActionKind(nil, input)) {
	case "state_change", "produce_state_change":
		return true
	default:
		return false
	}
}

func populateCoverageWorkItemConvergenceContract(run *runModel.Run, input map[string]any, failedStep *runModel.Step, contract *ConvergenceContract) bool {
	if contract == nil {
		return false
	}
	recomputeAuthority := contractViolationRecoveryRequiresAuthorityRecompute(input)
	if !recomputeAuthority && (boolValue(input["assessmentOnlyContractCorrection"]) ||
		(strings.TrimSpace(stringFromMap(input, "repairObjective")) == "complete_dod_goal_coverage_assessment" &&
			!coverageWorkItemContextHasActionableFailure(coverageWorkItemContextFromInput(input)) &&
			!explicitCoverageGapShouldPreemptAssessment(input, failedStep))) {
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
	if coverageWorkItemCanCloseDeterministically(focus) {
		populateCompleteConvergenceContract(contract, "current coverage focus is deterministically satisfied")
		return true
	}
	missing := coverageWorkItemMissingEvidenceKindSet(focus)
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
	if validationContractFailureMustRemainValidation(input, contract.RepairActionPackage, failedStep) {
		populateCoverageWorkItemValidationConvergenceContract(input, contract, false)
		return true
	}
	if pkg := stateChangingEvidenceRequirementRepairActionPackage(run, input); len(pkg) > 0 {
		populateProfileStateChangingEvidenceConvergenceContract(input, contract, pkg)
		contract.RepairActionPackage[coverageWorkItemContextKey] = context
		return true
	}
	if coverageWorkItemFocusReadyForAssessment(focus) &&
		(!coverageFailureStepHasExplicitCoverageGap(failedStep) ||
			!explicitCoverageGapShouldPreemptAssessment(input, failedStep)) {
		populateCoverageWorkItemAssessmentConvergenceContract(contract)
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
		if convergenceStateChangeAlreadySatisfied(input) &&
			!convergenceConcreteFailureTargetsOutsideSatisfiedStateChange(input, contract.RepairActionPackage) {
			populateCoverageWorkItemValidationConvergenceContract(input, contract, false)
			return true
		}
		switch coverageWorkItemStateChangeAuthorityNextActionKind(run, input, focus, recomputeAuthority) {
		case "state_change", "produce_state_change":
			if stateChangeRequiresTargetSnapshotAcquisition(input, contract.RepairActionPackage) {
				populateTargetSnapshotAcquisitionConvergenceContract(input, contract)
				return true
			}
			populateCoverageWorkItemStateChangeConvergenceContract(input, contract)
			return true
		case "acquire_target_snapshot":
			populateTargetSnapshotAcquisitionConvergenceContract(input, contract)
			return true
		case "validate", "validation", "produce_validation_evidence", "artifact", "produce_artifact_evidence":
			populateCoverageWorkItemValidationConvergenceContract(input, contract, false)
			return true
		case "repair", "acquire_evidence":
			if (missing["validation"] || missing["artifact"]) && rejectedContinuationSchemaInvalidValidationFirst(run, input, failedStep) {
				populateCoverageWorkItemValidationConvergenceContract(input, contract, false)
				return true
			}
			populateCoverageWorkItemEvidenceConvergenceContract(input, contract)
			return true
		}
		if stateChangingRepairHasActionableImplementationSurface(input, contract.RepairActionPackage) ||
			len(mapSliceFromValue(contract.RepairActionPackage["targetSnapshots"])) > 0 ||
			len(filterAdmissibleRuntimeRepairTargetRefs(stringSliceFromValue(contract.RepairActionPackage["repairTargetSet"]))) > 0 {
			populateCoverageWorkItemStateChangeConvergenceContract(input, contract)
			return true
		}
		if (missing["validation"] || missing["artifact"]) && rejectedContinuationSchemaInvalidValidationFirst(run, input, failedStep) {
			populateCoverageWorkItemValidationConvergenceContract(input, contract, false)
			return true
		}
		populateCoverageWorkItemEvidenceConvergenceContract(input, contract)
		return true
	}
	if missing["validation"] || missing["artifact"] {
		if convergenceInputRequiresPendingArtifactStateChangingOutput(input, contract.RepairActionPackage) {
			populateCoverageWorkItemStateChangeConvergenceContract(input, contract)
			return true
		}
		if convergenceStateChangeAlreadySatisfied(input) &&
			!convergenceConcreteFailureTargetsOutsideSatisfiedStateChange(input, contract.RepairActionPackage) {
			populateCoverageWorkItemValidationConvergenceContract(input, contract, false)
			return true
		}
		if concreteValidationFailureCanDriveStateChange(input, contract.RepairActionPackage) {
			populateCoverageWorkItemStateChangeConvergenceContract(input, contract)
			return true
		}
		populateCoverageWorkItemValidationConvergenceContract(input, contract, false)
		return true
	}
	if convergenceConcreteValidationCannotAdvanceWithoutTarget(input, failedStep) {
		populateCoverageWorkItemEvidenceConvergenceContract(input, contract)
		return true
	}
	if explicitCoverageGapShouldPreemptAssessment(input, failedStep) {
		if pkg := stateChangingEvidenceRequirementRepairActionPackage(run, input); len(pkg) > 0 {
			populateProfileStateChangingEvidenceConvergenceContract(input, contract, pkg)
			return true
		}
		populateCoverageWorkItemValidationConvergenceContract(input, contract, true)
		return true
	}
	if coverageWorkItemFocusHasActionableFailure(focus) {
		populateCoverageWorkItemValidationConvergenceContract(input, contract, false)
		return true
	}
	if coverageWorkItemCanCloseDeterministically(focus) {
		populateCompleteConvergenceContract(contract, "current coverage focus is deterministically satisfied")
		return true
	}
	populateCoverageWorkItemAssessmentConvergenceContract(contract)
	return true
}

func coverageWorkItemStateChangeAuthorityNextActionKind(run *runModel.Run, input map[string]any, focus map[string]any, recomputeAuthority bool) string {
	kind := strings.TrimSpace(convergenceNextActionKind(run, input))
	missing := coverageWorkItemMissingEvidenceKindSet(focus)
	if missing["state_change"] && (!missing["validation"] && !missing["artifact"] || coverageWorkItemStateChangeCanPreemptValidation(input)) {
		switch kind {
		case "", "repair", "acquire_evidence", "validate", "validation", "produce_validation_evidence", "artifact", "produce_artifact_evidence", "assessment", "complete_coverage_assessment":
			if derived := strings.TrimSpace(stringFromMap(gapNextActionForCoverageItem(focus), "kind")); derived != "" {
				return derived
			}
			return "state_change"
		}
	}
	if recomputeAuthority {
		switch kind {
		case "state_change", "produce_state_change", "acquire_target_snapshot":
			return kind
		}
		if derived := strings.TrimSpace(stringFromMap(gapNextActionForCoverageItem(focus), "kind")); derived != "" {
			return derived
		}
	}
	return kind
}

func coverageWorkItemStateChangeCanPreemptValidation(input map[string]any) bool {
	pkg := mapFromValue(input["repairActionPackage"])
	if stateChangingRepairHasActionableImplementationSurface(input, pkg) ||
		len(mapSliceFromValue(pkg["targetSnapshots"])) > 0 ||
		len(filterAdmissibleRuntimeRepairTargetRefs(convergenceConcreteFailureTargetRefs(input, pkg))) > 0 {
		return true
	}
	return inputHasConcreteValidationFailureDiagnostic(input, pkg) &&
		len(filterAdmissibleRuntimeRepairTargetRefs(convergenceConcreteFailureTargetRefs(input, pkg))) > 0
}

func populateCoverageWorkItemAssessmentConvergenceContract(contract *ConvergenceContract) {
	if contract == nil {
		return
	}
	contract.State = convergenceStateNeedAssessment
	contract.RuntimeStage = runtimeStageVerification
	contract.RuntimeStageMode = "strict_convergence"
	contract.RepairObjective = "complete_dod_goal_coverage_assessment"
}

func populateCoverageWorkItemEvidenceConvergenceContract(input map[string]any, contract *ConvergenceContract) {
	if contract == nil {
		return
	}
	contract.State = convergenceStateNeedEvidence
	contract.RuntimeStage = runtimeStageRepair
	contract.RuntimeStageMode = "strict_convergence"
	contract.RepairObjective = repairObjectiveAcquireFailureEvidence
	contract.ConcreteFailureEvidenceRequired = true
	if contract.RepairActionPackage == nil {
		contract.RepairActionPackage = map[string]any{}
	}
	required := evidenceAcquisitionRequiredOutputWithScratch(input, contract.RepairActionPackage, mapFromValue(contract.RepairActionPackage["requiredOutput"]))
	required["evidenceAcquisitionSubtype"] = "diagnostic_probe"
	required["mustReferenceCoverageWorkItem"] = true
	contract.RepairActionPackage["requiredOutput"] = required
}

func populateCompleteConvergenceContract(contract *ConvergenceContract, reason string) {
	if contract == nil {
		return
	}
	contract.State = ""
	contract.RuntimeStage = runtimeStageVerification
	contract.RuntimeStageMode = "deterministic_ledger"
	contract.DeterministicComplete = true
	contract.CompletionReason = strings.TrimSpace(reason)
	if contract.RepairActionPackage == nil {
		contract.RepairActionPackage = map[string]any{}
	}
	contract.RepairActionPackage["completionReason"] = contract.CompletionReason
}

func populateTargetSnapshotAcquisitionConvergenceContract(input map[string]any, contract *ConvergenceContract) {
	if contract == nil {
		return
	}
	contract.State = convergenceStateNeedEvidence
	contract.RuntimeStage = runtimeStageRepair
	contract.RuntimeStageMode = "target_snapshot_acquisition"
	contract.RepairObjective = repairObjectiveAcquireFailureEvidence
	contract.StateChangingOutputRequired = false
	contract.ConcreteFailureEvidenceRequired = true
	contract.Prerequisite = targetSnapshotAcquisitionPrerequisite()
	if contract.RepairActionPackage == nil {
		contract.RepairActionPackage = map[string]any{}
	}
	pkg := contract.RepairActionPackage
	pkg["parentNextAction"] = map[string]any{"kind": "state_change", "reason": "state_change requires current target snapshot"}
	pkg["prerequisite"] = cloneStepPayload(contract.Prerequisite)
	pkg["requiresCurrentTargetSnapshotForStateChange"] = true
	pkg["targetSnapshotAcquisitionRequired"] = true
	if targetSet := repairActionTargetSet(input, pkg); len(targetSet) > 0 {
		pkg["repairTargetSet"] = targetSet
	}
	required := evidenceAcquisitionRequiredOutputWithScratch(input, pkg, mapFromValue(pkg["requiredOutput"]))
	required["evidenceAcquisitionSubtype"] = "target_snapshot"
	required["parentNextActionKind"] = "state_change"
	required["prerequisiteForNextAction"] = "state_change"
	required["finiteExitRule"] = "accepted_target_snapshot_evidence_or_scoped_blocker_or_internal_fail_fast"
	applyTargetSnapshotAcquisitionRequiredOutputContract(required)
	required["forbiddenKinds"] = appendUniqueStrings(stringSliceFromValue(required["forbiddenKinds"]), "patch/apply", "file/write", "shell/write")
	if len(stringSliceFromValue(pkg["repairTargetSet"])) > 0 {
		required["mustTargetRepairTargetSet"] = true
	}
	pkg["requiredOutput"] = required
	contract.RepairActionPackage = pkg
	contract.TargetEvidence = convergenceTargetEvidence(input, pkg)
}

func targetSnapshotAcquisitionPrerequisite() map[string]any {
	return map[string]any{
		"kind":             "acquire_target_snapshot",
		"state":            convergenceStateNeedEvidence,
		"runtimeStage":     runtimeStageRepair,
		"runtimeStageMode": "target_snapshot_acquisition",
		"finiteExitRule":   "accepted_target_snapshot_evidence_or_scoped_blocker_or_internal_fail_fast",
		"reason":           "state_change requires current target snapshot",
	}
}

func populateCoverageWorkItemStateChangeConvergenceContract(input map[string]any, contract *ConvergenceContract) {
	if contract == nil {
		return
	}
	contract.State = convergenceStateNeedStateChange
	contract.RuntimeStage = runtimeStageRepair
	contract.RuntimeStageMode = "strict_convergence"
	contract.RepairObjective = repairObjectiveStateChangingOutput
	contract.StateChangingOutputRequired = true
	if contract.RepairActionPackage == nil {
		contract.RepairActionPackage = map[string]any{}
	}
	alreadyRequiresStateChange := repairActionPackageRequiresStateChangingOutput(contract.RepairActionPackage)
	required := map[string]any{}
	if alreadyRequiresStateChange {
		required = mapFromValue(contract.RepairActionPackage["requiredOutput"])
	} else {
		required = enforceStateChangingRequiredOutputForConvergence(contract.RepairActionPackage, input)
	}
	applyEvidenceKindRequirementsToStateChangingRequiredOutput(contract.RepairActionPackage, required, input)
	required["mustReferenceCoverageWorkItem"] = true
	contract.RepairActionPackage["requiredOutput"] = required
	contract.TargetEvidence = convergenceTargetEvidence(input, contract.RepairActionPackage)
}

func stateChangeRequiresTargetSnapshotAcquisition(input map[string]any, pkg map[string]any) bool {
	if len(input) == 0 && len(pkg) == 0 {
		return false
	}
	if stateChangeHasCurrentTargetSnapshot(input, pkg) {
		return false
	}
	hasReadableTarget := stateChangeHasReadableTargetSnapshotRef(input, pkg)
	requiresSnapshot := boolValue(input["requiresCurrentTargetSnapshotForStateChange"]) ||
		boolValue(pkg["requiresCurrentTargetSnapshotForStateChange"]) ||
		stateChangeRequiredOutputRequiresCurrentTargetSnapshot(input, pkg) ||
		boolValue(pkg["primaryStateChangeWithoutTargetEvidence"]) ||
		boolValue(pkg["primaryStateChangeCoverageScoped"]) ||
		boolValue(input["primaryStateChangeWithoutTargetEvidence"]) ||
		boolValue(input["primaryStateChangeCoverageScoped"])
	if requiresSnapshot {
		return true
	}
	if hasReadableTarget {
		return true
	}
	if len(convergenceConcreteFailureTargetRefs(input, pkg)) > 0 {
		return hasReadableTarget
	}
	if stateChangingRepairHasActionableImplementationSurface(input, pkg) {
		return false
	}
	return false
}

func stateChangeRequiredOutputRequiresCurrentTargetSnapshot(input map[string]any, pkg map[string]any) bool {
	for _, required := range []map[string]any{
		mapFromValue(pkg["requiredOutput"]),
		mapFromValue(input["requiredOutput"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]),
		mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]),
		mapFromValue(mapFromValue(input[currentActionContractKey])["requiredOutput"]),
		mapFromValue(mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"])["requiredOutput"]),
		mapFromValue(mapFromValue(mapFromValue(input["executionContext"])[stepInputKeyStagePacket])["requiredOutput"]),
	} {
		if boolValue(required["mustUseCurrentTargetSnapshot"]) {
			return true
		}
	}
	return false
}

func stateChangeHasReadableTargetSnapshotRef(input map[string]any, pkg map[string]any) bool {
	if len(runtimeManagedTargetSnapshotRefs(input)) > 0 {
		return true
	}
	if len(pkg) > 0 && len(runtimeManagedTargetSnapshotRefs(map[string]any{"repairActionPackage": pkg})) > 0 {
		return true
	}
	for _, targetRef := range convergenceConcreteFailureTargetRefs(input, pkg) {
		if runtimeManagedTargetSnapshotRefReadable(targetRef) {
			return true
		}
	}
	return false
}

func stateChangeHasCurrentTargetSnapshot(input map[string]any, pkg map[string]any) bool {
	if repairTargetSnapshotsHaveExactPatchSafe(mergeRepairTargetSnapshots(
		mapSliceFromValue(pkg["targetSnapshots"]),
		mapSliceFromValue(pkg["currentTargetSnapshots"]),
	)) {
		return true
	}
	if repairTargetSnapshotsHaveExactPatchSafe(repairActionTargetSnapshots(input)) {
		return true
	}
	for _, key := range []string{"targetSnapshots", "currentTargetSnapshots", "failureTargetRepairContext", "patchRepairContext", "rootCauseCandidateContext"} {
		if repairTargetSnapshotsHaveExactPatchSafe(mapSliceFromValue(input[key])) {
			return true
		}
	}
	return false
}

func convergenceInputRequiresPendingArtifactStateChangingOutput(input map[string]any, pkg map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if convergenceStateChangeAlreadySatisfied(input) &&
		!convergenceConcreteFailureTargetsOutsideSatisfiedStateChange(input, pkg) {
		return false
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	return strings.TrimSpace(stringFromMap(input, "requestedOutputMode")) == outputModeIncrementalArtifact ||
		outputModeFromInput(input) == outputModeIncrementalArtifact ||
		strings.TrimSpace(stringFromMap(required, "outputMode")) == outputModeIncrementalArtifact ||
		boolValue(required["mustReturnArtifactPlan"]) ||
		boolValue(required["mustReturnArtifactChunk"]) ||
		boolValue(required["stateChangingEvidenceDeferredUntilApply"])
}

func populateCoverageWorkItemValidationConvergenceContract(input map[string]any, contract *ConvergenceContract, includeRepairTargets bool) {
	if contract == nil {
		return
	}
	contract.State = convergenceStateNeedValidation
	contract.RuntimeStage = runtimeStageVerification
	contract.RuntimeStageMode = "strict_convergence"
	contract.RepairObjective = repairObjectiveConcreteValidationEvidence
	if contract.RepairActionPackage == nil {
		contract.RepairActionPackage = map[string]any{}
	}
	required := concreteValidationEvidenceRequiredOutputWithScratch(input, contract.RepairActionPackage, mapFromValue(contract.RepairActionPackage["requiredOutput"]))
	required["mustReferenceCoverageWorkItem"] = true
	if includeRepairTargets {
		if targets := coverageTargetSetFromRepairInput(input); len(targets) > 0 {
			contract.RepairActionPackage["coverageRepairRequired"] = true
			contract.RepairActionPackage["coverageTargetSet"] = targets
			required["mustDeclareCoverageTargets"] = true
			required["coverageTargetMatchRule"] = "validation evidence must map to one unresolved DoD item text, goalRequirementId, goal item, or deliverable item from coverageTargetSet"
		}
	}
	contract.RepairActionPackage["requiredOutput"] = required
}

func coverageWorkItemFocusReadyForAssessment(focus map[string]any) bool {
	if len(focus) == 0 {
		return false
	}
	if coverageWorkItemCanCloseDeterministically(focus) {
		return false
	}
	missing := coverageWorkItemMissingEvidenceKindSet(focus)
	if len(missing) > 0 {
		return false
	}
	required := coverageWorkItemEffectiveRequiredEvidenceKinds(focus)
	acceptedByKind := mapStringSliceValue(focus["acceptedEvidenceByKind"])
	requiredCounts := mapIntValue(focus["requiredEvidenceCounts"])
	for _, kind := range required {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		requiredCount := requiredCounts[kind]
		if requiredCount <= 0 {
			requiredCount = 1
		}
		if coverageEvidenceCountForKind(kind, acceptedByKind[kind]) < requiredCount {
			return false
		}
	}
	return true
}

func convergenceCurrentFocusCanCloseDeterministically(input map[string]any) bool {
	context := coverageWorkItemContextFromInput(input)
	if len(context) == 0 {
		return false
	}
	if coverageWorkItemCanCloseDeterministically(coverageWorkItemPrimaryFocus(context)) {
		return true
	}
	for _, item := range mapSliceFromValue(context["currentFocusItems"]) {
		if coverageWorkItemCanCloseDeterministically(item) {
			return true
		}
	}
	return false
}

func coverageWorkItemContextHasActionableFailure(context map[string]any) bool {
	if coverageWorkItemFocusHasActionableFailure(mapFromValue(context["currentFocus"])) {
		return true
	}
	for _, item := range mapSliceFromValue(context["currentFocusItems"]) {
		if coverageWorkItemFocusHasActionableFailure(item) {
			return true
		}
	}
	return false
}

func coverageWorkItemFocusHasActionableFailure(item map[string]any) bool {
	if len(item) == 0 {
		return false
	}
	return strings.TrimSpace(stringFromMap(item, "status")) == "failed" ||
		len(stringSliceFromValue(item["rejectedEvidenceRefs"])) > 0
}

func coverageWorkItemContextHasMissingEvidenceKind(context map[string]any, evidenceKind string) bool {
	evidenceKind = strings.TrimSpace(evidenceKind)
	if len(context) == 0 || evidenceKind == "" {
		return false
	}
	for _, key := range []string{"currentFocus", "currentFocusItems", "unresolvedItems", "ledgerSummary"} {
		if item := mapFromValue(context[key]); len(item) > 0 && coverageWorkItemMissingEvidenceKindSet(item)[evidenceKind] {
			return true
		}
		for _, item := range mapSliceFromValue(context[key]) {
			if coverageWorkItemMissingEvidenceKindSet(item)[evidenceKind] {
				return true
			}
		}
	}
	return false
}

func convergenceCurrentFocusNeedsValidationWithoutStateChange(input map[string]any) bool {
	for _, item := range convergenceCurrentFocusCandidates(input) {
		if len(item) == 0 {
			continue
		}
		missing := coverageWorkItemExplicitOrDerivedMissingEvidenceKindSet(item)
		if missing["validation"] && !missing["state_change"] {
			return true
		}
	}
	return false
}

func convergenceCurrentFocusNeedsStateChange(input map[string]any) bool {
	for _, item := range convergenceCurrentFocusCandidates(input) {
		if len(item) == 0 {
			continue
		}
		missing := coverageWorkItemExplicitOrDerivedMissingEvidenceKindSet(item)
		if missing["state_change"] && !missing["validation"] && !missing["artifact"] {
			return true
		}
	}
	return false
}

func coverageWorkItemExplicitOrDerivedMissingEvidenceKindSet(item map[string]any) map[string]bool {
	if len(item) == 0 {
		return nil
	}
	if _, ok := item["missingEvidenceKinds"]; ok {
		return stringSet(compactUniqueStrings(stringSliceFromValue(item["missingEvidenceKinds"])))
	}
	return coverageWorkItemMissingEvidenceKindSet(item)
}

func convergenceCurrentFocusCandidates(input map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := []map[string]any{}
	if context := coverageWorkItemContextFromInput(input); len(context) > 0 {
		if focus := coverageWorkItemPrimaryFocus(context); len(focus) > 0 {
			ret = append(ret, focus)
		}
	}
	for _, source := range []map[string]any{
		mapFromValue(input[convergenceAuthorityKey]),
		mapFromValue(input[gapBaselineKey]),
		mapFromValue(mapFromValue(input["executionContext"])[convergenceAuthorityKey]),
		mapFromValue(mapFromValue(input["executionContext"])[gapBaselineKey]),
	} {
		if focus := mapFromValue(source["currentFocus"]); len(focus) > 0 {
			ret = append(ret, focus)
		}
	}
	return ret
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
	if convergenceStateChangeAlreadySatisfied(input) &&
		!convergenceConcreteFailureTargetsOutsideSatisfiedStateChange(input, pkg) {
		return false
	}
	if convergenceCoverageFocusRequiresValidation(input) &&
		validationContractFailureMustRemainValidation(input, pkg, nil) {
		return false
	}
	if !inputHasConcreteValidationFailureDiagnostic(input, pkg) {
		return false
	}
	return repairHasImplementationTargetEvidence(input, pkg)
}

func validationContractFailureMustRemainValidation(input map[string]any, pkg map[string]any, failedStep *runModel.Step) bool {
	if validationContractViolationMustRemainValidation(input, pkg, failedStep) {
		return true
	}
	if validationOnlyFocusPreemptsStateChangingRepair(input, pkg, failedStep) {
		return true
	}
	if validationRequiredOutputCapabilityMismatch(input, pkg, failedStep) {
		return true
	}
	for _, diagnostic := range validationFailureDiagnosticCandidates(input, pkg, failedStep) {
		if validationFailureDiagnosticIsCommandContractFailure(diagnostic) {
			return true
		}
	}
	text := strings.ToLower(strings.Join(validationFailureDiagnosticCurrentTextCandidates(input, pkg, failedStep), "\n"))
	return strings.Contains(text, "shell_validation_contract_rejected") ||
		strings.Contains(text, "validation command masks command failure") ||
		strings.Contains(text, "validation shell command masks command failure") ||
		strings.Contains(text, "validation_required_output_capability_mismatch") ||
		strings.Contains(text, "not accepted by current requiredoutput.acceptedkinds") ||
		strings.Contains(text, "validation runtime dispatch requires validationevidence") ||
		strings.Contains(text, "validation runtime dispatch requires concrete validationevidence")
}

func validationContractViolationMustRemainValidation(input map[string]any, pkg map[string]any, failedStep *runModel.Step) bool {
	if !convergenceCurrentFocusNeedsValidationWithoutStateChange(input) {
		return false
	}
	if latestFailedStepHasConcreteImplementationTargetEvidence(failedStep) {
		return false
	}
	for _, rule := range validationContractViolationRuleCandidates(input, pkg, failedStep) {
		if validationContractViolationRuleMustRemainValidation(rule) {
			return true
		}
	}
	return false
}

func latestFailedStepHasConcreteImplementationTargetEvidence(failedStep *runModel.Step) bool {
	if failedStep == nil || failedStep.State != common.StepStateFailed {
		return false
	}
	if validationFailureWithConcreteTargetRequiresStateChangingRepair(failedStep) {
		return true
	}
	if !repairHasImplementationTargetEvidence(failedStep.Output, mapFromValue(failedStep.Output["repairActionPackage"])) {
		return false
	}
	for _, diagnostic := range validationFailureDiagnosticCandidates(failedStep.Output, nil, nil) {
		if allowedValidationDiagnosticClass(stringFromMap(diagnostic, "class")) == runtimemodel.ValidationDiagnosticImplementationGap ||
			boolValue(diagnostic["stateChangingEvidenceNeeded"]) ||
			boolValue(diagnostic["implementationGap"]) {
			return true
		}
	}
	return false
}

func validationContractViolationRuleCandidates(input map[string]any, pkg map[string]any, failedStep *runModel.Step) []string {
	seen := map[string]bool{}
	rules := make([]string, 0, 4)
	addRule := func(rule string) {
		rule = strings.TrimSpace(rule)
		if rule == "" || seen[rule] {
			return
		}
		seen[rule] = true
		rules = append(rules, rule)
	}
	var addSource func(map[string]any)
	addSource = func(source map[string]any) {
		if len(source) == 0 {
			return
		}
		addRule(stringFromMap(source, "contractViolationRule"))
		addRule(stringFromMap(source, "violatedRule"))
		if violation := mapFromValue(source[stepOutputKeyContractViolation]); len(violation) > 0 {
			addRule(stringFromMap(violation, "violatedRule"))
		}
		if recovery := mapFromValue(source[stepInputKeyContractViolationRecovery]); len(recovery) > 0 {
			addRule(stringFromMap(recovery, "violatedRule"))
			addSource(mapFromValue(recovery["correctionContract"]))
		}
	}
	if failedStep != nil {
		addSource(failedStep.Output)
		addSource(failedStep.Input)
	}
	addSource(input)
	addSource(pkg)
	for _, key := range []string{"failedStepOutput", "currentFailure", "lastFailureDiagnostics"} {
		addSource(mapFromValue(input[key]))
		addSource(mapFromValue(pkg[key]))
	}
	return rules
}

func validationContractViolationRuleMustRemainValidation(rule string) bool {
	switch strings.TrimSpace(rule) {
	case contractViolationRuleConcreteValidationMissing,
		contractViolationRuleValidationEvidenceMetadataMissing,
		contractViolationRuleValidationScratchRequired,
		contractViolationRuleValidationScratchEscape,
		contractViolationRuleValidationProcessControlForbidden,
		contractViolationRuleValidationDetachedProcessForbidden,
		contractViolationRuleValidationShellSafetyUnproven,
		contractViolationRuleValidationStaticOnly,
		contractViolationRuleManagedValidationRequired,
		contractViolationRuleValidationRequiredOutputCapabilityMismatch:
		return true
	default:
		return false
	}
}

func validationRequiredOutputCapabilityMismatch(input map[string]any, pkg map[string]any, failedStep *runModel.Step) bool {
	text := strings.ToLower(strings.Join(validationFailureDiagnosticCurrentTextCandidates(input, pkg, failedStep), "\n"))
	if !strings.Contains(text, "validation_required_output_capability_mismatch") &&
		!strings.Contains(text, "not accepted by current requiredoutput.acceptedkinds") {
		return false
	}
	return currentActionRequiresConcreteValidation(input) ||
		convergenceCurrentFocusNeedsValidationWithoutStateChange(input)
}

func validationOnlyFocusPreemptsStateChangingRepair(input map[string]any, pkg map[string]any, failedStep *runModel.Step) bool {
	if !convergenceCurrentFocusNeedsValidationWithoutStateChange(input) || coverageRepairInputRequiresStateChange(input) {
		return false
	}
	if validationFailureWithConcreteTargetRequiresStateChangingRepair(failedStep) &&
		convergenceCurrentFocusRequiresEvidenceKind(input, "state_change") {
		return false
	}
	hasValidationDiagnostic := false
	for _, diagnostic := range validationFailureDiagnosticCandidates(input, pkg, failedStep) {
		class := allowedValidationDiagnosticClass(stringFromMap(diagnostic, "class"))
		if class == "" {
			continue
		}
		hasValidationDiagnostic = true
		if class == runtimemodel.ValidationDiagnosticImplementationGap ||
			boolValue(diagnostic["stateChangingEvidenceNeeded"]) ||
			boolValue(diagnostic["implementationGap"]) {
			return false
		}
	}
	return hasValidationDiagnostic
}

func convergenceCurrentFocusRequiresEvidenceKind(input map[string]any, kind string) bool {
	if strings.TrimSpace(kind) == "" {
		return false
	}
	for _, item := range convergenceCurrentFocusCandidates(input) {
		if stringSet(coverageWorkItemEffectiveRequiredEvidenceKinds(item))[kind] {
			return true
		}
	}
	return false
}

func validationFailureDiagnosticCandidates(input map[string]any, pkg map[string]any, failedStep *runModel.Step) []map[string]any {
	candidates := make([]map[string]any, 0)
	add := func(source map[string]any) {
		if len(source) == 0 {
			return
		}
		if diagnostic := mapFromValue(source["validationFailureDiagnostic"]); len(diagnostic) > 0 {
			candidates = append(candidates, diagnostic)
		}
		if diagnostics := mapFromValue(source["runtimeDiagnostics"]); len(diagnostics) > 0 {
			if diagnostic := mapFromValue(diagnostics["validationFailureDiagnostic"]); len(diagnostic) > 0 {
				candidates = append(candidates, diagnostic)
			}
		}
	}
	add(input)
	add(pkg)
	for _, key := range []string{
		"lastFailureDiagnostics",
		"currentFailure",
		"failedStepOutput",
		"currentExecutionProgress",
	} {
		add(mapFromValue(input[key]))
		add(mapFromValue(pkg[key]))
	}
	for _, progress := range []map[string]any{
		mapFromValue(input["currentExecutionProgress"]),
		mapFromValue(pkg["currentExecutionProgress"]),
	} {
		add(mapFromValue(progress["currentFailure"]))
		add(mapFromValue(progress["lastObservedFailure"]))
	}
	if failedStep != nil {
		add(failedStep.Input)
		add(failedStep.Output)
	}
	return candidates
}

func validationFailureDiagnosticIsCommandContractFailure(diagnostic map[string]any) bool {
	if len(diagnostic) == 0 {
		return false
	}
	class := strings.TrimSpace(stringFromMap(diagnostic, "class"))
	targetType := strings.TrimSpace(stringFromMap(diagnostic, "recommendedEvidenceTargetType"))
	stage := strings.TrimSpace(stringFromMap(diagnostic, "stage"))
	return class == "validation_command_invalid" ||
		targetType == "command_contract" ||
		stage == "shell_validation_contract_rejected" ||
		boolValue(diagnostic["commandPolicyViolation"])
}

func validationFailureDiagnosticTextCandidates(input map[string]any, pkg map[string]any, failedStep *runModel.Step) []string {
	var texts []string
	add := func(source map[string]any) {
		if len(source) == 0 {
			return
		}
		for _, key := range []string{
			"error",
			"reason",
			"stage",
			"failureReason",
			failureOutputKeySignature,
			"failureSignature",
		} {
			if value := strings.TrimSpace(stringFromMap(source, key)); value != "" {
				texts = append(texts, value)
			}
		}
	}
	add(input)
	add(pkg)
	for _, key := range []string{
		"lastFailureDiagnostics",
		"currentFailure",
		"failedStepOutput",
		"currentExecutionProgress",
	} {
		add(mapFromValue(input[key]))
		add(mapFromValue(pkg[key]))
	}
	for _, progress := range []map[string]any{
		mapFromValue(input["currentExecutionProgress"]),
		mapFromValue(pkg["currentExecutionProgress"]),
	} {
		add(mapFromValue(progress["currentFailure"]))
		add(mapFromValue(progress["lastObservedFailure"]))
	}
	if failedStep != nil {
		add(failedStep.Input)
		add(failedStep.Output)
	}
	return texts
}

func validationFailureDiagnosticCurrentTextCandidates(input map[string]any, pkg map[string]any, failedStep *runModel.Step) []string {
	var texts []string
	add := func(source map[string]any) {
		if len(source) == 0 {
			return
		}
		for _, key := range []string{
			"error",
			"reason",
			"stage",
			"failureReason",
			failureOutputKeySignature,
			"failureSignature",
		} {
			if value := strings.TrimSpace(stringFromMap(source, key)); value != "" {
				texts = append(texts, value)
			}
		}
	}
	add(input)
	add(pkg)
	for _, key := range []string{
		"lastFailureDiagnostics",
		"failedStepOutput",
	} {
		add(mapFromValue(input[key]))
		add(mapFromValue(pkg[key]))
	}
	if failedStep != nil {
		add(failedStep.Input)
		add(failedStep.Output)
		add(mapFromValue(failedStep.Output["lastFailureDiagnostics"]))
	}
	return texts
}

func convergenceStateChangeAlreadySatisfied(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if len(mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"])) > 0 ||
		boolValue(input["stateChangeAlreadySatisfiedByCurrentTarget"]) {
		return true
	}
	pkg := mapFromValue(input["repairActionPackage"])
	return len(mapSliceFromValue(pkg["currentTargetAlreadySatisfiesFailedPatch"])) > 0 ||
		boolValue(pkg["stateChangeAlreadySatisfiedByCurrentTarget"])
}

func convergenceConcreteFailureTargetsOutsideSatisfiedStateChange(input map[string]any, pkg map[string]any) bool {
	satisfiedTargets := convergenceAlreadySatisfiedStateChangeTargetSet(input, pkg)
	if len(satisfiedTargets) == 0 {
		return false
	}
	for _, targetRef := range convergenceConcreteFailureTargetRefs(input, pkg) {
		if !satisfiedTargets[targetRef] {
			return true
		}
	}
	return false
}

func convergenceAlreadySatisfiedStateChangeTargetSet(input map[string]any, pkg map[string]any) map[string]bool {
	targets := map[string]bool{}
	add := func(value string) {
		for _, targetRef := range filterAdmissibleRuntimeRepairTargetRefs([]string{value}) {
			targets[targetRef] = true
		}
	}
	addItems := func(items []map[string]any) {
		for _, item := range items {
			add(stringFromMap(item, "targetRef"))
			for _, targetRef := range stringSliceFromValue(item["targetRefs"]) {
				add(targetRef)
			}
		}
	}
	addItems(mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"]))
	addItems(mapSliceFromValue(pkg["currentTargetAlreadySatisfiesFailedPatch"]))
	if len(targets) == 0 {
		return nil
	}
	return targets
}

func convergenceConcreteFailureTargetRefs(input map[string]any, pkg map[string]any) []string {
	seen := map[string]bool{}
	targets := make([]string, 0)
	add := func(value string) {
		for _, targetRef := range filterAdmissibleRuntimeRepairTargetRefs([]string{value}) {
			if seen[targetRef] {
				continue
			}
			seen[targetRef] = true
			targets = append(targets, targetRef)
		}
	}
	var addFailureMap func(map[string]any)
	addFailureMap = func(source map[string]any) {
		if len(source) == 0 {
			return
		}
		for _, targetRef := range stringSliceFromValue(source["failureTargets"]) {
			add(targetRef)
		}
		for _, targetRef := range stringSliceFromValue(source["targetRefs"]) {
			add(targetRef)
		}
		for _, key := range []string{"targetRef", "conflictRef", "path"} {
			add(stringFromMap(source, key))
		}
		for _, lineRef := range mapSliceFromValue(source["lineRefs"]) {
			add(stringFromMap(lineRef, "targetRef"))
		}
		for _, key := range []string{failureOutputKeySignature, "failureSignature", "signature", "targetScope", "failureTargetScope"} {
			for _, targetRef := range failureTargetsFromSignatureText(stringFromMap(source, key)) {
				add(targetRef)
			}
		}
		if diagnostics := mapFromValue(source["runtimeDiagnostics"]); len(diagnostics) > 0 {
			addFailureMap(diagnostics)
		}
	}
	addFailureMap(mapFromValue(input["lastFailureDiagnostics"]))
	addFailureMap(mapFromValue(pkg["lastFailureDiagnostics"]))
	addFailureMap(mapFromValue(input["currentFailure"]))
	addFailureMap(mapFromValue(pkg["currentFailure"]))
	addFailureMap(mapFromValue(input["structuredFailureSignals"]))
	addFailureMap(mapFromValue(pkg["structuredFailureSignals"]))
	for _, progress := range []map[string]any{
		mapFromValue(input["currentExecutionProgress"]),
		mapFromValue(pkg["currentExecutionProgress"]),
	} {
		if len(progress) == 0 {
			continue
		}
		addFailureMap(progress)
		addFailureMap(mapFromValue(progress["currentFailure"]))
		addFailureMap(mapFromValue(progress["lastObservedFailure"]))
		for _, key := range []string{"latestActionEvidence", "completedGraphEvidence"} {
			for _, item := range mapSliceFromValue(progress[key]) {
				addFailureMap(item)
				addFailureMap(mapFromValue(item["outputSummary"]))
				addFailureMap(mapFromValue(item["output"]))
			}
		}
	}
	if len(targets) == 0 {
		return nil
	}
	return targets
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
	if len(input) == 0 || failedStep == nil {
		return false
	}
	if convergenceInputHasUnresolvedStateChangeWithoutActionableSurface(input) {
		return false
	}
	if convergenceConcreteValidationCannotAdvanceWithoutTarget(input, failedStep) {
		return false
	}
	if runtimeFailureIsLLMContractLoop(failedStep) && convergenceCoverageFocusRequiresValidation(input) {
		return true
	}
	if !stateChangingSchemaInvalidRepair(failedStep) {
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

func convergenceInputHasUnresolvedStateChangeWithoutActionableSurface(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if !inputRequiresStateChangingOutput(input) &&
		!coverageWorkItemContextHasMissingEvidenceKind(coverageWorkItemContextFromInput(input), "state_change") &&
		!coverageWorkItemSetRequiresEvidenceKind(mapSliceFromValue(input["coverageTargetSet"]), "state_change") {
		return false
	}
	return !stateChangingRepairHasActionableImplementationSurface(input, mapFromValue(input["repairActionPackage"]))
}

func convergenceCoverageFocusRequiresValidation(input map[string]any) bool {
	context := coverageWorkItemContextFromInput(input)
	if len(context) == 0 {
		return false
	}
	focus := coverageWorkItemPrimaryFocus(context)
	if len(focus) == 0 {
		return false
	}
	state := deriveCoverageWorkItemState(focus)
	if !state.HasActionableEvidenceGap() {
		return false
	}
	missing := stringSet(state.MissingEvidenceKinds)
	if missing["state_change"] {
		return false
	}
	if missing["validation"] || missing["artifact"] {
		return true
	}
	return stringSliceContains(stringSliceFromValue(focus["nextAllowedConvergenceStates"]), convergenceStateNeedValidation)
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
	required := concreteValidationEvidenceRequiredOutputWithScratch(input, contract.RepairActionPackage, mapFromValue(contract.RepairActionPackage["requiredOutput"]))
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
	inheritRepeatedValidationRepairTargetEvidence(pkg, failedStep)
	if !stateChangingRepairHasConcreteTargetEvidence(input, pkg) &&
		!stateChangingRepairCanStartWithoutTargetEvidence(input, failedStep) {
		required := evidenceAcquisitionRequiredOutputWithScratch(input, pkg, mapFromValue(pkg["requiredOutput"]))
		required["evidenceAcquisitionRule"] = "current target evidence is required before state-changing patch/apply repair"
		pkg["requiredOutput"] = required
		pkg["evidenceAcquisitionRequired"] = true
		pkg["evidenceAcquisitionReason"] = "missing current target evidence for repeated validation repair escalation"
		return pkg
	}
	required := enforceStateChangingRequiredOutputForConvergence(pkg, input)
	addStrategyReassessmentRequiredOutputContract(pkg, required)
	return pkg
}

func stateChangingRepairCanStartWithoutTargetEvidence(input map[string]any, failedStep *runModel.Step) bool {
	return repairInputHasEmptyWorkspaceSurfaceEvidence(input) ||
		runtimeRepairInputHasEmptyWorkspaceSurfaceEvidence(failedStep)
}

func inheritRepeatedValidationRepairTargetEvidence(pkg map[string]any, failedStep *runModel.Step) {
	if len(pkg) == 0 || failedStep == nil || len(failedStep.Input) == 0 {
		return
	}
	if len(mapSliceFromValue(pkg["targetSnapshots"])) > 0 {
		return
	}
	for _, key := range []string{"failureTargetRepairContext", "patchRepairContext", "rootCauseCandidateContext"} {
		if repairContextHasAdmissibleTargetEvidence(failedStep.Input[key]) {
			pkg["targetSnapshots"] = cloneStepInputValue(failedStep.Input[key])
			return
		}
	}
	if snapshots := repairActionTargetSnapshots(failedStep.Input); len(snapshots) > 0 {
		pkg["targetSnapshots"] = cloneStepInputValue(snapshots)
	}
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
	targetSnapshotsChanged := false
	inherit := func(key string) {
		value, ok := targetEvidence[key]
		if !ok || value == nil {
			return
		}
		input[key] = cloneStepInputValue(value)
		pkg[key] = cloneStepInputValue(value)
		pkgChanged = true
		if key == "targetSnapshots" {
			targetSnapshotsChanged = true
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
	if targetSnapshotsChanged || len(mapSliceFromValue(pkg["currentTargetSnapshots"])) == 0 {
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
	targets := mapSliceFromValue(input["coverageTargetSet"])
	if len(targets) == 0 {
		targets = coverageTargetSetFromRepairInput(input)
	}
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
			EvidenceRole:       normalizedCoverageEvidenceRole(stringFromMap(item, "evidenceRole")),
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
		if !convergenceConcreteFailureTargetsOutsideSatisfiedStateChange(input, mapFromValue(input["repairActionPackage"])) {
			return false
		}
	}
	if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
		if len(mapSliceFromValue(pkg["currentTargetAlreadySatisfiesFailedPatch"])) > 0 ||
			boolValue(pkg["stateChangeAlreadySatisfiedByCurrentTarget"]) {
			if !convergenceConcreteFailureTargetsOutsideSatisfiedStateChange(input, pkg) {
				return false
			}
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
	if convertedReadableDecisionHasActiveCoverageDefinition(run) {
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

func convertedReadableDecisionHasActiveCoverageDefinition(run *runModel.Run) bool {
	if run == nil {
		return false
	}
	if len(effectiveDoDItemTexts(run)) > 0 ||
		len(normalizedRunGoalCoverageItems(run)) > 0 ||
		len(requiredProfileDeliverableExpectations(run)) > 0 {
		return true
	}
	if run.DoDID != "" {
		return true
	}
	if len(run.ExecutionContext) == 0 {
		return false
	}
	for _, key := range []string{
		"goal",
		"goals",
		"goalText",
		"effectiveGoalView",
		"dod",
		"dods",
		"executionContract",
	} {
		if convergenceDefinitionValueHasContent(run.ExecutionContext[key]) {
			return true
		}
	}
	return false
}

func convergenceDefinitionValueHasContent(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case []string:
		return len(typed) > 0
	case []any:
		return len(typed) > 0
	case []map[string]any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
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
	var rawFailedOutput []map[string]any
	if failedStep != nil && len(failedStep.Output) > 0 {
		rawFailedOutput = append(rawFailedOutput, failedStep.Output)
	}
	if validationCommandContractFailure(input, existing, failedStep) {
		if pkg := validationCommandContractRepairActionPackage(input, existing, rawFailedOutput...); len(pkg) > 0 {
			return pkg
		}
	}
	if len(existing) > 0 {
		syncRepairActionPackageTargetSnapshots(input, existing)
		required := mapFromValue(existing["requiredOutput"])
		if boolValue(required["mustIncludeStateChangingNode"]) {
			applyConcreteRepairEvidenceOutputContract(input, existing, required)
			addStrategyReassessmentRequiredOutputContract(existing, required)
			existing["requiredOutput"] = required
			reapplyRepairCoverageFocusContract(input, existing)
			return existing
		}
	}
	pkg := repairActionPackageFromRepairInput(input, rawFailedOutput...)
	if len(pkg) == 0 {
		pkg = existing
	}
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	required := enforceStateChangingRequiredOutputForConvergence(pkg, input)
	addStrategyReassessmentRequiredOutputContract(pkg, required)
	reapplyRepairCoverageFocusContract(input, pkg)
	return pkg
}

func syncRepairActionPackageTargetSnapshots(input map[string]any, pkg map[string]any) {
	if len(input) == 0 || len(pkg) == 0 {
		return
	}
	targets := repairActionTargetSnapshots(input)
	if len(targets) == 0 {
		return
	}
	merged := mergeRepairTargetSnapshots(mapSliceFromValue(pkg["targetSnapshots"]), targets)
	if len(merged) == 0 {
		return
	}
	pkg["targetSnapshots"] = merged
	pkg["currentTargetSnapshots"] = repairActionTargetSnapshotMetadata(merged)
	required := mapFromValue(pkg["requiredOutput"])
	if len(required) == 0 {
		required = stateChangingRepairRequiredOutput()
	}
	required["mustTargetFailureScope"] = true
	required["targetRelation"] = "failure_target_repair_snapshot_or_declared_related_failure_target"
	applyTargetSnapshotSafetyRequiredOutputContract(required, merged)
	pkg["requiredOutput"] = required
}

func enforceStateChangingRequiredOutputForConvergence(pkg map[string]any, inputs ...map[string]any) map[string]any {
	clearEvidenceAcquisitionStateForStateChange(pkg)
	required := stateChangingRepairRequiredOutput()
	preserveStateChangingRequiredOutputSignals(required, mapFromValue(pkg["requiredOutput"]))
	if len(mapSliceFromValue(pkg["targetSnapshots"])) > 0 || len(mapSliceFromValue(pkg["currentTargetSnapshots"])) > 0 {
		snapshots := mergeRepairTargetSnapshots(mapSliceFromValue(pkg["targetSnapshots"]), mapSliceFromValue(pkg["currentTargetSnapshots"]))
		required["mustTargetFailureScope"] = true
		required["targetRelation"] = "failure_target_repair_snapshot_or_declared_related_failure_target"
		applyTargetSnapshotSafetyRequiredOutputContract(required, snapshots)
	}
	if len(stringSliceFromValue(pkg["repairTargetSet"])) > 0 {
		required["mustTargetRepairTargetSet"] = true
	}
	if len(mapSliceFromValue(pkg["coverageTargetSet"])) > 0 {
		required["mustDeclareCoverageTargets"] = true
		required["coverageTargetMatchRule"] = "patch/apply coverageTargets must match one unresolved DoD item text, goalRequirementId, goal item, or deliverable item from coverageTargetSet"
	}
	applyEvidenceKindRequirementsToStateChangingRequiredOutput(pkg, required, inputs...)
	pkg["requiredOutput"] = required
	if len(inputs) > 0 {
		reapplyRepairCoverageFocusContract(inputs[0], pkg)
		required = mapFromValue(pkg["requiredOutput"])
		applyEvidenceKindRequirementsToStateChangingRequiredOutput(pkg, required, inputs...)
		pkg["requiredOutput"] = required
	}
	return required
}

func applyEvidenceKindRequirementsToStateChangingRequiredOutput(pkg map[string]any, required map[string]any, inputs ...map[string]any) {
	if len(required) == 0 || !requiredOutputRequiresStateChanging(required) {
		return
	}
	requirements := evidenceKindRequirementSummariesForRequiredOutput(pkg, inputs...)
	if len(requirements) == 0 {
		return
	}
	acceptedKinds := stringSliceFromValue(required["acceptedKinds"])
	acceptedOperations := stringSliceFromValue(required["acceptedOperations"])
	foundStateChangingRequirement := false
	for _, requirement := range requirements {
		requirementKinds := stringSliceFromValue(requirement["acceptedStepKinds"])
		if len(requirementKinds) == 0 {
			continue
		}
		stateChangingKinds := make([]string, 0, len(requirementKinds))
		for _, kind := range requirementKinds {
			if requiredOutputKindIsStateChanging(kind) {
				stateChangingKinds = append(stateChangingKinds, strings.TrimSpace(kind))
			}
		}
		if len(stateChangingKinds) == 0 {
			continue
		}
		foundStateChangingRequirement = true
		acceptedKinds = append(acceptedKinds, stateChangingKinds...)
		acceptedOperations = append(acceptedOperations, stringSliceFromValue(requirement["acceptedOperations"])...)
	}
	if !foundStateChangingRequirement {
		return
	}
	acceptedKinds = compactUniqueStrings(acceptedKinds)
	required["acceptedKinds"] = acceptedKinds
	if len(acceptedOperations) > 0 {
		required["acceptedOperations"] = compactUniqueStrings(lowerStringSlice(acceptedOperations))
	}
	required["forbiddenKinds"] = removeStringsFromSlice(stringSliceFromValue(required["forbiddenKinds"]), acceptedKinds...)
	if strings.TrimSpace(stringFromMap(required, "evidenceKindRequirementRule")) == "" {
		required["evidenceKindRequirementRule"] = "state-changing outputs may satisfy current focus using acceptedStepKinds and acceptedOperations declared by evidenceKindRequirements; do not replace profile-required evidence kinds with unrelated generic patch output"
	}
}

func evidenceKindRequirementSummariesForRequiredOutput(pkg map[string]any, inputs ...map[string]any) []map[string]any {
	seen := map[string]bool{}
	ret := make([]map[string]any, 0)
	add := func(items []map[string]any) {
		for _, item := range items {
			if len(item) == 0 {
				continue
			}
			key := strings.Join(compactUniqueStrings(append(
				[]string{stringFromMap(item, "scope"), stringFromMap(item, "id"), stringFromMap(item, "evidenceRole")},
				append(stringSliceFromValue(item["acceptedStepKinds"]), stringSliceFromValue(item["acceptedOperations"])...)...,
			)), "|")
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			ret = append(ret, item)
		}
	}
	add(mapSliceFromValue(pkg["evidenceKindRequirements"]))
	for _, input := range inputs {
		add(mapSliceFromValue(input["evidenceKindRequirements"]))
		add(mapSliceFromValue(mapFromValue(input["repairActionPackage"])["evidenceKindRequirements"]))
		add(mapSliceFromValue(mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"])["evidenceKindRequirements"]))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func clearEvidenceAcquisitionStateForStateChange(pkg map[string]any) {
	if len(pkg) == 0 {
		return
	}
	for _, key := range []string{
		"evidenceAcquisitionRequired",
		"concreteFailureEvidenceRequired",
		"patchForbiddenUntilConcreteTargetEvidence",
		"deliverableWriteForbidden",
		"stateChangingForbidden",
	} {
		delete(pkg, key)
	}
	if strings.TrimSpace(stringFromMap(pkg, "repairObjective")) == repairObjectiveAcquireFailureEvidence {
		delete(pkg, "repairObjective")
	}
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
		case "acceptedKinds",
			"acceptedOperations",
			"toolStateChangingInputContract",
			"toolStateChangingRule":
			if boolValue(existing["mustIncludeStateChangingNode"]) {
				required[key] = value
			}
		case "forbiddenKinds":
			preserved := make([]string, 0, len(stringSliceFromValue(value)))
			for _, kind := range stringSliceFromValue(value) {
				if requiredOutputKindIsStateChanging(kind) {
					continue
				}
				preserved = append(preserved, kind)
			}
			required[key] = compactUniqueStrings(append(stringSliceFromValue(required[key]), preserved...))
		case "mustReturnValidJSON",
			"mustNotReuseRejectedJSON",
			"forbiddenStalePatchReuse",
			"mustReturnPatchApplyNode",
			"patchApplyOnlyReason",
			"mustRebuildFromCurrentEvidence",
			"mustAddressPrimaryDiagnostics",
			"primaryDiagnosticMatchRule",
			"mustPatchDirectlyFromProvidedSnapshot",
			"mustUseCurrentTargetSnapshot",
			"sameGraphReadPatchRule",
			"patchApplyFieldRule",
			"readBeforePatchRecoveryRule",
			"fileWriteExistingTargetRule",
			"targetSnapshotSafetyRule",
			"mustTargetFailureScope",
			"targetRelation",
			"rootCauseCandidateRule",
			"resourceShapeAlignmentRule",
			"mustRepairCapabilityPolicyFollowUp",
			"capabilityPolicyFollowUpRule",
			"capabilityPolicyRepairScopeRule",
			"mustAvoidNoOpPatch",
			"mustProduceSubstantivePatch",
			"mustDeclareStrategyReassessmentFields",
			"strategyReassessmentRequiredFields",
			"strategyReassessmentRepairModes",
			"strategyReassessmentMatchRule",
			"mustReassessRepeatedFailedSurface",
			"mustShiftRootCauseTarget",
			"sameTargetContinuationRule",
			"rootCauseTargetShiftRule",
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
	if contract.State == convergenceStateBlocked && strings.TrimSpace(contract.GovernanceReason) != "" {
		contract.State = convergenceStateNeedGovernance
		contract.RuntimeStage = runtimeStageDecision
		if strings.TrimSpace(contract.RuntimeStageMode) == "" {
			contract.RuntimeStageMode = "structured_governance"
		}
	}
	if !contract.DeterministicComplete && (contract.State == "" || contract.State == convergenceStateBlocked) {
		return input
	}
	clearConvergenceDerivedOutputModeFields(input)
	syncConvergenceNextActionWithContract(input, contract)
	if contract.DeterministicComplete {
		delete(input, "convergenceContract")
		setRuntimeStage(input, runtimeStageVerification)
		input[stepInputKeySoulStage] = soulStageValidation
		delete(input, "replanStateChangingContinuation")
		delete(input, stepInputKeyAssessmentOnly)
		delete(input, "assessmentOnlyReason")
		delete(input, "assessmentOnlyContractCorrection")
		delete(input, "runtimeRepairFollowUp")
		delete(input, stepInputKeyProviderRetryFollowUp)
		delete(input, "repairObjective")
		delete(input, "stateChangingOutputRequired")
		delete(input, "concreteFailureEvidenceRequired")
		delete(input, "boundedValidationRequired")
		delete(input, "evidenceRepairRequired")
		delete(input, "repairGuidance")
		delete(input, "repairActionPackage")
		delete(input, "requiredOutput")
		delete(input, stepInputKeyStagePacket)
		return input
	}
	input["convergenceContract"] = compactConvergenceContract(contract)
	switch contract.State {
	case convergenceStateNeedEvidence:
		setRuntimeStage(input, runtimeStageRepair)
		input[stepInputKeySoulStage] = soulStageExecution
		input["repairObjective"] = repairObjectiveAcquireFailureEvidence
		input["concreteFailureEvidenceRequired"] = true
		delete(input, stepInputKeyAssessmentOnly)
		delete(input, "assessmentOnlyReason")
		delete(input, "stateChangingOutputRequired")
		delete(input, "boundedValidationRequired")
		delete(input, "evidenceRepairRequired")
		pkg := cloneStepPayload(contract.RepairActionPackage)
		if len(pkg) == 0 {
			pkg = map[string]any{}
		}
		required := evidenceAcquisitionRequiredOutputWithScratch(input, pkg, mapFromValue(pkg["requiredOutput"]))
		if contract.RuntimeStageMode == "target_snapshot_acquisition" {
			required["evidenceAcquisitionSubtype"] = "target_snapshot"
			applyTargetSnapshotAcquisitionRequiredOutputContract(required)
			required["forbiddenKinds"] = appendUniqueStrings(stringSliceFromValue(required["forbiddenKinds"]), "patch/apply", "file/write", "shell/write")
		}
		pkg["requiredOutput"] = required
		pkg["evidenceAcquisitionRequired"] = true
		sanitizeRepairActionPackageTargetEvidence(pkg)
		input["repairActionPackage"] = pkg
		currentActionSetRequiredOutput(input, required)
		if boolValue(pkg["capabilityInputLocatorInvalid"]) {
			input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
				"The current failure is an invalid capability input locator. Acquire valid input evidence before retrying the capability or producing state-changing output.",
				"Do not modify deliverables merely to make the previous invalid locator coordinate become valid.",
			)
		}
	case convergenceStateNeedStateChange:
		setRuntimeStage(input, runtimeStageRepair)
		input[stepInputKeySoulStage] = soulStageExecution
		input["repairObjective"] = repairObjectiveStateChangingOutput
		input["stateChangingOutputRequired"] = true
		clearValidationOnlyConvergenceFields(input)
		delete(input, stepInputKeyAssessmentOnly)
		delete(input, "assessmentOnlyReason")
		input["repairGuidance"] = stateChangingConvergenceGuidance(input["repairGuidance"])
		pkg := cloneStepPayload(contract.RepairActionPackage)
		if len(pkg) == 0 {
			pkg = cloneStepPayload(mapFromValue(input["repairActionPackage"]))
		}
		if len(pkg) == 0 {
			pkg = map[string]any{}
		}
		if convergenceContractHasTargetSnapshotPrerequisite(contract) {
			input["concreteFailureEvidenceRequired"] = true
			pkg["parentNextAction"] = map[string]any{"kind": "state_change", "reason": "state_change requires current target snapshot"}
			pkg["prerequisite"] = cloneStepPayload(contract.Prerequisite)
			pkg["repairObjective"] = repairObjectiveStateChangingOutput
			pkg["stateChangingOutputRequired"] = true
			pkg["requiresCurrentTargetSnapshotForStateChange"] = true
			pkg["targetSnapshotAcquisitionRequired"] = true
			required := evidenceAcquisitionRequiredOutputWithScratch(input, pkg, mapFromValue(pkg["requiredOutput"]))
			required["evidenceAcquisitionSubtype"] = "target_snapshot"
			required["parentNextActionKind"] = "state_change"
			required["prerequisiteForNextAction"] = "state_change"
			required["finiteExitRule"] = "accepted_target_snapshot_evidence_or_scoped_blocker_or_internal_fail_fast"
			applyTargetSnapshotAcquisitionRequiredOutputContract(required)
			required["forbiddenKinds"] = appendUniqueStrings(stringSliceFromValue(required["forbiddenKinds"]), "patch/apply", "file/write", "shell/write")
			pkg["requiredOutput"] = required
			sanitizeRepairActionPackageTargetEvidence(pkg)
			input["repairActionPackage"] = pkg
			currentActionSetRequiredOutput(input, required)
			break
		}
		required := mapFromValue(pkg["requiredOutput"])
		if !boolValue(required["mustIncludeStateChangingNode"]) {
			required = enforceStateChangingRequiredOutputForConvergence(pkg, input)
			addStrategyReassessmentRequiredOutputContract(pkg, required)
		}
		normalizeStateChangingCurrentActionPackage(input, pkg, required)
	case convergenceStateNeedValidation:
		setRuntimeStage(input, runtimeStageVerification)
		input[stepInputKeySoulStage] = soulStageValidation
		input["repairObjective"] = repairObjectiveConcreteValidationEvidence
		input["boundedValidationRequired"] = true
		delete(input, "replanStateChangingContinuation")
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
		required := concreteValidationEvidenceRequiredOutputWithScratch(input, pkg, mapFromValue(pkg["requiredOutput"]))
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
		if satisfied := mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"]); len(satisfied) > 0 {
			pkg["currentTargetAlreadySatisfiesFailedPatch"] = satisfied
			pkg["stateChangeAlreadySatisfiedByCurrentTarget"] = true
			required["forbiddenKinds"] = compactUniqueStrings(append(stringSliceFromValue(required["forbiddenKinds"]), "repeat_same_failed_patch"))
			pkg["forbiddenActions"] = repairActionForbiddenActions(input, pkg)
		}
		stripRepairActionPackageImplementationTargetEvidence(pkg)
		normalizeValidationCurrentActionPackage(input, pkg, required)
	case convergenceStateNeedAssessment:
		setRuntimeStage(input, runtimeStageVerification)
		input[stepInputKeySoulStage] = soulStageValidation
		input[stepInputKeyAssessmentOnly] = true
		delete(input, "replanStateChangingContinuation")
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
		delete(input, "replanStateChangingContinuation")
		delete(input, stepInputKeyAssessmentOnly)
		delete(input, "assessmentOnlyReason")
		delete(input, "runtimeRepairFollowUp")
		delete(input, stepInputKeyProviderRetryFollowUp)
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

func normalizeStateChangingCurrentActionPackage(input map[string]any, pkg map[string]any, required map[string]any) {
	if len(input) == 0 {
		return
	}
	if pkg == nil {
		pkg = map[string]any{}
	}
	pkg["repairObjective"] = repairObjectiveStateChangingOutput
	pkg["stateChangingOutputRequired"] = true
	for _, key := range []string{
		"evidenceAcquisitionRequired",
		"concreteFailureEvidenceRequired",
		"evidenceAcquisitionReason",
		"boundedValidationRequired",
		"evidenceRepairRequired",
		"outputDecisionContract",
		"exitCriteria",
	} {
		delete(pkg, key)
	}
	if len(required) == 0 {
		required = stateChangingRepairRequiredOutput()
	}
	pkg["requiredOutput"] = cloneStepPayload(required)
	input["repairActionPackage"] = pkg
	currentActionSetRequiredOutput(input, required)
}

func normalizeValidationCurrentActionPackage(input map[string]any, pkg map[string]any, required map[string]any) {
	if len(input) == 0 {
		return
	}
	if pkg == nil {
		pkg = map[string]any{}
	}
	pkg["repairObjective"] = repairObjectiveConcreteValidationEvidence
	pkg["boundedValidationRequired"] = true
	for _, key := range []string{
		"stateChangingOutputRequired",
		"evidenceAcquisitionRequired",
		"concreteFailureEvidenceRequired",
		"evidenceAcquisitionReason",
		"outputDecisionContract",
		"exitCriteria",
	} {
		delete(pkg, key)
	}
	if len(required) == 0 {
		required = concreteValidationEvidenceRequiredOutputWithScratch(input, pkg, nil)
	}
	pkg["requiredOutput"] = cloneStepPayload(required)
	input["repairActionPackage"] = pkg
	currentActionSetRequiredOutput(input, required)
}

func syncConvergenceNextActionWithContract(input map[string]any, contract ConvergenceContract) {
	if len(input) == 0 {
		return
	}
	action := convergenceNextActionForContract(contract)
	if len(action) == 0 {
		return
	}
	syncConvergenceNextActionMap(input, convergenceAuthorityKey, action)
	syncConvergenceNextActionMap(input, gapBaselineKey, action)
	if executionContext := mapFromValue(input["executionContext"]); len(executionContext) > 0 {
		syncConvergenceNextActionMap(executionContext, convergenceAuthorityKey, action)
		syncConvergenceNextActionMap(executionContext, gapBaselineKey, action)
		input["executionContext"] = executionContext
	}
}

func syncConvergenceNextActionMap(input map[string]any, key string, action map[string]any) {
	if len(input) == 0 || len(action) == 0 {
		return
	}
	source := mapFromValue(input[key])
	if len(source) == 0 {
		return
	}
	next := cloneStepPayload(source)
	next["nextAction"] = cloneStepPayload(action)
	input[key] = next
}

func convergenceNextActionForContract(contract ConvergenceContract) map[string]any {
	if contract.DeterministicComplete {
		return map[string]any{"kind": "complete", "reason": firstNonEmptyString(contract.CompletionReason, "current convergence contract is deterministically satisfied")}
	}
	switch contract.State {
	case convergenceStateNeedEvidence:
		kind := "acquire_evidence"
		reason := "current convergence contract requires bounded evidence before continuing"
		if contract.RuntimeStageMode == "target_snapshot_acquisition" {
			kind = "acquire_target_snapshot"
			reason = "current convergence contract requires target snapshot evidence before state change"
		}
		return map[string]any{"kind": kind, "reason": reason}
	case convergenceStateNeedStateChange:
		action := map[string]any{"kind": "state_change", "reason": "current convergence contract requires state-changing output"}
		if convergenceContractHasTargetSnapshotPrerequisite(contract) {
			action["reason"] = "state_change requires current target snapshot prerequisite"
			action["prerequisite"] = cloneStepPayload(contract.Prerequisite)
		}
		return action
	case convergenceStateNeedValidation:
		return map[string]any{"kind": "validate", "reason": "current convergence contract requires bounded validation evidence"}
	case convergenceStateNeedAssessment:
		return map[string]any{"kind": "assessment", "reason": "current convergence contract requires bounded coverage assessment"}
	case convergenceStateNeedGovernance:
		return map[string]any{"kind": "request_governance", "reason": firstNonEmptyString(contract.GovernanceReason, "current convergence contract requires governance")}
	default:
		return nil
	}
}

func convergenceContractHasTargetSnapshotPrerequisite(contract ConvergenceContract) bool {
	if strings.TrimSpace(stringFromMap(contract.Prerequisite, "kind")) == "acquire_target_snapshot" {
		return true
	}
	return contract.RuntimeStageMode == "target_snapshot_acquisition"
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
		"do not repeat assessment-only validation",
		"produce concrete verification evidence",
		"produce bounded shell/tool/mcp validation evidence",
		"unresolved goal/dod/deliverable coverage is currently a validation evidence gap",
		"do not return patch/apply unless a later validation failure",
		"bounded executiongraph that runs available verification",
	}, append(stateChangingRepairGuidance(),
		"When stateChangingOutputRequired is true, dodAssessment alone cannot satisfy the repair contract; first produce an accepted executable state-changing operation or return a real governance blocker.",
	)...)
}

func clearValidationOnlyConvergenceFields(input map[string]any) {
	if len(input) == 0 {
		return
	}
	for _, key := range []string{
		"concreteFailureEvidenceRequired",
		"boundedValidationRequired",
		"evidenceRepairRequired",
		"evidenceRepairSource",
		"verificationEvidenceGap",
	} {
		delete(input, key)
	}
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
	if decisionBlockDeclaresValidationCommandContractRetryExhausted(spec.Context) {
		if validationCommandContractRetryExhaustedCanContinue(input, spec.Context, contract) {
			return false
		}
		return true
	}
	if decisionBlockDeclaresRuntimeRepairExhausted(spec.Context) {
		return runtimeRepairExhaustedDecisionCanCreateTicket(input, spec.Context, contract)
	}
	if decisionBlockDeclaresContractViolationRecoveryExhausted(spec.Context) {
		return true
	}
	if decisionBlockDeclaresNonContractRuntimeDependency(spec.Context) {
		return true
	}
	if continuableNextActionKind(convergenceNextActionKind(nil, input)) {
		return false
	}
	if decisionBlockDeclaresRequiredRealExternalDependency(spec.Context) {
		return true
	}
	if decisionBlockDeclaresRuntimeContractDependency(spec.Context) && continuableDecisionContract(input, spec.Context, contract) {
		return false
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

func runtimeRepairExhaustedDecisionCanCreateTicket(input map[string]any, context map[string]any, contract ConvergenceContract) bool {
	if decisionBlockDeclaresNonContractRuntimeDependency(context) ||
		decisionBlockDeclaresRequiredRealExternalDependency(context) {
		return true
	}
	if continuableDecisionContract(input, context, contract) {
		return false
	}
	return !continuableNextActionKind(convergenceNextActionKind(nil, input))
}

func continuableDecisionContract(input map[string]any, context map[string]any, contract ConvergenceContract) bool {
	if decisionBlockDeclaresValidationCommandContractRetryExhausted(context) {
		return false
	}
	if continuableNextActionKind(convergenceNextActionKind(nil, input)) {
		return true
	}
	if continuableContractViolationRule(contractViolationRuleFromDecisionContext(context)) {
		return true
	}
	switch contract.State {
	case convergenceStateNeedEvidence:
		return mechanicalEvidenceAcquisitionContractCanContinue(input, contract)
	case convergenceStateNeedStateChange:
		return mechanicalStateChangingContractCanContinue(contract)
	case convergenceStateNeedValidation:
		return mechanicalConcreteValidationContractCanContinue(input, contract)
	default:
		return false
	}
}

func decisionBlockDeclaresValidationCommandContractRetryExhausted(context map[string]any) bool {
	if len(context) == 0 {
		return false
	}
	if boolValue(context["validationCommandContractRetryExhausted"]) {
		return true
	}
	if diagnostic := validationCommandContractFailureDiagnosticFromSources(
		context,
		mapFromValue(context["validationFailureDiagnostic"]),
		mapFromValue(context["runtimeDiagnostics"]),
		mapFromValue(context["failedStepInput"]),
		mapFromValue(context["failedStepOutput"]),
	); len(diagnostic) > 0 {
		switch strings.TrimSpace(stringFromMap(context, "blockerKind")) {
		case "validation_command_contract_retry_exhausted":
			return true
		default:
			return boolValue(mapFromValue(context["runtimeDiagnostics"])["validationCommandContractRetryExhausted"])
		}
	}
	return false
}

func validationCommandContractRetryExhaustedCanContinue(input map[string]any, context map[string]any, contract ConvergenceContract) bool {
	if contract.State != convergenceStateNeedValidation {
		return false
	}
	if !boolValue(input["managedValidationAvailable"]) &&
		!boolValue(context["managedValidationAvailable"]) &&
		!boolValue(mapFromValue(context["runtimeDiagnostics"])["managedValidationAvailable"]) {
		return false
	}
	pkg := mapFromValue(contract.RepairActionPackage)
	if len(pkg) == 0 {
		pkg = mapFromValue(input["repairActionPackage"])
	}
	required := mapFromValue(pkg["requiredOutput"])
	if !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), managedValidationSessionNodeKind) &&
		!boolValue(required["mustUseManagedHarness"]) {
		return false
	}
	text := strings.ToLower(strings.Join([]string{
		stringFromMap(context, "reason"),
		stringFromMap(context, "blockerKind"),
		stringFromMap(mapFromValue(context["runtimeDiagnostics"]), "reason"),
		stringFromMap(mapFromValue(context["runtimeDiagnostics"]), "error"),
		stringFromMap(mapFromValue(context["validationFailureDiagnostic"]), "class"),
		stringFromMap(mapFromValue(context["validationFailureDiagnostic"]), "reason"),
		stringFromMap(mapFromValue(context["contractViolation"]), "violatedRule"),
	}, "\n"))
	return strings.Contains(text, "validation_detached_process_forbidden") ||
		strings.Contains(text, "managed validation shell must wait") ||
		strings.Contains(text, "detached") ||
		strings.Contains(text, "background") ||
		strings.Contains(text, "process_control") ||
		strings.Contains(text, runtimemodel.ValidationDiagnosticManagedValidationRequired)
}

func decisionBlockDeclaresRuntimeRepairExhausted(context map[string]any) bool {
	if len(context) == 0 {
		return false
	}
	if boolValue(context["runtimeRepairExhausted"]) {
		return true
	}
	switch strings.TrimSpace(stringFromMap(context, "blockerKind")) {
	case "runtime_repair_contract_exhausted",
		"runtime_ineffective_convergence",
		"runtime_convergence_budget_exhausted":
		return true
	default:
		return false
	}
}

func decisionBlockDeclaresContractViolationRecoveryExhausted(context map[string]any) bool {
	if len(context) == 0 {
		return false
	}
	return strings.TrimSpace(stringFromMap(context, "runtimeRepairLoopState")) == "contract_violation_recovery_exhausted"
}

func continuableNextActionKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "assess_gap", "repair", "acquire_target_snapshot", "state_change", "produce_state_change",
		"validate", "validation", "produce_validation_evidence",
		"assessment", "complete_coverage_assessment",
		"artifact", "produce_artifact_evidence":
		return true
	default:
		return false
	}
}

func contractViolationRuleFromDecisionContext(context map[string]any) string {
	for _, source := range []map[string]any{
		context,
		mapFromValue(context["contractViolation"]),
		mapFromValue(context["correctionContract"]),
		mapFromValue(context["knownValues"]),
	} {
		if len(source) == 0 {
			continue
		}
		for _, key := range []string{"violatedRule", "rule", "contractViolationRule"} {
			if value := strings.TrimSpace(stringFromMap(source, key)); value != "" {
				return value
			}
		}
	}
	return ""
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
	if boolValue(input["validationCommandContractRetryExhausted"]) {
		return false
	}
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
	if boolValue(input[stepInputKeyAssessmentOnly]) {
		return !coverageAssessmentOnlyFailureHasActionableGap(failedStep)
	}
	if boolValue(input["assessmentOnlyContractCorrection"]) ||
		boolValue(input[stepInputKeyGraphLimitFollowUp]) {
		return !coverageAssessmentOnlyFailureHasActionableGap(failedStep)
	}
	return doDCoverageContractCorrectionRequired(failedStep) && !coverageCorrectionNeedsConcreteEvidence(failedStep)
}

func coverageAssessmentOnlyFailureHasActionableGap(failedStep *runModel.Step) bool {
	if !assessmentOnlyCoverageCorrectionFailedWithActionableGaps(failedStep) {
		return false
	}
	return len(missingDoDItemsFromStep(failedStep)) > 0 ||
		len(missingGoalCoverageFromStep(failedStep)) > 0 ||
		len(missingDeliverableCoverageFromStep(failedStep)) > 0
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
		return !convergenceConcreteFailureTargetsOutsideSatisfiedStateChange(input, mapFromValue(input["repairActionPackage"]))
	}
	if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
		if len(mapSliceFromValue(pkg["currentTargetAlreadySatisfiesFailedPatch"])) > 0 ||
			boolValue(pkg["stateChangeAlreadySatisfiedByCurrentTarget"]) {
			return !convergenceConcreteFailureTargetsOutsideSatisfiedStateChange(input, pkg)
		}
		required := mapFromValue(pkg["requiredOutput"])
		return boolValue(required["mustProduceConcreteValidationEvidence"])
	}
	return false
}

func explicitCoverageGapPresent(input map[string]any, failedStep *runModel.Step) bool {
	if len(mapSliceFromValue(input["missingDoDItems"])) > 0 ||
		len(missingGoalCoverageFromInput(input)) > 0 ||
		len(mapSliceFromValue(input["missingDeliverableCoverage"])) > 0 {
		return true
	}
	if failedStep == nil {
		return false
	}
	return len(missingDoDItemsFromFailureOutput(failedStep.Output)) > 0 ||
		len(missingGoalCoverageFromStep(failedStep)) > 0 ||
		len(missingDeliverableCoverageFromStep(failedStep)) > 0
}

func coverageFailureStepHasExplicitCoverageGap(failedStep *runModel.Step) bool {
	if failedStep == nil {
		return false
	}
	return len(missingDoDItemsFromStep(failedStep)) > 0 ||
		len(missingGoalCoverageFromStep(failedStep)) > 0 ||
		len(missingDeliverableCoverageFromStep(failedStep)) > 0
}

func explicitCoverageGapShouldPreemptAssessment(input map[string]any, failedStep *runModel.Step) bool {
	if !explicitCoverageGapPresent(input, failedStep) {
		return false
	}
	if explicitCoverageGapsClosedByWorkItems(input, failedStep) {
		return false
	}
	if failedStep == nil {
		return true
	}
	if assessmentOutputContractRepairRequired(failedStep) {
		return false
	}
	assessment := doDAssessmentFromOutput(failedStep.Output)
	if satisfied, ok := assessment["satisfied"].(bool); ok && satisfied {
		return false
	}
	return true
}

func explicitCoverageGapsClosedByWorkItems(input map[string]any, failedStep *runModel.Step) bool {
	gaps := explicitCoverageGapItems(input, failedStep)
	if len(gaps) == 0 {
		return false
	}
	context := coverageWorkItemContextFromInput(input)
	if len(context) == 0 {
		return false
	}
	for _, gap := range gaps {
		if !coverageGapClosedByWorkItemContext(context, gap) {
			return false
		}
	}
	return true
}

func explicitCoverageGapItems(input map[string]any, failedStep *runModel.Step) []map[string]any {
	ret := make([]map[string]any, 0)
	ret = append(ret, mapSliceFromValue(input["missingDoDItems"])...)
	ret = append(ret, missingGoalCoverageFromInput(input)...)
	ret = append(ret, mapSliceFromValue(input["missingDeliverableCoverage"])...)
	if failedStep != nil {
		ret = append(ret, missingDoDItemsFromStep(failedStep)...)
		ret = append(ret, missingGoalCoverageFromStep(failedStep)...)
		ret = append(ret, missingDeliverableCoverageFromStep(failedStep)...)
	}
	return ret
}

func filterCoverageGapsOpenByWorkItems(input map[string]any, gaps []map[string]any) []map[string]any {
	if len(gaps) == 0 {
		return nil
	}
	context := coverageWorkItemContextFromInput(input)
	if len(context) == 0 {
		return cloneMapSlice(gaps)
	}
	ret := make([]map[string]any, 0, len(gaps))
	for _, gap := range gaps {
		if coverageGapClosedByWorkItemContext(context, gap) {
			continue
		}
		ret = append(ret, cloneStepPayload(gap))
	}
	return ret
}

func coverageGapClosedByWorkItemContext(context map[string]any, gap map[string]any) bool {
	if len(context) == 0 || len(gap) == 0 {
		return false
	}
	for _, item := range coverageWorkItemContextItems(context) {
		if coverageWorkItemResolvedForExplicitGap(item) && coverageGapMatchesWorkItem(gap, item) {
			return true
		}
	}
	return false
}

func coverageWorkItemContextItems(context map[string]any) []map[string]any {
	ret := make([]map[string]any, 0)
	if item := mapFromValue(context["currentFocus"]); len(item) > 0 {
		ret = append(ret, item)
	}
	for _, key := range []string{"currentFocusItems", "unresolvedItems", "ledgerSummary", "historicalItems"} {
		ret = append(ret, mapSliceFromValue(context[key])...)
	}
	return ret
}

func coverageWorkItemResolvedForExplicitGap(item map[string]any) bool {
	if len(item) == 0 || deriveCoverageWorkItemState(item).HasActionableEvidenceGap() {
		return false
	}
	switch strings.TrimSpace(stringFromMap(item, "status")) {
	case "satisfied":
		return true
	case "evidence_ready":
		return coverageWorkItemFocusReadyForAssessment(item) ||
			len(stringSliceFromValue(item["acceptedEvidenceRefs"])) > 0
	default:
		return false
	}
}

func coverageGapMatchesWorkItem(gap map[string]any, item map[string]any) bool {
	gapScope := coverageGapScope(gap)
	itemScope := coverageGapScope(item)
	if knownEvidenceKindRequirementScope(gapScope) && knownEvidenceKindRequirementScope(itemScope) && gapScope != itemScope {
		return false
	}
	gapAliases := evidenceKindRequirementItemAliases(gap)
	itemAliases := evidenceKindRequirementItemAliases(item)
	for _, gapAlias := range gapAliases {
		for _, itemAlias := range itemAliases {
			if strings.EqualFold(strings.TrimSpace(gapAlias), strings.TrimSpace(itemAlias)) {
				return true
			}
		}
	}
	return false
}

func coverageGapScope(item map[string]any) string {
	for _, key := range []string{"sourceScope", "kind", "source"} {
		if scope := normalizeEvidenceKindRequirementScope(stringFromMap(item, key)); knownEvidenceKindRequirementScope(scope) {
			return scope
		}
	}
	return ""
}

func stateChangingEvidenceRequirementRepairActionPackage(run *runModel.Run, input map[string]any) map[string]any {
	if run == nil || len(input) == 0 {
		return nil
	}
	if boolValue(mapFromValue(input["repairActionPackage"])["capabilityPolicyRepairRequired"]) {
		return nil
	}
	refreshRepairStrategyReassessment(input)
	targets := mapSliceFromValue(input["coverageTargetSet"])
	if len(targets) == 0 {
		targets = coverageTargetSetFromRepairInput(input)
	}
	if len(targets) == 0 {
		targets = coverageTargetSetFromWorkItemContext(coverageWorkItemContextFromInput(input))
	}
	if len(targets) == 0 {
		return nil
	}
	targets = coverageTargetsMissingProfileStateChangeEvidence(targets)
	if len(targets) == 0 {
		return nil
	}
	requirements := coverageTargetEvidenceKindRequirementsFromInput(run, input, targets)
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
			"evidenceRole":       requirement.EvidenceRole,
			"acceptedStepKinds":  compactUniqueStrings(requirement.AcceptedStepKinds),
			"acceptedOperations": compactUniqueStrings(requirement.AcceptedOperations),
			"requireDirect":      requirement.RequireDirect,
			"description":        requirement.Description,
		})
	}
	if !requiresStateChange {
		return nil
	}
	if gap := profileEvidenceRuntimeCapabilityGap(matchedRequirements); len(gap) > 0 {
		pkg := cloneStepPayload(mapFromValue(input["repairActionPackage"]))
		if len(pkg) == 0 {
			pkg = map[string]any{}
		}
		pkg["coverageRepairRequired"] = true
		pkg["coverageTargetSet"] = targets
		pkg["evidenceKindRequirements"] = requirementSummaries
		pkg["runtimeCapabilityGap"] = gap
		pkg["requiredOutput"] = map[string]any{
			"mustNotUseUnavailableKinds":     true,
			"requiredMissingKinds":           gap["missingKinds"],
			"availableKinds":                 gap["availableKinds"],
			"profileEvidenceRequirementRule": "profile evidence requirements are authoritative; if no acceptedStepKinds are declared by runtimeCapabilities.blockKinds, do not substitute unrelated block kinds and request capability/configuration resolution",
		}
		pkg["exitCriteria"] = []string{
			"runtime_capability_enabled_then_profile_required_evidence_produced",
			"evidence_requirement_policy_changed_then_reassess",
		}
		return pkg
	}
	primaryStateChangeRequired := coverageTargetsRequirePrimaryStateChangingOutput(targets, matchedRequirements)
	pkg := cloneStepPayload(mapFromValue(input["repairActionPackage"]))
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	clearEvidenceAcquisitionStateForStateChange(pkg)
	pkg["coverageRepairRequired"] = true
	pkg["coverageTargetSet"] = targets
	pkg["evidenceKindRequirements"] = requirementSummaries
	if progress := mapFromValue(input["currentExecutionProgress"]); len(progress) > 0 {
		pkg["currentExecutionProgress"] = compactLLMContextMap(progress, 0)
	}
	required := stateChangingRepairRequiredOutput()
	for key, value := range mapFromValue(pkg["requiredOutput"]) {
		switch key {
		case "mustReturnValidJSON",
			"mustNotReuseRejectedJSON",
			"forbiddenStalePatchReuse",
			"mustDeclareStrategyReassessmentFields",
			"strategyReassessmentRequiredFields",
			"strategyReassessmentRepairModes",
			"strategyReassessmentMatchRule",
			"mustReassessRepeatedFailedSurface",
			"sameTargetContinuationRule",
			"rootCauseTargetShiftRule":
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
		if !stateChangingRepairHasActionableImplementationSurface(input, pkg) {
			if !profilePrimaryStateChangeCanProceedFromCoverageTargets(input, pkg, targets) {
				return profileStateChangingEvidenceAcquisitionRepairActionPackage(input, pkg, required)
			}
			pkg["primaryStateChangeWithoutTargetEvidence"] = true
			pkg["primaryStateChangeCoverageScoped"] = true
			required["mustReferenceCoverageWorkItem"] = true
			required["targetRelation"] = "coverage_target_scoped_greenfield_or_expansion_state_change"
			required["coverageScopedTargetRule"] = "when no current target snapshot is available, patch/apply may create or extend cohesive deliverable resources only when it declares coverageTargets matching coverageTargetSet and does not claim to patch an existing unsafe snapshot"
		} else if boolValue(input["emptyWorkspaceEvidenceEscalatedToImplementation"]) {
			pkg["primaryStateChangeWithoutTargetEvidence"] = true
			required["mustReferenceCoverageWorkItem"] = true
		}
		required["mustIncludePrimaryStateChangingNode"] = true
		required["primaryAcceptedKinds"] = stateChangingRepairAcceptedKinds()
		required["primaryStateChangingRule"] = "when unresolved coverage targets include general deliverable, goal, or DoD state change beyond profile-specific write evidence, include a primary state-changing artifact before any supplementary evidence-only write such as snapshot/commit"
	}
	addStrategyReassessmentRequiredOutputContract(pkg, required)
	pkg["requiredOutput"] = required
	pkg["exitCriteria"] = []string{
		"profile_required_state_changing_evidence_produced_then_verify",
		"failed_assessment_or_blocker_when_required_evidence_cannot_be_produced",
	}
	return pkg
}

func profilePrimaryStateChangeCanProceedFromCoverageTargets(input map[string]any, pkg map[string]any, targets []map[string]any) bool {
	if len(targets) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromMap(input, "repairObjective")) != repairObjectiveAcquireFailureEvidence ||
		!boolValue(input["concreteFailureEvidenceRequired"]) {
		return false
	}
	if repairContextHasCapabilityInputLocatorInvalid(input, pkg) {
		return false
	}
	if boolValue(input["requiresCurrentTargetSnapshotForStateChange"]) ||
		boolValue(pkg["requiresCurrentTargetSnapshotForStateChange"]) {
		return false
	}
	if !coverageInputCurrentFocusAllowsPrimaryStateChange(input) {
		return false
	}
	for _, target := range targets {
		if coverageTargetClosedForPrimaryStateChange(target) {
			continue
		}
		for _, key := range []string{"coverageTargetToken", "goalRequirementId", "sourceRequirementId", "targetText", "item", "raw", "requirement"} {
			if strings.TrimSpace(stringFromMap(target, key)) != "" {
				return true
			}
		}
	}
	return false
}

func coverageInputCurrentFocusAllowsPrimaryStateChange(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	for _, focus := range []map[string]any{
		mapFromValue(input[coverageFocusKey]),
		mapFromValue(mapFromValue(input[coverageWorkItemContextKey])["currentFocus"]),
		mapFromValue(mapFromValue(mapFromValue(input["repairActionPackage"])[coverageWorkItemContextKey])["currentFocus"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])[coverageFocusKey]),
	} {
		if !coverageFocusAllowsPrimaryStateChange(focus) {
			continue
		}
		return true
	}
	return false
}

func coverageFocusAllowsPrimaryStateChange(focus map[string]any) bool {
	if len(focus) == 0 {
		return false
	}
	switch strings.TrimSpace(stringFromMap(focus, "state")) {
	case "satisfied", "evidence_ready":
		return false
	}
	switch strings.TrimSpace(stringFromMap(focus, "status")) {
	case "satisfied", "evidence_ready":
		return false
	}
	if normalizedCoverageEvidenceRole(stringFromMap(focus, "evidenceRole")) == "primary_state_change" {
		return true
	}
	for _, key := range []string{"id", "sliceId", "sourceRequirementId", "targetText", "coverageTargetToken", "sourcePath"} {
		if coverageFocusTokenAllowsPrimaryStateChange(stringFromMap(focus, key)) {
			return true
		}
	}
	for _, key := range []string{"targetRefs", "sourceRequirementAliases", "coverageTargets"} {
		for _, token := range stringSliceFromValue(focus[key]) {
			if coverageFocusTokenAllowsPrimaryStateChange(token) {
				return true
			}
		}
	}
	return false
}

func coverageFocusTokenAllowsPrimaryStateChange(token string) bool {
	token = normalizeCoverageGapRepairToken(token)
	compact := strings.NewReplacer("_", "", "-", "", " ", "").Replace(token)
	return strings.Contains(compact, "primarystatechange")
}

func coverageTargetsMissingProfileStateChangeEvidence(targets []map[string]any) []map[string]any {
	if len(targets) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		if coverageTargetProfileStateChangeEvidenceSatisfied(target) {
			continue
		}
		ret = append(ret, target)
	}
	return ret
}

func rejectedContinuationSchemaInvalidValidationFirst(run *runModel.Run, input map[string]any, failedStep *runModel.Step) bool {
	if failedStep == nil {
		return false
	}
	class := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(failedStep.Output, failureOutputKeyClass),
		stringFromMap(mapFromValue(failedStep.Output["runtimeDiagnostics"]), failureOutputKeyClass),
	))
	if class != failureClassLLMDecisionInvalid {
		return false
	}
	if len(mapFromValue(input["rejectedResultContinuation"])) > 0 ||
		len(mapFromValue(mapFromValue(input["executionContext"])["rejectedResultContinuation"])) > 0 {
		return true
	}
	return run != nil && len(mapFromValue(run.ExecutionContext["rejectedResultContinuation"])) > 0
}

func coverageTargetProfileStateChangeEvidenceSatisfied(target map[string]any) bool {
	if len(target) == 0 {
		return false
	}
	state := deriveCoverageWorkItemState(target)
	missingKinds := stringSet(state.MissingEvidenceKinds)
	if coverageWorkItemAcceptedEvidenceCountForKind(target, "state_change", state.RequiredEvidenceKinds) == 0 {
		return !missingKinds["state_change"] && len(stringSliceFromValue(target["acceptedEvidenceRefs"])) > 0
	}
	return !missingKinds["state_change"]
}

func profileEvidenceRuntimeCapabilityGap(requirements []profileEvidenceKindRequirement) map[string]any {
	if len(requirements) == 0 {
		return nil
	}
	available := currentRuntimeEvidenceBlockKinds()
	missing := make([]string, 0)
	requirementSummaries := make([]map[string]any, 0)
	for _, requirement := range requirements {
		if len(requirement.AcceptedStepKinds) == 0 {
			continue
		}
		hasAvailable := false
		for _, kind := range requirement.AcceptedStepKinds {
			if available[strings.TrimSpace(kind)] {
				hasAvailable = true
				break
			}
		}
		if hasAvailable {
			continue
		}
		missing = append(missing, requirement.AcceptedStepKinds...)
		requirementSummaries = append(requirementSummaries, map[string]any{
			"scope":              normalizeEvidenceKindRequirementScope(requirement.Scope),
			"id":                 requirement.ID,
			"evidenceRole":       requirement.EvidenceRole,
			"acceptedStepKinds":  compactUniqueStrings(requirement.AcceptedStepKinds),
			"acceptedOperations": compactUniqueStrings(requirement.AcceptedOperations),
			"description":        requirement.Description,
		})
	}
	missing = compactUniqueStrings(missing)
	if len(missing) == 0 {
		return nil
	}
	return map[string]any{
		"kind":           "runtime_capability_gap",
		"missingKinds":   missing,
		"availableKinds": compactUniqueStrings(mapKeys(available)),
		"requirements":   requirementSummaries,
		"reason":         "profile evidence requirement accepts only block kinds that are not declared by current runtime capabilities",
	}
}

func currentRuntimeEvidenceBlockKinds() map[string]bool {
	return runtimeEvidenceBlockKindCatalog()
}

func runtimeEvidenceBlockKindCatalog() map[string]bool {
	return map[string]bool{
		"llm/workspace_execution":        true,
		"shell/command":                  true,
		"file/list":                      true,
		"file/read":                      true,
		"file/edit":                      true,
		"ssh/command":                    true,
		"web/search":                     true,
		"web/fetch":                      true,
		"patch/apply":                    true,
		"patch/preview":                  true,
		"patch/revert":                   true,
		"mcp/tool_call":                  true,
		"mcp/resource_read":              true,
		"mcp/prompt_get":                 true,
		managedValidationSessionNodeKind: true,
		"git/read":                       true,
		"git/write":                      true,
	}
}

func mapKeys(input map[string]bool) []string {
	ret := make([]string, 0, len(input))
	for key, enabled := range input {
		if enabled {
			ret = append(ret, key)
		}
	}
	return ret
}

func profileStateChangingEvidenceAcquisitionRepairActionPackage(input map[string]any, pkg map[string]any, previousRequired map[string]any) map[string]any {
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	required := evidenceAcquisitionRequiredOutputWithScratch(input, pkg, mapFromValue(previousRequired))
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
			applyTargetSnapshotSafetyRequiredOutputContract(required, merged)
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
		if coverageTargetClosedForPrimaryStateChange(target) {
			continue
		}
		matched := false
		for _, requirement := range filtered {
			if evidenceKindRequirementMatchesCoverageTarget(requirement, target) {
				matched = true
				if normalizedCoverageEvidenceRole(requirement.EvidenceRole) == "primary_state_change" {
					return true
				}
				break
			}
		}
		if !matched {
			return true
		}
	}
	return false
}

func coverageTargetClosedForPrimaryStateChange(target map[string]any) bool {
	if len(target) == 0 {
		return true
	}
	derived := deriveCoverageWorkItemState(target)
	missingKinds := stringSet(derived.MissingEvidenceKinds)
	status := strings.TrimSpace(firstNonEmptyString(stringFromMap(target, "state"), stringFromMap(target, "status")))
	if status != "satisfied" && status != "evidence_ready" {
		return false
	}
	return !missingKinds["state_change"]
}

func coverageTargetEvidenceKindRequirements(run *runModel.Run, targets []map[string]any) []profileEvidenceKindRequirement {
	return coverageTargetEvidenceKindRequirementsFromList(profileEvidenceKindRequirements(run), targets)
}

func coverageTargetEvidenceKindRequirementsFromInput(run *runModel.Run, input map[string]any, targets []map[string]any) []profileEvidenceKindRequirement {
	requirements := profileEvidenceKindRequirements(run)
	if len(requirements) == 0 {
		requirements = profileEvidenceKindRequirementsFromInput(input)
	}
	return coverageTargetEvidenceKindRequirementsFromList(requirements, targets)
}

func profileEvidenceKindRequirementsFromInput(input map[string]any) []profileEvidenceKindRequirement {
	if len(input) == 0 {
		return nil
	}
	if requirements := profileEvidenceKindRequirementsFromExecutionContext(input); len(requirements) > 0 {
		return requirements
	}
	for _, key := range []string{"executionContextPackage", "executionContext"} {
		if requirements := profileEvidenceKindRequirementsFromExecutionContext(mapFromValue(input[key])); len(requirements) > 0 {
			return requirements
		}
	}
	return nil
}

func coverageTargetEvidenceKindRequirementsFromList(requirements []profileEvidenceKindRequirement, targets []map[string]any) []profileEvidenceKindRequirement {
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
	if convergenceCapabilityInputLocatorNeedsEvidence(input, failedStep) {
		return true
	}
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

func convergenceCapabilityInputLocatorNeedsEvidence(input map[string]any, failedStep *runModel.Step) bool {
	return capabilityInputLocatorInvalidFailure(failedStep) || inputHasCapabilityInputLocatorInvalid(input)
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
	if failedConcreteValidationAssessmentOutput(failedStep.Output) && convergenceCoverageFocusRequiresValidation(input) {
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
	if runtimeFailureIsLLMContractLoop(failedStep) && validationContractLoopNeedsConcreteTargetEvidence(failedStep) {
		return true
	}
	return false
}

func validationContractLoopNeedsConcreteTargetEvidence(failedStep *runModel.Step) bool {
	if failedStep == nil {
		return false
	}
	text := strings.ToLower(strings.Join([]string{
		stringFromMap(failedStep.Output, "error"),
		stringFromMap(failedStep.Output, "reason"),
		stringFromMap(failedStep.Output, failureOutputKeySignature),
	}, "\n"))
	return strings.Contains(text, "validation follow-up llm step returned read-only executiongraph without concrete validation evidence") ||
		strings.Contains(text, "concrete validation evidence repair must return bounded")
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
	ret := map[string]any{}
	if contract.State != "" {
		ret["state"] = contract.State
	}
	if contract.RuntimeStage != "" {
		ret["runtimeStage"] = contract.RuntimeStage
	}
	if contract.RuntimeStageMode != "" {
		ret["runtimeStageMode"] = contract.RuntimeStageMode
	}
	if contract.DeterministicComplete {
		ret["deterministicComplete"] = true
		if contract.CompletionReason != "" {
			ret["completionReason"] = contract.CompletionReason
		}
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
	if len(contract.Prerequisite) > 0 {
		ret["prerequisite"] = cloneStepPayload(contract.Prerequisite)
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

func convergenceContractShouldPersist(contract ConvergenceContract) bool {
	if contract.State == "" {
		return false
	}
	if contract.State == convergenceStateBlocked && strings.TrimSpace(contract.GovernanceReason) == "" {
		return false
	}
	return true
}
