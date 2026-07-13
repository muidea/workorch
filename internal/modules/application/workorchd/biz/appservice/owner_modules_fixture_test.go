package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
)

func installAppserviceOwnerModuleTestFactories() func() {
	originalBackend := newBootstrapWithBackend
	originalCommandHub := newBootstrapWithBackendAndCommandHub

	newBootstrapWithBackend = func(backend string, dataRoot string) *bootstrap.Runtime {
		return testkit.NewRuntime(backend, dataRoot, nil)
	}
	newBootstrapWithBackendAndCommandHub = func(backend string, dataRoot string, commandHub magicevent.Hub) *bootstrap.Runtime {
		return testkit.NewRuntime(backend, dataRoot, commandHub)
	}
	return func() {
		newBootstrapWithBackend = originalBackend
		newBootstrapWithBackendAndCommandHub = originalCommandHub
	}
}
