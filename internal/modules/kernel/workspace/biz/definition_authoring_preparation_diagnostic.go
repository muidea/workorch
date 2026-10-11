package workspace

import (
	"context"

	workspaceeventmapping "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func preparationDiagnosticInWorkspace(w *workspace.Workspace, id string) (workspace.DefinitionAuthoringPreparationDiagnostic, bool, error) {
	if w == nil || w.DefinitionAuthoring == nil {
		return workspace.DefinitionAuthoringPreparationDiagnostic{}, false, common.ErrNotFound
	}
	if err := validateWorkspaceAuthoringMappingOutcomes(w); err != nil {
		return workspace.DefinitionAuthoringPreparationDiagnostic{}, false, err
	}
	if w.DefinitionAuthoring.Input.WorkspaceID != w.ID || w.DefinitionAuthoring.Validate() != nil || w.ValidateDefinitionAuthoringHistory() != nil {
		return workspace.DefinitionAuthoringPreparationDiagnostic{}, false, common.ErrConflict
	}
	records := []*workspace.DefinitionAuthoringRecord{w.DefinitionAuthoring}
	if w.DefinitionChangeCandidate != nil {
		records = append(records, &w.DefinitionChangeCandidate.Record)
	}
	for n := range w.DefinitionAuthoringHistory {
		records = append(records, &w.DefinitionAuthoringHistory[n].Record)
	}
	var found workspace.DefinitionAuthoringPreparationDiagnostic
	matched := false
	for _, record := range records {
		for _, diagnostic := range record.PreparationDiagnostics {
			if diagnostic.ObservedWorkspaceRevision > w.Revision {
				return workspace.DefinitionAuthoringPreparationDiagnostic{}, false, common.ErrConflict
			}
			if diagnostic.DiagnosticID == id {
				if matched && (diagnostic.ExpectedDiagnosticID() != found.ExpectedDiagnosticID() || diagnostic.ObservedWorkspaceRevision != found.ObservedWorkspaceRevision) {
					return workspace.DefinitionAuthoringPreparationDiagnostic{}, false, common.ErrConflict
				}
				found, matched = diagnostic.Clone(), true
			}
		}
	}
	return found, matched, nil
}

func replayPreparationDiagnostic(w *workspace.Workspace, cmd workspaceevents.RecordPreparationDiagnosticCommand) (definitionSpecificationTransition, bool, error) {
	if w == nil || w.ID != cmd.Diagnostic.WorkspaceID {
		return definitionSpecificationTransition{}, false, common.ErrConflict
	}
	if err := workspaceDeletionRejectsMutation(w); err != nil {
		return definitionSpecificationTransition{}, false, err
	}
	d, matched, err := preparationDiagnosticInWorkspace(w, cmd.Diagnostic.DiagnosticID)
	if err != nil {
		return definitionSpecificationTransition{}, false, err
	}
	if !matched {
		return definitionSpecificationTransition{}, false, nil
	}
	result := workspaceevents.RecordPreparationDiagnosticResult{WorkspaceID: w.ID, WorkspaceRevision: w.Revision, Diagnostic: workspaceeventmapping.DefinitionAuthoringPreparationDiagnosticMetadataDTO(d), Duplicate: true}
	if result.ValidateFor(cmd) != nil {
		return definitionSpecificationTransition{}, false, common.ErrConflict
	}
	return definitionSpecificationTransition{PreparationDiagnostic: &result}, true, nil
}

func reducePreparationDiagnostic(o definitionSpecificationObservation, cmd workspaceevents.RecordPreparationDiagnosticCommand) (definitionSpecificationTransition, error) {
	w, d := o.workspace, cmd.Diagnostic
	if replay, matched, err := replayPreparationDiagnostic(w, cmd); matched || err != nil {
		return replay, err
	}
	if w.ActiveRunID != "" || w.DefinitionSpecificationLedger != nil || workspaceDeletionRejectsMutation(w) != nil {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	r, candidate, err := authoringMutationRecord(w, d.InputHash, "", false)
	if err != nil {
		return definitionSpecificationTransition{}, err
	}
	if w.Revision != d.ObservedWorkspaceRevision || w.Revision == ^uint64(0) || d.InputHash != r.InputHash || d.TemplateSnapshotHash != r.TemplateSnapshotHash || o.template == nil || workspace.WorkspaceTemplateSnapshotHash(o.template) != r.TemplateSnapshotHash {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	// 模型先检查 reserved target，再检查诊断容量。
	nextRecord, stored, duplicate, err := r.RecordPreparationDiagnostic(d)
	if err != nil {
		return definitionSpecificationTransition{}, err
	}
	if duplicate {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	next := withAuthoringMutationRecord(w, nextRecord, candidate)
	result := workspaceevents.RecordPreparationDiagnosticResult{WorkspaceID: w.ID, WorkspaceRevision: w.Revision + 1, Diagnostic: workspaceeventmapping.DefinitionAuthoringPreparationDiagnosticMetadataDTO(stored)}
	if result.ValidateFor(cmd) != nil {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	return definitionSpecificationTransition{Next: next, PreparationDiagnostic: &result}, nil
}

func (m *Biz) queryPreparationDiagnostic(ctx context.Context, cmd workspaceevents.QueryPreparationDiagnosticCommand) (workspaceevents.QueryPreparationDiagnosticResult, error) {
	if cmd.Validate() != nil || m == nil || m.manager == nil || m.workspaces == nil || ctx == nil {
		return workspaceevents.QueryPreparationDiagnosticResult{}, common.ErrInvalidArgument
	}
	m.manager.definitionMu.Lock()
	defer m.manager.definitionMu.Unlock()
	m.templateMutationMu.Lock()
	defer m.templateMutationMu.Unlock()
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspaceevents.QueryPreparationDiagnosticResult{}, err
	}
	w, err := m.workspaces.Get(bound, cmd.WorkspaceID)
	if err != nil {
		return workspaceevents.QueryPreparationDiagnosticResult{}, err
	}
	if w.ID != cmd.WorkspaceID {
		return workspaceevents.QueryPreparationDiagnosticResult{}, common.ErrConflict
	}
	if err := workspaceDeletionRejectsMutation(w); err != nil {
		return workspaceevents.QueryPreparationDiagnosticResult{}, err
	}
	d, matched, err := preparationDiagnosticInWorkspace(w, cmd.DiagnosticID)
	if err != nil {
		return workspaceevents.QueryPreparationDiagnosticResult{}, err
	}
	if !matched {
		return workspaceevents.QueryPreparationDiagnosticResult{}, common.ErrNotFound
	}
	result := workspaceevents.QueryPreparationDiagnosticResult{WorkspaceID: w.ID, Diagnostic: d}
	if result.ValidateFor(cmd) != nil {
		return workspaceevents.QueryPreparationDiagnosticResult{}, common.ErrConflict
	}
	return result.DeepClone(), nil
}
