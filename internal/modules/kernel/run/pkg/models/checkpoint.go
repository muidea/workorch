package run

import "github.com/muidea/workorch/internal/pkg/common"

type RuntimeCheckpoint struct {
	ID     common.CheckpointID
	RunID  common.RunID
	StepID common.StepID
	Data   map[string]any
}
