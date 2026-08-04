package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/setupdraftcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/setupdraftports"
	"github.com/muidea/workorch/internal/pkg/common"
)

type WorkspaceSetupDraftGenerator interface {
	GenerateWorkspaceSetupDraft(context.Context, setupdraftcore.GenerationRequest) (setupdraftcore.Draft, error)
}

func GenerateWorkspaceSetupDraftWithBackend(ctx context.Context, backend string, dataRoot string, cmd setupdraftcore.GenerationCommand) (setupdraftcore.Draft, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (setupdraftcore.Draft, error) {
		return GenerateWorkspaceSetupDraftWithSession(ctx, session, cmd, nil)
	})
}

func GenerateWorkspaceSetupDraftWithSession(ctx context.Context, session *runtimecore.Session, cmd setupdraftcore.GenerationCommand, generator WorkspaceSetupDraftGenerator) (ret setupdraftcore.Draft, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (setupdraftcore.Draft, error) {
		if generator == nil {
			generator = setupdraftports.EventHubGenerator{CommandHub: hub}
		}
		request, err := setupdraftports.BuildGenerationRequest(ctx, hub, cmd)
		if err != nil {
			return setupdraftcore.Draft{}, err
		}
		draft, err := generator.GenerateWorkspaceSetupDraft(ctx, request)
		if err != nil {
			return setupdraftcore.Draft{}, err
		}
		return MergeWorkspaceSetupDraft(request.CurrentDraft, draft), nil
	})
}

func BuildWorkspaceSetupDraftGenerationRequest(ctx context.Context, session *runtimecore.Session, cmd setupdraftcore.GenerationCommand) (setupdraftcore.GenerationRequest, error) {
	if session == nil {
		return setupdraftcore.GenerationRequest{}, common.ErrInvalidArgument
	}
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (setupdraftcore.GenerationRequest, error) {
		return setupdraftports.BuildGenerationRequest(ctx, hub, cmd)
	})
}

func NormalizeWorkspaceSetupDraft(draft setupdraftcore.Draft) setupdraftcore.Draft {
	return setupdraftcore.NormalizeDraft(draft)
}

func MergeWorkspaceSetupDraft(base setupdraftcore.Draft, update setupdraftcore.Draft) setupdraftcore.Draft {
	return setupdraftcore.MergeDraft(base, update)
}

func WorkspaceSetupDraftRevisionBaseline(draft setupdraftcore.Draft) setupdraftcore.Draft {
	return setupdraftcore.RevisionBaseline(draft)
}

func ParseWorkspaceSetupDraftCompletion(payload []byte) (setupdraftcore.Draft, error) {
	return setupdraftcore.ParseCompletion(payload)
}
