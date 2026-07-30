package runtimev2

import (
	"errors"
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
	candidateAdmissionCodeInvalid                     = "candidate_contract_invalid"
	candidateAdmissionCodeReplaysRejectedShape        = "candidate_replays_rejected_shape"
	candidateAdmissionCodePartiallyReplaysRejected    = "candidate_partially_replays_rejected_shape"
	candidateAdmissionCodeReplaysRejectedCandidate    = "candidate_replays_rejected_candidate_shape"
	candidateAdmissionCodeReplaysNoProgressShape      = "candidate_replays_no_progress_shape"
	candidateAdmissionCodeActionEvidenceMismatch      = "candidate_action_evidence_mismatch"
	candidateAdmissionCodeTargetOutsideGrounding      = "candidate_target_outside_grounding"
	candidateAdmissionCodeRepairClosureMismatch       = "candidate_repair_closure_mismatch"
	candidateAdmissionCodeCreationScopeMissingTarget  = "candidate_creation_scope_missing_file_target"
	candidateAdmissionSignatureInvalid                = "candidate-contract-invalid"
	candidateAdmissionSignatureReplaysRejectedShape   = "candidate-replays-rejected-shape"
	candidateAdmissionSignaturePartialReplay          = "candidate-partially-replays-rejected-shape"
	candidateAdmissionSignatureRejectedCandidate      = "candidate-replays-rejected-candidate-shape"
	candidateAdmissionSignatureNoProgressReplay       = "candidate-replays-no-progress-shape"
	candidateAdmissionSignatureActionEvidenceMismatch = "candidate-action-evidence-mismatch"
	candidateAdmissionSignatureTargetOutsideGrounding = "candidate-target-outside-grounding"
	candidateAdmissionSignatureRepairClosureMismatch  = "candidate-repair-closure-mismatch"
	candidateAdmissionSignatureCreationScopeTarget    = "candidate-creation-scope-missing-file-target"
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

func rejectedCandidateAdmissionDiagnostic(code, signature string, err error) CandidateAdmissionResult {
	result := rejectedCandidateAdmission(code, signature, err.Error())
	var diagnostic *workorchruntimecontract.RuntimeCandidateContractDiagnostic
	if errors.As(err, &diagnostic) {
		result.Failure.Diagnostic = workorchruntimecontract.RuntimeCandidateFailureDiagnosticFromContractDiagnostic(diagnostic)
	}
	return result
}

func rejectedCandidateAdmissionWithAllowedTargets(code, signature, reason string, actual, allowed []string, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	result := rejectedCandidateAdmission(code, signature, reason)
	result.Failure.Diagnostic = &workorchruntimecontract.RuntimeCandidateFailureDiagnostic{
		Path:              "$.operations.targetRefs",
		Actual:            strings.Join(actual, ","),
		Allowed:           append([]string(nil), allowed...),
		RejectedShapeHash: workorchruntimecontract.StableRuntimeCandidateSemanticShape(candidate),
	}
	return result
}

func sealCandidateAdmissionRejection(result CandidateAdmissionResult, manifest workorchruntimecontract.RuntimeCandidateContractManifest, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	if result.Accepted() || result.Failure == nil {
		return result
	}
	if result.Failure.Diagnostic != nil && result.Failure.Diagnostic.RejectedShapeHash == "" {
		result.Failure.Diagnostic.RejectedShapeHash = workorchruntimecontract.StableRuntimeCandidateSemanticShape(candidate)
	}
	if result.Failure.Diagnostic != nil && result.Failure.Diagnostic.ManifestHash == "" {
		result.Failure.Diagnostic.ManifestHash = manifest.Hash
	}
	sealed, directive, err := SealCandidateRejection(result.Failure, manifest)
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
	// Fixed evaluator order (plan §7 first-failure):
	// 1. manifest identity / authority match
	// 2. wire/shape, required/forbidden field (ValidateRuntimeCandidateAgainstManifest)
	// 3. operation kind/variant/parameter (same pure validator + ValidateWithActivation)
	// 4-5. effect/coverage/dependency (ValidateWithActivation)
	// 6. semantic replay / prior attempt
	// 7. action/evidence and target-policy grounding (owner feasibility stays N5)
	// 8. accepted
	var result CandidateAdmissionResult
	if err := workorchruntimecontract.ValidateRuntimeCandidateContractManifestForDecision(manifest, aggregate.Decision, workorchruntimecontract.RuntimeCandidateManifestOwnerFactsFromResources((aggregate.Facts).Resources)); err != nil {
		result = rejectedCandidateAdmission(candidateAdmissionCodeInvalid, candidateAdmissionInvalidSignature(candidate), fmt.Sprintf("runtime candidate manifest contract: %v", err))
	} else if err := workorchruntimecontract.ValidateRuntimeCandidateAgainstManifest(manifest, candidate); err != nil {
		result = rejectedCandidateAdmissionDiagnostic(candidateAdmissionCodeInvalid, candidateAdmissionInvalidSignature(candidate), err)
	} else if err := candidate.ValidateWithActivation(aggregate.Decision, aggregate.Facts.Activation); err != nil {
		result = rejectedCandidateAdmission(candidateAdmissionCodeInvalid, candidateAdmissionInvalidSignature(candidate), err.Error())
	} else if result = rejectCandidateReplay(aggregate, candidate); !result.Accepted() {
		// first-failure already selected
	} else if result = rejectCandidateAttemptSemanticReplay(aggregate, candidate); !result.Accepted() {
		// first-failure already selected
	} else if result = rejectCandidateNoProgressReplay(aggregate, candidate); !result.Accepted() {
		// first-failure already selected
	} else if result = rejectCandidateRepairClosure(aggregate, candidate); !result.Accepted() {
		// first-failure already selected
	} else if result = rejectCandidateMissingValidationGap(aggregate.Facts, aggregate.Decision, candidate); !result.Accepted() {
		// first-failure already selected
	} else if result = rejectCandidateOutsideGroundedTargets(aggregate.Facts, aggregate.Decision, candidate); !result.Accepted() {
		// first-failure already selected
	} else {
		return acceptedCandidateAdmission()
	}
	return sealCandidateAdmissionRejection(result, manifest, candidate)
}

// rejectCandidateRepairClosure is the SM-3 counterpart to the SM-2 repair
// directive.  A repair may contain multiple candidate dispatches, but each
// executable operation must contribute to the same sealed gap closure.  This
// does not infer domain semantics: it checks only persisted coverage and
// owner-grounded target references.
func rejectCandidateRepairClosure(aggregate RuntimeAggregate, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	directive := aggregate.Facts.RepairDirective
	if directive == nil || directive.Closure == nil || directive.FocusID != aggregate.Facts.Focus.ID {
		return acceptedCandidateAdmission()
	}
	closure := directive.Closure.Normalized()
	if aggregate.Decision.RequiredOutput.RepairClosureID != closure.ID {
		return rejectedCandidateAdmission(
			candidateAdmissionCodeRepairClosureMismatch,
			candidateAdmissionSignatureRepairClosureMismatch,
			"sealed repair decision is not bound to the active repair closure",
		)
	}
	for _, operation := range candidateRuntimeOperations(candidate) {
		if !coverageIDsIntersect(closure.CoverageWorkItemIDs, operation.CoverageWorkItemIDs) {
			return rejectedCandidateAdmission(
				candidateAdmissionCodeRepairClosureMismatch,
				candidateAdmissionSignatureRepairClosureMismatch,
				"runtime repair candidate operation is outside the sealed repair coverage",
			)
		}
		if len(closure.GroundedTargetRefs) > 0 && !runtimeTargetRefsIntersect(closure.GroundedTargetRefs, operation.TargetRefs) {
			return rejectedCandidateAdmissionWithAllowedTargets(
				candidateAdmissionCodeRepairClosureMismatch,
				candidateAdmissionSignatureRepairClosureMismatch,
				"runtime repair candidate operation does not target the sealed repair closure",
				operation.TargetRefs,
				closure.GroundedTargetRefs,
				candidate,
			)
		}
	}
	return acceptedCandidateAdmission()
}

// rejectCandidateOutsideGroundedTargets is the SM-3 fence for a focus whose
// profile requires target grounding. The reducer owns which targets are
// authoritative; admission only verifies that a state-changing proposal does
// not bypass that sealed set. It deliberately does not classify file names or
// infer target roles from a provider explanation.
func rejectCandidateOutsideGroundedTargets(facts workorchruntimecontract.RuntimeFacts, decision workorchruntimecontract.RuntimeDecision, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	facts = facts.Normalized()
	if decision.State != workorchruntimecontract.ConvergenceStateNeedStateChange || !facts.Focus.RequireTargetGrounding {
		return acceptedCandidateAdmission()
	}
	if hasOwnerGroundedCreationScope(facts) {
		selection := currentFocusTargetSelection(facts).Normalized()
		durableFileEffect := false
		for _, operation := range candidateRuntimeOperations(candidate) {
			descriptor, known := workorchruntimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
			if !known || !workorchruntimecontract.RuntimeEffectIsDurable(workorchruntimecontract.ResolveExpectedEffectScope(operation, facts.Activation.MCPCapabilities)) {
				continue
			}
			if descriptor.OwnerModuleID != workorchruntimecontract.RuntimeModuleFile || len(operation.TargetRefs) != 1 || operation.TargetRefs[0] != selection.ProspectiveTargetRef {
				return rejectedCandidateAdmissionWithAllowedTargets(
					candidateAdmissionCodeTargetOutsideGrounding,
					candidateAdmissionSignatureTargetOutsideGrounding,
					"empty workspace creation scope permits durable effects only on the sealed prospective File target",
					operation.TargetRefs,
					[]string{selection.ProspectiveTargetRef},
					candidate,
				)
			}
			// The sealed prospective target is profile-declared and File-owner
			// verified. It is not a generic durable side-effect grant.
			durableFileEffect = true
		}
		if durableFileEffect {
			return acceptedCandidateAdmission()
		}
		return rejectedCandidateAdmissionWithAllowedTargets(
			candidateAdmissionCodeCreationScopeMissingTarget,
			candidateAdmissionSignatureCreationScopeTarget,
			"empty workspace creation scope requires a durable File operation targeting the sealed prospective workspace-relative path",
			nil,
			[]string{selection.ProspectiveTargetRef},
			candidate,
		)
	}
	requiredTargets := GroundedRepairTargets(facts)
	if ownerGroundedTargetScopeMaySelectNewFileTarget(facts) {
		selection := currentFocusTargetSelection(facts).Normalized()
		durableEffect := false
		for _, operation := range candidateRuntimeOperations(candidate) {
			descriptor, known := workorchruntimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
			if !known || !workorchruntimecontract.RuntimeEffectIsDurable(workorchruntimecontract.ResolveExpectedEffectScope(operation, facts.Activation.MCPCapabilities)) {
				continue
			}
			durableEffect = true
			if descriptor.OwnerModuleID == workorchruntimecontract.RuntimeModuleFile && len(operation.TargetRefs) != 0 {
				// A scope authorizes the File owner to adjudicate a concrete
				// workspace-relative target. It does not authorize an operation
				// without a target or bypass the later owner feasibility check.
				continue
			}
			if descriptor.AllowsGroundedWorkspaceTarget && runtimeTargetRefsAllWithin(operation.TargetRefs, groundedWorkspaceScopeTargets(&selection)) {
				// This is a deliberately narrow exception for a technical owner
				// whose descriptor is explicit. The target must already be in the
				// File owner's sealed scope; File retains new-target authority.
				continue
			}
			return rejectedCandidateAdmissionWithAllowedTargets(
				candidateAdmissionCodeTargetOutsideGrounding,
				candidateAdmissionSignatureTargetOutsideGrounding,
				"owner-grounded workspace scope permits only a durable File operation or an explicitly target-bound operation within the sealed File inventory",
				operation.TargetRefs,
				groundedWorkspaceScopeTargets(&selection),
				candidate,
			)
		}
		if durableEffect {
			return acceptedCandidateAdmission()
		}
		return rejectedCandidateAdmissionWithAllowedTargets(
			candidateAdmissionCodeTargetOutsideGrounding,
			candidateAdmissionSignatureTargetOutsideGrounding,
			"owner-grounded workspace scope requires a durable File operation or an explicitly target-bound operation within the sealed File inventory",
			nil,
			groundedWorkspaceScopeTargets(&selection),
			candidate,
		)
	}
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
	return rejectedCandidateAdmissionWithAllowedTargets(
		candidateAdmissionCodeTargetOutsideGrounding,
		candidateAdmissionSignatureTargetOutsideGrounding,
		"runtime state-changing candidate does not target the sealed focus, failure, or repair target set",
		actualTargetsFromCandidate(candidate),
		requiredTargets,
		candidate,
	)
}

// groundedWorkspaceScopeTargets returns the existing, File-owner-observed
// targets for the current scope. CandidateTargetRefs is the canonical bounded
// inventory; TargetRefs is retained as a defensive fallback for a persisted
// scope produced before the inventory field was populated. This helper never
// discovers a target and is only used by SM-3 to verify a descriptor-declared
// technical operation.
func actualTargetsFromCandidate(candidate workorchruntimecontract.RuntimeCandidate) []string {
	actual := make([]string, 0)
	for _, operation := range candidateRuntimeOperations(candidate) {
		actual = append(actual, operation.TargetRefs...)
	}
	return actual
}

func groundedWorkspaceScopeTargets(selection *workorchruntimecontract.RuntimeTargetSelection) []string {
	if selection == nil {
		return nil
	}
	value := selection.Normalized()
	if len(value.CandidateTargetRefs) != 0 {
		return value.CandidateTargetRefs
	}
	return value.TargetRefs
}

// runtimeTargetRefsAllWithin requires every submitted target to be a member
// of the sealed File scope. Unlike intersection, an empty or partially
// outside target set must not be able to piggyback on one admissible target.
func runtimeTargetRefsAllWithin(targetRefs, allowedTargets []string) bool {
	if len(targetRefs) == 0 || len(allowedTargets) == 0 {
		return false
	}
	allowed := make(map[string]struct{}, len(allowedTargets))
	for _, target := range allowedTargets {
		if target = strings.TrimSpace(target); target != "" {
			allowed[target] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return false
	}
	for _, target := range targetRefs {
		target = strings.TrimSpace(target)
		if target == "" {
			return false
		}
		if _, found := allowed[target]; !found {
			return false
		}
	}
	return true
}

// ownerGroundedTargetScopeMaySelectNewFileTarget preserves the File owner's
// bounded workspace-scope form for an otherwise untargeted focus. A scope is
// not a blanket override: once the focus, an owner-observed failure, the
// current failure, or a repair directive supplies a concrete target, that
// higher-precedence target is the only admissible durable repair target.
//
// Keeping this distinction here is an SM-3 admission fence. File still owns
// path feasibility; this code neither discovers a path nor interprets source
// semantics.
func ownerGroundedTargetScopeMaySelectNewFileTarget(facts workorchruntimecontract.RuntimeFacts) bool {
	facts = facts.Normalized()
	if !hasOwnerGroundedTargetScope(facts) || len(facts.Focus.TargetRefs) != 0 {
		return false
	}
	if facts.Resources.FailureTarget != nil && len(facts.Resources.FailureTarget.TargetRefs) != 0 {
		return false
	}
	if facts.Failure != nil && len(facts.Failure.TargetRefs) != 0 {
		return false
	}
	return facts.RepairDirective == nil || len(facts.RepairDirective.TargetRefs) == 0
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
	if facts.Resources.FailureTarget != nil && len(facts.Resources.FailureTarget.TargetRefs) > 0 {
		return append([]string(nil), facts.Resources.FailureTarget.TargetRefs...)
	}
	if len(facts.Focus.TargetRefs) > 0 {
		return append([]string(nil), facts.Focus.TargetRefs...)
	}
	if targets := ownerGroundedAssessmentGapTargets(facts); len(targets) > 0 {
		return targets
	}
	if selection := currentFocusTargetSelection(facts); selection != nil && targetSelectionHasConcreteTarget(*selection) {
		return append([]string(nil), selection.TargetRefs...)
	}
	if selection := currentFocusTargetSelection(facts); hasOwnerGroundedCreationScope(facts) && selection != nil {
		return []string{selection.ProspectiveTargetRef}
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

// rejectCandidateAttemptSemanticReplay is the correction-specific replay fence.
// Rejected owner evidence is represented by operation shapes above; this fence
// closes the gap where an otherwise complete, structurally valid candidate was
// rejected before any owner receipt existed. The first failed proposal is the
// only persisted comparison source, and the pure contract hash deliberately
// ignores operation IDs and JSON order while retaining variant, target, effect,
// coverage, dependency graph and execution parameters.
func rejectCandidateAttemptSemanticReplay(aggregate RuntimeAggregate, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	transaction := aggregate.Decision.AuthorityTransaction()
	contractID := workorchruntimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction)
	wanted := workorchruntimecontract.StableRuntimeCandidateSemanticShape(candidate)
	for _, attempt := range aggregate.CandidateAttempts {
		if attempt.Attempt != 0 || attempt.State != workorchruntimecontract.RuntimeCandidateAttemptStateFailed || attempt.Proposal == nil ||
			workorchruntimecontract.CandidateAttemptAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, attempt) != contractID {
			continue
		}
		if workorchruntimecontract.StableRuntimeCandidateSemanticShape(*attempt.Proposal) != wanted {
			continue
		}
		return rejectedCandidateAdmission(
			candidateAdmissionCodeReplaysRejectedCandidate,
			candidateAdmissionSignatureRejectedCandidate,
			"runtime candidate replays the prior rejected complete candidate shape; change kind, targets, coverage, dependencies, or structural parameters before retry",
		)
	}
	return acceptedCandidateAdmission()
}

func rejectCandidateMissingValidationGap(facts workorchruntimecontract.RuntimeFacts, decision workorchruntimecontract.RuntimeDecision, candidate workorchruntimecontract.RuntimeCandidate) CandidateAdmissionResult {
	facts = facts.Normalized()
	if decision.State != workorchruntimecontract.ConvergenceStateNeedStateChange {
		return acceptedCandidateAdmission()
	}
	if facts.RepairDirective != nil && facts.RepairDirective.FocusID == facts.Focus.ID &&
		facts.RepairDirective.RequiredState == workorchruntimecontract.ConvergenceStateNeedStateChange {
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
