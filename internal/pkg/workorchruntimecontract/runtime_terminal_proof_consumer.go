package workorchruntimecontract

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

func runtimeTerminalReceiptSourceValid(scope, id string) bool {
	switch scope {
	case "goal", "goalCoverage", "dodItem", "acceptanceDimension", "deliverable":
	default:
		return false
	}
	return id != "" && id != "*" && strings.TrimSpace(id) == id && len(id) <= 512 && !strings.ContainsFunc(id, unicode.IsControl)
}

// ValidateRuntimeTerminalProofDeclarationSources rejects unsupported selectors
// before any execution. The selector is a source identity, never a wildcard,
// provider-proposed CWI or best-effort match in a later projection.
func ValidateRuntimeTerminalProofDeclarationSources(items []RuntimeCoverageItem, profiles []RuntimeProfileContract) error {
	for _, profile := range profiles {
		for _, requirement := range profile.EvidenceRequirements {
			for _, rule := range requirement.LineageAssertions {
				if rule.Kind != RuntimeLineageTerminalOwnerReceipt && rule.Kind != RuntimeLineageTerminalResultIndex && rule.Kind != RuntimeLineageTerminalOwnerSnapshot {
					continue
				}
				if err := rule.Validate(); err != nil {
					return err
				}
				matches := 0
				for _, item := range items {
					if (rule.Kind == RuntimeLineageTerminalResultIndex && item.SourceScope == "dodItem") || ((rule.Kind == RuntimeLineageTerminalOwnerReceipt || rule.Kind == RuntimeLineageTerminalOwnerSnapshot) && item.SourceScope == rule.ReceiptSourceScope && item.SourceID == rule.ReceiptSourceID) {
						if item.ID == "" || item.SourceID == "" || strings.TrimSpace(item.TargetText) == "" {
							return fmt.Errorf("terminal proof declaration requires complete original source preimage")
						}
						matches++
					}
				}
				if matches == 0 || (rule.Kind != RuntimeLineageTerminalResultIndex && matches != 1) {
					return fmt.Errorf("terminal proof declaration has missing or ambiguous original source")
				}
				if rule.Kind == RuntimeLineageTerminalOwnerSnapshot {
					count := 0
					for _, item := range items {
						if item.SourceScope == rule.ValidationSourceScope && item.SourceID == rule.ValidationSourceID && item.TargetText != "" {
							count++
						}
					}
					if count != 1 {
						return fmt.Errorf("terminal snapshot requires unique original validation source")
					}
				}
			}
		}
	}
	return nil
}

func runtimeTerminalProofSources(source RuntimeStateRecord) RuntimeLineageAssertionSources {
	return RuntimeLineageAssertionSources{
		Definition: source.Definition, Facts: source.Facts, Work: source.ImplementationWork,
		Semantic: source.SemanticPlanning, Active: source.ActiveDispatch, Completed: source.CompletedDispatches,
		Receipts: source.OperationReceipts, Governance: source.OperationGovernanceBindings,
		Budget: source.ExecutionBudget, EvidenceSequence: source.EvidenceSequence,
		Resolutions: source.GovernanceResolutions, Attempts: source.CandidateAttempts,
		Sessions: source.CandidateRequestSessions, ProcessedEventIDs: source.ProcessedEventIDs,
		ProofLineage: source.CoverageProofLineage, Confirmations: source.RepairSurfaceConfirmations,
		Reconciliation: source.Reconciliation,
	}
}

// ValidateRuntimeTerminalProofMaterial checks only completed-outcome material obligations.
// Failed/cancelled/superseded outcomes must still be delivered faithfully, not upgraded
// or trapped waiting for a success proof. No acceptance credit or terminal is rewritten.
// The caller must independently validate/generate the immutable material and source.
func ValidateRuntimeTerminalProofMaterial(source RuntimeStateRecord, material RuntimeTerminalResultMaterial) error {
	bindings, err := RuntimeDeliveryLineageBindingsFromState(source)
	if err != nil {
		return err
	}
	required := false
	for _, binding := range bindings {
		for _, assertion := range binding.Assertions {
			required = required || assertion.Rule.Kind != RuntimeLineageDeliveryAcknowledgements
		}
	}
	if !required {
		return nil
	}
	if material.RuntimeCloseout == nil {
		return fmt.Errorf("terminal proof requires original structured material")
	}
	outcome, terminal, err := DeriveRuntimeTerminalOutcome(source.Decision, source.Termination)
	if err != nil || !terminal || material.RuntimeCloseout.TerminalOutcome != outcome || material.WorkspaceID != source.Facts.WorkspaceID || material.RunID != source.Facts.RunID || material.RuntimeCloseout.DefinitionRevision != source.Facts.DefinitionRevision {
		return fmt.Errorf("terminal proof material differs from original terminal identity")
	}
	if err := material.ValidateRaw(); err != nil {
		return err
	}
	if material.RuntimeCloseout.TerminalOutcome.Kind != TerminalOutcomeKindCompleted {
		return nil
	}
	for _, binding := range bindings {
		for _, assertion := range binding.Assertions {
			if err := runtimeTerminalMaterialAssertion(source, material, assertion.Rule); err != nil {
				return fmt.Errorf("terminal proof %q CWI %q: %w", assertion.Rule.Ref, binding.CoverageWorkItemID, err)
			}
		}
	}
	return nil
}

func runtimeTerminalMaterialAssertion(source RuntimeStateRecord, material RuntimeTerminalResultMaterial, rule RuntimeLineageAssertionRule) error {
	switch rule.Kind {
	case RuntimeLineageDeliveryAcknowledgements:
		// ACK is consumed separately after actual ordered owner delivery.
		return nil
	case RuntimeLineageTerminalResultIndex:
		return runtimeTerminalResultIndex(source, material)
	case RuntimeLineageTerminalOwnerSnapshot:
		expected, err := CompileRuntimeTerminalSnapshotSources(source)
		if err != nil {
			return err
		}
		matchedSources := 0
		for _, candidate := range expected {
			if candidate.Assertion.Rule != rule {
				continue
			}
			count := 0
			for _, proof := range material.RuntimeCloseout.TerminalSnapshotProofs {
				if candidate == proof.Source {
					if proof.ValidateRaw() != nil {
						return fmt.Errorf("invalid original owner snapshot proof")
					}
					count++
				}
			}
			if count != 1 {
				return fmt.Errorf("missing or ambiguous original owner snapshot proof")
			}
			matchedSources++
		}
		if matchedSources == 0 {
			return fmt.Errorf("missing or ambiguous original owner snapshot proof")
		}
		return nil
	case RuntimeLineageTerminalOwnerReceipt:
		var id common.CoverageWorkItemID
		matches := 0
		for _, item := range source.Facts.DefinitionSeed.CoverageItems {
			if item.SourceScope == rule.ReceiptSourceScope && item.SourceID == rule.ReceiptSourceID {
				id, matches = item.ID, matches+1
			}
		}
		if matches != 1 {
			return fmt.Errorf("terminal receipt selector requires unique original source")
		}
		refs, err := runtimeTerminalAcceptedRefs(material, id)
		if err != nil {
			return err
		}
		sources := runtimeTerminalProofSources(source)
		if !runtimeCanonicalLineageCutoff(source.Facts.EvidenceRevision) || ValidateRuntimeEvidenceLedgerSequence(source.EvidenceSequence, source.Facts, source.ActiveDispatch, source.CompletedDispatches, source.GovernanceResolutions) != nil || runtimeValidateLineageOperations(sources) != nil {
			return fmt.Errorf("terminal receipt requires original operation/evidence chronology")
		}
		for _, dispatch := range source.CompletedDispatches {
			if dispatch.State != RuntimeDispatchStateCommitted || dispatch.ImplementationWorkCommitment != nil {
				continue
			}
			for _, operation := range runtimeCandidateAdmissionOperations(dispatch.Candidate) {
				if rule.ReceiptChoiceContractRef == "" && operation.Kind != rule.OperationKind {
					continue
				}
				contract, found := RuntimeOperationContractForKind(operation.Kind)
				if !found {
					continue
				}
				variantID := ""
				if len(contract.Variants) != 0 {
					variant, err := MatchRuntimeOperationVariant(operation, contract)
					if err != nil {
						continue
					}
					variantID = variant.ID
				}
				fact, receipt, valid := runtimeLineageOperationFact(dispatch, operation, sources, true)
				if !runtimeTerminalReceiptOperationMatches(rule, operation.Kind, variantID) || !valid || receipt.ObservedEffectScope != rule.RequiredEffectScope || !slices.Contains(refs, string(fact.ID)) || !slices.Contains(fact.CoverageWorkItemIDs, id) || EvidenceRevisionAfter(fact.Revision, source.Facts.EvidenceRevision) {
					continue
				}
				for _, binding := range dispatch.CoverageBindings {
					if binding.CoverageWorkItemID == id && slices.Contains(operation.BindingRefs, binding.ID) && slices.Contains(fact.BindingRefs, binding.ID) && slices.Contains(material.EvidenceRefs, string(fact.ID)) && slices.Contains(material.EvidenceRefs, fact.SourceRef) {
						return nil
					}
				}
			}
		}
		return fmt.Errorf("original accepted owner receipt is absent from selected terminal requirement")
	default:
		return fmt.Errorf("unsupported terminal material consumer")
	}
}

func runtimeTerminalAcceptedRefs(material RuntimeTerminalResultMaterial, id common.CoverageWorkItemID) ([]string, error) {
	var refs []string
	matches := 0
	for _, outcome := range material.RuntimeCloseout.RequirementOutcomes {
		if outcome.CoverageWorkItemID != id {
			continue
		}
		matches++
		if outcome.CoverageState != RuntimeRequirementCovered || outcome.AssessmentState != RuntimeRequirementSatisfied || len(outcome.AcceptedEvidenceRefs) == 0 {
			return nil, fmt.Errorf("terminal requirement has no current accepted proof")
		}
		refs = outcome.AcceptedEvidenceRefs
	}
	if matches != 1 {
		return nil, fmt.Errorf("terminal requirement index is missing or ambiguous")
	}
	return refs, nil
}

// runtimeTerminalResultIndex independently checks complete original DoD/source
// inventory against formal per-item refs, current proof and the top-level index.
// It does not assert owner-currentness of the historical dependency snapshots.
func runtimeTerminalResultIndex(source RuntimeStateRecord, material RuntimeTerminalResultMaterial) error {
	count := 0
	for _, item := range source.Facts.DefinitionSeed.CoverageItems {
		if item.SourceScope != "dodItem" {
			continue
		}
		count++
		refs, err := runtimeTerminalAcceptedRefs(material, item.ID)
		if err != nil {
			return err
		}
		var coverage RuntimeCoverageItemFact
		matches := 0
		for _, candidate := range source.Facts.Coverage {
			if candidate.ID == item.ID && candidate.SourceScope == item.SourceScope && candidate.SourceID == item.SourceID && candidate.TargetText == item.TargetText {
				coverage, matches = candidate, matches+1
			}
		}
		binding, found := runtimeTerminalCurrentCoverageBinding(source.Facts, coverage)
		if matches != 1 || !found {
			return fmt.Errorf("Result index requires original current DoD binding")
		}
		for _, ref := range refs {
			if !slices.Contains(material.EvidenceRefs, ref) || !slices.Contains(material.RuntimeCloseout.AcceptedEvidenceRefs, ref) {
				return fmt.Errorf("Result item evidence is absent from the formal index")
			}
			facts := 0
			for _, fact := range source.Facts.Evidence.Accepted {
				if string(fact.ID) == ref && runtimeTerminalEvidenceMatchesCoverageProof(source, fact, coverage, binding) {
					facts++
				}
			}
			if facts != 1 {
				return fmt.Errorf("Result item reference does not resolve to unique current accepted proof")
			}
		}
		note := item.SourceScope + "/" + item.SourceID
		if !runtimeTerminalAssessmentIndexRow(material.DoDAssessment.Items, note, item.TargetText, refs) || !runtimeTerminalAssessmentIndexRow(material.SummaryDetail.DoDItems, note, item.TargetText, refs) {
			return fmt.Errorf("formal Result DoD item omits original text or evidence refs")
		}
		for _, check := range item.SourceObligationChecks {
			note := runtimeTerminalSourceObligationNote(source.Facts.DefinitionSeed.SourceObligations, check)
			var sourceRefs []string
			matches := 0
			for _, outcome := range material.RuntimeCloseout.RequirementOutcomes {
				if outcome.CoverageWorkItemID != item.ID {
					continue
				}
				for _, value := range outcome.SourceObligations {
					if value.Check == check && value.CoverageState == RuntimeRequirementCovered && value.AssessmentState == RuntimeRequirementSatisfied {
						sourceRefs, matches = value.AcceptedEvidenceRefs, matches+1
					}
				}
			}
			if matches != 1 || len(sourceRefs) == 0 || !runtimeTerminalAssessmentIndexRow(material.DoDAssessment.Items, note, check.Criterion, sourceRefs) || !runtimeTerminalAssessmentIndexRow(material.SummaryDetail.DoDItems, note, check.Criterion, sourceRefs) {
				return fmt.Errorf("formal Result omits original source obligation index")
			}
			for _, ref := range sourceRefs {
				if !slices.Contains(refs, ref) {
					return fmt.Errorf("source obligation evidence escapes original DoD proof")
				}
			}
		}
	}
	if count == 0 {
		return fmt.Errorf("Result index consumer requires complete original DoD inventory")
	}
	return nil
}

func runtimeTerminalAssessmentIndexRow(rows []workorchresultcontract.DoDAssessmentItem, note, text string, refs []string) bool {
	matches := 0
	for _, row := range rows {
		if row.SourceNote == note {
			if row.Item != text || row.State != string(RuntimeRequirementSatisfied) || !slices.Equal(row.EvidenceRefs, refs) {
				return false
			}
			matches++
		}
	}
	return matches == 1
}
