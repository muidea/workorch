package workorchport

import runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

const (
	StepKindWorkspaceExecution = runtimemodel.StepKindLLMWorkspaceExecution
)

const (
	RuntimeBlockKindPatchApply      = runtimemodel.RuntimeBlockKindPatchApply
	RuntimeBlockKindPatchPreview    = runtimemodel.RuntimeBlockKindPatchPreview
	RuntimeBlockKindPatchRevert     = runtimemodel.RuntimeBlockKindPatchRevert
	RuntimeBlockKindDecisionRequest = runtimemodel.RuntimeBlockKindDecisionRequest
	RuntimeBlockKindApprovalRequest = runtimemodel.RuntimeBlockKindApprovalRequest
	RuntimeBlockKindMemoryWrite     = runtimemodel.RuntimeBlockKindMemoryWrite
	RuntimeBlockKindDoDAssessment   = runtimemodel.RuntimeBlockKindDoDAssessment
)

func CanonicalRuntimeBlockKind(kind string) string {
	return runtimemodel.CanonicalRuntimeBlockKind(kind)
}
