package workspaceports

import (
	"context"
	"errors"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type workspaceCreateIDConflictError struct {
	err error
}

func (e workspaceCreateIDConflictError) Error() string {
	if e.err == nil {
		return common.ErrConflict.Error()
	}
	return e.err.Error()
}

func (e workspaceCreateIDConflictError) Unwrap() error {
	return e.err
}

func isWorkspaceCreateIDConflict(err error) bool {
	var target workspaceCreateIDConflictError
	return errors.As(err, &target)
}

func createWorkspaceViewAttempt(ctx context.Context, hub magicevent.Hub, req CreateWorkspaceViewRequest) (*workspacemodel.Workspace, error) {
	id := workspacecore.GenerateWorkspaceID()
	layout, err := workspacecore.ResolveDirectoryLayout(req.RepositoryBackend, req.RepositoryDataDir, string(id), req.Command.DeliverableDirectory)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace directory layout for %q: %w", id, err)
	}
	if err := workspacecore.EnsureDirectoryLayout(layout); err != nil {
		return nil, fmt.Errorf("ensure workspace directory layout for %q: %w", id, err)
	}
	if err := CreateWorkspaceRecord(ctx, hub, id, req.Command.Title, req.Command.Description, layout.DeliverableDirectory, layout.InternalDirectory); err != nil {
		if errors.Is(err, common.ErrConflict) {
			return nil, workspaceCreateIDConflictError{err: fmt.Errorf("create workspace record %q: %w", id, err)}
		}
		return nil, fmt.Errorf("create workspace record %q: %w", id, err)
	}
	item, err := ReadWorkspaceRecord(ctx, hub, id)
	if err != nil {
		return nil, fmt.Errorf("read created workspace record %q: %w", id, err)
	}
	item, err = ApplyWorkspaceTemplateOnCreate(ctx, hub, item, req.Command.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("apply workspace template for %q: %w", id, err)
	}
	return item, nil
}
