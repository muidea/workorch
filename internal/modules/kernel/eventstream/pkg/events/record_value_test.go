package events

import (
	"reflect"
	"testing"

	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestEventRecordDataRoundTripWithoutAnyInContract(t *testing.T) {
	input := &eventstreammodel.EventRecord{
		ID:          common.EventRecordID("event-1"),
		Topic:       "/run/state/changed",
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		Payload: map[string]any{
			"state": "running",
			"count": 3,
			"flags": []any{true, "bounded"},
			"scope": map[string]any{"workspaceId": "ws-1"},
		},
		PayloadSummary: map[string]any{"fieldCount": 4},
	}

	data, err := EventRecordDataFromModel(input)
	if err != nil {
		t.Fatalf("EventRecordDataFromModel: %v", err)
	}
	if len(data.Payload.Fields) != 4 {
		t.Fatalf("payload fields = %d, want 4", len(data.Payload.Fields))
	}
	output := data.Model()
	if output == nil {
		t.Fatal("round trip model is nil")
	}
	if !reflect.DeepEqual(output.Payload, map[string]any{
		"state": "running",
		"count": int64(3),
		"flags": []any{true, "bounded"},
		"scope": map[string]any{"workspaceId": "ws-1"},
	}) {
		t.Fatalf("round trip payload = %#v", output.Payload)
	}
}

func TestEventRecordDataRejectsUnsupportedPayloadValue(t *testing.T) {
	_, err := EventRecordDataFromModel(&eventstreammodel.EventRecord{
		ID:      common.EventRecordID("event-invalid"),
		Payload: map[string]any{"invalid": make(chan struct{})},
	})
	if err == nil {
		t.Fatal("unsupported payload value was accepted")
	}
}
