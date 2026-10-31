package recordpayload

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestSanitizeAuditPayloadPreservesBusinessValuesAndBounds(t *testing.T) {
	array := make([]workorcheventpayload.Value, 40)
	for index := range array {
		array[index] = workorcheventpayload.Value{Kind: workorcheventpayload.ValueString, String: strings.Repeat("v", 100)}
	}
	input := workorcheventpayload.New(
		workorcheventpayload.ObjectField("nested", workorcheventpayload.New(
			workorcheventpayload.StringField("credentialRef", "owner/ref"),
			workorcheventpayload.StringField("tokenFormat", "[REDACTED] token=literal"),
			workorcheventpayload.IntField("secretCount", 3),
			workorcheventpayload.StringField("body", strings.Repeat("x", 700)),
		)),
		workorcheventpayload.Field{Name: "items", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: array}},
	)

	got := SanitizeAuditPayload(input)
	nested, ok := got.Object("nested")
	if !ok {
		t.Fatalf("nested payload = %+v", got)
	}
	if ref, _ := nested.String("credentialRef"); ref != "owner/ref" {
		t.Fatalf("ordinary reference changed: %q", ref)
	}
	if count, ok := nested.Int64("secretCount"); !ok || count != 3 {
		t.Fatal("business metadata was replaced because of its field name")
	}
	if text, _ := nested.String("tokenFormat"); text != "[REDACTED] token=literal" {
		t.Fatal("business text was masked")
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
	if len(encoded) > MaxAuditPayloadBytes {
		t.Fatalf("sanitized payload bytes=%d payload=%s", len(encoded), encoded)
	}
	if repeated := SanitizeAuditPayload(got); !repeated.Equal(got) {
		t.Fatalf("sanitizer is not idempotent: first=%+v second=%+v", got, repeated)
	}
}

func TestAuditCredentialPayloadIsExplicitlyOmittedWithoutRewritingInput(t *testing.T) {
	input := workorcheventpayload.New(workorcheventpayload.ObjectField("nested", workorcheventpayload.New(workorcheventpayload.StringField("access_token", "fixture-value"))))
	original := input.Clone()
	got := SanitizeAuditPayload(input)
	reason, _ := got.String("auditPayloadOmittedReason")
	hash, _ := got.String("payloadSha256")
	if reason != "credential_fields_outside_audit_contract" || hash != PayloadSHA256(input) || !input.Equal(original) || !SanitizeAuditPayload(got).Equal(got) {
		t.Fatal("audit omission lost explicit source identity or rewrote original")
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "fixture-value") || strings.Contains(string(encoded), "[redacted]") {
		t.Fatal("audit disguised a credential payload as a rewritten original")
	}
}
