package readmodelcore

import (
	"strings"

	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	DefaultEventRecordLimit         = 100
	DefaultLLMUsageEventRecordLimit = 200
)

func EventRecordOwnerQuery(query EventRecordQuery) eventstreamevents.EventRecordQuery {
	return eventstreamevents.EventRecordQuery{
		WorkspaceID:    common.WorkspaceID(strings.TrimSpace(query.WorkspaceID)),
		RunID:          common.RunID(strings.TrimSpace(query.RunID)),
		Topic:          strings.TrimSpace(query.Topic),
		ExcludeTopics:  normalizeEventRecordTopics(query.ExcludeTopics),
		SubjectType:    strings.TrimSpace(query.SubjectType),
		SubjectID:      strings.TrimSpace(query.SubjectID),
		CursorAfterID:  common.EventRecordID(strings.TrimSpace(query.CursorAfterID)),
		OccurredAfter:  strings.TrimSpace(query.OccurredAfter),
		OccurredBefore: strings.TrimSpace(query.OccurredBefore),
		Limit:          query.Limit,
		IncludePayload: query.IncludePayload,
	}
}

func RunEventRecordOwnerQuery(runID common.RunID, limit int) eventstreamevents.EventRecordQuery {
	return eventstreamevents.EventRecordQuery{
		RunID: common.RunID(strings.TrimSpace(string(runID))),
		Limit: limit,
	}
}

func RunObservabilityEventRecordOwnerQuery(runID common.RunID, limit int) eventstreamevents.EventRecordQuery {
	query := RunEventRecordOwnerQuery(runID, limit)
	query.IncludePayload = true
	return query
}

func RunProjectionEventRecordOwnerQuery(runID common.RunID, limit int) eventstreamevents.EventRecordQuery {
	query := RunEventRecordOwnerQuery(runID, limit)
	query.ExcludeTopics = []string{"/run/llm_stream/progress"}
	return query
}

func RunLLMUsageEventRecordOwnerQuery(runID common.RunID, limit int) eventstreamevents.EventRecordQuery {
	query := RunEventRecordOwnerQuery(runID, limit)
	query.Topic = "/run/llm_usage/recorded"
	query.IncludePayload = true
	return query
}

func WorkspaceEventRecordOwnerQuery(workspaceID common.WorkspaceID, limit int) eventstreamevents.EventRecordQuery {
	return eventstreamevents.EventRecordQuery{
		WorkspaceID: common.WorkspaceID(strings.TrimSpace(string(workspaceID))),
		Limit:       limit,
	}
}

func WorkspaceLLMUsageEventRecordOwnerQuery(workspaceID common.WorkspaceID, limit int) eventstreamevents.EventRecordQuery {
	query := WorkspaceEventRecordOwnerQuery(workspaceID, limit)
	query.Topic = "/run/llm_usage/recorded"
	query.IncludePayload = true
	return query
}

func normalizeEventRecordTopics(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(input))
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		ret = append(ret, item)
	}
	return ret
}
