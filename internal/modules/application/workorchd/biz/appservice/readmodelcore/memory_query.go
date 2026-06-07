package readmodelcore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func MemoryRecallTraceRepositoryQuery(query MemoryRecallTraceQuery) repository.MemoryRecallTraceQuery {
	return repository.MemoryRecallTraceQuery{
		WorkspaceID: common.WorkspaceID(strings.TrimSpace(query.WorkspaceID)),
		RunID:       common.RunID(strings.TrimSpace(query.RunID)),
		StepID:      common.StepID(strings.TrimSpace(query.StepID)),
		Stage:       strings.TrimSpace(query.Stage),
		Source:      strings.TrimSpace(query.Source),
		Status:      strings.TrimSpace(query.Status),
		Limit:       query.Limit,
	}
}

func RunMemoryRecallTraceRepositoryQuery(workspaceID common.WorkspaceID, runID common.RunID) repository.MemoryRecallTraceQuery {
	return repository.MemoryRecallTraceQuery{
		WorkspaceID: common.WorkspaceID(strings.TrimSpace(string(workspaceID))),
		RunID:       common.RunID(strings.TrimSpace(string(runID))),
	}
}
