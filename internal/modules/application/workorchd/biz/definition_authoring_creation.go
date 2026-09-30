package biz

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func (b *Biz) CreateDefinitionAuthoringView(ctx context.Context, backend, dataDir string, request workorchmanagementcontract.DefinitionAuthoringCreate) (workorchmanagementcontract.DefinitionAuthoringView, error) {
	if b == nil || b.EventHub() == nil || ctx == nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrInvalidArgument
	}
	id := workspace.DefinitionAuthoringWorkspaceID(request.RequestID)
	layout, err := workspacecore.ResolveDirectoryLayout(backend, dataDir, string(id), request.DeliverableDirectory)
	if err != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, err
	}
	cmd := workspaceevents.CreateDefinitionAuthoringCommand{
		RequestID: request.RequestID, WorkspaceID: id, Title: strings.TrimSpace(request.Title), Description: strings.TrimSpace(request.Description),
		DeliverableDirectory: layout.DeliverableDirectory, InternalDirectory: layout.InternalDirectory,
		Input: workspace.DefinitionAuthoringInput{Version: workspace.DefinitionAuthoringInputVersionCurrent, WorkspaceID: id, Revision: 1, TemplateID: request.TemplateID, GoalText: request.Input.GoalText, AcceptanceRequirements: request.Input.AcceptanceRequirements, ResourceRefs: request.Input.ResourceRefs}.Normalized(),
	}
	if err := cmd.Validate(); err != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, err
	}
	if err := ctx.Err(); err != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDefinitionAuthoringCreate, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, cmd)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceDefinitionAuthoringCreate, eventcontract.LaneScope{WorkspaceID: string(id)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, err
	}
	value, getErr := result.Get()
	if getErr != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, getErr
	}
	receipt, ok := value.(workspaceevents.CreateDefinitionAuthoringResult)
	if !ok || receipt.ValidateFor(cmd) != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, common.ErrConflict
	}
	// 先持久身份，目录初始化失败后同一创建请求仍能恢复原 Workspace。
	if err := workspacecore.EnsureDirectoryLayout(layout); err != nil {
		return workorchmanagementcontract.DefinitionAuthoringView{}, err
	}
	return b.ReadDefinitionAuthoringView(ctx, id)
}
