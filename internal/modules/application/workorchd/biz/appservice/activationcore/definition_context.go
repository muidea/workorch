package activationcore

import (
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/doditems"
)

func LatestWorkspaceGoal(items []*workspacemodel.Goal) *workspacemodel.Goal {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i] != nil && strings.TrimSpace(items[i].Text) != "" {
			return items[i]
		}
	}
	return nil
}

func LatestWorkspaceDoD(items []*workspacemodel.DoD) *workspacemodel.DoD {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i] != nil && len(doditems.Normalize(items[i].Items)) > 0 {
			return items[i]
		}
	}
	return nil
}
