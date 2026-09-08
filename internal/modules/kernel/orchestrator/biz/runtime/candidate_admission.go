package runtime

import (
	"strings"

	candidateadmission "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/candidateadmission"
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
	candidateAdmissionCodeInvalid                    = candidateadmission.CodeInvalid
	candidateAdmissionCodeReplaysRejectedShape       = candidateadmission.CodeRejectedShape
	candidateAdmissionCodePartiallyReplaysRejected   = candidateadmission.CodePartialReplay
	candidateAdmissionCodeReplaysRejectedCandidate   = candidateadmission.CodeRejectedCandidate
	candidateAdmissionCodeReplaysNoProgressShape     = candidateadmission.CodeNoProgressShape
	candidateAdmissionCodeActionEvidenceMismatch     = candidateadmission.CodeActionEvidenceMismatch
	candidateAdmissionCodeTargetOutsideGrounding     = candidateadmission.CodeTargetOutsideGrounding
	candidateAdmissionCodeRepairClosureMismatch      = candidateadmission.CodeRepairClosureMismatch
	candidateAdmissionCodeCreationScopeMissingTarget = candidateadmission.CodeCreationScopeMissingTarget
	candidateAdmissionCodeOwnerTargetEffectOverlap   = candidateadmission.CodeOwnerTargetEffectOverlap
)

// CandidateAdmissionResult is a typed, expected candidate outcome. Error is
// reserved for aggregate corruption, CAS, repository, or other internal faults.
// Every rejected result carries the SM-3 owner-local correction seal, so no
// other component re-derives retry or terminal state from raw reason text.
type CandidateAdmissionResult struct {
	Disposition CandidateAdmissionDisposition
	Failure     *workorchruntimecontract.RuntimeCandidateFailure
	// Correction is the pure same-manifest correction or final terminal seal for
	// a complete rejection. Accepted results leave it nil.
	Correction *workorchruntimecontract.RuntimeCandidateCorrectionDirective
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

func candidateAdmissionResultFromFailure(failure *workorchruntimecontract.RuntimeCandidateFailure) CandidateAdmissionResult {
	if failure == nil {
		return acceptedCandidateAdmission()
	}
	return CandidateAdmissionResult{Disposition: CandidateAdmissionRejectedContract, Failure: failure}
}

func sealCandidateAdmissionRejection(result CandidateAdmissionResult, manifest workorchruntimecontract.RuntimeCandidateContractManifest, candidate workorchruntimecontract.RuntimeCandidate, frontier candidateCorrectionFrontier) CandidateAdmissionResult {
	if result.Accepted() || result.Failure == nil {
		return result
	}
	if result.Failure.Diagnostic != nil && result.Failure.Diagnostic.RejectedShapeHash == "" {
		result.Failure.Diagnostic.RejectedShapeHash = workorchruntimecontract.StableRuntimeCandidateSemanticShape(candidate)
	}
	if result.Failure.Diagnostic != nil && result.Failure.Diagnostic.ManifestHash == "" {
		result.Failure.Diagnostic.ManifestHash = manifest.Hash
	}
	sealed, directive, err := sealCandidateRejectionWithFrontier(result.Failure, manifest, frontier)
	if err != nil {
		// SealCandidateRejection is owner-local and currently infallible for a
		// non-nil failure; treat any error as a contract defect rather than an
		// ordinary undefined correction.
		defect := &workorchruntimecontract.RuntimeCandidateFailure{
			Class:     workorchruntimecontract.RuntimeCandidateFailureClassInternal,
			Code:      "runtime_contract_defect",
			Signature: "runtime-contract-defect",
			Reason:    err.Error(),
		}
		result.Failure = defect
		result.Correction = &workorchruntimecontract.RuntimeCandidateCorrectionDirective{
			Disposition:  workorchruntimecontract.RuntimeCandidateCorrectionRejectedFinal,
			ManifestHash: manifest.Hash,
			TerminalCode: "runtime_contract_defect",
		}
		return result
	}
	result.Failure = sealed
	result.Correction = directive
	return result
}

// EvaluateCandidateAdmissionWithManifest is the manifest-bound SM-3 single
// candidate evaluator entry. The caller supplies the exact manifest carried by
// the request session; this function only verifies it against the current sealed
// authority and evaluates the decoded candidate in a fixed first-failure order.
// It does not mutate the aggregate, choose a convergence state, call EventHub, or
// invoke the reducer. The owner-local policy seals every complete rejection
// with correctable_once or rejected_final.
func EvaluateCandidateAdmissionWithManifest(aggregate RuntimeAggregate, manifest workorchruntimecontract.RuntimeCandidateContractManifest, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	input := candidateAdmissionEvaluationInput(aggregate, manifest, candidate)
	var failure *workorchruntimecontract.RuntimeCandidateFailure
	if err := validateAttemptCandidateCrossesPersistedWitness(aggregate, manifest, candidate); err != nil {
		failure = &workorchruntimecontract.RuntimeCandidateFailure{
			Class: workorchruntimecontract.RuntimeCandidateFailureClassContract,
			Code:  candidateadmission.CodeRejectedCandidate, Signature: candidateadmission.SignatureRejectedCandidate,
			Reason: err.Error(),
			Diagnostic: &workorchruntimecontract.RuntimeCandidateFailureDiagnostic{
				Path: "$.operations", Actual: "correction_witness_mismatch", ManifestHash: manifest.Hash,
				RejectedShapeHash: workorchruntimecontract.StableRuntimeCandidateSemanticShape(candidate),
			},
		}
	} else {
		failure = candidateadmission.Evaluate(input)
	}
	if failure == nil {
		return acceptedCandidateAdmission()
	}
	result := candidateAdmissionResultFromFailure(failure)
	return sealCandidateAdmissionRejection(result, manifest, candidate, candidateCorrectionFrontierForEvaluation(input))
}

// candidateAdmissionEvaluationInput is the only aggregate projection for the
// pure CandidateEvaluator. Authority/lineage filtering remains with Runtime;
// the focused package receives detached typed facts and cannot mutate them.
func candidateAdmissionEvaluationInput(aggregate RuntimeAggregate, manifest workorchruntimecontract.RuntimeCandidateContractManifest, candidate workorchruntimecontract.RuntimeCandidate) candidateadmission.EvaluationInput {
	ledger := aggregate.Facts.Repair.Normalized()
	var noProgressShapes []workorchruntimecontract.RuntimeRejectedOperationShape
	if ledger.FocusID == aggregate.Facts.Focus.ID {
		noProgressShapes = append([]workorchruntimecontract.RuntimeRejectedOperationShape(nil), ledger.NoProgressOperationShapes...)
	}
	return candidateadmission.EvaluationInput{
		Manifest:                  manifest,
		Decision:                  aggregate.Decision,
		Facts:                     aggregate.Facts,
		Candidate:                 candidate,
		RejectedOperationShapes:   ActiveRejectedOperationShapes(aggregate),
		PriorFailedProposalShapes: activeFailedCandidateProposalShapes(aggregate),
		NoProgressOperationShapes: noProgressShapes,
		ValidationGap:             candidateValidationGapFacts(aggregate),
		RepairClosure:             candidateRepairClosureFacts(aggregate),
	}
}

// Reuse the same reverse proof as aggregate fresh-load. Neither NeedValidation
// nor an empty repair purpose alone can retire semantic/confirmed repair target
// authority. This detached projection never edits the closure or archives.
func candidateRepairClosureFacts(aggregate RuntimeAggregate) candidateadmission.RepairClosureFacts {
	if err := validateConfirmedRepairFreshValidationContinuation(aggregate); err != nil {
		return candidateadmission.RepairClosureFacts{LineageError: err.Error()}
	}
	semanticRevision, err := semanticRepairFreshValidationEvidenceRevision(aggregate)
	if err != nil {
		return candidateadmission.RepairClosureFacts{LineageError: err.Error()}
	}
	if semanticRevision != "" {
		return candidateadmission.RepairClosureFacts{CompletedRepairEvidenceRevision: semanticRevision}
	}
	if runtimeConfirmedRepairOwnsFreshValidationDirective(aggregate.Facts) {
		return candidateadmission.RepairClosureFacts{
			CompletedRepairEvidenceRevision: aggregate.Facts.RepairDirective.Normalized().SupersedesEvidenceRevision,
		}
	}
	return candidateadmission.RepairClosureFacts{}
}

// rejectCandidateRepairClosure is the SM-3 counterpart to the SM-2 repair
// directive. A repair may contain multiple candidate dispatches. Operations
// that can close a CWI must contribute to the same sealed gap closure;
// diagnostic operations that cannot satisfy a binding for the current
// decision are permitted without inventing a binding or a repair target. This
// does not infer domain semantics: it checks only persisted proof declarations
// and owner-grounded target references.
func rejectCandidateRepairClosure(aggregate RuntimeAggregate, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	return candidateAdmissionResultFromFailure(candidateadmission.RepairClosureFailure(aggregate.Facts, aggregate.Decision, candidate, candidateRepairClosureFacts(aggregate)))
}

// rejectCandidateOutsideGroundedTargets is the SM-3 fence for a focus whose
// profile requires target grounding. The reducer owns which targets are
// authoritative; admission only verifies that a state-changing proposal does
// not bypass that sealed set. It deliberately does not classify file names or
// infer target roles from a provider explanation.
func rejectCandidateOutsideGroundedTargets(facts workorchruntimecontract.RuntimeFacts, decision workorchruntimecontract.RuntimeDecision, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	return candidateAdmissionResultFromFailure(candidateadmission.GroundedTargetFailure(facts, decision, candidate))
}

// GroundedRepairTargets returns the single persisted target-precedence set for
// the current focus. A committed, owner-grounded FailureTarget is a repair
// retarget and therefore overrides the original Focus target without mutating
// that Focus declaration. This is an SM-2 fact projection, not target
// discovery: all members must already originate in the sealed focus, owner
// observation, failure fact, target selection, or repair directive. SM-3
// admission and prerequisite re-acquisition consume the same set so a
// resource-change refresh cannot silently return to a superseded focus target.
func GroundedRepairTargets(facts workorchruntimecontract.RuntimeFacts) []string {
	return runtimeTargetAuthorityPlanForFacts(facts).targets()
}

func candidateAdmissionInvalidSignature(candidate workorchruntimecontract.RuntimeCandidate) string {
	return candidateadmission.InvalidSignature(candidate)
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
	latestAcceptedRevision := latestAcceptedEvidenceRevision(facts, aggregate.CoverageProofLineage)
	// A completed repair is causation-only, so it correctly does not
	// appear in the CWI-credit projection above. Its exact receipt-chain fence
	// nevertheless retires failures from the pre-repair workspace: fresh
	// validation must be allowed to rerun the same sealed check on new state.
	// The same reverse proof controls both this replay fence and closure targets.
	if repair := candidateRepairClosureFacts(aggregate); evidenceRevisionAfter(repair.CompletedRepairEvidenceRevision, latestAcceptedRevision) {
		latestAcceptedRevision = repair.CompletedRepairEvidenceRevision
	}
	ret := make([]workorchruntimecontract.RuntimeRejectedOperationShape, 0, len(facts.Evidence.Rejected))
	for _, evidence := range facts.Evidence.Rejected {
		dispatch, ok := dispatchByID[strings.TrimSpace(evidence.DispatchID)]
		if !ok {
			continue
		}
		if !rejectedEvidenceAppliesToCurrentFocus(facts, evidence, dispatch) {
			continue
		}
		if latestAcceptedRevision != "" && !evidenceRevisionAfter(evidence.Revision, latestAcceptedRevision) {
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
	return workorchruntimecontract.NormalizeRuntimeRejectedOperationShapes(ret)
}

// rejectedEvidenceAppliesToCurrentFocus keeps rejected receipt lineage local
// to the SM-3 transaction that actually executed it. Rejected owner receipts
// deliberately receive no CWI credit, so their EvidenceFact can have an empty
// CoverageWorkItemIDs set even while later focuses remain queued. In that
// shape the completed dispatch's immutable FocusID is the attribution fact.
// The persisted fact must name that same focus independently so a coordinated
// current-focus rewrite cannot reactivate an earlier rejected shape. This is
// candidate-context attribution only and grants no proof.
func rejectedEvidenceAppliesToCurrentFocus(facts workorchruntimecontract.RuntimeFacts, evidence workorchruntimecontract.EvidenceFact, dispatch SealedDispatch) bool {
	dispatchFocusID := strings.TrimSpace(dispatch.Transaction.FocusID)
	return dispatchFocusID != "" &&
		dispatchFocusID == strings.TrimSpace(evidence.FocusID) &&
		workorchruntimecontract.EvidenceBelongsToCurrentFocusForDiagnosticLineage(facts, evidence)
}

func rejectCandidateReplay(aggregate RuntimeAggregate, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	finding, found := candidateadmission.RejectedShapeReplay(candidate, ActiveRejectedOperationShapes(aggregate))
	if !found {
		return acceptedCandidateAdmission()
	}
	return rejectedCandidateAdmission(finding.Code, finding.Signature, finding.Reason)
}

// activeFailedCandidateProposalShapes projects the correction-specific replay
// fence for the current authority contract. Rejected owner evidence is covered
// separately by ActiveRejectedOperationShapes.
func activeFailedCandidateProposalShapes(aggregate RuntimeAggregate) []string {
	transaction := aggregate.Decision.AuthorityTransaction()
	contractID := workorchruntimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction)
	failedShapes := make([]string, 0, 1)
	for _, attempt := range aggregate.CandidateAttempts {
		if attempt.Attempt != 0 || attempt.State != workorchruntimecontract.RuntimeCandidateAttemptStateFailed || attempt.Proposal == nil ||
			workorchruntimecontract.CandidateAttemptAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, attempt) != contractID {
			continue
		}
		failedShapes = append(failedShapes, workorchruntimecontract.StableRuntimeCandidateSemanticShape(*attempt.Proposal))
	}
	return failedShapes
}

func rejectCandidateMissingValidationGap(aggregate RuntimeAggregate, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	return candidateAdmissionResultFromFailure(candidateadmission.MissingValidationFailure(
		aggregate.Facts,
		aggregate.Decision,
		candidate,
		candidateValidationGapFacts(aggregate),
	))
}

func candidateValidationGapFacts(aggregate RuntimeAggregate) candidateadmission.ValidationGapFacts {
	facts := aggregate.Facts.Normalized()
	missing := missingEvidenceKinds(facts, aggregate.CoverageProofLineage)
	hasAcceptedStateChange := false
	for _, evidence := range facts.Evidence.Accepted {
		if evidence.Kind == workorchruntimecontract.EvidenceKindStateChange &&
			runtimeEvidenceAppliesToCurrentCoverageWithLineage(facts, evidence, aggregate.CoverageProofLineage) {
			hasAcceptedStateChange = true
			break
		}
	}
	return candidateadmission.ValidationGapFacts{
		MissingEvidenceKinds:       append([]string(nil), missing...),
		HasAcceptedStateChange:     hasAcceptedStateChange,
		LatestFailureStateChanging: latestFocusFailureIsStateChange(facts),
	}
}

func latestAcceptedEvidenceRevision(facts workorchruntimecontract.RuntimeFacts, proofLineages ...[]workorchruntimecontract.RuntimeCoverageProofLineage) string {
	proofLineage := runtimeCoverageProofLineageArgument(proofLineages)
	latest := ""
	for _, evidence := range facts.Evidence.Accepted {
		if !runtimeAcceptedEvidenceCreditsCurrentFocusWithLineage(facts, evidence, proofLineage) {
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
		if !workorchruntimecontract.EvidenceBelongsToCurrentFocusForDiagnosticLineage(facts, evidence) || evidence.Kind != workorchruntimecontract.EvidenceKindStateChange {
			continue
		}
		if latestRevision == "" || evidenceRevisionAfter(evidence.Revision, latestRevision) {
			latestRevision = evidence.Revision
			isStateChange = true
		}
	}
	for _, evidence := range facts.Evidence.Rejected {
		if !workorchruntimecontract.EvidenceBelongsToCurrentFocusForDiagnosticLineage(facts, evidence) || evidence.Kind != workorchruntimecontract.EvidenceKindValidation {
			continue
		}
		if latestRevision == "" || evidenceRevisionAfter(evidence.Revision, latestRevision) {
			return false
		}
	}
	return isStateChange
}
