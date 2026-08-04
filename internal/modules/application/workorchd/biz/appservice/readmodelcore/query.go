package readmodelcore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

const (
	DefaultEventRecordLimit         = 100
	DefaultLLMUsageEventRecordLimit = 200
)

func EventRecordOwnerQuery(query workorchquerycontract.EventRecordQuery) workorchquerycontract.EventRecordQuery {
	return query.Normalized()
}

func RunEventRecordOwnerQuery(runID common.RunID, limit int) workorchquerycontract.EventRecordQuery {
	return workorchquerycontract.EventRecordQuery{
		RunID: common.RunID(strings.TrimSpace(string(runID))),
		Limit: limit,
	}
}

func RunObservabilityEventRecordOwnerQuery(runID common.RunID, limit int) workorchquerycontract.EventRecordQuery {
	query := RunEventRecordOwnerQuery(runID, limit)
	query.IncludePayload = true
	return query
}

func RunProjectionEventRecordOwnerQuery(runID common.RunID, limit int) workorchquerycontract.EventRecordQuery {
	query := RunEventRecordOwnerQuery(runID, limit)
	query.ExcludeTopics = []string{"/run/llm_stream/progress"}
	return query
}

func RunLLMUsageEventRecordOwnerQuery(runID common.RunID, limit int) workorchquerycontract.EventRecordQuery {
	query := RunEventRecordOwnerQuery(runID, limit)
	query.Topic = "/run/llm_usage/recorded"
	query.IncludePayload = true
	return query
}

func WorkspaceEventRecordOwnerQuery(workspaceID common.WorkspaceID, limit int) workorchquerycontract.EventRecordQuery {
	return workorchquerycontract.EventRecordQuery{
		WorkspaceID: common.WorkspaceID(strings.TrimSpace(string(workspaceID))),
		Limit:       limit,
	}
}

func WorkspaceLLMUsageEventRecordOwnerQuery(workspaceID common.WorkspaceID, limit int) workorchquerycontract.EventRecordQuery {
	query := WorkspaceEventRecordOwnerQuery(workspaceID, limit)
	query.Topic = "/run/llm_usage/recorded"
	query.IncludePayload = true
	return query
}
