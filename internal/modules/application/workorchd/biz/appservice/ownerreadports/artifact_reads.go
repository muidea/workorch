package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"

	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ListArtifactsByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[resultevents.ListArtifactsByWorkspaceCommand, resultevents.ResultCommandResult](ctx, hub, resultevents.TopicArtifactCommandListByWorkspace, resultevents.CommandDestination, "workspace", string(workspaceID), resultevents.ListArtifactsByWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	return response.Artifacts, nil
}

func ListArtifactsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*resultmodel.Artifact, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := sendMagicCommand[resultevents.ListArtifactsByRunCommand, resultevents.ResultCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, resultevents.TopicArtifactCommandListByRun, resultevents.CommandDestination, "run", string(runID), resultevents.ListArtifactsByRunCommand{RunID: runID})
	if err != nil {
		return nil, err
	}
	return response.Artifacts, nil
}
