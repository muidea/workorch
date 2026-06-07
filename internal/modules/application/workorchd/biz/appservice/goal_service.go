package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func DefineGoalWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, text string) (ret Goal, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (Goal, error) {
		return definitionports.DefineOrConsolidateWorkspaceGoalView(ctx, hub, common.WorkspaceID(workspaceID), text)
	})
}

func DefineGoalWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, text string) (Goal, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Goal, error) {
		return DefineGoalWithSession(ctx, session, workspaceID, text)
	})
}

func DefineGoalFromEnv(ctx context.Context, workspaceID string, text string) (Goal, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (Goal, error) {
		return DefineGoalWithSession(ctx, session, workspaceID, text)
	})
}

func UpdateGoalWithSession(ctx context.Context, session *RuntimeSession, goalID string, text string) (ret Goal, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (Goal, error) {
		return definitionports.UpdateGoalView(ctx, hub, common.GoalID(goalID), text)
	})
}

func UpdateGoalWithBackend(ctx context.Context, backend string, dataRoot string, goalID string, text string) (Goal, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Goal, error) {
		return UpdateGoalWithSession(ctx, session, goalID, text)
	})
}

func UpdateGoalFromEnv(ctx context.Context, goalID string, text string) (Goal, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (Goal, error) {
		return UpdateGoalWithSession(ctx, session, goalID, text)
	})
}

func ReadGoalWithSession(ctx context.Context, session *RuntimeSession, goalID string) (ret Goal, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (Goal, error) {
		return definitionports.ReadGoalView(ctx, hub, common.GoalID(goalID))
	})
}

func ReadGoalWithBackend(ctx context.Context, backend string, dataRoot string, goalID string) (Goal, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Goal, error) {
		return ReadGoalWithSession(ctx, session, goalID)
	})
}

func ListGoalsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret []Goal, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) ([]Goal, error) {
		return definitionports.ListConsolidatedWorkspaceGoalViews(ctx, hub, common.WorkspaceID(workspaceID))
	})
}

func ListGoalsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]Goal, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]Goal, error) {
		return ListGoalsWithSession(ctx, session, workspaceID)
	})
}
