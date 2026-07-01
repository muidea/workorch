package server

import (
	"strings"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/events"
)

const (
	eventStreamViewFull       = "full"
	eventStreamViewProjection = "projection"
)

func normalizeEventStreamView(view string) string {
	switch strings.ToLower(strings.TrimSpace(view)) {
	case "", eventStreamViewProjection:
		return eventStreamViewProjection
	case eventStreamViewFull:
		return eventStreamViewFull
	default:
		return eventStreamViewProjection
	}
}

func isProjectionEventStreamView(view string) bool {
	return normalizeEventStreamView(view) == eventStreamViewProjection
}

func subscribeServerStreamEvents(hub events.Hub, view string, handler events.Handler) []func() {
	if hub == nil || handler == nil {
		return nil
	}
	unsubscribers := []func(){
		hub.Subscribe(workorchdevents.TopicWorkspaceProjectionChanged, handler),
	}
	if isProjectionEventStreamView(view) {
		return unsubscribers
	}
	for _, topic := range events.WorkorchTopics() {
		topic := topic
		unsubscribers = append(unsubscribers, hub.Subscribe(topic, handler))
	}
	return unsubscribers
}

func unsubscribeAll(unsubscribers []func()) {
	for _, unsubscribe := range unsubscribers {
		if unsubscribe != nil {
			unsubscribe()
		}
	}
}
