package workorchrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWorkspaceDeletionFileFenceSerializesIndependentRepositorySets(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	first := NewWorkspaceScopedFileRepositorySet(dataRoot)
	second := NewWorkspaceScopedFileRepositorySet(dataRoot)

	t.Run("memory", func(t *testing.T) {
		workspaceID := common.WorkspaceID("ws-memory-race")
		scope := deletioncontract.Scope{WorkspaceID: workspaceID, DeletionID: "delete-memory", DeletionGeneration: 1, Mode: deletioncontract.ModeRetain, QuiescenceID: "quiescence-memory"}
		scope.RequestHash = deletioncontract.StableRequestHash(scope)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		fence := &memorymodel.WorkspaceDeletion{
			WorkspaceID: workspaceID, DeletionID: scope.DeletionID, DeletionGeneration: 1, Mode: scope.Mode,
			QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash, FenceID: memoryevents.StableMemoryDeletionFenceID(scope),
			Phase: memorymodel.WorkspaceDeletionPhaseFenced, Revision: 1, SealAckHash: memoryevents.StableMemoryDeletionSealAckHash(scope), RequestedAt: now, UpdatedAt: now,
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			results <- first.MemoryItems.Create(ctx, &memorymodel.MemoryItem{ID: "memory-race", WorkspaceID: workspaceID})
		}()
		go func() {
			<-start
			results <- second.MemoryDeletions.Create(ctx, fence)
		}()
		close(start)
		for range 2 {
			err := <-results
			if err != nil && !errors.Is(err, common.ErrNotFound) {
				t.Fatalf("concurrent result = %v", err)
			}
		}
		if err := first.MemoryItems.Create(ctx, &memorymodel.MemoryItem{ID: "memory-after-fence", WorkspaceID: workspaceID}); !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("post-fence memory write = %v, want not found", err)
		}
		if _, err := NewWorkspaceScopedFileRepositorySet(dataRoot).MemoryDeletions.Get(ctx, workspaceID); err != nil {
			t.Fatalf("reopen memory tombstone: %v", err)
		}
	})

	t.Run("result", func(t *testing.T) {
		workspaceID := common.WorkspaceID("ws-result-race")
		scope := deletioncontract.Scope{WorkspaceID: workspaceID, DeletionID: "delete-result", DeletionGeneration: 1, Mode: deletioncontract.ModeRetain, QuiescenceID: "quiescence-result"}
		scope.RequestHash = deletioncontract.StableRequestHash(scope)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		fence := &resultmodel.WorkspaceDeletion{
			WorkspaceID: workspaceID, DeletionID: scope.DeletionID, DeletionGeneration: 1, Mode: scope.Mode,
			QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash, FenceID: resultevents.StableResultDeletionFenceID(scope),
			Phase: resultmodel.WorkspaceDeletionPhaseFenced, Revision: 1, SealAckHash: resultevents.StableResultDeletionSealAckHash(scope), RequestedAt: now, UpdatedAt: now,
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			results <- first.Results.Create(ctx, &resultmodel.Result{ID: "result-race", WorkspaceID: workspaceID, RunID: "run-race"})
		}()
		go func() {
			<-start
			results <- second.ResultDeletions.Create(ctx, fence)
		}()
		close(start)
		for range 2 {
			err := <-results
			if err != nil && !errors.Is(err, common.ErrNotFound) {
				t.Fatalf("concurrent result = %v", err)
			}
		}
		if err := first.Results.Create(ctx, &resultmodel.Result{ID: "result-after-fence", WorkspaceID: workspaceID, RunID: "run-race"}); !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("post-fence result write = %v, want not found", err)
		}
		if _, err := NewWorkspaceScopedFileRepositorySet(dataRoot).ResultDeletions.Get(ctx, workspaceID); err != nil {
			t.Fatalf("reopen result tombstone: %v", err)
		}
	})
}

func TestWorkspaceDeletionPurgeTombstonesPersistAcrossReopen(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	repositories := NewWorkspaceScopedFileRepositorySet(dataRoot)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	memoryScope := deletioncontract.Scope{WorkspaceID: "ws-memory-purged-reopen", DeletionID: "delete-memory", DeletionGeneration: 1, Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-memory"}
	memoryScope.RequestHash = deletioncontract.StableRequestHash(memoryScope)
	memoryFence := &memorymodel.WorkspaceDeletion{
		WorkspaceID: memoryScope.WorkspaceID, DeletionID: memoryScope.DeletionID, DeletionGeneration: 1, Mode: memoryScope.Mode,
		QuiescenceID: memoryScope.QuiescenceID, RequestHash: memoryScope.RequestHash, FenceID: memoryevents.StableMemoryDeletionFenceID(memoryScope),
		Phase: memorymodel.WorkspaceDeletionPhaseFenced, Revision: 1, SealAckHash: memoryevents.StableMemoryDeletionSealAckHash(memoryScope), RequestedAt: now, UpdatedAt: now,
	}
	if err := repositories.MemoryDeletions.Create(ctx, memoryFence); err != nil {
		t.Fatal(err)
	}
	memoryPurged := memorymodel.CloneWorkspaceDeletion(memoryFence)
	memoryPurged.Phase, memoryPurged.Revision = memorymodel.WorkspaceDeletionPhasePurged, 2
	memoryPurged.PurgeID = memoryevents.StableMemoryDeletionPurgeID(memoryScope)
	memoryPurged.PurgeProofHash = memoryevents.StableMemoryDeletionPurgeProofHash(memoryScope)
	memoryPurged.PurgeAckHash = memoryevents.StableMemoryDeletionPurgeAckHash(memoryScope)
	memoryPurged.PurgedAt, memoryPurged.UpdatedAt = now, now
	if err := repositories.MemoryDeletions.CompareAndSwap(ctx, memoryPurged, 1); err != nil {
		t.Fatal(err)
	}

	resultScope := deletioncontract.Scope{WorkspaceID: "ws-result-purged-reopen", DeletionID: "delete-result", DeletionGeneration: 1, Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-result"}
	resultScope.RequestHash = deletioncontract.StableRequestHash(resultScope)
	resultFence := &resultmodel.WorkspaceDeletion{
		WorkspaceID: resultScope.WorkspaceID, DeletionID: resultScope.DeletionID, DeletionGeneration: 1, Mode: resultScope.Mode,
		QuiescenceID: resultScope.QuiescenceID, RequestHash: resultScope.RequestHash, FenceID: resultevents.StableResultDeletionFenceID(resultScope),
		Phase: resultmodel.WorkspaceDeletionPhaseFenced, Revision: 1, SealAckHash: resultevents.StableResultDeletionSealAckHash(resultScope), RequestedAt: now, UpdatedAt: now,
	}
	if err := repositories.ResultDeletions.Create(ctx, resultFence); err != nil {
		t.Fatal(err)
	}
	resultPurged := resultmodel.CloneWorkspaceDeletion(resultFence)
	resultPurged.Phase, resultPurged.Revision = resultmodel.WorkspaceDeletionPhasePurged, 2
	resultPurged.PurgeID = resultevents.StableResultDeletionPurgeID(resultScope)
	resultPurged.PurgeProofHash = resultevents.StableResultDeletionPurgeProofHash(resultScope)
	resultPurged.PurgeAckHash = resultevents.StableResultDeletionPurgeAckHash(resultScope)
	resultPurged.PurgedAt, resultPurged.UpdatedAt = now, now
	if err := repositories.ResultDeletions.CompareAndSwap(ctx, resultPurged, 1); err != nil {
		t.Fatal(err)
	}

	reopened := NewWorkspaceScopedFileRepositorySet(dataRoot)
	if current, err := reopened.MemoryDeletions.Get(ctx, memoryScope.WorkspaceID); err != nil || current.Phase != memorymodel.WorkspaceDeletionPhasePurged || current.PurgeID != memoryPurged.PurgeID {
		t.Fatalf("reopened memory tombstone = %#v err=%v", current, err)
	}
	if current, err := reopened.ResultDeletions.Get(ctx, resultScope.WorkspaceID); err != nil || current.Phase != resultmodel.WorkspaceDeletionPhasePurged || current.PurgeID != resultPurged.PurgeID {
		t.Fatalf("reopened result tombstone = %#v err=%v", current, err)
	}
	if err := reopened.MemoryItems.Create(ctx, &memorymodel.MemoryItem{ID: "late", WorkspaceID: memoryScope.WorkspaceID}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("memory write after purged reopen = %v", err)
	}
	if err := reopened.Results.Create(ctx, &resultmodel.Result{ID: "late", WorkspaceID: resultScope.WorkspaceID, RunID: "run"}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("result write after purged reopen = %v", err)
	}
}

func mustWorkspaceScopedPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestWorkspaceScopedFileStoreRejectsInvalidCurrentIndex(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
	}{
		{name: "empty", data: "  \n"},
		{name: "unknown field", data: `{"workspaces":[],"retiredLayout":"v1"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newWorkspaceScopedFileStore(t.TempDir())
			if err := os.MkdirAll(filepath.Dir(store.workspaceIndexPath()), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(store.workspaceIndexPath(), []byte(tc.data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := store.loadWorkspaceIndexLocked(); err == nil {
				t.Fatal("invalid current workspace index was accepted")
			}
		})
	}
}

func TestWorkspaceScopedGovernanceGetRequiresUniqueUnscopedID(t *testing.T) {
	ctx := context.Background()
	repos := NewWorkspaceScopedFileRepositorySet(t.TempDir())
	for _, workspaceID := range []common.WorkspaceID{"ws-governance-a", "ws-governance-b"} {
		if err := repos.Runs.Create(ctx, &runmodel.Run{
			ID:          "run-shared-governance",
			WorkspaceID: workspaceID,
			State:       common.RunStateBlockedOnDecision,
		}); err != nil {
			t.Fatalf("create run %s: %v", workspaceID, err)
		}
		if err := repos.Tickets.Create(ctx, &decisionmodel.Ticket{
			ID:          "ticket-shared-governance",
			WorkspaceID: workspaceID,
			RunID:       "run-shared-governance",
			State:       common.DecisionTicketStateOpen,
			Reason:      "scope regression",
		}); err != nil {
			t.Fatalf("create ticket %s: %v", workspaceID, err)
		}
		if err := repos.Approvals.Create(ctx, &approvalmodel.Approval{
			ID:          "approval-shared-governance",
			WorkspaceID: workspaceID,
			RunID:       "run-shared-governance",
			State:       common.ApprovalStatePending,
			Reason:      "scope regression",
			Action:      "approve scoped action",
		}); err != nil {
			t.Fatalf("create approval %s: %v", workspaceID, err)
		}
	}

	if _, err := repos.Tickets.Get(ctx, "ticket-shared-governance"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate ticket get err = %v, want conflict", err)
	}
	if _, err := repos.Approvals.Get(ctx, "approval-shared-governance"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate approval get err = %v, want conflict", err)
	}

	scoped := common.ContextWithRuntimeWorkspaceID(ctx, "ws-governance-a")
	ticket, err := repos.Tickets.Get(scoped, "ticket-shared-governance")
	if err != nil {
		t.Fatalf("scoped ticket get: %v", err)
	}
	if ticket.WorkspaceID != "ws-governance-a" {
		t.Fatalf("scoped ticket workspace = %q, want ws-governance-a", ticket.WorkspaceID)
	}
	approval, err := repos.Approvals.Get(scoped, "approval-shared-governance")
	if err != nil {
		t.Fatalf("scoped approval get: %v", err)
	}
	if approval.WorkspaceID != "ws-governance-a" {
		t.Fatalf("scoped approval workspace = %q, want ws-governance-a", approval.WorkspaceID)
	}
}

func TestWorkspaceScopedFileApprovalRepositoryAddressesExactRunScope(t *testing.T) {
	ctx := context.Background()
	repos := NewWorkspaceScopedFileRepositorySet(t.TempDir())
	const (
		workspaceID = common.WorkspaceID("ws-file-approval-run-scope")
		approvalID  = common.ApprovalID("approval-shared-across-file-runs")
	)
	for _, runID := range []common.RunID{"run-file-approval-a", "run-file-approval-b"} {
		if err := repos.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
			t.Fatalf("create run %s: %v", runID, err)
		}
		if err := repos.Approvals.Create(ctx, &approvalmodel.Approval{
			ID: approvalID, WorkspaceID: workspaceID, RunID: runID,
			State: common.ApprovalStatePending, Reason: "run-scoped file approval",
		}); err != nil {
			t.Fatalf("create approval for %s: %v", runID, err)
		}
	}
	workspaceCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	wrongWorkspaceCtx := common.ContextWithRuntimeWorkspaceID(ctx, "ws-file-approval-wrong-scope")
	if _, err := repos.Approvals.Get(workspaceCtx, approvalID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("workspace-only duplicate get err = %v, want conflict", err)
	}
	if _, err := repos.Approvals.GetScoped(wrongWorkspaceCtx, workspaceID, "run-file-approval-a", approvalID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong workspace context get err = %v, want conflict", err)
	}
	if _, err := repos.Approvals.ListByWorkspaceID(wrongWorkspaceCtx, workspaceID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong workspace context list err = %v, want conflict", err)
	}
	first, err := repos.Approvals.GetScoped(workspaceCtx, workspaceID, "run-file-approval-a", approvalID)
	if err != nil {
		t.Fatal(err)
	}
	if first.RunID != "run-file-approval-a" {
		t.Fatalf("first scoped approval run = %q", first.RunID)
	}
	first.State = common.ApprovalStateApproved
	if err := repos.Approvals.Update(wrongWorkspaceCtx, first); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong workspace context update err = %v, want conflict", err)
	}
	first, err = repos.Approvals.GetScoped(workspaceCtx, workspaceID, "run-file-approval-a", approvalID)
	if err != nil || first.State != common.ApprovalStatePending {
		t.Fatalf("approval after rejected cross-scope update = %+v err=%v", first, err)
	}
	if err := repos.Approvals.Create(wrongWorkspaceCtx, &approvalmodel.Approval{
		ID: "approval-wrong-context-create", WorkspaceID: workspaceID, RunID: "run-file-approval-a",
		State: common.ApprovalStatePending, Reason: "must not cross workspace context",
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong workspace context create err = %v, want conflict", err)
	}
	if err := repos.Approvals.DeleteScoped(workspaceCtx, workspaceID, "run-file-approval-a", approvalID); err != nil {
		t.Fatal(err)
	}
	second, err := repos.Approvals.GetScoped(workspaceCtx, workspaceID, "run-file-approval-b", approvalID)
	if err != nil {
		t.Fatalf("second scoped approval after first delete: %v", err)
	}
	if second.RunID != "run-file-approval-b" {
		t.Fatalf("second scoped approval run = %q", second.RunID)
	}
}

func TestWorkspaceScopedDefinitionGetRequiresUniqueUnscopedID(t *testing.T) {
	ctx := context.Background()
	repos := NewWorkspaceScopedFileRepositorySet(t.TempDir())
	for _, workspaceID := range []common.WorkspaceID{"ws-definition-a", "ws-definition-b"} {
		if err := repos.Goals.Create(ctx, &workspacemodel.Goal{
			ID:          "goal-shared-definition",
			WorkspaceID: workspaceID,
			Text:        "scoped goal",
		}); err != nil {
			t.Fatalf("create goal %s: %v", workspaceID, err)
		}
		if err := repos.DoDs.Create(ctx, &workspacemodel.DoD{
			ID:          "dod-shared-definition",
			WorkspaceID: workspaceID,
			Items:       []string{"scoped dod"},
			State:       "ready",
		}); err != nil {
			t.Fatalf("create dod %s: %v", workspaceID, err)
		}
	}

	if _, err := repos.Goals.Get(ctx, "goal-shared-definition"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate goal get err = %v, want conflict", err)
	}
	if _, err := repos.DoDs.Get(ctx, "dod-shared-definition"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate dod get err = %v, want conflict", err)
	}

	scoped := common.ContextWithRuntimeWorkspaceID(ctx, "ws-definition-b")
	goal, err := repos.Goals.Get(scoped, "goal-shared-definition")
	if err != nil {
		t.Fatalf("scoped goal get: %v", err)
	}
	if goal.WorkspaceID != "ws-definition-b" {
		t.Fatalf("scoped goal workspace = %q, want ws-definition-b", goal.WorkspaceID)
	}
	dod, err := repos.DoDs.Get(scoped, "dod-shared-definition")
	if err != nil {
		t.Fatalf("scoped dod get: %v", err)
	}
	if dod.WorkspaceID != "ws-definition-b" {
		t.Fatalf("scoped dod workspace = %q, want ws-definition-b", dod.WorkspaceID)
	}
}

func TestWorkspaceScopedRunLocalGetRequiresUniqueUnscopedID(t *testing.T) {
	ctx := context.Background()
	repos := NewWorkspaceScopedFileRepositorySet(t.TempDir())
	for _, workspaceID := range []common.WorkspaceID{"ws-runlocal-a", "ws-runlocal-b"} {
		scoped := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
		if err := repos.Runs.Create(scoped, &runmodel.Run{
			ID:          "run-shared-local",
			WorkspaceID: workspaceID,
			State:       common.RunStateCompleted,
		}); err != nil {
			t.Fatalf("create run %s: %v", workspaceID, err)
		}
		if err := repos.Steps.Create(scoped, &runmodel.Step{
			ID:    "run-shared-local/step-001",
			RunID: "run-shared-local",
			Kind:  "validation",
			State: common.StepStateSucceeded,
		}); err != nil {
			t.Fatalf("create step %s: %v", workspaceID, err)
		}
	}

	if _, err := repos.Runs.Get(ctx, "run-shared-local"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate run get err = %v, want conflict", err)
	}
	if _, err := repos.Steps.Get(ctx, "run-shared-local/step-001"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate step get err = %v, want conflict", err)
	}
	if _, err := repos.Steps.ListByRun(ctx, "run-shared-local"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate step list err = %v, want conflict", err)
	}

	scoped := common.ContextWithRuntimeWorkspaceID(ctx, "ws-runlocal-a")
	run, err := repos.Runs.Get(scoped, "run-shared-local")
	if err != nil {
		t.Fatalf("scoped run get: %v", err)
	}
	if run.WorkspaceID != "ws-runlocal-a" {
		t.Fatalf("scoped run workspace = %q, want ws-runlocal-a", run.WorkspaceID)
	}
	step, err := repos.Steps.Get(scoped, "run-shared-local/step-001")
	if err != nil {
		t.Fatalf("scoped step get: %v", err)
	}
	if step.RunID != "run-shared-local" {
		t.Fatalf("scoped step run = %q, want run-shared-local", step.RunID)
	}
	steps, err := repos.Steps.ListByRun(scoped, "run-shared-local")
	if err != nil {
		t.Fatalf("scoped step list: %v", err)
	}
	if len(steps) != 1 || steps[0].ID != "run-shared-local/step-001" {
		t.Fatalf("scoped step list = %#v, want one scoped step", steps)
	}
}

func TestWorkspaceScopedRunGenerationAndIDSurviveConcurrentCreateDeletePurgeAndRestart(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := common.WorkspaceID("ws-run-generation")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	repos := NewWorkspaceScopedFileRepositorySet(dataRoot)

	// Low-level explicit terminal fixtures still use the same owner-persistence
	// head. Concurrent commits must receive distinct generations.
	items := []*runmodel.Run{
		{ID: "historical-a", WorkspaceID: workspaceID, State: common.RunStateCompleted},
		{ID: "historical-b", WorkspaceID: workspaceID, State: common.RunStateFailed},
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(items))
	for _, item := range items {
		item := item
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- repos.Runs.Create(ctx, item)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Run create: %v", err)
		}
	}
	if items[0].WorkspaceRunGeneration == items[1].WorkspaceRunGeneration ||
		(items[0].WorkspaceRunGeneration != 1 && items[0].WorkspaceRunGeneration != 2) ||
		(items[1].WorkspaceRunGeneration != 1 && items[1].WorkspaceRunGeneration != 2) {
		t.Fatalf("concurrent generations = %d, %d; want distinct 1/2", items[0].WorkspaceRunGeneration, items[1].WorkspaceRunGeneration)
	}
	// A response-loss replay sees the durable identity and cannot consume a new
	// generation.
	if err := repos.Runs.Create(ctx, &runmodel.Run{ID: items[0].ID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate replay err=%v, want conflict", err)
	}
	for _, item := range items {
		if err := repos.Runs.Delete(ctx, item.ID); err != nil {
			t.Fatalf("delete %s: %v", item.ID, err)
		}
	}
	// Simulate workspace runtime-directory purge. The Run-owner high-watermark
	// lives under system state and must survive a fresh repository set.
	if err := os.RemoveAll(filepath.Join(dataRoot, "internal", "workspaces", string(workspaceID), "runs")); err != nil {
		t.Fatal(err)
	}
	restarted := NewWorkspaceScopedFileRepositorySet(dataRoot)
	next := &runmodel.Run{WorkspaceID: workspaceID, State: common.RunStatePending}
	if err := restarted.Runs.CreateNextWorkspaceRun(ctx, next); err != nil {
		t.Fatalf("create after purge/restart: %v", err)
	}
	if next.WorkspaceRunGeneration != 3 || next.ID != "run-003" {
		t.Fatalf("post-purge Run = id:%s generation:%d, want run-003/3", next.ID, next.WorkspaceRunGeneration)
	}
	if err := restarted.Runs.CreateNextWorkspaceRun(ctx, &runmodel.Run{WorkspaceID: workspaceID, State: common.RunStatePending}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("second live Run err=%v, want conflict", err)
	}
	stored, err := restarted.Runs.GetByWorkspaceID(ctx, workspaceID, next.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.State = common.RunStateCompleted
	if err := restarted.Runs.CompareAndSwap(ctx, stored, stored.Revision); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Runs.Delete(ctx, stored.ID); err != nil {
		t.Fatal(err)
	}
	last := &runmodel.Run{WorkspaceID: workspaceID, State: common.RunStatePending}
	if err := NewWorkspaceScopedFileRepositorySet(dataRoot).Runs.CreateNextWorkspaceRun(ctx, last); err != nil {
		t.Fatal(err)
	}
	if last.WorkspaceRunGeneration != 4 || last.ID != "run-004" {
		t.Fatalf("Run identity was reused after delete/restart: %+v", last)
	}
}

func TestWorkspaceScopedRunRejectsRawBootstrapBeforeAllocatingGeneration(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := common.WorkspaceID("ws-invalid-bootstrap-generation")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	repos := NewWorkspaceScopedFileRepositorySet(dataRoot)
	policy := runtimecontract.RuntimeEvidencePolicy{
		ID: "policy-1", AppliesTo: runtimecontract.StepKindFileWrite,
		CoverageRequirements: []runtimecontract.RuntimeCoverageEvidenceRequirement{{
			ID: "dod-1", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			ValidationRecipeID: "recipe-1", RequiredCheckRefs: []string{"check-1", "check-1"},
		}},
	}
	bootstrap := runtimecontract.RuntimeBootstrap{Activation: runtimecontract.RuntimeActivationContext{EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{policy}}}
	invalid := &runmodel.Run{WorkspaceID: workspaceID, State: common.RunStatePending, RuntimeBootstrap: &bootstrap}
	if err := repos.Runs.CreateNextWorkspaceRun(ctx, invalid); err == nil || !strings.Contains(err.Error(), "evidence polic") {
		t.Fatalf("workspace-scoped Run accepted noncanonical bootstrap: %v", err)
	}
	if invalid.ID != "" || invalid.WorkspaceRunGeneration != 0 {
		t.Fatalf("rejected Run consumed identity: id=%q generation=%d", invalid.ID, invalid.WorkspaceRunGeneration)
	}
	headPath := filepath.Join(dataRoot, "system", workspaceScopedRunGenerationDir, string(workspaceID)+".json")
	if _, err := os.Stat(headPath); !os.IsNotExist(err) {
		t.Fatalf("rejected Run persisted generation head: %v", err)
	}

	valid := &runmodel.Run{WorkspaceID: workspaceID, State: common.RunStatePending}
	if err := repos.Runs.CreateNextWorkspaceRun(ctx, valid); err != nil {
		t.Fatalf("create valid Run after rejected bootstrap: %v", err)
	}
	if valid.ID != "run-001" || valid.WorkspaceRunGeneration != 1 {
		t.Fatalf("rejected bootstrap consumed generation: %+v", valid)
	}
}

func TestWorkspaceScopedRunGenerationSerializesIndependentRepositorySets(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := common.WorkspaceID("ws-independent-run-generation")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	repositories := []WorkspaceScopedFileRepositorySet{
		NewWorkspaceScopedFileRepositorySet(dataRoot),
		NewWorkspaceScopedFileRepositorySet(dataRoot),
	}
	items := []*runmodel.Run{
		{ID: "historical-a", WorkspaceID: workspaceID, State: common.RunStateCompleted},
		{ID: "historical-b", WorkspaceID: workspaceID, State: common.RunStateFailed},
	}
	errs := make(chan error, len(items))
	var wg sync.WaitGroup
	for idx := range items {
		idx := idx
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- repositories[idx].Runs.Create(ctx, items[idx])
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("independent repository create: %v", err)
		}
	}
	if items[0].WorkspaceRunGeneration == items[1].WorkspaceRunGeneration ||
		(items[0].WorkspaceRunGeneration != 1 && items[0].WorkspaceRunGeneration != 2) ||
		(items[1].WorkspaceRunGeneration != 1 && items[1].WorkspaceRunGeneration != 2) {
		t.Fatalf("independent repository generations = %d/%d, want distinct 1/2", items[0].WorkspaceRunGeneration, items[1].WorkspaceRunGeneration)
	}
}

func TestWorkspaceScopedArtifactListByResultHonorsWorkspaceContext(t *testing.T) {
	ctx := context.Background()
	repos := NewWorkspaceScopedFileRepositorySet(t.TempDir())
	for idx, workspaceID := range []common.WorkspaceID{"ws-artifact-a", "ws-artifact-b"} {
		runID := common.RunID("run-artifact-" + string(rune('a'+idx)))
		scoped := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
		if err := repos.Runs.Create(scoped, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
			t.Fatalf("create run %s: %v", workspaceID, err)
		}
		if err := repos.Artifacts.Create(scoped, &resultmodel.Artifact{
			ID: common.ArtifactID("artifact-" + string(rune('a'+idx))), WorkspaceID: workspaceID, RunID: runID,
			ResultID: "result-shared", State: common.ArtifactStateAvailable,
		}); err != nil {
			t.Fatalf("create artifact %s: %v", workspaceID, err)
		}
	}

	items, err := repos.Artifacts.ListByResult(common.ContextWithRuntimeWorkspaceID(ctx, "ws-artifact-a"), "result-shared")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].WorkspaceID != "ws-artifact-a" || items[0].ID != "artifact-a" {
		t.Fatalf("scoped artifacts = %+v", items)
	}
	if _, err := repos.Artifacts.ListByResult(ctx, "result-shared"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate result query err = %v, want conflict", err)
	}
}

func TestWorkspaceScopedResultRecoveryInventoryIsBounded(t *testing.T) {
	ctx := context.Background()
	repos := NewWorkspaceScopedFileRepositorySet(t.TempDir())
	for idx, workspaceID := range []common.WorkspaceID{"ws-result-recovery-a", "ws-result-recovery-b", "ws-result-recovery-c"} {
		runID := common.RunID("run-result-recovery-" + string(rune('a'+idx)))
		if err := repos.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
			t.Fatal(err)
		}
		if err := repos.Results.Create(ctx, &resultmodel.Result{
			ID: common.ResultID("result-recovery-" + string(rune('a'+idx))), WorkspaceID: workspaceID, RunID: runID,
			State: common.ResultStateAccepted,
		}); err != nil {
			t.Fatal(err)
		}
	}
	inventory, ok := repos.Results.(repository.ResultRecoveryRepository)
	if !ok {
		t.Fatalf("workspace-scoped Result repository %T has no recovery inventory", repos.Results)
	}
	first, next, err := inventory.ListForRecovery(ctx, "", 2)
	if err != nil || len(first) != 2 || next == "" {
		t.Fatalf("first recovery page = %+v next=%q err=%v", first, next, err)
	}
	last, finalCursor, err := inventory.ListForRecovery(ctx, next, 2)
	if err != nil || len(last) != 1 || finalCursor != "" {
		t.Fatalf("last recovery page = %+v next=%q err=%v", last, finalCursor, err)
	}
}

func TestWorkspaceScopedPreSealDecisionPathSupportsWorkspaceCRUDAndReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repositories := NewWorkspaceScopedFileRepositorySet(root)
	workspaceID := common.WorkspaceID("ws-preseal-decision-crud")
	ticketID := common.TicketID("ticket-preseal-decision-crud")
	scoped := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	ticket := &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		State:       common.DecisionTicketStateOpen,
		DefinitionClarification: &decisionmodel.DefinitionClarificationBinding{
			Class: decisionmodel.DefinitionClarificationWorkspacePreSeal,
		},
	}
	if err := repositories.Tickets.Create(scoped, ticket); err != nil {
		t.Fatalf("create workspace-scoped pre-seal ticket: %v", err)
	}
	if err := repositories.Tickets.Create(scoped, &decisionmodel.Ticket{
		ID:          "ticket-runtime-without-run",
		WorkspaceID: workspaceID,
		DefinitionClarification: &decisionmodel.DefinitionClarificationBinding{
			Class: decisionmodel.DefinitionClarificationRuntime,
		},
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create runless Runtime clarification err=%v, want invalid argument", err)
	}
	listed, err := repositories.Tickets.ListByWorkspaceID(scoped, workspaceID)
	if err != nil || len(listed) != 1 || listed[0].ID != ticketID || listed[0].RunID != "" {
		t.Fatalf("workspace ticket list = %+v err=%v", listed, err)
	}

	resolution := &decisionmodel.Resolution{
		ID:         "resolution-preseal-decision-crud",
		TicketID:   ticketID,
		ResolvedAt: "2026-08-30T00:00:00Z",
		Type:       "candidate_adjusted",
	}
	if err := repositories.Resolutions.Create(scoped, resolution); err != nil {
		t.Fatalf("create workspace-scoped resolution: %v", err)
	}
	resolved := decisionmodel.CloneTicket(ticket)
	resolved.State = common.DecisionTicketStateResolved
	resolved.ResolvedAt = resolution.ResolvedAt
	resolved.DefinitionClarification.ResolutionID = resolution.ID
	if err := repositories.Tickets.Update(scoped, resolved); err != nil {
		t.Fatalf("update workspace-scoped ticket: %v", err)
	}

	reopened := NewWorkspaceScopedFileRepositorySet(root)
	got, err := reopened.Tickets.Get(scoped, ticketID)
	if err != nil || got.State != common.DecisionTicketStateResolved || got.RunID != "" || got.DefinitionClarification == nil || got.DefinitionClarification.ResolutionID != resolution.ID {
		t.Fatalf("reopened workspace-scoped ticket = %+v err=%v", got, err)
	}
	if gotResolution, loadErr := reopened.Resolutions.GetByTicketID(scoped, ticketID); loadErr != nil || gotResolution.ID != resolution.ID {
		t.Fatalf("reopened workspace-scoped resolution = %+v err=%v", gotResolution, loadErr)
	}
	if err := reopened.Resolutions.DeleteByTicketID(scoped, ticketID); err != nil {
		t.Fatalf("delete workspace-scoped resolution: %v", err)
	}
	if err := reopened.Tickets.Delete(scoped, ticketID); err != nil {
		t.Fatalf("delete workspace-scoped ticket: %v", err)
	}
	afterDelete := NewWorkspaceScopedFileRepositorySet(root)
	if _, loadErr := afterDelete.Tickets.Get(scoped, ticketID); !errors.Is(loadErr, common.ErrNotFound) {
		t.Fatalf("reopened deleted workspace-scoped ticket err=%v, want not found", loadErr)
	}
	if _, loadErr := afterDelete.Resolutions.GetByTicketID(scoped, ticketID); !errors.Is(loadErr, common.ErrNotFound) {
		t.Fatalf("reopened deleted workspace-scoped resolution err=%v, want not found", loadErr)
	}
}

func TestWorkspaceScopedEventRecordListHonorsRunScopeCursorAndLimit(t *testing.T) {
	ctx := context.Background()
	repos := NewWorkspaceScopedFileRepositorySet(t.TempDir())
	workspaceID := common.WorkspaceID("ws-event-scope")
	scoped := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	for _, runID := range []common.RunID{"run-a", "run-b"} {
		if err := repos.Runs.Create(scoped, &runmodel.Run{
			ID:          runID,
			WorkspaceID: workspaceID,
			State:       common.RunStateCompleted,
		}); err != nil {
			t.Fatalf("create run %s: %v", runID, err)
		}
	}
	records := []*eventstreammodel.EventRecord{
		{
			ID:          "evt-run-a-001",
			Topic:       "/run/state/changed",
			OccurredAt:  "2026-06-16T00:01:00Z",
			WorkspaceID: workspaceID,
			RunID:       "run-a",
			Payload:     mustWorkspaceScopedPayload(t, map[string]any{"state": "running"}),
		},
		{
			ID:          "evt-run-a-002",
			Topic:       "/step/state/changed",
			OccurredAt:  "2026-06-16T00:02:00Z",
			WorkspaceID: workspaceID,
			RunID:       "run-a",
			Payload:     mustWorkspaceScopedPayload(t, map[string]any{"state": "completed"}),
		},
		{
			ID:          "evt-run-b-001",
			Topic:       "/run/state/changed",
			OccurredAt:  "2026-06-16T00:03:00Z",
			WorkspaceID: workspaceID,
			RunID:       "run-b",
			Payload:     mustWorkspaceScopedPayload(t, map[string]any{"state": "running"}),
		},
	}
	for _, record := range records {
		if err := repos.EventRecords.Append(scoped, record); err != nil {
			t.Fatalf("append event %s: %v", record.ID, err)
		}
	}

	firstPage, err := repos.EventRecords.List(scoped, workorchquerycontract.EventRecordQuery{
		WorkspaceID: workspaceID,
		RunID:       "run-a",
		Limit:       1,
	})
	if err != nil {
		t.Fatalf("list first run page: %v", err)
	}
	if len(firstPage) != 1 || firstPage[0].ID != "evt-run-a-002" {
		t.Fatalf("first run page = %+v, want latest evt-run-a-002", firstPage)
	}
	nextPage, err := repos.EventRecords.List(scoped, workorchquerycontract.EventRecordQuery{
		WorkspaceID:   workspaceID,
		RunID:         "run-a",
		CursorAfterID: "evt-run-a-001",
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("list next run page: %v", err)
	}
	if len(nextPage) != 1 || nextPage[0].ID != "evt-run-a-002" {
		t.Fatalf("next run page = %+v, want only evt-run-a-002", nextPage)
	}
	workspaceLatest, err := repos.EventRecords.List(scoped, workorchquerycontract.EventRecordQuery{
		WorkspaceID: workspaceID,
		Limit:       2,
	})
	if err != nil {
		t.Fatalf("list workspace latest: %v", err)
	}
	if len(workspaceLatest) != 2 || workspaceLatest[0].ID != "evt-run-a-002" || workspaceLatest[1].ID != "evt-run-b-001" {
		t.Fatalf("workspace latest = %+v, want newest two in chronological order", workspaceLatest)
	}
}

func TestWorkspaceScopedEventRecordWorkspaceLatestUsesBoundedPerRunLists(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repos := NewWorkspaceScopedFileRepositorySet(root)
	workspaceID := common.WorkspaceID("ws-event-bounded")
	scoped := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	for _, runID := range []common.RunID{"run-a", "run-b"} {
		if err := repos.Runs.Create(scoped, &runmodel.Run{
			ID:          runID,
			WorkspaceID: workspaceID,
			// Event retention is independent of the current Run lifecycle.
			// Historical fixtures must not violate the Run owner's single-current
			// Workspace invariant.
			State: common.RunStateCompleted,
		}); err != nil {
			t.Fatalf("create run %s: %v", runID, err)
		}
	}
	for _, record := range []*eventstreammodel.EventRecord{
		{
			ID:          "evt-run-a-001",
			Topic:       "/step/state/changed",
			OccurredAt:  "2026-06-16T00:01:00Z",
			WorkspaceID: workspaceID,
			RunID:       "run-a",
			Payload:     mustWorkspaceScopedPayload(t, map[string]any{"state": "old"}),
		},
		{
			ID:          "evt-run-a-002",
			Topic:       "/step/state/changed",
			OccurredAt:  "2026-06-16T00:02:00Z",
			WorkspaceID: workspaceID,
			RunID:       "run-a",
			Payload:     mustWorkspaceScopedPayload(t, map[string]any{"state": "latest-a"}),
		},
		{
			ID:          "evt-run-b-001",
			Topic:       "/run/state/changed",
			OccurredAt:  "2026-06-16T00:03:00Z",
			WorkspaceID: workspaceID,
			RunID:       "run-b",
			Payload:     mustWorkspaceScopedPayload(t, map[string]any{"state": "latest-b"}),
		},
	} {
		if err := repos.EventRecords.Append(scoped, record); err != nil {
			t.Fatalf("append event %s: %v", record.ID, err)
		}
	}
	corruptFirstJSONLLine(t, filepath.Join(root, "internal", "workspaces", string(workspaceID), "runs", "run-a", "events", "events-000001.jsonl"))

	workspaceLatest, err := repos.EventRecords.List(scoped, workorchquerycontract.EventRecordQuery{
		WorkspaceID: workspaceID,
		Limit:       1,
	})
	if err != nil {
		t.Fatalf("list workspace latest with bounded per-run reads: %v", err)
	}
	if len(workspaceLatest) != 1 || workspaceLatest[0].ID != "evt-run-b-001" {
		t.Fatalf("workspace latest = %+v, want newest run-b event without scanning corrupt old run-a event", workspaceLatest)
	}
}

func TestWorkspaceScopedEventPurgePreservesOwnerFilesAndGlobalPosition(t *testing.T) {
	ctx := context.Background()
	repos := NewWorkspaceScopedFileRepositorySet(t.TempDir())
	workspaceID := common.WorkspaceID("ws-event-purge-files")
	runID := common.RunID("run-event-purge-files")
	scoped := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	if err := repos.Workspaces.Create(scoped, &workspacemodel.Workspace{ID: workspaceID, Title: "preserve me", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	if err := repos.Runs.Create(scoped, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	positions := make([]int64, 2)
	for index := range positions {
		position, err := repos.EventRecords.ReserveEventRecordStreamPosition(ctx)
		if err != nil {
			t.Fatal(err)
		}
		positions[index] = position
	}
	for _, item := range []*eventstreammodel.EventRecord{
		{ID: "event-purge-system", Topic: "/test", OccurredAt: "2026-08-25T00:04:00Z", WorkspaceID: workspaceID, StreamPosition: positions[0], Revision: 1},
		{ID: "event-purge-run", Topic: "/test", OccurredAt: "2026-08-25T00:04:01Z", WorkspaceID: workspaceID, RunID: runID, StreamPosition: positions[1], Revision: 2},
	} {
		if err := repos.EventRecords.Append(scoped, item); err != nil {
			t.Fatal(err)
		}
	}
	scope := deletioncontract.Scope{
		WorkspaceID: workspaceID, DeletionID: "delete-event-purge-files", DeletionGeneration: 1,
		Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-event-purge-files",
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	fence := &eventstreammodel.WorkspaceDeletion{
		WorkspaceID: workspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		Mode: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
		FenceID: "eventstream-fence-event-purge-files", Phase: eventstreammodel.WorkspaceDeletionPhaseFenced,
		Revision: 1, SealAckHash: "eventstream-seal-ack-event-purge-files", RequestedAt: now, UpdatedAt: now,
	}
	if err := repos.EventStreamDeletions.Create(scoped, fence); err != nil {
		t.Fatal(err)
	}
	if deleted, err := repos.EventStreamPurger.PurgeWorkspaceEventRecords(scoped, *fence); err != nil || deleted != 2 {
		t.Fatalf("purge deleted=%d err=%v", deleted, err)
	}
	if workspace, err := repos.Workspaces.Get(scoped, workspaceID); err != nil || workspace.Title != "preserve me" {
		t.Fatalf("workspace after event purge = %+v err=%v", workspace, err)
	}
	if run, err := repos.Runs.Get(scoped, runID); err != nil || run.State != common.RunStateActive {
		t.Fatalf("run after event purge = %+v err=%v", run, err)
	}
	if items, err := repos.EventRecords.List(scoped, workorchquerycontract.EventRecordQuery{WorkspaceID: workspaceID}); err != nil || len(items) != 0 {
		t.Fatalf("events after purge = %+v err=%v", items, err)
	}
	thirdPosition, err := repos.EventRecords.ReserveEventRecordStreamPosition(ctx)
	if err != nil || thirdPosition <= positions[1] {
		t.Fatalf("post-purge position=%d err=%v, want > %d", thirdPosition, err, positions[1])
	}
}

func corruptFirstJSONLLine(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read event jsonl %s: %v", path, err)
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		t.Fatalf("event jsonl %s has no first line to corrupt", path)
	}
	lines[0] = "{not-json"
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("write corrupt event jsonl %s: %v", path, err)
	}
}
