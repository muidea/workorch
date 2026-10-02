package workspace

import (
	"context"

	definitionadmission "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/definitionadmission"

	workspaceeventmapping "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) queryDefinitionAuthoring(ctx context.Context, cmd workspaceevents.QueryDefinitionAuthoringCommand) (workspaceevents.QueryDefinitionAuthoringResult, error) {
	if m == nil || m.manager == nil || m.workspaces == nil || ctx == nil || cmd.WorkspaceID == "" {
		return workspaceevents.QueryDefinitionAuthoringResult{}, common.ErrInvalidArgument
	}
	m.manager.definitionMu.Lock()
	defer m.manager.definitionMu.Unlock()
	m.templateMutationMu.Lock()
	defer m.templateMutationMu.Unlock()
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspaceevents.QueryDefinitionAuthoringResult{}, err
	}
	w, err := m.workspaces.Get(bound, cmd.WorkspaceID)
	if err != nil {
		return workspaceevents.QueryDefinitionAuthoringResult{}, err
	}
	if err := workspaceDeletionRejectsMutation(w); err != nil {
		return workspaceevents.QueryDefinitionAuthoringResult{}, err
	}
	result := workspaceevents.QueryDefinitionAuthoringResult{WorkspaceID: cmd.WorkspaceID, WorkspaceRevision: w.Revision, DeliverableDirectory: w.DeliverableDirectory}
	if template, templateErr := m.resolvedTemplate(bound, w.TemplateID); templateErr == nil {
		result.CurrentTemplateSnapshotHash = workspacemodel.WorkspaceTemplateSnapshotHash(template)
		result.CurrentTemplateAvailable = true
	} else if w.DefinitionAuthoring == nil {
		return workspaceevents.QueryDefinitionAuthoringResult{}, templateErr
	}
	// 有已封存草稿时，现行模板删除也不能阻断历史请求查询及完成。
	if w.DefinitionAuthoring != nil {
		record := workspaceeventmapping.DefinitionAuthoringRecordDTO(*w.DefinitionAuthoring)
		result.Record = &record
	}
	candidate, present, err := w.ValidateActivationAdmissionPersistence()
	if err != nil {
		return workspaceevents.QueryDefinitionAuthoringResult{}, err
	}
	result.History = workspaceeventmapping.DefinitionAuthoringHistoryDTOs(w.DefinitionAuthoringHistory)
	if w.DefinitionAuthoringConfirmation != nil {
		confirmation := w.DefinitionAuthoringConfirmation.Clone()
		result.Confirmation = &confirmation
	}
	if len(w.DefinitionAdmissionCandidates) > 0 && w.DefinitionAuthoringConfirmation != nil {
		if !present && len(w.DefinitionAdmissionCandidates) != len(w.DefinitionAuthoringHistory) {
			candidate = w.DefinitionAdmissionCandidates[len(w.DefinitionAdmissionCandidates)-1].Clone()
		}
		if present || len(w.DefinitionAdmissionCandidates) != len(w.DefinitionAuthoringHistory) {
			admission, err := definitionadmission.Compile(candidate)
			if err != nil {
				return workspaceevents.QueryDefinitionAuthoringResult{}, err
			}
			result.AdmissionCandidate = &candidate
			result.AdmissionResult = &admission
			if prior := outstandingPreSealDefinitionClarification(w, candidate); prior != nil {
				binding := prior.Clone()
				result.PriorClarification = &binding
			}
			if present {
				result = withAuthoringActivation(result, w.ActivationAdmission)
			}
		}
	}
	if err := result.ValidateFor(cmd); err != nil {
		return workspaceevents.QueryDefinitionAuthoringResult{}, err
	}
	return result, nil
}

func withAuthoringActivation(result workspaceevents.QueryDefinitionAuthoringResult, admission *workspacemodel.WorkspaceActivationAdmission) workspaceevents.QueryDefinitionAuthoringResult {
	if admission != nil {
		activation := admission.Normalized()
		return workspaceevents.QueryDefinitionAuthoringResult{
			WorkspaceID: result.WorkspaceID, WorkspaceRevision: result.WorkspaceRevision,
			Record: result.Record, History: result.History, Confirmation: result.Confirmation, PriorClarification: result.PriorClarification, AdmissionCandidate: result.AdmissionCandidate,
			AdmissionResult: result.AdmissionResult, ActivationAdmission: &activation,
			DeliverableDirectory: result.DeliverableDirectory, CurrentTemplateSnapshotHash: result.CurrentTemplateSnapshotHash,
			CurrentTemplateAvailable: result.CurrentTemplateAvailable,
		}
	}
	return result
}
