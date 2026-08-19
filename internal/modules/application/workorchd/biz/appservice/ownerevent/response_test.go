package ownerevent

import (
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestMemoryResponseRequiresConcreteResultType(t *testing.T) {
	if _, err := Memory[memoryevents.GetMemoryResult](nil, memoryevents.TopicMemoryCommandGet); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("nil response error = %v", err)
	}

	response := magicevent.NewResult(memoryevents.TopicMemoryCommandGet, memoryevents.CommandDestination, "test")
	response.Set(memoryevents.ListMemoryByWorkspaceResult{}, nil)
	if _, err := Memory[memoryevents.GetMemoryResult](response, memoryevents.TopicMemoryCommandGet); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("wrong concrete response error = %v", err)
	}

	response = magicevent.NewResult(memoryevents.TopicMemoryCommandGet, memoryevents.CommandDestination, "test")
	want := memoryevents.GetMemoryResult{}
	response.Set(want, nil)
	got, err := Memory[memoryevents.GetMemoryResult](response, memoryevents.TopicMemoryCommandGet)
	if err != nil {
		t.Fatalf("decode concrete memory result: %v", err)
	}
	if got.Item != nil {
		t.Fatalf("decoded concrete memory result = %+v", got)
	}
}
