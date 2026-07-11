package events

import "testing"

func TestStructuredPayloadRoundTripsContractDiagnostics(t *testing.T) {
	payload, err := StructuredPayloadFromMap(map[string]any{
		"attempt": 2,
		"targets": map[string][]string{"coverage": {"dod-001", "goal-001"}},
	})
	if err != nil {
		t.Fatalf("convert structured payload: %v", err)
	}
	roundTrip := payload.Map()
	if roundTrip["attempt"] != int64(2) {
		t.Fatalf("attempt = %#v", roundTrip["attempt"])
	}
	targets, _ := roundTrip["targets"].(map[string]any)
	coverage, _ := targets["coverage"].([]any)
	if len(coverage) != 2 || coverage[0] != "dod-001" {
		t.Fatalf("targets = %#v", targets)
	}
}
