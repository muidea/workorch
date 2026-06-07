package appmodelcore

const (
	SoulBindingStagePlanning   = "planning"
	SoulBindingStageExecution  = "execution"
	SoulBindingStageDecision   = "decision"
	SoulBindingStageApproval   = "approval"
	SoulBindingStageValidation = "validation"
	SoulBindingStageArtifact   = "artifact"
)

func SoulBindingStages() []string {
	return []string{
		SoulBindingStagePlanning,
		SoulBindingStageExecution,
		SoulBindingStageDecision,
		SoulBindingStageApproval,
		SoulBindingStageValidation,
		SoulBindingStageArtifact,
	}
}

func IsSoulBindingStage(stage string) bool {
	for _, candidate := range SoulBindingStages() {
		if stage == candidate {
			return true
		}
	}
	return false
}
