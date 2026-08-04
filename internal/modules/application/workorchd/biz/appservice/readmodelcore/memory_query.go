package readmodelcore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func MemoryRecallTraceOwnerQuery(query workorchquerycontract.MemoryRecallTraceQuery) workorchquerycontract.MemoryRecallTraceQuery {
	return query.Normalized()
}

func RunMemoryRecallTraceOwnerQuery(workspaceID common.WorkspaceID, runID common.RunID) workorchquerycontract.MemoryRecallTraceQuery {
	return workorchquerycontract.MemoryRecallTraceQuery{
		WorkspaceID: common.WorkspaceID(strings.TrimSpace(string(workspaceID))),
		RunID:       common.RunID(strings.TrimSpace(string(runID))),
	}
}
