package events

import "testing"

func TestNotificationPayloadRoundTripsNativeCollections(t *testing.T) {
	payload, err := NotificationPayloadFromMap(map[string]any{
		"bindings": map[string][]string{"validation": {"step-001", "step-002"}},
		"counts":   []int{1, 2},
	})
	if err != nil {
		t.Fatalf("convert notification payload: %v", err)
	}
	roundTrip := payload.Map()
	bindings, _ := roundTrip["bindings"].(map[string]any)
	validation, _ := bindings["validation"].([]any)
	if len(validation) != 2 || validation[0] != "step-001" || validation[1] != "step-002" {
		t.Fatalf("bindings = %#v", bindings)
	}
	counts, _ := roundTrip["counts"].([]any)
	if len(counts) != 2 || counts[0] != int64(1) || counts[1] != int64(2) {
		t.Fatalf("counts = %#v", counts)
	}
}
