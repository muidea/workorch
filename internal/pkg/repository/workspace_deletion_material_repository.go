package repository

import (
	"context"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// WorkspaceDeletionMaterialCleanupScope is the exact Workspace-owner fence
// for removing ordinary Workspace configuration material. It carries no
// target phase: only the Workspace private deletion dispatcher may construct
// it from the currently persisted directory_cleanup aggregate.
type WorkspaceDeletionMaterialCleanupScope struct {
	WorkspaceID               common.WorkspaceID
	DeletionID                string
	DeletionGeneration        uint64
	IntentHash                string
	ExpectedWorkspaceRevision uint64
	ExpectedDeletionRevision  uint64
	CleanupID                 string
}

func (s WorkspaceDeletionMaterialCleanupScope) Validate() error {
	if s.WorkspaceID == "" || strings.TrimSpace(s.DeletionID) == "" || s.DeletionID != strings.TrimSpace(s.DeletionID) ||
		s.DeletionGeneration != 1 || strings.TrimSpace(s.IntentHash) == "" || s.IntentHash != strings.TrimSpace(s.IntentHash) ||
		s.ExpectedWorkspaceRevision == 0 || s.ExpectedDeletionRevision == 0 ||
		strings.TrimSpace(s.CleanupID) == "" || s.CleanupID != strings.TrimSpace(s.CleanupID) {
		return common.ErrInvalidArgument
	}
	return nil
}

func ValidateWorkspaceDeletionMaterialCleanupContext(ctx context.Context, scope WorkspaceDeletionMaterialCleanupScope) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if common.RuntimeWorkspaceIDFromContext(ctx) != scope.WorkspaceID {
		return common.ErrConflict
	}
	return nil
}

// ValidateWorkspaceDeletionMaterialCleanupCurrent proves that the requested
// physical cleanup still belongs to the exact current Workspace deletion
// aggregate. A stale or forged identity fails closed before any side effect.
func ValidateWorkspaceDeletionMaterialCleanupCurrent(current *workspacemodel.Workspace, scope WorkspaceDeletionMaterialCleanupScope) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if current == nil || current.ID != scope.WorkspaceID || current.Revision != scope.ExpectedWorkspaceRevision ||
		current.Deletion == nil || current.Deletion.Validate(current.ID) != nil {
		return common.ErrConflict
	}
	record := current.Deletion
	if record.Phase != workspacemodel.WorkspaceDeletionPhaseDirectoryCleanup || record.DeletionID != scope.DeletionID ||
		record.DeletionGeneration != scope.DeletionGeneration || record.IntentHash != scope.IntentHash ||
		record.Revision != scope.ExpectedDeletionRevision ||
		workspacemodel.StableWorkspaceDeletionCleanupID(record.Clone()) != scope.CleanupID {
		return common.ErrConflict
	}
	return nil
}

// WorkspaceDeletionMaterialCleaner is an owner repository sink, not a public
// service facade. Production callers are restricted to Workspace's private
// directory_cleanup_requested dispatcher.
type WorkspaceDeletionMaterialCleaner interface {
	CleanupDeletionMaterial(context.Context, WorkspaceDeletionMaterialCleanupScope) error
}
