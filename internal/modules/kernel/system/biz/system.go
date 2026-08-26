package biz

import (
	"context"
	"fmt"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	mcpregistryevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/system/pkg/common"
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

func NewWithDependencies(deps Dependencies) (*System, error) {
	if deps.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return &System{Base: basebiz.New(modulecommon.ModuleID, deps.EventHub, deps.BackgroundRoutine)}, nil
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
	result, err := s.sendEnsure(event.NewEventWithContext(soulevents.TopicSoulCommandEnsureDefaults, modulecommon.ModuleID, soulevents.CommandDestination, event.NewHeader(), ctx, soulevents.EnsureDefaultsCommand{}))
	if err != nil {
		return err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return resultCause(resultErr)
	}
	response, ok := data.(soulevents.EnsureDefaultsResult)
	if !ok || !response.Ensured {
		return common.ErrInvalidArgument
	}
	return nil
}

func (s *System) ensureWorkspaceTemplateDefaults(ctx context.Context) error {
	command := workspaceevents.EnsureWorkspaceTemplateDefaultsCommand{}
	result, err := s.sendEnsure(event.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandEnsureDefaults, modulecommon.ModuleID, workspaceevents.CommandDestination, event.NewHeader(), ctx, command))
	if err != nil {
		return err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return resultCause(resultErr)
	}
	response, ok := data.(workspaceevents.EnsureWorkspaceTemplateDefaultsResult)
	if !ok {
		return common.ErrInvalidArgument
	}
	return response.ValidateFor(command)
}

func (s *System) ensureMCPDefaults(ctx context.Context) error {
	result, err := s.sendEnsure(event.NewEventWithContext(mcpregistryevents.TopicMCPCommandEnsureDefaults, modulecommon.ModuleID, mcpregistryevents.CommandDestination, event.NewHeader(), ctx, mcpregistryevents.EnsureDefaultsCommand{}))
	if err != nil {
		return err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return resultCause(resultErr)
	}
	response, ok := data.(mcpregistryevents.EnsureDefaultsResult)
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
