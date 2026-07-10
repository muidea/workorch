package activationports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func WorkspaceExecutionContract(ctx context.Context, hub magicevent.Hub, workspace *workspacemodel.Workspace) map[string]any {
	if hub == nil {
		return map[string]any{"stateChangePolicy": "allowed_but_not_required"}
	}
	return activationcore.WorkspaceExecutionContract(ctx, workspaceTemplateReader{hub: hub}, workspace)
}

type workspaceTemplateReader struct {
	hub magicevent.Hub
}

func (r workspaceTemplateReader) Get(ctx context.Context, id string) (*repository.WorkspaceTemplate, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	return workspaceports.ReadWorkspaceTemplateRecord(ctx, r.hub, strings.TrimSpace(id))
}

func (r workspaceTemplateReader) List(ctx context.Context) ([]*repository.WorkspaceTemplate, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	return workspaceports.ListWorkspaceTemplateRecords(ctx, r.hub)
}
