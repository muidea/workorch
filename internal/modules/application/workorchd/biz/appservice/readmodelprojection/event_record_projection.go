package readmodelprojection

import (
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: event.go

func mapEventRecords(items []*eventstreammodel.EventRecord) []projectioncontract.EventRecord {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.EventRecord, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		projected, ok := mapEventRecordWithPayload(item, item.Payload)
		if !ok {
			continue
		}
		ret = append(ret, projected)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: event_projected_records.go

func mapProjectedEventRecords(items []*eventstreammodel.EventRecord) []projectioncontract.EventRecord {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.EventRecord, 0, len(items))
	for _, item := range items {
		projected, ok := mapProjectedEventRecord(item)
		if !ok {
			continue
		}
		ret = append(ret, projected)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
