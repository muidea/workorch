package runtimeports

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
)

func NewManagedRuntimeFromCommandHub(commandHub magicevent.Hub) *runtimecore.ManagedRuntime {
	if commandHub == nil {
		return nil
	}
	return runtimecore.NewManagedRuntime(runtimecore.NewAdapter(NewEventHubDependencies(commandHub)), commandHub)
}
