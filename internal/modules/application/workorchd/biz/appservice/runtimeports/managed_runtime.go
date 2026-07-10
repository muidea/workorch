package runtimeports

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/events"
)

func NewManagedRuntimeFromEventHub(eventHub events.Hub, commandHub magicevent.Hub) *runtimecore.ManagedRuntime {
	if eventHub == nil || commandHub == nil {
		return nil
	}
	return runtimecore.NewManagedRuntime(runtimecore.NewAdapter(NewEventHubDependencies(eventHub, commandHub)), eventHub)
}
