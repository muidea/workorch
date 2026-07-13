package workorchruntimecontract

import "github.com/muidea/workorch/internal/pkg/common"

type StepExecutionRequest struct {
	WorkspaceID                   common.WorkspaceID
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	RunID                         common.RunID
	StepID                        common.StepID
	Kind                          string
	Input                         map[string]any
	EvidencePolicies              []RuntimeEvidencePolicy
}

type StepExecutionResult struct {
	Output map[string]any
}
