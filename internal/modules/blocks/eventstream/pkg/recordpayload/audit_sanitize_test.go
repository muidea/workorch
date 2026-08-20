package recordpayload

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestSanitizeAuditPayloadRecursivelyRedactsAndBounds(t *testing.T) {
	array := make([]workorcheventpayload.Value, 40)
	for index := range array {
		array[index] = workorcheventpayload.Value{Kind: workorcheventpayload.ValueString, String: strings.Repeat("v", 100)}
	}
	input := workorcheventpayload.New(
		workorcheventpayload.ObjectField("nested", workorcheventpayload.New(
			workorcheventpayload.StringField("access_token", "must-not-survive"),
			workorcheventpayload.StringField("body", strings.Repeat("x", 700)),
		)),
		workorcheventpayload.Field{Name: "items", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: array}},
	)

	got := SanitizeAuditPayload(input)
	nested, ok := got.Object("nested")
	if !ok {
		t.Fatalf("nested payload = %+v", got)
	}
	if secret, _ := nested.String("access_token"); secret != "[redacted]" {
		t.Fatalf("nested secret = %q", secret)
	}
	if body, _ := nested.String("body"); len(body) > maxAuditPayloadStringBytes || !strings.HasSuffix(body, "...") {
		t.Fatalf("bounded body bytes=%d value=%q", len(body), body)
	}
	items, ok := got.Field("items")
	if !ok || items.Kind != workorcheventpayload.ValueArray || len(items.Array) != maxAuditPayloadArrayItems {
		t.Fatalf("bounded array = %+v", items)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > MaxAuditPayloadBytes || strings.Contains(string(encoded), "must-not-survive") {
		t.Fatalf("sanitized payload bytes=%d payload=%s", len(encoded), encoded)
	}
	if repeated := SanitizeAuditPayload(got); !repeated.Equal(got) {
		t.Fatalf("sanitizer is not idempotent: first=%+v second=%+v", got, repeated)
	}
}
