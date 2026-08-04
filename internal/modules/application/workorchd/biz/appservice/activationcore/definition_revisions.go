package activationcore

import (
	"context"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func WorkspaceDefinitionRevisionContexts(ctx context.Context, eventRecords EventRecordLister, workspaceID common.WorkspaceID, topic string) []map[string]any {
	if eventRecords == nil {
		return nil
	}
	records, err := eventRecords.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: workspaceID, Topic: topic})
	if err != nil || len(records) == 0 {
		return nil
	}
	type revision struct {
		id             string
		count          int
		lastEventID    string
		lastOccurredAt string
		lastPos        int
	}
	byID := map[string]*revision{}
	for idx, record := range records {
		if record == nil || record.SubjectID == "" {
			continue
		}
		item := byID[record.SubjectID]
		if item == nil {
			item = &revision{id: record.SubjectID}
		}
		byID[record.SubjectID] = item
		item.count++
		item.lastEventID = string(record.ID)
		item.lastOccurredAt = strings.TrimSpace(record.OccurredAt)
		item.lastPos = idx
	}
	items := make([]*revision, 0, len(byID))
	for _, item := range byID {
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].lastPos < items[j].lastPos
	})
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item.count <= 1 {
			continue
		}
		ret = append(ret, map[string]any{
			"id":             item.id,
			"revisionCount":  item.count,
			"lastEventId":    item.lastEventID,
			"lastOccurredAt": item.lastOccurredAt,
		})
	}
	return ret
}
