package eventvalue

import "testing"

func TestPayloadRoundTripWithoutSerialization(t *testing.T) {
	input := map[string]any{
		"mode":  "resume",
		"count": 2,
		"flags": []any{true, "bounded"},
		"scope": map[string]any{"workspaceId": "ws-1"},
	}
	payload, err := FromMap(input)
	if err != nil {
		t.Fatalf("FromMap: %v", err)
	}
	output := payload.Map()
	if output["mode"] != "resume" || output["count"] != int64(2) {
		t.Fatalf("output = %#v", output)
	}
	scope, ok := output["scope"].(map[string]any)
	if !ok || scope["workspaceId"] != "ws-1" {
		t.Fatalf("scope = %#v", output["scope"])
	}
}

func TestPayloadRejectsUnsupportedValues(t *testing.T) {
	if _, err := FromMap(map[string]any{"channel": make(chan struct{})}); err == nil {
		t.Fatal("expected unsupported value error")
	}
}
