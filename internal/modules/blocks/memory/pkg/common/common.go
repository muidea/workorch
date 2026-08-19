package common

const ModuleID = "blocks/memory"

const (
	MemoryListDefaultLimit       = 100
	MemoryListMaxLimit           = 200
	MemorySearchDefaultLimit     = 20
	MemorySearchMaxLimit         = 100
	MemoryRecallListDefaultLimit = 100
	MemoryRecallListMaxLimit     = 200
	MemoryVectorMaxDimension     = 16384
)

const (
	MemoryTypeDecision        = "decision"
	MemoryTypeExecutionLesson = "execution_lesson"
	MemoryTypeProjectContext  = "project_context"
	MemoryTypeDoDFeedback     = "dod_feedback"
	MemoryTypeUserPreference  = "user_preference"
	MemoryTypeSkillFeedback   = "skill_feedback"
	MemoryTypeToolFeedback    = "tool_feedback"
	MemoryTypeRunSummary      = "run_summary"
)

const (
	MemorySourceUser             = "user"
	MemorySourceRun              = "run"
	MemorySourceDecision         = "decision"
	MemorySourceApproval         = "approval"
	MemorySourceValidationResult = "validation_result"
	MemorySourcePatch            = "patch"
	MemorySourceToolCall         = "tool_call"
)

func IsSupportedMemoryType(value string) bool {
	switch value {
	case MemoryTypeDecision,
		MemoryTypeExecutionLesson,
		MemoryTypeProjectContext,
		MemoryTypeDoDFeedback,
		MemoryTypeUserPreference,
		MemoryTypeSkillFeedback,
		MemoryTypeToolFeedback,
		MemoryTypeRunSummary:
		return true
	default:
		return false
	}
}

func IsSupportedSourceType(value string) bool {
	switch value {
	case MemorySourceUser,
		MemorySourceRun,
		MemorySourceDecision,
		MemorySourceApproval,
		MemorySourceValidationResult,
		MemorySourcePatch,
		MemorySourceToolCall:
		return true
	default:
		return false
	}
}
