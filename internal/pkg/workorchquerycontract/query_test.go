package workorchquerycontract

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRunResourcePageQueryNormalizedPreservesTypedScopeAndBoundsLimit(t *testing.T) {
	query := RunResourcePageQuery{
		WorkspaceID:   " ws-1 ",
		RunID:         " run-1 ",
		CursorAfterID: " item-1 ",
		Limit:         500,
	}.Normalized(25, 100)
	if query.WorkspaceID != "ws-1" || query.RunID != "run-1" || query.CursorAfterID != "item-1" {
		t.Fatalf("normalized scope = %+v", query)
	}
	if query.Limit != 100 {
		t.Fatalf("normalized limit = %d, want 100", query.Limit)
	}
	if got := (RunResourcePageQuery{}).Normalized(25, 100).Limit; got != 25 {
		t.Fatalf("default limit = %d, want 25", got)
	}
}

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

func TestWorkspaceRuntimeViewQueryNormalized(t *testing.T) {
	query := WorkspaceRuntimeViewQuery{
		WorkspaceID:   " ws-1 ",
		ActivityLimit: 50,
		BeforeCursor:  " event-9 ",
	}.Normalized()
	if query.WorkspaceID != "ws-1" || query.ActivityLimit != 50 || query.BeforeCursor != "event-9" {
		t.Fatalf("normalized Runtime view query = %+v", query)
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

func TestEventRecordQueryCanonicalizedUsesUTCAndRejectsInvalidRanges(t *testing.T) {
	query, err := (EventRecordQuery{
		OccurredAfter:  " 2026-08-30T08:00:00.25+08:00 ",
		OccurredBefore: "2026-08-30T00:00:00.75Z",
	}).Canonicalized()
	if err != nil {
		t.Fatal(err)
	}
	if query.OccurredAfter != "2026-08-30T00:00:00.25Z" || query.OccurredBefore != "2026-08-30T00:00:00.75Z" {
		t.Fatalf("canonical range = %+v", query)
	}
	for _, query := range []EventRecordQuery{
		{OccurredAfter: "invalid"},
		{OccurredBefore: "invalid"},
		{OccurredAfter: "2026-08-30T01:00:00Z", OccurredBefore: "2026-08-30T00:00:00Z"},
		{OccurredAfter: "0001-01-01T00:00:01Z", OccurredBefore: "0001-01-01T00:00:00Z"},
	} {
		if _, err := query.Canonicalized(); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("query %+v error = %v, want invalid argument", query, err)
		}
	}
}
