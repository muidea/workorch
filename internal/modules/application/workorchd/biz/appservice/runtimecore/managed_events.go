package runtimecore

import "github.com/muidea/workorch/internal/pkg/events"

func mergePublishedWorkorchEvents(hubEvents []events.WorkorchEvent, collectorEvents []events.WorkorchEvent) []events.WorkorchEvent {
	published := append([]events.WorkorchEvent{}, hubEvents...)
	seen := make(map[string]struct{}, len(published))
	for _, item := range published {
		seen[workorchEventKey(item)] = struct{}{}
	}
	for _, item := range collectorEvents {
		key := workorchEventKey(item)
		if _, ok := seen[key]; ok {
			continue
		}
		published = append(published, item)
		seen[key] = struct{}{}
	}
	return published
}
