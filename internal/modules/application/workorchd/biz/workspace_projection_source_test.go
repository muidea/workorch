package biz

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
)

func TestWorkspaceProjectionChangedConsumerRequiresExactApplicationOwnerSource(t *testing.T) {
	data := workorchdevents.WorkspaceProjectionChangedData{WorkspaceID: "ws-projection-source", StreamGeneration: "generation-1"}
	eventFor := func(source string, payload any) magicevent.Event {
		return magicevent.NewEventWithContext(
			workorchdevents.TopicWorkspaceProjectionChanged,
			source,
			workorchdevents.NotificationDestination,
			magicevent.NewHeader(),
			context.Background(),
			payload,
		)
	}

	for name, event := range map[string]magicevent.Event{
		"nil event":           nil,
		"forged source":       eventFor("application/workorchd/projection", data),
		"noncanonical source": eventFor(" application/workorchd ", data),
		"wrong payload":       eventFor(ModuleID, struct{}{}),
	} {
		t.Run(name, func(t *testing.T) {
			if got, ok := workspaceProjectionChangedFromOwner(event); ok || got.WorkspaceID != "" {
				t.Fatalf("accepted invalid projection notification: %+v", got)
			}
		})
	}

	got, ok := workspaceProjectionChangedFromOwner(eventFor(ModuleID, data))
	if !ok || got.WorkspaceID != data.WorkspaceID || got.StreamGeneration != data.StreamGeneration {
		t.Fatalf("exact owner notification = %+v/%t, want %+v", got, ok, data)
	}
}
