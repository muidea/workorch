package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func ListWorkspaceTemplatesWithBackend(ctx context.Context, backend string, dataRoot string) ([]workorchmanagementcontract.WorkspaceTemplate, error) {
	return withBackendSession(ctx, backend, dataRoot, ListWorkspaceTemplatesWithSession)
}

func ListWorkspaceTemplatesWithSession(ctx context.Context, session *runtimecore.Session) (ret []workorchmanagementcontract.WorkspaceTemplate, err error) {
	return withCommandHubSession(ctx, session, workspaceports.ListWorkspaceTemplates)
}

func ReadWorkspaceTemplateWithBackend(ctx context.Context, backend string, dataRoot string, templateID string) (workorchmanagementcontract.WorkspaceTemplate, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.WorkspaceTemplate, error) {
		return ReadWorkspaceTemplateWithSession(ctx, session, templateID)
	})
}

func ReadWorkspaceTemplateWithSession(ctx context.Context, session *runtimecore.Session, templateID string) (ret workorchmanagementcontract.WorkspaceTemplate, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.WorkspaceTemplate, error) {
		return workspaceports.ReadWorkspaceTemplate(ctx, hub, templateID)
	})
}
