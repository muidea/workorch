package appservice

import (
	"github.com/muidea/workorch/application/workorchd/bootstrap"
	"github.com/muidea/workorch/application/workorchd/observed"
	"github.com/muidea/workorch/internal/pkg/events"
)

type ManagedRuntime struct {
	Runtime
	collector *observed.Collector
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
	if r == nil || r.collector == nil {
		return nil
	}
	return r.collector.PublishedWorkorchEvents()
}

func (r *ManagedRuntime) Close() {
	if r == nil || r.collector == nil {
		return
	}
	r.collector.Close()
	r.collector = nil
}
