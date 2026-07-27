package readmodelprojection

import (
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
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
	var selectedNonSuperseded *eventstreammodel.EventRecord
	for _, item := range eventRecords {
		if !isSelectableWorkspaceRunEvent(workspaceID, runs, item) {
			continue
		}
		if selected == nil || item.OccurredAt >= selected.OccurredAt {
			selected = item
		}
		run := runs[item.RunID]
		if run != nil && run.State == common.RunStateSuperseded {
			continue
		}
		if runEventState(item) == common.RunStateSuperseded {
			continue
		}
		if selectedNonSuperseded == nil || item.OccurredAt >= selectedNonSuperseded.OccurredAt {
			selectedNonSuperseded = item
		}
	}
	if selectedNonSuperseded != nil {
		return selectedNonSuperseded
	}
	if hasNonSupersededRun(runs) {
		return nil
	}
	if selected == nil {
		return nil
	}
	return selected
}

func hasNonSupersededRun(runs map[common.RunID]*runmodel.Run) bool {
	for _, item := range runs {
		if item == nil {
			continue
		}
		if item.State != common.RunStateSuperseded {
			return true
		}
	}
	return false
}

func runEventState(item *eventstreammodel.EventRecord) common.RunState {
	if item == nil || len(item.Payload.Fields) == 0 {
		return ""
	}
	for _, key := range []string{"state", "State"} {
		if value, ok := item.Payload.String(key); ok {
			return common.RunState(value)
		}
	}
	return ""
}
