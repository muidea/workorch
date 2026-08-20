package events

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestEventQueryNormalizedAppliesOwnerBounds(t *testing.T) {
	query, err := (EventQuery{Topic: " /run/state/changed ", ExcludeTopics: []string{" /a ", "/a", ""}}).Normalized()
	if err != nil {
		t.Fatal(err)
	}
	if query.Topic != "/run/state/changed" || query.Limit != DefaultListLimit || len(query.ExcludeTopics) != 1 || query.ExcludeTopics[0] != "/a" {
		t.Fatalf("normalized query = %+v", query)
	}
	for _, limit := range []int{-1, MaxListLimit + 1} {
		if _, err := (EventQuery{Limit: limit}).Normalized(); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("limit %d err = %v, want invalid argument", limit, err)
		}
	}
}
