package readmodelprojection

import (
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func isSelectableWorkspaceRunEvent(workspaceID common.WorkspaceID, runs map[common.RunID]*runmodel.Run, item *eventstreammodel.EventRecord) bool {
	if item == nil || item.WorkspaceID != workspaceID || item.RunID == "" {
		return false
	}
	if _, ok := runs[item.RunID]; !ok {
		return false
	}
	switch item.Topic {
	case runevents.TopicRunCreated, runevents.TopicRunStateChanged:
		return true
	default:
		return false
	}
}
