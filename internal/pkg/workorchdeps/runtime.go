package workorchdeps

import (
	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/framework/plugin/initiator"
	runtimecommon "github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
)

type RuntimeHelper = runtimecommon.Helper

func ResolveRuntimeHelper(consumer string) (runtimecommon.Helper, *cd.Error) {
	_ = consumer
	var helper runtimecommon.Helper
	return initiator.GetEntity(runtimecommon.InitiatorID, helper)
}

func ResolveEventHub(consumer string) (events.Hub, *cd.Error) {
	runtime, err := ResolveRuntimeHelper(consumer)
	if err != nil {
		return nil, err
	}
	return runtime.EventHub(), nil
}

func ResolveRuntimePolicy(consumer string) (workorchruntimepolicy.RuntimePolicy, *cd.Error) {
	runtime, err := ResolveRuntimeHelper(consumer)
	if err != nil {
		return workorchruntimepolicy.RuntimePolicy{}, err
	}
	return runtime.RuntimePolicy(), nil
}
