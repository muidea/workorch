package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestImplementationWireDerivesExactFrozenIdentityAndRejectsEchoes(t *testing.T) {
	m, d := reviewedImplementationPurposeFixture(t, false)
	raw, err := EncodeRuntimeSemanticPlanPayloadWithManifest(m, d)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	var intent map[string]json.RawMessage
	_ = json.Unmarshal(fields["implementationIntent"], &intent)
	for _, field := range []string{"declarationRef", "contentHash"} {
		if _, ok := intent[field]; ok {
			t.Fatal("derived identity leaked to provider output", field)
		}
	}
	var previous map[string]json.RawMessage
	_ = json.Unmarshal(intent["previousRoundReview"], &previous)
	for _, field := range []string{"dispatchId", "receiptSetHash"} {
		if _, ok := previous[field]; ok {
			t.Fatal("derived receipt leaked to provider output", field)
		}
	}
	for n := 0; n < 2; n++ {
		decoded, err := DecodeRuntimeSemanticPlanPayloadWithManifest(m, raw)
		if err != nil || !reflect.DeepEqual(decoded.ImplementationIntent, d.ImplementationIntent) {
			t.Fatalf("frozen identity replay differs: %v", err)
		}
	}
	for _, field := range []string{"declarationRef", "contentHash", "dispatchId", "receiptSetHash"} {
		t.Run(field, func(t *testing.T) {
			var envelope map[string]any
			_ = json.Unmarshal(raw, &envelope)
			i := envelope["implementationIntent"].(map[string]any)
			if field == "dispatchId" || field == "receiptSetHash" {
				i = i["previousRoundReview"].(map[string]any)
			}
			i[field] = "provider-guessed-authority"
			bad, _ := json.Marshal(envelope)
			if _, err := DecodeRuntimeSemanticPlanPayloadWithManifest(m, bad); err == nil {
				t.Fatal("provider supplied derived authority")
			}
		})
	}
	// Removing a real semantic judgment still fails; removing identity echoes
	// never supplies a readiness or previous-round judgment on the model's behalf.
	delete(intent, "previousRoundReview")
	fields["implementationIntent"], _ = json.Marshal(intent)
	bad, _ := json.Marshal(fields)
	if _, err := DecodeRuntimeSemanticPlanPayloadWithManifest(m, bad); err == nil {
		t.Fatal("missing semantic review was derived")
	}
}
