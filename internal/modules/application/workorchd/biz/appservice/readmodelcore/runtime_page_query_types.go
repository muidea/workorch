package readmodelcore

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

type RuntimeCheckpointPage struct {
	RunID              string                                 `json:"runId"`
	RuntimeCheckpoints []projectioncontract.RuntimeCheckpoint `json:"runtimeCheckpoints,omitempty"`
	Limit              int                                    `json:"limit"`
	CursorAfterID      string                                 `json:"cursorAfterId,omitempty"`
	NextCursorAfterID  string                                 `json:"nextCursorAfterId,omitempty"`
	HasMore            bool                                   `json:"hasMore"`
}
