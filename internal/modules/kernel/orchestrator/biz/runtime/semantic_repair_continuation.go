package runtime

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// semanticRepairFreshValidationEvidenceRevision reverse-proves the semantic
// repair's target-free handoff from its retired binding and completed owner
// receipt set. Candidate admission, replay suppression and fresh-load consume
// this same read-only projection. It grants no CWI credit and never changes a
// directive, binding, archive, or state machine.
func semanticRepairFreshValidationEvidenceRevision(aggregate RuntimeAggregate) (string, error) {
	facts := aggregate.Facts
	if len(facts.Focus.CoverageWorkItemIDs) != 1 {
		return "", nil
	}
	coverageID := facts.Focus.CoverageWorkItemIDs[0]
	index := runtimeCWICoverageIndex(facts.Coverage, coverageID)
	if index < 0 || runtimeValidationFailureRepairReopenReason(facts.Coverage[index]) != runtimeCWIReopenReasonValidationFailureRepairSettled {
		return "", nil
	}
	invalid := func(detail string) (string, error) {
		return "", fmt.Errorf("runtime semantic repair fresh-validation continuation: %s", detail)
	}
	item := facts.Coverage[index]
	if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied ||
		(aggregate.Decision.State == runtimecontract.ConvergenceStateFailed && aggregate.Decision.Terminal != nil && facts.Failure != nil) {
		return "", nil
	}
	lineage := item.ReopenLineage[len(item.ReopenLineage)-1]
	var trigger runtimecontract.EvidenceFact
	triggers := 0
	for _, fact := range facts.Evidence.Accepted {
		if string(fact.ID) == lineage.TriggerRef {
			trigger, triggers = fact, triggers+1
		}
	}
	if triggers != 1 {
		return invalid("handoff has no unique final repair evidence")
	}
	// An exact fresh validation consumes this handoff whether it passed or
	// failed. A failure may open another bounded repair, but cannot revive the
	// old semantic continuation or reuse its target authority.
	if item.CurrentBindingRef != nil {
		observed := append(append([]runtimecontract.EvidenceFact(nil), facts.Evidence.Accepted...), facts.Evidence.Rejected...)
		for _, fact := range observed {
			if fact.Kind == runtimecontract.EvidenceKindValidation && runtimecontract.EvidenceRevisionAfter(fact.Revision, trigger.Revision) &&
				containsString(fact.BindingRefs, item.CurrentBindingRef.BindingID) {
				if facts.RepairDirective != nil && strings.HasPrefix(facts.RepairDirective.ID, "validation-failure-repair/") && facts.RepairDirective.SupersedesEvidenceRevision == trigger.Revision {
					return invalid("consumed handoff retained its old directive")
				}
				return "", nil
			}
		}
	}
	if facts.RepairDirective == nil {
		return invalid("outstanding handoff has no directive")
	}
	directive := facts.RepairDirective.Normalized()
	if directive.Closure == nil || len(facts.Focus.CoverageWorkItemIDs) != 1 || directive.FocusID != facts.Focus.ID ||
		directive.ID != fmt.Sprintf("validation-failure-repair/%s/%d/fresh-validation", facts.Focus.ID, facts.Repair.RepairAttempts) ||
		directive.Trigger != runtimecontract.RuntimeRepairTriggerRejectedValidation || directive.RequiredState != runtimecontract.ConvergenceStateNeedValidation ||
		directive.Purpose != "" || len(directive.TargetRefs) != 0 || directive.SupersedesEvidenceRevision == "" {
		return invalid("handoff is not canonical and target-free")
	}
	ref := lineage.PreviousBindingRef
	repair, found, err := runtimeSemanticValidationFailureRepairForBinding(aggregate, coverageID, ref)
	if err != nil {
		return invalid(err.Error())
	}
	closure := directive.Closure.Normalized()
	if !found || !repair.RequireFreshValidation || closure.GapSignature != repair.FailureSignature ||
		!closure.RequireValidationAfterStateChange || !sameCoverageIDs(closure.CoverageWorkItemIDs, facts.Focus.CoverageWorkItemIDs) ||
		!sameCoverageIDs(directive.CoverageWorkItemIDs, facts.Focus.CoverageWorkItemIDs) ||
		!sameRuntimeStringSet(closure.RequiredEvidenceKinds, []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}) {
		return invalid("closure differs from the retired semantic repair contract")
	}
	if trigger.Revision != directive.SupersedesEvidenceRevision ||
		!runtimecontract.EvidenceRevisionAfter(trigger.Revision, closure.BaselineEvidenceRevision) {
		return invalid("handoff has no unique final repair evidence revision")
	}
	var dispatch SealedDispatch
	dispatches := 0
	for _, candidate := range aggregate.CompletedDispatches {
		if candidate.ID == trigger.DispatchID {
			dispatch, dispatches = candidate, dispatches+1
		}
	}
	if dispatches != 1 || dispatch.State != DispatchStateCommitted || dispatch.Candidate.StateChange == nil ||
		len(dispatch.CompletedOperationIDs) != candidateOperationCount(dispatch.Candidate) {
		return invalid("repair dispatch is missing, duplicate or incomplete")
	}
	var binding runtimecontract.CoverageEvidenceBinding
	bindings := 0
	for _, candidate := range dispatch.CoverageBindings {
		if candidate.ID == ref.BindingID && candidate.Hash == ref.BindingHash && candidate.FocusID == ref.FocusID && candidate.CoverageWorkItemID == coverageID {
			binding, bindings = candidate, bindings+1
		}
	}
	if bindings != 1 || !sameRuntimeTargetRefs(binding.ProspectiveTargetRefs, closure.GroundedTargetRefs) {
		return invalid("repair closure does not match its exact retired binding")
	}
	manifests := 0
	for _, session := range aggregate.CandidateRequestSessions {
		manifest := session.CandidateManifest
		if manifest != nil && manifest.Hash == dispatch.CandidateManifestHash && session.Transaction == dispatch.Transaction &&
			manifest.RepairClosureID == closure.ID && runtimecontract.RuntimeCoverageEvidenceBindingProjectionsEqual(manifest.Bindings, dispatch.CoverageBindings) {
			manifests++
		}
	}
	if manifests != 1 {
		return invalid("closure has no unique pre-seal candidate manifest")
	}
	var targets []string
	finalRevision := ""
	for _, operation := range candidateRuntimeOperations(dispatch.Candidate) {
		execution, found := operationExecutionRecord(dispatch.OperationExecutions, operation.ID)
		if !found || execution.State != runtimecontract.OperationExecutionEvidenceCommitted {
			return invalid("repair operation has not committed evidence")
		}
		receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
		if !found || execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) ||
			execution.OwnerReceiptRef != "runtime-receipt/"+receipt.ExecutionKey || !receipt.Changed {
			return invalid("repair operation has no exact changed owner receipt")
		}
		matchedBindings, matchedCoverage, diagnostic := coverageEvidenceCreditForReceipt([]runtimecontract.CoverageEvidenceBinding{binding}, operation, runtimecontract.EvidenceKindStateChange, &receipt)
		if diagnostic != "" || len(matchedBindings) != 1 || matchedBindings[0] != binding.ID ||
			len(matchedCoverage) != 1 || matchedCoverage[0] != coverageID {
			return invalid("repair receipt does not prove its exact binding: " + diagnostic)
		}
		var evidence runtimecontract.EvidenceFact
		matches := 0
		for _, fact := range facts.Evidence.Accepted {
			if fact.ID == execution.EvidenceFactID {
				evidence, matches = fact, matches+1
			}
		}
		if matches != 1 || runtimecontract.StableRuntimeEvidenceFactHash(evidence) != execution.EvidenceFactHash ||
			evidence.DispatchID != dispatch.ID || evidence.OperationID != operation.ID || evidence.Kind != runtimecontract.EvidenceKindStateChange ||
			evidence.AuthoritySnapshotID != dispatch.Transaction.AuthoritySnapshotID || weakRuntimeEvidence(evidence) ||
			!sameRuntimeStringSet(evidence.BindingRefs, operation.BindingRefs) || !sameRuntimeTargetRefs(evidence.TargetRefs, receipt.TargetRefs) ||
			!sameCoverageIDs(evidence.CoverageWorkItemIDs, closure.CoverageWorkItemIDs) || !containsString(aggregate.ProcessedEventIDs, string(evidence.ID)) {
			return invalid("repair evidence differs from its exact execution/receipt lineage")
		}
		targets = append(targets, receipt.TargetRefs...)
		if runtimecontract.EvidenceRevisionAfter(evidence.Revision, finalRevision) {
			finalRevision = evidence.Revision
		}
	}
	if !sameRuntimeTargetRefs(targets, binding.ProspectiveTargetRefs) || finalRevision != trigger.Revision {
		return invalid("handoff lacks the complete same-dispatch all-of repair receipt set")
	}
	return finalRevision, nil
}
