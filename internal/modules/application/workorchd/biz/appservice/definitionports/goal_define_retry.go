package definitionports

import (
	"context"
	"errors"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitioncore"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func defineNewWorkspaceGoal(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, text string) (*workspacemodel.Goal, error) {
	for attempt := 0; attempt < 8; attempt++ {
		id := common.GoalID(definitioncore.GenerateBusinessID("goal"))
		item, err := DefineGoal(ctx, hub, workspaceID, id, text)
		if err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return nil, err
		}
		return item, nil
	}
	return nil, common.ErrConflict
}
