package orchestrator

import (
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func validateLLMExecutionOutputContract(run *runModel.Run, step *runModel.Step, output map[string]any) error {
	if run == nil || step == nil || output == nil || step.Kind != initialStepKind(run) {
		return nil
	}
	stage := runtimeStageFromStepInput(step.Input)
	if err := validateIncrementalArtifactOutputContract(run, step, output); err != nil {
		return err
	}
	if err := validateLLMExecutionGraphOutputContract(step, output); err != nil {
		return err
	}
	if err := validateLLMNextStepPatchTargetRelevance(step, output); err != nil {
		return err
	}
	if err := validatePrimaryDiagnosticPatchOutputContract(step, output); err != nil {
		return err
	}
	if err := validateLLMSoulGraphCapabilityContract(run, step, output); err != nil {
		return err
	}
	if err := validateLLMCapabilityUsagePolicyContract(run, step, output); err != nil {
		return err
	}
	if err := validateLLMFollowUpGraphStateChangingContract(run, step, output); err != nil {
		return err
	}
	if err := validateLLMCoverageBoundStateChangingIntent(run, step, output); err != nil {
		return err
	}
	if err := validateLLMRepairStrategyReassessmentContract(step, output); err != nil {
		return err
	}
	if err := validateScopedStateChangingRepairVerificationContract(step, output); err != nil {
		return err
	}
	if err := validateRuntimeRepairFailureDiagnosticsContract(step); err != nil {
		return err
	}
	if err := validateConcreteValidationEvidenceOutputContract(step, output); err != nil {
		return err
	}
	if boolValue(step.Input["stateChangingOutputRequired"]) {
		if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
			if outputModeIsIncrementalArtifact(step.Input) && artifactOutputPhaseFromInput(step.Input) == artifactOutputPhaseContext {
				return nil
			}
			if allowPatchRepairValidationEvidenceGraph(step, graph) {
				return nil
			}
			if allowDeclaredToolStateChangingRepairGraph(step, graph) {
				return nil
			}
			if allowAcceptedStateChangingEvidenceGraph(step, graph) {
				return nil
			}
			if err := validateStateChangingExecutionGraph(graph); err != nil {
				return err
			}
			return nil
		}
		if hasStateChangingLLMExecutionOutput(output) {
			return nil
		}
		if outputModeIsIncrementalArtifact(step.Input) &&
			(artifactOutputFieldPresent(output, "artifactPlan") ||
				artifactOutputFieldPresent(output, "artifactChunk") ||
				artifactOutputFieldPresent(output, "artifactChunkRepair")) {
			return nil
		}
		if assessment := doDAssessmentFromOutput(output); assessment != nil {
			if boolValue(step.Input[stepInputKeyProviderRetryFollowUp]) && boolValue(assessment["satisfied"]) {
				return nil
			}
			return fmt.Errorf("%w: state-changing llm repair step returned dodAssessment without executable state-changing output", common.ErrInvalidArgument)
		}
		if spec := decisionBlockSpec(output); spec.Reason != "" {
			return nil
		}
		if spec := approvalBlockSpec(output); spec.Reason != "" {
			return nil
		}
		return fmt.Errorf("%w: state-changing llm repair step returned no state-changing output", common.ErrInvalidArgument)
	}
	if graphNodeID(step) == "" {
		return nil
	}
	if stage != runtimeStageExecution {
		return nil
	}
	if llmExecutionGraphNodeReturnedOnlyGraph(run, step, output) {
		return fmt.Errorf("%w: execution-stage llm graph node returned executionGraph without concrete output", common.ErrInvalidArgument)
	}
	if !graphNodeRequiresStateChangingOutput(step) {
		return nil
	}
	if hasStateChangingLLMExecutionOutput(output) || doDAssessmentFromOutput(output) != nil {
		return nil
	}
	if spec := decisionBlockSpec(output); spec.Reason != "" {
		return nil
	}
	if spec := approvalBlockSpec(output); spec.Reason != "" {
		return nil
	}
	return fmt.Errorf("%w: state-changing execution-stage llm graph node returned no state-changing output", common.ErrInvalidArgument)
}

func validateConcreteValidationEvidenceOutputContract(step *runModel.Step, output map[string]any) error {
	if step == nil || step.Input == nil || output == nil || !inputRequiresConcreteValidationEvidence(step.Input) {
		return nil
	}
	if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
		if !graphIsConcreteValidationEvidenceGraph(graph) {
			return fmt.Errorf("%w: concrete validation evidence repair must return bounded shell/command or mcp/tool_call executionGraph or a satisfied dodAssessment with concrete evidenceRefs", common.ErrInvalidArgument)
		}
		if err := validateConcreteValidationBehaviorEvidenceGraph(step, graph); err != nil {
			return err
		}
		return nil
	}
	if spec := decisionBlockSpec(output); spec.Reason != "" {
		return nil
	}
	if spec := approvalBlockSpec(output); spec.Reason != "" {
		return nil
	}
	if assessment := doDAssessmentFromOutput(output); assessment != nil {
		if boolValue(assessment["satisfied"]) {
			return nil
		}
		return fmt.Errorf("%w: concrete validation evidence repair must return bounded shell/command or mcp/tool_call executionGraph; plain failed dodAssessment does not satisfy repairActionPackage.requiredOutput", common.ErrInvalidArgument)
	}
	return fmt.Errorf("%w: concrete validation evidence repair must return bounded shell/command or mcp/tool_call executionGraph or a satisfied dodAssessment with concrete evidenceRefs", common.ErrInvalidArgument)
}

func validateConcreteValidationBehaviorEvidenceGraph(step *runModel.Step, graph runtimemodel.BlockExecutionGraph) error {
	if step == nil || step.Input == nil {
		return nil
	}
	required := mapFromValue(mapFromValue(step.Input["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustPreferExecutableBehaviorValidation"]) &&
		!boolValue(required["staticInspectionOnlyMayNotSatisfyRuntimeBehavior"]) &&
		!boolValue(required["mustDeclareValidationEvidenceMode"]) &&
		!boolValue(required["mustDeclareValidationEvidenceObservation"]) {
		return nil
	}
	if graphHasDeclaredConcreteValidationEvidenceNode(graph) {
		return nil
	}
	return fmt.Errorf("%w: concrete validation evidence repair returned executionGraph without a declared non-static validationEvidence contract; shell/command or mcp/tool_call nodes must declare validationEvidence.mode and validationEvidence.observationType, and controlled_substitute must declare validationEvidence.substituteFor or validationEvidence.substitute", common.ErrInvalidArgument)
}

func graphHasDeclaredConcreteValidationEvidenceNode(graph runtimemodel.BlockExecutionGraph) bool {
	for _, node := range graph.Nodes {
		switch strings.TrimSpace(node.Kind) {
		case "shell/command", "mcp/tool_call":
			if nodeValidationEvidenceContractIsConcrete(node) {
				return true
			}
		}
	}
	return false
}

func nodeValidationEvidenceContractIsConcrete(node runtimemodel.BlockExecutionNode) bool {
	evidence := mapFromValue(node.Input["validationEvidence"])
	mode := strings.TrimSpace(stringFromMap(evidence, "mode"))
	if mode := strings.TrimSpace(stringFromMap(node.Input, "validationEvidenceMode")); mode != "" {
		evidence["mode"] = mode
	}
	if mode == "" {
		mode = strings.TrimSpace(stringFromMap(node.Input, "evidenceMode"))
	}
	if !validationEvidenceModeIsConcrete(mode) {
		return false
	}
	observationType := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(evidence, "observationType"),
		stringFromMap(evidence, "observation"),
		stringFromMap(node.Input, "validationObservationType"),
	))
	if !validationEvidenceObservationTypeIsConcrete(observationType) {
		return false
	}
	if mode == "controlled_substitute" &&
		len(mapFromValue(evidence["substitute"])) == 0 &&
		strings.TrimSpace(stringFromMap(evidence, "substituteFor")) == "" {
		return false
	}
	return true
}

func validationEvidenceModeIsConcrete(mode string) bool {
	switch strings.TrimSpace(mode) {
	case "runtime_behavior", "runtime_observation", "controlled_substitute", "controlled_substitute_observation", "executable_contract", "executable_contract_observation", "dynamic_execution", "dynamic_execution_observation":
		return true
	default:
		return false
	}
}

func validationEvidenceObservationTypeIsConcrete(observationType string) bool {
	switch strings.TrimSpace(observationType) {
	case "runtime_observation", "controlled_substitute_observation", "executable_contract_observation", "dynamic_execution_observation":
		return true
	default:
		return false
	}
}

func validateRuntimeStateChangingOutputBeforeSideEffects(run *runModel.Run, step *runModel.Step, output map[string]any) error {
	if run == nil || step == nil || output == nil {
		return nil
	}
	if step.Kind == initialStepKind(run) {
		return validateLLMExecutionOutputContract(run, step, output)
	}
	if strings.TrimSpace(step.Kind) != "patch/apply" {
		return nil
	}
	return validatePatchStepCoverageTargetsBeforeRuntime(run, step, output)
}

func validateScopedStateChangingRepairVerificationContract(step *runModel.Step, output map[string]any) error {
	if step == nil || step.Input == nil || output == nil {
		return nil
	}
	if !stateChangingRepairContractActive(step) {
		return nil
	}
	if runtimeStageFromStepInput(step.Input) != runtimeStageRepair {
		return nil
	}
	if !strategyFailedSurfaceRequiresBoundedVerification(
		strings.TrimSpace(stringFromMap(step.Input, "failureTargetScope")),
		mapFromValue(step.Input["strategyReassessment"]),
	) {
		return nil
	}
	if !strategyOutputGraphUsesUnboundedVerificationForStep(step, output) {
		return nil
	}
	return fmt.Errorf("%w: scoped state-changing runtime repair requires bounded verification; unbounded verification without explicit target scope is not allowed", common.ErrInvalidArgument)
}

func validateRuntimeRepairFailureDiagnosticsContract(step *runModel.Step) error {
	if step == nil || step.Input == nil {
		return nil
	}
	if !stateChangingRepairContractActive(step) {
		return nil
	}
	if strings.TrimSpace(stringFromMap(step.Input, "failedStepId")) == "" &&
		strings.TrimSpace(stringFromMap(step.Input, stepInputKeyRepairOfStepID)) == "" {
		return nil
	}
	diagnostics := runtimeRepairFailureDiagnosticsFromInput(step.Input)
	if strings.TrimSpace(firstNonEmptyString(
		stringFromMap(diagnostics, "failureSignature"),
		stringFromMap(diagnostics, "failureClass"),
		stringFromMap(diagnostics, "error"),
	)) == "" {
		return fmt.Errorf("%w: state-changing runtime repair requires lastFailureDiagnostics or equivalent failedStepOutput diagnostics", common.ErrInvalidArgument)
	}
	return nil
}

func runtimeRepairFailureDiagnosticsFromInput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	for _, candidate := range []map[string]any{
		mapFromValue(input["lastFailureDiagnostics"]),
		mapFromValue(input["failedStepOutput"]),
		mapFromValue(input["currentFailure"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["currentFailure"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["lastFailureDiagnostics"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["lastObservedFailure"]),
		mapFromValue(mapFromValue(input["currentProgressDigest"])["lastObservedFailure"]),
	} {
		if runtimeRepairDiagnosticsHasSignal(candidate) {
			return map[string]any{
				"failureSignature": firstNonEmptyString(
					stringFromMap(candidate, failureOutputKeySignature),
					stringFromMap(input, failureOutputKeySignature),
				),
				"failureClass": firstNonEmptyString(
					stringFromMap(candidate, failureOutputKeyClass),
					stringFromMap(input, failureOutputKeyClass),
				),
				"error": firstNonEmptyString(
					stringFromMap(candidate, "error"),
					stringFromMap(candidate, "reason"),
					stringFromMap(candidate, "failureReason"),
					stringFromMap(input, "failureReason"),
				),
			}
		}
	}
	return nil
}

func runtimeRepairDiagnosticsHasSignal(diagnostics map[string]any) bool {
	if len(diagnostics) == 0 {
		return false
	}
	return strings.TrimSpace(firstNonEmptyString(
		stringFromMap(diagnostics, failureOutputKeySignature),
		stringFromMap(diagnostics, failureOutputKeyClass),
		stringFromMap(diagnostics, "error"),
		stringFromMap(diagnostics, "reason"),
		stringFromMap(diagnostics, "failureReason"),
	)) != ""
}
