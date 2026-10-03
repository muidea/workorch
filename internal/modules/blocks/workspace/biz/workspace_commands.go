package workspace

import (
	"context"
	"fmt"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const workspaceDeletionIngressSource = "application/workorchd"

func (m *Biz) executeWorkspaceDefinitionCommand(ctx context.Context, ev magicevent.Event) (workspaceCommandResponse, error) {
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
	default:
		return workspaceCommandResponse{}, common.ErrInvalidArgument
	}
}

func (m *Biz) createWorkspace(ctx context.Context, ev magicevent.Event) (workspaceCommandResponse, error) {
	cmd, ok := ev.Data().(workspaceevents.CreateWorkspaceCommand)
	if !ok {
		return workspaceCommandResponse{}, common.ErrInvalidArgument
	}
	if err := m.manager.CreateWorkspace(ctx, workspaceevents.CreateWorkspaceCommand{
		WorkspaceID: cmd.WorkspaceID, Title: cmd.Title, Description: cmd.Description,
		DeliverableDirectory: cmd.DeliverableDirectory, InternalDirectory: cmd.InternalDirectory,
	}); err != nil {
		return workspaceCommandResponse{}, err
	}
	item, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspaceCommandResponse{}, err
	}
	return workspaceCommandResponse{Workspace: workspaceeventmapping.WorkspaceDTO(item)}, nil
}

func (m *Biz) getWorkspace(ctx context.Context, ev magicevent.Event) (workspaceCommandResponse, error) {
	cmd, ok := ev.Data().(workspaceevents.GetWorkspaceCommand)
	if !ok || cmd.WorkspaceID == "" || m.workspaces == nil {
		return workspaceCommandResponse{}, common.ErrInvalidArgument
	}
	item, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspaceCommandResponse{}, err
	}
	if workspaceDeletionCompleted(item) {
		return workspaceCommandResponse{}, common.ErrNotFound
	}
	return workspaceCommandResponse{Workspace: workspaceDTOWithAuthoringSummary(item)}, nil
}

func (m *Biz) listWorkspaces(ctx context.Context, ev magicevent.Event) (workspaceCommandResponse, error) {
	if _, ok := ev.Data().(workspaceevents.ListWorkspacesCommand); !ok || m.workspaces == nil {
		return workspaceCommandResponse{}, common.ErrInvalidArgument
	}
	items, err := m.workspaces.List(ctx)
	if err != nil {
		return workspaceCommandResponse{}, err
	}
	visible := make([]*workspacemodel.Workspace, 0, len(items))
	for _, item := range items {
		if !workspaceDeletionCompleted(item) {
			visible = append(visible, item)
		}
	}
	return workspaceCommandResponse{Workspaces: workspaceDTOsWithAuthoringSummary(visible)}, nil
}

func workspaceDTOWithAuthoringSummary(item *workspacemodel.Workspace) *workspaceevents.Workspace {
	dto := workspaceeventmapping.WorkspaceDTO(item)
	if dto != nil {
		dto.AuthoringSummary = nil
	}
	if item == nil || dto == nil || item.DefinitionAuthoring == nil || item.DefinitionAuthoringConfirmation != nil || item.StartupInput != nil || item.ActivationAdmission != nil || item.ActiveRunID != "" || len(item.DefinitionAdmissionCandidates) != len(item.DefinitionAuthoringHistory) {
		return dto
	}
	state, failurePhase, failureCode, reviewCount := item.DefinitionAuthoring.CurrentAuthoringSummary()
	if state == "" {
		return dto
	}
	dto.AuthoringSummary = &workspaceevents.AuthoringSummary{
		State: state, FailurePhase: failurePhase, FailureCode: failureCode,
		ReviewCount: reviewCount, InputRevision: item.DefinitionAuthoring.Input.Revision,
	}
	return dto
}

func workspaceDTOsWithAuthoringSummary(items []*workspacemodel.Workspace) []*workspaceevents.Workspace {
	if items == nil {
		return nil
	}
	ret := make([]*workspaceevents.Workspace, 0, len(items))
	for _, item := range items {
		ret = append(ret, workspaceDTOWithAuthoringSummary(item))
	}
	return ret
}

func (m *Biz) updateWorkspace(ctx context.Context, ev magicevent.Event) (workspaceCommandResponse, error) {
	cmd, ok := ev.Data().(workspaceevents.UpdateWorkspaceCommand)
	if !ok || cmd.WorkspaceID == "" || m.workspaces == nil || soulbinding.Validate(cmd.SoulBindings) != nil {
		return workspaceCommandResponse{}, common.ErrInvalidArgument
	}
	current, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspaceCommandResponse{}, err
	}
	if err := workspaceDeletionRejectsMutation(current); err != nil {
		return workspaceCommandResponse{}, err
	}
	if current.DefinitionAuthoringConfirmation != nil {
		return workspaceCommandResponse{}, common.ErrConflict
	}
	// Generic metadata updates deliberately preserve lifecycle-owned fields.
	// Workspace.State and ActiveRunID may only be changed by lifecycle_reducer.
	item := workspaceFromUpdateCommand(current, cmd)
	if cmd.TemplateBindingHash != "" && (cmd.TemplateID != current.TemplateID || cmd.TemplateBindingHash != current.TemplateBindingHash) {
		// A materialized binding claims one exact live Template observation.
		// Serialize its verification/persistence with Template CRUD.
		m.templateMutationMu.Lock()
		defer m.templateMutationMu.Unlock()
		if m.templates == nil {
			return workspaceCommandResponse{}, common.ErrInvalidArgument
		}
		template, err := m.templates.Get(ctx, cmd.TemplateID)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
		if workspacemodel.WorkspaceTemplateSnapshotHash(template) != cmd.TemplateBindingHash {
			return workspaceCommandResponse{}, common.ErrConflict
		}
		item.TemplateBindingHash = cmd.TemplateBindingHash
	}
	if err := m.workspaces.CompareAndSwap(ctx, item, item.Revision); err != nil {
		return workspaceCommandResponse{}, err
	}
	m.publishWorkspaceUpdated(ctx, item)
	return workspaceCommandResponse{Workspace: workspaceeventmapping.WorkspaceDTO(item)}, nil
}

func (m *Biz) updateWorkspaceLLMSelection(ctx context.Context, ev magicevent.Event) (workspaceCommandResponse, error) {
	cmd, ok := ev.Data().(workspaceevents.UpdateWorkspaceLLMSelectionCommand)
	if !ok || cmd.WorkspaceID == "" || m.workspaces == nil {
		return workspaceCommandResponse{}, common.ErrInvalidArgument
	}
	item, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspaceCommandResponse{}, err
	}
	if err := workspaceDeletionRejectsMutation(item); err != nil {
		return workspaceCommandResponse{}, err
	}
	if item.DefinitionAuthoringConfirmation != nil {
		return workspaceCommandResponse{}, common.ErrConflict
	}
	if cmd.ExpectedRevision == 0 || item.Revision != cmd.ExpectedRevision {
		return workspaceCommandResponse{}, common.ErrConflict
	}
	currentSelection := runtimecontract.CloneProviderInferenceSelection(item.DefaultLLMSelection)
	if cmd.Selection == nil {
		item.DefaultLLMSelection = nil
	} else {
		selection := cmd.Selection.Normalized()
		if err := selection.Validate(); err != nil {
			return workspaceCommandResponse{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
		}
		item.DefaultLLMSelection = &selection
	}
	if (currentSelection == nil && item.DefaultLLMSelection == nil) ||
		(currentSelection != nil && item.DefaultLLMSelection != nil && currentSelection.Same(*item.DefaultLLMSelection)) {
		return workspaceCommandResponse{Workspace: workspaceeventmapping.WorkspaceDTO(item), Changed: false}, nil
	}
	if err := m.workspaces.CompareAndSwap(ctx, item, cmd.ExpectedRevision); err != nil {
		return workspaceCommandResponse{}, err
	}
	m.publishWorkspaceUpdated(ctx, item)
	return workspaceCommandResponse{Workspace: workspaceeventmapping.WorkspaceDTO(item), Changed: true}, nil
}

func (m *Biz) deleteWorkspace(ctx context.Context, ev magicevent.Event) (workspaceevents.DeleteWorkspaceResult, error) {
	if ev == nil || ev.Source() != workspaceDeletionIngressSource {
		return workspaceevents.DeleteWorkspaceResult{}, common.ErrInvalidArgument
	}
	cmd, ok := ev.Data().(workspaceevents.DeleteWorkspaceCommand)
	if !ok {
		return workspaceevents.DeleteWorkspaceResult{}, common.ErrInvalidArgument
	}
	cmd = cmd.Normalized()
	if err := cmd.Validate(); err != nil {
		return workspaceevents.DeleteWorkspaceResult{}, err
	}
	transition, err := m.submitWorkspaceDeletionInternalEvent(ctx, workspaceDeletionInternalEvent{
		Kind:      workspaceDeletionInternalEventRequested,
		Requested: &workspaceDeletionRequestedFact{Command: cmd, OccurredAt: time.Now().UTC()},
	})
	if err != nil {
		return workspaceevents.DeleteWorkspaceResult{}, err
	}
	item, err := m.driveWorkspaceDeletion(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspaceevents.DeleteWorkspaceResult{}, err
	}
	if item == nil || item.Deletion == nil || !item.Deletion.Completed {
		return workspaceevents.DeleteWorkspaceResult{}, common.ErrConflict
	}
	disposition := workspaceevents.DeleteWorkspaceCompleted
	if transition.Duplicate {
		disposition = workspaceevents.DeleteWorkspaceDuplicate
	}
	response := workspaceevents.DeleteWorkspaceResult{
		WorkspaceID: item.ID, DeletionID: item.Deletion.DeletionID, RequestID: item.Deletion.RequestID,
		Mode: item.Deletion.Mode, Disposition: disposition, DeletionRevision: item.Deletion.Revision,
		WorkspaceRevision: item.Revision, Completed: true,
	}
	if err := response.ValidateFor(cmd); err != nil {
		return workspaceevents.DeleteWorkspaceResult{}, err
	}
	return response, nil
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
	if cmd.TemplateID != current.TemplateID {
		item.TemplateBindingHash = ""
	}
	item.ProfileID = cmd.ProfileID
	item.DefaultLLMSelection = runtimecontract.CloneProviderInferenceSelection(cmd.DefaultLLMSelection)
	item.DefaultSoulID = cmd.DefaultSoulID
	item.SoulBindings = soulbinding.SoulIDMap(cmd.SoulBindings)
	item.ContextHints = cmd.ContextHints.Clone()
	item.EvidencePolicies = workorcheventpayload.CloneSlice(cmd.EvidencePolicies)
	return &item
}
