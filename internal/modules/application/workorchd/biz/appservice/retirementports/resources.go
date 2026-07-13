package retirementports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/retirementcore"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReleaseRuntimeV1Resources(ctx context.Context, hub magicevent.Hub, plan retirementcore.RuntimeV1RetirementPlan) (int, error) {
	if hub == nil {
		return 0, common.ErrInvalidArgument
	}
	total := 0
	for _, action := range plan.Runs {
		result, err := sendRetirementCommand[orchestratorevents.ReleaseRuntimeV1ResourcesCommand, orchestratorevents.ReleaseRuntimeV1ResourcesResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandReleaseRuntimeV1Resources, orchestratorevents.CommandDestination, "run", orchestratorevents.ReleaseRuntimeV1ResourcesCommand{
			WorkspaceID: action.WorkspaceID,
			RunID:       action.RunID,
		})
		if err != nil {
			return total, err
		}
		total += result.ReleasedTotal
	}
	return total, nil
}
