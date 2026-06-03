package runtimeevent

import (
	"context"

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type Adapter interface {
	ExecuteRuntimeStep(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error)
}

type Executor struct {
	moduleID string
	hub      events.Hub
	adapter  Adapter
	kinds    []string
	unsub    []func()
}

func NewExecutor(moduleID string, hub events.Hub, adapter Adapter, kinds ...string) *Executor {
	return &Executor{
		moduleID: moduleID,
		hub:      hub,
		adapter:  adapter,
		kinds:    append([]string(nil), kinds...),
	}
}

func (e *Executor) Run() error {
	if e == nil || e.hub == nil || e.adapter == nil || len(e.kinds) == 0 {
		return common.ErrInvalidArgument
	}
	if len(e.unsub) > 0 {
		return nil
	}
	for _, kind := range e.kinds {
		if kind == "" {
			return common.ErrInvalidArgument
		}
		e.unsub = append(e.unsub, e.hub.Subscribe(events.RuntimeBlockExecuteTopic(kind), e.handle))
	}
	return nil
}

func (e *Executor) Teardown() {
	if e == nil {
		return
	}
	for idx := len(e.unsub) - 1; idx >= 0; idx-- {
		if e.unsub[idx] != nil {
			e.unsub[idx]()
		}
	}
	e.unsub = nil
}

func (e *Executor) handle(ctx context.Context, envelope events.Envelope) error {
	request, ok := envelope.Payload[events.PayloadKeyRuntimeRequest].(runtimemodel.StepExecutionRequest)
	if !ok {
		return common.ErrInvalidArgument
	}
	result, err := e.adapter.ExecuteRuntimeStep(ctx, request)
	payload := map[string]any{
		events.PayloadKeyRuntimeResult: result,
	}
	if err != nil {
		payload[events.PayloadKeyError] = err.Error()
	}
	return events.Response(events.Envelope{
		Topic:        envelope.Topic + "/response",
		SourceModule: e.moduleID,
		SubjectType:  envelope.SubjectType,
		SubjectID:    envelope.SubjectID,
		WorkspaceID:  envelope.WorkspaceID,
		RunID:        envelope.RunID,
		StepID:       envelope.StepID,
		Payload:      payload,
	})
}
