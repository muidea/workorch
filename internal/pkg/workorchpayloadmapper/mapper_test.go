package workorchpayloadmapper

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchport"
)

func TestFromEventPayloadBuildsConcreteFields(t *testing.T) {
	payload := recordpayload.RunPayloadData{State: "failed", FailureReason: "provider unavailable"}.EventPayload()
	info := FromEventPayload(payload)
	if len(info.Fields) != 4 {
		t.Fatalf("fields = %+v, want typed event fields", info.Fields)
	}
	if info.Fields[0].Value.Kind != workorchport.EventPayloadValueString {
		t.Fatalf("field kind = %q, want string", info.Fields[0].Value.Kind)
	}
}

func TestFromEventPayloadPreservesNestedTypedValues(t *testing.T) {
	payload := workorcheventpayload.New(
		workorcheventpayload.ObjectField("context", workorcheventpayload.New(workorcheventpayload.StringField("scope", "run"))),
	)
	info := FromEventPayload(payload)
	if len(info.Fields) != 1 || len(info.Fields[0].Value.Object) != 1 || info.Fields[0].Value.Object[0].Name != "scope" {
		t.Fatalf("mapped payload = %+v, want nested concrete field", info)
	}
}
