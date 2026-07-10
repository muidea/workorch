package appservice

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
)

func installAppserviceOwnerModuleTestFactories() func() {
	originalBackend := newBootstrapWithBackend
	originalPolicyHub := newBootstrapWithBackendRuntimePolicyAndEventHubs
	originalPolicy := newBootstrapWithBackendAndRuntimePolicy

	newBootstrapWithBackend = func(backend string, dataRoot string) *bootstrap.Runtime {
		return testkit.NewRuntime(backend, dataRoot, workorchruntimepolicy.RuntimePolicy{}, nil, nil)
	}
	newBootstrapWithBackendRuntimePolicyAndEventHubs = testkit.NewRuntime
	newBootstrapWithBackendAndRuntimePolicy = func(backend string, dataRoot string, policy workorchruntimepolicy.RuntimePolicy) *bootstrap.Runtime {
		return testkit.NewRuntime(backend, dataRoot, policy, nil, nil)
	}
	return func() {
		newBootstrapWithBackend = originalBackend
		newBootstrapWithBackendRuntimePolicyAndEventHubs = originalPolicyHub
		newBootstrapWithBackendAndRuntimePolicy = originalPolicy
	}
}
