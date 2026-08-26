package workspaceports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type CreateWorkspaceViewRequest struct {
	RepositoryBackend  string
	RepositoryDataDir  string
	Command            workorchmanagementcontract.WorkspaceCreateRequest
	MaxConflictRetries int
}

type DeleteWorkspaceViewRequest struct {
	WorkspaceID string
	Command     appmodelcore.DeleteWorkspaceCommand
}

func CreateWorkspaceView(ctx context.Context, hub magicevent.Hub, req CreateWorkspaceViewRequest) (appmodelcore.Workspace, error) {
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

func DeleteWorkspaceView(ctx context.Context, hub magicevent.Hub, req DeleteWorkspaceViewRequest) error {
	id := common.WorkspaceID(strings.TrimSpace(req.WorkspaceID))
	if id == "" {
		return common.ErrInvalidArgument
	}
	command := req.Command.Normalized(string(id))
	_, err := DeleteWorkspaceRecord(ctx, hub, id, command)
	return err
}
