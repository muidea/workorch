package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	workorchdeventmapping "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/eventmapping"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (d *CLICommandDispatcher) handleTypedCLIWorkspace(event magicevent.Event, result magicevent.Result) {
	ctx := ContextWithCommandHub(event.Context(), d.EventHub())
	switch command := event.Data().(type) {
	case workorchdevents.CLIWorkspaceCreateCommand:
		setRetiredDefinitionWrite(result)
	case workorchdevents.CLIWorkspaceGetCommand:
		if command.WorkspaceID == "" {
			setInvalidTypedCLICommand(result)
			return
		}
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := ReadWorkspaceWithSession(ctx, d.borrow(), string(command.WorkspaceID))
		if setTypedCLIError(result, err) {
			return
		}
		workspace, mapErr := workorchdeventmapping.WorkspaceDTO(appmodelcore.ProjectWorkspace(item))
		if setTypedCLIError(result, mapErr) {
			return
		}
		result.Set(workorchdevents.CLIWorkspaceGetResult{Workspace: workspace}, nil)
	case workorchdevents.CLIWorkspaceListCommand:
		if d.viewReader == nil {
			setInvalidTypedCLICommand(result)
			return
		}
		items, err := d.viewReader.ListWorkspaceView(ctx)
		if setTypedCLIError(result, err) {
			return
		}
		workspaces, mapErr := workorchdeventmapping.WorkspaceDTOs(appmodelcore.ProjectWorkspaces(items))
		if setTypedCLIError(result, mapErr) {
			return
		}
		result.Set(workorchdevents.CLIWorkspaceListResult{Workspaces: workspaces}, nil)
	case workorchdevents.CLIWorkspaceSnapshotCommand:
		if command.WorkspaceID == "" || d.viewReader == nil {
			setInvalidTypedCLICommand(result)
			return
		}
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := d.viewReader.ReadWorkspaceViewSnapshot(ctx, string(command.WorkspaceID))
		if setTypedCLIError(result, err) {
			return
		}
		item = readmodelports.ProjectWorkspaceSnapshotDetail(item)
		snapshot, err := workorchdeventmapping.SnapshotDTO(readmodelports.WorkspaceSnapshotProjection(item))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIWorkspaceSnapshotResult{Snapshot: snapshot}, nil)
	case workorchdevents.CLIWorkspaceDeleteCommand:
		if command.WorkspaceID == "" {
			setInvalidTypedCLICommand(result)
			return
		}
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		err := DeleteWorkspaceWithSession(ctx, d.borrow(), string(command.WorkspaceID), appmodelcore.DeleteWorkspaceCommand{Purge: command.Purge})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIWorkspaceDeleteResult{Deleted: true}, nil)
	case workorchdevents.CLIWorkspaceUpdateLLMSelectionCommand:
		if command.WorkspaceID == "" {
			setInvalidTypedCLICommand(result)
			return
		}
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := UpdateWorkspaceLLMSelectionWithHub(ctx, d.EventHub(), string(command.WorkspaceID), command.Selection, command.ExpectedRevision)
		if setTypedCLIError(result, err) {
			return
		}
		workspace, mapErr := workorchdeventmapping.WorkspaceDTO(appmodelcore.ProjectWorkspace(item))
		if setTypedCLIError(result, mapErr) {
			return
		}
		result.Set(workorchdevents.CLIWorkspaceUpdateLLMSelectionResult{Workspace: workspace}, nil)
	default:
		setInvalidTypedCLICommand(result)
	}
}

func (d *CLICommandDispatcher) handleTypedWorkspaceRuntimeView(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(workorchdevents.ReadWorkspaceRuntimeViewCommand)
	if !ok {
		setInvalidTypedCLICommand(result)
		return
	}
	query := command.Query.Normalized()
	if query.WorkspaceID == "" || d.viewReader == nil {
		setInvalidTypedCLICommand(result)
		return
	}
	ctx := ContextWithCommandHub(event.Context(), d.EventHub())
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, query.WorkspaceID)
	item, err := d.viewReader.ReadWorkspaceViewSnapshot(ctx, string(query.WorkspaceID))
	if setTypedCLIError(result, err) {
		return
	}
	item = readmodelports.ProjectWorkspaceSnapshotDetail(item)
	activity, hasActivity, activityErr := d.viewReader.ReadWorkspaceActivity(ctx, string(query.WorkspaceID))
	if setTypedCLIError(result, activityErr) {
		return
	}
	view := readmodelports.ProjectWorkspaceRuntimeViewWithActivity(item, activity, hasActivity, query.ActivityLimit, query.BeforeCursor)
	dto, mapErr := workorchdeventmapping.RuntimeViewDTO(view)
	if setTypedCLIError(result, mapErr) {
		return
	}
	result.Set(workorchdevents.ReadWorkspaceRuntimeViewResult{View: dto}, nil)
}
