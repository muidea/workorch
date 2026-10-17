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
	// 通知只是读取刷新提示；重放也补发当前事实，不把 Post 当持久完成回执。
	if err := m.manager.publishWorkspaceEvent(bound, workspaceevents.TopicWorkspaceCreated, created); err != nil {
		return workspaceevents.CreateDefinitionAuthoringResult{}, err
	}
	result := workspaceevents.CreateDefinitionAuthoringResult{WorkspaceID: cmd.WorkspaceID, RequestID: cmd.RequestID, InputHash: cmd.CreationInputHash()}
	return result, result.ValidateFor(cmd)
}

func (m *Biz) recordAuthoringDirectoryPrepared(ctx context.Context, cmd workspaceevents.RecordAuthoringDirectoryPreparedCommand) (workspaceevents.RecordAuthoringDirectoryPreparedResult, error) {
	if m == nil || m.manager == nil || ctx == nil || cmd.Creation.Validate() != nil {
		return workspaceevents.RecordAuthoringDirectoryPreparedResult{}, common.ErrInvalidArgument
	}
	m.manager.definitionMu.Lock()
	defer m.manager.definitionMu.Unlock()
	c := cmd.Creation
	bound, err := common.BindRuntimeWorkspaceContext(ctx, c.WorkspaceID)
	if err != nil {
		return workspaceevents.RecordAuthoringDirectoryPreparedResult{}, err
	}
	w, err := m.workspaces.Get(bound, c.WorkspaceID)
	if err != nil {
		return workspaceevents.RecordAuthoringDirectoryPreparedResult{}, err
	}
	if err := workspaceDeletionRejectsMutation(w); err != nil {
		return workspaceevents.RecordAuthoringDirectoryPreparedResult{}, err
	}
	creation := w.DefinitionAuthoringCreation
	if creation == nil || creation.Validate(w.ID) != nil || creation.RequestID != c.RequestID || creation.InputHash != c.CreationInputHash() || creation.PendingPersistence != nil || w.DeliverableDirectory != c.DeliverableDirectory || w.InternalDirectory != c.InternalDirectory {
		return workspaceevents.RecordAuthoringDirectoryPreparedResult{}, common.ErrConflict
	}
	if !creation.DirectoryPrepared {
		next := *w
		prepared := creation.Clone()
		prepared.DirectoryPrepared = true
		next.DefinitionAuthoringCreation = &prepared
		if err := m.workspaces.CompareAndSwap(bound, &next, w.Revision); err != nil {
			return workspaceevents.RecordAuthoringDirectoryPreparedResult{}, err
		}
	}
	return workspaceevents.RecordAuthoringDirectoryPreparedResult{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, InputHash: c.CreationInputHash()}, nil
}
