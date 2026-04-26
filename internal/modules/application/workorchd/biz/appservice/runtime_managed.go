package appservice

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	"github.com/muidea/workorch/internal/pkg/events"
)

type ManagedRuntime struct {
	Runtime
	collector *observed.Collector
	eventHub  events.Hub
}

func NewRuntimeFromBootstrap(runtime *bootstrap.Runtime) *ManagedRuntime {
	if runtime == nil {
		return nil
	}
	return NewRuntimeFromExports(runtime.Exports())
}

func NewRuntimeFromExports(exports bootstrap.Exports) *ManagedRuntime {
	return newManagedRuntimeFromAssembly(managedRuntimeAssemblyFromExports(exports))
}

func NewManagedRuntime(runtime Runtime, eventHub events.Hub) *ManagedRuntime {
	if runtime == nil {
		return nil
	}
	return newManagedRuntimeFromAssembly(managedRuntimeAssemblyFromEventHub(runtime, eventHub))
}

func (r *ManagedRuntime) PublishedWorkorchEvents() []events.WorkorchEvent {
	if r == nil {
		return nil
	}
	published := append([]events.WorkorchEvent{}, publishedWorkorchEventsFromHub(r.eventHub)...)
	seen := make(map[string]struct{}, len(published))
	for _, item := range published {
		seen[workorchEventKey(item)] = struct{}{}
	}
	if r.collector != nil {
		for _, item := range r.collector.PublishedWorkorchEvents() {
			key := workorchEventKey(item)
			if _, ok := seen[key]; ok {
				continue
			}
			published = append(published, item)
			seen[key] = struct{}{}
		}
	}
	return published
}

func (r *ManagedRuntime) Close() {
	if r == nil {
		return
	}
	if r.collector != nil {
		r.collector.Close()
		r.collector = nil
	}
	r.eventHub = nil
}

type publishedEnvelopeStore interface {
	Published() []events.Envelope
}

func publishedWorkorchEventsFromHub(eventHub events.Hub) []events.WorkorchEvent {
	store, ok := eventHub.(publishedEnvelopeStore)
	if !ok || store == nil {
		return nil
	}
	envelopes := store.Published()
	published := make([]events.WorkorchEvent, 0, len(envelopes))
	for _, envelope := range envelopes {
		item, ok := events.DecodeWorkorchEvent(envelope)
		if !ok {
			continue
		}
		published = append(published, item)
	}
	return published
}

func workorchEventKey(item events.WorkorchEvent) string {
	if item == nil {
		return ""
	}
	envelope := item.Envelope()
	return envelope.Topic + "\x00" +
		envelope.OccurredAt + "\x00" +
		envelope.SubjectType + "\x00" +
		envelope.SubjectID + "\x00" +
		envelope.WorkspaceID + "\x00" +
		envelope.RunID + "\x00" +
		envelope.StepID
}
