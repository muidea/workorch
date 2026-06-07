package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeAdapterRegistry struct {
	adapters map[string]RuntimeAdapter
}

func NewRuntimeAdapterRegistry() *RuntimeAdapterRegistry {
	return &RuntimeAdapterRegistry{
		adapters: map[string]RuntimeAdapter{},
	}
}

func (r *RuntimeAdapterRegistry) Register(kind string, adapter RuntimeAdapter) error {
	if r == nil || strings.TrimSpace(kind) == "" || adapter == nil {
		return common.ErrInvalidArgument
	}
	r.adapters[kind] = adapter
	return nil
}

func (r *RuntimeAdapterRegistry) ExecuteRuntimeStep(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	if r == nil {
		return runtimemodel.StepExecutionResult{}, common.ErrInvalidArgument
	}
	kind := strings.TrimSpace(request.Kind)
	if adapter := r.adapters[kind]; adapter != nil {
		return adapter.ExecuteRuntimeStep(ctx, request)
	}
	return runtimemodel.StepExecutionResult{
		Output: map[string]any{
			"status": "failed",
			"stage":  "runtime_adapter_unavailable",
			"kind":   kind,
			"error":  fmt.Sprintf("runtime adapter is not registered for step kind %s", kind),
		},
	}, common.ErrInvalidArgument
}
