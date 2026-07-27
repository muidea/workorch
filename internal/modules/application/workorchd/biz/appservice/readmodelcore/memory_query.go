package readmodelcore

import (
	"strings"

	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func MemoryRecallTraceOwnerQuery(query MemoryRecallTraceQuery) memoryevents.MemoryRecallTraceQuery {
	return memoryevents.MemoryRecallTraceQuery{
		WorkspaceID: common.WorkspaceID(strings.TrimSpace(query.WorkspaceID)),
		RunID:       common.RunID(strings.TrimSpace(query.RunID)),
		StepID:      common.StepID(strings.TrimSpace(query.StepID)),
		Stage:       strings.TrimSpace(query.Stage),
		Source:      strings.TrimSpace(query.Source),
		Status:      strings.TrimSpace(query.Status),
		Limit:       query.Limit,
	}
}

func RunMemoryRecallTraceOwnerQuery(workspaceID common.WorkspaceID, runID common.RunID) memoryevents.MemoryRecallTraceQuery {
	return memoryevents.MemoryRecallTraceQuery{
		WorkspaceID: common.WorkspaceID(strings.TrimSpace(string(workspaceID))),
		RunID:       common.RunID(strings.TrimSpace(string(runID))),
	}
}
