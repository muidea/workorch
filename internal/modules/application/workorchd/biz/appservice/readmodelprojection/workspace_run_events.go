package readmodelprojection

import (
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func SelectLatestRunByEvent(workspaceID common.WorkspaceID, runs map[common.RunID]*runmodel.Run, eventRecords []*eventstreammodel.EventRecord) *runmodel.Run {
	selected := SelectLatestRunEvent(workspaceID, runs, eventRecords)
	if selected == nil {
		return nil
	}
	return runs[selected.RunID]
}

func SelectLatestRunEvent(workspaceID common.WorkspaceID, runs map[common.RunID]*runmodel.Run, eventRecords []*eventstreammodel.EventRecord) *eventstreammodel.EventRecord {
	var selected *eventstreammodel.EventRecord
	for _, item := range eventRecords {
		if !isSelectableWorkspaceRunEvent(workspaceID, runs, item) {
			continue
		}
		if selected == nil || item.OccurredAt >= selected.OccurredAt {
			selected = item
		}
	}
	if selected == nil {
		return nil
	}
	return selected
}
