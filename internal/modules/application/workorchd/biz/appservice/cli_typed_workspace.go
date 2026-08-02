package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (d *CLICommandDispatcher) handleTypedCLIWorkspace(event magicevent.Event, result magicevent.Result) {
	ctx := ContextWithCommandHub(event.Context(), d.EventHub())
	switch command := event.Data().(type) {
	case workorchdevents.CLIWorkspaceCreateCommand:
		item, err := CreateWorkspaceWithTemplateWithSession(ctx, d.borrow(), appmodelcore.CreateWorkspaceCommand{
			Title: command.Request.Title, Description: command.Request.Description,
			DeliverableDirectory: command.Request.DeliverableDirectory, TemplateID: command.Request.TemplateID,
		})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIWorkspaceCreateResult{Workspace: readmodelports.PortWorkspaceInfo(item)}, nil)
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
		result.Set(workorchdevents.CLIWorkspaceGetResult{Workspace: readmodelports.PortWorkspaceInfo(item)}, nil)
	case workorchdevents.CLIWorkspaceListCommand:
		if d.viewReader == nil {
			setInvalidTypedCLICommand(result)
			return
		}
		items, err := d.viewReader.ListWorkspaceView(ctx)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIWorkspaceListResult{Workspaces: readmodelports.PortWorkspaceInfos(items)}, nil)
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
		item = ProjectWorkspaceSnapshotDetail(item)
		result.Set(workorchdevents.CLIWorkspaceSnapshotResult{Snapshot: readmodelports.PortWorkspaceSnapshotInfo(item)}, nil)
	case workorchdevents.CLIWorkspaceDeleteCommand:
		if command.WorkspaceID == "" {
			setInvalidTypedCLICommand(result)
			return
		}
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		err := DeleteWorkspaceWithSession(ctx, d.borrow(), string(command.WorkspaceID), DeleteWorkspaceCommand{Purge: command.Purge})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIWorkspaceDeleteResult{Deleted: true}, nil)
	case workorchdevents.CLIWorkspaceUpdateLLMTargetCommand:
		if command.WorkspaceID == "" {
			setInvalidTypedCLICommand(result)
			return
		}
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := UpdateWorkspaceLLMTargetWithHub(ctx, d.EventHub(), string(command.WorkspaceID), command.Target)
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIWorkspaceUpdateLLMTargetResult{Workspace: readmodelports.PortWorkspaceInfo(item)}, nil)
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
	if command.WorkspaceID == "" || d.viewReader == nil {
		setInvalidTypedCLICommand(result)
		return
	}
	ctx := ContextWithCommandHub(event.Context(), d.EventHub())
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
	item, err := d.viewReader.ReadWorkspaceViewSnapshot(ctx, string(command.WorkspaceID))
	if setTypedCLIError(result, err) {
		return
	}
	item = ProjectWorkspaceSnapshotDetail(item)
	activity, hasActivity, activityErr := d.viewReader.ReadWorkspaceActivity(ctx, string(command.WorkspaceID))
	if setTypedCLIError(result, activityErr) {
		return
	}
	view := readmodelports.ProjectAndPortWorkspaceRuntimeViewWithActivity(item, activity, hasActivity, command.ActivityLimit, command.BeforeCursor)
	result.Set(workorchdevents.ReadWorkspaceRuntimeViewResult{View: view}, nil)
}
