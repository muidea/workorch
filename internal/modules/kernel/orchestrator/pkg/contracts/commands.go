package contracts

import "github.com/muidea/workorch/internal/pkg/common"

type PurgeRunCommand struct {
	RunID common.RunID
}

type DispatchStepCommand struct {
	WorkspaceID    common.WorkspaceID `json:"workspaceId,omitempty"`
	RunID          common.RunID       `json:"runId"`
	StepID         common.StepID      `json:"stepId"`
	Kind           string             `json:"kind"`
	IdempotencyKey string             `json:"idempotencyKey,omitempty"`
	Input          map[string]any     `json:"input,omitempty"`
}
