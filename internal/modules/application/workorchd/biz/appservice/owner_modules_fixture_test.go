package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
)

func installAppserviceOwnerModuleTestFactories() func() {
	originalBackend := newBootstrapWithBackend
	originalPolicyHub := newBootstrapWithBackendRuntimePolicyAndCommandHub
	originalPolicy := newBootstrapWithBackendAndRuntimePolicy

	newBootstrapWithBackend = func(backend string, dataRoot string) *bootstrap.Runtime {
		return testkit.NewRuntime(backend, dataRoot, workorchruntimepolicy.RuntimePolicy{}, nil)
	}
	newBootstrapWithBackendRuntimePolicyAndCommandHub = func(backend string, dataRoot string, policy workorchruntimepolicy.RuntimePolicy, commandHub magicevent.Hub) *bootstrap.Runtime {
		return testkit.NewRuntime(backend, dataRoot, policy, commandHub)
	}
	newBootstrapWithBackendAndRuntimePolicy = func(backend string, dataRoot string, policy workorchruntimepolicy.RuntimePolicy) *bootstrap.Runtime {
		return testkit.NewRuntime(backend, dataRoot, policy, nil)
	}
	return func() {
		newBootstrapWithBackend = originalBackend
		newBootstrapWithBackendRuntimePolicyAndCommandHub = originalPolicyHub
		newBootstrapWithBackendAndRuntimePolicy = originalPolicy
	}
}
