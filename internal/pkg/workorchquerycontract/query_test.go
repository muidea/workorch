package workorchquerycontract

import "testing"

func TestMemorySearchQueryNormalizedClonesAndCompactsSlices(t *testing.T) {
	input := MemorySearchQuery{
		WorkspaceID: " ws-1 ",
		Query:       " symbol ",
		Tags:        []string{" go ", "", "go"},
		Types:       []string{" source ", "source"},
		Vector:      []float64{1, 2},
	}
	normalized := input.Normalized()
	input.Tags[0] = "changed"
	input.Vector[0] = 9
	if normalized.WorkspaceID != "ws-1" || normalized.Query != "symbol" {
		t.Fatalf("normalized identity = %+v", normalized)
	}
	if len(normalized.Tags) != 1 || normalized.Tags[0] != "go" {
		t.Fatalf("normalized tags = %#v", normalized.Tags)
	}
	if len(normalized.Types) != 1 || normalized.Types[0] != "source" {
		t.Fatalf("normalized types = %#v", normalized.Types)
	}
	if normalized.Vector[0] != 1 {
		t.Fatalf("normalized vector shares input: %#v", normalized.Vector)
	}
}

func TestEventRecordQueryNormalizedClonesAndCompactsTopics(t *testing.T) {
	input := EventRecordQuery{
		WorkspaceID:   " ws-1 ",
		RunID:         " run-1 ",
		Topic:         " /run/state ",
		ExcludeTopics: []string{" /run/noise ", "", "/run/noise"},
		CursorAfterID: " event-1 ",
	}
	normalized := input.Normalized()
	input.ExcludeTopics[0] = "changed"
	if normalized.WorkspaceID != "ws-1" || normalized.RunID != "run-1" || normalized.CursorAfterID != "event-1" {
		t.Fatalf("normalized scope = %+v", normalized)
	}
	if len(normalized.ExcludeTopics) != 1 || normalized.ExcludeTopics[0] != "/run/noise" {
		t.Fatalf("normalized exclude topics = %#v", normalized.ExcludeTopics)
	}
}
