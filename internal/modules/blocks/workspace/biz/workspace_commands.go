package workspace

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) executeWorkspaceDefinitionCommand(ctx context.Context, ev magicevent.Event) (workspaceevents.WorkspaceCommandResult, error) {
	switch ev.ID() {
	case workspaceevents.TopicWorkspaceCommandCreate:
		return m.createWorkspace(ctx, ev)
	case workspaceevents.TopicWorkspaceCommandGet:
		return m.getWorkspace(ctx, ev)
	case workspaceevents.TopicWorkspaceCommandList:
		return m.listWorkspaces(ctx, ev)
	case workspaceevents.TopicWorkspaceCommandUpdate:
		return m.updateWorkspace(ctx, ev)
	case workspaceevents.TopicWorkspaceCommandUpdateLLMSelection:
		return m.updateWorkspaceLLMSelection(ctx, ev)
	case workspaceevents.TopicWorkspaceCommandDelete:
		return m.deleteWorkspace(ctx, ev)
	default:
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
	}
}

func (m *Biz) createWorkspace(ctx context.Context, ev magicevent.Event) (workspaceevents.WorkspaceCommandResult, error) {
	cmd, ok := ev.Data().(workspaceevents.CreateWorkspaceCommand)
	if !ok {
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
	}
	if err := m.manager.CreateWorkspace(ctx, workspaceevents.CreateWorkspaceCommand{
		WorkspaceID: cmd.WorkspaceID, Title: cmd.Title, Description: cmd.Description,
		DeliverableDirectory: cmd.DeliverableDirectory, InternalDirectory: cmd.InternalDirectory,
	}); err != nil {
		return workspaceevents.WorkspaceCommandResult{}, err
	}
	return workspaceevents.WorkspaceCommandResult{}, nil
}

func (m *Biz) getWorkspace(ctx context.Context, ev magicevent.Event) (workspaceevents.WorkspaceCommandResult, error) {
	cmd, ok := ev.Data().(workspaceevents.GetWorkspaceCommand)
	if !ok || cmd.WorkspaceID == "" || m.workspaces == nil {
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
	}
	item, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspaceevents.WorkspaceCommandResult{}, err
	}
	return workspaceevents.WorkspaceCommandResult{Workspace: workspaceeventmapping.WorkspaceDTO(item)}, nil
}

func (m *Biz) listWorkspaces(ctx context.Context, ev magicevent.Event) (workspaceevents.WorkspaceCommandResult, error) {
	if _, ok := ev.Data().(workspaceevents.ListWorkspacesCommand); !ok || m.workspaces == nil {
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
	}
	items, err := m.workspaces.List(ctx)
	if err != nil {
		return workspaceevents.WorkspaceCommandResult{}, err
	}
	return workspaceevents.WorkspaceCommandResult{Workspaces: workspaceeventmapping.WorkspaceDTOs(items)}, nil
}

func (m *Biz) updateWorkspace(ctx context.Context, ev magicevent.Event) (workspaceevents.WorkspaceCommandResult, error) {
	cmd, ok := ev.Data().(workspaceevents.UpdateWorkspaceCommand)
	if !ok || cmd.WorkspaceID == "" || m.workspaces == nil || soulbinding.Validate(cmd.SoulBindings) != nil {
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
	}
	current, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspaceevents.WorkspaceCommandResult{}, err
	}
	// Generic metadata updates deliberately preserve lifecycle-owned fields.
	// Workspace.State and ActiveRunID may only be changed by lifecycle_reducer.
	item := workspaceFromUpdateCommand(current, cmd)
	if err := m.workspaces.CompareAndSwap(ctx, item, item.Revision); err != nil {
		return workspaceevents.WorkspaceCommandResult{}, err
	}
	m.publishWorkspaceUpdated(ctx, item)
	return workspaceevents.WorkspaceCommandResult{Workspace: workspaceeventmapping.WorkspaceDTO(item)}, nil
}

func (m *Biz) updateWorkspaceLLMSelection(ctx context.Context, ev magicevent.Event) (workspaceevents.WorkspaceCommandResult, error) {
	cmd, ok := ev.Data().(workspaceevents.UpdateWorkspaceLLMSelectionCommand)
	if !ok || cmd.WorkspaceID == "" || m.workspaces == nil {
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
	}
	item, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspaceevents.WorkspaceCommandResult{}, err
	}
	if cmd.ExpectedRevision == 0 || item.Revision != cmd.ExpectedRevision {
		return workspaceevents.WorkspaceCommandResult{}, common.ErrConflict
	}
	currentSelection := runtimecontract.CloneProviderInferenceSelection(item.DefaultLLMSelection)
	if cmd.Selection == nil {
		item.DefaultLLMSelection = nil
	} else {
		selection := cmd.Selection.Normalized()
		if err := selection.Validate(); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
		}
		item.DefaultLLMSelection = &selection
	}
	if (currentSelection == nil && item.DefaultLLMSelection == nil) ||
		(currentSelection != nil && item.DefaultLLMSelection != nil && currentSelection.Same(*item.DefaultLLMSelection)) {
		return workspaceevents.WorkspaceCommandResult{Workspace: workspaceeventmapping.WorkspaceDTO(item), Changed: false}, nil
	}
	if err := m.workspaces.CompareAndSwap(ctx, item, cmd.ExpectedRevision); err != nil {
		return workspaceevents.WorkspaceCommandResult{}, err
	}
	m.publishWorkspaceUpdated(ctx, item)
	return workspaceevents.WorkspaceCommandResult{Workspace: workspaceeventmapping.WorkspaceDTO(item), Changed: true}, nil
}

func (m *Biz) deleteWorkspace(ctx context.Context, ev magicevent.Event) (workspaceevents.WorkspaceCommandResult, error) {
	cmd, ok := ev.Data().(workspaceevents.DeleteWorkspaceCommand)
	if !ok || cmd.WorkspaceID == "" {
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
	}
	if err := m.manager.DeleteWorkspace(ctx, cmd.WorkspaceID); err != nil {
		return workspaceevents.WorkspaceCommandResult{}, err
	}
	return workspaceevents.WorkspaceCommandResult{}, nil
}

func workspaceFromUpdateCommand(current *workspacemodel.Workspace, cmd workspaceevents.UpdateWorkspaceCommand) *workspacemodel.Workspace {
	if current == nil {
		return nil
	}
	item := *current
	item.Title = cmd.Title
	item.Description = cmd.Description
	item.DeliverableDirectory = cmd.DeliverableDirectory
	item.InternalDirectory = cmd.InternalDirectory
	item.TemplateID = cmd.TemplateID
	item.ProfileID = cmd.ProfileID
	item.DefaultLLMSelection = runtimecontract.CloneProviderInferenceSelection(cmd.DefaultLLMSelection)
	item.DefaultSoulID = cmd.DefaultSoulID
	item.SoulBindings = soulbinding.SoulIDMap(cmd.SoulBindings)
	item.ContextHints = cmd.ContextHints.Clone()
	item.EvidencePolicies = workorcheventpayload.CloneSlice(cmd.EvidencePolicies)
	return &item
}
