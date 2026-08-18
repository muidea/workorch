package orchestrator

import (
	"context"
	"errors"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestQueryRuntimeRecoveryInventoryReadsBoundedPages(t *testing.T) {
	hub := magicevent.NewHub(16)
	observer := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
	var commands []runevents.QueryRuntimeRecoveryInventoryCommand
	observer.Subscribe(runevents.TopicRunCommandRuntimeRecoveryInventory, func(event magicevent.Event, result magicevent.Result) {
		command, _ := event.Data().(runevents.QueryRuntimeRecoveryInventoryCommand)
		commands = append(commands, command)
		switch command.Cursor {
		case "":
			result.Set(runevents.QueryRuntimeRecoveryInventoryResult{
				Items:      []runevents.RuntimeRecoveryInventoryItem{{WorkspaceID: "ws-1", RunID: "run-1"}},
				NextCursor: "page-a",
			}, nil)
		case "page-a":
			result.Set(runevents.QueryRuntimeRecoveryInventoryResult{
				Items: []runevents.RuntimeRecoveryInventoryItem{{WorkspaceID: "ws-2", RunID: "run-2"}},
			}, nil)
		default:
			result.Set(runevents.QueryRuntimeRecoveryInventoryResult{}, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunCommandRuntimeRecoveryInventory) })

	items, err := queryRuntimeRecoveryInventoryBounded(context.Background(), hub, 7, 3, 3)
	if err != nil {
		t.Fatalf("queryRuntimeRecoveryInventoryBounded: %v", err)
	}
	if len(items) != 2 || items[0].RunID != "run-1" || items[1].RunID != "run-2" {
		t.Fatalf("items = %+v", items)
	}
	wantCommands := []runevents.QueryRuntimeRecoveryInventoryCommand{{Limit: 7}, {Cursor: "page-a", Limit: 7}}
	if !reflect.DeepEqual(commands, wantCommands) {
		t.Fatalf("commands = %+v, want %+v", commands, wantCommands)
	}
}

func TestQueryRuntimeRecoveryInventoryRejectsCursorCycle(t *testing.T) {
	hub := magicevent.NewHub(16)
	observer := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
	calls := 0
	observer.Subscribe(runevents.TopicRunCommandRuntimeRecoveryInventory, func(event magicevent.Event, result magicevent.Result) {
		calls++
		command, _ := event.Data().(runevents.QueryRuntimeRecoveryInventoryCommand)
		next := map[string]string{"": "page-a", "page-a": "page-b", "page-b": " page-a "}[command.Cursor]
		result.Set(runevents.QueryRuntimeRecoveryInventoryResult{NextCursor: next}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunCommandRuntimeRecoveryInventory) })

	_, err := queryRuntimeRecoveryInventoryBounded(context.Background(), hub, 10, 10, 10)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("cursor cycle error = %v, want conflict", err)
	}
	if calls != 3 {
		t.Fatalf("cursor cycle calls = %d, want 3", calls)
	}
}

func TestQueryRuntimeRecoveryInventoryRejectsPageAndItemOverflow(t *testing.T) {
	t.Run("pages", func(t *testing.T) {
		hub := magicevent.NewHub(16)
		observer := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
		calls := 0
		observer.Subscribe(runevents.TopicRunCommandRuntimeRecoveryInventory, func(_ magicevent.Event, result magicevent.Result) {
			calls++
			result.Set(runevents.QueryRuntimeRecoveryInventoryResult{NextCursor: string(rune('a' + calls))}, nil)
		})
		t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunCommandRuntimeRecoveryInventory) })

		_, err := queryRuntimeRecoveryInventoryBounded(context.Background(), hub, 1, 2, 10)
		if !errors.Is(err, common.ErrConflict) || calls != 2 {
			t.Fatalf("page overflow err=%v calls=%d", err, calls)
		}
	})

	t.Run("items", func(t *testing.T) {
		hub := magicevent.NewHub(16)
		observer := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
		observer.Subscribe(runevents.TopicRunCommandRuntimeRecoveryInventory, func(_ magicevent.Event, result magicevent.Result) {
			result.Set(runevents.QueryRuntimeRecoveryInventoryResult{Items: []runevents.RuntimeRecoveryInventoryItem{
				{WorkspaceID: "ws-1", RunID: "run-1"},
				{WorkspaceID: "ws-2", RunID: "run-2"},
			}}, nil)
		})
		t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunCommandRuntimeRecoveryInventory) })

		_, err := queryRuntimeRecoveryInventoryBounded(context.Background(), hub, 2, 2, 1)
		if !errors.Is(err, common.ErrConflict) {
			t.Fatalf("item overflow error = %v, want conflict", err)
		}
	})
}
