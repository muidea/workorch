package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
)

func ActivateWorkspaceExecutionWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, cmd ActivateWorkspaceExecutionCommand) (ret WorkspaceExecutionActivation, err error) {
	ctx = configcore.ContextWithLoader(ctx, session.ConfigLoader())
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (WorkspaceExecutionActivation, error) {
		defaults, err := workspaceExecutionDefaultsFromContext(ctx, hub)
		if err != nil {
			return WorkspaceExecutionActivation{}, err
		}
		return activationports.ActivateWorkspaceExecutionView(ctx, activationports.ActivateWorkspaceExecutionViewRequest{
			Hub:                            hub,
			WorkspaceID:                    workspaceID,
			Command:                        cmd,
			SystemDefaultSoulID:            defaults.systemDefaultSoulID,
			DefaultDialogLanguage:          defaults.defaultDialogLanguage,
			DefaultEmbeddingProviderLoader: defaults.defaultEmbeddingProviderLoader,
			SkillHubCatalog:                defaults.skillHubCatalog,
		})
	})
}

func ActivateWorkspaceExecutionWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd ActivateWorkspaceExecutionCommand) (WorkspaceExecutionActivation, error) {
	return withSharedBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (WorkspaceExecutionActivation, error) {
		return ActivateWorkspaceExecutionWithSession(ctx, session, workspaceID, cmd)
	})
}

func ActivateWorkspaceExecutionFromEnv(ctx context.Context, workspaceID string, cmd ActivateWorkspaceExecutionCommand) (WorkspaceExecutionActivation, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (WorkspaceExecutionActivation, error) {
		return ActivateWorkspaceExecutionWithSession(ctx, session, workspaceID, cmd)
	})
}
