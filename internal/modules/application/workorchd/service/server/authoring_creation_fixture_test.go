package server

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// HTTP 资源测试只创建真实 owner draft，不生成审查、准入或执行凭据。
func (s *serverState) configureAuthoringCreationFixture(session *runtimecore.Session) {
	s.authoring.Create = func(ctx context.Context, backend, dataRoot string, request workorchmanagementcontract.DefinitionAuthoringCreate) (workorchmanagementcontract.DefinitionAuthoringView, error) {
		id := workspace.DefinitionAuthoringWorkspaceID(request.RequestID)
		layout, err := workspacecore.ResolveDirectoryLayout(backend, dataRoot, string(id), request.DeliverableDirectory)
		if err != nil {
			return workorchmanagementcontract.DefinitionAuthoringView{}, err
		}
		command := workspaceevents.CreateDefinitionAuthoringCommand{
			RequestID: request.RequestID, WorkspaceID: id, Title: strings.TrimSpace(request.Title), Description: strings.TrimSpace(request.Description),
			DeliverableDirectory: layout.DeliverableDirectory, InternalDirectory: layout.InternalDirectory,
			Input: workspace.DefinitionAuthoringInput{Version: workspace.DefinitionAuthoringInputVersionCurrent, WorkspaceID: id, Revision: 1, TemplateID: request.TemplateID, GoalText: request.Input.GoalText, AcceptanceRequirements: request.Input.AcceptanceRequirements, ResourceRefs: request.Input.ResourceRefs}.Normalized(),
		}
		if command.Validate() != nil {
			return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrInvalidArgument
		}
		hub, err := session.RequiredCommandHub()
		if err != nil {
			return workorchmanagementcontract.DefinitionAuthoringView{}, err
		}
		ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDefinitionAuthoringCreate, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		ev.BindLaneKey(events.EventHubLaneKeyFor(workspaceevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(id)}))
		result := hub.Send(ev)
		if result == nil {
			return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrConflict
		}
		value, getErr := result.Get()
		if getErr != nil {
			return workorchmanagementcontract.DefinitionAuthoringView{}, getErr
		}
		receipt, ok := value.(workspaceevents.CreateDefinitionAuthoringResult)
		if !ok || receipt.ValidateFor(command) != nil {
			return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrConflict
		}
		if err := workspacecore.EnsureDirectoryLayout(layout); err != nil {
			return workorchmanagementcontract.DefinitionAuthoringView{}, err
		}
		stored, err := s.readWorkspace(ctx, backend, dataRoot, string(id))
		if err != nil {
			return workorchmanagementcontract.DefinitionAuthoringView{}, err
		}
		if projection, ok := s.workspaceProjection.(interface {
			RebuildWorkspace(context.Context, string) error
		}); ok {
			if err := projection.RebuildWorkspace(ctx, string(id)); err != nil {
				return workorchmanagementcontract.DefinitionAuthoringView{}, err
			}
		}
		return workorchmanagementcontract.DefinitionAuthoringView{WorkspaceID: stored.ID, WorkspaceRevision: stored.Revision, State: "draft", TemplateID: request.TemplateID, InputHash: command.Input.InputHash(), Input: &request.Input}, nil
	}
}
