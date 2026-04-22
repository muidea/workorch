package appservice

import (
	"github.com/muidea/workorch/application/workorchd/bootstrap"
	"github.com/muidea/workorch/application/workorchd/observed"
	"github.com/muidea/workorch/internal/pkg/events"
)

type managedRuntimeAssembly struct {
	runtime   Runtime
	collector *observed.Collector
}

func managedRuntimeAssemblyFromExports(exports bootstrap.Exports) managedRuntimeAssembly {
	return managedRuntimeAssembly{
		runtime:   NewRuntimeAdapter(RuntimeDependenciesFromExports(exports)),
		collector: observed.NewCollector(exports.Infrastructure.EventHub),
	}
}

func managedRuntimeAssemblyFromEventHub(runtime Runtime, eventHub events.Hub) managedRuntimeAssembly {
	return managedRuntimeAssembly{
		runtime:   runtime,
		collector: observed.NewCollector(eventHub),
	}
}

func newManagedRuntimeFromAssembly(assembly managedRuntimeAssembly) *ManagedRuntime {
	return &ManagedRuntime{
		Runtime:   assembly.runtime,
		collector: assembly.collector,
	}
}
