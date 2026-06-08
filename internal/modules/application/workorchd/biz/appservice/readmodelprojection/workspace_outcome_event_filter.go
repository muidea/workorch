package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
)

func latestWorkspaceOutcomeRunEvent(snapshot readmodelcore.WorkspaceSnapshot, runsByID map[string]*readmodelcore.Run) *readmodelcore.EventRecord {
	var selectedEvent *readmodelcore.EventRecord
	for idx := range snapshot.EventRecords {
		item := &snapshot.EventRecords[idx]
		if item.WorkspaceID != snapshot.Workspace.ID || item.RunID == "" {
			continue
		}
		if _, ok := runsByID[item.RunID]; !ok {
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
	}
	return selectedEvent
}
