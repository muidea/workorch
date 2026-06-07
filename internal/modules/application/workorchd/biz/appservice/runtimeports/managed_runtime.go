package runtimeports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/events"
)

func NewManagedRuntimeFromEventHub(eventHub events.Hub) *runtimecore.ManagedRuntime {
	if eventHub == nil {
		return nil
	}
	return runtimecore.NewManagedRuntime(runtimecore.NewAdapter(NewEventHubDependencies(eventHub)), eventHub)
}
