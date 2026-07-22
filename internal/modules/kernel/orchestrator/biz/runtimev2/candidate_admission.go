package runtimev2

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CandidateAdmissionDisposition is the owner-local outcome of evaluating a
// provider candidate against the current persisted SM-3 transaction. It is not
// an EventHub contract and must never be reconstructed from an error message.
type CandidateAdmissionDisposition string

const (
	CandidateAdmissionAccepted         CandidateAdmissionDisposition = "accepted"
	CandidateAdmissionRejectedContract CandidateAdmissionDisposition = "rejected_contract"
)

const (
	candidateAdmissionCodeInvalid                     = "candidate_admission_invalid"
	candidateAdmissionCodeReplaysRejectedShape        = "candidate_replays_rejected_shape"
	candidateAdmissionCodePartiallyReplaysRejected    = "candidate_partially_replays_rejected_shape"
	candidateAdmissionCodeReplaysNoProgressShape      = "candidate_replays_no_progress_shape"
	candidateAdmissionCodeActionEvidenceMismatch      = "candidate_action_evidence_mismatch"
	candidateAdmissionCodeTargetOutsideGrounding      = "candidate_target_outside_grounding"
	candidateAdmissionCodeDeclaredNewFile             = "candidate_declared_new_file_contract"
	candidateAdmissionSignatureInvalid                = "candidate-admission-invalid"
	candidateAdmissionSignatureReplaysRejectedShape   = "candidate-replays-rejected-shape"
	candidateAdmissionSignaturePartialReplay          = "candidate-partially-replays-rejected-shape"
	candidateAdmissionSignatureNoProgressReplay       = "candidate-replays-no-progress-shape"
	candidateAdmissionSignatureActionEvidenceMismatch = "candidate-action-evidence-mismatch"
	candidateAdmissionSignatureTargetOutsideGrounding = "candidate-target-outside-grounding"
	candidateAdmissionSignatureDeclaredNewFile        = "candidate-declared-new-file-contract"
)

// CandidateAdmissionResult is a typed, expected candidate outcome. Error is
// reserved for aggregate corruption, CAS, repository, or other internal faults.
type CandidateAdmissionResult struct {
	Disposition CandidateAdmissionDisposition
	Failure     *workorchruntimecontract.RuntimeCandidateFailure
}

func (r CandidateAdmissionResult) Accepted() bool {
	return r.Disposition == CandidateAdmissionAccepted && r.Failure == nil
}

func acceptedCandidateAdmission() CandidateAdmissionResult {
	return CandidateAdmissionResult{Disposition: CandidateAdmissionAccepted}
}

func rejectedCandidateAdmission(code, signature, reason string) CandidateAdmissionResult {
	failure := workorchruntimecontract.RuntimeCandidateFailure{
		Class:     workorchruntimecontract.RuntimeCandidateFailureClassContract,
		Code:      strings.TrimSpace(code),
		Signature: strings.TrimSpace(signature),
		Reason:    strings.TrimSpace(reason),
	}
	return CandidateAdmissionResult{Disposition: CandidateAdmissionRejectedContract, Failure: &failure}
}

// EvaluateCandidateAdmission is the sole owner-local admission policy for a
// candidate before its proposal or seal transition. It reads persisted runtime
// lineage but does not mutate the aggregate, choose a new RuntimeAction, call
// EventHub, or invoke the reducer.
func EvaluateCandidateAdmission(aggregate RuntimeAggregate, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	if err := candidate.ValidateWithActivation(aggregate.Decision, aggregate.Facts.Activation); err != nil {
		return rejectedCandidateAdmission(candidateAdmissionCodeInvalid, candidateAdmissionInvalidSignature(candidate), err.Error())
	}
	if result := rejectCandidateReplay(aggregate, candidate); !result.Accepted() {
		return result
	}
	if result := rejectCandidateNoProgressReplay(aggregate, candidate); !result.Accepted() {
		return result
	}
	if result := rejectCandidateMissingValidationGap(aggregate.Facts, aggregate.Decision, candidate); !result.Accepted() {
		return result
	}
	if result := rejectCandidateOutsideGroundedTargets(aggregate.Facts, aggregate.Decision, candidate); !result.Accepted() {
		return result
	}
	if result := rejectCandidateDeclaredNewFileShape(aggregate.Facts, aggregate.Decision, candidate); !result.Accepted() {
		return result
	}
	return acceptedCandidateAdmission()
}

// rejectCandidateDeclaredNewFileShape preserves the activation-sealed create
// precondition. It is deliberately structural: no file path semantics are
// inferred here, and File owner still verifies absence immediately before
// execution.
func rejectCandidateDeclaredNewFileShape(facts workorchruntimecontract.RuntimeFacts, decision workorchruntimecontract.RuntimeDecision, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	facts = facts.Normalized()
	declaration := facts.Activation.TargetDeclaration
	if decision.Action != workorchruntimecontract.RuntimeActionProduceStateChange || declaration == nil {
		return acceptedCandidateAdmission()
	}
	value := declaration.Normalized()
	if value.Validate() != nil || value.Mode != workorchruntimecontract.RuntimeTargetDeclarationNewFile {
		return acceptedCandidateAdmission()
	}
	operations := candidateRuntimeOperations(candidate)
	for _, operation := range operations {
		if !workorchruntimecontract.RuntimeEffectIsDurable(workorchruntimecontract.ResolveExpectedEffectScope(operation, facts.Activation.MCPCapabilities)) {
			continue
		}
		operation = operation.Normalized()
		if operation.Kind != workorchruntimecontract.RuntimeBlockKindPatchApply || len(operation.TargetRefs) != 1 || operation.TargetRefs[0] != value.TargetRef ||
			candidateOperationStringParameter(operation, "targetRef") != value.TargetRef || candidateOperationStringParameter(operation, "patchType") != "file_write" || candidateOperationStringParameter(operation, "baseHash") != "" {
			return rejectedCandidateAdmission(
				candidateAdmissionCodeDeclaredNewFile,
				candidateAdmissionSignatureDeclaredNewFile,
				"activation-sealed new target requires exactly patch/apply:file_write on the declared target without baseHash",
			)
		}
	}
	return acceptedCandidateAdmission()
}

func candidateOperationStringParameter(operation workorchruntimecontract.RuntimeOperation, name string) string {
	for _, parameter := range operation.Normalized().Parameters {
		if parameter.Name == name && parameter.String != nil {
			return strings.TrimSpace(*parameter.String)
		}
	}
	return ""
}

// rejectCandidateOutsideGroundedTargets is the SM-3 fence for a focus whose
// profile requires target grounding. The reducer owns which targets are
// authoritative; admission only verifies that a state-changing proposal does
// not bypass that sealed set. It deliberately does not classify file names or
// infer target roles from a provider explanation.
func rejectCandidateOutsideGroundedTargets(facts workorchruntimecontract.RuntimeFacts, decision workorchruntimecontract.RuntimeDecision, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	facts = facts.Normalized()
	if decision.Action != workorchruntimecontract.RuntimeActionProduceStateChange || !facts.Focus.RequireTargetGrounding {
		return acceptedCandidateAdmission()
	}
	requiredTargets := GroundedRepairTargets(facts)
	if len(requiredTargets) == 0 {
		// Bounded discovery without a selected target remains admissible only
		// through owner operation inventories. A profile adapter, not the core,
		// supplies any domain-specific target selection.
		return acceptedCandidateAdmission()
	}
	for _, operation := range candidateRuntimeOperations(candidate) {
		if !workorchruntimecontract.RuntimeEffectIsDurable(workorchruntimecontract.ResolveExpectedEffectScope(operation, facts.Activation.MCPCapabilities)) {
			continue
		}
		if runtimeTargetRefsIntersect(requiredTargets, operation.TargetRefs) {
			return acceptedCandidateAdmission()
		}
	}
	return rejectedCandidateAdmission(
		candidateAdmissionCodeTargetOutsideGrounding,
		candidateAdmissionSignatureTargetOutsideGrounding,
		"runtime state-changing candidate does not target the sealed focus, failure, or repair target set",
	)
}

// GroundedRepairTargets returns the single persisted target-precedence set for
// the current focus. It is an SM-2 fact projection, not target discovery: all
// members must already originate in the sealed focus, owner observation,
// failure fact, target selection, or repair directive. SM-3 admission and
// prerequisite re-acquisition must consume this same set so a resource-change
// refresh cannot silently fall back to an empty Focus.TargetRefs.
func GroundedRepairTargets(facts workorchruntimecontract.RuntimeFacts) []string {
	if len(facts.Focus.TargetRefs) > 0 {
		return append([]string(nil), facts.Focus.TargetRefs...)
	}
	if facts.Resources.FailureTarget != nil && len(facts.Resources.FailureTarget.TargetRefs) > 0 {
		return append([]string(nil), facts.Resources.FailureTarget.TargetRefs...)
	}
	if facts.Failure != nil && len(facts.Failure.TargetRefs) > 0 {
		return append([]string(nil), facts.Failure.TargetRefs...)
	}
	if selection := facts.Resources.TargetSelection; selection != nil && selection.Disposition == workorchruntimecontract.RuntimeTargetSelectionSelected && len(selection.TargetRefs) > 0 {
		return append([]string(nil), selection.TargetRefs...)
	}
	if facts.RepairDirective != nil && len(facts.RepairDirective.TargetRefs) > 0 {
		return append([]string(nil), facts.RepairDirective.TargetRefs...)
	}
	return nil
}

func rejectCandidateNoProgressReplay(aggregate RuntimeAggregate, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	ledger := aggregate.Facts.Repair.Normalized()
	if ledger.FocusID != aggregate.Facts.Focus.ID || len(ledger.NoProgressOperationShapes) == 0 {
		return acceptedCandidateAdmission()
	}
	blocked := make(map[string]struct{}, len(ledger.NoProgressOperationShapes))
	for _, shape := range ledger.NoProgressOperationShapes {
		blocked[shape.Hash] = struct{}{}
	}
	for _, operation := range candidateRuntimeOperations(candidate) {
		if _, found := blocked[workorchruntimecontract.StableRuntimeOperationShape(operation)]; found {
			return rejectedCandidateAdmission(
				candidateAdmissionCodeReplaysNoProgressShape,
				candidateAdmissionSignatureNoProgressReplay,
				"runtime candidate replays an accepted operation shape that did not resolve the active gap; change kind, targets, or structural parameters before retry",
			)
		}
	}
	return acceptedCandidateAdmission()
}

func candidateAdmissionInvalidSignature(candidate workorchruntimecontract.RuntimeCandidate) string {
	// Invalid candidates cannot be persisted as proposals, but duplicate delivery
	// still needs a durable identity independent of provider error wording. The
	// candidate hash is a pure structural DTO hash and does not select authority.
	hash := workorchruntimecontract.StableRuntimeCandidateHash(candidate)
	if len(hash) > 20 {
		hash = hash[:20]
	}
	return candidateAdmissionSignatureInvalid + "-" + hash
}

// ActiveRejectedOperationShapes derives volatile C3/C4 candidate context from
// persisted evidence, sealed dispatches, and failed feasibility records. This
// is Orchestrator authority, not a shared contract-package helper.
func ActiveRejectedOperationShapes(aggregate RuntimeAggregate) []workorchruntimecontract.RuntimeRejectedOperationShape {
	facts := aggregate.Facts.Normalized()
	dispatchByID := make(map[string]SealedDispatch, len(aggregate.CompletedDispatches))
	for _, dispatch := range aggregate.CompletedDispatches {
		dispatchByID[strings.TrimSpace(dispatch.ID)] = dispatch
	}
	latestAcceptedRevision := latestAcceptedEvidenceRevision(facts)
	ret := make([]workorchruntimecontract.RuntimeRejectedOperationShape, 0, len(facts.Evidence.Rejected))
	for _, evidence := range facts.Evidence.Rejected {
		if !workorchruntimecontract.EvidenceAppliesToCurrentFocus(facts, evidence) {
			continue
		}
		if latestAcceptedRevision != "" && !evidenceRevisionAfter(evidence.Revision, latestAcceptedRevision) {
			continue
		}
		dispatch, ok := dispatchByID[strings.TrimSpace(evidence.DispatchID)]
		if !ok {
			continue
		}
		for _, operation := range candidateRuntimeOperations(dispatch.Candidate) {
			if operation.Normalized().ID != strings.TrimSpace(evidence.OperationID) {
				continue
			}
			ret = append(ret, workorchruntimecontract.RuntimeRejectedOperationShape{
				Hash:             workorchruntimecontract.StableRuntimeOperationShape(operation),
				Operation:        operation.Normalized(),
				EvidenceID:       evidence.ID,
				FailureClass:     evidence.FailureClass,
				FailureCode:      evidence.FailureCode,
				FailureSignature: evidence.FailureSignature,
				FailureMessage:   evidence.FailureMessage,
			})
		}
	}
	for _, attempt := range aggregate.CandidateAttempts {
		if attempt.State != workorchruntimecontract.RuntimeCandidateAttemptStateFailed || attempt.Failure == nil ||
			attempt.Failure.Class != workorchruntimecontract.RuntimeCandidateFailureClassInfeasible ||
			attempt.Transaction.FocusID != facts.Focus.ID || attempt.Proposal == nil ||
			(latestAcceptedRevision != "" && attempt.Transaction.EvidenceRevision != latestAcceptedRevision && !evidenceRevisionAfter(attempt.Transaction.EvidenceRevision, latestAcceptedRevision)) {
			continue
		}
		for _, feasibility := range attempt.Feasibilities {
			if feasibility.Feasible || feasibility.Failure == nil {
				continue
			}
			for _, operation := range candidateRuntimeOperations(*attempt.Proposal) {
				if operation.Normalized().ID != strings.TrimSpace(feasibility.OperationID) {
					continue
				}
				ret = append(ret, workorchruntimecontract.RuntimeRejectedOperationShape{
					Hash:             workorchruntimecontract.StableRuntimeOperationShape(operation),
					Operation:        operation.Normalized(),
					FailureClass:     feasibility.Failure.Class,
					FailureCode:      feasibility.Failure.Code,
					FailureSignature: feasibility.Failure.Signature,
					FailureMessage:   feasibility.Failure.Message,
				})
			}
		}
	}
	return workorchruntimecontract.NormalizeRuntimeRejectedOperationShapes(ret)
}

func rejectCandidateReplay(aggregate RuntimeAggregate, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	shapes := ActiveRejectedOperationShapes(aggregate)
	if len(shapes) == 0 {
		return acceptedCandidateAdmission()
	}
	rejected := make(map[string]struct{}, len(shapes))
	for _, shape := range shapes {
		rejected[shape.Hash] = struct{}{}
	}
	operations := candidateRuntimeOperations(candidate)
	if len(operations) == 0 {
		return acceptedCandidateAdmission()
	}
	replayCount := 0
	for _, operation := range operations {
		if _, found := rejected[workorchruntimecontract.StableRuntimeOperationShape(operation)]; found {
			replayCount++
		}
	}
	if replayCount == 0 {
		return acceptedCandidateAdmission()
	}
	if replayCount == len(operations) {
		return rejectedCandidateAdmission(
			candidateAdmissionCodeReplaysRejectedShape,
			candidateAdmissionSignatureReplaysRejectedShape,
			"runtime candidate replays active rejected operation shape(s); change kind, targets, or structural parameters before retry",
		)
	}
	return rejectedCandidateAdmission(
		candidateAdmissionCodePartiallyReplaysRejected,
		candidateAdmissionSignaturePartialReplay,
		fmt.Sprintf("runtime candidate includes %d operation(s) that replay active rejected shape(s); remove or rewrite those operations", replayCount),
	)
}

func rejectCandidateMissingValidationGap(facts workorchruntimecontract.RuntimeFacts, decision workorchruntimecontract.RuntimeDecision, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	facts = facts.Normalized()
	if decision.Action != workorchruntimecontract.RuntimeActionProduceStateChange {
		return acceptedCandidateAdmission()
	}
	if facts.RepairDirective != nil && facts.RepairDirective.FocusID == facts.Focus.ID &&
		facts.RepairDirective.RequiredAction == workorchruntimecontract.RuntimeActionProduceStateChange {
		return acceptedCandidateAdmission()
	}
	missing := workorchruntimecontract.MissingRequiredEvidenceKinds(facts)
	if len(missing) != 1 || missing[0] != workorchruntimecontract.EvidenceKindValidation {
		return acceptedCandidateAdmission()
	}
	hasAcceptedStateChange := false
	for _, evidence := range facts.Evidence.Accepted {
		if workorchruntimecontract.EvidenceAppliesToCurrentFocus(facts, evidence) && evidence.Kind == workorchruntimecontract.EvidenceKindStateChange {
			hasAcceptedStateChange = true
			break
		}
	}
	if !hasAcceptedStateChange || !latestFocusFailureIsStateChange(facts) || len(candidateRuntimeOperations(candidate)) == 0 {
		return acceptedCandidateAdmission()
	}
	return rejectedCandidateAdmission(
		candidateAdmissionCodeActionEvidenceMismatch,
		candidateAdmissionSignatureActionEvidenceMismatch,
		"runtime candidate continues state-changing retries while only validation evidence is missing; produce_validation instead",
	)
}

func latestAcceptedEvidenceRevision(facts workorchruntimecontract.RuntimeFacts) string {
	latest := ""
	for _, evidence := range facts.Evidence.Accepted {
		if !workorchruntimecontract.EvidenceAppliesToCurrentFocus(facts, evidence) {
			continue
		}
		if latest == "" || evidenceRevisionAfter(evidence.Revision, latest) {
			latest = evidence.Revision
		}
	}
	return latest
}

func latestFocusFailureIsStateChange(facts workorchruntimecontract.RuntimeFacts) bool {
	latestRevision := ""
	isStateChange := false
	for _, evidence := range facts.Evidence.Rejected {
		if !workorchruntimecontract.EvidenceAppliesToCurrentFocus(facts, evidence) || evidence.Kind != workorchruntimecontract.EvidenceKindStateChange {
			continue
		}
		if latestRevision == "" || evidenceRevisionAfter(evidence.Revision, latestRevision) {
			latestRevision = evidence.Revision
			isStateChange = true
		}
	}
	for _, evidence := range facts.Evidence.Rejected {
		if !workorchruntimecontract.EvidenceAppliesToCurrentFocus(facts, evidence) || evidence.Kind != workorchruntimecontract.EvidenceKindValidation {
			continue
		}
		if latestRevision == "" || evidenceRevisionAfter(evidence.Revision, latestRevision) {
			return false
		}
	}
	return isStateChange
}
