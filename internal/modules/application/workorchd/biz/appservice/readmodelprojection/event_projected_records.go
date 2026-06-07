package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
)

func MapProjectedEventRecords(items []*eventstreammodel.EventRecord) []readmodelcore.EventRecord {
	if len(items) == 0 {
		return nil
	}
	ret := make([]readmodelcore.EventRecord, 0, len(items))
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
