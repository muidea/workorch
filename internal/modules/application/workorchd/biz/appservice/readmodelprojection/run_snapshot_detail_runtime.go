package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func projectRunSnapshotRuntimeDetails(ret *readmodelcore.RunSnapshot) {
	if ret == nil {
		return
	}

	if len(ret.ToolCalls) > 0 {
		limit := min(len(ret.ToolCalls), toolCallPageLimit)
		projected := make([]readmodelcore.ToolCall, 0, limit)
		for _, item := range ret.ToolCalls[:limit] {
			projected = append(projected, ProjectToolCallView(item))
		}
		ret.ToolCalls = projected
	}
	if len(ret.RuntimeCheckpoints) > 0 {
		limit := min(len(ret.RuntimeCheckpoints), checkpointPageLimit)
		projected := make([]readmodelcore.RuntimeCheckpoint, 0, limit)
		for _, item := range ret.RuntimeCheckpoints[:limit] {
			projected = append(projected, ProjectRuntimeCheckpointView(item))
		}
		ret.RuntimeCheckpoints = projected
	}
}
