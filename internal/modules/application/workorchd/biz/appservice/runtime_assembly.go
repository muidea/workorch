package appservice

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	"github.com/muidea/workorch/internal/pkg/events"
)

type managedRuntimeAssembly struct {
	runtime   Runtime
	collector *observed.Collector
	eventHub  events.Hub
}

func managedRuntimeAssemblyFromExports(exports bootstrap.Exports) managedRuntimeAssembly {
	if !hasWorkorchRuntimeExports(exports) {
		return managedRuntimeAssembly{}
	}
	return managedRuntimeAssembly{
		runtime:   NewRuntimeAdapter(RuntimeDependenciesFromExports(exports)),
		collector: observed.NewCollector(exports.Infrastructure.EventHub),
		eventHub:  exports.Infrastructure.EventHub,
	}
}

func managedRuntimeAssemblyFromEventHub(runtime Runtime, eventHub events.Hub) managedRuntimeAssembly {
	return managedRuntimeAssembly{
		runtime:   runtime,
		collector: observed.NewCollector(eventHub),
		eventHub:  eventHub,
	}
}

func newManagedRuntimeFromAssembly(assembly managedRuntimeAssembly) *ManagedRuntime {
	if assembly.runtime == nil {
		return nil
	}
	return &ManagedRuntime{
		Runtime:   assembly.runtime,
		collector: assembly.collector,
		eventHub:  assembly.eventHub,
	}
}

func hasWorkorchRuntimeExports(exports bootstrap.Exports) bool {
	return exports.Infrastructure.EventHub != nil
}
