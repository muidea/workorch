package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
)

func MapEventRecords(items []*eventstreammodel.EventRecord) []readmodelcore.EventRecord {
	if len(items) == 0 {
		return nil
	}
	ret := make([]readmodelcore.EventRecord, 0, len(items))
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
