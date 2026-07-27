package observed

import (
	"testing"

	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type eventRecordFixture struct {
	ID             string
	Version        string
	Topic          string
	OccurredAt     string
	SourceModule   string
	SubjectType    string
	SubjectID      string
	WorkspaceID    string
	RunID          string
	StepID         string
	Revision       int64
	Payload        map[string]any
	PayloadSummary map[string]any
	PayloadRef     string
}

func eventRecordDataFromFixture(record eventRecordFixture) (eventstreamevents.EventRecordData, error) {
	payload, err := workorcheventpayload.FromMap(record.Payload)
	if err != nil {
		return eventstreamevents.EventRecordData{}, err
	}
	summary, err := workorcheventpayload.FromMap(record.PayloadSummary)
	if err != nil {
		return eventstreamevents.EventRecordData{}, err
	}
	return eventstreamevents.EventRecordData{
		ID:             common.EventRecordID(record.ID),
		Version:        record.Version,
		Topic:          record.Topic,
		OccurredAt:     record.OccurredAt,
		SourceModule:   record.SourceModule,
		SubjectType:    record.SubjectType,
		SubjectID:      record.SubjectID,
		WorkspaceID:    common.WorkspaceID(record.WorkspaceID),
		RunID:          common.RunID(record.RunID),
		StepID:         common.StepID(record.StepID),
		Revision:       record.Revision,
		Payload:        payload,
		PayloadSummary: summary,
		PayloadRef:     record.PayloadRef,
	}, nil
}

func mustEventRecordData(t *testing.T, record eventRecordFixture) eventstreamevents.EventRecordData {
	t.Helper()
	data, err := eventRecordDataFromFixture(record)
	if err != nil {
		t.Fatalf("event record fixture %s: %v", record.Topic, err)
	}
	return data
}
