package repository

import (
	"context"
	"sort"
	"sync/atomic"

	eventstream "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type EventRecordRepository interface {
	Append(context.Context, *eventstream.EventRecord) error
	Get(context.Context, common.EventRecordID) (*eventstream.EventRecord, error)
	List(context.Context, workorchquerycontract.EventRecordQuery) ([]*eventstream.EventRecord, error)
	Delete(context.Context, common.EventRecordID) error
	PurgeWorkspace(context.Context, common.WorkspaceID) (int, error)
	EventRecordStreamPositionStore
}

// EventRecordStreamPositionStore is the durable sequence capability required
// by the Eventstream owner. A reservation may be skipped after a failed append,
// but a returned position must never be returned again, including after
// workspace purge or process restart.
type EventRecordStreamPositionStore interface {
	ReserveEventRecordStreamPosition(context.Context) (int64, error)
}

// SortEventRecords applies the durable Eventstream cursor order. Every current
// record carries the owner-issued global StreamPosition.
func SortEventRecords(items []*eventstream.EventRecord, query workorchquerycontract.EventRecordQuery) {
	_ = query
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if left == nil || right == nil {
			return left == nil && right != nil
		}
		if left.StreamPosition != right.StreamPosition {
			return left.StreamPosition < right.StreamPosition
		}
		if left.OccurredAt != right.OccurredAt {
			return left.OccurredAt < right.OccurredAt
		}
		if left.WorkspaceID != right.WorkspaceID {
			return string(left.WorkspaceID) < string(right.WorkspaceID)
		}
		return string(left.ID) < string(right.ID)
	})
}

type EventRecordIOMetrics struct {
	MetadataScansTotal          uint64
	MetadataRecordsScannedTotal uint64
	PayloadHydratesTotal        uint64
	PayloadHydrateBytesTotal    uint64
}

var eventRecordMetadataScansTotal atomic.Uint64
var eventRecordMetadataRecordsScannedTotal atomic.Uint64
var eventRecordPayloadHydratesTotal atomic.Uint64
var eventRecordPayloadHydrateBytesTotal atomic.Uint64

func RecordEventRecordMetadataScan(recordCount int) {
	eventRecordMetadataScansTotal.Add(1)
	if recordCount > 0 {
		eventRecordMetadataRecordsScannedTotal.Add(uint64(recordCount))
	}
}

func RecordEventRecordPayloadHydrate(byteCount int) {
	eventRecordPayloadHydratesTotal.Add(1)
	if byteCount > 0 {
		eventRecordPayloadHydrateBytesTotal.Add(uint64(byteCount))
	}
}

func EventRecordIOMetricsSnapshot() EventRecordIOMetrics {
	return EventRecordIOMetrics{
		MetadataScansTotal:          eventRecordMetadataScansTotal.Load(),
		MetadataRecordsScannedTotal: eventRecordMetadataRecordsScannedTotal.Load(),
		PayloadHydratesTotal:        eventRecordPayloadHydratesTotal.Load(),
		PayloadHydrateBytesTotal:    eventRecordPayloadHydrateBytesTotal.Load(),
	}
}
