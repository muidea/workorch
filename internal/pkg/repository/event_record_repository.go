package repository

import (
	"context"
	"sync/atomic"

	eventstream "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type EventRecordQuery struct {
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	Topic          string
	ExcludeTopics  []string
	SubjectType    string
	SubjectID      string
	CursorAfterID  common.EventRecordID
	OccurredAfter  string
	OccurredBefore string
	Limit          int
	IncludePayload bool
}

type EventRecordRepository interface {
	Append(context.Context, *eventstream.EventRecord) error
	Get(context.Context, common.EventRecordID) (*eventstream.EventRecord, error)
	List(context.Context, EventRecordQuery) ([]*eventstream.EventRecord, error)
	Delete(context.Context, common.EventRecordID) error
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
