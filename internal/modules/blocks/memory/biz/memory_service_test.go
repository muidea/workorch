package memory

import (
	"context"
	"errors"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memoryservice "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func TestServiceCreateSearchAndValidateMemory(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMemoryItemRepository())

	userMemory := memoryservice.CreateMemoryCommand{
		MemoryID:    common.MemoryID("memory-user-context"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		SourceType:  modulecommon.MemorySourceUser,
		MemoryType:  modulecommon.MemoryTypeProjectContext,
		Summary:     "workspace uses local HTTP acceptance",
		Content:     "Verify workorchd through local HTTP APIs.",
		Tags:        []string{"local", "acceptance"},
		Importance:  5,
		Confidence:  8,
		Visibility:  "workspace",
	}
	if err := service.CreateMemoryItem(ctx, userMemory); err != nil {
		t.Fatalf("create user memory: %v", err)
	}

	runMemory := memoryservice.CreateMemoryCommand{
		MemoryID:    common.MemoryID("memory-run-summary"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		SourceType:  modulecommon.MemorySourceRun,
		SourceRef:   "run-1",
		MemoryType:  modulecommon.MemoryTypeRunSummary,
		Summary:     "run completed local verification",
		Tags:        []string{"local", "run"},
		Importance:  9,
		Confidence:  9,
	}
	if err := service.CreateMemoryItem(ctx, runMemory); err != nil {
		t.Fatalf("create run memory: %v", err)
	}

	found, err := service.SearchMemory(ctx, workorchquerycontract.MemorySearchQuery{
		WorkspaceID: common.WorkspaceID("ws-1"),
		Query:       "local",
		Tags:        []string{"local"},
		Limit:       1,
	})
	if err != nil {
		t.Fatalf("search memory: %v", err)
	}
	if len(found) != 1 || found[0].ID != runMemory.MemoryID {
		t.Fatalf("search result = %+v, want highest importance run memory", found)
	}

	list, err := service.ListMemoryByWorkspace(ctx, common.WorkspaceID("ws-1"))
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list memory len = %d, want 2", len(list))
	}
}

func TestServiceRejectsExecutionMemoryWithoutSource(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMemoryItemRepository())

	err := service.CreateMemoryItem(ctx, memoryservice.CreateMemoryCommand{
		MemoryID:    common.MemoryID("memory-invalid"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		SourceType:  modulecommon.MemorySourceRun,
		MemoryType:  modulecommon.MemoryTypeRunSummary,
		Summary:     "missing source ref",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create execution memory without source err = %v, want invalid argument", err)
	}

	err = service.CreateMemoryItem(ctx, memoryservice.CreateMemoryCommand{
		MemoryID:    common.MemoryID("memory-blank-source"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		SourceType:  modulecommon.MemorySourceRun,
		SourceRef:   "   ",
		MemoryType:  modulecommon.MemoryTypeRunSummary,
		Summary:     "source ref is blank",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create execution memory with blank source err = %v, want invalid argument", err)
	}
}

func TestServiceRejectsUnsupportedMemoryType(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMemoryItemRepository())

	err := service.CreateMemoryItem(ctx, memoryservice.CreateMemoryCommand{
		MemoryID:    common.MemoryID("memory-invalid-type"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		SourceType:  modulecommon.MemorySourceUser,
		MemoryType:  "unknown",
		Summary:     "unsupported type",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create unsupported memory type err = %v, want invalid argument", err)
	}
}

func TestServiceRejectsMemoryTypeNotAllowedForSource(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMemoryItemRepository())

	err := service.CreateMemoryItem(ctx, memoryservice.CreateMemoryCommand{
		MemoryID:    common.MemoryID("memory-source-type-invalid"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		SourceType:  modulecommon.MemorySourceUser,
		SourceRef:   "user-1",
		MemoryType:  modulecommon.MemoryTypeDecision,
		Summary:     "single decision text",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create memory type not allowed for source err = %v, want invalid argument", err)
	}
}

func TestServiceRejectsRunSourceDecisionMemory(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMemoryItemRepository())

	err := service.CreateMemoryItem(ctx, memoryservice.CreateMemoryCommand{
		MemoryID:    common.MemoryID("memory-run-decision-invalid"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		SourceType:  modulecommon.MemorySourceRun,
		SourceRef:   "run-1",
		MemoryType:  modulecommon.MemoryTypeDecision,
		Summary:     "single approval raw text",
		Content:     "approval transcript",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create run decision memory err = %v, want invalid argument", err)
	}
}

func TestServiceAllowsDecisionSourceDecisionMemory(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMemoryItemRepository())

	cmd := memoryservice.CreateMemoryCommand{
		MemoryID:    common.MemoryID("memory-decision"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		SourceType:  modulecommon.MemorySourceDecision,
		SourceRef:   "ticket-1",
		MemoryType:  modulecommon.MemoryTypeDecision,
		Summary:     "operator selected the safe retry path",
		Content:     "Decision ticket ticket-1 was resolved with safe retry.",
	}
	if err := service.CreateMemoryItem(ctx, cmd); err != nil {
		t.Fatalf("create decision source memory: %v", err)
	}

	items, err := service.ListMemoryByWorkspace(ctx, cmd.WorkspaceID)
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	if len(items) != 1 || items[0].SourceType != modulecommon.MemorySourceDecision || items[0].MemoryType != modulecommon.MemoryTypeDecision {
		t.Fatalf("memory items = %+v, want decision source decision memory", items)
	}
}

func TestServiceRejectsSensitiveMemoryText(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMemoryItemRepository())

	err := service.CreateMemoryItem(ctx, memoryservice.CreateMemoryCommand{
		MemoryID:    common.MemoryID("memory-sensitive"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		SourceType:  modulecommon.MemorySourceUser,
		MemoryType:  modulecommon.MemoryTypeProjectContext,
		Summary:     "api key for deployment",
		Content:     "api key: sk-test-secret",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create sensitive memory err = %v, want invalid argument", err)
	}
}

func TestServiceRejectsBlankMemoryTextAndTags(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMemoryItemRepository())

	err := service.CreateMemoryItem(ctx, memoryservice.CreateMemoryCommand{
		MemoryID:    common.MemoryID("memory-blank-summary"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		SourceType:  modulecommon.MemorySourceUser,
		MemoryType:  modulecommon.MemoryTypeProjectContext,
		Summary:     "   ",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create blank summary memory err = %v, want invalid argument", err)
	}

	err = service.CreateMemoryItem(ctx, memoryservice.CreateMemoryCommand{
		MemoryID:    common.MemoryID("memory-blank-tag"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		SourceType:  modulecommon.MemorySourceUser,
		MemoryType:  modulecommon.MemoryTypeProjectContext,
		Summary:     "workspace memory",
		Tags:        []string{"workspace", "   "},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create blank tag memory err = %v, want invalid argument", err)
	}
}
