package common

const ModuleID = "kernel/memory"

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
