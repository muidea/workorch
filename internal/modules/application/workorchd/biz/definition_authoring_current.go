package biz

import (
	"context"

	events "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Refresh only unconfirmed machine input using the normal Workspace CAS command.
// Query stays read-only. Confirmed documents, in-flight request sources and
// exact user confirmation are never replaced by this operation.
func (b *Biz) currentDefinitionAuthoring(ctx context.Context, id common.WorkspaceID, inputHash string) (events.QueryDefinitionAuthoringResult, error) {
	view, err := b.QueryDefinitionAuthoring(ctx, events.QueryDefinitionAuthoringCommand{WorkspaceID: id, InputHash: inputHash})
	if err != nil {
		return view, err
	}
	if inputHash != "" || view.Record == nil || view.Confirmation != nil || view.ActivationAdmission != nil || !view.CurrentTemplateAvailable || view.Record.TemplateSnapshotHash == view.CurrentTemplateSnapshotHash {
		return view, nil
	}
	record, err := view.Record.Model()
	if err != nil {
		return events.QueryDefinitionAuthoringResult{}, err
	}
	// Accepted derivation and its subsequent mapping/reviews belong to one exact
	// source. A later deployment cannot silently replace that source or resample
	// a completed candidate. A fresh derivation selects current declarations.
	for _, r := range record.DerivationRequests {
		if r.FailureCode == "" && r.Input.InputHash() == view.Record.InputHash && workspace.WorkspaceTemplateSnapshotHash(&r.Template) == view.Record.TemplateSnapshotHash {
			return view, nil
		}
	}
	for _, r := range record.SpecificationRequests {
		if r.Proposal == nil && r.Review == nil && r.FailureCode == "" {
			return view, nil
		}
	}
	_, err = b.SaveDefinitionAuthoring(ctx, events.SaveDefinitionAuthoringCommand{RequestID: "builtin-refresh/" + workspace.DefinitionDerivationRequestID(view.Record.InputHash, view.CurrentTemplateSnapshotHash), ExpectedInputHash: view.Record.InputHash, ExpectedWorkspaceRevision: view.WorkspaceRevision, Input: view.Record.Input.Clone()})
	if err != nil {
		return events.QueryDefinitionAuthoringResult{}, err
	}
	return b.QueryDefinitionAuthoring(ctx, events.QueryDefinitionAuthoringCommand{WorkspaceID: id})
}
