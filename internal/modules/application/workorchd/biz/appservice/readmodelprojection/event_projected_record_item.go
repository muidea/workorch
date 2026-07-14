package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func mapProjectedEventRecord(item *eventstreammodel.EventRecord) (readmodelcore.EventRecord, bool) {
	if item == nil {
		return readmodelcore.EventRecord{}, false
	}
	return mapEventRecordWithPayload(item, projectEventPayload(item.Payload))
}

func mapEventRecordWithPayload(item *eventstreammodel.EventRecord, payload workorcheventpayload.Payload) (readmodelcore.EventRecord, bool) {
	if item == nil {
		return readmodelcore.EventRecord{}, false
	}
	return readmodelcore.EventRecord{
		ID:             string(item.ID),
		Version:        item.Version,
		Topic:          item.Topic,
		OccurredAt:     item.OccurredAt,
		SourceModule:   item.SourceModule,
		SubjectType:    item.SubjectType,
		SubjectID:      item.SubjectID,
		WorkspaceID:    string(item.WorkspaceID),
		RunID:          string(item.RunID),
		StepID:         string(item.StepID),
		Revision:       item.Revision,
		Payload:        payload.Clone(),
		PayloadSummary: item.PayloadSummary.Clone(),
		PayloadRef:     item.PayloadRef,
	}, true
}
