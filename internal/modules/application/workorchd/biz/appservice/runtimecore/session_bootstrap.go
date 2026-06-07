package runtimecore

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/common"
)

func BuildBootstrapRuntime(factory func() *bootstrap.Runtime) (runtime *bootstrap.Runtime, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			message := fmt.Sprint(recovered)
			if strings.HasPrefix(message, "unsupported repository backend: ") {
				err = fmt.Errorf("%w: %s", common.ErrInvalidArgument, message)
				runtime = nil
				return
			}
			panic(recovered)
		}
	}()
	return factory(), nil
}

func newManagedRuntimeFromBootstrap(runtime *bootstrap.Runtime, factory ManagedRuntimeFactory) *ManagedRuntime {
	if runtime == nil {
		return nil
	}
	return newManagedRuntimeFromExports(runtime.Exports(), factory)
}

func newManagedRuntimeFromExports(exports bootstrap.Exports, factory ManagedRuntimeFactory) *ManagedRuntime {
	if factory == nil || !HasRuntimeExports(exports) {
		return nil
	}
	return factory(exports.Infrastructure.EventHub)
}
