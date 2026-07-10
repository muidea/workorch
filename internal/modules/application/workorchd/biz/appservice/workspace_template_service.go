package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
)

type WorkspaceTemplate = workspacecore.WorkspaceTemplate

func CreateWorkspaceTemplateWithBackend(ctx context.Context, backend string, dataRoot string, template WorkspaceTemplate) (WorkspaceTemplate, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (WorkspaceTemplate, error) {
		return CreateWorkspaceTemplateWithSession(ctx, session, template)
	})
}

func CreateWorkspaceTemplateWithSession(ctx context.Context, session *RuntimeSession, template WorkspaceTemplate) (ret WorkspaceTemplate, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (WorkspaceTemplate, error) {
		return workspaceports.CreateWorkspaceTemplate(ctx, hub, template)
	})
}

func UpdateWorkspaceTemplateWithBackend(ctx context.Context, backend string, dataRoot string, templateID string, template WorkspaceTemplate) (WorkspaceTemplate, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (WorkspaceTemplate, error) {
		return UpdateWorkspaceTemplateWithSession(ctx, session, templateID, template)
	})
}

func UpdateWorkspaceTemplateWithSession(ctx context.Context, session *RuntimeSession, templateID string, template WorkspaceTemplate) (ret WorkspaceTemplate, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (WorkspaceTemplate, error) {
		return workspaceports.UpdateWorkspaceTemplate(ctx, hub, templateID, template)
	})
}

func DeleteWorkspaceTemplateWithBackend(ctx context.Context, backend string, dataRoot string, templateID string) error {
	_, err := withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (struct{}, error) {
		return struct{}{}, DeleteWorkspaceTemplateWithSession(ctx, session, templateID)
	})
	return err
}

func DeleteWorkspaceTemplateWithSession(ctx context.Context, session *RuntimeSession, templateID string) (err error) {
	_, err = withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (struct{}, error) {
		return struct{}{}, workspaceports.DeleteWorkspaceTemplate(ctx, hub, templateID)
	})
	return err
}
