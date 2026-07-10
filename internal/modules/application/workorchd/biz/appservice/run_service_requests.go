package appservice

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runports"
)

func createRunViewRequest(ctx context.Context, hub magicevent.Hub, workspaceID string, runID string, goalID string, dodID string) (runports.CreateRunViewRequest, error) {
	request := runports.CreateRunViewRequest{
		Hub:         hub,
		WorkspaceID: workspaceID,
		RunID:       runID,
		GoalID:      goalID,
		DoDID:       dodID,
	}
	if strings.TrimSpace(goalID) == "" && strings.TrimSpace(dodID) == "" {
		return request, nil
	}
	defaults, err := workspaceExecutionDefaultsFromContext(ctx, hub)
	if err != nil {
		return runports.CreateRunViewRequest{}, err
	}
	request.SystemDefaultSoulID = defaults.systemDefaultSoulID
	request.DefaultDialogLanguage = defaults.defaultDialogLanguage
	request.DefaultEmbeddingProviderLoader = defaults.defaultEmbeddingProviderLoader
	request.SkillHubCatalog = defaults.skillHubCatalog
	return request, nil
}
