package readmodelprojection

import (
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func isSelectableWorkspaceRunEvent(workspaceID common.WorkspaceID, runs map[common.RunID]*runmodel.Run, item *eventstreammodel.EventRecord) bool {
	if item == nil || item.WorkspaceID != workspaceID || item.RunID == "" {
		return false
	}
	if _, ok := runs[item.RunID]; !ok {
		return false
	}
	switch item.Topic {
	case events.TopicRunCreated, events.TopicRunStateChanged:
		return true
	default:
		return false
	}
}
