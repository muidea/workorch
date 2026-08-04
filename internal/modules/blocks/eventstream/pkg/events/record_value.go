package events

import (
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type EventRecordData struct {
	ID             common.EventRecordID
	Version        string
	Topic          string
	OccurredAt     string
	SourceModule   string
	SubjectType    string
	SubjectID      string
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	StepID         common.StepID
	Revision       int64
	Payload        workorcheventpayload.Payload
	PayloadSummary workorcheventpayload.Payload
	PayloadRef     string
}

func EventRecordDataFromModel(item *eventstreammodel.EventRecord) (EventRecordData, error) {
	if item == nil {
		return EventRecordData{}, common.ErrInvalidArgument
	}
	return EventRecordData{
		ID:             item.ID,
		Version:        item.Version,
		Topic:          item.Topic,
		OccurredAt:     item.OccurredAt,
		SourceModule:   item.SourceModule,
		SubjectType:    item.SubjectType,
		SubjectID:      item.SubjectID,
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		StepID:         item.StepID,
		Revision:       item.Revision,
		Payload:        item.Payload.Clone(),
		PayloadSummary: item.PayloadSummary.Clone(),
		PayloadRef:     item.PayloadRef,
	}, nil
}

func (item EventRecordData) Model() *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		ID:             item.ID,
		Version:        item.Version,
		Topic:          item.Topic,
		OccurredAt:     item.OccurredAt,
		SourceModule:   item.SourceModule,
		SubjectType:    item.SubjectType,
		SubjectID:      item.SubjectID,
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		StepID:         item.StepID,
		Revision:       item.Revision,
		Payload:        item.Payload.Clone(),
		PayloadSummary: item.PayloadSummary.Clone(),
		PayloadRef:     item.PayloadRef,
	}
}
