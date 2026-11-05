package workorchruntimecontract

import (
	"context"
	"runtime/trace"
)

// Standard Go trace regions are optional diagnostics only. No business input,
// credential, clock result or mutable aggregate enters the trace. CPU/heap
// profiles attribute work to the same named compiler/validator functions.
func runtimeProcessingRegion(name string, body func() error) error {
	region := trace.StartRegion(context.Background(), "workorch.runtime."+name)
	defer region.End()
	return body()
}
