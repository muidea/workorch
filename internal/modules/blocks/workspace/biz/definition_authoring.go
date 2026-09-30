package workspace

import (
	"context"
	"reflect"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// 与规格编译共用 owner 锁及唯一 CAS dispatcher，但不读取尚不存在的 Goal/DoD。
func (m *Biz) observeDefinitionAuthoring(ctx context.Context, id common.WorkspaceID) (definitionSpecificationObservation, error) {
	w, err := m.workspaces.Get(ctx, id)
	if err != nil {
		return definitionSpecificationObservation{}, err
	}
	if err := workspaceDeletionRejectsMutation(w); err != nil {
		return definitionSpecificationObservation{}, err
	}
	goals, err := m.goals.ListByWorkspaceID(ctx, id)
	if err != nil {
		return definitionSpecificationObservation{}, err
	}
	dods, err := m.dods.ListByWorkspaceID(ctx, id)
	if err != nil {
		return definitionSpecificationObservation{}, err
	}
	// 正式定义与新建草稿不能成为并行的输入权威。
	if len(goals) != 0 || len(dods) != 0 {
		// Prepared confirmation may have written only one formal repository. Its
		// after-images are still non-authoritative until the journal completes.
		if w.DefinitionAuthoringConfirmation != nil {
			if w.DefinitionAuthoringConfirmation.Validate(w.DefinitionAuthoring, w.ID) != nil || len(goals) < len(w.DefinitionAuthoringHistory) || len(goals) > len(w.DefinitionAuthoringHistory)+1 || len(dods) < len(w.DefinitionAuthoringHistory) || len(dods) > len(w.DefinitionAuthoringHistory)+1 {
				return definitionSpecificationObservation{}, common.ErrConflict
			}
			if len(goals) > len(w.DefinitionAuthoringHistory) && (currentGoalLeaf(goals, id) == nil || !reflect.DeepEqual(*currentGoalLeaf(goals, id), w.DefinitionAuthoringConfirmation.Goal)) || len(dods) > len(w.DefinitionAuthoringHistory) && (currentDoDLeaf(dods, id) == nil || !reflect.DeepEqual(*currentDoDLeaf(dods, id), w.DefinitionAuthoringConfirmation.DoD)) {
				return definitionSpecificationObservation{}, common.ErrConflict
			}
			if len(goals) == len(w.DefinitionAuthoringHistory) && len(goals) > 0 && (currentGoalLeaf(goals, id) == nil || !reflect.DeepEqual(*currentGoalLeaf(goals, id), w.DefinitionAuthoringHistory[len(w.DefinitionAuthoringHistory)-1].Confirmation.Goal)) || len(dods) == len(w.DefinitionAuthoringHistory) && len(dods) > 0 && (currentDoDLeaf(dods, id) == nil || !reflect.DeepEqual(*currentDoDLeaf(dods, id), w.DefinitionAuthoringHistory[len(w.DefinitionAuthoringHistory)-1].Confirmation.DoD)) {
				return definitionSpecificationObservation{}, common.ErrConflict
			}
		} else if !authoringSuccessorDraft(w) || len(goals) != len(w.DefinitionAuthoringHistory) || len(dods) != len(w.DefinitionAuthoringHistory) || currentGoalLeaf(goals, id) == nil || currentDoDLeaf(dods, id) == nil || !reflect.DeepEqual(*currentGoalLeaf(goals, id), w.DefinitionAuthoringHistory[len(w.DefinitionAuthoringHistory)-1].Confirmation.Goal) || !reflect.DeepEqual(*currentDoDLeaf(dods, id), w.DefinitionAuthoringHistory[len(w.DefinitionAuthoringHistory)-1].Confirmation.DoD) {
			return definitionSpecificationObservation{}, common.ErrConflict
		}
	}
	// 完成历史派生回执不依赖可变的现行模板。仅新保存与 reserve 校验模板。
	return definitionSpecificationObservation{workspace: w}, nil
}

func reduceDefinitionAuthoring(o definitionSpecificationObservation, cmd workspaceevents.SaveDefinitionAuthoringCommand) (definitionSpecificationTransition, error) {
	if err := cmd.Validate(); err != nil {
		return definitionSpecificationTransition{}, common.ErrInvalidArgument
	}
	input := cmd.Input.Normalized()
	w := o.workspace
	if w == nil || o.template == nil || input.WorkspaceID != w.ID || o.template.ID != input.TemplateID {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	// 此入口仅服务新建草稿，不是同 Run 规格修订或旧准入的降级通道。
	if w.StartupInput != nil || w.ActivationAdmission != nil || w.ActiveRunID != "" || w.DefinitionSpecificationLedger != nil {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if len(w.DefinitionAdmissionCandidates) != 0 || w.DefinitionAuthoringConfirmation != nil || len(w.DefinitionAuthoringHistory) != 0 {
		if !blockedAuthoringSuccessor(w) && !authoringSuccessorDraft(w) {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
	}
	if err := w.ValidateDefinitionAuthoringHistory(); err != nil {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	directory := w.DeliverableDirectory
	if cmd.DeliverableDirectory != nil {
		directory = *cmd.DeliverableDirectory
	}
	record := workspacemodel.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshotHash: workspacemodel.WorkspaceTemplateSnapshotHash(o.template), TemplateSnapshot: o.template.Clone()}
	previous := w.DefinitionAuthoring
	if previous != nil {
		if previous.Validate() != nil || previous.Input.WorkspaceID != w.ID {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
		if previous.InputHash == record.InputHash && previous.TemplateSnapshotHash == record.TemplateSnapshotHash && w.TemplateID == input.TemplateID && directory == w.DeliverableDirectory && w.DefinitionAuthoringConfirmation == nil {
			return definitionSpecificationTransition{Authoring: &workspaceevents.SaveDefinitionAuthoringResult{Record: previous.Clone()}}, nil
		}
		if previous.InputHash == record.InputHash && previous.TemplateSnapshotHash == record.TemplateSnapshotHash {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
		record.DerivationRequests = previous.Clone().DerivationRequests
		record.SpecificationRequests = previous.Clone().SpecificationRequests
		if previous.Input.Revision == ^uint64(0) || input.Revision != previous.Input.Revision+1 {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
	} else if input.Revision != 1 {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if w.Revision != cmd.ExpectedWorkspaceRevision {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if err := record.Validate(); err != nil {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if w.DefinitionAuthoringConfirmation != nil && len(w.DefinitionAdmissionCandidates) > 0 && previous != nil && previous.InputHash == w.DefinitionAuthoringConfirmation.InputHash {
		archived := workspacemodel.DefinitionAuthoringConfirmedRevision{Record: previous.Clone(), Confirmation: w.DefinitionAuthoringConfirmation.Clone()}
		next := *w
		applyAuthoringConfiguration(&next, input.TemplateID, directory)
		next.DefinitionAuthoringHistory = append(append([]workspacemodel.DefinitionAuthoringConfirmedRevision(nil), w.DefinitionAuthoringHistory...), archived)
		next.DefinitionAuthoring = &record
		next.DefinitionAuthoringConfirmation = nil
		if next.ValidateDefinitionAuthoringHistory() != nil {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
		return definitionSpecificationTransition{Next: &next, Authoring: &workspaceevents.SaveDefinitionAuthoringResult{Record: record.Clone()}}, nil
	}
	if w.DefinitionAuthoringConfirmation != nil {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if len(w.DefinitionAdmissionCandidates) > 0 && !authoringSuccessorDraft(w) {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	next := *w
	applyAuthoringConfiguration(&next, input.TemplateID, directory)
	next.DefinitionAuthoring = &record
	return definitionSpecificationTransition{Next: &next, Authoring: &workspaceevents.SaveDefinitionAuthoringResult{Record: record.Clone()}}, nil
}

func applyAuthoringConfiguration(w *workspacemodel.Workspace, templateID, directory string) {
	if w.TemplateID != templateID {
		w.TemplateBindingHash = ""
	}
	w.TemplateID = templateID
	w.DeliverableDirectory = directory
}
