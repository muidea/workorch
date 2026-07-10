package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
)

func ListWorkspaceTemplatesWithBackend(ctx context.Context, backend string, dataRoot string) ([]WorkspaceTemplate, error) {
	return withBackendSession(ctx, backend, dataRoot, ListWorkspaceTemplatesWithSession)
}

func ListWorkspaceTemplatesWithSession(ctx context.Context, session *RuntimeSession) (ret []WorkspaceTemplate, err error) {
	return withCommandHubSession(ctx, session, workspaceports.ListWorkspaceTemplates)
}

func ReadWorkspaceTemplateWithBackend(ctx context.Context, backend string, dataRoot string, templateID string) (WorkspaceTemplate, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (WorkspaceTemplate, error) {
		return ReadWorkspaceTemplateWithSession(ctx, session, templateID)
	})
}

func ReadWorkspaceTemplateWithSession(ctx context.Context, session *RuntimeSession, templateID string) (ret WorkspaceTemplate, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (WorkspaceTemplate, error) {
		return workspaceports.ReadWorkspaceTemplate(ctx, hub, templateID)
	})
}
