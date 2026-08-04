package readmodelprojection

import (
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func runtimeSemanticPlanningProjection(state runtimecontract.RuntimeStateRecord) *projectioncontract.RuntimeSemanticPlanning {
	ledger := state.SemanticPlanning.Clone()
	if ledger.SchemaVersion == "" {
		return nil
	}
	ret := &projectioncontract.RuntimeSemanticPlanning{
		WorkspaceID: string(state.Facts.WorkspaceID), RunID: string(state.Facts.RunID),
		DefinitionRevision: state.Facts.DefinitionRevision, StateRevision: state.StateRevision,
		State: semanticPlanningDisplayState(ledger.Phase), Phase: string(ledger.Phase),
		Attempt: ledger.Attempt, CatalogRefreshAttempts: ledger.CatalogRefreshAttempts,
		LogicalRequestID: ledger.LogicalRequestID, DocumentHash: ledger.DocumentHash,
		FeasibilityBatchID: ledger.FeasibilityBatchID, FeasibilitySetHash: ledger.FeasibilitySetHash,
		SealedBindingSetHash: ledger.SealedBindingSetHash,
	}
	if ledger.Manifest != nil {
		ret.ManifestHash = ledger.Manifest.Hash
	}
	proposals := make(map[string]runtimecontract.RuntimeSemanticProofProposal)
	if ledger.Document != nil {
		for _, proposal := range ledger.Document.Proposals {
			proposals[string(proposal.CoverageWorkItemID)] = proposal
		}
	}
	observations := make(map[string]runtimecontract.RuntimeSemanticProofFeasibility)
	for _, observation := range ledger.Feasibilities {
		observations[string(observation.CoverageWorkItemID)] = observation
	}
	for _, coverage := range ledger.DefinitionSeed.CoverageItems {
		key := string(coverage.ID)
		ret.Items = append(ret.Items, semanticPlanningItem(
			key,
			proposals[key],
			observations[key],
			semanticPlanningItemStatus(ledger.Phase, proposals[key], observations[key]),
		))
	}
	for _, rejection := range ledger.Rejections {
		ret.Rejections = append(ret.Rejections, semanticDiagnosticProjection(rejection))
	}
	for _, gap := range ledger.FeasibilityGaps {
		item := projectioncontract.RuntimeSemanticFeasibilityGap{
			RefreshGeneration: gap.RefreshGeneration, ManifestHash: gap.ManifestHash,
			DocumentHash: gap.DocumentHash, FeasibilityBatchID: gap.FeasibilityBatchID,
			FeasibilitySetHash: gap.FeasibilitySetHash,
		}
		for _, observation := range gap.Feasibilities {
			key := string(observation.CoverageWorkItemID)
			item.Items = append(item.Items, semanticPlanningItem(key, proposals[key], observation, string(observation.Status)))
		}
		for _, rejection := range gap.Rejections {
			item.Rejections = append(item.Rejections, semanticDiagnosticProjection(rejection))
		}
		ret.FeasibilityGaps = append(ret.FeasibilityGaps, item)
	}
	if ledger.Failure != nil {
		value := semanticDiagnosticProjection(*ledger.Failure)
		ret.Failure = &value
	}
	return ret
}

func semanticPlanningDisplayState(phase runtimecontract.RuntimeSemanticPlanningPhase) string {
	switch phase {
	case runtimecontract.RuntimeSemanticPlanningRequired:
		return "semantic_contract_required"
	case runtimecontract.RuntimeSemanticPlanningRequestOpen, runtimecontract.RuntimeSemanticPlanningCorrectionRequired:
		return "planning"
	case runtimecontract.RuntimeSemanticPlanningProposalAccepted, runtimecontract.RuntimeSemanticPlanningFeasibilityRequired:
		return "feasibility"
	case runtimecontract.RuntimeSemanticPlanningSealed:
		return "sealed"
	case runtimecontract.RuntimeSemanticPlanningFailed:
		return "rejected"
	default:
		return strings.TrimSpace(string(phase))
	}
}

func semanticPlanningItemStatus(
	phase runtimecontract.RuntimeSemanticPlanningPhase,
	proposal runtimecontract.RuntimeSemanticProofProposal,
	observation runtimecontract.RuntimeSemanticProofFeasibility,
) string {
	if observation.Status != "" {
		return string(observation.Status)
	}
	if phase == runtimecontract.RuntimeSemanticPlanningFailed {
		return "rejected"
	}
	if phase == runtimecontract.RuntimeSemanticPlanningSealed {
		return "sealed"
	}
	if proposal.CoverageWorkItemID != "" {
		if phase == runtimecontract.RuntimeSemanticPlanningFeasibilityRequired {
			return "awaiting_feasibility"
		}
		return "proposed"
	}
	return "pending"
}

func semanticPlanningItem(
	coverageID string,
	proposal runtimecontract.RuntimeSemanticProofProposal,
	observation runtimecontract.RuntimeSemanticProofFeasibility,
	status string,
) projectioncontract.RuntimeSemanticPlanningItem {
	ret := projectioncontract.RuntimeSemanticPlanningItem{
		CoverageWorkItemID: coverageID, Status: status,
		ProposalHash: observation.ProposalHash, OwnerModuleID: proposal.OwnerModuleID,
		OperationKind: proposal.OperationKind, TargetRefs: append([]string(nil), proposal.Target.ExactTargetRefs...),
		ValidationRecipeID: proposal.ValidationRecipeID, RequiredCheckRefs: append([]string(nil), proposal.RequiredCheckRefs...),
		ObservedRevision: observation.ObservedRevision,
	}
	if ret.ProposalHash == "" && proposal.CoverageWorkItemID != "" {
		ret.ProposalHash = runtimecontract.StableRuntimeSemanticProofProposalHash(proposal)
	}
	if observation.OwnerModuleID != "" {
		ret.OwnerModuleID = observation.OwnerModuleID
	}
	if len(observation.TargetRefs) != 0 {
		ret.TargetRefs = append([]string(nil), observation.TargetRefs...)
	}
	if observation.ValidationRecipeID != "" {
		ret.ValidationRecipeID = observation.ValidationRecipeID
	}
	if len(observation.RequiredCheckRefs) != 0 {
		ret.RequiredCheckRefs = append([]string(nil), observation.RequiredCheckRefs...)
	}
	if observation.OriginalDiagnostic != nil {
		value := semanticDiagnosticProjection(*observation.OriginalDiagnostic)
		ret.Diagnostic = &value
	}
	return ret
}

func semanticDiagnosticProjection(value runtimecontract.RuntimeOriginalDiagnostic) projectioncontract.RuntimeSemanticDiagnostic {
	ret := projectioncontract.RuntimeSemanticDiagnostic{
		SchemaVersion: value.SchemaVersion, OwnerModuleID: value.OwnerModuleID,
		OriginalClass: value.OriginalClass, OriginalCode: value.OriginalCode,
		OriginalMessage: value.OriginalMessage, Signature: value.Signature,
		OperationID: value.OperationID, TargetRefs: append([]string(nil), value.TargetRefs...),
		ExpectedRevision: value.ExpectedRevision, ObservedRevision: value.ObservedRevision,
		DetailsRef: value.DetailsRef, DetailsHash: value.DetailsHash, DetailsBytes: value.DetailsBytes,
		ContentType: value.ContentType, OmittedReason: value.OmittedReason,
		Truncated: value.Truncated, Redacted: value.Redacted,
	}
	ret.FieldViolations = runtimecontract.CloneRuntimeFieldViolations(value.FieldViolations)
	return ret
}
