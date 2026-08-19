package ownerreadports

import (
	"context"
	"errors"
	"fmt"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestListSkillsByOwnerCommandExhaustsBoundedPages(t *testing.T) {
	hub := magicevent.NewHub(8)
	observer := magicevent.NewSimpleObserver(skillevents.CommandDestination, hub)
	calls := 0
	observer.Subscribe(skillevents.TopicSkillCommandList, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(skillevents.ListSkillsCommand)
		if !ok {
			t.Fatalf("command type = %T", ev.Data())
		}
		if cmd.Limit != skillReadPageLimit {
			t.Fatalf("page limit = %d, want %d", cmd.Limit, skillReadPageLimit)
		}
		calls++
		switch cmd.CursorAfterID {
		case "":
			result.Set(skillevents.ListSkillsResult{
				Skills: []*skillmodel.Skill{{ID: "skill-a"}, {ID: "skill-b"}}, NextCursor: "skill-b",
			}, nil)
		case "skill-b":
			result.Set(skillevents.ListSkillsResult{Skills: []*skillmodel.Skill{{ID: "skill-c"}}}, nil)
		default:
			t.Fatalf("unexpected cursor %q", cmd.CursorAfterID)
		}
	})

	items, err := ListSkillsByOwnerCommand(context.Background(), hub)
	if err != nil {
		t.Fatalf("list Skills: %v", err)
	}
	if calls != 2 || len(items) != 3 || items[0].ID != "skill-a" || items[1].ID != "skill-b" || items[2].ID != "skill-c" {
		t.Fatalf("calls/items = %d/%+v", calls, items)
	}
}

func TestListSkillsByOwnerCommandRejectsMalformedPagination(t *testing.T) {
	tests := []struct {
		name   string
		result skillevents.ListSkillsResult
	}{
		{name: "unordered page", result: skillevents.ListSkillsResult{Skills: []*skillmodel.Skill{{ID: "skill-b"}, {ID: "skill-a"}}}},
		{name: "duplicate item", result: skillevents.ListSkillsResult{Skills: []*skillmodel.Skill{{ID: "skill-a"}, {ID: "skill-a"}}}},
		{name: "empty continuation", result: skillevents.ListSkillsResult{NextCursor: "skill-a"}},
		{name: "cursor not last item", result: skillevents.ListSkillsResult{Skills: []*skillmodel.Skill{{ID: "skill-a"}}, NextCursor: "skill-b"}},
	}
	oversized := make([]*skillmodel.Skill, 0, skillReadPageLimit+1)
	for idx := 0; idx <= skillReadPageLimit; idx++ {
		oversized = append(oversized, &skillmodel.Skill{ID: common.SkillID(fmt.Sprintf("skill-%03d", idx))})
	}
	tests = append(tests, struct {
		name   string
		result skillevents.ListSkillsResult
	}{name: "oversized page", result: skillevents.ListSkillsResult{Skills: oversized}})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := magicevent.NewHub(4)
			observer := magicevent.NewSimpleObserver(skillevents.CommandDestination, hub)
			observer.Subscribe(skillevents.TopicSkillCommandList, func(_ magicevent.Event, result magicevent.Result) {
				result.Set(tt.result, nil)
			})
			if _, err := ListSkillsByOwnerCommand(context.Background(), hub); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("error = %v, want conflict", err)
			}
		})
	}
}

func TestListSkillsByOwnerCommandRejectsRepeatedContinuationCursor(t *testing.T) {
	hub := magicevent.NewHub(4)
	observer := magicevent.NewSimpleObserver(skillevents.CommandDestination, hub)
	observer.Subscribe(skillevents.TopicSkillCommandList, func(ev magicevent.Event, result magicevent.Result) {
		cmd := ev.Data().(skillevents.ListSkillsCommand)
		if cmd.CursorAfterID == "" {
			result.Set(skillevents.ListSkillsResult{Skills: []*skillmodel.Skill{{ID: "skill-a"}}, NextCursor: "skill-a"}, nil)
			return
		}
		result.Set(skillevents.ListSkillsResult{Skills: []*skillmodel.Skill{{ID: "skill-b"}}, NextCursor: "skill-a"}, nil)
	})
	if _, err := ListSkillsByOwnerCommand(context.Background(), hub); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("error = %v, want conflict", err)
	}
}
