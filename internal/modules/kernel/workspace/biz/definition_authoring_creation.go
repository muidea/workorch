package workspace

import (
	"context"
	"errors"

	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) createDefinitionAuthoring(ctx context.Context, cmd workspaceevents.CreateDefinitionAuthoringCommand) (workspaceevents.CreateDefinitionAuthoringResult, error) {
	if cmd.Validate() != nil || m == nil || m.manager == nil {
		return workspaceevents.CreateDefinitionAuthoringResult{}, common.ErrInvalidArgument
	}
	m.manager.definitionMu.Lock()
	defer m.manager.definitionMu.Unlock()
	m.templateMutationMu.Lock()
	defer m.templateMutationMu.Unlock()
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspaceevents.CreateDefinitionAuthoringResult{}, err
	}
	// 重放不重新读取模板；模板删除也不能让初始创建再执行一次。
	fact := workspaceCreationRequestedFact{WorkspaceID: cmd.WorkspaceID, Title: cmd.Title, Description: cmd.Description, DeliverableDirectory: cmd.DeliverableDirectory, InternalDirectory: cmd.InternalDirectory, Authoring: &cmd}
	_, err = m.workspaces.Get(bound, cmd.WorkspaceID)
	fresh := errors.Is(err, common.ErrNotFound)
	if fresh {
		fact.Template, err = m.resolvedTemplate(bound, cmd.Input.TemplateID)
	}
	if err != nil {
		return workspaceevents.CreateDefinitionAuthoringResult{}, err
	}
	created, err := m.manager.submitWorkspaceCreationInternalEvent(bound, workspaceCreationInternalEvent{Kind: workspaceCreationInternalEventRequested, Requested: &fact})
	if err != nil {
		return workspaceevents.CreateDefinitionAuthoringResult{}, err
	}
	if fresh {
		if err := m.manager.publishWorkspaceEvent(bound, workspaceevents.TopicWorkspaceCreated, created); err != nil {
			return workspaceevents.CreateDefinitionAuthoringResult{}, err
		}
	}
	result := workspaceevents.CreateDefinitionAuthoringResult{WorkspaceID: cmd.WorkspaceID, RequestID: cmd.RequestID, InputHash: cmd.CreationInputHash()}
	return result, result.ValidateFor(cmd)
}
