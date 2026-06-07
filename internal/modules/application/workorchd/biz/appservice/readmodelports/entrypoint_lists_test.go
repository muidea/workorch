package readmodelports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type memoryRecallTraceReaderFunc func(context.Context, repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error)

func (fn memoryRecallTraceReaderFunc) List(ctx context.Context, query repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	return fn(ctx, query)
}

type eventRecordReaderFunc func(context.Context, repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)

func (fn eventRecordReaderFunc) List(ctx context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	return fn(ctx, query)
}

func TestMemoryRecallTracesNormalizesQueryAndMapsItems(t *testing.T) {
	items, err := MemoryRecallTraces(
		context.Background(),
		memoryRecallTraceReaderFunc(func(_ context.Context, query repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
			if query.WorkspaceID != "ws-001" || query.RunID != "run-001" {
				t.Fatalf("query = %+v, want trimmed workspace/run", query)
			}
			return []*memorymodel.MemoryRecallTrace{{
				ID:          "trace-001",
				WorkspaceID: "ws-001",
				RunID:       "run-001",
				Status:      "selected",
			}}, nil
		}),
		readmodelcore.MemoryRecallTraceQuery{WorkspaceID: " ws-001 ", RunID: " run-001 "},
	)
	if err != nil {
		t.Fatalf("MemoryRecallTraces err = %v", err)
	}
	if len(items) != 1 || items[0].ID != "trace-001" || items[0].Status != "selected" {
		t.Fatalf("items = %#v, want mapped trace-001", items)
	}
}

func TestEventRecordsNormalizesQueryAndMapsItems(t *testing.T) {
	items, err := EventRecords(
		context.Background(),
		eventRecordReaderFunc(func(_ context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
			if query.WorkspaceID != "ws-001" || query.RunID != "run-001" || query.Limit != 5 {
				t.Fatalf("query = %+v, want trimmed workspace/run with limit", query)
			}
			return []*eventstreammodel.EventRecord{{
				ID:          "event-001",
				Topic:       "/topic",
				WorkspaceID: common.WorkspaceID("ws-001"),
				RunID:       common.RunID("run-001"),
			}}, nil
		}),
		readmodelcore.EventRecordQuery{WorkspaceID: " ws-001 ", RunID: " run-001 ", Limit: 5},
	)
	if err != nil {
		t.Fatalf("EventRecords err = %v", err)
	}
	if len(items) != 1 || items[0].ID != "event-001" || items[0].Topic != "/topic" {
		t.Fatalf("items = %#v, want mapped event-001", items)
	}
}
