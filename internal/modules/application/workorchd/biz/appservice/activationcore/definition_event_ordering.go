package activationcore

import (
	"context"
	"sort"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func OrderedWorkspaceDefinitions[T any, ID ~string](ctx context.Context, eventRecords EventRecordLister, workspaceID common.WorkspaceID, topic string, items []T, idOf func(T) ID) []T {
	ordered := append([]T(nil), items...)
	if eventRecords == nil || len(ordered) == 0 {
		return ordered
	}
	records, err := eventRecords.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID, Topic: topic})
	if err != nil || len(records) == 0 {
		return ordered
	}
	byID := make(map[string]T, len(ordered))
	for _, item := range ordered {
		id := string(idOf(item))
		if id != "" {
			byID[id] = item
		}
	}
	type orderedRecord struct {
		id  string
		pos int
	}
	lastRecordByID := map[string]orderedRecord{}
	for idx, record := range records {
		if record == nil || record.SubjectID == "" {
			continue
		}
		if _, ok := byID[record.SubjectID]; !ok {
			continue
		}
		lastRecordByID[record.SubjectID] = orderedRecord{id: record.SubjectID, pos: idx}
	}
	eventOrder := make([]orderedRecord, 0, len(lastRecordByID))
	for _, record := range lastRecordByID {
		eventOrder = append(eventOrder, record)
	}
	sort.SliceStable(eventOrder, func(i, j int) bool {
		return eventOrder[i].pos < eventOrder[j].pos
	})
	ret := make([]T, 0, len(ordered))
	seen := map[string]struct{}{}
	for _, record := range eventOrder {
		item := byID[record.id]
		ret = append(ret, item)
		seen[record.id] = struct{}{}
	}
	for _, item := range ordered {
		id := string(idOf(item))
		if _, ok := seen[id]; ok {
			continue
		}
		ret = append(ret, item)
	}
	return ret
}
