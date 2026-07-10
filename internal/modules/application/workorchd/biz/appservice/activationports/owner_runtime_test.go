package activationports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
)

func newOwnerRuntime() *bootstrap.Runtime {
	return testkit.NewRuntime("memory", "", workorchruntimepolicy.RuntimePolicy{}, nil, nil)
}
