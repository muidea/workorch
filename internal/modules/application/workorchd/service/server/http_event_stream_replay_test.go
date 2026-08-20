package server

import (
	"context"
	"fmt"
	"testing"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func TestReplayPersistedEventStreamReadsEveryBoundedPage(t *testing.T) {
	items := make([]projectioncontract.EventRecord, 1201)
	for index := range items {
		items[index] = projectioncontract.EventRecord{
			ID: fmt.Sprintf("event-%04d", index), Version: "workorch.event", Topic: "/test",
			OccurredAt: "2026-08-20T00:00:00Z", SourceModule: "blocks/test",
			SubjectType: "test", SubjectID: fmt.Sprintf("subject-%04d", index),
			WorkspaceID: "ws-replay-pages", Revision: int64(index + 1),
		}
	}
	pageCalls := 0
	state := &serverState{
		config: Config{Backend: "memory"},
		applicationOperations: applicationOperations{runOperations: runOperations{
			listEventRecords: func(_ context.Context, _, _ string, query workorchquerycontract.EventRecordQuery) ([]projectioncontract.EventRecord, error) {
				pageCalls++
				start := 0
				if query.CursorAfterID != "" {
					for index := range items {
						if items[index].ID == string(query.CursorAfterID) {
							start = index + 1
							break
						}
					}
				}
				end := start + query.Limit
				if end > len(items) {
					end = len(items)
				}
				return append([]projectioncontract.EventRecord(nil), items[start:end]...), nil
			},
		}},
	}
	emitted := make([]string, 0, len(items)-1)
	ok := state.replayPersistedEventStream(context.Background(), items[0].ID, "ws-replay-pages", "", "", func(item streamEvent) bool {
		emitted = append(emitted, item.ID)
		return true
	})
	if !ok || len(emitted) != len(items)-1 || emitted[0] != items[1].ID || emitted[len(emitted)-1] != items[len(items)-1].ID {
		t.Fatalf("replay ok=%v emitted=%d first=%q last=%q", ok, len(emitted), emitted[0], emitted[len(emitted)-1])
	}
	if pageCalls != 3 {
		t.Fatalf("page calls = %d, want 3 bounded pages", pageCalls)
	}
}
