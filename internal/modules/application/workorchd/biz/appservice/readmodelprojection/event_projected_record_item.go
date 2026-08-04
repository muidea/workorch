package readmodelprojection

import (
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mapProjectedEventRecord(item *eventstreammodel.EventRecord) (projectioncontract.EventRecord, bool) {
	if item == nil {
		return projectioncontract.EventRecord{}, false
	}
	return mapEventRecordWithPayload(item, projectEventPayload(item.Payload))
}

func mapEventRecordWithPayload(item *eventstreammodel.EventRecord, payload workorcheventpayload.Payload) (projectioncontract.EventRecord, bool) {
	if item == nil {
		return projectioncontract.EventRecord{}, false
	}
	return projectioncontract.EventRecord{
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
