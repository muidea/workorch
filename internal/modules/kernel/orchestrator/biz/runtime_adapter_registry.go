package orchestrator

import (
	"context"
	"strings"

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type RuntimeAdapterRegistry struct {
	defaultAdapter RuntimeAdapter
	adapters       map[string]RuntimeAdapter
}

func NewRuntimeAdapterRegistry(defaultAdapter RuntimeAdapter) *RuntimeAdapterRegistry {
	return &RuntimeAdapterRegistry{
		defaultAdapter: defaultAdapter,
		adapters:       map[string]RuntimeAdapter{},
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
	if adapter := r.adapters[request.Kind]; adapter != nil {
		return adapter.ExecuteRuntimeStep(ctx, request)
	}
	if r.defaultAdapter != nil {
		return r.defaultAdapter.ExecuteRuntimeStep(ctx, request)
	}
	return runtimemodel.StepExecutionResult{}, common.ErrInvalidArgument
}
