package orchestrator

import "strings"

const (
	stepInputKeyRuntimeStage = "runtimeStage"
	stepInputKeyRuntimeStageMode = "runtimeStageMode"

	runtimeStageExploration = "exploration"
	runtimeStagePlanning    = "planning"
	runtimeStageExecution   = "execution"
	runtimeStageVerification = "verification"
	runtimeStageAcceptance  = "acceptance"
	runtimeStageRepair      = "repair"
	runtimeStageReplan      = "replan"
	runtimeStageDecision    = "decision"
	runtimeStageApproval    = "approval"
)

var validRuntimeStages = map[string]struct{}{
	runtimeStageExploration: {},
	runtimeStagePlanning:    {},
	runtimeStageExecution:   {},
	runtimeStageVerification: {},
	runtimeStageAcceptance:  {},
	runtimeStageRepair:      {},
	runtimeStageReplan:      {},
	runtimeStageDecision:    {},
	runtimeStageApproval:    {},
}

func normalizedRuntimeStage(value any) string {
	stage := strings.ToLower(strings.TrimSpace(runtimeStageStringValue(value)))
	if _, ok := validRuntimeStages[stage]; !ok {
		return ""
	}
	return stage
}

func runtimeStageFromStepInput(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	if stage := normalizedRuntimeStage(input[stepInputKeyRuntimeStage]); stage != "" {
		return stage
	}
	switch {
	case boolValue(input[stepInputKeyRuntimeRepairFollowUp]):
		return runtimeStageRepair
	case boolValue(input[stepInputKeyProviderRetryFollowUp]):
		if boolValue(input[stepInputKeyAssessmentOnly]) || boolValue(input["providerRetryAssessmentOnly"]) {
			return runtimeStageVerification
		}
		return runtimeStageRepair
	case boolValue(input[stepInputKeyAssessmentOnly]),
		boolValue(input["graphFollowUp"]),
		boolValue(input["completionFollowUp"]),
		boolValue(input["continuationFollowUp"]),
		boolValue(input[stepInputKeyGraphLimitFollowUp]),
		boolValue(input["assessmentOnlyContractCorrection"]):
		return runtimeStageVerification
	case input["decisionResolution"] != nil:
		return runtimeStageDecision
	case input["approvalResolution"] != nil:
		return runtimeStageApproval
	}
	switch normalizedOrchestratorSoulStage(input[stepInputKeySoulStage]) {
	case soulStagePlanning:
		return runtimeStagePlanning
	case soulStageExecution:
		return runtimeStageExecution
	case soulStageValidation:
		return runtimeStageVerification
	case soulStageDecision:
		return runtimeStageDecision
	case soulStageApproval:
		return runtimeStageApproval
	}
	return ""
}

func runtimeStageStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func normalizedOrchestratorSoulStage(value any) string {
	stage := strings.ToLower(strings.TrimSpace(runtimeStageStringValue(value)))
	switch stage {
	case soulStagePlanning, soulStageExecution, soulStageDecision, soulStageApproval, soulStageValidation:
		return stage
	default:
		return ""
	}
}

func setRuntimeStage(input map[string]any, stage string) {
	if len(input) == 0 {
		return
	}
	if stage = normalizedRuntimeStage(stage); stage == "" {
		return
	}
	input[stepInputKeyRuntimeStage] = stage
	if runtimeStageIsOpenJudgment(stage) {
		input[stepInputKeyRuntimeStageMode] = "open_judgment"
		return
	}
	if runtimeStageIsStrictConvergence(stage) {
		input[stepInputKeyRuntimeStageMode] = "strict_convergence"
		return
	}
	input[stepInputKeyRuntimeStageMode] = "structured_governance"
}

func runtimeStageIsOpenJudgment(stage string) bool {
	switch normalizedRuntimeStage(stage) {
	case runtimeStageExploration, runtimeStagePlanning:
		return true
	default:
		return false
	}
}

func runtimeStageIsStrictConvergence(stage string) bool {
	switch normalizedRuntimeStage(stage) {
	case runtimeStageExecution, runtimeStageVerification, runtimeStageAcceptance, runtimeStageRepair:
		return true
	default:
		return false
	}
}
