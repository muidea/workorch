package runports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
)

func newOwnerRuntime() *bootstrap.Runtime {
	return testkit.NewRuntime("memory", "", nil)
}

func testRuntimeExports(runtime *bootstrap.Runtime) testkit.Exports {
	return testkit.RuntimeExports(runtime)
}
