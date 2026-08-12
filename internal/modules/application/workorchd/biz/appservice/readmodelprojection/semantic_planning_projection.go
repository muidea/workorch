package readmodelprojection

import (
	"sort"
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
	proposalsByCoverage := make(map[string][]runtimecontract.RuntimeSemanticProofProposal)
	for _, proposal := range semanticPlanningProjectionProposals(ledger) {
		proposal = proposal.Clone()
		proposalsByCoverage[string(proposal.CoverageWorkItemID)] = append(proposalsByCoverage[string(proposal.CoverageWorkItemID)], proposal)
	}
	observationsByCoverage := make(map[string][]runtimecontract.RuntimeSemanticProofFeasibility)
	for _, observation := range ledger.Feasibilities {
		observation = observation.Clone()
		observationsByCoverage[string(observation.CoverageWorkItemID)] = append(observationsByCoverage[string(observation.CoverageWorkItemID)], observation)
	}
	for _, coverage := range ledger.DefinitionSeed.CoverageItems {
		key := string(coverage.ID)
		ret.Items = append(ret.Items, semanticPlanningItem(key, ledger.Phase, proposalsByCoverage[key], observationsByCoverage[key]))
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
		item.Items = semanticPlanningGapItems(gap)
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

func semanticPlanningProjectionProposals(ledger runtimecontract.RuntimeSemanticPlanningLedger) []runtimecontract.RuntimeSemanticProofProposal {
	if ledger.MergedDocument != nil {
		return append([]runtimecontract.RuntimeSemanticProofProposal(nil), ledger.MergedDocument.Clone().Proposals...)
	}
	if ledger.ActiveBatchID != "" {
		for _, document := range ledger.BatchDocuments {
			if document.BatchID == ledger.ActiveBatchID {
				return append([]runtimecontract.RuntimeSemanticProofProposal(nil), document.Clone().Proposals...)
			}
		}
	}
	if ledger.Document != nil && ledger.Document.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch {
		return append([]runtimecontract.RuntimeSemanticProofProposal(nil), ledger.Document.Clone().Proposals...)
	}
	return nil
}

func semanticPlanningDisplayState(phase runtimecontract.RuntimeSemanticPlanningPhase) string {
	switch phase {
	case runtimecontract.RuntimeSemanticPlanningRequired:
		return "semantic_contract_required"
	case runtimecontract.RuntimeSemanticPlanningRequestOpen, runtimecontract.RuntimeSemanticPlanningCorrectionRequired:
		return "planning"
	case runtimecontract.RuntimeSemanticPlanningOrderAccepted,
		runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen,
		runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted,
		runtimecontract.RuntimeSemanticPlanningProofBatchFeasibilityPending,
		runtimecontract.RuntimeSemanticPlanningProofBatchFeasibilityComplete,
		runtimecontract.RuntimeSemanticPlanningFeasibilityRequired:
		return "feasibility"
	case runtimecontract.RuntimeSemanticPlanningSealed:
		return "sealed"
	case runtimecontract.RuntimeSemanticPlanningFailed:
		return "rejected"
	default:
		return strings.TrimSpace(string(phase))
	}
}

func semanticPlanningItem(
	coverageID string,
	phase runtimecontract.RuntimeSemanticPlanningPhase,
	proposals []runtimecontract.RuntimeSemanticProofProposal,
	observations []runtimecontract.RuntimeSemanticProofFeasibility,
) projectioncontract.RuntimeSemanticPlanningItem {
	observationsByProposal := make(map[string]runtimecontract.RuntimeSemanticProofFeasibility, len(observations))
	for _, observation := range observations {
		if observation.ProposalHash != "" {
			observationsByProposal[observation.ProposalHash] = observation.Clone()
		}
	}
	ret := projectioncontract.RuntimeSemanticPlanningItem{CoverageWorkItemID: coverageID}
	for _, proposal := range proposals {
		proposal = proposal.Clone()
		proposalHash := runtimecontract.StableRuntimeSemanticProofProposalHash(proposal)
		observation, found := observationsByProposal[proposalHash]
		if found {
			delete(observationsByProposal, proposalHash)
		}
		ret.Proofs = append(ret.Proofs, semanticPlanningProof(phase, proposal, observation))
	}
	for _, observation := range observations {
		if observation.ProposalHash != "" {
			if _, found := observationsByProposal[observation.ProposalHash]; !found {
				continue
			}
			delete(observationsByProposal, observation.ProposalHash)
		}
		ret.Proofs = append(ret.Proofs, semanticPlanningProof(phase, runtimecontract.RuntimeSemanticProofProposal{}, observation))
	}
	sort.SliceStable(ret.Proofs, func(i, j int) bool {
		if ret.Proofs[i].EvidenceKind != ret.Proofs[j].EvidenceKind {
			return ret.Proofs[i].EvidenceKind < ret.Proofs[j].EvidenceKind
		}
		if ret.Proofs[i].ProposalHash != ret.Proofs[j].ProposalHash {
			return ret.Proofs[i].ProposalHash < ret.Proofs[j].ProposalHash
		}
		return ret.Proofs[i].OwnerModuleID < ret.Proofs[j].OwnerModuleID
	})
	ret.Status = semanticPlanningCWIStatus(phase, ret.Proofs)
	return ret
}

func semanticPlanningGapItems(gap runtimecontract.RuntimeSemanticFeasibilityGap) []projectioncontract.RuntimeSemanticPlanningItem {
	byCoverage := make(map[string][]runtimecontract.RuntimeSemanticProofFeasibility, len(gap.Feasibilities))
	for _, observation := range gap.Feasibilities {
		observation = observation.Clone()
		byCoverage[string(observation.CoverageWorkItemID)] = append(byCoverage[string(observation.CoverageWorkItemID)], observation)
	}
	coverageIDs := make([]string, 0, len(byCoverage))
	for coverageID := range byCoverage {
		coverageIDs = append(coverageIDs, coverageID)
	}
	sort.Strings(coverageIDs)
	items := make([]projectioncontract.RuntimeSemanticPlanningItem, 0, len(coverageIDs))
	for _, coverageID := range coverageIDs {
		items = append(items, semanticPlanningItem(coverageID, runtimecontract.RuntimeSemanticPlanningFeasibilityRequired, nil, byCoverage[coverageID]))
	}
	return items
}

func semanticPlanningProof(
	phase runtimecontract.RuntimeSemanticPlanningPhase,
	proposal runtimecontract.RuntimeSemanticProofProposal,
	observation runtimecontract.RuntimeSemanticProofFeasibility,
) projectioncontract.RuntimeSemanticPlanningProof {
	proposal = proposal.Normalized()
	observation = observation.Clone()
	proof := projectioncontract.RuntimeSemanticPlanningProof{
		ProposalHash: observation.ProposalHash, OwnerModuleID: proposal.OwnerModuleID, OperationKind: proposal.OperationKind,
		TargetRefs: semanticPlanningProposalTargetRefs(proposal), ValidationRecipeID: proposal.ValidationRecipeID,
		RequiredCheckRefs: append([]string(nil), proposal.RequiredCheckRefs...), ObservedRevision: observation.ObservedRevision,
	}
	if len(proposal.RequiredEvidenceKinds) == 1 {
		proof.EvidenceKind = proposal.RequiredEvidenceKinds[0]
	}
	if proof.ProposalHash == "" && proposal.CoverageWorkItemID != "" {
		proof.ProposalHash = runtimecontract.StableRuntimeSemanticProofProposalHash(proposal)
	}
	if observation.OwnerModuleID != "" {
		proof.OwnerModuleID = observation.OwnerModuleID
	}
	if len(observation.TargetRefs) != 0 {
		proof.TargetRefs = append([]string(nil), observation.TargetRefs...)
	}
	if observation.ValidationRecipeID != "" {
		proof.ValidationRecipeID = observation.ValidationRecipeID
	}
	if len(observation.RequiredCheckRefs) != 0 {
		proof.RequiredCheckRefs = append([]string(nil), observation.RequiredCheckRefs...)
	}
	if observation.OriginalDiagnostic != nil {
		value := semanticDiagnosticProjection(*observation.OriginalDiagnostic)
		proof.Diagnostic = &value
	}
	proof.Status = semanticPlanningProofStatus(phase, proposal, observation)
	return proof
}

func semanticPlanningProposalTargetRefs(proposal runtimecontract.RuntimeSemanticProofProposal) []string {
	target := proposal.Target.Normalized()
	switch target.Policy {
	case runtimecontract.RuntimeOperationTargetPolicyExactTarget:
		return append([]string(nil), target.ExactTargetRefs...)
	case runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope:
		return append([]string(nil), target.OwnerScopeTargetRefs...)
	case runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		if target.ProspectiveTargetRef != "" {
			return []string{target.ProspectiveTargetRef}
		}
	}
	return nil
}

func semanticPlanningProofStatus(
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

func semanticPlanningCWIStatus(phase runtimecontract.RuntimeSemanticPlanningPhase, proofs []projectioncontract.RuntimeSemanticPlanningProof) string {
	if len(proofs) == 0 {
		return semanticPlanningProofStatus(phase, runtimecontract.RuntimeSemanticProofProposal{}, runtimecontract.RuntimeSemanticProofFeasibility{})
	}
	if phase == runtimecontract.RuntimeSemanticPlanningFailed {
		return "rejected"
	}
	if phase == runtimecontract.RuntimeSemanticPlanningSealed {
		return "sealed"
	}
	status := proofs[0].Status
	for _, proof := range proofs[1:] {
		if proof.Status != status {
			return "partial_feasibility"
		}
	}
	return status
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
