package memory

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memoryservice "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func TestServiceCreateSearchAndValidateMemory(t *testing.T) {
	ctx := context.Background()
	service := newService(backend.NewMemoryItemRepository())

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
	if err := service.createMemoryItem(ctx, userMemory); err != nil {
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
	if err := service.createMemoryItem(ctx, runMemory); err != nil {
		t.Fatalf("create run memory: %v", err)
	}

	found, err := service.searchMemory(ctx, workorchquerycontract.MemorySearchQuery{
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

	list, _, err := service.listMemoryByWorkspace(ctx, common.WorkspaceID("ws-1"), "", 100)
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list memory len = %d, want 2", len(list))
	}
}

func TestServiceRejectsExecutionMemoryWithoutSource(t *testing.T) {
	ctx := context.Background()
	service := newService(backend.NewMemoryItemRepository())

	err := service.createMemoryItem(ctx, memoryservice.CreateMemoryCommand{
		MemoryID:    common.MemoryID("memory-invalid"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		SourceType:  modulecommon.MemorySourceRun,
		MemoryType:  modulecommon.MemoryTypeRunSummary,
		Summary:     "missing source ref",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create execution memory without source err = %v, want invalid argument", err)
	}

	err = service.createMemoryItem(ctx, memoryservice.CreateMemoryCommand{
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
	service := newService(backend.NewMemoryItemRepository())

	err := service.createMemoryItem(ctx, memoryservice.CreateMemoryCommand{
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
	service := newService(backend.NewMemoryItemRepository())

	err := service.createMemoryItem(ctx, memoryservice.CreateMemoryCommand{
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
	service := newService(backend.NewMemoryItemRepository())

	err := service.createMemoryItem(ctx, memoryservice.CreateMemoryCommand{
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
	service := newService(backend.NewMemoryItemRepository())

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
	if err := service.createMemoryItem(ctx, cmd); err != nil {
		t.Fatalf("create decision source memory: %v", err)
	}

	items, _, err := service.listMemoryByWorkspace(ctx, cmd.WorkspaceID, "", 100)
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	if len(items) != 1 || items[0].SourceType != modulecommon.MemorySourceDecision || items[0].MemoryType != modulecommon.MemoryTypeDecision {
		t.Fatalf("memory items = %+v, want decision source decision memory", items)
	}
}

func TestServiceRejectsSensitiveMemoryText(t *testing.T) {
	ctx := context.Background()
	service := newService(backend.NewMemoryItemRepository())

	err := service.createMemoryItem(ctx, memoryservice.CreateMemoryCommand{
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
	service := newService(backend.NewMemoryItemRepository())

	err := service.createMemoryItem(ctx, memoryservice.CreateMemoryCommand{
		MemoryID:    common.MemoryID("memory-blank-summary"),
		WorkspaceID: common.WorkspaceID("ws-1"),
		SourceType:  modulecommon.MemorySourceUser,
		MemoryType:  modulecommon.MemoryTypeProjectContext,
		Summary:     "   ",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create blank summary memory err = %v, want invalid argument", err)
	}

	err = service.createMemoryItem(ctx, memoryservice.CreateMemoryCommand{
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

func TestServiceBoundsMemoryAndRecallQueriesWithCursors(t *testing.T) {
	ctx := context.Background()
	items := backend.NewMemoryItemRepository()
	traces := backend.NewMemoryRecallTraceRepository()
	service := newServiceWithRecallTraces(items, traces)
	workspaceID := common.WorkspaceID("ws-bounded-memory")
	for idx := 0; idx < 205; idx++ {
		id := common.MemoryID(fmt.Sprintf("memory-%03d", idx))
		if err := service.createMemoryItem(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), memoryservice.CreateMemoryCommand{
			MemoryID: id, WorkspaceID: workspaceID, SourceType: modulecommon.MemorySourceUser,
			MemoryType: modulecommon.MemoryTypeProjectContext, Summary: string(id),
		}); err != nil {
			t.Fatalf("create memory %s: %v", id, err)
		}
		trace := &memorymodel.MemoryRecallTrace{
			ID: common.MemoryRecallTraceID(fmt.Sprintf("trace-%03d", idx)), WorkspaceID: workspaceID,
			RunID: common.RunID(fmt.Sprintf("run-%03d", idx)), Source: "recovery", Status: "inherited",
			OccurredAt: fmt.Sprintf("2026-08-19T00:%02d:%02dZ", idx/60, idx%60),
		}
		if err := traces.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), trace); err != nil {
			t.Fatalf("create trace %s: %v", trace.ID, err)
		}
	}

	first, itemCursor, err := service.listMemoryByWorkspace(ctx, workspaceID, "", 0)
	if err != nil {
		t.Fatalf("list default memory page: %v", err)
	}
	if len(first) != modulecommon.MemoryListDefaultLimit || itemCursor != "memory-099" {
		t.Fatalf("default memory page len/cursor = %d/%q", len(first), itemCursor)
	}
	second, nextItemCursor, err := service.listMemoryByWorkspace(ctx, workspaceID, itemCursor, 1000)
	if err != nil {
		t.Fatalf("list capped memory page: %v", err)
	}
	if len(second) != 105 || nextItemCursor != "" || second[0].ID != "memory-100" {
		t.Fatalf("second memory page = len:%d cursor:%q first:%v", len(second), nextItemCursor, second[0].ID)
	}

	for name, limit := range map[string]int{"zero": 0, "negative": -1} {
		found, err := service.searchMemory(ctx, workorchquerycontract.MemorySearchQuery{WorkspaceID: workspaceID, Limit: limit})
		if err != nil {
			t.Fatalf("%s-limit search: %v", name, err)
		}
		if len(found) != modulecommon.MemorySearchDefaultLimit {
			t.Fatalf("%s-limit search len = %d", name, len(found))
		}
	}
	found, err := service.searchMemory(ctx, workorchquerycontract.MemorySearchQuery{WorkspaceID: workspaceID, Limit: 1000})
	if err != nil {
		t.Fatalf("capped search: %v", err)
	}
	if len(found) != modulecommon.MemorySearchMaxLimit {
		t.Fatalf("capped search len = %d", len(found))
	}

	tracePage, traceCursor, err := service.listMemoryRecallTraces(ctx, workorchquerycontract.MemoryRecallTraceQuery{WorkspaceID: workspaceID})
	if err != nil {
		t.Fatalf("list default trace page: %v", err)
	}
	if len(tracePage) != modulecommon.MemoryRecallListDefaultLimit || traceCursor != "trace-099" {
		t.Fatalf("default trace page len/cursor = %d/%q", len(tracePage), traceCursor)
	}
	tracePage, nextTraceCursor, err := service.listMemoryRecallTraces(ctx, workorchquerycontract.MemoryRecallTraceQuery{WorkspaceID: workspaceID, CursorAfterID: traceCursor, Limit: 1000})
	if err != nil {
		t.Fatalf("list capped trace page: %v", err)
	}
	if len(tracePage) != 105 || nextTraceCursor != "" || tracePage[0].ID != "trace-100" {
		t.Fatalf("second trace page = len:%d cursor:%q first:%v", len(tracePage), nextTraceCursor, tracePage[0].ID)
	}
	if _, _, err := service.listMemoryRecallTraces(ctx, workorchquerycontract.MemoryRecallTraceQuery{WorkspaceID: workspaceID, CursorAfterID: "missing"}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unknown trace cursor error = %v, want invalid argument", err)
	}
}

func TestServiceRejectsInvalidMemoryVectorsAndScope(t *testing.T) {
	ctx := context.Background()
	repo := backend.NewMemoryItemRepository()
	service := newService(repo)
	base := memoryservice.CreateMemoryCommand{
		MemoryID: "memory-vector", WorkspaceID: "ws-vector", SourceType: modulecommon.MemorySourceUser,
		MemoryType: modulecommon.MemoryTypeProjectContext, Summary: "vector memory",
	}
	for name, vector := range map[string][]float64{
		"nan":       {math.NaN()},
		"infinity":  {math.Inf(1)},
		"oversized": make([]float64, modulecommon.MemoryVectorMaxDimension+1),
	} {
		command := base
		command.MemoryID = common.MemoryID("memory-" + name)
		command.Vector = vector
		if err := service.createMemoryItem(ctx, command); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("create %s vector error = %v", name, err)
		}
	}
	if err := service.createMemoryItem(common.ContextWithRuntimeWorkspaceID(ctx, base.WorkspaceID), base); err != nil {
		t.Fatalf("create scoped memory: %v", err)
	}
	if _, err := service.getMemoryItemScoped(common.ContextWithRuntimeWorkspaceID(ctx, base.WorkspaceID), base.WorkspaceID, base.MemoryID); err != nil {
		t.Fatalf("get matching workspace memory: %v", err)
	}
	if _, err := service.getMemoryItemScoped(common.ContextWithRuntimeWorkspaceID(ctx, "ws-other"), "ws-other", base.MemoryID); err == nil {
		t.Fatal("cross-workspace memory get was accepted")
	}
	for name, query := range map[string]workorchquerycontract.MemorySearchQuery{
		"nan score":        {WorkspaceID: base.WorkspaceID, MinScore: math.NaN()},
		"infinite vector":  {WorkspaceID: base.WorkspaceID, Vector: []float64{math.Inf(-1)}},
		"oversized vector": {WorkspaceID: base.WorkspaceID, Vector: make([]float64, modulecommon.MemoryVectorMaxDimension+1)},
	} {
		if _, err := service.searchMemory(ctx, query); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("%s search error = %v", name, err)
		}
	}
}
