package workorchport

import "strings"

const (
	StepKindWorkspaceExecution = "llm/workspace_execution"
	StepKindWorkspacePlan      = "llm/workspace-plan"
)

const (
	RuntimeBlockKindPatchApply      = "patch/apply"
	RuntimeBlockKindPatchPreview    = "patch/preview"
	RuntimeBlockKindPatchRevert     = "patch/revert"
	RuntimeBlockKindDecisionRequest = "decision/request"
	RuntimeBlockKindApprovalRequest = "approval/request"
	RuntimeBlockKindMemoryWrite     = "memory/write"
	RuntimeBlockKindDoDAssessment   = "dod/assessment"
)

func CanonicalRuntimeBlockKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case StepKindWorkspaceExecution, StepKindWorkspacePlan:
		return StepKindWorkspaceExecution
	case "patchRequests", "patch/request":
		return RuntimeBlockKindPatchApply
	default:
		return strings.TrimSpace(kind)
	}
}
