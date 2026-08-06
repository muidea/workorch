package readmodelcore

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

type RuntimeCheckpointPage struct {
	RunID              string                                 `json:"runId"`
	RuntimeCheckpoints []projectioncontract.RuntimeCheckpoint `json:"runtimeCheckpoints,omitempty"`
	projectioncontract.CursorPage
}
