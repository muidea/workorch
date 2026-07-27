package events

import (
	"testing"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestEventRecordDataRoundTripWithoutAnyInContract(t *testing.T) {
	input := &eventstreammodel.EventRecord{
		ID:          common.EventRecordID("event-1"),
		Topic:       "/run/state/changed",
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		Payload: workorcheventpayload.New(
			workorcheventpayload.StringField("state", "running"),
			workorcheventpayload.IntField("count", 3),
			workorcheventpayload.Field{Name: "flags", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: []workorcheventpayload.Value{{Kind: workorcheventpayload.ValueBool, Bool: true}, {Kind: workorcheventpayload.ValueString, String: "bounded"}}}},
			workorcheventpayload.ObjectField("scope", workorcheventpayload.New(workorcheventpayload.StringField("workspaceId", "ws-1"))),
		),
		PayloadSummary: workorcheventpayload.New(workorcheventpayload.IntField("fieldCount", 4)),
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
	if !output.Payload.Equal(input.Payload) {
		t.Fatalf("round trip payload = %#v", output.Payload)
	}
}

func TestEventRecordDataClonesPayload(t *testing.T) {
	input := &eventstreammodel.EventRecord{
		ID:      common.EventRecordID("event-clone"),
		Payload: workorcheventpayload.New(workorcheventpayload.StringField("state", "running")),
	}
	data, err := EventRecordDataFromModel(input)
	if err != nil {
		t.Fatalf("EventRecordDataFromModel: %v", err)
	}
	input.Payload.Fields[0].Value.String = "mutated"
	if state, ok := data.Payload.String("state"); !ok || state != "running" {
		t.Fatalf("cloned payload state = %q/%v, want running/true", state, ok)
	}
}
