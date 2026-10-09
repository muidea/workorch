package eventmapping

import (
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func DefinitionDerivationRequestDTO(r workspace.DefinitionDerivationRequest) workspaceevents.DefinitionDerivationRequest {
	r = r.Clone()
	if r.Diagnostic != nil {
		d := r.Diagnostic.Metadata()
		r.Diagnostic = &d
	}
	return workspaceevents.DefinitionDerivationRequest{
		RequestID: r.RequestID, Input: r.Input, Template: *TemplateDTO(&r.Template),
		Resources: r.Resources, ResourceSnapshotHash: r.ResourceSnapshotHash,
		ProviderBindingHash: r.ProviderBindingHash, PermitID: r.PermitID,
		Proposal: r.Proposal, FailureCode: r.FailureCode, Diagnostic: r.Diagnostic,
	}
}

func DefinitionAuthoringRecordDTO(r workspace.DefinitionAuthoringRecord) workspaceevents.DefinitionAuthoringRecord {
	r = r.Clone()
	ret := workspaceevents.DefinitionAuthoringRecord{
		Input: r.Input, InputHash: r.InputHash, TemplateSnapshotHash: r.TemplateSnapshotHash,
		TemplateSnapshot: *TemplateDTO(&r.TemplateSnapshot), SpecificationRequests: r.SpecificationRequests,
		SaveReceipts: r.SaveReceipts,
	}
	for n := range ret.SpecificationRequests {
		ret.SpecificationRequests[n] = ret.SpecificationRequests[n].Metadata()
	}
	if r.DerivationRequests != nil {
		ret.DerivationRequests = make([]workspaceevents.DefinitionDerivationRequest, len(r.DerivationRequests))
	}
	for n, request := range r.DerivationRequests {
		ret.DerivationRequests[n] = DefinitionDerivationRequestDTO(request)
	}
	if r.PreparationDiagnostics != nil {
		ret.PreparationDiagnostics = make([]workspaceevents.DefinitionAuthoringPreparationDiagnosticMetadata, len(r.PreparationDiagnostics))
	}
	for n, diagnostic := range r.PreparationDiagnostics {
		ret.PreparationDiagnostics[n] = DefinitionAuthoringPreparationDiagnosticMetadataDTO(diagnostic)
	}
	return ret
}

func DefinitionAuthoringPreparationDiagnosticMetadataDTO(d workspace.DefinitionAuthoringPreparationDiagnostic) workspaceevents.DefinitionAuthoringPreparationDiagnosticMetadata {
	d = d.Metadata()
	return workspaceevents.DefinitionAuthoringPreparationDiagnosticMetadata{
		DiagnosticID: d.DiagnosticID, WorkspaceID: d.WorkspaceID, InputHash: d.InputHash, TemplateSnapshotHash: d.TemplateSnapshotHash,
		ObservedWorkspaceRevision: d.ObservedWorkspaceRevision, Stage: d.Stage, TargetRequestID: d.TargetRequestID,
		DerivationPermitID: d.DerivationPermitID, ReviewKind: d.ReviewKind, ProposalHash: d.ProposalHash,
		PhaseCode: d.PhaseCode, ResourceRef: d.ResourceRef, ProviderBindingHash: d.ProviderBindingHash,
		Cause: d.Cause, CauseSHA256: d.CauseSHA256, CauseBytes: d.CauseBytes, CauseSaved: d.CauseSaved, NotSavedReason: d.NotSavedReason,
	}
}

func DefinitionAuthoringHistoryDTOs(history []workspace.DefinitionAuthoringConfirmedRevision) []workspaceevents.DefinitionAuthoringConfirmedRevision {
	if history == nil {
		return nil
	}
	ret := make([]workspaceevents.DefinitionAuthoringConfirmedRevision, len(history))
	for n, revision := range history {
		ret[n] = workspaceevents.DefinitionAuthoringConfirmedRevision{Record: DefinitionAuthoringRecordDTO(revision.Record), Confirmation: revision.Confirmation.Clone()}
	}
	return ret
}
