package server

import (
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/events"
)

func subscribeServerStreamEvents(hub events.Hub, handler events.Handler) []func() {
	if hub == nil || handler == nil {
		return nil
	}
	unsubscribers := []func(){
		hub.Subscribe(workorchdevents.TopicWorkspaceProjectionChanged, handler),
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
