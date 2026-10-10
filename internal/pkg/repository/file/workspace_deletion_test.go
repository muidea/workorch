package file

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestWorkspaceDeletionJournalSurvivesFreshFileRepository(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workspaces.json")
	repo := NewWorkspaceRepository(path)
	workspaceID := common.WorkspaceID("ws-deletion-recovery")
	now := time.Now().UTC()
	record := workspacemodel.WorkspaceDeletionRecord{
		DeletionID: "deletion-recovery", DeletionGeneration: 1, RequestID: "request-recovery", Mode: deletioncontract.ModePurge,
		Phase: workspacemodel.WorkspaceDeletionPhaseRequested, Revision: 1, RequestedAt: now, UpdatedAt: now,
		SealedDeliverableDirectory: "/tmp/workorch/ws-deletion-recovery", SealedInternalDirectory: "/tmp/workorch-data/ws-deletion-recovery",
	}
	record.IntentHash = workspacemodel.StableWorkspaceDeletionIntentHash(workspaceID, record)
	item := &workspacemodel.Workspace{ID: workspaceID, Title: "recovery", DeliverableDirectory: record.SealedDeliverableDirectory, InternalDirectory: record.SealedInternalDirectory, State: common.WorkspaceStateActive, Deletion: &record}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatal(err)
	}

	fresh, err := NewWorkspaceRepository(path).Get(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Deletion == nil || fresh.Deletion.DeletionID != record.DeletionID || fresh.Deletion.IntentHash != record.IntentHash || fresh.Deletion.Phase != workspacemodel.WorkspaceDeletionPhaseRequested {
		t.Fatalf("fresh deletion journal = %+v, want %+v", fresh.Deletion, record)
	}
	if err := fresh.Deletion.Validate(workspaceID); err != nil {
		t.Fatalf("fresh deletion journal invalid: %v", err)
	}
}
