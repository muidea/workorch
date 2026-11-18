package recordpayload

import (
	"encoding/json"
	"math"
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

func TestIsSanitizedAuditPayloadFastPathMatchesSanitizer(t *testing.T) {
	clean := workorcheventpayload.New(
		workorcheventpayload.StringField("aKey", "alpha"),
		workorcheventpayload.IntField("bCount", 42),
		workorcheventpayload.ObjectField("cMeta", workorcheventpayload.New(
			workorcheventpayload.StringField("credentialRef", "safe-ref"),
		)),
	)
	if !IsSanitizedAuditPayload(clean) || !SanitizeAuditPayload(clean).Equal(clean) {
		t.Fatal("expected clean sorted payload to pass")
	}
	if allocs := testing.AllocsPerRun(50, func() {
		if !IsSanitizedAuditPayload(clean) {
			t.Fatal("clean payload unexpectedly rejected")
		}
	}); allocs != 0 {
		t.Fatalf("clean fast path allocated %.1f times, want 0", allocs)
	}
	if !IsSanitizedAuditPayload(workorcheventpayload.Payload{}) {
		t.Fatal("expected empty payload to pass")
	}

	cases := []struct {
		name  string
		input workorcheventpayload.Payload
		want  bool
	}{
		{
			name:  "credential_uppercase_underscore",
			input: workorcheventpayload.New(workorcheventpayload.StringField("Access_Token", "secret")),
			want:  false,
		},
		{
			name:  "credential_hyphen",
			input: workorcheventpayload.New(workorcheventpayload.StringField("api-key", "secret")),
			want:  false,
		},
		{
			name:  "untrimmed_field_name",
			input: workorcheventpayload.New(workorcheventpayload.StringField(" kind ", "ok")),
			want:  false,
		},
		{
			name:  "unsorted_clean_fields",
			input: workorcheventpayload.New(workorcheventpayload.StringField("z", "1"), workorcheventpayload.StringField("a", "2")),
			want:  true,
		},
		{
			name:  "oversized_string",
			input: workorcheventpayload.New(workorcheventpayload.StringField("a", strings.Repeat("x", maxAuditPayloadStringBytes+1))),
			want:  false,
		},
		{
			name:  "nan_float",
			input: workorcheventpayload.New(workorcheventpayload.FloatField("a", math.NaN())),
			want:  false,
		},
		{
			name: "depth_4_object_truncated",
			input: workorcheventpayload.New(workorcheventpayload.ObjectField("d0", workorcheventpayload.New(
				workorcheventpayload.ObjectField("d1", workorcheventpayload.New(
					workorcheventpayload.ObjectField("d2", workorcheventpayload.New(
						workorcheventpayload.ObjectField("d3", workorcheventpayload.New(
							workorcheventpayload.ObjectField("d4", workorcheventpayload.Payload{}),
						)),
					)),
				)),
			))),
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsSanitizedAuditPayload(tc.input); got != tc.want {
				t.Fatalf("IsSanitizedAuditPayload=%v, want %v (Sanitize.Equal=%v)", got, tc.want, SanitizeAuditPayload(tc.input).Equal(tc.input))
			}
		})
	}
}
