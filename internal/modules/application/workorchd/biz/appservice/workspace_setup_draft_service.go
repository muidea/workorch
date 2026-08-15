package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/setupdraft"
)

type WorkspaceSetupDraftGenerator interface {
	GenerateWorkspaceSetupDraft(context.Context, setupdraft.GenerationRequest) (setupdraft.Draft, error)
}

func GenerateWorkspaceSetupDraftWithBackend(ctx context.Context, backend string, dataRoot string, cmd setupdraft.GenerationCommand) (setupdraft.Draft, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (setupdraft.Draft, error) {
		return GenerateWorkspaceSetupDraftWithSession(ctx, session, cmd, nil)
	})
}

func GenerateWorkspaceSetupDraftWithSession(ctx context.Context, session *runtimecore.Session, cmd setupdraft.GenerationCommand, generator WorkspaceSetupDraftGenerator) (ret setupdraft.Draft, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (setupdraft.Draft, error) {
		if generator == nil {
			generator = setupdraft.EventHubGenerator{CommandHub: hub}
		}
		request, err := setupdraft.BuildGenerationRequest(ctx, hub, cmd)
		if err != nil {
			return setupdraft.Draft{}, err
		}
		draft, err := generator.GenerateWorkspaceSetupDraft(ctx, request)
		if err != nil {
			return setupdraft.Draft{}, err
		}
		return setupdraft.MergeDraft(request.CurrentDraft, draft), nil
	})
}
