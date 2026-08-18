package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ListArtifactsByResultOwnerCommand(ctx context.Context, hub magicevent.Hub, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	if hub == nil || resultID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	commandCtx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return collectResultOwnerPages(func(cursor string) ([]*resultmodel.Artifact, string, error) {
		event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandListByResult, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), commandCtx, resultevents.ListArtifactsByResultCommand{WorkspaceID: workspaceID, ResultID: resultID, Cursor: cursor, Limit: resultevents.MaxListLimit})
		ownerevent.BindLane(event, resultevents.Contracts(), resultevents.TopicArtifactCommandListByResult, "result", string(resultID), workspaceID, "", "")
		response, err := ownerevent.Result[resultevents.ListArtifactsByResultResult](hub.Send(event), resultevents.TopicArtifactCommandListByResult)
		return response.Artifacts, response.NextCursor, err
	})
}
