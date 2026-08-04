package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func CreateWorkspaceTemplateWithBackend(ctx context.Context, backend string, dataRoot string, template workorchmanagementcontract.WorkspaceTemplate) (workorchmanagementcontract.WorkspaceTemplate, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.WorkspaceTemplate, error) {
		return CreateWorkspaceTemplateWithSession(ctx, session, template)
	})
}

func CreateWorkspaceTemplateWithSession(ctx context.Context, session *runtimecore.Session, template workorchmanagementcontract.WorkspaceTemplate) (ret workorchmanagementcontract.WorkspaceTemplate, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.WorkspaceTemplate, error) {
		return workspaceports.CreateWorkspaceTemplate(ctx, hub, template)
	})
}

func UpdateWorkspaceTemplateWithBackend(ctx context.Context, backend string, dataRoot string, templateID string, template workorchmanagementcontract.WorkspaceTemplate) (workorchmanagementcontract.WorkspaceTemplate, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (workorchmanagementcontract.WorkspaceTemplate, error) {
		return UpdateWorkspaceTemplateWithSession(ctx, session, templateID, template)
	})
}

func UpdateWorkspaceTemplateWithSession(ctx context.Context, session *runtimecore.Session, templateID string, template workorchmanagementcontract.WorkspaceTemplate) (ret workorchmanagementcontract.WorkspaceTemplate, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.WorkspaceTemplate, error) {
		return workspaceports.UpdateWorkspaceTemplate(ctx, hub, templateID, template)
	})
}

func DeleteWorkspaceTemplateWithBackend(ctx context.Context, backend string, dataRoot string, templateID string) error {
	_, err := withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (struct{}, error) {
		return struct{}{}, DeleteWorkspaceTemplateWithSession(ctx, session, templateID)
	})
	return err
}

func DeleteWorkspaceTemplateWithSession(ctx context.Context, session *runtimecore.Session, templateID string) (err error) {
	_, err = withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (struct{}, error) {
		return struct{}{}, workspaceports.DeleteWorkspaceTemplate(ctx, hub, templateID)
	})
	return err
}
