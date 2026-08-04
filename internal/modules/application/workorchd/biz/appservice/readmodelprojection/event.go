package readmodelprojection

import (
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func MapEventRecords(items []*eventstreammodel.EventRecord) []projectioncontract.EventRecord {
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
