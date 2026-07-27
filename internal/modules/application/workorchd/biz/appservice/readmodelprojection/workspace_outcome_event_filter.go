package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func latestWorkspaceOutcomeRunEvent(snapshot readmodelcore.WorkspaceSnapshot, runsByID map[string]*readmodelcore.Run) *readmodelcore.EventRecord {
	var selectedEvent *readmodelcore.EventRecord
	var selectedNonSuperseded *readmodelcore.EventRecord
	for idx := range snapshot.EventRecords {
		item := &snapshot.EventRecords[idx]
		if item.WorkspaceID != snapshot.Workspace.ID || item.RunID == "" {
			continue
		}
		run, ok := runsByID[item.RunID]
		if !ok {
			continue
		}
		switch item.Topic {
		case runevents.TopicRunCreated, runevents.TopicRunStateChanged:
		default:
			continue
		}
		if selectedEvent == nil || item.OccurredAt >= selectedEvent.OccurredAt {
			selectedEvent = item
		}
		if run != nil && run.State == string(common.RunStateSuperseded) {
			continue
		}
		if outcomeRunEventState(item) == common.RunStateSuperseded {
			continue
		}
		if selectedNonSuperseded == nil || item.OccurredAt >= selectedNonSuperseded.OccurredAt {
			selectedNonSuperseded = item
		}
	}
	if selectedNonSuperseded != nil {
		return selectedNonSuperseded
	}
	if hasNonSupersededOutcomeRun(runsByID) {
		return nil
	}
	return selectedEvent
}

func hasNonSupersededOutcomeRun(runsByID map[string]*readmodelcore.Run) bool {
	for _, item := range runsByID {
		if item == nil {
			continue
		}
		if item.State != string(common.RunStateSuperseded) {
			return true
		}
	}
	return false
}

func outcomeRunEventState(item *readmodelcore.EventRecord) common.RunState {
	if item == nil {
		return ""
	}
	for _, key := range []string{"state", "State"} {
		if value, ok := item.Payload.String(key); ok {
			return common.RunState(value)
		}
	}
	return ""
}
