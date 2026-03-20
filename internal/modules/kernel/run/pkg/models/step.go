package run

import "github.com/muidea/workorch/internal/pkg/common"

type Step struct {
	ID     common.StepID
	RunID  common.RunID
	Kind   string
	State  common.StepState
	Input  map[string]any
	Output map[string]any
}
