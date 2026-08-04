package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/setupdraftcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/setupdraftports"
	"github.com/muidea/workorch/internal/pkg/common"
)

type WorkspaceSetupDraftWorkspace = setupdraftcore.Workspace
type WorkspaceSetupDraftMessage = setupdraftcore.Message
type WorkspaceSetupDraft = setupdraftcore.Draft
type WorkspaceSetupDraftGenerationCommand = setupdraftcore.GenerationCommand
type WorkspaceSetupDraftGenerationRequest = setupdraftcore.GenerationRequest

type WorkspaceSetupDraftGenerator interface {
	GenerateWorkspaceSetupDraft(context.Context, WorkspaceSetupDraftGenerationRequest) (WorkspaceSetupDraft, error)
}

func GenerateWorkspaceSetupDraftWithBackend(ctx context.Context, backend string, dataRoot string, cmd WorkspaceSetupDraftGenerationCommand) (WorkspaceSetupDraft, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (WorkspaceSetupDraft, error) {
		return GenerateWorkspaceSetupDraftWithSession(ctx, session, cmd, nil)
	})
}

func GenerateWorkspaceSetupDraftWithSession(ctx context.Context, session *RuntimeSession, cmd WorkspaceSetupDraftGenerationCommand, generator WorkspaceSetupDraftGenerator) (ret WorkspaceSetupDraft, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (WorkspaceSetupDraft, error) {
		if generator == nil {
			generator = setupdraftports.EventHubGenerator{CommandHub: hub}
		}
		request, err := setupdraftports.BuildGenerationRequest(ctx, hub, cmd)
		if err != nil {
			return WorkspaceSetupDraft{}, err
		}
		draft, err := generator.GenerateWorkspaceSetupDraft(ctx, request)
		if err != nil {
			return WorkspaceSetupDraft{}, err
		}
		return MergeWorkspaceSetupDraft(request.CurrentDraft, draft), nil
	})
}

func BuildWorkspaceSetupDraftGenerationRequest(ctx context.Context, session *RuntimeSession, cmd WorkspaceSetupDraftGenerationCommand) (WorkspaceSetupDraftGenerationRequest, error) {
	if session == nil {
		return WorkspaceSetupDraftGenerationRequest{}, common.ErrInvalidArgument
	}
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (WorkspaceSetupDraftGenerationRequest, error) {
		return setupdraftports.BuildGenerationRequest(ctx, hub, cmd)
	})
}

func NormalizeWorkspaceSetupDraft(draft WorkspaceSetupDraft) WorkspaceSetupDraft {
	return setupdraftcore.NormalizeDraft(draft)
}

func MergeWorkspaceSetupDraft(base WorkspaceSetupDraft, update WorkspaceSetupDraft) WorkspaceSetupDraft {
	return setupdraftcore.MergeDraft(base, update)
}

func WorkspaceSetupDraftRevisionBaseline(draft WorkspaceSetupDraft) WorkspaceSetupDraft {
	return setupdraftcore.RevisionBaseline(draft)
}

func ParseWorkspaceSetupDraftCompletion(payload []byte) (WorkspaceSetupDraft, error) {
	return setupdraftcore.ParseCompletion(payload)
}
