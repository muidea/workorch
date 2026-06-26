package readmodelcore

import (
	"strings"

	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const (
	DefaultEventRecordLimit         = 100
	DefaultLLMUsageEventRecordLimit = 200
)

func EventRecordRepositoryQuery(query EventRecordQuery) repository.EventRecordQuery {
	return repository.EventRecordQuery{
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

func RunEventRecordRepositoryQuery(runID common.RunID, limit int) repository.EventRecordQuery {
	return repository.EventRecordQuery{
		RunID: common.RunID(strings.TrimSpace(string(runID))),
		Limit: limit,
	}
}

func RunObservabilityEventRecordRepositoryQuery(runID common.RunID, limit int) repository.EventRecordQuery {
	query := RunEventRecordRepositoryQuery(runID, limit)
	query.IncludePayload = true
	return query
}

func RunProjectionEventRecordRepositoryQuery(runID common.RunID, limit int) repository.EventRecordQuery {
	query := RunEventRecordRepositoryQuery(runID, limit)
	query.ExcludeTopics = []string{runevents.TopicRunLLMStreamProgress}
	return query
}

func RunLLMUsageEventRecordRepositoryQuery(runID common.RunID, limit int) repository.EventRecordQuery {
	query := RunEventRecordRepositoryQuery(runID, limit)
	query.Topic = runevents.TopicRunLLMUsageRecorded
	query.IncludePayload = true
	return query
}

func WorkspaceEventRecordRepositoryQuery(workspaceID common.WorkspaceID, limit int) repository.EventRecordQuery {
	return repository.EventRecordQuery{
		WorkspaceID: common.WorkspaceID(strings.TrimSpace(string(workspaceID))),
		Limit:       limit,
	}
}

func WorkspaceLLMUsageEventRecordRepositoryQuery(workspaceID common.WorkspaceID, limit int) repository.EventRecordQuery {
	query := WorkspaceEventRecordRepositoryQuery(workspaceID, limit)
	query.Topic = runevents.TopicRunLLMUsageRecorded
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
