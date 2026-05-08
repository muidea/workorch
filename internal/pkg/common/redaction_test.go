package common

import (
	"fmt"
	"strings"
	"testing"
)

func TestRedactSensitiveMapRedactsKeysAndSecretShapedValues(t *testing.T) {
	input := map[string]any{
		"apiKey": "sk-secret123456",
		"nested": map[string]any{
			"note": "password = p-value token: t-value bearer abcdefghijk",
		},
		"items": []any{
			map[string]any{"privateKey": "pem-value"},
			"plain sk-anothersecret",
		},
		"safe": "visible",
	}

	got := RedactSensitiveMap(input)
	rendered := strings.ToLower(toRedactionTestString(got))
	for _, secret := range []string{"sk-secret123456", "p-value", "t-value", "abcdefghijk", "pem-value", "sk-anothersecret"} {
		if strings.Contains(rendered, strings.ToLower(secret)) {
			t.Fatalf("redacted payload = %+v, still contains %q", got, secret)
		}
	}
	if got["safe"] != "visible" {
		t.Fatalf("safe value = %+v, want preserved", got["safe"])
	}
	if got["apiKey"] != redactedValue {
		t.Fatalf("apiKey = %+v, want redacted marker", got["apiKey"])
	}
}

func toRedactionTestString(value any) string {
	return fmt.Sprintf("%#v", value)
}
