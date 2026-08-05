package events

import (
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventrecordcontract"
)

// EventRecordData is the Eventstream-owned EventHub value. The named type keeps
// the owner contract explicit while sharing the immutable field definition.
type EventRecordData workorcheventrecordcontract.Record

func EventRecordDataFromModel(item *eventstreammodel.EventRecord) (EventRecordData, error) {
	if item == nil {
		return EventRecordData{}, common.ErrInvalidArgument
	}
	ret := EventRecordData(*item)
	ret.Payload = item.Payload.Clone()
	ret.PayloadSummary = item.PayloadSummary.Clone()
	return ret, nil
}

func (item EventRecordData) Model() *eventstreammodel.EventRecord {
	ret := eventstreammodel.EventRecord(item)
	ret.Payload = item.Payload.Clone()
	ret.PayloadSummary = item.PayloadSummary.Clone()
	return &ret
}
