package ownerreadports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadArtifactContentByResultOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, runID common.RunID, uri string) (resultevents.ReadArtifactContentResult, error) {
	uri = strings.TrimSpace(uri)
	if hub == nil || workspaceID == "" || runID == "" || uri == "" {
		return resultevents.ReadArtifactContentResult{}, common.ErrInvalidArgument
	}
	commandCtx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return resultevents.ReadArtifactContentResult{}, err
	}
	event := magicevent.NewEventWithContext(resultevents.TopicArtifactCommandReadContent, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), commandCtx, resultevents.ReadArtifactContentCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
		URI:         uri,
	})
	ownerevent.BindLane(event, resultevents.Contracts(), resultevents.TopicArtifactCommandReadContent, "artifact", uri, workspaceID, runID, "")
	response, err := ownerevent.Result[resultevents.ReadArtifactContentResult](hub.Send(event), resultevents.TopicArtifactCommandReadContent)
	if err != nil {
		return resultevents.ReadArtifactContentResult{}, err
	}
	if err := response.Validate(); err != nil {
		return resultevents.ReadArtifactContentResult{}, err
	}
	if response.WorkspaceID != workspaceID || response.RunID != runID || response.URI != uri {
		return resultevents.ReadArtifactContentResult{}, common.ErrConflict
	}
	return response.Clone(), nil
}
