package workspaceports

import (
	"context"
	"errors"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type CreateWorkspaceViewRequest struct {
	RepositoryBackend  string
	RepositoryDataDir  string
	Command            appmodelcore.CreateWorkspaceCommand
	MaxConflictRetries int
}

type DeleteWorkspaceViewRequest struct {
	RepositoryBackend string
	RepositoryDataDir string
	WorkspaceID       string
	Command           appmodelcore.DeleteWorkspaceCommand
}

func CreateWorkspaceView(ctx context.Context, hub events.Hub, req CreateWorkspaceViewRequest) (appmodelcore.Workspace, error) {
	attempts := req.MaxConflictRetries
	if attempts <= 0 {
		attempts = 8
	}
	var lastConflict error
	for attempt := 0; attempt < attempts; attempt++ {
		item, err := createWorkspaceViewAttempt(ctx, hub, req)
		if err != nil {
			if isWorkspaceCreateIDConflict(err) {
				lastConflict = err
				continue
			}
			return appmodelcore.Workspace{}, err
		}
		return workspacecore.MapWorkspace(item), nil
	}
	if lastConflict != nil {
		return appmodelcore.Workspace{}, lastConflict
	}
	return appmodelcore.Workspace{}, common.ErrConflict
}

func DeleteWorkspaceView(ctx context.Context, hub events.Hub, req DeleteWorkspaceViewRequest) error {
	id := common.WorkspaceID(strings.TrimSpace(req.WorkspaceID))
	if id == "" {
		return common.ErrInvalidArgument
	}
	workspace, err := ReadWorkspaceRecord(ctx, hub, id)
	if err != nil {
		return err
	}
	if req.Command.Purge {
		if err := PurgeWorkspaceRecords(ctx, hub, id); err != nil {
			return err
		}
	}
	if err := DeleteWorkspaceRecord(ctx, hub, id); err != nil {
		return err
	}
	if !req.Command.Purge {
		return nil
	}
	if err := purgeDeletedWorkspaceView(ctx, hub, req, id, workspace); err != nil {
		if errors.Is(err, common.ErrNotFound) || errors.Is(err, common.ErrConflict) {
			return nil
		}
		return err
	}
	return nil
}
