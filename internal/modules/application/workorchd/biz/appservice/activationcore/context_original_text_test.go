package activationcore

import "testing"

func TestContextProjectionDoesNotMaskBusinessTextOrDropLiteralMarkers(t *testing.T) {
	const original = "  token=example [REDACTED] [SECRET] [MASKED]\r\n"
	value := CompactContextValue(original, 1)
	if value != original {
		t.Fatalf("literal original text changed: %#v", value)
	}
	payload := CompactDecisionPayload(map[string]any{"answer": original})
	if payload["answer"] != original {
		t.Fatalf("decision context changed original answer: %#v", payload)
	}
}
