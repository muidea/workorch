package runtime

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The report lives in the existing persisted semantic batch, not another
// aggregate or state machine. Only the SM-2 feasibility-required event can
// classify it, before any capability probe or binding seal.
func runtimeInputReviewDocument(ledger runtimecontract.RuntimeSemanticPlanningLedger) (runtimecontract.RuntimeSemanticPlanDocument, bool) {
	if ledger.Manifest == nil || ledger.ActiveBatchID == "" {
		return runtimecontract.RuntimeSemanticPlanDocument{}, false
	}
	return runtimeSemanticBatchDocument(ledger.BatchDocuments, ledger.ActiveBatchID)
}

func runtimeInputReviewRef(a RuntimeAggregate, ledger runtimecontract.RuntimeSemanticPlanningLedger, d runtimecontract.RuntimeSemanticPlanDocument) string {
	identity := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s", a.Facts.WorkspaceID, a.Facts.RunID,
		a.Definition.Revision, a.Definition.StartupInputHash, ledger.Manifest.Hash, ledger.LogicalRequestID, runtimecontract.StableRuntimeSemanticPlanDocumentHash(d))
	return fmt.Sprintf("runtime-input-review-%x", sha256.Sum256([]byte(identity)))
}

func runtimeInputReviewConclusion(d runtimecontract.RuntimeSemanticPlanDocument) string {
	conclusion := ""
	for _, proposal := range d.Proposals {
		if proposal.ValidationInputReview == nil {
			continue
		}
		value := runtimecontract.RuntimeValidationInputReviewConclusion(*proposal.ValidationInputReview)
		if value == runtimecontract.RuntimeValidationInputConflict {
			return value
		}
		if conclusion == "" || value == runtimecontract.RuntimeValidationInputInconclusive {
			conclusion = value
		}
	}
	return conclusion
}

func runtimeInputReviewDependency(a RuntimeAggregate, ledger runtimecontract.RuntimeSemanticPlanningLedger, d runtimecontract.RuntimeSemanticPlanDocument) runtimecontract.ExternalDependency {
	var question strings.Builder
	question.WriteString("Provide factual information for a new validation-input consistency review. Original Goal/DoD remain unchanged; this ticket cannot approve a waiver or grant validation credit.")
	for _, proposal := range d.Proposals {
		if proposal.ValidationInputReview == nil {
			continue
		}
		for _, check := range proposal.ValidationInputReview.Checks {
			if check.Conclusion == runtimecontract.RuntimeValidationInputInconclusive {
				fmt.Fprintf(&question, "\nCWI %s; original check %s; input %s; question: %s", proposal.CoverageWorkItemID, check.RequiredCheckRef, strings.Join(check.InputTargetRefs, ", "), check.Reason)
			}
		}
	}
	return runtimecontract.ExternalDependency{
		Kind: string(runtimecontract.GovernanceDecision), SubjectRef: runtimeInputReviewRef(a, ledger, d),
		RequestedAction: runtimecontract.RuntimeValidationInputReviewInformationAction, Reason: question.String(),
	}
}

func runtimeInputReviewInformationFailure(historyCount int, information []runtimecontract.RuntimeValidationInputReviewInformation, document runtimecontract.RuntimeSemanticPlanDocument) string {
	for _, proposal := range document.Proposals {
		if proposal.ValidationInputReview == nil || runtimecontract.RuntimeValidationInputReviewConclusion(*proposal.ValidationInputReview) != runtimecontract.RuntimeValidationInputInconclusive {
			continue
		}
		for _, source := range information {
			if source.SubjectHash == proposal.ValidationInputReview.SubjectHash {
				return "validation_input_review_information_exhausted"
			}
		}
	}
	// Do not open a ticket whose answer cannot be consumed within the
	// existing generation budget; the capacity guard precedes the write.
	if historyCount >= runtimecontract.RuntimeSemanticPlanningHistoryLimit {
		return "validation_input_review_history_exhausted"
	}
	return ""
}

func classifyRuntimeInputReview(transaction *semanticPlanningMutationTransaction) (handled, changed bool, err error) {
	ledger := transaction.ledger
	if ledger.Phase != runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted {
		return false, false, nil
	}
	document, found := runtimeInputReviewDocument(ledger)
	if !found {
		return false, false, nil
	}
	if err := document.ValidateAgainstManifest(*ledger.Manifest); err != nil {
		return true, false, err
	}
	conclusion := runtimeInputReviewConclusion(document)
	if conclusion == "" {
		return false, false, nil
	}
	// A direct private-event injection cannot turn a guessed report into a
	// business clarification. It must come from this exact completed request.
	matched := false
	for _, session := range transaction.aggregate.CandidateRequestSessions {
		if session.LogicalRequestID == ledger.LogicalRequestID && session.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan &&
			session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete && session.ExpectedDocument != nil &&
			session.ManifestHash == ledger.Manifest.Hash && session.StablePrefixHash == ledger.StablePrefixHash &&
			session.ExpectedDocument.LogicalRequestID == ledger.LogicalRequestID && session.ExpectedDocument.ManifestHash == ledger.Manifest.Hash &&
			session.ExpectedDocument.DefinitionSeedHash == document.DefinitionSeedHash && session.ExpectedDocument.PlanStep == document.PlanStep &&
			session.ExpectedDocument.BatchID == document.BatchID && session.ExpectedDocument.BatchSequence == document.BatchSequence &&
			reflect.DeepEqual(session.ExpectedDocument.BatchCoverageWorkItemIDs, document.BatchCoverageWorkItemIDs) {
			matched = true
		}
	}
	if !matched {
		return true, false, fmt.Errorf("%w: input review has no exact complete request", common.ErrConflict)
	}
	if conclusion == runtimecontract.RuntimeValidationInputConsistent {
		return false, false, nil
	}
	a := transaction.aggregate
	a.SemanticPlanning = ledger
	code := "validation_input_review_conflict"
	if conclusion == runtimecontract.RuntimeValidationInputInconclusive {
		code = runtimeInputReviewInformationFailure(len(ledger.History), ledger.Manifest.InputReviewInformation, document)
		if code == "" {
			dependency := runtimeInputReviewDependency(a, ledger, document)
			if a.Facts.Failure != nil && a.Facts.Failure.GovernanceDependency != nil && runtimecontract.SameExternalDependency(*a.Facts.Failure.GovernanceDependency, dependency) {
				return true, false, nil
			}
			failure := runtimecontract.FailureFact{Class: runtimecontract.FailureClassGovernanceDependency, Code: "validation_input_review_inconclusive", Signature: dependency.SubjectRef,
				Message: "complete validation input review requires factual clarification", GovernanceDependency: &dependency}
			if err := replaceRuntimeFailureAuthority(&a.Facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
				return true, false, err
			}
			facts, decision, err := reduceRuntimeAggregateWithDirective(a, a.Facts, &a.Decision.AuthoritySnapshot)
			if err != nil {
				return true, false, err
			}
			a.Facts, a.Decision = facts, decision
			transaction.aggregate = a
			return true, true, nil
		}
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator, OriginalClass: "contract",
		OriginalCode: code, OriginalMessage: "validation input review did not establish consistency with the immutable original criteria", Signature: runtimeInputReviewRef(a, ledger, document)}
	updated, err := applySemanticContractFailureToAggregate(a, code, diagnostic)
	if err != nil {
		return true, false, err
	}
	transaction.aggregate, transaction.ledger = updated, updated.SemanticPlanning
	return true, true, nil
}

func runtimeInputReviewResolutionMatches(a RuntimeAggregate, ledger runtimecontract.RuntimeSemanticPlanningLedger, resolution runtimecontract.RuntimeGovernanceResolution) bool {
	document, found := runtimeInputReviewDocument(ledger)
	return found && runtimeInputReviewConclusion(document) == runtimecontract.RuntimeValidationInputInconclusive &&
		resolution.TicketID != "" && resolution.ApprovalID == "" && resolution.DependencyKind == string(runtimecontract.GovernanceDecision) &&
		resolution.SubjectRef == runtimeInputReviewRef(a, ledger, document)
}

func retireRuntimeInputReviewForInformation(a *RuntimeAggregate, resolution runtimecontract.RuntimeGovernanceResolution) error {
	ledger := a.SemanticPlanning.Clone()
	if !runtimeInputReviewResolutionMatches(*a, ledger, resolution) || resolution.Outcome != runtimecontract.RuntimeGovernanceResolutionOutcomeContinue ||
		ledger.Phase != runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted || a.ActiveDispatch != nil || len(ledger.History) >= runtimecontract.RuntimeSemanticPlanningHistoryLimit {
		return fmt.Errorf("%w: input review information cannot reopen this semantic transaction", common.ErrConflict)
	}
	document, _ := runtimeInputReviewDocument(ledger)
	for _, proposal := range document.Proposals {
		if proposal.ValidationInputReview == nil || runtimecontract.RuntimeValidationInputReviewConclusion(*proposal.ValidationInputReview) != runtimecontract.RuntimeValidationInputInconclusive {
			continue
		}
		info := runtimecontract.RuntimeValidationInputReviewInformation{SubjectHash: proposal.ValidationInputReview.SubjectHash, ReviewRef: resolution.SubjectRef,
			TicketID: resolution.TicketID, ResolutionID: resolution.ResolutionID, Summary: resolution.Summary}
		if err := info.Validate(); err != nil {
			return err
		}
	}
	snapshot := ledger.Clone()
	snapshot.History = nil
	history := runtimecontract.RuntimeSemanticPlanningGeneration{RetiredAtStateRevision: a.StateRevision + 1,
		Reason: runtimecontract.RuntimeSemanticPlanningRetirementInputReviewInformation, InputReviewResolutionID: resolution.ResolutionID, Snapshot: snapshot}
	if err := history.Validate(); err != nil {
		return err
	}
	if err := retireSemanticPlanningRequestSessions(a, ledger, "input_review_information_supplied"); err != nil {
		return err
	}
	a.SemanticPlanning = runtimecontract.RuntimeSemanticPlanningLedger{
		SchemaVersion: ledger.SchemaVersion, Phase: runtimecontract.RuntimeSemanticPlanningRequired, DefinitionSeed: ledger.DefinitionSeed.Clone(),
		CatalogRefreshAttempts: ledger.CatalogRefreshAttempts, FeasibilityGaps: ledger.FeasibilityGaps,
		History: append(ledger.History, history),
	}
	return nil
}

// RuntimeInputReviewInformation reads only exact persisted Decision resolution
// lineage. It does not treat a summary or a ticket status as review approval.
func RuntimeInputReviewInformation(a RuntimeAggregate) ([]runtimecontract.RuntimeValidationInputReviewInformation, error) {
	var ret []runtimecontract.RuntimeValidationInputReviewInformation
	for _, history := range a.SemanticPlanning.History {
		if history.Reason != runtimecontract.RuntimeSemanticPlanningRetirementInputReviewInformation {
			continue
		}
		var resolution *runtimecontract.RuntimeGovernanceResolutionRecord
		for _, record := range a.GovernanceResolutions {
			if record.ResolutionID == history.InputReviewResolutionID {
				value := record
				resolution = &value
				break
			}
		}
		if resolution == nil || resolution.Outcome != runtimecontract.RuntimeGovernanceResolutionOutcomeContinue || !runtimeInputReviewResolutionMatches(a, history.Snapshot, resolution.Resolution()) {
			return nil, fmt.Errorf("input review retirement has no exact information resolution")
		}
		document, _ := runtimeInputReviewDocument(history.Snapshot)
		for _, proposal := range document.Proposals {
			if proposal.ValidationInputReview == nil || runtimecontract.RuntimeValidationInputReviewConclusion(*proposal.ValidationInputReview) != runtimecontract.RuntimeValidationInputInconclusive {
				continue
			}
			info := runtimecontract.RuntimeValidationInputReviewInformation{SubjectHash: proposal.ValidationInputReview.SubjectHash, ReviewRef: resolution.SubjectRef,
				TicketID: resolution.TicketID, ResolutionID: resolution.ResolutionID, Summary: resolution.Summary}
			if err := info.Validate(); err != nil {
				return nil, err
			}
			ret = append(ret, info)
		}
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].SubjectHash < ret[j].SubjectHash })
	for i := 1; i < len(ret); i++ {
		if ret[i-1].SubjectHash == ret[i].SubjectHash {
			return nil, fmt.Errorf("input review repeats information ticket for the same exact subject")
		}
	}
	return ret, nil
}

func validateRuntimeInputReviewAggregate(a RuntimeAggregate) error {
	information, err := RuntimeInputReviewInformation(a)
	if err != nil {
		return err
	}
	var preceding []runtimecontract.RuntimeValidationInputReviewInformation
	validateManifest := func(manifest *runtimecontract.RuntimeSemanticPlanManifest) error {
		if manifest == nil {
			return nil
		}
		expected := runtimecontract.RuntimeSemanticInputReviewInformationForCatalogs(preceding, manifest.OwnerCatalogs)
		if !reflect.DeepEqual(manifest.InputReviewInformation, expected) {
			return fmt.Errorf("semantic manifest input information has no exact preceding resolution preimage")
		}
		return nil
	}
	for _, history := range a.SemanticPlanning.History {
		if err := validateManifest(history.Snapshot.Manifest); err != nil {
			return err
		}
		if history.Reason == runtimecontract.RuntimeSemanticPlanningRetirementInputReviewInformation {
			if history.Snapshot.Phase != runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted {
				return fmt.Errorf("input review information retired a non-review transaction")
			}
			for _, source := range information {
				if source.ResolutionID == history.InputReviewResolutionID {
					preceding = append(preceding, source)
				}
			}
		}
	}
	if err := validateManifest(a.SemanticPlanning.Manifest); err != nil {
		return err
	}
	if document, found := runtimeInputReviewDocument(a.SemanticPlanning); found {
		conclusion := runtimeInputReviewConclusion(document)
		if conclusion != "" && conclusion != runtimecontract.RuntimeValidationInputConsistent &&
			a.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted && a.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed {
			return fmt.Errorf("non-consistent input review advanced beyond document observation")
		}
	}
	if a.Facts.Failure != nil && a.Facts.Failure.GovernanceDependency != nil && a.Facts.Failure.GovernanceDependency.RequestedAction == runtimecontract.RuntimeValidationInputReviewInformationAction {
		document, found := runtimeInputReviewDocument(a.SemanticPlanning)
		if !found || runtimeInputReviewConclusion(document) != runtimecontract.RuntimeValidationInputInconclusive ||
			!runtimecontract.SameExternalDependency(*a.Facts.Failure.GovernanceDependency, runtimeInputReviewDependency(a, a.SemanticPlanning, document)) {
			return fmt.Errorf("input review governance requires the exact complete inconclusive report")
		}
	}
	return nil
}
