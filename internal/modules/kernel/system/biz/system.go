package biz

import (
	"context"
	"fmt"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/system/pkg/common"
	systemevents "github.com/muidea/workorch/internal/modules/kernel/system/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

// System owns the system-defaults use case. Its cross-owner coordination is
// expressed only through the typed System EventHub contracts.
type System struct {
	basebiz.Base
}

type Dependencies struct {
	EventHub          event.Hub
	BackgroundRoutine task.BackgroundRoutine
}

func New(eventHub event.Hub, backgroundRoutine task.BackgroundRoutine) (*System, error) {
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return &System{Base: basebiz.New(modulecommon.ModuleID, eventHub, backgroundRoutine)}, nil
}

func NewWithDependencies(deps Dependencies) (*System, error) {
	return New(deps.EventHub, deps.BackgroundRoutine)
}

func (s *System) Start(ctx context.Context) error {
	if s == nil || s.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if err := s.ensureSoulDefaults(ctx); err != nil {
		return err
	}
	if err := s.ensureWorkspaceTemplateDefaults(ctx); err != nil {
		return err
	}
	return s.ensureMCPDefaults(ctx)
}

func (s *System) Teardown(context.Context) {}

func (s *System) ensureSoulDefaults(ctx context.Context) error {
	result, err := s.sendEnsure(event.NewEventWithContext(systemevents.TopicSystemCommandEnsureSoulDefaults, modulecommon.ModuleID, systemevents.SoulDefaultsDestination, event.NewHeader(), ctx, systemevents.EnsureSoulDefaultsCommand{}))
	if err != nil {
		return err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return resultCause(resultErr)
	}
	response, ok := data.(systemevents.EnsureSoulDefaultsResult)
	if !ok || !response.Ensured {
		return common.ErrInvalidArgument
	}
	return nil
}

func (s *System) ensureWorkspaceTemplateDefaults(ctx context.Context) error {
	result, err := s.sendEnsure(event.NewEventWithContext(systemevents.TopicSystemCommandEnsureWorkspaceTemplateDefaults, modulecommon.ModuleID, systemevents.WorkspaceTemplateDefaultsDestination, event.NewHeader(), ctx, systemevents.EnsureWorkspaceTemplateDefaultsCommand{}))
	if err != nil {
		return err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return resultCause(resultErr)
	}
	response, ok := data.(systemevents.EnsureWorkspaceTemplateDefaultsResult)
	if !ok || !response.Ensured {
		return common.ErrInvalidArgument
	}
	return nil
}

func (s *System) ensureMCPDefaults(ctx context.Context) error {
	result, err := s.sendEnsure(event.NewEventWithContext(systemevents.TopicSystemCommandEnsureMCPDefaults, modulecommon.ModuleID, systemevents.MCPDefaultsDestination, event.NewHeader(), ctx, systemevents.EnsureMCPDefaultsCommand{}))
	if err != nil {
		return err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return resultCause(resultErr)
	}
	response, ok := data.(systemevents.EnsureMCPDefaultsResult)
	if !ok || !response.Ensured {
		return common.ErrInvalidArgument
	}
	return nil
}

func (s *System) sendEnsure(value event.Event) (event.Result, error) {
	result := s.SendEvent(value)
	if result == nil {
		return nil, fmt.Errorf("system defaults command returned no result")
	}
	if err := result.Error(); err != nil {
		return nil, resultCause(err)
	}
	return result, nil
}

func resultCause(err *cd.Error) error {
	if err.Cause != nil {
		return err.Cause
	}
	return err
}
