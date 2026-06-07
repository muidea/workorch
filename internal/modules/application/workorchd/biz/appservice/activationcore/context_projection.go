package activationcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func GoalContext(item *workspacemodel.Goal) map[string]any {
	if item == nil {
		return nil
	}
	return map[string]any{
		"id":          string(item.ID),
		"workspaceId": string(item.WorkspaceID),
		"text":        item.Text,
	}
}

func DoDContext(item *workspacemodel.DoD) map[string]any {
	if item == nil {
		return nil
	}
	return map[string]any{
		"id":          string(item.ID),
		"workspaceId": string(item.WorkspaceID),
		"items":       contextregistrycore.CloneStrings(item.Items),
		"state":       string(item.State),
	}
}
