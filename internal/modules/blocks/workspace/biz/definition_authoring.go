package workspace

import (
	"context"
	"reflect"

	workspaceeventmapping "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/eventmapping"
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
	if replay, matched, err := replayDefinitionAuthoringSave(w, cmd); matched || err != nil {
		return replay, err
	}
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
	if w.Revision != cmd.ExpectedWorkspaceRevision || (previous != nil && cmd.ExpectedInputHash != previous.InputHash) || (previous == nil && cmd.ExpectedInputHash != "") {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	receipt := workspacemodel.DefinitionAuthoringSaveReceipt{RequestID: cmd.RequestID, RequestHash: workspacemodel.DefinitionAuthoringSaveRequestHash(cmd.RequestID, cmd.Input, cmd.ExpectedWorkspaceRevision, cmd.ExpectedInputHash, cmd.DeliverableDirectory), InputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, WorkspaceRevision: w.Revision + 1}
	if w.Revision == ^uint64(0) {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if previous != nil {
		if previous.Validate() != nil || previous.Input.WorkspaceID != w.ID {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
		if len(previous.SaveReceipts) >= workspacemodel.DefinitionAuthoringSaveReceiptLimit {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
		record.SaveReceipts = append(append([]workspacemodel.DefinitionAuthoringSaveReceipt(nil), previous.SaveReceipts...), receipt)
		if previous.InputHash == record.InputHash && previous.TemplateSnapshotHash == record.TemplateSnapshotHash && w.TemplateID == input.TemplateID && directory == w.DeliverableDirectory && w.DefinitionAuthoringConfirmation == nil {
			next := *w
			copy := previous.Clone()
			copy.SaveReceipts = record.SaveReceipts
			next.DefinitionAuthoring = &copy
			return definitionSpecificationTransition{Next: &next, Authoring: &workspaceevents.SaveDefinitionAuthoringResult{Record: workspaceeventmapping.DefinitionAuthoringRecordDTO(copy), Receipt: receipt}}, nil
		}
		if previous.InputHash == record.InputHash && previous.TemplateSnapshotHash == record.TemplateSnapshotHash {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
		record.DerivationRequests = previous.Clone().DerivationRequests
		record.SpecificationRequests = previous.Clone().SpecificationRequests
		record.PreparationDiagnostics = previous.Clone().PreparationDiagnostics
		if previous.Input.Revision == ^uint64(0) || input.Revision != previous.Input.Revision+1 {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
	} else {
		if input.Revision != 1 {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
		record.SaveReceipts = []workspacemodel.DefinitionAuthoringSaveReceipt{receipt}
	}
	if err := record.Validate(); err != nil {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if w.DefinitionAuthoringConfirmation != nil && len(w.DefinitionAdmissionCandidates) > 0 && previous != nil && previous.InputHash == w.DefinitionAuthoringConfirmation.InputHash {
		archived := workspacemodel.DefinitionAuthoringConfirmedRevision{Record: previous.Clone(), Confirmation: w.DefinitionAuthoringConfirmation.Clone()}
		next := *w
		if err := applyAuthoringConfiguration(&next, o.template, directory); err != nil {
			return definitionSpecificationTransition{}, err
		}
		next.DefinitionAuthoringHistory = append(append([]workspacemodel.DefinitionAuthoringConfirmedRevision(nil), w.DefinitionAuthoringHistory...), archived)
		next.DefinitionAuthoring = &record
		next.DefinitionAuthoringConfirmation = nil
		if next.ValidateDefinitionAuthoringHistory() != nil {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
		return definitionSpecificationTransition{Next: &next, Authoring: &workspaceevents.SaveDefinitionAuthoringResult{Record: workspaceeventmapping.DefinitionAuthoringRecordDTO(record), Receipt: receipt}}, nil
	}
	if w.DefinitionAuthoringConfirmation != nil {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if len(w.DefinitionAdmissionCandidates) > 0 && !authoringSuccessorDraft(w) {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	next := *w
	if err := applyAuthoringConfiguration(&next, o.template, directory); err != nil {
		return definitionSpecificationTransition{}, err
	}
	next.DefinitionAuthoring = &record
	return definitionSpecificationTransition{Next: &next, Authoring: &workspaceevents.SaveDefinitionAuthoringResult{Record: workspaceeventmapping.DefinitionAuthoringRecordDTO(record), Receipt: receipt}}, nil
}

func applyAuthoringConfiguration(w *workspacemodel.Workspace, template *workspacemodel.WorkspaceTemplate, directory string) error {
	if w.TemplateID != template.ID {
		w.TemplateBindingHash = ""
	}
	w.TemplateID = template.ID
	w.DeliverableDirectory = directory
	next, err := workspacemodel.WorkspaceForCurrentTemplate(w, template)
	if err != nil {
		return err
	}
	*w = *next
	return nil
}

// 精确回执重放不观察现行模板，但必须校验 owner scope 与原 CAS 结果。
func replayDefinitionAuthoringSave(w *workspacemodel.Workspace, cmd workspaceevents.SaveDefinitionAuthoringCommand) (definitionSpecificationTransition, bool, error) {
	if w == nil || w.ID != cmd.Input.WorkspaceID {
		return definitionSpecificationTransition{}, false, common.ErrConflict
	}
	if w.DefinitionAuthoring == nil {
		return definitionSpecificationTransition{}, false, nil
	}
	if w.DefinitionAuthoring.Validate() != nil || w.DefinitionAuthoring.Input.WorkspaceID != w.ID {
		return definitionSpecificationTransition{}, false, common.ErrConflict
	}
	for _, receipt := range w.DefinitionAuthoring.SaveReceipts {
		if receipt.WorkspaceRevision > w.Revision {
			return definitionSpecificationTransition{}, false, common.ErrConflict
		}
		if receipt.RequestID != cmd.RequestID {
			continue
		}
		if cmd.ExpectedWorkspaceRevision == ^uint64(0) || receipt.WorkspaceRevision != cmd.ExpectedWorkspaceRevision+1 || receipt.RequestHash != workspacemodel.DefinitionAuthoringSaveRequestHash(cmd.RequestID, cmd.Input, cmd.ExpectedWorkspaceRevision, cmd.ExpectedInputHash, cmd.DeliverableDirectory) {
			return definitionSpecificationTransition{}, false, common.ErrConflict
		}
		return definitionSpecificationTransition{Authoring: &workspaceevents.SaveDefinitionAuthoringResult{Record: workspaceeventmapping.DefinitionAuthoringRecordDTO(*w.DefinitionAuthoring), Receipt: receipt}}, true, nil
	}
	return definitionSpecificationTransition{}, false, nil
}
