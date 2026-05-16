package models

import "github.com/muidea/workorch/internal/pkg/common"

type StepExecutionRequest struct {
	WorkspaceID                   common.WorkspaceID
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	RunID                         common.RunID
	StepID                        common.StepID
	Kind                          string
	Input                         map[string]any
}

type StepExecutionResult struct {
	Output map[string]any
}

type BlockExecutionGraph struct {
	Nodes []BlockExecutionNode `json:"nodes"`
}

type BlockExecutionNode struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	DependsOn []string       `json:"dependsOn,omitempty"`
	Branch    string         `json:"branch,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
}
