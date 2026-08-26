package workorchrepo

import (
	"context"
	"errors"
	"os"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

type eventStreamWorkspaceDeletionRepository struct {
	dataRoot     string
	ownerBinding repository.OwnerRepositoryBinding
}

func newEventStreamWorkspaceDeletionRepository(dataRoot string, binding repository.OwnerRepositoryBinding) *eventStreamWorkspaceDeletionRepository {
	return &eventStreamWorkspaceDeletionRepository{dataRoot: dataRoot, ownerBinding: binding}
}

func (r *eventStreamWorkspaceDeletionRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *eventStreamWorkspaceDeletionRepository) Create(_ context.Context, item *eventstreammodel.WorkspaceDeletion) error {
	if r == nil || item == nil || item.Validate() != nil || item.Revision != 1 {
		return common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(r.dataRoot, "eventstream", item.WorkspaceID)
	if err != nil {
		return err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	if _, err := os.Stat(path); err == nil {
		return common.ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeWorkspaceDeletionFile(path, eventstreammodel.CloneWorkspaceDeletion(item))
}

func (r *eventStreamWorkspaceDeletionRepository) Get(_ context.Context, workspaceID common.WorkspaceID) (*eventstreammodel.WorkspaceDeletion, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(r.dataRoot, "eventstream", workspaceID)
	if err != nil {
		return nil, err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	item, err := readWorkspaceDeletionFile[eventstreammodel.WorkspaceDeletion](path)
	if err != nil {
		return nil, err
	}
	if item.WorkspaceID != workspaceID || item.Validate() != nil {
		return nil, common.ErrConflict
	}
	return eventstreammodel.CloneWorkspaceDeletion(item), nil
}

func (r *eventStreamWorkspaceDeletionRepository) CompareAndSwap(_ context.Context, item *eventstreammodel.WorkspaceDeletion, expected uint64) error {
	if r == nil || item == nil || item.Validate() != nil || expected == 0 || item.Revision != expected+1 {
		return common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(r.dataRoot, "eventstream", item.WorkspaceID)
	if err != nil {
		return err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	current, err := readWorkspaceDeletionFile[eventstreammodel.WorkspaceDeletion](path)
	if err != nil {
		return err
	}
	if current.Validate() != nil || current.Revision != expected || !sameEventStreamWorkspaceDeletion(current, item, false) {
		return common.ErrConflict
	}
	return writeWorkspaceDeletionFile(path, eventstreammodel.CloneWorkspaceDeletion(item))
}

func (r *eventStreamWorkspaceDeletionRepository) WithWorkspaceActiveFence(ctx context.Context, workspaceID common.WorkspaceID, operation func(context.Context) error) error {
	return withWorkspaceDeletionActiveFence(ctx, r.dataRoot, "eventstream", workspaceID, operation)
}

func (r *eventStreamWorkspaceDeletionRepository) WithWorkspacePurgeFence(ctx context.Context, expected eventstreammodel.WorkspaceDeletion, operation func(context.Context) error) error {
	if r == nil || expected.Validate() != nil || expected.Phase != eventstreammodel.WorkspaceDeletionPhaseFenced || expected.Mode != deletioncontract.ModePurge || operation == nil {
		return common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(r.dataRoot, "eventstream", expected.WorkspaceID)
	if err != nil {
		return err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	current, err := readWorkspaceDeletionFile[eventstreammodel.WorkspaceDeletion](path)
	if err != nil {
		return err
	}
	if current.Validate() != nil || *current != expected {
		return common.ErrConflict
	}
	guarded := context.WithValue(ctx, workspaceDeletionMutationContextKey{}, workspaceDeletionMutationContext{
		owner: "eventstream", workspaceID: expected.WorkspaceID,
	})
	return operation(guarded)
}

func sameEventStreamWorkspaceDeletion(current, expected *eventstreammodel.WorkspaceDeletion, includePhase bool) bool {
	if current == nil || expected == nil || current.Scope() != expected.Scope() || current.FenceID != expected.FenceID ||
		current.SealAckHash != expected.SealAckHash || current.RequestedAt != expected.RequestedAt {
		return false
	}
	if includePhase && (current.Phase != expected.Phase || current.Revision != expected.Revision || current.PurgeID != expected.PurgeID ||
		current.PurgeAckHash != expected.PurgeAckHash || current.PurgeProofHash != expected.PurgeProofHash) {
		return false
	}
	return true
}
