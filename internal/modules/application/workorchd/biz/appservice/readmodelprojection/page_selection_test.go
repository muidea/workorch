package readmodelprojection

import "testing"

type pageSelectionFixture struct {
	id    string
	value string
}

func TestSelectProjectedPageAppliesCursorLimitAndNilFiltering(t *testing.T) {
	items := []*pageSelectionFixture{
		{id: "item-1", value: "first"},
		nil,
		{id: "item-2", value: "second"},
		{id: "item-3", value: "third"},
		{id: "item-4", value: "fourth"},
	}
	selected, nextCursor, hasMore := selectProjectedPage(items, 2, "item-1",
		func(item *pageSelectionFixture) string { return item.id },
		func(item *pageSelectionFixture) string { return item.value },
	)
	if len(selected) != 2 || selected[0] != "second" || selected[1] != "third" {
		t.Fatalf("selected = %#v", selected)
	}
	if nextCursor != "item-3" || !hasMore {
		t.Fatalf("nextCursor=%q hasMore=%v", nextCursor, hasMore)
	}
}

func TestSelectProjectedPageMissingCursorReturnsEmptyPage(t *testing.T) {
	items := []*pageSelectionFixture{{id: "item-1", value: "first"}}
	selected, nextCursor, hasMore := selectProjectedPage(items, 1, "missing",
		func(item *pageSelectionFixture) string { return item.id },
		func(item *pageSelectionFixture) string { return item.value },
	)
	if len(selected) != 0 || nextCursor != "" || hasMore {
		t.Fatalf("selected=%#v nextCursor=%q hasMore=%v", selected, nextCursor, hasMore)
	}
}
